package tui

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"lazymusic/internal/music"
	"lazymusic/internal/termimage"
	"lazymusic/internal/video"
)

type videoAPI struct {
	fakeAPI
	calls int
	frame video.Frame
}

func TestVideoCadenceIncludesDecodeAndUploadTimeAndEditsExistingPicture(t *testing.T) {
	m, api := videoModel(t)
	start := time.Now()
	m.video.nextFrame = start.Add(video.Interval)
	if delay := m.videoDelay(start.Add(20 * time.Millisecond)); delay != video.Interval-20*time.Millisecond {
		t.Fatal("processing time was added on top of frame interval", delay)
	}
	if m.videoDelay(start.Add(time.Second)) != 0 {
		t.Fatal("late frame added another sleep")
	}
	m.videoMessage(m.syncVideo()())
	_, done, ok := m.TerminalOutput(m.syncVideo()())
	if !ok {
		t.Fatal("first picture missing")
	}
	m.videoMessage(done)
	m.videoMessage(uv.KittyGraphicsEvent{Options: kitty.Options{ID: m.videoImageID()}, Payload: []byte("OK")})
	m.video.ticking = false
	m.video.frame = api.frame
	m.video.frame.Position += video.Interval
	out, _, ok := m.TerminalOutput(m.syncVideo()())
	if !ok || !strings.Contains(out, "a=f,") || strings.Contains(out, "a=T,") {
		t.Fatal("video reallocated its terminal image on every frame")
	}
	if !m.graphics.ready() {
		t.Fatal("video frame probe revoked native image support")
	}
}

func (a *videoAPI) VideoFrame(context.Context, string) (video.Frame, error) {
	a.calls++
	return a.frame, nil
}
func videoModel(t *testing.T) (*Model, *videoAPI) {
	m := nativeArtModel(t)
	m.UI.Focus = 1
	m.UI.Video = true
	m.State.Source.Capabilities.Video = true
	m.State.Playback.Track = &music.Track{Source: m.State.Source.ID, ID: "movie", MediaType: "video", Kind: "music-videos", Title: "Movie"}
	m.State.Playback.Playing = true
	m.State.Playback.Status = "playing"
	m.video.observed = m.videoKey()
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 64, 36)))
	api := &videoAPI{frame: video.Frame{Source: m.State.Source.ID, ID: "movie", Width: 64, Height: 36, PNG: b.Bytes(), Position: time.Second}}
	m.api = api
	return m, api
}
func TestContentVideoUsesOwnImageAndLateFramesCannotReopenHiddenView(t *testing.T) {
	m, api := videoModel(t)
	read := m.syncVideo()
	if read == nil {
		t.Fatal("video did not request a frame")
	}
	msg := read()
	m.videoMessage(msg)
	write := m.syncVideo()
	if write == nil {
		t.Fatal("decoded frame not uploaded")
	}
	data, done, ok := m.TerminalOutput(write())
	if !ok || !strings.Contains(data, "a=T,U=1,f=100") {
		t.Fatal("missing native video frame")
	}
	m.videoMessage(done)
	if !m.video.active || api.calls != 1 || m.videoImageID() == m.graphics.id || m.videoImageID() == m.graphics.probeID() {
		t.Fatal("video shares turntable/query ownership or repeated a request")
	}
	m.State.Playback.Playing = false
	m.video.ticking = false
	m.State.Playback.Position = time.Second
	m.videoMessage(videoTick{m.video.generation})
	if m.syncVideo() != nil {
		t.Fatal("in-flight animation tick kept requesting paused pictures")
	}
	generation := m.video.generation
	m.normal("h")
	cleanup := m.syncVideo()
	if m.showingVideo() || cleanup == nil {
		t.Fatal("back did not return to browser and clean video")
	}
	old := videoWrite{generation: generation, data: "old frame"}
	if _, _, ok := m.TerminalOutput(old); ok {
		t.Fatal("late hidden frame still has terminal access")
	}
	m.videoMessage(videoLoaded{generation: generation, frame: api.frame})
	if m.video.frame.Valid() || !m.graphics.ready() {
		t.Fatal("late frame restored hidden video or revoked terminal support")
	}
	final := m.GraphicsCleanup()
	if !strings.Contains(final, termimage.Delete(m.videoImageID())) {
		t.Fatal("video image leaked on final detach")
	}
}
func TestVideoHelpAndControlsTargetVisibleVideoRatherThanOldBrowserCursor(t *testing.T) {
	m, api := videoModel(t)
	help := ansi.Strip(strings.Join(m.helpLines(), "\n"))
	if !strings.Contains(help, "Video") || !strings.Contains(help, "Global") || strings.Contains(help, "Filter loaded") {
		t.Fatal("video inherited browser actions")
	}
	subject, _, ok := m.workSubject()
	if !ok || subject.ID != "movie" {
		t.Fatal("work actions target stale content selection")
	}
	cmd := m.normal("space")
	cmd()
	if api.lastCommand.Op != "toggle" {
		t.Fatal("global pause does not work on video")
	}
	m.normal("v")
	if m.showingVideo() {
		t.Fatal("v did not restore browser")
	}
	m.normal("v")
	if !m.showingVideo() {
		t.Fatal("v did not restore video")
	}
	for _, size := range [][2]int{{60, 16}, {120, 40}, {200, 60}} {
		m.SetSize(size[0], size[1])
		for _, line := range strings.Split(m.View().Content, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("video overflowed content bounds")
			}
		}
	}
}
func TestUnsupportedTerminalDoesNotRequestOrUploadVideoFrames(t *testing.T) {
	m, api := videoModel(t)
	m.graphics.disabled = true
	m.syncVideo()
	if api.calls != 0 || m.video.loading || m.video.active {
		t.Fatal("decoded video without image support")
	}
	rows := ansi.Strip(strings.Join(m.videoRows(12), "\n"))
	if !strings.Contains(rows, "native image support") {
		t.Fatal("missing unsupported terminal explanation")
	}
}

func TestVideoCatalogCategoryDoesNotSurviveSwitchToLibrarySearch(t *testing.T) {
	m, _ := videoModel(t)
	m.UI.Video = false
	m.UI.Location.Page = "search"
	m.UI.Location.Section = "music-videos"
	m.UI.SearchSection = "music-videos"
	m.UI.SearchScope = "apple"
	m.State.Source.Capabilities.CatalogSearch = true
	m.State.Source.SearchSections = []music.SourceSection{{ID: "songs", Title: "Songs"}, {ID: "music-videos", Title: "Videos"}}
	m.normal("s")
	if m.UI.SearchScope != "library" || m.UI.Location.Section != "songs" || m.UI.SearchSection != "songs" {
		t.Fatal("video category leaked into library search")
	}
}
