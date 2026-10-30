package engine

import (
	"fmt"

	"lazymusic/internal/music"
	"lazymusic/internal/vibez/provider"
)

// QueueEditor applies a guarded edit without reloading surviving audio.
type QueueEditor interface {
	EditQueue(music.QueueEdit) (music.QueueEditResult, error)
}

func (d *Queue) editLocked(e music.QueueEdit) (music.QueueEditResult, error) {
	d.refresh()
	plan, err := music.PlanQueueEdit(d.q, d.idx, e)
	if err != nil {
		return music.QueueEditResult{}, err
	}
	d.q = plan.Tracks(d.q, e.Tracks)
	d.idx = plan.Index
	if plan.RemovedCurrent {
		d.s.Playing = false
		d.s.Position = 0
	}
	d.selectTrack()
	return music.QueueEditResult{QueueKey: music.QueueKey(d.q), QueueLength: len(d.q), RemovedCurrent: plan.RemovedCurrent}, nil
}
func (d *Queue) EditQueue(e music.QueueEdit) (music.QueueEditResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.editLocked(e)
}

func (l *Local) EditQueue(e music.QueueEdit) (music.QueueEditResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.syncLocked()
	result, err := l.d.EditQueue(e)
	if err != nil {
		return result, err
	}
	if result.RemovedCurrent {
		l.out.Stop()
		l.loaded = ""
		l.playing = false
	}
	return result, nil
}

func (n *Native) EditQueue(e music.QueueEdit) (music.QueueEditResult, error) {
	var result music.QueueEditResult
	inserted := toNativeTracks(e.Tracks)
	err := n.p.EditTracks(func(q []provider.Track, index int) ([]provider.Track, int, bool, error) {
		old := fromNativeTracks(q)
		plan, err := music.PlanQueueEdit(old, index, e)
		if err != nil {
			return nil, 0, false, err
		}
		next := make([]provider.Track, len(plan.Order))
		for i, ref := range plan.Order {
			if ref >= 0 {
				next[i] = q[ref]
			} else {
				next[i] = inserted[-ref-1]
			}
		}
		result = music.QueueEditResult{QueueKey: music.QueueKey(fromNativeTracks(next)), QueueLength: len(next), RemovedCurrent: plan.RemovedCurrent}
		return next, plan.Index, plan.RemovedCurrent, nil
	})
	return result, err
}

func (c *Continuous) EditQueue(e music.QueueEdit) (music.QueueEditResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	editor, ok := c.Engine.(QueueEditor)
	if !ok {
		return music.QueueEditResult{}, fmt.Errorf("this player cannot edit placements")
	}
	result, err := editor.EditQueue(e)
	if err == nil && result.RemovedCurrent {
		c.wanted = false
	}
	return result, err
}
