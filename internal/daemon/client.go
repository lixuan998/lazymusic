package daemon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"lazymusic/internal/artwork"
	"lazymusic/internal/catalog"
	"lazymusic/internal/lyrics"
	"lazymusic/internal/session"
	"lazymusic/internal/video"
)

type Client struct {
	http   *http.Client
	token  string
	source string
}

func NewClient(dir string) (*Client, error) {
	path, err := SocketPath(dir)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialIPC(ctx, path)
	}}
	return &Client{http: &http.Client{Transport: tr, Timeout: 90 * time.Second}}, nil
}

func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://lazymusic.local"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Lazymusic-Client", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var e APIError
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("%s", e.Error)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func (c *Client) State(ctx context.Context) (State, error) {
	var s State
	err := c.call(ctx, http.MethodGet, "/v1/state", nil, &s)
	return s, err
}
func (c *Client) Attach(ctx context.Context) (Attachment, error) {
	var a Attachment
	err := c.call(ctx, http.MethodPost, "/v1/attach", AttachRequest{PID: os.Getpid()}, &a)
	if err == nil {
		c.token = a.Token
		c.source = a.State.Source.ID
	}
	return a, err
}
func (c *Client) SaveUI(ctx context.Context, u session.UI, revision uint64) error {
	return c.call(ctx, http.MethodPost, "/v1/ui", UIUpdate{UI: u, Revision: revision}, nil)
}
func (c *Client) Detach(ctx context.Context, u session.UI, revision uint64) error {
	if err := c.SaveUI(ctx, u, revision); err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, "/v1/detach", nil, nil)
}
func (c *Client) Quit(ctx context.Context) error {
	return c.call(ctx, http.MethodPost, "/v1/quit", nil, nil)
}
func (c *Client) Command(ctx context.Context, cmd Command) (State, error) {
	var s State
	err := c.call(ctx, http.MethodPost, "/v1/command", cmd, &s)
	return s, err
}
func (c *Client) Browse(ctx context.Context, q catalog.Query) (catalog.Page, error) {
	b, _ := json.Marshal(q)
	var p catalog.Page
	err := c.call(ctx, http.MethodGet, "/v1/browse?q="+url.QueryEscape(string(b)), nil, &p)
	return p, err
}
func (c *Client) Inspect(ctx context.Context, subject catalog.Subject) (catalog.Details, error) {
	var details catalog.Details
	err := c.call(ctx, http.MethodPost, "/v1/inspect", subject, &details)
	return details, err
}
func (c *Client) Lyrics(ctx context.Context, subject catalog.Subject) (lyrics.Document, error) {
	var doc lyrics.Document
	err := c.call(ctx, http.MethodPost, "/v1/lyrics", subject, &doc)
	return doc, err
}
func (c *Client) Artwork(ctx context.Context, subject catalog.Subject) (artwork.Image, error) {
	var im artwork.Image
	err := c.call(ctx, http.MethodPost, "/v1/artwork", subject, &im)
	return im, err
}
func (c *Client) Actions(ctx context.Context, subject catalog.Subject) (catalog.Actions, error) {
	var actions catalog.Actions
	err := c.call(ctx, http.MethodPost, "/v1/actions", subject, &actions)
	return actions, err
}
func (c *Client) Mutate(ctx context.Context, mutation catalog.Mutation) (catalog.MutationResult, error) {
	var result catalog.MutationResult
	err := c.call(ctx, http.MethodPost, "/v1/mutate", mutation, &result)
	return result, err
}
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) VideoFrame(ctx context.Context, id string) (video.Frame, error) {
	data, _ := json.Marshal(catalog.Subject{Source: c.source, ID: id})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://lazymusic.local/v1/video", bytes.NewReader(data))
	if err != nil {
		return video.Frame{}, err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Lazymusic-Client", c.token)
	response, err := c.http.Do(r)
	if err != nil {
		return video.Frame{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return video.Frame{}, nil
	}
	if response.StatusCode != 200 {
		var failure APIError
		json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&failure)
		if failure.Error == "" {
			failure.Error = response.Status
		}
		return video.Frame{}, fmt.Errorf("%s", failure.Error)
	}
	if response.Header.Get("Content-Type") != "image/png" || len(response.Header.Get(videoMetadataHeader)) > 4096 {
		return video.Frame{}, fmt.Errorf("invalid video response")
	}
	metadata, err := base64.RawStdEncoding.DecodeString(response.Header.Get(videoMetadataHeader))
	if err != nil {
		return video.Frame{}, fmt.Errorf("invalid video metadata")
	}
	var frame video.Frame
	if json.Unmarshal(metadata, &frame) != nil {
		return video.Frame{}, fmt.Errorf("invalid video metadata")
	}
	frame.PNG, err = io.ReadAll(io.LimitReader(response.Body, video.MaxBytes+1))
	if err != nil {
		return video.Frame{}, err
	}
	if frame.Source != c.source || frame.ID != id || !frame.Valid() {
		return video.Frame{}, fmt.Errorf("invalid video frame")
	}
	return frame, nil
}
