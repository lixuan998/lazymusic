package app

import (
	"encoding/json"
	"fmt"
	"lazymusic/internal/backend"
	"lazymusic/internal/music"
	"os"
	"path/filepath"
)

func SelectedSource(base string) (string, error) {
	b, err := os.ReadFile(filepath.Join(base, "source.json"))
	if os.IsNotExist(err) {
		return music.Apple, nil
	}
	if err != nil {
		return "", err
	}
	var s struct {
		Source string `json:"source"`
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return "", err
	}
	if _, err = backend.Lookup(s.Source); err != nil {
		return "", err
	}
	return s.Source, nil
}
func SaveSource(base, source string) error {
	if _, err := backend.Lookup(source); err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	b, _ := json.Marshal(struct {
		Source string `json:"source"`
	}{source})
	f, err := os.CreateTemp(base, ".source-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(b, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(base, "source.json"))
}
func ResolveProfile(base string, explicit bool, requested string) (string, string, error) {
	source := requested
	if source == "" && explicit {
		var s struct {
			Backend string `json:"backend"`
		}
		if b, err := os.ReadFile(filepath.Join(base, "session.json")); err == nil && json.Unmarshal(b, &s) == nil {
			source = s.Backend
		}
		if source == "" {
			if _, err := os.Stat(filepath.Join(base, "local.json")); err == nil {
				source = music.Local
			}
		}
	}
	if source == "" {
		if explicit {
			source = music.Apple
		} else {
			var err error
			source, err = SelectedSource(base)
			if err != nil {
				return "", "", err
			}
		}
	}
	if _, err := backend.Lookup(source); err != nil {
		return "", "", err
	}
	if explicit && requested != "" {
		existing := ""
		var saved struct {
			Backend string `json:"backend"`
		}
		if b, e := os.ReadFile(filepath.Join(base, "session.json")); e == nil && json.Unmarshal(b, &saved) == nil {
			existing = saved.Backend
		}
		if existing == "" {
			if _, e := os.Stat(filepath.Join(base, "local.json")); e == nil {
				existing = music.Local
			} else if _, e := os.Stat(filepath.Join(base, "credentials.json")); e == nil {
				existing = music.Apple
			}
		}
		if existing != "" && existing != source {
			return "", "", fmt.Errorf("this --state-dir belongs to %s; choose a separate profile for %s", existing, source)
		}
	}
	if !explicit && source != music.Apple {
		base = filepath.Join(base, "profiles", source)
	}
	return base, source, nil
}
