//go:build darwin || linux || windows

package browserless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (s *StreamServer) closeMedia() { s.mediaClient().Transport.(*cdnTransport).close() }

// Retry an unreadable alternate body once. Media is buffered before decoding or
// output, so a retry cannot duplicate audio or append half a video fragment.
func (s *StreamServer) readMedia(ctx context.Context, uri string, offset, length, limit int64) ([]byte, string, error) {
	if _, err := mediaURL(uri, ""); err != nil {
		return nil, "", err
	}
	for attempt := 0; attempt < 2; attempt++ {
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, "", fmt.Errorf("invalid media request")
		}
		if length > 0 {
			r.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		}
		response, err := s.mediaClient().Do(r)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			return nil, "", fmt.Errorf("media network request failed")
		}
		if response.StatusCode != 200 && response.StatusCode != 206 {
			response.Body.Close()
			return nil, "", fmt.Errorf("media HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		response.Body.Close()
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if err != nil {
			var alternate *cdnReadError
			if attempt == 0 && errors.As(err, &alternate) {
				continue
			}
			return nil, "", fmt.Errorf("media body read failed")
		}
		if int64(len(data)) > limit {
			return nil, "", fmt.Errorf("media exceeds size limit")
		}
		finalURL := uri
		if response.Request != nil {
			finalURL = response.Request.URL.String()
		}
		return data, finalURL, nil
	}
	return nil, "", fmt.Errorf("media retry failed")
}

func (s *StreamServer) mediaClient() *http.Client {
	s.mediaOnce.Do(func() {
		primary := http.DefaultTransport
		if transport, ok := primary.(*http.Transport); ok {
			primary = transport.Clone()
		}
		transport := newCDNTransport(primary, func(message string) {
			if s.onLog != nil {
				s.onLog(message)
			}
		})
		s.mediaHTTP = &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return io.ErrUnexpectedEOF
			}
			_, err := mediaURL(r.URL.String(), "")
			return err
		}}
	})
	return s.mediaHTTP
}
