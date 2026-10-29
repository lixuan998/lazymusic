package daemon

import (
	"context"
	"lazymusic/internal/catalog"
	"testing"
)

type detailsBrowser struct{ testBrowser }

func (detailsBrowser) Inspect(_ context.Context, subject catalog.Subject) (catalog.Details, error) {
	return catalog.Details{Title: subject.ID, Credits: []catalog.Credit{{Name: "Composer", Roles: []string{"Composer"}}}}, nil
}

func TestInspectIPCIsSourceScopedAndDoesNotStartAudio(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	opts.Browser = detailsBrowser{}
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	d, err := c.Inspect(context.Background(), catalog.Subject{Source: "local", ID: "work", Credits: true})
	if err != nil || d.Title != "work" || len(d.Credits) != 1 || s.State().EngineReady {
		t.Fatalf("metadata IPC: %+v %v", d, err)
	}
	if _, err = c.Inspect(context.Background(), catalog.Subject{Source: "apple", ID: "work"}); err == nil {
		t.Fatal("accepted a work from another source")
	}
	if _, err = c.Inspect(context.Background(), catalog.Subject{Source: "local"}); err == nil {
		t.Fatal("accepted an empty identity")
	}
}
