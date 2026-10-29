package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"lazymusic/internal/account"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	localfiles "lazymusic/internal/sources/local"
	"lazymusic/internal/vibez/player/browserless"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

type LaunchOptions struct {
	Base, Dir, Source, MusicDir string
	Explicit, OpenLogin         bool
}
type loginDone struct {
	generation  uint64
	dir, source string
	err         error
}
type loginPage struct {
	allowBack, back                   bool
	ctx                               context.Context
	opts                              LaunchOptions
	entries                           []music.Source
	cursor, width, height             int
	phase, source, dir, root, problem string
	ready, busy, complete             bool
	generation                        uint64
	cancel                            context.CancelFunc
}

func readyProfile(dir, source string) bool {
	if source == music.Local {
		if _, err := os.Stat(filepath.Join(dir, "local.json")); err != nil {
			return false
		}
		cfg, err := localfiles.ReadConfig(dir)
		if err != nil {
			return false
		}
		fi, err := os.Stat(cfg.Root)
		return err == nil && fi.IsDir()
	}
	cfg, err := account.ReadConfig(dir)
	if err != nil || cfg.AppleDeveloperToken == "" || cfg.AppleUserToken == "" {
		return false
	}
	parts := strings.Split(cfg.AppleDeveloperToken, ".")
	if len(parts) == 3 {
		if b, e := base64.RawURLEncoding.DecodeString(parts[1]); e == nil {
			var jwt struct {
				Expires int64 `json:"exp"`
			}
			if json.Unmarshal(b, &jwt) == nil && jwt.Expires > 0 && jwt.Expires <= time.Now().Unix() {
				return false
			}
		}
	}
	return true
}
func (m *loginPage) Init() tea.Cmd { return nil }
func (m *loginPage) choose() tea.Cmd {
	source := m.entries[m.cursor]
	dir, _, err := ResolveProfile(m.opts.Base, m.opts.Explicit, source.ID)
	if err != nil {
		m.problem = err.Error()
		return nil
	}
	m.source, m.dir = source.ID, dir
	m.problem = ""
	m.ready = readyProfile(dir, source.ID)
	if source.RequiresLogin {
		m.phase = "account"
	} else {
		m.phase = "folder"
		cfg, _ := localfiles.ReadConfig(dir)
		m.root = cfg.Root
		if m.opts.MusicDir != "" {
			m.root = m.opts.MusicDir
		}
	}
	return nil
}
func (m *loginPage) connect(force bool) tea.Cmd {
	m.busy = true
	m.problem = ""
	m.generation++
	generation := m.generation
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	source, dir, root, wasReady := m.source, m.dir, m.root, m.ready
	return func() tea.Msg {
		defer cancel()
		changed := force || !wasReady
		finish := func(err error) tea.Msg { return loginDone{generation, dir, source, err} }
		if source == music.Local {
			old, _ := localfiles.ReadConfig(dir)
			if err := localfiles.Configure(dir, root); err != nil {
				return finish(err)
			}
			cfg, _ := localfiles.ReadConfig(dir)
			changed = changed || old.Root != cfg.Root
		} else if source == music.Apple {
			if force || !wasReady {
				helper, err := ensureAuthHelper(ctx, dir)
				if err != nil {
					return finish(err)
				}
				if err = account.LoginViaVibezQuiet(ctx, dir, helper, 38747); err != nil {
					return finish(fmt.Errorf("Apple sign-in did not complete; try again"))
				}
			}
			cfg, err := account.ReadConfig(dir)
			if err != nil {
				return finish(err)
			}
			if _, err = account.ValidateAccount(ctx, cfg); err != nil {
				return finish(fmt.Errorf("Apple Music authorization could not be verified; retry or sign in again"))
			}
			if _, err = os.Stat(account.CDMPath(dir)); err != nil {
				if _, err = browserless.DownloadCDM(ctx, account.CDMPath(dir)); err != nil {
					return finish(fmt.Errorf("could not prepare native playback; check the network and retry"))
				}
			}
		} else {
			return finish(fmt.Errorf("source setup is unavailable"))
		}
		if ctx.Err() != nil {
			return finish(ctx.Err())
		}
		if changed {
			if c, e := daemon.NewClient(dir); e == nil {
				if _, e = c.State(ctx); e == nil {
					if source == music.Local {
						_, e = c.Command(ctx, daemon.Command{Op: "clear"})
					}
					if e == nil {
						_, e = c.Command(ctx, daemon.Command{Op: "reload-auth"})
					}
					c.Close()
					if e != nil {
						return finish(e)
					}
				} else {
					c.Close()
				}
			}
		}

		return finish(nil)
	}
}
func (m *loginPage) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loginDone:
		if msg.generation != m.generation {
			return m, nil
		}
		m.busy = false
		if msg.err != nil {
			m.problem = msg.err.Error()
			return m, nil
		}
		m.dir, m.source = msg.dir, msg.source
		m.complete = true
		return m, nil
	case tea.PasteMsg:
		if m.phase == "folder" && !m.busy {
			m.root += strings.ReplaceAll(msg.Content, "\n", "")
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "esc" || key == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
				m.generation++
			}
			m.busy = false
			m.problem = ""
			if m.phase == "sources" {
				m.back = m.allowBack
				return m, nil
			}
			m.phase = "sources"
			return m, nil
		}
		if m.busy {
			return m, nil
		}
		if m.phase == "sources" {
			switch key {
			case "j", "down":
				m.cursor = (m.cursor + 1) % len(m.entries)
			case "k", "up":
				m.cursor = (m.cursor + len(m.entries) - 1) % len(m.entries)
			case "enter", "l":
				return m, m.choose()
			}
			return m, nil
		}
		if m.phase == "account" {
			switch key {
			case "enter":
				return m, m.connect(false)
			case "r":
				return m, m.connect(true)
			case "h":
				m.phase = "sources"
			}
			return m, nil
		}
		switch key {
		case "enter":
			return m, m.connect(false)
		case "ctrl+u":
			m.root = ""
		case "backspace":
			r := []rune(m.root)
			if len(r) > 0 {
				m.root = string(r[:len(r)-1])
			}
		default:
			if msg.Text != "" {
				m.root += msg.Text
			} else if msg.Mod == 0 && unicode.IsPrint(msg.Code) {
				m.root += string(msg.Code)
			}
		}
	}
	return m, nil
}
func (m *loginPage) Rows() []string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5c78"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#9293a4"))
	lines := []string{accent.Bold(true).Render("Login"), ""}
	{
		switch m.phase {
		case "sources":
			lines = append(lines, "Choose a music source", "")
			for i, s := range m.entries {
				prefix := "  "
				if i == m.cursor {
					prefix = "› "
				}
				label := prefix + s.Name
				if i == m.cursor {
					label = accent.Bold(true).Render(label)
				}
				description := "Your music folder · no account required"
				if s.RequiresLogin {
					description = "Your subscription, library and recommendations"
				}
				lines = append(lines, label, muted.Render("  "+description), "")
			}
			lines = append(lines, muted.Render("j/k select · Enter continue · Esc exit"))
		case "folder":
			lines = append(lines, "LOCAL", "Music folder", "", accent.Render(m.root+"▏"), "", muted.Render("Enter open folder · Ctrl-u clear · Esc back"))
		case "account":
			lines = append(lines, "APPLE MUSIC", "")
			if m.ready {
				lines = append(lines, "An Apple Music account is connected.", "", "Enter continue · r sign in again")
			} else {
				lines = append(lines, "Connect your Apple Music account.", "", "Enter sign in")
			}
			lines = append(lines, "", muted.Render("Sign in on the Apple page; close it when complete."), muted.Render("Playback runs natively after authorization."), muted.Render("Current authorization identity: Vibez"), "", muted.Render("Esc back"))
		}
	}
	if m.busy {
		label := "Opening music folder…"
		if m.source == music.Apple {
			label = "Connecting… complete Apple authorization in your browser."
		}
		lines = append(lines, "", accent.Render(label))
	}
	if m.problem != "" {
		lines = append(lines, "", accent.Render(strings.ReplaceAll(m.problem, "\n", " ")))
	}
	return lines
}

// View satisfies the component model contract; the application embeds Rows in
// its content panel and never starts a separate terminal program for Login.
func (m *loginPage) View() tea.View { return tea.NewView(strings.Join(m.Rows(), "\n")) }
