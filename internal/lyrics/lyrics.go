// Package lyrics contains source-independent, bounded line-synced lyrics.
package lyrics

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxBytes = 1 << 20
const MaxLines = 2000

type Line struct {
	Start time.Duration `json:"start"`
	Text  string        `json:"text"`
}
type Document struct {
	Lines    []Line `json:"lines,omitempty"`
	Provider string `json:"provider,omitempty"`
}

func (d Document) Valid() bool {
	if len(d.Lines) > MaxLines || len(d.Provider) > 80 {
		return false
	}
	total := 0
	for i, line := range d.Lines {
		total += len(line.Text)
		if line.Start < 0 || line.Start > 24*time.Hour || len(line.Text) > 4096 || !utf8.ValidString(line.Text) || i > 0 && line.Start <= d.Lines[i-1].Start {
			return false
		}
		for _, r := range line.Text {
			if unicode.IsControl(r) && r != '\n' {
				return false
			}
		}
	}
	return total <= MaxBytes
}

// Active returns -1 during the intro; seeking backwards uses the same lookup.
func (d Document) Active(position time.Duration) int {
	return sort.Search(len(d.Lines), func(i int) bool { return d.Lines[i].Start > position }) - 1
}

var timestamp = regexp.MustCompile(`^\[(\d{1,3}):(\d{2}(?:[.:]\d{1,3})?)\]`)

func ParseLRC(raw string) (Document, error) {
	if len(raw) > MaxBytes || !utf8.ValidString(raw) {
		return Document{}, fmt.Errorf("lyrics exceed the supported size or encoding")
	}
	var lines []Line
	for _, line := range strings.Split(strings.TrimPrefix(raw, "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		var starts []time.Duration
		for {
			parts := timestamp.FindStringSubmatch(line)
			if parts == nil {
				break
			}
			minutes, _ := strconv.Atoi(parts[1])
			seconds, err := strconv.ParseFloat(strings.Replace(parts[2], ":", ".", 1), 64)
			if err != nil || seconds >= 60 {
				return Document{}, fmt.Errorf("invalid lyric timestamp")
			}
			starts = append(starts, time.Duration((float64(minutes)*60+seconds)*float64(time.Second)))
			line = line[len(parts[0]):]
		}
		text := strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				if r == '\t' {
					return ' '
				}
				return -1
			}
			return r
		}, line))
		if len(text) > 4096 {
			return Document{}, fmt.Errorf("lyric line exceeds the supported size")
		}
		for _, start := range starts {
			lines = append(lines, Line{start, text})
		}
		if len(lines) > MaxLines {
			return Document{}, fmt.Errorf("lyrics exceed the line limit")
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Start < lines[j].Start })
	d := Document{}
	for _, line := range lines {
		if len(d.Lines) > 0 && d.Lines[len(d.Lines)-1].Start == line.Start {
			previous := &d.Lines[len(d.Lines)-1]
			if line.Text != "" && line.Text != previous.Text {
				previous.Text = strings.TrimSpace(previous.Text + "\n" + line.Text)
			}
		} else {
			d.Lines = append(d.Lines, line)
		}
	}
	if !d.Valid() {
		return Document{}, fmt.Errorf("invalid lyric document")
	}
	return d, nil
}
