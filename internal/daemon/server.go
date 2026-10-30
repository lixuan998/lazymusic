package daemon

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"lazymusic/internal/backend"
	"lazymusic/internal/catalog"
	"lazymusic/internal/engine"
	"lazymusic/internal/eventlog"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/volume"
)

type Options struct {
	Mixer   *volume.Controller
	Backend string
	Dir     string
	Browser catalog.Browser
	Factory engine.Factory
}
type lease struct {
	token    string
	pid      int
	seen     time.Time
	revision uint64
}
type cachedPage struct {
	page catalog.Page
	at   time.Time
}

type Service struct {
	logs              *eventlog.Store
	mixer             *volume.Controller
	lastOp            string
	lastPlaybackError string
	source            music.Source
	opts              Options
	mu                sync.Mutex
	commandMu         sync.Mutex
	catalogMu         sync.Mutex
	cacheVersion      uint64
	saved             session.Snapshot
	engine            engine.Engine
	browser           catalog.Browser
	factory           engine.Factory
	lease             lease
	cache             map[string]cachedPage
	started           time.Time
	lastError         string
	secrets           []string
	stopping          bool
	quit              chan struct{}
}

func New(opts Options) (*Service, error) {
	saved, err := session.Load(opts.Dir)
	warning := ""
	if err != nil {
		// Preserve a damaged/unknown snapshot for recovery instead of overwriting it.
		backup := filepath.Join(opts.Dir, fmt.Sprintf("session.corrupt-%d.json", time.Now().UnixNano()))
		if renameErr := os.Rename(filepath.Join(opts.Dir, "session.json"), backup); renameErr != nil {
			return nil, fmt.Errorf("%v; preserving snapshot: %w", err, renameErr)
		}
		warning = "Previous session could not be read; preserved as " + filepath.Base(backup)
		saved, err = session.Load(opts.Dir)
		if err != nil {
			return nil, err
		}
	}
	backendID := opts.Backend
	if backendID == "" {
		backendID = music.Apple
	}
	if saved.Backend != "" && saved.Backend != backendID {
		return nil, fmt.Errorf("saved profile uses %s backend; choose another --state-dir", saved.Backend)
	}
	for i := range saved.Playback.Queue {
		t := &saved.Playback.Queue[i]
		if t.Source == "" {
			t.Source = backendID
		}
		if t.Source != backendID {
			return nil, fmt.Errorf("saved track belongs to %s, profile uses %s", t.Source, backendID)
		}
	}
	saved.Backend = backendID
	saved.Playback.WasPlaying = false
	s := &Service{opts: opts, saved: saved, browser: opts.Browser, factory: opts.Factory, cache: map[string]cachedPage{}, started: time.Now().UTC(), lastError: warning, quit: make(chan struct{})}
	entry, err := backend.Lookup(backendID)
	if err != nil {
		return nil, err
	}
	s.source = entry.Source
	if s.browser == nil {
		b, secrets, err := entry.Open(opts.Dir)
		if err != nil {
			return nil, err
		}
		s.browser = b
		s.secrets = secrets
	}
	if s.factory == nil {
		s.factory = func(ctx context.Context, p session.Playback) (engine.Engine, error) {
			return entry.Engine(ctx, opts.Dir, p)
		}
	}

	s.mixer = opts.Mixer
	if s.mixer == nil {
		s.mixer = volume.New()
	}
	s.logs = eventlog.New(opts.Dir, s.redact)
	s.logs.Add("info", "service", "Background service started")
	if warning != "" {
		s.logs.Add("warn", "session", warning)
	}
	return s, nil
}

func (s *Service) redact(msg string) string {
	s.mu.Lock()
	secrets := append([]string(nil), s.secrets...)
	s.mu.Unlock()
	for _, v := range secrets {
		if v != "" {
			msg = strings.ReplaceAll(msg, v, "[REDACTED]")
		}
	}
	return eventlog.Redact(msg)
}

func (s *Service) State() State {
	s.mu.Lock()
	e := s.engine
	p := s.saved.Playback
	u := s.saved.UI.Clone()
	placement := s.saved.Placement
	err := s.lastError
	s.mu.Unlock()
	state := music.State{Continuous: p.Continuous, Position: p.Position, Volume: p.Volume, RepeatMode: p.Repeat, ShuffleMode: p.Shuffle}
	q := session.CleanTracks(p.Queue)
	index := p.Index
	if len(q) > 0 {
		t := q[max(0, min(index, len(q)-1))]
		state.Track = &t
	}
	if e != nil {
		state, q, index = e.State()
	}
	state.Logs = nil
	state.Error = s.redact(state.Error)
	if state.Track != nil {
		t := session.CleanTrack(*state.Track)
		state.Track = &t
	}
	systemVolume := s.mixer.Read(context.Background())
	state.Volume = systemVolume.Level
	s.mu.Lock()
	lastOp := s.lastOp
	previousError := s.lastPlaybackError
	s.lastPlaybackError = state.Error
	s.mu.Unlock()
	state.Status = "paused"
	if state.Track == nil || lastOp == "stop" || lastOp == "clear" {
		state.Status = "stopped"
	}
	if state.Playing {
		state.Status = "playing"
	}
	if state.Loading {
		state.Status = "buffering"
	}
	if state.Error != "" {
		state.Status = "error"
		if state.Error != previousError {
			s.logs.Add("error", "player", state.Error)
		}
	}
	preferred := p.Quality
	if preferred == "" {
		preferred = s.source.DefaultQuality
	}
	return State{Placement: placement.Status(q), Equalizer: p.Equalizer, PreferredQuality: preferred, SystemVolume: systemVolume, Logs: s.logs.Entries(), Source: s.source, ObservedAt: time.Now().UTC(), Protocol: ProtocolVersion, PID: os.Getpid(), Backend: s.saved.Backend, StartedAt: s.started, Playback: state, Queue: q, Index: index, UI: u, EngineReady: e != nil, Error: s.redact(err)}
}

// Call only while commandMu is held. Native playback continues during disk IO.
func (s *Service) saveLocked() error {
	s.mu.Lock()
	e := s.engine
	copy := s.saved
	copy.UI = s.saved.UI.Clone()
	s.mu.Unlock()
	if e != nil {
		quality := copy.Playback.Quality
		eq := copy.Playback.Equalizer
		copy.Playback = engine.Capture(e)
		copy.Playback.Quality = quality
		copy.Playback.Equalizer = eq
		copy.Playback.Volume = s.mixer.Read(context.Background()).Level
	}
	if err := session.Save(s.opts.Dir, copy); err != nil {
		return err
	}
	s.mu.Lock()
	s.saved.Playback = copy.Playback
	s.saved.UpdatedAt = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

func (s *Service) ensureEngine(ctx context.Context) (engine.Engine, error) {
	s.mu.Lock()
	e := s.engine
	saved := s.saved.Playback
	s.mu.Unlock()
	if e != nil {
		return e, nil
	}
	e, err := s.factory(ctx, saved)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.engine = e
	s.lastError = ""
	s.mu.Unlock()
	if stream, ok := e.(interface{ Events() <-chan eventlog.Entry }); ok {
		go func() {
			for message := range stream.Events() {
				s.logs.Add(message.Level, message.Component, message.Message)
			}
		}()
	}
	return e, nil
}

func (s *Service) execute(ctx context.Context, c Command) error {
	s.commandMu.Lock()
	defer s.commandMu.Unlock()
	s.mu.Lock()
	stopping := s.stopping
	s.mu.Unlock()
	if stopping {
		return fmt.Errorf("background service is shutting down")
	}
	if c.Op == "reload-auth" {
		if err := s.saveLocked(); err != nil {
			return err
		}
		s.mu.Lock()
		old := s.engine
		s.engine = nil
		s.mu.Unlock()
		if old != nil {
			_ = old.Close()
		}
		entry, err := backend.Lookup(s.saved.Backend)
		if err != nil {
			return err
		}
		b, secrets, err := entry.Open(s.opts.Dir)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.browser = b
		s.secrets = secrets
		s.cache = map[string]cachedPage{}
		s.cacheVersion++
		s.mu.Unlock()

		return nil
	}
	if c.Op == "volume" {
		if err := s.mixer.Set(ctx, c.Volume); err != nil {
			s.logs.Add("error", "volume", err.Error())
			return err
		}
		s.logs.Add("info", "volume", fmt.Sprintf("System output volume %.0f%%", c.Volume*100))
		return s.saveLocked()
	}
	e, err := s.ensureEngine(ctx)
	if err != nil {
		return err
	}
	if c.Placement {
		return s.place(ctx, e, c)
	}
	if c.Op == "undo-placement" {
		return s.undoPlacement(ctx, e, c)
	}
	switch c.Op {
	case "equalizer":
		controller, ok := e.(interface{ SetEqualizer(music.Equalizer) error })
		if !ok || !s.source.Capabilities.Equalizer {
			return fmt.Errorf("this player does not support an equalizer")
		}
		s.mu.Lock()
		eq := s.saved.Playback.Equalizer
		s.mu.Unlock()
		switch {
		case c.EQPreset != "":
			eq, err = music.EQPreset(c.EQPreset)
		case c.EQAction == "toggle":
			eq.Bypassed = !eq.Bypassed
		case c.EQAction == "" && c.EQBand >= 0 && c.EQBand < 10:
			eq.Gains[c.EQBand] = max(-12, min(12, eq.Gains[c.EQBand]+c.EQDelta))
		default:
			return fmt.Errorf("invalid equalizer band")
		}
		if err != nil {
			return err
		}
		if err = eq.Validate(); err != nil {
			return err
		}
		if err = controller.SetEqualizer(eq); err == nil {
			s.mu.Lock()
			s.saved.Playback.Equalizer = eq
			s.mu.Unlock()
		}
	case "quality":
		valid := false
		for _, q := range s.source.Qualities {
			if q.ID == c.Quality {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("this quality is not supported by %s", s.source.Name)
		}
		controller, ok := e.(interface{ SetQuality(string) error })
		if !ok {
			return fmt.Errorf("this player does not offer selectable quality")
		}
		if err = controller.SetQuality(c.Quality); err == nil {
			s.mu.Lock()
			s.saved.Playback.Quality = c.Quality
			s.mu.Unlock()
			s.logs.Add("info", "quality", "Preferred quality: "+c.Quality)
		}
	case "play":
		err = e.Play()
	case "pause":
		err = e.Pause()
	case "toggle":
		state, _, _ := e.State()
		if state.Playing {
			err = e.Pause()
		} else {
			err = e.Play()
		}
	case "stop":
		err = e.Stop()
	case "next":
		err = e.Next()
	case "previous":
		err = e.Previous()
	case "seek":
		state, _, _ := e.State()
		pos := c.Position
		if c.Delta != 0 {
			pos = state.Position + c.Delta
		}
		err = e.Seek(max(0, pos))
	case "playback-mode":
		state, _, _ := e.State()
		mode := state.RepeatMode
		if state.ShuffleMode {
			mode = 3
		}
		step := c.ModeStep
		if step == 0 {
			step = 1
		}
		mode = (mode + step%4 + 4) % 4
		err = e.SetShuffle(mode == 3)
		if err == nil {
			repeat := mode
			if repeat == 3 {
				repeat = 0
			}
			err = e.SetRepeat(repeat)
		}
	case "repeat":
		state, _, _ := e.State()
		err = e.SetRepeat((state.RepeatMode + 1) % 3)
	case "shuffle":
		state, _, _ := e.State()
		err = e.SetShuffle(!state.ShuffleMode)
	case "select":
		_, q, _ := e.State()
		if c.ExpectedQueue != "" && c.ExpectedQueue != music.QueueKey(q) {
			return fmt.Errorf("queue changed; select the song again")
		}
		if c.Index < 0 || c.Index >= len(q) {
			return fmt.Errorf("queue index out of range")
		}
		if selector, ok := e.(interface{ Select(int) error }); ok {
			err = selector.Select(c.Index)
		} else {
			err = e.Replace(q, c.Index, true)
		}
	case "remove":
		_, q, _ := e.State()
		if c.ExpectedQueue != "" && c.ExpectedQueue != music.QueueKey(q) {
			return fmt.Errorf("queue changed; select the song again")
		}
		err = e.Remove(c.Index)
	case "move":
		_, q, _ := e.State()
		if c.ExpectedQueue != "" && c.ExpectedQueue != music.QueueKey(q) {
			return fmt.Errorf("queue changed; pick the song again")
		}
		if c.Index < 0 || c.Index >= len(q) || c.To < 0 || c.To >= len(q) {
			return fmt.Errorf("queue index out of range")
		}
		err = e.Move(c.Index, c.To)
	case "clear":
		err = e.Clear()
	case "replace", "append", "play-next", "insert":
		tracks := session.CleanTracks(c.Tracks)
		if c.Query != nil {
			s.mu.Lock()
			b := s.browser
			s.mu.Unlock()
			q := *c.Query
			if q.Source != "" && q.Source != s.source.ID {
				return fmt.Errorf("selection belongs to another music source")
			}
			q.Source = s.source.ID
			if q.Continuous && c.Op == "replace" {
				tuner, ok := e.(interface {
					StartContinuous(context.Context, music.Query) error
				})
				if !ok {
					return fmt.Errorf("this source does not support continuous stations")
				}
				if err = tuner.StartContinuous(ctx, q); err != nil {
					return err
				}
				s.mu.Lock()
				s.lastOp = "replace"
				s.mu.Unlock()
				s.invalidatePlacement()
				s.logs.Add("info", "command", "Start station")
				return s.saveLocked()
			}
			tracks, err = catalog.All(ctx, b, q)
			if err != nil {
				return err
			}
		}
		if len(c.IDs) > 0 {
			tracks = nil
			for _, id := range c.IDs {
				if id != "" {
					tracks = append(tracks, music.Track{Source: s.source.ID, ID: id, PlaybackID: id})
				}
			}
		}
		for i := range tracks {
			if tracks[i].Source == "" {
				tracks[i].Source = s.source.ID
			}
			if tracks[i].Source != s.source.ID {
				return fmt.Errorf("%s cannot play a track from %s", s.source.Name, tracks[i].Source)
			}
		}
		if len(tracks) == 0 {
			return fmt.Errorf("no playable songs in this selection")
		}
		if len(tracks) > 10000 {
			return fmt.Errorf("queue exceeds 10000 tracks")
		}
		if c.Op == "replace" {
			i := c.Index
			if c.StartID != "" {
				for n, t := range tracks {
					if t.ID == c.StartID {
						i = n
						break
					}
				}
			}
			err = e.Replace(tracks, i, true)
		} else {
			_, q, _ := e.State()
			if len(q)+len(tracks) > 10000 {
				return fmt.Errorf("queue exceeds 10000 tracks")
			}
			if c.Op == "insert" {
				if c.ExpectedQueue != music.QueueKey(q) {
					return fmt.Errorf("queue changed; choose the insertion position again")
				}
				err = e.Insert(tracks, c.To)
			} else {
				err = e.Append(tracks, c.Op == "play-next")
			}
		}
	default:
		return fmt.Errorf("unknown command %q", c.Op)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	switch c.Op {
	case "play", "replace", "select", "next", "previous", "toggle", "pause", "stop", "clear":
		s.lastOp = c.Op
	}
	s.mu.Unlock()
	switch c.Op {
	case "replace", "clear", "remove", "move", "insert", "play-next":
		s.invalidatePlacement()
	}
	s.logs.Add("info", "command", c.Op)
	return s.saveLocked()
}

func (s *Service) browse(ctx context.Context, q catalog.Query, refresh bool) (catalog.Page, error) {
	if q.Source != "" && q.Source != s.source.ID {
		return catalog.Page{}, fmt.Errorf("query belongs to another music source")
	}
	q.Source = s.source.ID
	refresh = refresh || q.Refresh
	q.Refresh = false
	b, _ := json.Marshal(q)
	key := string(b)
	s.mu.Lock()
	entry, ok := s.cache[key]
	browser := s.browser
	version := s.cacheVersion
	s.mu.Unlock()
	if ok && !refresh && time.Since(entry.at) < 5*time.Minute {
		return entry.page, nil
	}
	browseQuery := q
	browseQuery.Refresh = refresh
	page, err := browser.Browse(ctx, browseQuery)
	if err != nil {
		return page, err
	}
	s.mu.Lock()
	if len(s.cache) >= 64 {
		s.cache = map[string]cachedPage{}
	}
	if version == s.cacheVersion {
		s.cache[key] = cachedPage{page, time.Now()}
	}
	s.mu.Unlock()
	return page, nil
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(value)
	}
	fail := func(w http.ResponseWriter, status int, err error) {
		if status != 409 && !errors.Is(err, context.Canceled) {
			s.logs.Add("error", "request", err.Error())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(APIError{s.redact(err.Error())})
	}
	decode := func(w http.ResponseWriter, r *http.Request, out any) bool {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(out); err != nil {
			fail(w, 400, fmt.Errorf("invalid request"))
			return false
		}
		return true
	}
	authorized := func(r *http.Request) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lease.token != "" && r.Header.Get("X-Lazymusic-Client") == s.lease.token
	}
	mux.HandleFunc("GET /v1/state", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		if r.Header.Get("X-Lazymusic-Client") == s.lease.token && s.lease.token != "" {
			s.lease.seen = time.Now()
		}
		s.mu.Unlock()
		write(w, s.State())
	})
	mux.HandleFunc("POST /v1/attach", func(w http.ResponseWriter, r *http.Request) {
		var a AttachRequest
		if !decode(w, r, &a) {
			return
		}
		if a.PID <= 0 {
			fail(w, 400, fmt.Errorf("invalid frontend PID"))
			return
		}
		s.mu.Lock()
		if s.stopping {
			s.mu.Unlock()
			fail(w, 409, fmt.Errorf("background service is shutting down"))
			return
		}
		alive := s.lease.token != "" && time.Since(s.lease.seen) < 10*time.Second && syscall.Kill(s.lease.pid, 0) == nil
		if alive {
			s.mu.Unlock()
			fail(w, 409, fmt.Errorf("a TUI is already attached; detach it with q q first"))
			return
		}
		token := rand.Text()
		s.lease = lease{token: token, pid: a.PID, seen: time.Now()}
		s.mu.Unlock()
		write(w, Attachment{Token: token, State: s.State()})
	})
	mux.HandleFunc("POST /v1/ui", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			fail(w, 409, fmt.Errorf("frontend session was replaced or expired"))
			return
		}
		var u UIUpdate
		if !decode(w, r, &u) {
			return
		}
		s.commandMu.Lock()
		defer s.commandMu.Unlock()
		s.mu.Lock()
		if s.stopping || s.lease.token == "" || r.Header.Get("X-Lazymusic-Client") != s.lease.token {
			s.mu.Unlock()
			fail(w, 409, fmt.Errorf("background service is shutting down or frontend session has changed"))
			return
		}
		if u.Revision >= s.lease.revision {
			s.saved.UI = u.UI.Clone()
			s.lease.revision = u.Revision
		}
		s.lease.seen = time.Now()
		s.mu.Unlock()
		if err := s.saveLocked(); err != nil {
			fail(w, 500, err)
			return
		}
		write(w, struct{}{})
	})
	mux.HandleFunc("POST /v1/detach", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			fail(w, 409, fmt.Errorf("frontend session no longer active"))
			return
		}
		s.mu.Lock()
		s.lease = lease{}
		s.mu.Unlock()
		write(w, struct{}{})
	})
	mux.HandleFunc("POST /v1/command", func(w http.ResponseWriter, r *http.Request) {
		var c Command
		if !decode(w, r, &c) {
			return
		}
		if err := s.execute(r.Context(), c); err != nil {
			fail(w, 400, err)
			return
		}
		write(w, s.State())
	})
	mux.HandleFunc("GET /v1/browse", func(w http.ResponseWriter, r *http.Request) {
		var q catalog.Query
		if err := json.Unmarshal([]byte(r.URL.Query().Get("q")), &q); err != nil {
			fail(w, 400, fmt.Errorf("invalid browse request"))
			return
		}
		page, err := s.browse(r.Context(), q, r.URL.Query().Get("refresh") == "1")
		if err != nil {
			fail(w, 400, err)
			return
		}
		write(w, page)
	})
	mux.HandleFunc("POST /v1/inspect", func(w http.ResponseWriter, r *http.Request) {
		var subject catalog.Subject
		if !decode(w, r, &subject) {
			return
		}
		if subject.ID == "" || (subject.Source != "" && subject.Source != s.source.ID) {
			fail(w, 400, fmt.Errorf("invalid work or music source"))
			return
		}
		subject.Source = s.source.ID
		s.mu.Lock()
		browser := s.browser
		s.mu.Unlock()
		inspector, ok := browser.(catalog.Inspector)
		if !ok {
			write(w, catalog.Details{CreditsNote: "This source does not provide contributor credits."})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		details, err := inspector.Inspect(ctx, subject)
		if err != nil {
			fail(w, 400, err)
			return
		}
		write(w, details)
	})
	mux.HandleFunc("POST /v1/actions", func(w http.ResponseWriter, r *http.Request) {
		var subject catalog.Subject
		if !decode(w, r, &subject) {
			return
		}
		if subject.ID == "" || (subject.Source != "" && subject.Source != s.source.ID) {
			fail(w, 400, fmt.Errorf("invalid work or music source"))
			return
		}
		subject.Source = s.source.ID
		s.mu.Lock()
		editor, ok := s.browser.(catalog.Editor)
		s.mu.Unlock()
		if !ok {
			write(w, catalog.Actions{})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		actions, err := editor.Actions(ctx, subject)
		if err != nil {
			fail(w, 400, err)
			return
		}
		write(w, actions)
	})
	mux.HandleFunc("POST /v1/mutate", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r) {
			fail(w, 409, fmt.Errorf("frontend session no longer active"))
			return
		}
		var mutation catalog.Mutation
		if !decode(w, r, &mutation) {
			return
		}
		if mutation.Source != s.source.ID || (mutation.Subject.Source != "" && mutation.Subject.Source != s.source.ID) {
			fail(w, 400, fmt.Errorf("mutation belongs to another music source"))
			return
		}
		switch mutation.Op {
		case catalog.AddLibrary, catalog.AddPlaylist, catalog.RemoveLibrary, catalog.DeletePlaylist:
			if mutation.Subject.ID == "" {
				fail(w, 400, fmt.Errorf("missing work identity"))
				return
			}
		case catalog.CreatePlaylist:
		default:
			fail(w, 400, fmt.Errorf("unsupported catalog operation"))
			return
		}
		// Cloud writes are serialized separately so a slow request cannot block
		// playback controls. Never replay a write after a timeout or lost reply.
		s.catalogMu.Lock()
		defer s.catalogMu.Unlock()
		s.mu.Lock()
		active := !s.stopping && s.lease.token != "" && r.Header.Get("X-Lazymusic-Client") == s.lease.token
		editor, ok := s.browser.(catalog.Editor)
		s.mu.Unlock()
		if !active {
			fail(w, 409, fmt.Errorf("background service is stopping or frontend session changed"))
			return
		}
		if !ok {
			fail(w, 400, fmt.Errorf("this source does not support library editing"))
			return
		}
		invalidate := func() {
			s.mu.Lock()
			s.cacheVersion++
			s.cache = map[string]cachedPage{}
			s.mu.Unlock()
		}
		invalidate()
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		result, err := editor.Mutate(ctx, mutation)
		invalidate()
		if err != nil {
			fail(w, 400, err)
			return
		}
		s.logs.Add("info", "catalog", mutation.Op)
		write(w, result)
	})
	mux.HandleFunc("POST /v1/quit", func(w http.ResponseWriter, r *http.Request) {
		s.commandMu.Lock()
		defer s.commandMu.Unlock()
		s.mu.Lock()
		stopping := s.stopping
		s.mu.Unlock()
		if !stopping {
			if err := s.saveLocked(); err != nil {
				fail(w, 500, err)
				return
			}
			s.mu.Lock()
			s.stopping = true
			e := s.engine
			s.mu.Unlock()
			if e != nil {
				_ = e.Pause()
			}
			close(s.quit)
		}
		write(w, struct{}{})
	})
	return mux
}

func Run(ctx context.Context, opts Options) error {
	lock, err := acquireLock(opts.Dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	s, err := New(opts)
	if err != nil {
		return err
	}
	path, err := SocketPath(opts.Dir)
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer ln.Close()
	defer os.Remove(path)
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	server := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ln) }()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	running := true
	for running {
		select {
		case <-s.quit:
			running = false
		case <-ctx.Done():
			running = false
		case err := <-serveErr:
			if err != http.ErrServerClosed {
				return err
			}
			running = false
		case <-tick.C:
			// Observe asynchronous decoder/output failures even while no TUI is attached.
			_ = s.State()
			s.commandMu.Lock()
			s.mu.Lock()
			stop := s.stopping
			s.mu.Unlock()
			if !stop {
				if err := s.saveLocked(); err != nil {
					s.mu.Lock()
					s.lastError = "Saving session: " + err.Error()
					s.mu.Unlock()
					s.logs.Add("error", "session", err.Error())
				}
			}
			s.commandMu.Unlock()
		}
	}
	s.commandMu.Lock()
	s.mu.Lock()
	alreadySaved := s.stopping
	s.stopping = true
	e := s.engine
	s.mu.Unlock()
	if !alreadySaved {
		if err := s.saveLocked(); err != nil {
			fmt.Fprintln(os.Stderr, s.redact(err.Error()))
		}
	}
	if e != nil {
		_ = e.Pause()
	}
	s.commandMu.Unlock()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	if e != nil {
		_ = e.Close()
	}
	return nil
}
