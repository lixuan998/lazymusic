package daemon

import (
	"context"
	"crypto/rand"
	"fmt"

	"lazymusic/internal/engine"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
)

// All placement mutations and history changes run under commandMu. History
// values are immutable after publication to State or the snapshot writer.
func (s *Service) invalidatePlacement() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.saved.Placement.Clone(); p != nil {
		p.Invalid = true
		s.saved.Placement = p
	}
}

func (s *Service) place(ctx context.Context, e engine.Engine, c Command) error {
	if (c.Op != "move" && c.Op != "insert") || c.Query != nil || len(c.IDs) > 0 {
		return fmt.Errorf("invalid pick & place operation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	editor, ok := e.(engine.QueueEditor)
	if !ok {
		return fmt.Errorf("this player cannot edit placements")
	}
	tracks := session.CleanTracks(c.Tracks)
	for i := range tracks {
		if tracks[i].Source == "" {
			tracks[i].Source = s.source.ID
		}
		if tracks[i].Source != s.source.ID {
			return fmt.Errorf("placement belongs to another source")
		}
	}
	edit := music.QueueEdit{Op: c.Op, Index: c.Index, To: c.To, Tracks: tracks, ExpectedQueue: c.ExpectedQueue}
	result, err := editor.EditQueue(edit)
	if err != nil {
		return err
	}
	if c.Op == "move" && c.Index == c.To {
		return nil
	}
	p := &session.Placement{ID: rand.Text(), Op: c.Op, From: c.Index, To: c.To, Tracks: tracks, ExpectedQueue: result.QueueKey, ExpectedLength: result.QueueLength}
	s.mu.Lock()
	s.saved.Placement = p
	s.mu.Unlock()
	s.logs.Add("info", "queue", "Pick & place")
	return s.saveLocked()
}

func (s *Service) undoPlacement(ctx context.Context, e engine.Engine, c Command) error {
	s.mu.Lock()
	p := s.saved.Placement.Clone()
	s.mu.Unlock()
	if p == nil {
		return fmt.Errorf("no pick & place to undo")
	}
	_, queue, _ := e.State()
	if !p.Matches(queue) {
		return fmt.Errorf("queue changed; the last placement can no longer be undone")
	}
	if c.PlacementID != p.ID || c.PlacementRevision != p.Revision {
		return fmt.Errorf("placement changed; wait for the latest queue state")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	editor, ok := e.(engine.QueueEditor)
	if !ok {
		return fmt.Errorf("this player cannot undo placements")
	}
	edit := p.Edit()
	edit.ExpectedQueue = music.QueueKey(queue)
	result, err := editor.EditQueue(edit)
	if err != nil {
		return err
	}
	p.Undone = !p.Undone
	p.Revision++
	p.ExpectedQueue = result.QueueKey
	p.ExpectedLength = result.QueueLength
	s.mu.Lock()
	s.saved.Placement = p
	s.mu.Unlock()
	message := "Undo pick & place"
	if !p.Undone {
		message = "Redo pick & place"
	}
	s.logs.Add("info", "queue", message)
	return s.saveLocked()
}
