// Package termimage implements original-resolution terminal image transport.
package termimage

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// IDs stay in a private 24-bit range so their RGB foreground values survive
// both ordinary terminal rendering and Neovim's terminal buffer.
const MinID = 0x6d0001
const MaxID = 0x6dffff

func Query(id int) string {
	return fmt.Sprintf("\x1b_Ga=q,i=%d,f=24,s=1,v=1;AAAA\x1b\\", id)
}
func Delete(id int) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}
func Placement(id, columns, rows int) string {
	return fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,p=1,c=%d,r=%d,q=2\x1b\\", id, columns, rows)
}

// Transmit sends only encoded image data over the PTY. File and shared-memory
// transfers would break when the terminal is on the other side of SSH/WSL.
func Transmit(png []byte, id, columns, rows int) string {
	data := base64.StdEncoding.EncodeToString(png)
	var out strings.Builder
	for first := true; len(data) > 0; first = false {
		n := min(4096, len(data))
		more := 0
		if n < len(data) {
			more = 1
		}
		if first {
			fmt.Fprintf(&out, "\x1b_Ga=T,U=1,f=100,t=d,i=%d,p=1,c=%d,r=%d,q=2,m=%d;", id, columns, rows, more)
		} else {
			fmt.Fprintf(&out, "\x1b_Gq=2,m=%d;", more)
		}
		out.WriteString(data[:n])
		out.WriteString("\x1b\\")
		data = data[n:]
	}
	return out.String()
}

// Relay wraps the same bounded graphics commands for a buffer-local Neovim
// TermRequest handler. OSC is also understood by older embedded terminals.
func Relay(commands string) string {
	commands = strings.ReplaceAll(commands, "\x1b_G", "\x1b]777;LAZYMUSIC;")
	return strings.ReplaceAll(commands, "\x1b\\", "\a")
}

func PlaceholderRows(id, columns, rows int) []string {
	out := make([]string, rows)
	for y := range out {
		var line strings.Builder
		fmt.Fprintf(&line, "\x1b[38;2;%d;%d;%dm", id>>16&255, id>>8&255, id&255)
		for x := 0; x < columns; x++ {
			line.WriteRune(kitty.Placeholder)
			line.WriteRune(kitty.Diacritic(y))
			line.WriteRune(kitty.Diacritic(x))
		}
		line.WriteString("\x1b[39m")
		out[y] = line.String()
	}
	return out
}
