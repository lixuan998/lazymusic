package music

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// QueueKey guards index-based edits against a changed queue. Playback position
// and resolved playback IDs/display metadata do not change its identity or order.
func QueueKey(tracks []Track) string {
	h := sha256.New()
	encoder := json.NewEncoder(h)
	for _, t := range tracks {
		_ = encoder.Encode([2]string{t.Source, t.ID})
	}
	return hex.EncodeToString(h.Sum(nil))
}
