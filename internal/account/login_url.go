package account

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
)

var localLoginURL = regexp.MustCompile(`http://(?:localhost|127\.0\.0\.1|\[::1\]):[0-9]+/login\?[^\s"<>]+`)
var loginState = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

// loginURLWriter consumes helper stdout without retaining or displaying its
// other output. Only a loopback /login URL for the requested port is reported.
type loginURLWriter struct {
	port  int
	onURL func(string)
	line  []byte
	last  string
}

func (w *loginURLWriter) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		part := data
		if i >= 0 {
			part, data = data[:i], data[i+1:]
		} else {
			data = nil
		}
		if len(w.line)+len(part) <= 8192 {
			w.line = append(w.line, part...)
		} else {
			w.line = nil
		}
		if i < 0 {
			break
		}
		for _, candidate := range localLoginURL.FindAllString(string(w.line), -1) {
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
		w.line = nil
	}
	return n, nil
}
