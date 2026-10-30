package session

import "lazymusic/internal/music"

// Placement retains only the latest successful pick/place, never playback
// snapshots. Applying its inverse must preserve the live player's state.
type Placement struct {
	ID             string        `json:"id"`
	Revision       uint64        `json:"revision"`
	Op             string        `json:"op"`
	From           int           `json:"from"`
	To             int           `json:"to"`
	Tracks         []music.Track `json:"tracks,omitempty"`
	ExpectedQueue  string        `json:"expected_queue"`
	ExpectedLength int           `json:"expected_length"`
	Undone         bool          `json:"undone"`
	Invalid        bool          `json:"invalid,omitempty"`
}

func (p *Placement) Clone() *Placement {
	if p == nil {
		return nil
	}
	q := *p
	q.Tracks = CleanTracks(p.Tracks)
	return &q
}
func (p *Placement) Status(queue []music.Track) *music.PlacementStatus {
	if p == nil {
		return nil
	}
	cursor := p.To
	if p.Op == "move" && p.Undone {
		cursor = p.From
	}
	return &music.PlacementStatus{ID: p.ID, Revision: p.Revision, Undone: p.Undone, Available: p.Matches(queue), Cursor: cursor}
}

// Tail appends preserve the indexed occurrences. Head edits or reorders do not.
func (p *Placement) Matches(queue []music.Track) bool {
	return p != nil && !p.Invalid && p.ExpectedLength >= 0 && p.ExpectedLength <= len(queue) && p.ExpectedQueue == music.QueueKey(queue[:p.ExpectedLength])
}
func (p *Placement) Edit() music.QueueEdit {
	e := music.QueueEdit{ExpectedQueue: p.ExpectedQueue}
	if p.Op == "move" {
		e.Op = "move"
		e.Index, e.To = p.To, p.From
		if p.Undone {
			e.Index, e.To = p.From, p.To
		}
	} else if p.Undone {
		e.Op = "insert"
		e.To = p.To
		e.Tracks = CleanTracks(p.Tracks)
	} else {
		e.Op = "remove-range"
		e.Index = p.To
		e.Count = len(p.Tracks)
	}
	return e
}
