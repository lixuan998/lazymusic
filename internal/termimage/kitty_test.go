package termimage

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOriginalImageIsTransmittedUnchangedInBoundedDirectChunks(t *testing.T) {
	original := bytes.Repeat([]byte("original image data"), 1000)
	s := Transmit(original, MinID, 12, 6)
	parts := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\").FindAllStringSubmatch(s, -1)
	var payload strings.Builder
	for i, p := range parts {
		if len(p[2]) > 4096 {
			t.Fatal("oversize protocol chunk")
		}
		if i == 0 && (!strings.Contains(p[1], "U=1") || !strings.Contains(p[1], "t=d") || !strings.Contains(p[1], "f=100")) {
			t.Fatal("not a direct virtual PNG placement")
		}
		payload.WriteString(p[2])
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil || !bytes.Equal(decoded, original) {
		t.Fatal("original data changed during terminal transport")
	}
	for _, line := range PlaceholderRows(MinID, 12, 6) {
		if ansi.StringWidth(line) != 12 {
			t.Fatal("native placeholders changed cell geometry")
		}
	}
	relay := Relay(s)
	if strings.Contains(relay, "\x1b_G") || !strings.HasPrefix(relay, "\x1b]777;LAZYMUSIC;") {
		t.Fatal("invalid Neovim relay")
	}
}

func TestFrameUpdatesOnlyEditTheOwnedRootInBoundedChunks(t *testing.T) {
	data := bytes.Repeat([]byte("frame pixels"), 1000)
	command := UpdateFrame(data, MinID, 163, 63)
	parts := regexp.MustCompile("\x1b_G([^;]*);([^\x1b]*)\x1b\\\\").FindAllStringSubmatch(command, -1)
	var decoded strings.Builder
	for _, part := range parts {
		if !strings.Contains(part[1], "a=f,i=") || !strings.Contains(part[1], "r=1") || len(part[2]) > 4096 {
			t.Fatal("frame created extra images or oversized packets")
		}
		decoded.WriteString(part[2])
	}
	payload, err := base64.StdEncoding.DecodeString(decoded.String())
	if err != nil || !bytes.Equal(payload, data) || !strings.Contains(command, "x=163,y=63,X=1") || !strings.Contains(command, "a=a,i=") {
		t.Fatal("corrupt or misplaced disc patch")
	}
	if !strings.Contains(ProbeFrames(MinID), "r=1,f=32") || !strings.Contains(ProbeFrames(MinID), ";AAAAAA==") {
		t.Fatal("probe changes the photograph")
	}
}
