// lazymusic is a native music TUI with a persistent background player.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"lazymusic/internal/account"
	"lazymusic/internal/app"
	"lazymusic/internal/backend"
	"lazymusic/internal/daemon"
	"lazymusic/internal/music"
	localfiles "lazymusic/internal/sources/local"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// version may be set by release builds with -ldflags "-X main.version=...".
var version = "0.5.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lazymusic:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "tui"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if command == "help" || len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print(`lazymusic — native APPLE MUSIC and LOCAL in your terminal

Run lazymusic to open or restore your session. Login lives in the content panel.
  q then q / Q   Background / quit completely
  + / _          Next / previous layout (Full, Content, Queue)
  @              Toggle Log window; Esc dismisses a runtime notification
  h/j/k/l        Navigate within a panel
  Ctrl-h/j/k/l   Focus a neighboring panel
  H/J/K/L        Resize the focused panel (Shift+h/j/k/l)
  o / c          Album/artist links / credits in panels 2, 3 and 4
  a              Add menu; create playlist in Playlists (panel 2)
  y / P          Queue selection / play song or collection (panel 2)
  v              Switch video / browser (panel 2)
  d / D          Remove / clear playback queue (panel 3)
  [ / ]          Reorder queue (panel 3)
  < / >          Previous / next song (Global)
  Ctrl+< / Ctrl+>  Seek backward / forward 10s (Global)
  Space / x      Play-pause / stop (Global)
  m              Cycle playback mode (Global)
  r / s          Repeat / shuffle (Queue / Player)
  , / .         Content category, navigation animation, or player mode
  b              Choose a supported preferred audio quality (panel 4)
  = / -          System output volume
  ?              Current panel + Global commands

Commands: tui (default), status, ctl, quit, logout, sources, version
Options: --source apple|local, --music-dir PATH, --state-dir PATH
`)
		return nil
	}
	if command == "version" {
		fmt.Printf("lazymusic %s\n", version)
		return nil
	}
	if command == "setup-nvim" {
		return setupNeovim(args)
	}
	if command == "sources" {
		for _, s := range backend.Sources() {
			fmt.Printf("%s\t%s\n", s.ID, s.Name)
		}
		return nil
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	defaultDir, err := app.DefaultDir()
	if err != nil {
		return err
	}
	state := f.String("state-dir", defaultDir, "private runtime/session directory")
	source := f.String("source", "", "apple or local")
	musicDir := f.String("music-dir", "", "LOCAL root folder, default ~/Music")
	op := f.String("op", "", "control command")
	ids := f.String("ids", "", "comma-separated track IDs")
	index := f.Int("index", 0, "queue index")
	to := f.Int("to", 0, "destination index")
	position := f.Duration("position", 0, "absolute position")
	delta := f.Duration("delta", 0, "relative seek")
	volume := f.Float64("volume", .5, "system volume 0..1")
	quality := f.String("quality", "", "preferred quality ID")
	eqBand := f.Int("eq-band", 0, "equalizer band 0..9")
	eqDelta := f.Float64("eq-delta", 0, "equalizer adjustment in dB")
	eqPreset := f.String("eq-preset", "", "flat, bass, vocal or treble")
	eqAction := f.String("eq-action", "", "toggle bypass")
	step := f.Int("step", 1, "playback mode step")
	if err = f.Parse(args); err != nil {
		return err
	}
	if len(f.Args()) > 0 {
		return fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	explicit := os.Getenv("LAZYMUSIC_STATE_DIR") != ""
	f.Visit(func(v *flag.Flag) {
		if v.Name == "state-dir" {
			explicit = true
		}
	})
	base, err := filepath.Abs(*state)
	if err != nil {
		return err
	}
	*state, *source, err = app.ResolveProfile(base, explicit, *source)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *musicDir != "" {
		if *source != music.Local {
			return fmt.Errorf("--music-dir applies to LOCAL")
		}
		if err = localfiles.Configure(*state, *musicDir); err != nil {
			return err
		}
		if c, e := daemon.NewClient(*state); e == nil {
			if _, stateErr := c.State(ctx); stateErr == nil {
				_, e = c.Command(ctx, daemon.Command{Op: "clear"})
				if e == nil {
					_, e = c.Command(ctx, daemon.Command{Op: "reload-auth"})
				}
			}
			c.Close()
			if e != nil {
				return e
			}
		}
	}
	switch command {
	case "tui", "login":
		return app.RunApplication(ctx, app.LaunchOptions{Base: base, Dir: *state, Source: *source, Explicit: explicit, OpenLogin: command == "login", MusicDir: *musicDir})
	case "daemon":
		signal.Ignore(syscall.SIGHUP)
		return daemon.Run(ctx, daemon.Options{Dir: *state, Backend: *source})
	case "status":
		c, e := daemon.NewClient(*state)
		if e != nil {
			return e
		}
		defer c.Close()
		s, e := c.State(ctx)
		if e != nil {
			return fmt.Errorf("no background service for this profile")
		}
		return json.NewEncoder(os.Stdout).Encode(s)
	case "ctl":
		if *op == "" {
			return fmt.Errorf("ctl requires --op")
		}
		c, e := daemon.Ensure(ctx, *state, *source)
		if e != nil {
			return e
		}
		defer c.Close()
		var tracks []string
		for _, id := range strings.Split(*ids, ",") {
			if id = strings.TrimSpace(id); id != "" {
				tracks = append(tracks, id)
			}
		}
		s, e := c.Command(ctx, daemon.Command{EQBand: *eqBand, EQDelta: *eqDelta, EQPreset: *eqPreset, EQAction: *eqAction, Op: *op, IDs: tracks, Index: *index, To: *to, Position: *position, Delta: *delta, Volume: *volume, Quality: *quality, ModeStep: *step})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(s)
	case "quit", "logout":
		c, e := daemon.NewClient(*state)
		if e != nil {
			return e
		}
		defer c.Close()
		if _, e = c.State(ctx); e == nil {
			if e = c.Quit(ctx); e != nil {
				return e
			}
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				if _, e = c.State(ctx); e != nil {
					break
				}
				time.Sleep(30 * time.Millisecond)
			}
			if _, stillRunning := c.State(ctx); stillRunning == nil {
				return fmt.Errorf("background service is still stopping; retry %s", command)
			}
		}
		if command == "quit" {
			return nil
		}
		if *source == music.Apple {
			if e = account.Logout(*state); e != nil {
				return e
			}
		}
		for _, name := range []string{"local.json", "session.json", "credential-source.json"} {
			if e = os.Remove(filepath.Join(*state, name)); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q; run lazymusic help", command)
	}
}
