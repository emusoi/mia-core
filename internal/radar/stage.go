package radar

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The radar's look: a list of two-line rows on the left, the selected
// worktree's stage on the right — its signal, its windows as tabs, and the
// shown window as it is now, framed.

var (
	edge    = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	selBand = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	ghost   = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	line_   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	amber   = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
)

// machineColours gives each machine its own colour, this computer first.
var machineColours = []string{"75", "141", "79", "179", "211", "114"}

func (m model) colour(machine string) lipgloss.Style {
	if machine == "" || machine == "local" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(machineColours[0]))
	}
	i := 1
	for _, state := range m.data.Machines {
		if state.Name == "" || state.Name == "local" {
			continue
		}
		if state.Name == machine {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(machineColours[i%len(machineColours)]))
		}
		i++
	}
	if m.data.Away != nil && machine == m.data.Away.From {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(machineColours[0]))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(machineColours[len(machineColours)-1]))
}

type filter struct {
	label string
	keep  func(row) bool
}

// filters are what tab walks: everything, the signalled, this computer's,
// then each machine's.
func (m model) filters() []filter {
	fs := []filter{
		{"all", func(r row) bool { return !r.other }},
		{"signals", func(r row) bool { return len(r.signals) > 0 }},
		{"here", func(r row) bool { return r.machine == "" && !r.other }},
	}
	seen := map[string]bool{}
	for _, r := range m.worktrees() {
		if r.machine != "" && !seen[r.machine] {
			seen[r.machine] = true
			machine := r.machine
			fs = append(fs, filter{machine, func(r row) bool { return r.machine == machine }})
		}
	}
	return fs
}

// rank orders a filtered list: signals by their rank, then what moved last.
func rank(r row) int {
	if len(r.signals) > 0 {
		return r.signals[0].Rank
	}
	if r.listing != nil && !r.listing.LastWork.IsZero() {
		return 1000 + int(time.Since(r.listing.LastWork).Minutes())
	}
	return 1 << 30
}

func (m model) View() string {
	if m.mode == making {
		return m.formView()
	}
	full := max(m.width-1, 40)
	listW := full
	var stage []string
	room := max(m.height-6, 4)
	if full >= 90 {
		listW = min(max(full*42/100, 44), 64)
		stage = m.stage(full-listW-3, room)
	}
	var b strings.Builder
	prompt := edge.Render("› ")
	if m.query == "" {
		prompt += accent.Render("█") + ghost.Render("  find a worktree, a branch, an action")
	} else {
		prompt += ink.Render(m.query) + accent.Render("█")
	}
	b.WriteString(between(prompt, m.machinesLine(), full) + "\n\n")
	b.WriteString(m.tabsLine(full) + "\n\n")

	list := m.listLines(listW, room)
	for i := 0; i < room; i++ {
		left := ""
		if i < len(list) {
			left = list[i]
		}
		if stage != nil {
			right := ""
			if i < len(stage) {
				right = stage[i]
			}
			left = left + strings.Repeat(" ", max(listW-lipgloss.Width(left), 0)) + "   " + right
		}
		b.WriteString(left + "\n")
	}
	foot := keys("⏎", "switch") + keys("^n", "new") + keys("tab", "filter") + keys("←→", "windows") + keys("^x", "clear signal") + keys("^d", "remove") + keys("esc", "close")
	if m.confirm != "" {
		foot = Style("yellow").Render(m.confirm)
	}
	b.WriteString(cut(foot, full))
	return b.String()
}

func (m model) tabsLine(width int) string {
	var parts []string
	all := m.worktrees()
	for i, f := range m.filters() {
		n := 0
		for _, r := range all {
			if f.keep(r) {
				n++
			}
		}
		label := f.label + " " + fmt.Sprint(n)
		style := muted
		if i > 2 {
			style = m.colour(f.label)
		}
		if i == 1 && n > 0 {
			style = Style("red")
		}
		if i == m.filter {
			label = inkB.Underline(true).Render(f.label) + " " + soft.Render(fmt.Sprint(n))
		} else {
			label = style.Render(label)
		}
		parts = append(parts, label)
	}
	return cut(strings.Join(parts, "     "), width)
}

// listLines draws the rows, a worktree in two lines, keeping the cursor in
// view.
func (m model) listLines(width, room int) []string {
	var lines []string
	cursorAt := 0
	for i, r := range m.rows {
		on := i == m.cursor
		if r.kind == group && i > 0 {
			lines = append(lines, "")
		}
		if on {
			cursorAt = len(lines)
		}
		lines = append(lines, m.rowLines(r, on, width)...)
	}
	if len(m.rows) == 0 {
		lines = append(lines, ghost.Render("nothing called "+m.query+" — esc clears it"))
	}
	if len(lines) <= room {
		return lines
	}
	start := min(max(cursorAt-room/3, 0), len(lines)-room)
	return lines[start : start+room]
}

// rowLines is a worktree in one line — state, name, branch, where, its
// last day and how long since it moved — and a second when it is selected
// (what is going on in it) or signalled (what the signal says).
func (m model) rowLines(r row, on bool, width int) []string {
	mark := "  "
	if on {
		mark = edge.Render("▌ ")
	}
	paint := func(s string) string {
		s = cut(s, width)
		if on {
			return selBand.Render(s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0)))
		}
		return s
	}
	switch r.kind {
	case group:
		if strings.HasPrefix(r.label, "on ") {
			return []string{m.colour(strings.Fields(r.label)[1]).Italic(true).Render(r.label)}
		}
		return []string{italic.Render(r.label)}
	case action:
		return []string{paint(mark + ink.Render(r.label))}
	case layer:
		return []string{paint(mark + "    " + soft.Render(pad(r.label, 26)) + muted.Render(r.detail))}
	}
	name := strings.TrimPrefix(r.label, "▸ ")
	if len(r.layers) > 1 {
		name = "▸ " + name
		if m.open[r.name] {
			name = "▾ " + strings.TrimPrefix(r.label, "▸ ")
		}
	}
	nameStyle := ink
	if on {
		nameStyle = inkB
	}
	left := mark + m.glyph(r) + " " + nameStyle.Render(pad(name, 15)) + soft.Render(pad(r.branch, 18))
	var right []string
	switch {
	case r.machine != "":
		right = append(right, m.colour(r.machine).Render(r.machine))
	case r.name == m.data.Here:
		right = append(right, accent.Render("here"))
	case r.last:
		right = append(right, soft.Render("last"))
	}
	age := ""
	if r.listing != nil && !r.listing.LastWork.IsZero() {
		age = ago(time.Since(r.listing.LastWork))
	}
	right = append(right, muted.Render(fmt.Sprintf("%4s", age)))
	lines := []string{paint(between(left, strings.Join(right, "  "), width))}
	switch {
	case len(r.signals) > 0:
		s := r.signals[0]
		lines = append(lines, "      "+ghost.Render("└ ")+Style(s.Style).Bold(true).Render(s.Kind)+"  "+soft.Render(cut(s.Text, width-16)))
	}
	return lines
}

// glyph says the worktree's state before its name: a signal in its
// setter's colour, an open session, or nothing going on.
func (m model) glyph(r row) string {
	switch {
	case len(r.signals) > 0:
		return Style(r.signals[0].Style).Render("●")
	case r.listing != nil && r.listing.Session:
		return m.colour(r.machine).Render("◐")
	}
	return muted.Render("○")
}

// stage is the selected worktree in full: where it is, its signal in a
// band, its windows as tabs, and the shown one as it is now, framed.
func (m model) stage(width, height int) []string {
	r := m.selected()
	if r.kind != worktree && r.kind != layer {
		return nil
	}
	var out []string
	add := func(s string) { out = append(out, cut(s, width)) }
	name := strings.TrimPrefix(r.label, "▸ ")
	head := inkB.Render(name) + "  " + soft.Render(r.branch)
	if r.machine != "" {
		head += "  " + m.colour(r.machine).Render("◆ "+r.machine)
	} else {
		head += "  " + m.colour("").Render("◆ this computer")
	}
	add(head)
	if l := r.listing; l != nil && r.kind == worktree {
		var facts []string
		if l.Ahead > 0 {
			facts = append(facts, ink.Render(fmt.Sprintf("+%d", l.Ahead))+muted.Render(" ahead"))
		}
		if l.Dirty {
			facts = append(facts, amber.Render("✎ uncommitted"))
		}
		if !l.LastWork.IsZero() {
			facts = append(facts, muted.Render("last commit "+ago(time.Since(l.LastWork))))
		}
		if len(facts) > 0 {
			add(strings.Join(facts, muted.Render("  ·  ")))
		}
		if l.LastSubject != "" {
			add(muted.Render(l.LastSubject))
		}
	}
	add("")
	if r.kind == layer {
		for _, l := range r.layers {
			if l.Branch == r.layer {
				add(soft.Render(l.Branch) + muted.Render("  on "+l.Parent+"  ·  "+layerDetail(l)))
			}
		}
		add("")
		add(ghost.Render("⏎ moves the worktree to this layer, in place"))
		return out
	}
	for _, s := range r.signals {
		band := Style(s.Style).Bold(true).Render(s.Kind) + "  " + ink.Render(s.Text)
		band = between(band, muted.Render(s.From+" · "+ago(time.Since(s.At))), width-2)
		add(lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(" " + band + " "))
	}
	if len(r.signals) > 0 {
		add("")
	}
	screen := strings.TrimRight(m.screens[r.name], "\n ")
	if r.listing != nil && len(r.listing.Windows) > 0 {
		at, _ := m.windowAt(r)
		var tabs []string
		var under []string
		for i, w := range r.listing.Windows {
			label := fmt.Sprintf("%d %s", w.Index, w.Name)
			if i == at {
				tabs = append(tabs, inkB.Render(label))
				under = append(under, edge.Render(strings.Repeat("▔", lipgloss.Width(label))))
			} else {
				tabs = append(tabs, muted.Render(label))
				under = append(under, strings.Repeat(" ", lipgloss.Width(label)))
			}
		}
		add(strings.Join(tabs, "   "))
		add(strings.Join(under, "   "))
	}
	if screen == "" {
		if commits := m.commits[r.dir]; len(commits) > 0 {
			add(italic.Render("last commits"))
			for _, c := range commits {
				add(muted.Render(c.SHA+" ") + ink.Render(c.Subject) + muted.Render("  "+ago(time.Since(c.When))))
			}
		} else if r.listing == nil || !r.listing.Session {
			add(ghost.Render("no session yet — ⏎ starts one"))
		}
		return out
	}
	inner := width - 4
	room := max(height-len(out)-3, 3)
	lines := squeeze(strings.Split(screen, "\n"))
	if len(lines) > room {
		lines = lines[len(lines)-room:]
	}
	add(line_.Render("╭" + strings.Repeat("─", width-2) + "╮"))
	for _, l := range lines {
		text := ansi.Truncate(l, inner, "…") + "\x1b[0m"
		add(line_.Render("│ ") + text + strings.Repeat(" ", max(inner-lipgloss.Width(text), 0)) + line_.Render(" │"))
	}
	add(line_.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	live := Style("green").Render("live") + ghost.Render(" · as it is now")
	if r.listing != nil && len(r.listing.Windows) > 1 {
		live += ghost.Render(" · ←→ its other windows")
	}
	add(live)
	return out
}

// windowAt is which of the worktree's windows the stage shows: the one
// picked with ← →, else the active one.
func (m model) windowAt(r row) (int, bool) {
	if r.listing == nil || len(r.listing.Windows) == 0 {
		return 0, false
	}
	if i, ok := m.window[r.name]; ok && i < len(r.listing.Windows) {
		return i, true
	}
	for i, w := range r.listing.Windows {
		if w.Active {
			return i, true
		}
	}
	return 0, true
}

func (m model) shownWindow(r row) (struct{ Index int }, bool) {
	i, ok := m.windowAt(r)
	if !ok {
		return struct{ Index int }{}, false
	}
	return struct{ Index int }{r.listing.Windows[i].Index}, true
}

// squeeze keeps one blank line where a screen has several, so a window
// whose middle is empty shows its top and its bottom both.
func squeeze(lines []string) []string {
	var kept []string
	blank := false
	for _, l := range lines {
		empty := strings.TrimSpace(ansi.Strip(l)) == ""
		if empty && blank {
			continue
		}
		blank = empty
		kept = append(kept, l)
	}
	return kept
}

func keys(key, label string) string {
	return inkB.Render(key) + " " + muted.Render(label) + "    "
}

func (m model) machinesLine() string {
	var parts []string
	if len(m.data.Machines) > 1 {
		for _, state := range m.data.Machines {
			name := state.Name
			if name == "" {
				name = "local"
			}
			dot := m.colour(name).Render("●")
			if !state.Answers {
				dot = Style("red").Render("○")
			}
			parts = append(parts, dot+" "+soft.Render(name))
		}
	}
	if a := m.data.Away; a != nil {
		parts = append(parts, m.colour(a.From).Render("↩ "+a.From))
	}
	return strings.Join(parts, "   ")
}

func pad(s string, n int) string {
	if lipgloss.Width(s) >= n {
		return cut(s, n-1) + " "
	}
	return s + strings.Repeat(" ", n-lipgloss.Width(s))
}

// cut shortens s to n cells, styling and all.
func cut(s string, n int) string {
	return ansi.Truncate(s, n, "…")
}

func between(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return cut(left, max(width-lipgloss.Width(right)-2, 1)) + "  " + right
	}
	return left + strings.Repeat(" ", gap) + right
}
