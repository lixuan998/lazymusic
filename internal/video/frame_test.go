package video

import (
	"bytes"
	"testing"
	"time"
)

func TestEncodedPictureIsBoundedAndPreservesPresentationTime(t *testing.T) {
	f, err := Encode(make([]byte, 4*16*9), 16, 9, 1200*time.Millisecond)
	if err != nil || !f.Valid() || f.Position != 1200*time.Millisecond {
		t.Fatal("invalid decoded picture", err)
	}
	f.Width++
	if f.Valid() {
		t.Fatal("accepted dimensions different from PNG")
	}
	for _, size := range [][2]int{{0, 1}, {MaxWidth + 1, 1}, {1, MaxHeight + 1}, {16, 9}} {
		if _, err := Encode(nil, size[0], size[1], 0); err == nil {
			t.Fatal("accepted invalid dimensions or pixel length", size)
		}
	}
}

func TestPNGWorkspaceReuseDoesNotOverwriteEarlierPictures(t *testing.T) {
	pixels := make([]byte, 16*9*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+3] = 255, 255
	}
	first, err := Encode(pixels, 16, 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	retained := bytes.Clone(first.PNG)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+2] = 0, 255
	}
	if _, err := Encode(pixels, 16, 9, time.Second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, first.PNG) || !first.Valid() {
		t.Fatal("reusing PNG workspaces corrupted a queued picture")
	}
}
