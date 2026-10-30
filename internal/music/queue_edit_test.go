package music

import (
	"reflect"
	"testing"
)

func TestQueueMoveInversePreservesDuplicateOccurrences(t *testing.T) {
	q := []Track{{ID: "same", Title: "first"}, {ID: "same", Title: "second"}, {ID: "third"}, {ID: "fourth"}}
	for from := range q {
		for to := range q {
			for current := range q {
				plan, err := PlanQueueEdit(q, current, QueueEdit{ExpectedQueue: QueueKey(q), Op: "move", Index: from, To: to})
				if err != nil {
					t.Fatal(err)
				}
				next := plan.Tracks(q, nil)
				if next[plan.Index].Title != q[current].Title || next[plan.Index].ID != q[current].ID || plan.RemovedCurrent {
					t.Fatal("move changed the playing occurrence")
				}
				undo, err := PlanQueueEdit(next, plan.Index, QueueEdit{ExpectedQueue: QueueKey(next), Op: "move", Index: to, To: from})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(undo.Tracks(next, nil), q) || undo.Index != current {
					t.Fatalf("move inverse failed from=%d to=%d current=%d", from, to, current)
				}
			}
		}
	}
}

func TestBatchInsertionInversePreservesPlayingOccurrenceAtEveryPosition(t *testing.T) {
	q := []Track{{ID: "same", Title: "first"}, {ID: "same", Title: "second"}, {ID: "last"}}
	batch := []Track{{ID: "same", Title: "inserted"}, {ID: "other"}}
	for at := 0; at <= len(q); at++ {
		for current := range q {
			insert, err := PlanQueueEdit(q, current, QueueEdit{Op: "insert", To: at, Tracks: batch, ExpectedQueue: QueueKey(q)})
			if err != nil {
				t.Fatal(err)
			}
			next := insert.Tracks(q, batch)
			undo, err := PlanQueueEdit(next, insert.Index, QueueEdit{Op: "remove-range", Index: at, Count: len(batch), ExpectedQueue: QueueKey(next)})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(undo.Tracks(next, nil), q) || undo.Index != current || undo.RemovedCurrent {
				t.Fatal("batch undo removed a different occurrence")
			}
		}
	}
	if _, err := PlanQueueEdit(q, 0, QueueEdit{Op: "move", Index: 0, To: 1, ExpectedQueue: "stale"}); err == nil {
		t.Fatal("accepted a stale queue")
	}
	for _, edit := range []QueueEdit{{Op: "move", Index: -1, To: 1}, {Op: "insert", To: 4, Tracks: batch}, {Op: "remove-range", Index: 2, Count: 2}} {
		edit.ExpectedQueue = QueueKey(q)
		if _, err := PlanQueueEdit(q, 0, edit); err == nil {
			t.Fatal("accepted invalid queue edit")
		}
	}
}
