package tui

import (
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/mattn/go-runewidth"
)

var clock = time.Now

// SideState is what a person chose for the room's sides in one repository:
// the sides turned off and the tab showing on each side (plugin/id).
type SideState struct {
	Off  []string          `json:"off,omitempty"`
	Tabs map[string]string `json:"tabs,omitempty"`
}

const (
	topWide    = 78
	bottomWide = 85
	sideCols   = 30
	sideRows   = 6
)

var colours = map[string]string{
	"red": "1", "green": "2", "yellow": "3", "blue": "4",
	"magenta": "5", "cyan": "6", "white": "7", "gray": "8",
}

func styleOf(spec string) lipgloss.Style {
	s := lipgloss.NewStyle()
	for _, word := range strings.Fields(spec) {
		switch word {
		case "bold":
			s = s.Bold(true)
		case "dim":
			s = s.Faint(true)
		case "underline":
			s = s.Underline(true)
		case "strike":
			s = s.Strikethrough(true)
		default:
			if c, ok := colours[word]; ok {
				s = s.Foreground(lipgloss.Color(c))
			}
		}
	}
	return s
}

type cell struct {
	text  string
	style string
	cont  bool
}

type canvas [][]cell

func newCanvas(width, height int) canvas {
	c := make(canvas, height)
	for y := range c {
		c[y] = make([]cell, width)
	}
	return c
}

// put writes text from x up to (not including) end and returns where it
// stopped; a wide rune takes the cell after it too.
func (c canvas) put(x, y int, text, style string, end int) int {
	if y < 0 || y >= len(c) {
		return x
	}
	end = min(end, len(c[y]))
	for _, r := range text {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			continue
		}
		if x+w > end {
			break
		}
		if x >= 0 {
			c[y][x] = cell{text: string(r), style: style}
			if w == 2 {
				c[y][x+1] = cell{cont: true}
			}
		}
		x += w
	}
	return x
}

func (c canvas) segments(x, y int, segs []plugin.Segment, end int) int {
	for _, s := range segs {
		x = c.put(x, y, s.Text, s.Style, end)
	}
	return x
}

func segmentsWidth(segs []plugin.Segment) int {
	w := 0
	for _, s := range segs {
		w += runewidth.StringWidth(s.Text)
	}
	return w
}

func (c canvas) line(x, y int, line plugin.Line, end int) {
	for _, cl := range line {
		text := cl.Text
		w := runewidth.StringWidth(text)
		if cl.Width > 0 {
			text = runewidth.Truncate(text, cl.Width, "")
			w = cl.Width
		}
		at := x
		if cl.Align == "right" {
			at = x + w - runewidth.StringWidth(text)
		}
		c.put(at, y, text, cl.Style, min(x+w, end))
		x += w
		if x >= end {
			return
		}
	}
}

func lineWidth(line plugin.Line) int {
	w := 0
	for _, cl := range line {
		if cl.Width > 0 {
			w += cl.Width
		} else {
			w += runewidth.StringWidth(cl.Text)
		}
	}
	return w
}

// dots lays a · on every 2nd column from (x0, y0) toward (x1, y1), y
// rounded along x, on cells nothing else has taken.
func (c canvas) dots(x0, y0, x1, y1 int) {
	dx := x1 - x0
	step := 2
	if dx < 0 {
		dx, step = -dx, -2
	}
	if dx == 0 {
		return
	}
	for k := 0; k <= dx; k += 2 {
		x := x0 + k*step/2
		y := y0 + int(math.Round(float64((y1-y0)*k)/float64(dx)))
		if y >= 0 && y < len(c) && x >= 0 && x < len(c[y]) && c[y][x].text == "" && !c[y][x].cont {
			c[y][x] = cell{text: "·", style: "gray"}
		}
	}
}

func (c canvas) render(y, from, to int) string {
	var b strings.Builder
	run, style := "", ""
	flush := func() {
		if run == "" {
			return
		}
		if style == "" {
			b.WriteString(run)
		} else {
			b.WriteString(styleOf(style).Render(run))
		}
		run = ""
	}
	for x := from; x < to; x++ {
		cl := c[y][x]
		if cl.cont {
			continue
		}
		text := cl.text
		if text == "" {
			text = " "
		}
		if cl.style != style {
			flush()
			style = cl.style
		}
		run += text
	}
	flush()
	return b.String()
}

// box is where the dash's box sits: centered as lipgloss.Place puts it.
type box struct {
	width, height            int
	left, top, right, bottom int
}

func (b box) fits(place string) bool {
	switch place {
	case "top":
		return b.top >= sideRows && b.width >= topWide+4
	case "bottom":
		return b.height-1-b.bottom >= sideRows && b.width >= bottomWide+4
	case "left":
		return b.left >= sideCols
	case "right":
		return b.width-1-b.right >= sideCols
	}
	return false
}

func (m model) off(place string) bool { return slices.Contains(m.sides.Off, place) }

func tabKey(tab plugin.SideTab) string { return tab.Plugin + "/" + tab.ID }

func (m model) tabsAt(place string) []plugin.SideTab {
	if m.panel.Room == nil {
		return nil
	}
	var tabs []plugin.SideTab
	for _, tab := range m.panel.Room.Sides {
		if tab.Where == place {
			tabs = append(tabs, tab)
		}
	}
	return tabs
}

func (m model) activeAt(place string, tabs []plugin.SideTab) int {
	for i, tab := range tabs {
		if tabKey(tab) == m.sides.Tabs[place] {
			return i
		}
	}
	return 0
}

// room draws the dash's box inside the room — dotted lines to the
// terminal's corners, a status line in each corner and the plugins' sides
// where they fit — or says no when not one side fits.
func (m model) room(boxed string) (string, bool) {
	if m.panel.Room == nil || m.fill {
		return "", false
	}
	lines := strings.Split(boxed, "\n")
	b := box{width: m.width, height: m.height}
	bw := 0
	for _, line := range lines {
		bw = max(bw, lipgloss.Width(line))
	}
	b.left, b.top = (m.width-bw)/2, (m.height-len(lines))/2
	if m.off("bottom") {
		// An odd spare row goes above the box when nothing is drawn below
		// it, or the top side can miss its sixth row for nothing.
		b.top = (m.height - len(lines) + 1) / 2
	}
	b.right, b.bottom = b.left+bw-1, b.top+len(lines)-1
	if !b.fits("top") && !b.fits("left") && !b.fits("right") && !b.fits("bottom") {
		return "", false
	}
	c := newCanvas(m.width, m.height)
	m.corners(c, b)
	for _, place := range plugin.Places {
		if b.fits(place) && !m.off(place) {
			m.drawSide(c, b, place)
		}
	}
	c.dots(0, 0, b.left-1, b.top-1)
	c.dots(m.width, 0, b.right+2, b.top-1)
	c.dots(0, m.height-1, b.left-1, b.bottom+1)
	c.dots(m.width, m.height-1, b.right+2, b.bottom+1)

	out := make([]string, m.height)
	for y := range out {
		if y < b.top || y > b.bottom {
			out[y] = c.render(y, 0, m.width)
			continue
		}
		line := lines[y-b.top]
		if pad := bw - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out[y] = c.render(y, 0, b.left) + line + c.render(y, b.right+1, m.width)
	}
	return strings.Join(out, "\n"), true
}

func (m model) corners(c canvas, b box) {
	room := m.panel.Room
	x := c.put(2, 0, "mia", "bold", m.width)
	c.put(x+2, 0, room.Repo, "dim", m.width)

	now := clock()
	date := strings.ToLower(now.Format("Mon 2 Jan"))
	clockAt := m.width - 2 - runewidth.StringWidth(date+"  15:04")
	x = c.put(clockAt, 0, date, "dim", m.width)
	c.put(x+2, 0, now.Format("15:04"), "bold", m.width)

	var bl, br []plugin.Segment
	for _, corner := range room.Corners {
		segs := corner.Segments
		if corner.Problem != "" {
			segs = []plugin.Segment{{Text: corner.Problem, Style: "dim"}}
		}
		if corner.At == "bl" {
			if len(bl) > 0 {
				bl = append(bl, plugin.Segment{Text: "  "})
			}
			bl = append(bl, segs...)
		} else {
			if len(br) > 0 {
				br = append(br, plugin.Segment{Text: "  "})
			}
			br = append(br, segs...)
		}
	}
	if len(bl) > 0 {
		bl = append(bl, plugin.Segment{Text: "      "})
	}
	bl = append(bl, plugin.Segment{Text: "v sides", Style: "dim"})
	if m.legend {
		bl = m.legendSegments()
	}
	c.segments(2, m.height-1, bl, m.width)
	c.segments(m.width-2-segmentsWidth(br), m.height-1, br, m.width)
}

// drawSide draws one side's tab row and the active tab's lines in the block
// the room gives that side, cutting lines that do not fit.
func (m model) drawSide(c canvas, b box, place string) {
	tabs := m.tabsAt(place)
	if len(tabs) == 0 {
		return
	}
	active := m.activeAt(place, tabs)
	shown := tabs[active].Lines
	if tabs[active].Problem != "" {
		shown = []plugin.Line{{{Text: tabs[active].Problem, Style: "dim"}}}
	}
	center := b.left + (b.right-b.left+1)/2
	var x, wide, tabY, linesY, rows int
	align := "left"
	switch place {
	case "top":
		x, wide, align = center-topWide/2, topWide, "center"
		tabY, linesY = 3, 6
		rows = b.top - 1 - linesY
	case "bottom":
		x, wide, align = center-bottomWide/2, bottomWide, "center"
		linesY = b.bottom + 3
		rows = b.height - 5 - linesY
		tabY = linesY + min(len(shown), max(rows, 0)) + 1
	case "left":
		x, wide = 4, b.left-8
		tabY, linesY = b.top+2, b.top+4
		rows = b.bottom + 1 - linesY
	case "right":
		wide, align = b.width-b.right-10, "right"
		x = b.width - 5 - wide
		tabY, linesY = b.top+2, b.top+4
		rows = b.bottom + 1 - linesY
	}
	for i, line := range shown {
		if i >= rows {
			break
		}
		at := x
		if align == "right" {
			at = max(x+wide-lineWidth(line), x)
		}
		c.line(at, linesY+i, line, x+wide)
	}

	var row []plugin.Segment
	for i, tab := range tabs {
		if i > 0 {
			row = append(row, plugin.Segment{Text: "   "})
		}
		label, style := tab.Label, "dim"
		if tab.Count != "" && tab.Problem == "" {
			label += " " + tab.Count
		}
		if i == active {
			style = "bold underline"
		}
		row = append(row, plugin.Segment{Text: label, Style: style})
	}
	at := x
	switch align {
	case "center":
		at = x + (wide-segmentsWidth(row))/2
	case "right":
		at = x + wide - segmentsWidth(row)
	}
	c.segments(max(at, x), tabY, row, x+wide)
}

func (m model) roomed() bool {
	_, ok := m.room(m.boxed())
	return ok
}

// sideKeys turn plugin.Places on and off, in the same order.
var sideKeys = []string{"w", "i", "c", "a"}

// legendKey is the only place the sides take keys: w i c a turn a side on
// or off, [ ] switch the tabs of the side last turned, esc or v closes.
func (m model) legendKey(key string) model {
	switch key {
	case "esc", "v":
		m.legend = false
		return m
	case "[", "]":
		place := m.lastSide
		if place == "" {
			place = "top"
		}
		tabs := m.tabsAt(place)
		if len(tabs) == 0 {
			return m
		}
		step := 1
		if key == "[" {
			step = len(tabs) - 1
		}
		m.sides.Tabs = maps.Clone(m.sides.Tabs)
		if m.sides.Tabs == nil {
			m.sides.Tabs = map[string]string{}
		}
		m.sides.Tabs[place] = tabKey(tabs[(m.activeAt(place, tabs)+step)%len(tabs)])
	default:
		i := slices.Index(sideKeys, key)
		if i < 0 {
			return m
		}
		place := plugin.Places[i]
		m.lastSide = place
		if m.off(place) {
			m.sides.Off = slices.DeleteFunc(slices.Clone(m.sides.Off), func(p string) bool { return p == place })
		} else {
			m.sides.Off = append(slices.Clone(m.sides.Off), place)
		}
	}
	if m.hooks.Save != nil {
		m.hooks.Save(m.sides)
	}
	return m
}

func (m model) legendSegments() []plugin.Segment {
	segs := []plugin.Segment{{Text: "sides", Style: "dim"}, {Text: "   "}}
	for i, place := range plugin.Places {
		label := place
		if tabs := m.tabsAt(place); len(tabs) > 0 {
			label = tabs[m.activeAt(place, tabs)].Label
		}
		dot, style := plugin.Segment{Text: "●", Style: "green"}, ""
		if m.off(place) {
			dot, style = plugin.Segment{Text: "○", Style: "dim"}, "dim strike"
		}
		segs = append(segs, plugin.Segment{Text: sideKeys[i], Style: "bold"}, plugin.Segment{Text: " "}, dot,
			plugin.Segment{Text: " "}, plugin.Segment{Text: label, Style: style}, plugin.Segment{Text: "    "})
	}
	return append(segs, plugin.Segment{Text: "[ ]", Style: "bold"}, plugin.Segment{Text: " tabs", Style: "dim"},
		plugin.Segment{Text: "   "}, plugin.Segment{Text: "esc", Style: "bold"}, plugin.Segment{Text: " done", Style: "dim"})
}
