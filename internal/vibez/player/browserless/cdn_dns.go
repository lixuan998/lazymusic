//go:build darwin || linux || windows

package browserless

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var errCDNDNS = errors.New("alternate Apple video CDN resolution unavailable")

type cdnDNS struct{ http *http.Client }
type cdnDNSAnswer struct {
	Status   *int
	TC       bool
	Question []struct {
		Name string
		Type int
	}
	Answer []struct {
		Name, Data string
		Type       int
		TTL        uint32
	}
}

func (d *cdnDNS) lookup(ctx context.Context) ([]netip.Addr, time.Duration, error) {
	return d.lookupProviders(ctx, []string{"https://dns.google/resolve", "https://cloudflare-dns.com/dns-query"})
}
func (d *cdnDNS) lookupBackup(ctx context.Context) ([]netip.Addr, time.Duration, error) {
	return d.lookupProviders(ctx, []string{"https://cloudflare-dns.com/dns-query"})
}
func (d *cdnDNS) lookupProviders(ctx context.Context, providers []string) ([]netip.Addr, time.Duration, error) {
	// Only the public CDN hostname is sent, never a media URL or account data.
	for _, endpoint := range providers {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		query := url.Values{"name": {videoCDNHost}, "type": {"A"}, "cd": {"false"}, "do": {"false"}}
		if strings.Contains(endpoint, "dns.google") {
			query.Set("edns_client_subnet", "0.0.0.0/0")
		}
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return nil, 0, errCDNDNS
		}
		r.Header.Set("Accept", "application/dns-json")
		response, err := d.http.Do(r)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10+1))
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || len(body) > 64<<10 {
			continue
		}
		ips, ttl, err := parseCDNDNS(body)
		if err == nil {
			return ips, ttl, nil
		}
	}
	return nil, 0, errCDNDNS
}

func parseCDNDNS(body []byte) ([]netip.Addr, time.Duration, error) {
	var answer cdnDNSAnswer
	if json.Unmarshal(body, &answer) != nil || answer.Status == nil || *answer.Status != 0 || answer.TC || len(answer.Question) != 1 || len(answer.Answer) > 64 || answer.Question[0].Type != 1 || dnsName(answer.Question[0].Name) != videoCDNHost {
		return nil, 0, errCDNDNS
	}
	names := map[string]bool{videoCDNHost: true}
	ttl := 5 * time.Minute
	for range 8 {
		changed := false
		for _, record := range answer.Answer {
			if record.Type == 5 && names[dnsName(record.Name)] && !names[dnsName(record.Data)] {
				names[dnsName(record.Data)] = true
				ttl = min(ttl, time.Duration(record.TTL)*time.Second)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	var ips []netip.Addr
	seen := map[netip.Addr]bool{}
	for _, record := range answer.Answer {
		if record.Type != 1 || !names[dnsName(record.Name)] {
			continue
		}
		ip, err := netip.ParseAddr(record.Data)
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if !ip.Is4() || !publicCDNAddress(ip) || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		ttl = min(ttl, time.Duration(record.TTL)*time.Second)
		if len(ips) == 4 {
			break
		}
	}
	if len(ips) == 0 {
		return nil, 0, errCDNDNS
	}
	return ips, ttl, nil
}

func dnsName(name string) string { return strings.ToLower(strings.TrimSuffix(name, ".")) }

var excludedCDNAddresses = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
}

func publicCDNAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, prefix := range excludedCDNAddresses {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
