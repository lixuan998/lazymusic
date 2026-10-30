package music

import "fmt"

// QueueEdit describes one atomic structural edit. ExpectedQueue guards exact
// occurrences (including duplicate IDs), independently of playback progress.
type QueueEdit struct {
	ExpectedQueue    string
	Op               string
	Index, To, Count int
	Tracks           []Track
}

type QueueEditResult struct {
	QueueKey       string
	QueueLength    int
	RemovedCurrent bool
}

// QueuePlan references old entries by index and inserted entries by -index-1.
// Players can retain their full native metadata without round-tripping it.
type QueuePlan struct {
	Order          []int
	Index          int
	RemovedCurrent bool
}

func PlanQueueEdit(q []Track, current int, e QueueEdit) (QueuePlan, error) {
	var plan QueuePlan
	if e.ExpectedQueue == "" || e.ExpectedQueue != QueueKey(q) {
		return plan, fmt.Errorf("queue changed; the placement can no longer be edited safely")
	}
	order := make([]int, len(q))
	for i := range order {
		order[i] = i
	}
	switch e.Op {
	case "move":
		if e.Index < 0 || e.Index >= len(q) || e.To < 0 || e.To >= len(q) {
			return plan, fmt.Errorf("queue move index out of range")
		}
		entry := order[e.Index]
		order = append(order[:e.Index], order[e.Index+1:]...)
		order = append(order, 0)
		copy(order[e.To+1:], order[e.To:])
		order[e.To] = entry
	case "insert":
		if e.To < 0 || e.To > len(q) || len(e.Tracks) == 0 || len(q)+len(e.Tracks) > 10000 {
			return plan, fmt.Errorf("invalid queue insertion")
		}
		order = make([]int, 0, len(q)+len(e.Tracks))
		for i := 0; i < e.To; i++ {
			order = append(order, i)
		}
		for i := range e.Tracks {
			order = append(order, -i-1)
		}
		for i := e.To; i < len(q); i++ {
			order = append(order, i)
		}
	case "remove-range":
		if e.Index < 0 || e.Count <= 0 || e.Index > len(q) || e.Count > len(q)-e.Index {
			return plan, fmt.Errorf("queue removal range out of bounds")
		}
		order = append(order[:e.Index], order[e.Index+e.Count:]...)
	default:
		return plan, fmt.Errorf("unsupported queue edit")
	}
	plan.Order = order
	if len(q) > 0 {
		plan.RemovedCurrent = true
		for i, old := range order {
			if old == current {
				plan.Index = i
				plan.RemovedCurrent = false
				break
			}
		}
		if plan.RemovedCurrent {
			plan.Index = max(0, min(e.Index, len(order)-1))
		}
	}
	return plan, nil
}

func (p QueuePlan) Tracks(old, inserted []Track) []Track {
	q := make([]Track, len(p.Order))
	for i, ref := range p.Order {
		if ref >= 0 {
			q[i] = old[ref]
		} else {
			q[i] = inserted[-ref-1]
		}
	}
	return q
}

type PlacementStatus struct {
	ID        string `json:"id"`
	Revision  uint64 `json:"revision"`
	Undone    bool   `json:"undone"`
	Available bool   `json:"available"`
	Cursor    int    `json:"cursor"`
}
