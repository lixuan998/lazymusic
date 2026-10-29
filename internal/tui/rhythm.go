package tui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

type rhythmCell struct {
	char rune
	ink  int
}

// Palettes share the application's coral/mint accents. Faint trails stay in the
// foreground; no background fills or terminal palette changes are required.
var rhythmStyles = func() []lipgloss.Style {
	styles := []lipgloss.Style{lipgloss.NewStyle(), lipgloss.NewStyle().Foreground(lipgloss.Color("#343342")), lipgloss.NewStyle().Foreground(lipgloss.Color("#a8e4d0"))}
	for i := 0; i < 16; i++ {
		styles = append(styles, rhythmColor(0xc54164, 0xffd3b6, float64(i)/15, 1))
	}
	for i := 0; i < 4; i++ {
		styles = append(styles, rhythmColor(0x77334e, 0x242733, float64(i)/3, 1))
	}
	for _, strength := range []float64{.25, .5, 1} {
		for i := 0; i < 12; i++ {
			styles = append(styles, rhythmColor(0xff92ad, 0x9de5d5, float64(i)/11, strength))
		}
	}
	return styles
}()

func rhythmColor(from, to int, t, strength float64) lipgloss.Style {
	background := 0x1b1e28
	var color int
	for _, shift := range []int{16, 8, 0} {
		value := float64(from>>shift&255)*(1-t) + float64(to>>shift&255)*t
		value = float64(background>>shift&255)*(1-strength) + value*strength
		color |= int(value+.5) << shift
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%06x", color)))
}

func (m *Model) visualizerRows(width, height int) []string {
	return m.rhythmRows(m.UI.Visualizer, width, height)
}

func (m *Model) rhythmRows(mode, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	cells := make([]rhythmCell, width*height)
	a := m.rhythm
	if a.initialized {
		mode = a.mode
	} else {
		a.fade = 1
	}
	if mode == 1 {
		drawWave(cells, width, height, a)
	} else {
		drawPulse(cells, width, height, a)
	}
	return renderRhythm(cells, width, height)
}

func drawPulse(cells []rhythmCell, w, h int, a rhythmAnimation) {
	bands := min(rhythmBands, max(1, (w+1)/3))
	barWidth := min(2, max(1, (w-bands+1)/bands))
	used := bands*barWidth + bands - 1
	left := (w - used) / 2
	reflection := 0
	if h >= 9 {
		reflection = min(3, h/6)
	}
	chart := max(1, h-reflection-2)
	// A little air above the peaks and below the reflection frames the shape.
	top := (h - chart - reflection) / 2
	blocks := []rune(" ▁▂▃▄▅▆▇█")
	for band := 0; band < bands; band++ {
		index := 0
		if bands > 1 {
			index = int(math.Round(float64(band) / float64(bands-1) * (rhythmBands - 1)))
		}
		units := math.Max(.5, a.levels[index]*a.fade*float64(chart*8))
		for row := 0; row < chart; row++ {
			amount := int(math.Round(units - float64((chart-row-1)*8)))
			amount = max(0, min(8, amount))
			if amount == 0 {
				continue
			}
			ink := 3 + int(float64(chart-row-1)/float64(max(1, chart-1))*15)
			if a.energy < .01 {
				ink = 1
			}
			for x := 0; x < barWidth; x++ {
				cells[(top+row)*w+left+band*(barWidth+1)+x] = rhythmCell{blocks[amount], ink}
			}
		}
		peak := int(math.Round(a.peaks[index] * a.fade * float64(chart)))
		if peak >= 2 && float64(peak) > units/8+.7 {
			row := top + max(0, chart-peak)
			for x := 0; x < barWidth; x++ {
				cells[row*w+left+band*(barWidth+1)+x] = rhythmCell{'▔', 2}
			}
		}
		for row := 0; row < reflection; row++ {
			amount := max(0, min(8, int(math.Round(units*.24-float64(row*8)))))
			if amount == 0 {
				break
			}
			for x := 0; x < barWidth; x++ {
				cells[(top+chart+row)*w+left+band*(barWidth+1)+x] = rhythmCell{blocks[amount], 19 + row}
			}
		}
	}
}

func drawWave(cells []rhythmCell, w, h int, a rhythmAnimation) {
	// A braille cell holds a 2x4 dot grid, allowing curves to move in sub-cell
	// steps rather than jumping entire terminal rows between frames.
	dots := make([]uint8, w*h)
	tone := make([]uint8, w*h)
	bits := [4][2]uint8{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	plot := func(x, y, ink int) {
		if x < 0 || x >= w*2 || y < 0 || y >= h*4 {
			return
		}
		index := y/4*w + x/2
		dots[index] |= bits[y%4][x%2]
		tone[index] = max(tone[index], uint8(ink))
	}
	width, height := w*2, h*4
	center := float64(height-1) / 2
	strength := a.energy * a.fade
	amplitude := float64(max(1, height-8)) * .35 * strength
	for _, layer := range []struct {
		delay, scale float64
		ink          int
	}{{.85, .55, 1}, {.4, .78, 1}, {-.3, .84, 2}, {0, 1, 3}} {
		previous := 0
		for x := 0; x < width; x++ {
			u := float64(x) / float64(max(1, width-1))
			envelope := math.Pow(math.Sin(math.Pi*u), .7)
			phase := a.phase*1.9 - layer.delay
			wave := math.Sin(u*math.Pi*3.4-phase) + .18*math.Sin(u*math.Pi*7+phase*.7)
			y := int(math.Round(center + wave*amplitude*layer.scale*envelope))
			ink := layer.ink
			if strength < .02 {
				ink = 1
			}
			if x == 0 {
				previous = y
			}
			for point := min(previous, y); point <= max(previous, y); point++ {
				plot(x, point, ink)
			}
			previous = y
		}
	}
	for i, mask := range dots {
		if mask == 0 {
			continue
		}
		hue := min(11, i%w*12/max(1, w))
		cells[i] = rhythmCell{rune(0x2800) + rune(mask), 23 + (int(tone[i])-1)*12 + hue}
	}
}

func renderRhythm(cells []rhythmCell, width, height int) []string {
	rows := make([]string, height)
	for y := range rows {
		var line, run strings.Builder
		ink := 0
		flush := func() {
			if run.Len() > 0 {
				line.WriteString(rhythmStyles[ink].Render(run.String()))
				run.Reset()
			}
		}
		for _, cell := range cells[y*width : (y+1)*width] {
			if cell.char == 0 {
				cell = rhythmCell{' ', 0}
			}
			if cell.ink != ink {
				flush()
				ink = cell.ink
			}
			run.WriteRune(cell.char)
		}
		flush()
		rows[y] = line.String()
	}
	return rows
}
