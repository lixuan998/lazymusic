package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"lazymusic/internal/vibez/config"
)

func ReadConfig(stateDir string) (*config.Config, error) {
	cfg := &config.Config{AuthPort: 38747, Provider: "apple", AudioQuality: "high"}
	b, err := os.ReadFile(filepath.Join(stateDir, "credentials.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse credentials file: %w", err)
		}
	}
	if v := os.Getenv("LAZYMUSIC_DEVELOPER_TOKEN"); v != "" {
		cfg.AppleDeveloperToken = v
	}
	return cfg, nil
}

func Logout(stateDir string) error {
	err := os.Remove(filepath.Join(stateDir, "credentials.json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ValidateAccount is deliberately strict: network errors and 429/5xx are
// inconclusive failures, never evidence that credentials were accepted.
func ValidateAccount(ctx context.Context, cfg *config.Config) (string, error) {
	if cfg.AppleDeveloperToken == "" {
		return "", errors.New("Apple Music developer token missing")
	}
	if cfg.AppleUserToken == "" {
		return "", errors.New("Music User Token missing; run login")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.music.apple.com/v1/me/storefront", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AppleDeveloperToken)
	req.Header.Set("Music-User-Token", cfg.AppleUserToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Apple account check HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&body); err != nil {
		return "", err
	}
	if len(body.Data) == 0 || body.Data[0].ID == "" {
		return "", errors.New("Apple returned no storefront")
	}
	return body.Data[0].ID, nil
}
