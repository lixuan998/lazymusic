package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"lazymusic/internal/account"
	"lazymusic/internal/backend"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/tui"
	"os"
	"strings"
	"sync"
	"time"
)

type musicMessage struct {
	epoch   uint64
	message tea.Msg
}
type connectionMessage struct {
	client      *daemon.Client
	state       daemon.State
	dir, source string
	err         error
}
type application struct {
	ctx           context.Context
	options       LaunchOptions
	music         *tui.Model
	login         *loginPage
	client        *daemon.Client
	keys          tui.Keys
	epoch         uint64
	connecting    bool
	width, height int
	poolMu        sync.Mutex
	pool          []*daemon.Client
	closed        bool
}

func wrapCommand(cmd tea.Cmd, epoch uint64) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			commands := make([]tea.Cmd, len(batch))
			for i, c := range batch {
				commands[i] = wrapCommand(c, epoch)
			}
			return tea.Batch(commands...)()
		}
		if _, quit := msg.(tea.QuitMsg); quit {
			return msg
		}
		return musicMessage{epoch, msg}
	}
}
func RunApplication(ctx context.Context, opts LaunchOptions) error {
	if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("interactive interface requires a terminal")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	keys, err := tui.LoadKeys(opts.Dir)
	if err != nil {
		return err
	}
	a := &application{ctx: ctx, options: opts, keys: keys, epoch: 1, width: 100, height: 30}
	state := daemon.State{Source: music.Source{Name: "SELECT SOURCE"}, UI: session.DefaultUI()}
	state.UI.Location = session.Location{Page: "login", Title: "Login"}
	state.UI.NavPage = "login"
	var api tui.API = offlineAPI{state: state}
	ready := readyProfile(opts.Dir, opts.Source)
	if opts.Source == music.Apple {
		if _, err := os.Stat(account.CDMPath(opts.Dir)); err != nil {
			ready = false
		}
	}
	if ready {
		c, err := daemon.Ensure(ctx, opts.Dir, opts.Source)
		if err != nil {
			return err
		}
		attachment, err := c.Attach(ctx)
		if err != nil {
			c.Close()
			return err
		}
		a.client = c
		a.pool = append(a.pool, c)
		state = attachment.State
		api = c
	}
	a.music = tui.New(ctx, api, state, keys)
	if opts.OpenLogin {
		a.music.OpenLogin()
	}
	a.resetLogin()
	program := tea.NewProgram(a, tea.WithContext(ctx))
	_, err = program.Run()
	cancel()
	a.poolMu.Lock()
	a.closed = true
	clients := append([]*daemon.Client(nil), a.pool...)
	a.poolMu.Unlock()
	if a.login.cancel != nil {
		a.login.cancel()
	}
	for _, c := range clients {
		if c == a.client && a.music.ExitAction == "" {
			cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
			_ = c.Detach(cleanup, a.music.UI, a.music.RevisionForExit())
			done()
		}
		c.Close()
	}
	if err == tea.ErrProgramKilled && ctx.Err() != nil {
		return nil
	}
	return err
}
func (a *application) resetLogin() {
	a.login = &loginPage{ctx: a.ctx, opts: a.options, entries: backend.Sources(), phase: "sources", allowBack: a.client != nil, width: a.width, height: a.height}
}
func (a *application) Init() tea.Cmd { return wrapCommand(a.music.Init(), a.epoch) }
func (a *application) musicUpdate(msg tea.Msg) tea.Cmd {
	_, cmd := a.music.Update(msg)
	return wrapCommand(cmd, a.epoch)
}
func (a *application) connect() tea.Cmd {
	a.connecting = true
	a.login.busy = true
	targetDir, source := a.login.dir, a.login.source
	old, oldDir := a.client, a.options.Dir
	u, rev := a.music.UI.Clone(), a.music.RevisionForExit()
	opts := a.options
	return func() tea.Msg {
		result := connectionMessage{dir: targetDir, source: source}
		var c *daemon.Client
		if old != nil && oldDir == targetDir {
			c = old
			result.state, result.err = c.State(a.ctx)
		} else {
			c, result.err = daemon.Ensure(a.ctx, targetDir, source)
			if result.err == nil {
				var attachment daemon.Attachment
				attachment, result.err = c.Attach(a.ctx)
				result.state = attachment.State
			}
		}
		if result.err != nil {
			if c != nil && c != old {
				c.Close()
			}
			return result
		}
		a.poolMu.Lock()
		if a.closed {
			a.poolMu.Unlock()
			if c != old {
				c.Close()
			}
			return connectionMessage{err: context.Canceled}
		}
		if c != old {
			a.pool = append(a.pool, c)
		}
		a.poolMu.Unlock()
		if !opts.Explicit {
			if result.err = SaveSource(opts.Base, source); result.err != nil {
				if c != old {
					_ = c.Detach(a.ctx, result.state.UI, 1)
				}
				return result
			}
		}
		if old != nil && old != c {
			// One selected source owns audible playback. Its queue and UI remain resumable.
			if previous, stateErr := old.State(a.ctx); stateErr == nil {
				if previous.Playback.Playing || previous.Playback.Loading {
					if _, result.err = old.Command(a.ctx, daemon.Command{Op: "pause"}); result.err != nil {
						_ = c.Detach(a.ctx, result.state.UI, 1)
						return result
					}
				}
				if result.err = old.Detach(a.ctx, u, rev); result.err != nil {
					_ = c.Detach(a.ctx, result.state.UI, 1)
					return result
				}
			}
		}
		result.client = c
		return result
	}
}
func (a *application) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.login.width, a.login.height = msg.Width, msg.Height
		return a, a.musicUpdate(msg)
	case musicMessage:
		if msg.epoch != a.epoch {
			return a, nil
		}
		return a, a.musicUpdate(msg.message)
	case connectionMessage:
		a.connecting = false
		a.login.busy = false
		if msg.err != nil {
			a.login.problem = msg.err.Error()
			a.music.Report("error", "login", msg.err.Error())
			return a, wrapCommand(a.music.NotificationTimer(), a.epoch)
		}
		a.client = msg.client
		a.options.Dir, a.options.Source = msg.dir, msg.source
		a.epoch++
		// Connecting is an explicit transition back to the selected source's content.
		if msg.state.UI.NavPage == "login" || msg.state.UI.Location.Page == "login" {
			msg.state.UI = session.DefaultUI()
		}
		msg.state.UI.Focus = 1
		msg.state.UI.NavIndex = -1
		a.music = tui.New(a.ctx, a.client, msg.state, a.keys)
		a.music.SetSize(a.width, a.height)
		a.resetLogin()
		return a, wrapCommand(a.music.Init(), a.epoch)
	case loginDone:
		_, cmd := a.login.Update(msg)
		if a.login.complete {
			return a, a.connect()
		}
		if a.login.problem != "" {
			a.music.Report("error", "login", a.login.problem)
			cmd = tea.Batch(cmd, wrapCommand(a.music.NotificationTimer(), a.epoch))
		}
		return a, cmd
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" && a.music.HasToast() {
		return a, a.musicUpdate(msg)
	}
	if a.music.HasModal() {
		return a, a.musicUpdate(msg)
	}
	if paste, ok := msg.(tea.PasteMsg); ok && a.music.LoginSelected() && a.music.UI.Focus == 1 {
		_, cmd := a.login.Update(paste)
		return a, cmd
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && a.music.LoginSelected() && a.music.UI.Focus == 1 {
		k := key.String()
		global := k == "ctrl+h" || k == "ctrl+l" || k == "ctrl+j" || k == "ctrl+k" || k == "tab" || k == "shift+tab" || k == "@" || k == "ctrl+up" || k == "ctrl+down" || k == "ctrl+left" || k == "ctrl+right" || k == "ctrl+=" || k == "ctrl++" || k == "ctrl+-" || k == "ctrl+_"
		action := a.keys.Action(k)
		global = global || strings.HasPrefix(action, "resize_") || action == "zen" || action == "layout_reset"
		if a.login.phase != "folder" {
			global = global || k == "q" || k == "Q" || k == "?" || k == "+" || k == "_" || k == "=" || k == "-"
		}
		if global {
			return a, a.musicUpdate(msg)
		}
		if a.connecting {
			return a, nil
		}
		_, cmd := a.login.Update(msg)
		if a.login.back {
			a.login.back = false
			a.music.UI.NavIndex = a.music.IndexForPage(a.music.UI.NavPage)
			a.music.UI.Focus = 1
			return a, nil
		}
		// Esc on an unconfigured source remains in the frame; q opens its usual prompt.
		if a.login.phase == "sources" && (k == "esc" || k == "ctrl+c") {
			a.music.UI.Focus = 0
			return a, nil
		}
		return a, cmd
	}
	cmd := a.musicUpdate(msg)
	if !a.music.LoginSelected() && a.login.busy && a.login.cancel != nil {
		a.login.cancel()
		a.login.generation++
		a.login.busy = false
	}
	return a, cmd
}
func (a *application) View() tea.View {
	if a.music.LoginSelected() {
		return a.music.ViewWithContent(&tui.ContentPanel{Title: "Login", Rows: a.login.Rows()})
	}
	return a.music.View()
}

type offlineAPI struct{ state daemon.State }

func (o offlineAPI) State(context.Context) (daemon.State, error) {
	s := o.state
	s.ObservedAt = time.Now()
	return s, nil
}
func (offlineAPI) Browse(context.Context, catalog.Query) (catalog.Page, error) {
	return catalog.Page{}, nil
}
func (offlineAPI) Command(context.Context, daemon.Command) (daemon.State, error) {
	return daemon.State{}, fmt.Errorf("choose a music source in Login first")
}
func (offlineAPI) SaveUI(context.Context, session.UI, uint64) error { return nil }
func (offlineAPI) Detach(context.Context, session.UI, uint64) error { return nil }
func (offlineAPI) Quit(context.Context) error                       { return nil }
