// Package local browses a user-selected directory without accounts or cloud APIs.
package local

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"lazymusic/internal/catalog"
	"lazymusic/internal/music"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	Root string `json:"root"`
}

func DefaultRoot() string { h, _ := os.UserHomeDir(); return filepath.Join(h, "Music") }
func ReadConfig(dir string) (Config, error) {
	cfg := Config{Root: DefaultRoot()}
	b, err := os.ReadFile(filepath.Join(dir, "local.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(b, &cfg)
	if err != nil {
		return cfg, err
	}
	if !filepath.IsAbs(cfg.Root) {
		return cfg, fmt.Errorf("local music root must be absolute")
	}
	return cfg, nil
}
func Configure(dir, root string) error {
	if root == "" {
		root = DefaultRoot()
	}
	if root == "~" {
		h, _ := os.UserHomeDir()
		root = h
	} else if strings.HasPrefix(root, "~/") {
		h, _ := os.UserHomeDir()
		root = filepath.Join(h, root[2:])
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	fi, err := os.Stat(abs)
	if os.IsNotExist(err) && abs == DefaultRoot() {
		if e := os.MkdirAll(abs, 0700); e != nil {
			return e
		}
		fi, err = os.Stat(abs)
	}
	if err != nil {
		return fmt.Errorf("music directory: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("music root is not a directory")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		abs = resolved
	}
	b, _ := json.MarshalIndent(Config{Root: abs}, "", "  ")
	f, err := os.CreateTemp(dir, ".local-*")
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
	return os.Rename(f.Name(), filepath.Join(dir, "local.json"))
}
func Resolve(root, id string) (string, error) {
	if filepath.IsAbs(id) || id == "" {
		return "", fmt.Errorf("invalid local track identity")
	}
	path := filepath.Join(root, filepath.FromSlash(id))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local path escapes music directory")
	}
	// Existing symlinks are also checked before any file is opened.
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	actualRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(actualRoot, actual)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local symlink escapes music directory")
	}
	return actual, nil
}
func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".m4a", ".aac", ".wav", ".aiff", ".aif", ".flac", ".ogg", ".opus":
		return true
	}
	return false
}

type Browser struct{ Root string }

func New(dir string) (*Browser, error) {
	cfg, err := ReadConfig(dir)
	if err != nil {
		return nil, err
	}
	if resolved, e := filepath.EvalSymlinks(cfg.Root); e == nil {
		cfg.Root = resolved
	}
	return &Browser{Root: cfg.Root}, nil
}
func (b *Browser) scan(ctx context.Context) ([]music.Track, error) {
	var tracks []music.Track
	err := filepath.WalkDir(b.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if path != b.Root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !Supported(path) {
			return nil
		}
		rel, err := filepath.Rel(b.Root, path)
		if err != nil {
			return err
		}
		t := music.Track{Source: music.Local, ID: filepath.ToSlash(rel), Title: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))}
		folder := filepath.Dir(rel)
		if folder != "." {
			t.Album = filepath.Base(folder)
			artist := filepath.Dir(folder)
			if artist != "." {
				t.Artist = filepath.Base(artist)
			}
		}
		tracks = append(tracks, t)
		if len(tracks) > 100000 {
			return fmt.Errorf("local library exceeds 100000 audio files")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read local music directory (choose a folder in Login): %w", err)
	}
	sort.SliceStable(tracks, func(i, j int) bool { return strings.ToLower(tracks[i].ID) < strings.ToLower(tracks[j].ID) })
	return tracks, nil
}
func (b *Browser) Browse(ctx context.Context, q catalog.Query) (catalog.Page, error) {
	if q.Source != "" && q.Source != music.Local {
		return catalog.Page{}, fmt.Errorf("wrong source for LOCAL")
	}
	if q.Page == "home" {
		return catalog.Page{}, fmt.Errorf("LOCAL has no personalized recommendations")
	}
	tracks, err := b.scan(ctx)
	if err != nil {
		return catalog.Page{}, err
	}
	section := q.Section
	if section == "" {
		section = "songs"
	}
	var items []catalog.Item
	if q.Page == "folders" || q.Kind == "folders" {
		prefix := strings.Trim(q.ID, "/")
		if prefix != "" {
			prefix += "/"
		}
		seen := map[string]bool{}
		for _, t := range tracks {
			if !strings.HasPrefix(t.ID, prefix) {
				continue
			}
			tail := strings.TrimPrefix(t.ID, prefix)
			if cut := strings.Index(tail, "/"); cut >= 0 {
				id := prefix + tail[:cut]
				if !seen[id] {
					seen[id] = true
					open := catalog.Query{Source: music.Local, Page: "folders", Section: "songs", Kind: "folders", ID: id}
					play := catalog.Query{Source: music.Local, Page: "detail", Section: "songs", Kind: "folder-tracks", ID: id}
					items = append(items, catalog.Item{Source: music.Local, ID: id, Kind: "folders", Title: tail[:cut], Subtitle: "Folder", Open: &open, Play: &play})
				}
			} else {
				items = append(items, trackItem(t))
			}
		}
	} else {
		groups := map[string]bool{}
		for _, t := range tracks {
			if q.Page == "detail" {
				switch q.Kind {
				case "albums":
					if filepath.ToSlash(filepath.Dir(t.ID)) != q.ID {
						continue
					}
				case "artists":
					if t.Artist != q.ID {
						continue
					}
				case "folder-tracks":
					if !strings.HasPrefix(t.ID, strings.TrimSuffix(q.ID, "/")+"/") {
						continue
					}
				default:
					return catalog.Page{}, fmt.Errorf("unsupported local collection")
				}
			}
			if q.Page == "search" && !strings.Contains(strings.ToLower(t.Title+" "+t.Artist+" "+t.Album), strings.ToLower(q.Term)) {
				continue
			}
			if section == "songs" || q.Page == "detail" && q.Kind != "artists" {
				items = append(items, trackItem(t))
				continue
			}
			id, title, subtitle := filepath.ToSlash(filepath.Dir(t.ID)), t.Album, t.Artist
			if section == "artists" {
				id, title, subtitle = t.Artist, t.Artist, "Artist"
			}
			if title == "" || groups[id] {
				continue
			}
			groups[id] = true
			open := catalog.Query{Source: music.Local, Page: "detail", Section: "songs", Kind: section, ID: id}
			var play *catalog.Query
			if section == "albums" {
				v := open
				play = &v
			}
			items = append(items, catalog.Item{Source: music.Local, ID: id, Kind: section, Title: title, Subtitle: subtitle, Open: &open, Play: play})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title) })
	offset, err := strconv.Atoi(q.Next)
	if q.Next != "" && (err != nil || offset < 0) {
		return catalog.Page{}, fmt.Errorf("invalid local page")
	}
	offset = min(offset, len(items))
	end := min(offset+50, len(items))
	out := catalog.Page{Items: items[offset:end]}
	if end < len(items) {
		out.Next = strconv.Itoa(end)
	}
	return out, nil
}
func trackItem(t music.Track) catalog.Item {
	return catalog.Item{Source: music.Local, ID: t.ID, Kind: "songs", Title: t.Title, Subtitle: t.Artist, Track: &t}
}
