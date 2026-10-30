package tui

import (
	"fmt"
	"maps"
	"sort"
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

const (
	homeWidest  = 124
	homeTallest = 44
)

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

// dashRoom is the dashboard's room: its own, or for a panel opened from the
// dashboard, the dashboard's under it; nil outside the dashboard.
func (m model) dashRoom() *panel.Room {
	if m.panel.Room != nil {
		return m.panel.Room
	}
	for i := len(m.stack) - 1; i >= 0; i-- {
		if r := m.stack[i].panel.Room; r != nil && !m.stack[i].classic {
			return r
		}
	}
	return nil
}

// homes says whether the dashboard draws as home now; help, reports, the
// composer and the details pane keep the box.
func (m model) homes() bool {
	if m.dashRoom() == nil || m.classic || m.inPopup || m.fill || m.hooks.Only != "" {
		return false
	}
	switch m.mode {
	case browsing, filtering, working, confirming, helping, reporting, typing:
		return true
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
	where := inkB.Render("mia") + "   " + muted.Render(m.dashRoom().Repo)
	if m.panel.Room == nil {
		// a panel opened from the dashboard: where it is, no dots, no views
		title, _, _ := strings.Cut(m.panel.Title, "  —  ")
		where += faint.Render("  ›  ") + ink.Render(title)
	}
	top = append(top, between(where, faint.Render(m.cornerText()), width))
	top = append(top, "")
	if m.panel.Room != nil {
		marks, label := m.marks(width, sel)
		top = append(top, marks, label, "")
	} else {
		top = append(top, "", "", "")
	}
	switch {
	case m.reading != nil:
		return m.page(top[:1], width)
	case m.mode == typing && m.pending.action.Lines:
		body := m.composer()
		if m.picking {
			body = m.picker()
		}
		return m.sheet(top[:1], "", body, 0, strings.TrimLeft(m.footer(), " "), false)
	case m.details && m.mode == browsing && m.cursor < len(m.rows):
		return m.sheet(top[:1], nameOf(m.rows[m.cursor]), m.paneOf(m.cursor, width, homeTallest-8), 0,
			homeKey("j k", "scroll")+"    "+homeKey(m.shown("tabprev", "tabnext"), "tabs")+"    "+homeKey("i", "close"), false)
	case m.mode == helping:
		return m.sheet(top[:1], "Keys", m.homeHelp(), m.scroll, homeKey("j k", "scroll")+"    "+homeKey("?", "close"), false)
	case m.mode == reporting:
		title := m.reportOf
		if title == "" {
			title = "mia"
		}
		return m.sheet(top[:1], title, m.report, 0, strings.TrimLeft(m.footer(), " "), false)
	}
	if m.viewing {
		top = append(top, m.viewHead(width)...)
	} else {
		top = append(top, m.headword(sel, width)...)
	}

	bottom := []string{}
	if m.problem != "" && m.mode != typing && m.mode != confirming {
		bottom = append(bottom, danger.Render(fit(m.problem, width)))
	}
	// the headword carries the row's keys; the foot the ones that work anywhere
	foot := strings.TrimLeft(m.footer(), " ")
	if m.mode == browsing && m.notice == "" {
		foot = homeKey("j k", "move") + "    " + homeKey(m.shown("filter"), "find") + "    " + homeKey("?", "keys")
		if len(m.stack) > 0 {
			foot += "    " + homeKey("esc", "back")
		}
		if m.viewing {
			foot = homeKey("j k", "move") + "    " + homeKey("⏎", "open") + "    " + homeKey("esc", "back")
		}
	}
	// two lines, both centred: where tab goes, then the keys
	bottom = append(bottom, "")
	if views := m.viewsText(); views != "" {
		bottom = append(bottom, centred(fit(views, width), width))
	}
	bottom = append(bottom, centred(foot, width))

	// home is a page of a fixed size, centred both ways: on a tall terminal
	// the keys sit under the page, not on the terminal's last row
	height := min(m.height, homeTallest)
	room := height - len(top) - len(bottom) - 2
	list := m.others(walk, width, room)
	if m.viewing {
		list = m.viewList(width, room)
	}
	lines := append(append(top, "", ""), list...)
	for len(lines) < height-len(bottom) {
		lines = append(lines, "")
	}
	lines = append(lines[:height-len(bottom)], bottom...)
	left := strings.Repeat(" ", max((m.width-width)/2, 0))
	for i, line := range lines {
		lines[i] = left + fit(line, width)
	}
	above := (m.height - height) / 2
	page := append(make([]string, above), lines...)
	for len(page) < m.height {
		page = append(page, "")
	}
	return strings.Join(page, "\n")
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
	if sel.index < 0 || m.loading() {
		// before the first panel arrives Empty is unset: the worktrees are
		// still being read, which a big repository takes seconds to do
		line := faint.Render(m.panel.Empty)
		if m.panel.Empty == "" || m.loading() {
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
	// the name column fits the longest name, between 24 and 40
	nameW := 24
	for _, h := range walk {
		if len(h.row.Cells) > 0 {
			nameW = max(nameW, min(lipgloss.Width(strings.TrimRight(h.row.Cells[0], " ")), 40))
		}
	}
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
		name := ink.Render(fmt.Sprintf("%-*s", nameW, fit(strings.TrimRight(cells[0], " "), nameW)))
		if h.index == m.cursor {
			mark, cursorAt = inkB.Render("›")+" ", len(lines)
			name = inkB.Render(fmt.Sprintf("%-*s", nameW, fit(strings.TrimRight(cells[0], " "), nameW)))
		}
		if h.row.Dim {
			name = faint.Render(fmt.Sprintf("%-*s", nameW, fit(cells[0], nameW)))
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

// page is what a popup's command printed, shown under home's header when
// there is no tmux: its title, its lines, the keys to read and close it.
func (m model) page(top []string, width int) string {
	keys := homeKey("j k", "read") + "    " + homeKey("q", "close")
	return m.sheet(top, m.reading.title, m.reading.lines, m.readAt, keys, true)
}

// sheet is a page of text under home's header — a popup's output, the
// keys, a report — its title, its lines wrapped from at, the keys under it.
func (m model) sheet(top []string, title string, body []string, at int, keys string, boldFirst bool) string {
	width := min(m.width-4, homeWidest)
	height := min(m.height, homeTallest)
	lines := append(append([]string{}, top...), "", muted.Bold(true).Render(title), "")
	room := height - len(lines) - 2
	at = min(at, max(len(body)-1, 0))
	for i, line := range body[at:] {
		style := ink
		if boldFirst && i == 0 && at == 0 {
			style = head
		}
		for _, part := range wrap(strings.ReplaceAll(line, "\t", "    "), width) {
			if len(lines) >= height-2 {
				break
			}
			if ansi := strings.Contains(part, "\x1b["); !ansi {
				part = style.Render(part)
			}
			lines = append(lines, part)
		}
	}
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	if len(body) > room {
		keys += "    " + faint.Render(fmt.Sprintf("%d–%d of %d", at+1, min(at+room, len(body)), len(body)))
	}
	lines = append(lines, centred(keys, width))
	left := strings.Repeat(" ", max((m.width-width)/2, 0))
	for i, line := range lines {
		lines[i] = left + fit(line, width)
	}
	page := append(make([]string, (m.height-height)/2), lines...)
	for len(page) < m.height {
		page = append(page, "")
	}
	return strings.Join(page, "\n")
}

// homeHelp is every key home answers to: moving, the selected worktree's,
// the ones that work anywhere, and what the dots say.
func (m model) homeHelp() []string {
	entry := func(key, what, more string) string {
		return inkB.Render(fmt.Sprintf("%-9s", key)) + ink.Render(fmt.Sprintf("%-34s", fit(what, 34))) + faint.Render(more)
	}
	group := func(name string) string { return muted.Italic(true).Render(name) }
	lines := []string{group("moving"),
		entry(m.shown("down", "up"), "the next, the previous worktree", m.shown("first", "last")+" the first and last"),
		entry(m.shown("filter"), "find a worktree", "by name or branch · esc clears"),
		entry("space", "the next that needs you", ""),
		entry("tab", "the next view", "round the worktrees and the plugins' views · shift+tab back"),
		entry("⏎", "unfold a stack, open a view's line", ""),
		entry(m.shown("back"), "back, or quit", ""),
	}
	for _, section := range m.panel.Sections {
		if section.ToggleKey != "" {
			lines = append(lines, entry(section.ToggleKey, "show or hide "+section.Label, ""))
		}
	}
	var row panel.Row
	if m.cursor < len(m.rows) {
		row = m.rows[m.cursor]
	}
	type item struct {
		rank int
		line string
	}
	var onRow, anywhere []item
	for id, action := range m.panel.Actions {
		if action.Key == "" {
			continue
		}
		it := item{m.panel.Rank(id), entry(action.Key, action.Label, action.CLI())}
		switch {
		case !action.NeedsRow():
			anywhere = append(anywhere, it)
		case offers(row, id):
			onRow = append(onRow, it)
		}
	}
	add := func(title string, items []item) {
		if len(items) == 0 {
			return
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].rank < items[j].rank })
		lines = append(lines, "", title)
		for _, it := range items {
			lines = append(lines, it.line)
		}
	}
	add(group("on ")+inkB.Render(nameOf(row)), onRow)
	add(group("anywhere"), anywhere)
	return append(lines, "", group("the dots"),
		inkB.Render("●")+muted.Render(" needs you    ")+ink.Render("•")+muted.Render(" an agent works    ")+
			muted.Render("◦ in progress    ")+faint.Render("· rests"),
		faint.Render("one dot per worktree, in the list's order; ▔ marks the one selected"))
}

// ring moves one stop round the worktrees and the plugins' views.
func (m model) ring(step int) model {
	at, n := -1, len(m.panel.Room.Sides)
	if m.viewing {
		at = m.view
	}
	at += step
	if at < -1 {
		at = n - 1
	}
	if at == -1 || at == n {
		m.viewing, m.sideSel = false, ""
		return m
	}
	return m.showView(at)
}

// viewHead is the shown view's summary where the entry sits on home, the
// same height, so tab between them moves nothing under it.
func (m model) viewHead(width int) []string {
	tab := m.panel.Room.Sides[min(m.view, len(m.panel.Room.Sides)-1)]
	var lines []string
	switch {
	case tab.Problem != "":
		lines = []string{danger.Render(fit(tab.Plugin+": "+tab.Problem, width))}
	case tab.Head != "":
		lines = []string{head.Render(fit(tab.Head, width))}
	default:
		lines = []string{muted.Bold(true).Render(tab.Label)}
	}
	if tab.Note != "" {
		lines = append(lines, faint.Render(fit(tab.Note, width)))
	}
	return pad(lines, headwordRows)
}

// viewList is the shown view's lines: section labels, lines of cells — the
// growing one takes what the others leave — meters and charts.
func (m model) viewList(width, room int) []string {
	tab := m.panel.Room.Sides[min(m.view, len(m.panel.Room.Sides)-1)]
	var lines []string
	cursorAt := 0
	if len(tab.Lines) == 0 && tab.Problem == "" {
		lines = append(lines, faint.Render("Nothing here."))
	}
	for i, item := range tab.Lines {
		switch {
		case item.Label != "":
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, muted.Italic(true).Render(item.Label)+"  "+faint.Render(viewCells(item.Cells, width, false)))
			continue
		case len(item.Chart) > 0:
			lines = append(lines, chart(item.Chart)...)
			continue
		}
		mark := "  "
		if i == m.sideRow && item.Open != nil {
			mark, cursorAt = inkB.Render("›")+" ", len(lines)
		}
		lines = append(lines, fit(mark+viewCells(item.Cells, width-2, true), width))
	}
	if room <= 0 || len(lines) <= room {
		return lines
	}
	start := min(max(cursorAt-room+2, 0), len(lines)-room)
	return lines[start : start+room]
}

// viewCells draws a line's cells: each at its width, a meter as a bar, and
// (when grow) the cell marked to grow taking the width the others leave.
func viewCells(cells plugin.Line, width int, grow bool) string {
	used := 0
	for _, c := range cells {
		if !c.Grow || !grow {
			used += max(c.Width, lipgloss.Width(c.Text)) + 1
		}
	}
	var b strings.Builder
	for _, c := range cells {
		w := c.Width
		if c.Grow && grow {
			w = max(width-used-1, c.Width, 1)
		}
		if c.Meter != nil {
			if w <= 0 || w > 40 {
				w = 40 // a bar longer than this says no more
			}
			n := c.Meter.Filled(w)
			fill := ink
			if strings.Contains(c.Style, "red") {
				fill = danger
			}
			b.WriteString(fill.Render(strings.Repeat("━", n)) + dotS.Render(strings.Repeat("━", w-n)) + "   ")
			continue
		}
		text := c.Text
		if w > 0 {
			if c.Align == "right" {
				text = fmt.Sprintf("%*s", w, fit(text, w))
			} else {
				text = fmt.Sprintf("%-*s", w, fit(text, w))
			}
		}
		b.WriteString(viewStyle(c.Style).Render(text) + " ")
	}
	return b.String()
}

// chart is a few values as bars six rows high, their labels under them;
// the one marked on is in ink.
func chart(bars []plugin.Bar) []string {
	const tall = 6
	top := 0.0
	for _, b := range bars {
		top = max(top, b.Value)
	}
	rows := make([]string, tall+1)
	for _, b := range bars {
		h := 0
		if top > 0 {
			h = max(int(b.Value/top*tall+0.5), 1)
		}
		style := muted
		if b.On {
			style = ink
		}
		for r := 0; r < tall; r++ {
			cell := "    "
			if tall-r <= h {
				cell = "████"
			}
			rows[r] += style.Render(cell) + "    "
		}
		label := style.Render(fmt.Sprintf("%-8s", fit(b.Label, 7)))
		if !b.On {
			label = faint.Render(fmt.Sprintf("%-8s", fit(b.Label, 7)))
		}
		rows[tall] += label
	}
	return rows
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
	for _, corner := range m.dashRoom().Corners {
		if corner.Problem != "" {
			continue
		}
		parts = append(parts, plainSegments(corner.Segments))
	}
	return strings.Join(append(parts, clock().Format("15:04")), "   ·   ")
}

// viewsText is the ring tab walks: the worktrees and every plugin's view,
// with its count, the one showing in ink.
func (m model) viewsText() string {
	if m.panel.Room == nil || len(m.panel.Room.Sides) == 0 {
		return ""
	}
	stop := func(label, count string, on bool) string {
		if count != "" {
			label += " " + count
		}
		if on {
			return inkB.Render(label)
		}
		return faint.Render(label)
	}
	names := []string{stop("worktrees", "", !m.viewing)}
	for i, tab := range m.panel.Room.Sides {
		names = append(names, stop(tab.Label, tab.Count, m.viewing && i == m.view))
	}
	return inkB.Render("tab") + "  " + strings.Join(names, faint.Render(" · "))
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

// centred puts line in the middle of width.
func centred(line string, width int) string {
	return strings.Repeat(" ", max((width-lipgloss.Width(line))/2, 0)) + line
}

// between puts left at the start of width and right at its end.
func between(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return fit(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}
