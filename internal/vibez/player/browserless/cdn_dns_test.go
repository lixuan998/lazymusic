//go:build darwin || linux || windows

package browserless

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func syntheticCDNDNS() map[string]any {
	return map[string]any{
		"Status": 0, "Question": []any{map[string]any{"name": videoCDNHost + ".", "type": 1}},
		"Answer": []any{
			map[string]any{"name": videoCDNHost + ".", "type": 5, "TTL": 90, "data": "edge.example."},
			map[string]any{"name": "edge.example.", "type": 1, "TTL": 300, "data": "17.0.0.1"},
			map[string]any{"name": "unrelated.example.", "type": 1, "TTL": 300, "data": "17.0.0.2"},
		},
	}
}

func TestCDNDNSFollowsOnlyQueriedCNAMEChainAndRespectsTTL(t *testing.T) {
	data, _ := json.Marshal(syntheticCDNDNS())
	ips, ttl, err := parseCDNDNS(data)
	if err != nil || len(ips) != 1 || ips[0].String() != "17.0.0.1" || ttl != 90*time.Second {
		t.Fatal("unrelated addresses or invalid cache lifetime", ips, ttl, err)
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.1.1", "198.18.1.108", "100.64.0.1", "0.1.2.3", "192.0.2.1", "203.0.113.1", "240.0.0.1", "::1", "fc00::1", "::ffff:127.0.0.1"} {
		if publicCDNAddress(netip.MustParseAddr(ip)) {
			t.Fatal("unsafe DNS destination accepted", ip)
		}
	}
	for _, status := range []int{2, 3} {
		answer := syntheticCDNDNS()
		answer["Status"] = status
		data, _ = json.Marshal(answer)
		if _, _, err := parseCDNDNS(data); err == nil {
			t.Fatal("accepted failed DNS reply")
		}
	}
	answer := syntheticCDNDNS()
	answer["TC"] = true
	data, _ = json.Marshal(answer)
	if _, _, err := parseCDNDNS(data); err == nil {
		t.Fatal("accepted truncated DNS reply")
	}
	answer = syntheticCDNDNS()
	answer["Question"] = []any{map[string]any{"name": "other.example", "type": 1}}
	data, _ = json.Marshal(answer)
	if _, _, err := parseCDNDNS(data); err == nil {
		t.Fatal("accepted wrong DNS question")
	}
}

func TestCDNDNSUsesFixedProvidersAndSendsOnlyPublicHostname(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: cdnRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Query().Get("name") != videoCDNHost || r.URL.Query().Get("type") != "A" || r.Header.Get("Accept") != "application/dns-json" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Body != nil {
			t.Fatal("DNS request contains account/media data or incorrect query")
		}
		if calls == 1 {
			if r.URL.Host != "dns.google" || r.URL.Query().Get("edns_client_subnet") != "0.0.0.0/0" {
				t.Fatal("unexpected primary resolver or client-subnet disclosure")
			}
			return cdnResponse(503), nil
		}
		if r.URL.Host != "cloudflare-dns.com" {
			t.Fatal("unexpected secondary resolver")
		}
		data, _ := json.Marshal(syntheticCDNDNS())
		response := cdnResponse(200)
		response.Body = io.NopCloser(strings.NewReader(string(data)))
		return response, nil
	})}
	ips, _, err := (&cdnDNS{http: client}).lookup(context.Background())
	if err != nil || len(ips) != 1 || calls != 2 {
		t.Fatal("secondary DNS did not recover", err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = (&cdnDNS{http: client}).lookup(ctx); !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatal("canceled DNS continued network requests", err)
	}
}

func TestCDNDNSRejectsOversizedOrPrivateReplies(t *testing.T) {
	for _, body := range []string{
		strings.Repeat(" ", 65537),
		`{"Status":0,"Question":[{"name":"mvod.itunes.apple.com","type":1}],"Answer":[{"name":"mvod.itunes.apple.com","type":1,"TTL":60,"data":"198.18.1.108"}]}`,
		`{"Question":[{"name":"mvod.itunes.apple.com","type":1}]}`,
	} {
		client := &http.Client{Transport: cdnRoundTrip(func(*http.Request) (*http.Response, error) {
			r := cdnResponse(200)
			r.Body = io.NopCloser(strings.NewReader(body))
			return r, nil
		})}
		if _, _, err := (&cdnDNS{http: client}).lookup(context.Background()); err == nil {
			t.Fatal("accepted unusable DNS response")
		}
	}
}
