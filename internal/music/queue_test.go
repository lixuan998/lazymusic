package music

import "testing"

func TestQueueKeyTracksOccurrencesAndIgnoresResolvedMetadata(t *testing.T) {
	q := []Track{{Source: Apple, ID: "i.one"}, {Source: Apple, ID: "i.two"}}
	key := QueueKey(q)
	q[0].PlaybackID, q[0].Title = "resolved-catalog-id", "Updated title"
	if QueueKey(q) != key {
		t.Fatal("metadata resolution invalidated a queue selection")
	}
	if QueueKey([]Track{q[1], q[0]}) == key || QueueKey(append(q, q[0])) == key {
		t.Fatal("order or occurrence count did not change the snapshot key")
	}
	q[0].Source = Local
	if QueueKey(q) == key {
		t.Fatal("different sources shared a queue key")
	}
}
