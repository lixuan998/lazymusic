package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"lazymusic/internal/backend"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	"lazymusic/internal/session"
	"lazymusic/internal/tui"
	"testing"
)

func keyboardApplication() *application {
	ctx := context.Background()
	keys := tui.DefaultKeys()
	state := daemon.State{Source: music.Source{ID: "demo", Name: "DEMO", Capabilities: music.Capabilities{Home: true, Library: true, Playlists: true, CatalogSearch: true}}, UI: session.DefaultUI()}
	return &application{ctx: ctx, keys: keys, epoch: 1, music: tui.New(ctx, offlineAPI{state}, state, keys), login: &loginPage{ctx: ctx, entries: backend.Sources(), phase: "sources"}}
}
func press(text string) tea.KeyPressMsg { return tea.KeyPressMsg{Code: []rune(text)[0], Text: text} }
func TestNavigationIsImmediateAndRepeatedKeysNeverFocusAnotherPanel(t *testing.T) {
	a := keyboardApplication()
	a.music.UI.Focus = 0
	a.Update(press("j"))
	if a.music.UI.NavIndex != 1 || a.music.UI.Focus != 0 {
		t.Fatal("single j was delayed or changed focus")
	}
	for i := 0; i < 20; i++ {
		a.Update(press("j"))
	}
	if a.music.UI.Focus != 0 {
		t.Fatal("held j switched panels")
	}
	for _, letter := range []string{"h", "j", "k", "l"} {
		a = keyboardApplication()
		a.music.UI.Focus = 1
		for i := 0; i < 20; i++ {
			a.Update(press(letter))
		}
		if a.music.UI.Focus != 1 {
			t.Fatalf("repeated %s switched panels", letter)
		}
	}
}
