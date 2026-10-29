// Package eventlog keeps bounded, redacted runtime history in memory and on disk.
package eventlog

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	ID        uint64    `json:"id"`
	Time      time.Time `json:"time"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
}
type Store struct {
	mu      sync.Mutex
	dir     string
	seq     uint64
	entries []Entry
	last    map[string]time.Time
	redact  func(string) string
}

var urls = regexp.MustCompile(`https?://[^\s"<>]+`)
var bearer = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~-]+`)
var jwt = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

func Redact(s string) string {
	s = bearer.ReplaceAllString(s, "Bearer [REDACTED]")
	s = jwt.ReplaceAllString(s, "[REDACTED]")
	return urls.ReplaceAllStringFunc(s, func(u string) string {
		parsed, err := url.Parse(u)
		if err != nil {
			return "[REDACTED URL]"
		}
		// Signed credentials can occur in paths as well as query strings.
		return parsed.Scheme + "://" + parsed.Hostname() + "/[REDACTED]"
	})
}
func New(dir string, redact func(string) string) *Store {
	s := &Store{dir: dir, last: map[string]time.Time{}, redact: redact}
	// Only the latest bounded history is needed after a service restart.
	if dir != "" {
		for _, name := range []string{"runtime.log.1", "runtime.log"} {
			f, err := os.Open(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			scan := bufio.NewScanner(f)
			for scan.Scan() {
				var e Entry
				if json.Unmarshal(scan.Bytes(), &e) != nil {
					continue
				}
				e.Message = Redact(e.Message)
				if redact != nil {
					e.Message = redact(e.Message)
				}
				s.seq++
				e.ID = s.seq
				s.entries = append(s.entries, e)
				if len(s.entries) > 500 {
					s.entries = s.entries[1:]
				}
			}
			_ = f.Close()
		}
	}
	return s
}
func (s *Store) Add(level, component, message string) {
	message = Redact(message)
	if s.redact != nil {
		message = s.redact(message)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := level + component + message
	if time.Since(s.last[key]) < 10*time.Second {
		return
	}
	if len(s.last) > 1024 {
		s.last = map[string]time.Time{}
	}
	s.last[key] = time.Now()
	s.seq++
	e := Entry{s.seq, time.Now().UTC(), level, component, message}
	s.entries = append(s.entries, e)
	if len(s.entries) > 500 {
		s.entries = append([]Entry(nil), s.entries[len(s.entries)-500:]...)
	}
	if s.dir == "" {
		return
	}
	path := filepath.Join(s.dir, "runtime.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		_ = json.NewEncoder(f).Encode(e)
		_ = f.Close()
	}
}
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Entry(nil), s.entries...)
}
