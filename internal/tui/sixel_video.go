package tui

import (
	"bytes"
	"fmt"
	"image/png"
	"time"

	tea "charm.land/bubbletea/v2"
	"lazymusic/internal/termimage"
	"lazymusic/internal/video"
)

type sixelVideoPrepared struct {
	generation uint64
	position   time.Duration
	size, data string
	err        error
}

func (m *Model) videoInterval() time.Duration {
	// Sixel transmits a complete paletted image each time, rather than a GPU
	// frame patch. Keep decode + encode + output in one bounded 10 FPS pipeline.
	if m.graphics.protocol == "sixel" {
		return time.Second / 10
	}
	return video.Interval
}
func (m *Model) syncSixelVideoFrame() tea.Cmd {
	v, g := &m.video, &m.graphics
	_, _, columns, rows := m.videoRect(v.frame.Width, v.frame.Height)
	if columns < 1 || rows < 1 {
		return nil
	}
	size := m.sixelSize(columns, rows)
	if v.sixelData == "" || v.sixelPosition != v.frame.Position || v.sixelSize != size {
		if v.encoding {
			return nil
		}
		v.encoding = true
		frame, generation := v.frame, v.generation
		width, height := columns*g.cellWidth, rows*g.cellHeight
		return func() tea.Msg {
			msg := sixelVideoPrepared{generation: generation, position: frame.Position, size: size}
			config, err := png.DecodeConfig(bytes.NewReader(frame.PNG))
			if err != nil || config.Width != frame.Width || config.Height != frame.Height || config.Width > 1280 || config.Height > 720 {
				msg.err = fmt.Errorf("invalid video PNG dimensions")
				return msg
			}
			im, err := png.Decode(bytes.NewReader(frame.PNG))
			if err == nil {
				msg.data, err = termimage.EncodeSixel(im, width, height)
			}
			msg.err = err
			return msg
		}
	}
	if !v.active || v.columns != columns || v.rows != rows || v.sentPosition != v.frame.Position {
		v.pending, v.used, v.active = true, true, true
		v.columns, v.rows = columns, rows
		v.sentPosition = v.frame.Position
		out := videoWrite{v.generation, termimage.SixelRefresh, videoUploaded{v.generation, v.frame.Position, columns, rows}}
		return func() tea.Msg { return out }
	}
	return nil
}
