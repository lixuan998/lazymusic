package tui

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"lazymusic/internal/catalog"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
)

type editChoice struct{ label, op string }
type editDialog struct {
	operation                 string
	phase, title, draft, note string
	subject                   catalog.Subject
	origin                    string
	cursor                    int
	loading                   bool
	actions                   catalog.Actions
	choices                   []editChoice
	playlists                 []catalog.Item
	queue                     *daemon.Command
}
type editOptionsMsg struct {
	generation uint64
	actions    catalog.Actions
	err        error
}
type playlistPageMsg struct {
	generation uint64
	items      []catalog.Item
	err        error
}
type mutationMsg struct {
	generation uint64
	origin     string
	result     catalog.MutationResult
	err        error
}

func (m *Model) closeEdit() {
	if m.editCancel != nil {
		m.editCancel()
		m.editCancel = nil
	}
	m.editGeneration++
	m.edit = nil
}

func (m *Model) openEdit(mode string) tea.Cmd {
	if m.mutationBusy {
		m.Report("warn", "catalog", "A library change is still in progress; wait for the result before trying again")
		return nil
	}
	subject, title, ok := m.workSubject()
	if !ok {
		return nil
	}
	m.closeEdit()
	d := &editDialog{phase: mode, operation: mode, title: title, subject: subject, origin: m.query().Key(), loading: true}
	if mode == "add" {
		var track *music.Track
		if m.UI.Focus == 1 && m.showingVideo() {
			track = m.State.Playback.Track
		} else if m.UI.Focus == 1 {
			items := m.visible()
			item := items[m.cursor]
			track = item.Track
			if item.Play != nil {
				q := *item.Play
				d.queue = &daemon.Command{Op: "append", Query: &q}
			}
		} else if m.UI.Focus == 3 {
			track = m.State.Playback.Track
		}
		if track != nil {
			copy := *track
			d.queue = &daemon.Command{Op: "append", Tracks: []music.Track{copy}}
		}
	}
	m.edit = d
	generation := m.editGeneration
	editor, ok := m.api.(catalog.Editor)
	if !ok {
		return m.acceptEditOptions(editOptionsMsg{generation: generation})
	}
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	m.editCancel = cancel
	return func() tea.Msg {
		defer cancel()
		actions, err := editor.Actions(ctx, subject)
		return editOptionsMsg{generation, actions, err}
	}
}

func (m *Model) acceptEditOptions(msg editOptionsMsg) tea.Cmd {
	if m.edit == nil || msg.generation != m.editGeneration {
		return nil
	}
	d := m.edit
	d.loading = false
	if msg.err != nil {
		m.closeEdit()
		if !errors.Is(msg.err, context.Canceled) {
			m.Report("error", "catalog", msg.err.Error())
		}
		return nil
	}
	d.actions = msg.actions
	if d.phase == "remove" {
		if d.actions.Remove == catalog.RemoveLibrary || d.actions.Remove == catalog.DeletePlaylist {
			d.phase = "confirm"
			d.choices = []editChoice{{"Cancel", "cancel"}, {"Remove from music library", d.actions.Remove}}
		} else {
			d.phase = "info"
			d.note = d.actions.RemovalNote
			if d.note == "" {
				d.note = "This source does not support removing this item from its library."
			}
		}
		return nil
	}
	if d.queue != nil {
		d.choices = append(d.choices, editChoice{"Playback queue", "queue"})
		d.choices = append(d.choices, editChoice{"Play next", "play-next"})
	}
	if d.actions.AddLibrary {
		d.choices = append(d.choices, editChoice{"Music library", catalog.AddLibrary})
	}
	if d.actions.Playlists != nil {
		d.choices = append(d.choices, editChoice{"Choose playlist…", "playlist"})
	}
	if len(d.choices) == 0 {
		d.phase = "info"
		d.note = "No add actions are available for this item."
	}
	return nil
}

func (m *Model) choosePlaylists() tea.Cmd {
	d := m.edit
	d.phase = "playlists"
	d.loading = true
	d.cursor = 0
	q := *d.actions.Playlists
	q.Refresh = true
	generation := m.editGeneration
	source := m.State.Source.ID
	ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
	m.editCancel = cancel
	return func() tea.Msg {
		defer cancel()
		var items []catalog.Item
		seenPages := map[string]bool{}
		seenItems := map[string]bool{}
		for page := 0; page < 100; page++ {
			p, err := m.api.Browse(ctx, q)
			if err != nil {
				return playlistPageMsg{generation: generation, err: err}
			}
			for _, item := range p.Items {
				if item.Source != "" && item.Source != source {
					continue
				}
				if item.Writable && !seenItems[item.CursorID()] {
					items = append(items, item)
					seenItems[item.CursorID()] = true
				}
			}
			if len(items) > 5000 {
				return playlistPageMsg{generation: generation, err: errors.New("too many destination playlists")}
			}
			if p.Next == "" {
				return playlistPageMsg{generation: generation, items: items}
			}
			if seenPages[p.Next] {
				return playlistPageMsg{generation: generation, err: errors.New("source returned repeated playlist pages")}
			}
			seenPages[p.Next] = true
			q.Next = p.Next
		}
		return playlistPageMsg{generation: generation, err: errors.New("too many destination playlist pages")}
	}
}

func (m *Model) acceptPlaylistPage(msg playlistPageMsg) tea.Cmd {
	if m.edit == nil || msg.generation != m.editGeneration {
		return nil
	}
	if msg.err != nil {
		m.closeEdit()
		if !errors.Is(msg.err, context.Canceled) {
			m.Report("error", "catalog", msg.err.Error())
		}
		return nil
	}
	m.edit.loading = false
	m.edit.playlists = msg.items
	if len(msg.items) == 0 {
		m.edit.phase = "info"
		m.edit.note = "No editable playlists. Create one with a in Playlists, then try again."
	}
	return nil
}

func (m *Model) mutateCatalog(op, playlist string) tea.Cmd {
	if m.mutationBusy {
		return nil
	}
	d := m.edit
	editor, ok := m.api.(catalog.Editor)
	if !ok {
		m.Report("warn", "catalog", "This source does not support library editing")
		return nil
	}
	mutation := catalog.Mutation{Source: m.State.Source.ID, Op: op, Subject: d.subject, Name: strings.TrimSpace(d.draft), PlaylistID: playlist}
	generation, origin := m.editGeneration, d.origin
	m.mutationBusy = true
	d.phase = "writing"
	// Closing the dialog cannot undo a submitted write. Keep awaiting its reply
	// and never submit another write on repeated Enter.
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 55*time.Second)
		defer cancel()
		result, err := editor.Mutate(ctx, mutation)
		return mutationMsg{generation, origin, result, err}
	}
}

func (m *Model) acceptMutation(msg mutationMsg) tea.Cmd {
	m.mutationBusy = false
	if msg.generation == m.editGeneration {
		m.closeEdit()
	}
	if msg.err != nil {
		if !errors.Is(msg.err, context.Canceled) {
			m.Report("error", "catalog", msg.err.Error())
		}
		return nil
	}
	m.notice = msg.result.Message
	m.Report("info", "catalog", msg.result.Message)
	if msg.origin == m.query().Key() {
		m.restore = m.cursorFor(m.query())
		m.restoring = true
		return m.fetch(true, true)
	}
	return nil
}

func (m *Model) editKey(msg tea.KeyPressMsg, key string) tea.Cmd {
	d := m.edit
	action := m.keys.ActionFor("Dialog", key)
	if key == "esc" || key == "ctrl+c" || (d.phase != "name" && (key == "q" || action == "back")) {
		m.closeEdit()
		return nil
	}
	if d.phase == "name" {
		switch key {
		case "enter":
			if strings.TrimSpace(d.draft) == "" {
				return nil
			}
			if m.mutationBusy {
				return nil
			}
			return m.mutateCatalog(catalog.CreatePlaylist, "")
		case "backspace":
			r := []rune(d.draft)
			if len(r) > 0 {
				d.draft = string(r[:len(r)-1])
			}
		case "ctrl+u":
			d.draft = ""
		default:
			if msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
				d.draft += msg.Text
			} else if msg.Mod == 0 && unicode.IsPrint(msg.Code) {
				d.draft += string(msg.Code)
			}
			d.draft = boundedName(d.draft)
		}
		return nil
	}
	if d.loading || d.phase == "writing" || d.phase == "info" {
		return nil
	}
	n := len(d.choices)
	if d.phase == "playlists" {
		n = len(d.playlists)
	}
	if n == 0 {
		return nil
	}
	switch action {
	case "up":
		d.cursor = (d.cursor + n - 1) % n
	case "down":
		d.cursor = (d.cursor + 1) % n
	case "top":
		d.cursor = 0
	case "bottom":
		d.cursor = n - 1
	case "page_up":
		d.cursor = max(0, d.cursor-max(1, m.height/2))
	case "page_down":
		d.cursor = min(n-1, d.cursor+max(1, m.height/2))
	case "open", "activate":
		if d.phase == "playlists" {
			return m.mutateCatalog(catalog.AddPlaylist, d.playlists[d.cursor].ID)
		}
		choice := d.choices[d.cursor]
		switch choice.op {
		case "cancel":
			m.closeEdit()
		case "queue", "play-next":
			cmd := *d.queue
			if choice.op == "play-next" {
				cmd.Op = "play-next"
			}
			m.closeEdit()
			return m.command(cmd)
		case "playlist":
			return m.choosePlaylists()
		default:
			return m.mutateCatalog(choice.op, "")
		}
	}
	return nil
}

func (m *Model) editView() tea.View {
	d := m.edit
	w := min(76, m.width-4)
	title := "Add to"
	if d.operation == catalog.CreatePlaylist {
		title = "Create playlist"
	}
	if d.phase == "playlists" {
		title = "Choose editable playlist"
	}
	if d.operation == "remove" {
		title = "Remove"
	}
	rows := []string{text.Bold(true).Render(fit(plain(d.title), w-4)), ""}
	switch {
	case d.loading:
		rows = append(rows, muted.Render("Loading…"))
	case d.phase == "writing":
		rows = append(rows, good.Render("Saving to music library…"), muted.Render("Esc hides this dialog; the request continues."))
	case d.phase == "name":
		rows = append(rows, "Name: "+plain(d.draft)+"▏", muted.Render("Enter create · Esc cancel · Ctrl-u clear"))
	case d.phase == "info":
		rows = append(rows, strings.Split(ansi.Wrap(plain(d.note), w-4, ""), "\n")...)
	case d.phase == "playlists":
		available := max(1, m.height-10)
		start := max(0, d.cursor-available+1)
		for i := start; i < min(len(d.playlists), start+available); i++ {
			label := "  " + plain(d.playlists[i].Title)
			if i == d.cursor {
				label = selected.Render("› " + plain(d.playlists[i].Title))
			}
			rows = append(rows, label)
		}
	default:
		for i, choice := range d.choices {
			label := "  " + choice.label
			if i == d.cursor {
				label = selected.Render("› " + choice.label)
			}
			rows = append(rows, label)
		}
	}
	if d.phase != "name" {
		rows = append(rows, "", muted.Render("j/k select · Enter/l confirm · Esc close"))
	}
	rows = rows[:min(len(rows), max(1, m.height-4))]
	return terminalView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box(title, rows, w, len(rows)+2, true)))
}

// Bound paste separately from keystrokes, preserving valid UTF-8 names.
func boundedName(s string) string {
	r := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s))
	if len(r) > 200 {
		r = r[:200]
	}
	return string(r)
}
