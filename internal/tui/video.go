package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"lazymusic/internal/termimage"
	"lazymusic/internal/video"
)

type videoState struct {
	sixelData, sixelSize                                       string
	sixelPosition                                              time.Duration
	encoding                                                   bool
	observed, key, failed                                      string
	generation                                                 uint64
	loading, pending, active, used, ticking, clearing, waiting bool
	cancel                                                     context.CancelFunc
	frame                                                      video.Frame
	sentPosition                                               time.Duration
	columns, rows                                              int
	nextFrame                                                  time.Time
	frameSupport                                               int
	probing                                                    bool
	placeholders                                               []string
}
type videoLoaded struct {
	generation uint64
	frame      video.Frame
	err        error
}
type videoCleared struct{ generation uint64 }
type videoTick struct{ generation uint64 }
type videoProbeTimeout struct{ generation uint64 }
type videoWrite struct {
	generation uint64
	data       string
	done       tea.Msg
}
type videoUploaded struct {
	generation    uint64
	position      time.Duration
	columns, rows int
}

func (m *Model) videoKey() string {
	t := m.State.Playback.Track
	if t == nil || t.MediaType != "video" {
		return ""
	}
	return m.State.Source.ID + "|" + t.ID
}
func (m *Model) showingVideo() bool { return m.UI.Video && m.videoKey() != "" && !m.LoginSelected() }
func (m *Model) videoVisible() bool {
	return m.showingVideo() && !m.HasModal() && !m.UI.Zen && m.UI.Layout != "queue" && m.width >= 60 && m.height >= 16 && !(m.width < 100 && m.UI.Focus == 2 && m.UI.Layout == "full")
}
func (m *Model) videoImageID() int {
	return termimage.MinID + (m.graphics.id-termimage.MinID+2)%(termimage.MaxID-termimage.MinID+1)
}
func (m *Model) CancelVideo() {
	v := &m.video
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
	v.generation++
	v.loading = false
	v.pending = false
	v.ticking = false
	v.probing = false
	v.encoding = false
}
func (m *Model) clearVideo() tea.Cmd {
	v := &m.video
	m.CancelVideo()
	v.active = false
	v.frame = video.Frame{}
	v.key = ""
	v.failed = ""
	v.waiting = false
	v.placeholders = nil
	v.sixelData, v.sixelSize = "", ""
	if !v.used {
		return nil
	}
	generation := v.generation
	v.clearing = true
	cleanup := m.graphics.wrap(termimage.Delete(m.videoImageID()))
	if m.graphics.protocol == "sixel" {
		cleanup = termimage.SixelRefresh
	}
	out := videoWrite{generation: generation, data: cleanup, done: videoCleared{generation}}
	return func() tea.Msg { return out }
}
func (m *Model) syncVideo() tea.Cmd {
	v := &m.video
	key := m.videoKey()
	var commands []tea.Cmd
	if v.observed != key {
		v.observed = key
		m.UI.Video = key != ""
		if m.UI.Video && !m.LoginSelected() {
			m.UI.Zen = false
			if m.UI.Layout == "queue" {
				m.UI.Layout = "full"
			}
		}
		commands = append(commands, m.dirty())
	}
	if !m.videoVisible() || m.State.Playback.Status == "stopped" {
		if v.active || v.loading || v.pending || v.encoding || v.ticking || v.key != "" {
			commands = append(commands, m.clearVideo())
		}
		return tea.Batch(commands...)
	}
	if key != v.key {
		commands = append(commands, m.clearVideo())
		v.key = key
	}
	if v.clearing {
		return tea.Batch(commands...)
	}
	if probe := m.syncGraphicsProbe(); probe != nil {
		commands = append(commands, probe)
	}
	if !m.State.Playback.Playing && v.waiting {
		return tea.Batch(commands...)
	}
	if !m.graphics.ready() || m.State.Playback.Loading || v.loading || v.pending || v.encoding || v.ticking || v.failed == key {
		return tea.Batch(commands...)
	}
	provider, ok := m.api.(video.Provider)
	if !ok {
		v.failed = key
		m.Report("warn", "video", "This backend does not provide video frames")
		return tea.Batch(commands...)
	}
	if v.frame.Valid() {
		if m.graphics.protocol == "sixel" {
			if command := m.syncSixelVideoFrame(); command != nil {
				return tea.Batch(append(commands, command)...)
			}
			if v.encoding || v.pending || !m.State.Playback.Playing && absDuration(v.frame.Position-m.State.Playback.Position) < time.Second {
				return tea.Batch(commands...)
			}
		} else {
			_, _, columns, rows := m.videoRect(v.frame.Width, v.frame.Height)
			if !v.active || v.columns != columns || v.rows != rows || v.sentPosition != v.frame.Position {
				sequence := termimage.Transmit(v.frame.PNG, m.videoImageID(), columns, rows)
				if v.active && v.sentPosition == v.frame.Position {
					sequence = termimage.Placement(m.videoImageID(), columns, rows)
				} else if v.active && v.columns == columns && v.rows == rows && v.frameSupport == 1 {
					sequence = termimage.UpdateFrame(v.frame.PNG, m.videoImageID(), 0, 0)
				}
				if v.frameSupport == 0 && !v.probing {
					v.probing = true
					sequence += termimage.ProbeFrames(m.videoImageID())
				}
				v.pending, v.used = true, true
				generation := v.generation
				out := videoWrite{generation, m.graphics.wrap(sequence), videoUploaded{generation, v.frame.Position, columns, rows}}
				commands = append(commands, func() tea.Msg { return out })
				return tea.Batch(commands...)
			}
			if !m.State.Playback.Playing && absDuration(v.frame.Position-m.State.Playback.Position) < time.Second {
				return tea.Batch(commands...)
			}
		}
	}
	v.loading = true
	v.nextFrame = time.Now().Add(m.videoInterval())
	ctx, cancel := context.WithTimeout(m.ctx, 14*time.Second)
	v.cancel = cancel
	generation, id := v.generation, m.State.Playback.Track.ID
	commands = append(commands, func() tea.Msg {
		defer cancel()
		frame, err := provider.VideoFrame(ctx, id)
		return videoLoaded{generation, frame, err}
	})
	return tea.Batch(commands...)
}
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
func (m *Model) videoMessage(msg tea.Msg) {
	v := &m.video
	switch msg := msg.(type) {
	case sixelVideoPrepared:
		if msg.generation != v.generation || !v.encoding || m.graphics.protocol != "sixel" {
			return
		}
		v.encoding = false
		if msg.err != nil {
			v.failed = v.key
			m.Report("info", "video", "Sixel video frame unavailable")
			return
		}
		if msg.position != v.frame.Position {
			return
		}
		v.sixelData, v.sixelSize, v.sixelPosition = msg.data, msg.size, msg.position
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == m.videoImageID() && v.used {
			if string(msg.Payload) == "OK" && v.probing {
				v.frameSupport = 1
				v.probing = false
			} else if string(msg.Payload) != "OK" {
				v.frameSupport = -1
				v.probing = false
			}
		}
	case videoProbeTimeout:
		if msg.generation == v.generation && v.probing {
			v.probing = false
			v.frameSupport = -1
		}
	case videoCleared:
		if msg.generation == v.generation {
			v.clearing = false
		}
	case videoLoaded:
		if msg.generation != v.generation || !v.loading {
			return
		}
		v.cancel = nil
		v.loading = false
		if msg.err != nil {
			if m.ctx.Err() == nil && !errors.Is(msg.err, context.Canceled) {
				v.failed = v.key
				m.Report("error", "video", msg.err.Error())
			}
			return
		}
		if m.State.Playback.Track != nil && msg.frame.Valid() && msg.frame.ID == m.State.Playback.Track.ID && msg.frame.Source == m.State.Source.ID {
			v.waiting = false
			v.frame = msg.frame
		}
	case videoUploaded:
		if msg.generation != v.generation || !v.pending {
			return
		}
		v.pending = false
		v.active = true
		v.sentPosition = msg.position
		if m.graphics.protocol != "sixel" && (v.columns != msg.columns || v.rows != msg.rows || v.placeholders == nil) {
			v.placeholders = termimage.PlaceholderRows(m.videoImageID(), msg.columns, msg.rows)
		}
		v.columns, v.rows = msg.columns, msg.rows
		if m.State.Playback.Playing {
			v.ticking = true
		}
	case videoTick:
		if msg.generation == v.generation {
			v.ticking = false
			if m.State.Playback.Playing {
				v.frame.PNG = nil
			}
		}
	}
}

func (m *Model) videoDelay(now time.Time) time.Duration {
	return max(time.Duration(0), m.video.nextFrame.Sub(now))
}
func (m *Model) videoRect(width, height int) (left, top, columns, rows int) {
	w, h := m.ContentWidth(), min(255, max(1, m.bodyHeight()-4))
	if m.graphics.protocol == "sixel" && m.graphics.cellWidth > 0 && m.graphics.cellHeight > 0 {
		g := m.graphics
		columns = min(w, termimage.MaxSixelWidth/g.cellWidth)
		rows = max(1, (columns*g.cellWidth*max(1, height)+g.cellHeight*max(1, width)-1)/(g.cellHeight*max(1, width)))
		limit := min(h, termimage.MaxSixelHeight/g.cellHeight)
		if rows > limit {
			rows = limit
			columns = min(columns, rows*g.cellHeight*max(1, width)/(g.cellWidth*max(1, height)))
		}
		left, top = max(0, (w-columns)/2), max(0, (h-rows)/2)+1
		return
	}
	width, height = max(1, width), max(1, height)
	columns = min(w, 255)
	rows = max(1, columns*height/(2*width))
	if rows > h {
		rows = h
		columns = max(1, min(w, rows*2*width/height))
	}
	left, top = max(0, (w-columns)/2), max(0, (h-rows)/2)+1
	return
}
func (m *Model) videoRows(h int) []string {
	w := m.ContentWidth()
	rows := make([]string, h)
	for i := range rows {
		rows[i] = fit("", w)
	}
	if h == 0 {
		return rows
	}
	header := "VIDEO · h back · v browse / video"
	if m.video.active {
		header = fmt.Sprintf("VIDEO · %d×%d · h back · v browse / video", m.video.frame.Width, m.video.frame.Height)
	}
	rows[0] = muted.Render(centered(header, w))
	label := "Loading video…"
	switch {
	case m.State.Playback.Status == "stopped":
		label = "Video stopped · Space to play"
	case m.graphics.disabled || m.graphics.checked && !m.graphics.ready():
		label = "Video needs a terminal with native image support"
	case !m.graphics.ready():
		label = "Checking terminal image support…"
	case m.video.waiting:
		label = "Paused · Space to load video"
	case m.video.failed != "":
		label = "Video unavailable · v to return / retry · @ logs"
	case m.video.active:
		left, top, columns, n := m.videoRect(m.video.frame.Width, m.video.frame.Height)
		if columns == m.video.columns && n == m.video.rows {
			if m.graphics.protocol == "sixel" {
				return rows
			}
			for y, line := range m.video.placeholders {
				if top+y < len(rows) {
					rows[top+y] = fit(strings.Repeat(" ", left)+line, w)
				}
			}
			return rows
		}
	}
	rows[max(0, h/2)] = muted.Render(centered(label, w))
	return rows
}
func (m *Model) videoTitle() string {
	t := m.State.Playback.Track
	if t == nil {
		return "Video"
	}
	return fmt.Sprintf("Video · %s", plain(t.Title))
}
