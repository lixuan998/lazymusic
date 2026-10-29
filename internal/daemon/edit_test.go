package daemon

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"lazymusic/internal/catalog"
	"lazymusic/internal/session"
)

type editBrowser struct {
	mu               sync.Mutex
	reads, writes    int
	started, release chan struct{}
	err              error
}

func (b *editBrowser) Browse(context.Context, catalog.Query) (catalog.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads++
	return catalog.Page{Items: []catalog.Item{{ID: fmt.Sprint(b.writes)}}}, nil
}
func (b *editBrowser) Actions(context.Context, catalog.Subject) (catalog.Actions, error) {
	return catalog.Actions{AddLibrary: true}, nil
}
func (b *editBrowser) Mutate(ctx context.Context, _ catalog.Mutation) (catalog.MutationResult, error) {
	if b.started != nil {
		close(b.started)
		select {
		case <-b.release:
		case <-ctx.Done():
			return catalog.MutationResult{}, ctx.Err()
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writes++
	return catalog.MutationResult{Message: "Accepted", Pending: true}, b.err
}

func TestCatalogWritesRequireFrontendAndInvalidateCacheWithoutStartingAudio(t *testing.T) {
	for _, writeError := range []error{nil, fmt.Errorf("reply lost after acceptance")} {
		dir := t.TempDir()
		b := &editBrowser{err: writeError}
		opts := testOptions(dir)
		opts.Browser = b
		s, err := New(opts)
		if err != nil {
			t.Fatal(err)
		}
		c := serveTest(t, s, dir)
		ctx := context.Background()
		mutation := catalog.Mutation{Source: "local", Op: catalog.AddLibrary, Subject: catalog.Subject{Source: "local", ID: "song"}}
		if _, err = c.Mutate(ctx, mutation); err == nil {
			t.Fatal("unattached client wrote library")
		}
		if _, err = c.Attach(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = c.Actions(ctx, catalog.Subject{Source: "apple", ID: "song"}); err == nil {
			t.Fatal("cross-source actions accepted")
		}
		mutation.Source = "apple"
		if _, err = c.Mutate(ctx, mutation); err == nil {
			t.Fatal("cross-source write accepted")
		}
		mutation.Source = "local"
		q := catalog.Query{Source: "local", Page: "library"}
		_, _ = c.Browse(ctx, q)
		_, _ = c.Browse(ctx, q)
		if b.reads != 1 {
			t.Fatal("read cache missing")
		}
		result, err := c.Mutate(ctx, mutation)
		if (err != nil) != (writeError != nil) {
			t.Fatalf("write error not returned: %+v %v", result, err)
		}
		p, err := c.Browse(ctx, q)
		if err != nil || b.writes != 1 || b.reads != 2 || p.Items[0].ID != "1" || s.State().EngineReady {
			t.Fatal("write retried, left stale cache or initialized audio")
		}
		if err = c.Detach(ctx, session.DefaultUI(), 1); err != nil {
			t.Fatal(err)
		}
		if _, err = c.Mutate(ctx, mutation); err == nil {
			t.Fatal("detached client wrote library")
		}
	}
}

func TestSlowCloudWriteDoesNotBlockPlaybackCommands(t *testing.T) {
	dir := t.TempDir()
	b := &editBrowser{started: make(chan struct{}), release: make(chan struct{})}
	opts := testOptions(dir)
	opts.Browser = b
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	ctx := context.Background()
	_, _ = c.Attach(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := c.Mutate(ctx, catalog.Mutation{Source: "local", Op: catalog.CreatePlaylist, Name: "New"})
		done <- err
	}()
	select {
	case <-b.started:
	case <-time.After(3 * time.Second):
		t.Fatal("write did not start")
	}
	controlCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = c.Command(controlCtx, Command{Op: "repeat"})
	close(b.release)
	if err != nil {
		t.Fatalf("playback blocked by cloud request: %v", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

type staleReadBrowser struct {
	editBrowser
	first                    sync.Once
	readStarted, readRelease chan struct{}
}

func (b *staleReadBrowser) Browse(ctx context.Context, q catalog.Query) (catalog.Page, error) {
	first := false
	b.first.Do(func() { first = true })
	if first {
		close(b.readStarted)
		select {
		case <-b.readRelease:
		case <-ctx.Done():
			return catalog.Page{}, ctx.Err()
		}
		return catalog.Page{Items: []catalog.Item{{ID: "stale"}}}, nil
	}
	return b.editBrowser.Browse(ctx, q)
}

func TestReadStartedBeforeWriteCannotRefillInvalidatedCache(t *testing.T) {
	dir := t.TempDir()
	b := &staleReadBrowser{readStarted: make(chan struct{}), readRelease: make(chan struct{})}
	opts := testOptions(dir)
	opts.Browser = b
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	ctx := context.Background()
	_, _ = c.Attach(ctx)
	q := catalog.Query{Source: "local", Page: "library"}
	done := make(chan error, 1)
	go func() { _, err := c.Browse(ctx, q); done <- err }()
	select {
	case <-b.readStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not start")
	}
	_, err = c.Mutate(ctx, catalog.Mutation{Source: "local", Op: catalog.CreatePlaylist, Name: "New"})
	close(b.readRelease)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	p, err := c.Browse(ctx, q)
	if err != nil || len(p.Items) != 1 || p.Items[0].ID != "1" {
		t.Fatalf("stale browse repopulated cache: %+v %v", p, err)
	}
}
