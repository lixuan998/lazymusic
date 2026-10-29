package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// LoginViaVibez uses the official client's normal auth command. Its broker
// endpoint/client key are never extracted, copied, or embedded in lazymusic.
// Developer/user tokens remain associated with the Vibez application identity.
func LoginViaVibez(ctx context.Context, stateDir, helper string, port int) error {
	return loginViaVibez(ctx, stateDir, helper, port, false)
}

// LoginViaVibezQuiet keeps helper output out of the in-app Login screen.
func LoginViaVibezQuiet(ctx context.Context, stateDir, helper string, port int) error {
	return loginViaVibez(ctx, stateDir, helper, port, true)
}
func loginViaVibez(ctx context.Context, stateDir, helper string, port int, quiet bool) error {
	helper, err := exec.LookPath(helper)
	if err != nil {
		return fmt.Errorf("Vibez login helper: %w", err)
	}
	helper, err = filepath.Abs(helper)
	if err != nil {
		return err
	}
	stateDir, err = filepath.Abs(stateDir)
	if err != nil {
		return err
	}
	cfg, err := ReadConfig(stateDir)
	if err != nil {
		return err
	}
	cfg.AuthPort = port
	path := filepath.Join(stateDir, "credentials.json")
	if err := cfg.Save(path); err != nil {
		return err
	}
	if !quiet {
		fmt.Fprintln(os.Stderr, "Apple Music login uses Vibez's application identity and token service.")
		fmt.Fprintln(os.Stderr, "The browser is needed only for authorization; close its login pages when done.")
	}
	cmd := exec.CommandContext(ctx, helper, "auth", "login", "--config", path, "--no-update", "--no-discord")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if quiet {
		cmd.Stdin = nil
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Vibez authorization helper: %w", err)
	}
	cfg, err = ReadConfig(stateDir)
	if err != nil {
		return err
	}
	if cfg.AppleDeveloperToken == "" || cfg.AppleUserToken == "" {
		return fmt.Errorf("authorization helper finished without both required credentials")
	}
	// Presence is not validation; play still checks Apple and the licensed stream.
	provenance := map[string]any{
		"method": "official-vibez-auth-helper", "application_identity": "Vibez",
		"helper": helper, "authorized_at": time.Now().UTC(),
		"apple_account_validated": false,
	}
	b, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "credential-source.json"), append(b, '\n'), 0600); err != nil {
		return err
	}
	return nil
}
