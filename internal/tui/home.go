package tui

import (
	"fmt"
	"maps"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/plugin"
)

// The dashboard in mia's own design (design/dash.buni, pages 31–41): warm
// greys, colour only for danger, the worktrees drawn as the mark's dots, the
// selected one set as a dictionary headword and the rest listed under it.
// Text never goes below the faint grey; the dot grey is for dots and rules.
var (
	inkC    = lipgloss.AdaptiveColor{Light: "#1b1b1b", Dark: "#e8e7e3"}
	mutedC  = lipgloss.AdaptiveColor{Light: "#55544f", Dark: "#a3a29d"}
	faintC  = lipgloss.AdaptiveColor{Light: "#6b6a65", Dark: "#8a8984"}
	dotC    = lipgloss.AdaptiveColor{Light: "#c9c8c3", Dark: "#4a4946"}
	ruleC   = lipgloss.AdaptiveColor{Light: "#e2e1dc", Dark: "#2c2c2a"}
	dangerC = lipgloss.AdaptiveColor{Light: "#b5443a", Dark: "#e79891"}
	hiC     = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}

	ink    = lipgloss.NewStyle().Foreground(inkC)
	inkB   = ink.Bold(true)
	muted  = lipgloss.NewStyle().Foreground(mutedC)
	faint  = lipgloss.NewStyle().Foreground(faintC)
	dotS   = lipgloss.NewStyle().Foreground(dotC)
	ruleS  = lipgloss.NewStyle().Foreground(ruleC)
	danger = lipgloss.NewStyle().Foreground(dangerC)
	head   = lipgloss.NewStyle().Foreground(hiC).Bold(true)
)

const homeWidest = 124

// shape says how a section's worktrees look as dots: big needs you, small
// bright is working, a ring is in progress, faint rests.
func shape(section string) (string, lipgloss.Style) {
	switch section {
	case "needs", "waiting":
		return "●", inkB
	case "working", "finished":
		return "•", ink
	case "starred", "active":
		return "◦", muted
	}
	return "·", dotS
}

// homes says whether the dashboard draws as home now; help, reports, the
// composer and the details pane keep the box.
func (m model) homes() bool {
	if m.panel.Room == nil || m.classic || m.inPopup || m.fill || m.details || m.hooks.Only != "" || m.loading() {
		return false
	}
	switch m.mode {
	case browsing, filtering, working, confirming:
		return true
	case typing:
		return !m.pending.action.Lines
	}
	return false
}

type homeRow struct {
	row     panel.Row
	section panel.Section
	index   int // in m.rows, or -1 for a folded section's line
}

// walk is the sections as home lists them: each row with its section, in
// m.rows' order, and one line for a folded section.
func (m model) walk() []homeRow {
	var out []homeRow
	index := 0
	for _, section := range m.panel.Sections {
		rows := m.rowsOf(section)
		if m.query != "" && len(rows) == 0 {
			continue
		}
		if section.Collapsed && !m.expanded[section.ID] && m.query == "" {
			out = append(out, homeRow{section: section, index: -1})
			continue
		}
		for _, row := range rows {
			out = append(out, homeRow{row: row, section: section, index: index})
			index++
		}
	}
	return out
}

func (m model) home() string {
	width := min(m.width-4, homeWidest)
	walk := m.walk()
	sel := homeRow{index: -1}
	for _, h := range walk {
		if h.index == m.cursor {
			sel = h
		}
	}

	var top []string
	top = append(top, between(inkB.Render("mia")+"   "+muted.Render(m.panel.Room.Repo), faint.Render(m.cornerText()), width))
	top = append(top, "")
	marks, label := m.marks(width, sel)
	top = append(top, marks, label, "")
	top = append(top, m.headword(sel, width)...)

	bottom := []string{}
	if m.problem != "" && m.mode != typing && m.mode != confirming {
		bottom = append(bottom, danger.Render(fit(m.problem, width)))
	}
	// the headword carries the row's keys; the foot the ones that work anywhere
	foot := strings.TrimLeft(m.footer(), " ")
	if m.mode == browsing && m.problem == "" && m.notice == "" {
		foot = homeKey("j k", "move") + "    " + homeKey(m.shown("filter"), "find") + "    " + homeKey("?", "keys")
		if m.viewing {
			foot = homeKey("tab", "next view") + "    " + homeKey("j k", "move") + "    " + homeKey("⏎", "open") + "    " + homeKey("v", "back")
		}
	}
	views := m.viewsText()
	if m.viewing {
		views = ""
	}
	bottom = append(bottom, "", between(foot, views, width))

	room := m.height - len(top) - len(bottom) - 2
	list := m.others(walk, width, room)
	if m.viewing {
		list = m.viewList(width, room)
	}
	lines := append(append(top, "", ""), list...)
	for len(lines) < m.height-len(bottom) {
		lines = append(lines, "")
	}
	lines = append(lines[:m.height-len(bottom)], bottom...)
	left := strings.Repeat(" ", max((m.width-width)/2, 0))
	for i, line := range lines {
		lines[i] = left + fit(line, width)
	}
	return strings.Join(lines, "\n")
}

// marks is one dot per worktree, in the list's order, the selected one
// underlined and named on the line below; the counts sit after the dots.
func (m model) marks(width int, sel homeRow) (string, string) {
	var counts []string
	for _, section := range m.panel.Sections {
		n := fmt.Sprint(len(section.Rows))
		if section.ID == "needs" || section.ID == "waiting" {
			n = inkB.Render(n)
		} else {
			n = ink.Render(n)
		}
		counts = append(counts, n+" "+faint.Render(section.Label))
	}
	countText := strings.Join(counts, "   ")
	room := max((width-lipgloss.Width(countText)-6)/2, 4)

	var dots strings.Builder
	under, label := -1, ""
	i := 0
	for _, section := range m.panel.Sections {
		glyph, style := shape(section.ID)
		for _, row := range section.Rows {
			if i >= room {
				break
			}
			if sel.index >= 0 && (row.ID == sel.row.ID || hasChild(row, sel.row.ID)) {
				under, label = i, nameOf(row)
			}
			dots.WriteString(style.Render(glyph) + " ")
			i++
		}
	}
	total := 0
	for _, section := range m.panel.Sections {
		total += len(section.Rows)
	}
	line := dots.String()
	if total > i {
		line += faint.Render(fmt.Sprintf("+%d", total-i))
	}
	line = between(line, countText, width)
	if under < 0 {
		return line, ""
	}
	return line, strings.Repeat(" ", under*2) + faint.Render("▔ "+label)
}

func hasChild(row panel.Row, id string) bool {
	for _, child := range row.Children {
		if child.ID == id || hasChild(child, id) {
			return true
		}
	}
	return false
}

// The headword always takes the same rows — two for the title, five for
// facts — so the list under it stays put while j and k move the entry.
const (
	titleRows    = 2
	factRows     = 5
	headwordRows = 1 + 1 + titleRows + 1 + factRows + 1 + 1
)

// headword is the selected worktree set like a dictionary entry: its name
// and state, its task, its facts, and the keys that act on it.
func (m model) headword(sel homeRow, width int) []string {
	if sel.index < 0 {
		// before the first panel arrives Empty is unset: the worktrees are
		// still being read, which a big repository takes seconds to do
		line := faint.Render(m.panel.Empty)
		if m.panel.Empty == "" {
			line = muted.Render("Reading the worktrees…")
		}
		return pad([]string{line}, headwordRows)
	}
	row := sel.row
	cells := append(append([]string{}, row.Cells...), "", "", "", "")
	state := muted.Italic(true)
	if sel.section.ID == "needs" || sel.section.ID == "waiting" {
		state = ink.Italic(true)
	}
	left := muted.Bold(true).Render(nameOf(row))
	if s := strings.TrimSpace(cells[2]); s != "" {
		left += "   " + state.Render(s)
	}
	if a := strings.TrimSpace(cells[3]); a != "" {
		left += "   " + faint.Render(a)
	}
	right := ""
	if n := len(sel.section.Rows); n > 1 && (sel.section.ID == "needs" || sel.section.ID == "waiting") {
		for i, r := range sel.section.Rows {
			if r.ID == row.ID || hasChild(r, row.ID) {
				right = faint.Render(fmt.Sprintf("%d of %d", i+1, n))
			}
		}
	}
	lines := []string{between(left, right, width), ""}
	title := row.Title
	if title == "" {
		title = strings.TrimSpace(cells[1])
	}
	parts := wrap(title, width)
	if len(parts) > titleRows {
		parts = append(parts[:titleRows-1], fit(strings.Join(parts[titleRows-1:], " "), width))
	}
	var titled []string
	for _, part := range parts {
		titled = append(titled, head.Render(part))
	}
	lines = append(append(lines, pad(titled, titleRows)...), "")
	var facts []string
	for _, fact := range row.Facts {
		if len(facts) == factRows {
			break
		}
		key, value, ok := strings.Cut(fact, "  ")
		if !ok {
			key, value = "", fact
		}
		style := muted
		if strings.HasPrefix(key, "plugin") || strings.HasPrefix(key, "restack") {
			style = danger
		}
		facts = append(facts, fit(faint.Render(fmt.Sprintf("%-10s", strings.TrimSpace(key)))+style.Render(strings.TrimSpace(value)), width))
	}
	lines = append(append(lines, pad(facts, factRows)...), "")
	return append(lines, strings.Join(m.hints(), "    "))
}

func pad(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}

// others is every worktree under its section's label, the selected one
// marked in the margin, scrolled to keep it in view.
func (m model) others(walk []homeRow, width, room int) []string {
	var lines []string
	cursorAt := 0
	last := ""
	for _, h := range walk {
		if h.section.ID != last {
			if last != "" {
				lines = append(lines, "")
			}
			last = h.section.ID
			label := muted.Italic(true).Render(h.section.Label) + "  " + faint.Render(fmt.Sprint(len(h.section.Rows)))
			if h.index < 0 {
				key := h.section.ToggleKey
				if key == "" {
					key = "e"
				}
				label += "    " + homeKey(key, "show")
			}
			lines = append(lines, label)
			if h.index < 0 {
				continue
			}
		}
		cells := append(append([]string{}, h.row.Cells...), "", "", "")
		mark := "  "
		name := ink.Render(fmt.Sprintf("%-24s", fit(strings.TrimRight(cells[0], " "), 24)))
		if h.index == m.cursor {
			mark, cursorAt = inkB.Render("›")+" ", len(lines)
			name = inkB.Render(fmt.Sprintf("%-24s", fit(strings.TrimRight(cells[0], " "), 24)))
		}
		if h.row.Dim {
			name = faint.Render(fmt.Sprintf("%-24s", fit(cells[0], 24)))
		}
		state := muted.Italic(true)
		if h.section.ID == "needs" || h.section.ID == "waiting" {
			state = ink.Italic(true)
		}
		title := h.row.Title
		if title == "" {
			title = cells[1]
		}
		lines = append(lines, fit(mark+name+"  "+state.Render(fmt.Sprintf("%-16s", fit(cells[2], 16)))+"  "+faint.Render(title), width))
	}
	if m.query != "" && len(m.rows) == 0 {
		lines = append(lines, faint.Render("nothing matches "+m.query+" — esc clears the filter"))
	}
	if room <= 0 || len(lines) <= room {
		return lines
	}
	// scroll only as far as keeps the selection in view, so the list stays
	// still while the selection moves inside it
	start := min(max(cursorAt-room+2, 0), len(lines)-room)
	return lines[start : start+room]
}

// showView shows the room's i-th side in place of the list, through the
// room's own side state, so ⏎ and j k work on it as they do in the room.
func (m model) showView(i int) model {
	sides := m.panel.Room.Sides
	if len(sides) == 0 {
		return m
	}
	i = (i%len(sides) + len(sides)) % len(sides)
	tab := sides[i]
	m.sides.Tabs = maps.Clone(m.sides.Tabs)
	if m.sides.Tabs == nil {
		m.sides.Tabs = map[string]string{}
	}
	m.sides.Tabs[tab.Where] = tabKey(tab)
	m.viewing, m.view, m.sideSel = true, i, tab.Where
	m.sideRow = m.nextItem(-1, 1)
	return m
}

// viewList is the shown side: every view's name along the top, the shown
// one in ink, and its lines under them.
func (m model) viewList(width, room int) []string {
	sides := m.panel.Room.Sides
	var names []string
	for i, tab := range sides {
		name := faint.Render(tab.Label)
		if i == m.view {
			name = inkB.Render(tab.Label)
		}
		if tab.Count != "" {
			name += " " + faint.Render(tab.Count)
		}
		names = append(names, name)
	}
	lines := []string{strings.Join(names, faint.Render("   ·   ")), ""}
	tab := sides[min(m.view, len(sides)-1)]
	cursorAt := 0
	switch {
	case tab.Problem != "":
		lines = append(lines, danger.Render(fit(tab.Plugin+": "+tab.Problem, width)))
	case len(tab.Lines) == 0:
		lines = append(lines, faint.Render("Nothing here."))
	}
	for i, item := range tab.Lines {
		mark := "  "
		if i == m.sideRow && item.Open != nil {
			mark, cursorAt = inkB.Render("›")+" ", len(lines)
		}
		var b strings.Builder
		for _, c := range item.Cells {
			text := c.Text
			if c.Width > 0 {
				if c.Align == "right" {
					text = fmt.Sprintf("%*s", c.Width, fit(text, c.Width))
				} else {
					text = fmt.Sprintf("%-*s", c.Width, fit(text, c.Width))
				}
			}
			b.WriteString(viewStyle(c.Style).Render(text) + " ")
		}
		lines = append(lines, fit(mark+b.String(), width))
	}
	if room <= 0 || len(lines) <= room {
		return lines
	}
	start := min(max(cursorAt-room+2, 0), len(lines)-room)
	return lines[start : start+room]
}

// viewStyle is a plugin's cell style in home's palette: red stays danger,
// bold is ink, dim is faint, every other colour is plain ink.
func viewStyle(spec string) lipgloss.Style {
	style := ink
	for _, word := range strings.Fields(spec) {
		switch word {
		case "red":
			style = danger
		case "dim", "gray":
			style = faint
		case "bold":
			style = style.Bold(true)
		}
	}
	return style
}

// cornerText is the plugins' corners and the time, for the top line.
func (m model) cornerText() string {
	var parts []string
	for _, corner := range m.panel.Room.Corners {
		if corner.Problem != "" {
			continue
		}
		parts = append(parts, plainSegments(corner.Segments))
	}
	return strings.Join(append(parts, clock().Format("15:04")), "   ·   ")
}

// viewsText names the plugins' sides; v opens them as the room draws them.
func (m model) viewsText() string {
	var names []string
	for _, tab := range m.panel.Room.Sides {
		names = append(names, tab.Label)
	}
	if len(names) == 0 {
		return ""
	}
	return homeKey("v", strings.Join(names, " · "))
}

// homeKey is a key and what it does: the key in ink, the words faint.
func homeKey(key, label string) string {
	return inkB.Render(key) + " " + faint.Render(label)
}

func plainSegments(segs []plugin.Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

// between puts left at the start of width and right at its end.
func between(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return fit(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}
