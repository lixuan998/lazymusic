//go:build darwin || linux

package browserless

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cdnRoundTrip func(*http.Request) (*http.Response, error)

type stalledCDNBody struct {
	ctx     context.Context
	partial bool
}

func (b *stalledCDNBody) Read(p []byte) (int, error) {
	if !b.partial {
		b.partial = true
		return copy(p, "incomplete-prefix"), nil
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (*stalledCDNBody) Close() error { return nil }

func (f cdnRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func cdnResponse(code int) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response"))}
}
func cdnGet(ctx context.Context) *http.Request {
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://"+videoCDNHost+"/signed%2Fpath/segment?token=private-test-token", nil)
	return r
}

func testCDNTransport(primary http.RoundTripper, log func(string)) *cdnTransport {
	c := newCDNTransport(primary, log)
	c.lookupBackup = nil
	return c
}

func TestCDNOnlyFallsBackForForbiddenVideoMedia(t *testing.T) {
	for _, tc := range []struct {
		url, method string
		status      int
	}{
		{"https://" + videoCDNHost + "/segment", "GET", 200},
		{"https://" + videoCDNHost + "/segment", "GET", 401},
		{"https://" + videoCDNHost + "/segment", "GET", 404},
		{"https://" + videoCDNHost + "/segment", "GET", 500},
		{"https://" + videoCDNHost + "/license", "POST", 403},
		{"https://api.music.apple.com/v1/play/assets", "GET", 403},
		{"https://play.itunes.apple.com/license", "GET", 403},
		{"https://" + videoCDNHost + ".example/segment", "GET", 403},
		{"http://" + videoCDNHost + "/segment", "GET", 403},
		{"https://" + videoCDNHost + ":444/segment", "GET", 403},
	} {
		t.Run(tc.url+tc.method+http.StatusText(tc.status), func(t *testing.T) {
			c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(tc.status), nil }), nil)
			c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
				t.Fatal("unrelated request reached public DNS")
				return nil, 0, nil
			}
			r, _ := http.NewRequest(tc.method, tc.url, nil)
			response, err := c.RoundTrip(r)
			if err != nil || response.StatusCode != tc.status {
				t.Fatal("changed ordinary request", err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			c.close()
		})
	}
}

func TestCDNRecoversRangeWithoutChangingSignedURLAndCachesRouteUntilTTL(t *testing.T) {
	var primary, lookups, alternate atomic.Int32
	now := time.Now()
	var logs []string
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { primary.Add(1); return cdnResponse(403), nil }), func(s string) { logs = append(logs, s) })
	t.Cleanup(c.close)
	c.now = func() time.Time { return now }
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		lookups.Add(1)
		return []netip.Addr{netip.MustParseAddr("17.0.0.1")}, time.Minute, nil
	}
	c.newRoute = func(netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(r *http.Request) (*http.Response, error) {
			alternate.Add(1)
			if r.URL.String() != cdnGet(context.Background()).URL.String() || r.Header.Get("Range") != "bytes=7-1023" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Music-User-Token") != "" {
				t.Fatal("changed signature/range or forwarded account headers")
			}
			return cdnResponse(206), nil
		})
	}
	r := cdnGet(context.Background())
	r.Header.Set("Range", "bytes=7-1023")
	r.Header.Set("Authorization", "must-stay-local")
	r.Header.Set("Cookie", "must-stay-local")
	r.Header.Set("Music-User-Token", "must-stay-local")
	for i := 0; i < 3; i++ {
		resp, err := c.RoundTrip(r)
		if err != nil || resp.StatusCode != 206 {
			t.Fatal("fallback failed", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if primary.Load() != 1 || lookups.Load() != 1 || alternate.Load() != 3 {
		t.Fatal("repeated primary failures or DNS requests", primary.Load(), lookups.Load(), alternate.Load())
	}
	now = now.Add(2 * time.Minute)
	resp, err := c.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if primary.Load() != 2 || lookups.Load() != 2 {
		t.Fatal("cache ignored DNS expiry")
	}
	for _, log := range logs {
		if strings.Contains(log, "private-test-token") || strings.Contains(log, "signed") || strings.Contains(log, "17.0.0.1") {
			t.Fatal("sensitive routing data leaked to logs")
		}
	}
}

func TestCDNFailureIsBoundedAndCancellationDoesNotBecomeForbidden(t *testing.T) {
	var retries atomic.Int32
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	t.Cleanup(c.close)
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		return []netip.Addr{netip.MustParseAddr("17.0.0.1"), netip.MustParseAddr("17.0.0.2"), netip.MustParseAddr("17.0.0.3")}, time.Minute, nil
	}
	c.newRoute = func(netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(*http.Request) (*http.Response, error) { retries.Add(1); return cdnResponse(403), nil })
	}
	resp, err := c.RoundTrip(cdnGet(context.Background()))
	if err != nil || resp.StatusCode != 403 || retries.Load() != 2 {
		t.Fatal("unbounded retry or false success", err, retries.Load())
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.RoundTrip(cdnGet(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation became a CDN error", err)
	}
}

func TestCDNFailedPreferredNodeRotatesWithoutRepeatingFailedNode(t *testing.T) {
	var lookups, first, second atomic.Int32
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	defer c.close()
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		lookups.Add(1)
		return []netip.Addr{netip.MustParseAddr("17.0.0.1"), netip.MustParseAddr("17.0.0.2")}, time.Minute, nil
	}
	c.newRoute = func(ip netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(*http.Request) (*http.Response, error) {
			if ip.String() == "17.0.0.1" {
				if first.Add(1) > 1 {
					return cdnResponse(403), nil
				}
			} else {
				second.Add(1)
			}
			return cdnResponse(200), nil
		})
	}
	for range 3 {
		response, err := c.RoundTrip(cdnGet(context.Background()))
		if err != nil || response.StatusCode != 200 {
			t.Fatal("rotation failed", err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}
	if lookups.Load() != 1 || first.Load() != 2 || second.Load() != 2 {
		t.Fatal("repeated failed route or discarded valid DNS cache", lookups.Load(), first.Load(), second.Load())
	}
}

func TestCDNQueriesSecondProviderWhenFirstProvidersNodesAreForbidden(t *testing.T) {
	var primary, backup atomic.Int32
	counts := map[string]int{}
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { primary.Add(1); return cdnResponse(403), nil }), nil)
	defer c.close()
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		return []netip.Addr{netip.MustParseAddr("17.0.0.1"), netip.MustParseAddr("17.0.0.2")}, time.Minute, nil
	}
	c.lookupBackup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		backup.Add(1)
		return []netip.Addr{netip.MustParseAddr("17.0.0.1"), netip.MustParseAddr("17.0.0.3")}, time.Minute, nil
	}
	c.newRoute = func(ip netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(*http.Request) (*http.Response, error) {
			counts[ip.String()]++
			if ip.String() == "17.0.0.3" {
				return cdnResponse(200), nil
			}
			return cdnResponse(403), nil
		})
	}
	for range 2 {
		response, err := c.RoundTrip(cdnGet(context.Background()))
		if err != nil || response.StatusCode != 200 {
			t.Fatal("secondary provider did not recover valid media", err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}
	if primary.Load() != 1 || backup.Load() != 1 || counts["17.0.0.1"] != 1 || counts["17.0.0.2"] != 1 || counts["17.0.0.3"] != 2 {
		t.Fatal("repeated addresses, DNS or failed primary route", counts, primary.Load(), backup.Load())
	}
}

func TestCDNOldBodyCannotRestoreRouteReplacedByDNSRefresh(t *testing.T) {
	now := time.Now()
	n := 0
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	defer c.close()
	c.now = func() time.Time { return now }
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		n++
		return []netip.Addr{netip.AddrFrom4([4]byte{17, 0, 0, byte(n)})}, time.Second, nil
	}
	c.newRoute = func(netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(200), nil })
	}
	old, err := c.RoundTrip(cdnGet(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	current, err := c.RoundTrip(cdnGet(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, current.Body)
	current.Body.Close()
	preferred := c.preferred
	io.Copy(io.Discard, old.Body)
	old.Body.Close()
	if c.preferred != preferred || preferred.ip.String() != "17.0.0.2" {
		t.Fatal("late completion restored expired node")
	}
}

func TestMediaReadRetriesStalledAlternateWithoutCombiningPartialBodies(t *testing.T) {
	var first, second atomic.Int32
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	defer c.close()
	c.attemptTimeout = 20 * time.Millisecond
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		return []netip.Addr{netip.MustParseAddr("17.0.0.1"), netip.MustParseAddr("17.0.0.2")}, time.Minute, nil
	}
	c.newRoute = func(ip netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(r *http.Request) (*http.Response, error) {
			response := cdnResponse(206)
			if r.Header.Get("Range") != "bytes=4-7" {
				t.Error("lost byte range on body retry")
			}
			if ip.String() == "17.0.0.1" {
				first.Add(1)
				response.Body = &stalledCDNBody{ctx: r.Context()}
			} else {
				second.Add(1)
				response.Body = io.NopCloser(strings.NewReader("good"))
			}
			return response, nil
		})
	}
	s := &StreamServer{}
	s.mediaOnce.Do(func() { s.mediaHTTP = &http.Client{Transport: c, Timeout: time.Second} })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, err := s.mediaBytes(ctx, cdnGet(ctx).URL.String(), 4, 4, 128)
	if err != nil || string(data) != "good" || first.Load() != 1 || second.Load() != 1 {
		t.Fatal("partial packet was appended or stalled node retried", string(data), err, first.Load(), second.Load())
	}
}

func TestCDNConcurrentRequestsShareResolutionAndWaitingCanCancel(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	t.Cleanup(c.close)
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []netip.Addr{netip.MustParseAddr("17.0.0.1")}, time.Minute, nil
	}
	c.newRoute = func(netip.Addr) http.RoundTripper {
		return cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(200), nil })
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.RoundTrip(cdnGet(context.Background()))
			if err != nil || r.StatusCode != 200 {
				t.Error("concurrent recovery failed", err)
			}
			if r != nil {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}
		}()
	}
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan []*cdnRoute, 1)
	go func() { waiting <- c.candidates(ctx) }()
	cancel()
	select {
	case routes := <-waiting:
		if len(routes) != 0 {
			t.Fatal("canceled DNS waiter obtained routes")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter blocked behind another lookup")
	}
	if _, err := c.RoundTrip(cdnGet(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatal("waiting cancellation ignored", err)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("simultaneous audio/video retried DNS independently", calls.Load())
	}
}

func TestPinnedCDNVerifiesOriginalTLSHostnameAndPreservesHost(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: videoCDNHost}, DNSNames: []string{videoCDNHost}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != videoCDNHost || r.TLS.ServerName != videoCDNHost || r.URL.RawQuery != "token=private-test-token" {
			t.Error("lost original TLS or HTTP identity")
		}
		w.WriteHeader(206)
	}))
	s.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	s.StartTLS()
	defer s.Close()
	c := testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	defer c.close()
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		return []netip.Addr{netip.MustParseAddr("17.0.0.1")}, time.Minute, nil
	}
	c.newRoute = func(ip netip.Addr) http.RoundTripper {
		transport := pinnedCDNTransport(ip)
		transport.TLSClientConfig.RootCAs = roots
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != videoCDNHost+":443" {
				t.Error("HTTP transport changed URL identity", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
		}
		return transport
	}
	response, err := c.RoundTrip(cdnGet(context.Background()))
	if err != nil || response.StatusCode != 206 {
		t.Fatal("verified TLS fallback failed", err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	// A missing trusted root must reject the same endpoint; verification is
	// never disabled simply because an alternate IP was selected.
	c.close()
	c = testCDNTransport(cdnRoundTrip(func(*http.Request) (*http.Response, error) { return cdnResponse(403), nil }), nil)
	defer c.close()
	c.lookup = func(context.Context) ([]netip.Addr, time.Duration, error) {
		return []netip.Addr{netip.MustParseAddr("17.0.0.1")}, time.Minute, nil
	}
	c.newRoute = func(ip netip.Addr) http.RoundTripper {
		transport := pinnedCDNTransport(ip)
		transport.DialContext = func(ctx context.Context, n, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, n, s.Listener.Addr().String())
		}
		return transport
	}
	c.mu.Lock()
	c.expires = time.Time{}
	c.mu.Unlock()
	response, err = c.RoundTrip(cdnGet(context.Background()))
	if err != nil || response.StatusCode != 403 {
		t.Fatal("untrusted CDN certificate accepted", err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
}
