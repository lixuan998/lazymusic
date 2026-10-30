package music

import "encoding/json"

type Query struct {
	Continuous bool   `json:"continuous,omitempty"`
	Source     string `json:"source,omitempty"`
	Refresh    bool   `json:"refresh,omitempty"`
	Page       string `json:"page"`
	Section    string `json:"section"`
	Scope      string `json:"scope,omitempty"`
	Term       string `json:"term,omitempty"`
	Kind       string `json:"kind,omitempty"`
	ID         string `json:"id,omitempty"`
	Next       string `json:"next,omitempty"`
}

func (q Query) Key() string {
	q.Next = ""
	q.Refresh = false
	b, _ := json.Marshal(q)
	return string(b)
}
