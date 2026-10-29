package account

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var localLoginURL = regexp.MustCompile(`http://(?:localhost|127\.0\.0\.1|\[::1\]):[0-9]+/login\?[^\s"<>]+`)
var loginState = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

// loginURLWriter consumes both helper streams without forwarding raw output.
// Only the expected loopback login URL and allowlisted failure reasons survive.
type loginURLWriter struct {
	port    int
	onURL   func(string)
	line    []byte
	last    string
	problem error
	discard bool
}

func (w *loginURLWriter) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		i := bytes.IndexAny(data, "\r\n")
		part := data
		if i >= 0 {
			part, data = data[:i], data[i+1:]
		} else {
			data = nil
		}
		if !w.discard && len(w.line)+len(part) <= 8192 {
			w.line = append(w.line, part...)
		} else {
			w.line = nil
			w.discard = true
		}
		if i < 0 {
			break
		}
		w.flush()
	}
	return n, nil
}

func (w *loginURLWriter) flush() {
	defer func() { w.line, w.discard = nil, false }()
	if w.discard {
		return
	}
	line := ansi.Strip(string(w.line))
	if problem := helperProblem(line); problem != nil {
		w.problem = problem
	}
	if w.onURL != nil {
		for _, candidate := range localLoginURL.FindAllString(strings.TrimSpace(line), -1) {
			u, err := url.Parse(candidate)
			if err != nil || u.User != nil || u.Fragment != "" || u.Path != "/login" || u.Port() != strconv.Itoa(w.port) {
				continue
			}
			host := u.Hostname()
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				continue
			}
			query, err := url.ParseQuery(u.RawQuery)
			if err != nil || len(query) != 1 || len(query["state"]) != 1 || !loginState.MatchString(query.Get("state")) {
				continue
			}
			if candidate != w.last {
				w.last = candidate
				w.onURL(candidate)
			}
		}
	}
}
