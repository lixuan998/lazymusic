package daemon

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"lazymusic/internal/catalog"
	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/video"
)

type frameEngine struct {
	*engine.Queue
	frame video.Frame
	calls int
}

func (e *frameEngine) VideoFrame(context.Context, string) (video.Frame, error) {
	e.calls++
	return e.frame, nil
}
func TestVideoIPCRequiresFrontendAndCurrentSourceAndNeverPersistsFrames(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	var data bytes.Buffer
	_ = png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 2)))
	e := &frameEngine{Queue: engine.NewQueue(session.Playback{}), frame: video.Frame{Source: music.Local, ID: "movie", Width: 4, Height: 2, PNG: data.Bytes()}}
	opts.Factory = func(context.Context, session.Playback) (engine.Engine, error) { return e, nil }
	s, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	c := serveTest(t, s, dir)
	ctx := context.Background()
	if _, err = c.VideoFrame(ctx, "movie"); err == nil {
		t.Fatal("unattached process obtained pictures")
	}
	if _, err = c.Attach(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = c.Command(ctx, Command{Op: "append", Tracks: []music.Track{{Source: music.Local, ID: "movie", MediaType: "video"}}})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := c.VideoFrame(ctx, "movie")
	if err != nil || !frame.Valid() {
		t.Fatal("picture lost over IPC", err)
	}
	r, _ := http.NewRequestWithContext(ctx, "POST", "http://lazymusic.local/v1/video", bytes.NewBufferString(`{"source":"local","id":"movie"}`))
	r.Header.Set("X-Lazymusic-Client", c.token)
	response, err := c.http.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.Header.Get("Content-Type") != "image/png" || !bytes.Equal(wire, data.Bytes()) {
		t.Fatal("video IPC recompressed or base64-wrapped every picture", err)
	}
	if err = c.call(ctx, http.MethodPost, "/v1/video", catalog.Subject{Source: music.Apple, ID: "movie"}, &frame); err == nil {
		t.Fatal("cross-source video accepted")
	}
	e.frame.Width = 3
	if _, err = c.VideoFrame(ctx, "movie"); err == nil {
		t.Fatal("invalid frame accepted")
	}
	e.frame.Width = 4
	if _, err = c.Command(ctx, Command{Op: "clear"}); err != nil {
		t.Fatal(err)
	}
	calls := e.calls
	frame, err = c.VideoFrame(ctx, "movie")
	if err != nil || len(frame.PNG) != 0 || e.calls != calls {
		t.Fatal("old picture survived queue clear", err)
	}
	ui := session.DefaultUI()
	ui.Video = true
	if err = c.SaveUI(ctx, ui, 1); err != nil {
		t.Fatal(err)
	}
	saved, err := session.Load(dir)
	if err != nil || !saved.UI.Video {
		t.Fatal("video view preference not saved", err)
	}
	dataOnDisk, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil || bytes.Contains(dataOnDisk, []byte(`"png"`)) || bytes.Contains(dataOnDisk, []byte("iVBOR")) {
		t.Fatal("frame data leaked into session", err)
	}
}
