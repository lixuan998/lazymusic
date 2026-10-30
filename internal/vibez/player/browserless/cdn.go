//go:build darwin || linux

package browserless

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

const videoCDNHost = "mvod.itunes.apple.com"

type cdnRoute struct {
	ip        netip.Addr
	transport http.RoundTripper
	badUntil  time.Time // protected by cdnTransport.mu
}

// cdnTransport keeps fallback routing local to one playback server. Requests
// retain their original URL, Range and TLS hostname; only the connection's
// destination changes. Authentication endpoints never use this transport.
type cdnTransport struct {
	primary                      http.RoundTripper
	lookup                       func(context.Context) ([]netip.Addr, time.Duration, error)
	lookupBackup                 func(context.Context) ([]netip.Addr, time.Duration, error)
	newRoute                     func(netip.Addr) http.RoundTripper
	log                          func(string)
	now                          func() time.Time
	mu                           sync.Mutex
	routes                       []*cdnRoute
	preferred                    *cdnRoute
	expires, retryLookup, notice time.Time
	resolving                    chan struct{}
	closed                       bool
	backupTried                  bool
	attemptTimeout               time.Duration
}

func newCDNTransport(primary http.RoundTripper, log func(string)) *cdnTransport {
	c := &cdnTransport{primary: primary, log: log, now: time.Now, attemptTimeout: 6 * time.Second}
	dns := &cdnDNS{http: &http.Client{Transport: primary, Timeout: 2500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	c.lookup = dns.lookup
	c.lookupBackup = dns.lookupBackup
	c.newRoute = func(ip netip.Addr) http.RoundTripper { return pinnedCDNTransport(ip) }
	return c
}

func pinnedCDNTransport(ip netip.Addr) *http.Transport {
	dialer := &net.Dialer{Timeout: 2500 * time.Millisecond, KeepAlive: 30 * time.Second}
	return &http.Transport{
		// A CONNECT proxy would resolve the original host again. This one
		// retry connects directly to the resolved CDN, without changing any
		// proxy environment, system settings or other application clients.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), "443"))
		},
		TLSClientConfig:   &tls.Config{ServerName: videoCDNHost, MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true, TLSHandshakeTimeout: 3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second, IdleConnTimeout: time.Minute,
		MaxIdleConns: 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2,
	}
}

func eligibleCDN(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Scheme == "https" && strings.EqualFold(r.URL.Hostname(), videoCDNHost) &&
		(r.URL.Port() == "" || r.URL.Port() == "443") && r.URL.User == nil &&
		(r.Host == "" || strings.EqualFold(r.Host, r.URL.Host)) && r.Body == nil
}

func cdnRequest(r *http.Request) *http.Request {
	copy := r.Clone(r.Context())
	copy.Header = make(http.Header)
	for _, name := range []string{"Range", "If-Range", "Accept"} {
		for _, value := range r.Header.Values(name) {
			copy.Header.Add(name, value)
		}
	}
	return copy
}

func (c *cdnTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, net.ErrClosed
	}
	if !eligibleCDN(r) {
		return c.primary.RoundTrip(r)
	}
	c.mu.Lock()
	preferred := c.preferred
	if c.closed || !c.now().Before(c.expires) || preferred != nil && c.now().Before(preferred.badUntil) {
		preferred = nil
	}
	c.mu.Unlock()
	if preferred != nil {
		response, err := c.tryRoute(r, preferred)
		if err == nil && response.StatusCode != http.StatusForbidden {
			return response, nil
		}
		if response != nil {
			response.Body.Close()
		}
		if r.Context().Err() != nil {
			return nil, r.Context().Err()
		}
		c.mu.Lock()
		if c.preferred == preferred {
			c.preferred = nil
		}
		c.mu.Unlock()
	}
	response, err := c.primary.RoundTrip(r)
	if r.Context().Err() != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, r.Context().Err()
	}
	if err != nil || response.StatusCode != http.StatusForbidden {
		return response, err
	}
	// The media callers only consume the status of a failed response. Close it
	// before DNS/network retries and leave an empty, readable body on failure.
	response.Body.Close()
	response.Body = http.NoBody
	noticed := c.noticeFallback()
	routes := c.candidates(r.Context())
	tried := make(map[netip.Addr]bool)
	if preferred != nil {
		tried[preferred.ip] = true
	}
	for pass := 0; pass < 2; pass++ {
		for _, route := range routes {
			if len(tried) >= 4 {
				break
			}
			if route == preferred || tried[route.ip] {
				continue
			}
			if err := r.Context().Err(); err != nil {
				return nil, err
			}
			c.mu.Lock()
			bad := c.now().Before(route.badUntil)
			c.mu.Unlock()
			if bad {
				continue
			}
			tried[route.ip] = true
			alternate, err := c.tryRoute(r, route)
			if err != nil {
				continue
			}
			if alternate.StatusCode == http.StatusMovedPermanently || alternate.StatusCode == http.StatusFound || alternate.StatusCode == http.StatusSeeOther || alternate.StatusCode == http.StatusTemporaryRedirect || alternate.StatusCode == http.StatusPermanentRedirect {
				return alternate, nil // The media client validates and follows redirects.
			}
			if alternate.StatusCode == http.StatusOK || alternate.StatusCode == http.StatusPartialContent {
				return alternate, nil
			}
			alternate.Body.Close()
		}
		if pass == 0 && len(tried) < 4 {
			routes = c.backupCandidates(r.Context())
		} else {
			break
		}
	}
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	if noticed && c.log != nil {
		c.log("[cdn] warning: alternate Apple video CDN retry did not recover HTTP 403")
	}
	return response, nil
}

type cdnReadError struct{ cause error }

func (e *cdnReadError) Error() string { return "alternate Apple video CDN body read failed" }
func (e *cdnReadError) Unwrap() error { return e.cause }

type cdnBody struct {
	io.ReadCloser
	finish func(bool)
	once   sync.Once
}

func (b *cdnBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(func() { b.finish(err == io.EOF) })
		if err != io.EOF {
			return n, &cdnReadError{err}
		}
	}
	return n, err
}
func (b *cdnBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(false) })
	return err
}

func (c *cdnTransport) failedRoute(route *cdnRoute) {
	c.mu.Lock()
	route.badUntil = c.now().Add(10 * time.Second)
	if c.preferred == route {
		c.preferred = nil
	}
	c.mu.Unlock()
}

func (c *cdnTransport) tryRoute(r *http.Request, route *cdnRoute) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(r.Context(), c.attemptTimeout)
	request := cdnRequest(r).WithContext(ctx)
	response, err := route.transport.RoundTrip(request)
	if err != nil {
		cancel()
		if r.Context().Err() == nil {
			c.failedRoute(route)
		}
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		if response.StatusCode == http.StatusForbidden {
			c.failedRoute(route)
		}
		response.Body = &cdnBody{ReadCloser: response.Body, finish: func(bool) { cancel() }}
		return response, nil
	}
	response.Body = &cdnBody{ReadCloser: response.Body, finish: func(complete bool) {
		cancel()
		if !complete {
			if r.Context().Err() == nil {
				c.failedRoute(route)
			}
			return
		}
		c.mu.Lock()
		current := !c.closed && slices.Contains(c.routes, route)
		changed := current && c.preferred != route
		if current && c.now().Before(c.expires) && !c.now().Before(route.badUntil) {
			c.preferred = route
		}
		c.mu.Unlock()
		if changed && c.log != nil {
			c.log("[cdn] Apple video CDN recovered after HTTP 403; using a temporary alternate route")
		}
	}}
	return response, nil
}

func (c *cdnTransport) noticeFallback() bool {
	c.mu.Lock()
	log := !c.now().Before(c.notice)
	if log {
		c.notice = c.now().Add(30 * time.Second)
	}
	c.mu.Unlock()
	if log && c.log != nil {
		c.log("[cdn] Apple video media returned HTTP 403; trying alternate CDN resolution")
	}
	return log
}

func (c *cdnTransport) candidates(ctx context.Context) []*cdnRoute {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil
		}
		if c.now().Before(c.expires) {
			routes := append([]*cdnRoute(nil), c.routes...)
			c.mu.Unlock()
			return routes
		}
		if c.now().Before(c.retryLookup) {
			c.mu.Unlock()
			return nil
		}
		if pending := c.resolving; pending != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil
			case <-pending:
				continue
			}
		}
		pending := make(chan struct{})
		c.resolving = pending
		c.mu.Unlock()
		ips, ttl, err := c.lookup(ctx)
		var routes []*cdnRoute
		if err == nil {
			for _, ip := range ips {
				if publicCDNAddress(ip) {
					routes = append(routes, &cdnRoute{ip: ip, transport: c.newRoute(ip)})
				}
				if len(routes) == 2 {
					break
				}
			}
		}
		c.mu.Lock()
		old := c.routes
		// A DNS refresh need not discard a healthy TLS connection, nor forget
		// that the same address just stalled on a concurrent media request.
		for i, route := range routes {
			for _, previous := range old {
				if previous.ip == route.ip {
					closeIdle(route.transport)
					routes[i] = previous
					break
				}
			}
		}
		if c.closed {
			for _, route := range routes {
				closeIdle(route.transport)
			}
			routes = nil
		}
		c.routes, c.preferred = routes, nil
		c.backupTried = false
		c.expires = c.now().Add(min(ttl, 5*time.Minute))
		if len(routes) == 0 && ctx.Err() == nil {
			c.retryLookup = c.now().Add(10 * time.Second)
		}
		c.resolving = nil
		close(pending)
		c.mu.Unlock()
		for _, route := range old {
			if !slices.Contains(routes, route) {
				closeIdle(route.transport)
			}
		}
		return routes
	}
}

// A resolver can return valid addresses whose CDN nodes still reject the
// asset. Try the second provider once as well, without retrying duplicate IPs.
func (c *cdnTransport) backupCandidates(ctx context.Context) []*cdnRoute {
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil
		}
		if pending := c.resolving; pending != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil
			case <-pending:
				continue
			}
		}
		if c.backupTried || c.lookupBackup == nil {
			routes := append([]*cdnRoute(nil), c.routes...)
			c.mu.Unlock()
			return routes
		}
		c.backupTried = true
		pending := make(chan struct{})
		c.resolving = pending
		c.mu.Unlock()
		ips, ttl, err := c.lookupBackup(ctx)
		c.mu.Lock()
		var expired []*cdnRoute
		if err == nil && !c.closed {
			baseValid := c.now().Before(c.expires)
			if !baseValid {
				expired = c.routes
				c.routes = nil
				c.preferred = nil
			}
			seen := map[netip.Addr]bool{}
			for _, route := range c.routes {
				seen[route.ip] = true
			}
			for _, ip := range ips {
				if publicCDNAddress(ip) && !seen[ip] && len(c.routes) < 4 {
					c.routes = append(c.routes, &cdnRoute{ip: ip, transport: c.newRoute(ip)})
					seen[ip] = true
				}
			}
			if len(c.routes) > 0 {
				expires := c.now().Add(min(ttl, 5*time.Minute))
				if !baseValid || expires.Before(c.expires) {
					c.expires = expires
				}
			}
		}
		routes := append([]*cdnRoute(nil), c.routes...)
		c.resolving = nil
		close(pending)
		c.mu.Unlock()
		for _, route := range expired {
			closeIdle(route.transport)
		}
		return routes
	}
}

func closeIdle(transport http.RoundTripper) {
	if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (c *cdnTransport) close() {
	c.mu.Lock()
	c.closed = true
	routes := c.routes
	c.routes, c.preferred = nil, nil
	c.mu.Unlock()
	for _, route := range routes {
		closeIdle(route.transport)
	}
	closeIdle(c.primary)
}
