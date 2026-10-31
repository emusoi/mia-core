// Package radar is mia inside tmux: one popup that shows what there is and
// finds anything by typing, and the popup that makes a new worktree. It
// knows worktrees, stacks, machines and signals; nothing about agents.
package radar

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/signal"
	"github.com/emusoi/mia-core/internal/stack"
)

// Data is what the radar shows, gathered before it opens.
type Data struct {
	Repo     string
	Root     string // the main checkout
	MiaDir   string
	Here     string // the worktree the popup was opened over, or ""
	HereDir  string
	Branch   string // its branch
	Base     string
	Prefix   string
	Local    []app.Listing
	Machines []app.MachineState
	// First is set the first time mia makes a worktree in this repository:
	// what it guessed, shown once, saved when the worktree is made.
	First *Guess
}

type Guess struct {
	Base  string
	Setup []string
}

// Choice is what the person picked: argv for mia, run once the popup has
// closed, and where it runs.
type Choice struct {
	Argv   []string
	Dir    string
	Remote string // name@machine: switch by opening its session in a window
	Name   string // a local worktree to switch to
	Config *Guess // write this config first
	Stream bool   // show its output while it runs (making a worktree)
}

type kind int

const (
	group kind = iota
	worktree
	layer
	action
)

type row struct {
	kind    kind
	label   string
	branch  string
	detail  string
	signals []signal.Signal
	name    string // name, or name@machine
	machine string
	dir     string
	layers  []stack.Layer
	layer   string
	act     func(m model) (model, tea.Cmd)
}

var (
	ink    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	inkB   = ink.Bold(true)
	muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	faint  = lipgloss.NewStyle().Faint(true)
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	sel    = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	italic = muted.Italic(true)
)

// Style is a setter's style for a signal, as lipgloss draws it.
func Style(spec string) lipgloss.Style {
	s := ink
	for _, word := range strings.Fields(spec) {
		switch word {
		case "bold":
			s = s.Bold(true)
		case "dim":
			s = s.Faint(true)
		case "italic", "italics":
			s = s.Italic(true)
		case "underline":
			s = s.Underline(true)
		default:
			if c, ok := colours[word]; ok {
				s = s.Foreground(lipgloss.Color(c))
			}
		}
	}
	return s
}

var colours = map[string]string{"black": "0", "red": "1", "green": "2", "yellow": "3", "blue": "4", "magenta": "5", "cyan": "6", "white": "7"}

type mode int

const (
	browsing mode = iota
	making
)

type model struct {
	data    Data
	mode    mode
	query   string
	cursor  int
	open    map[string]bool // stacks opened into their layers
	rows    []row
	width   int
	height  int
	chosen  *Choice
	confirm string // a question waiting on y
	pending func(m model) (model, tea.Cmd)
	form    form
}

// Run opens the radar, or the new-worktree popup when newOnly, and returns
// what was picked, nil when nothing was.
func Run(data Data, newOnly bool) (*Choice, error) {
	m := model{data: data, open: map[string]bool{}, form: newForm(data)}
	if newOnly {
		m.mode = making
	}
	m.rows = m.build()
	m.cursor = m.next(0)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	return final.(model).chosen, nil
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tick, finished:
		return m.follow(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == making {
			return m.formKey(msg)
		}
		return m.browseKey(msg)
	}
	return m, nil
}

func (m model) browseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != "" {
		if msg.String() == "y" {
			m.confirm = ""
			return m.pending(m)
		}
		m.confirm = ""
		return m, nil
	}
	switch msg.String() {
	case "esc":
		if m.query != "" {
			m.query, m.cursor = "", 0
			m.rows = m.build()
			return m, nil
		}
		return m, tea.Quit
	case "up", "ctrl+p":
		m.cursor = m.next(-1)
	case "down", "ctrl+n":
		m.cursor = m.next(1)
	case "right":
		if r := m.selected(); r.kind == worktree && len(r.layers) > 1 {
			m.open[r.name] = true
			m.rows = m.build()
		}
	case "left":
		if r := m.selected(); r.kind == worktree || r.kind == layer {
			delete(m.open, r.name)
			m.rows = m.build()
			m.cursor = m.next(0)
		}
	case "ctrl+x":
		if r := m.selected(); r.kind == worktree && len(r.signals) > 0 {
			return clearSignals(r)(m)
		}
	case "enter":
		r := m.selected()
		switch r.kind {
		case worktree:
			return switchTo(r)(m)
		case layer:
			return goLayer(r)(m)
		case action:
			return r.act(m)
		}
	case "backspace":
		if m.query != "" {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.rows, m.cursor = m.build(), 0
			m.cursor = m.next(0)
		}
	default:
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
			m.query += string(msg.Runes)
			if msg.Type == tea.KeySpace {
				m.query += " "
			}
			m.rows, m.cursor = m.build(), 0
			m.cursor = m.next(0)
		}
	}
	return m, nil
}

func (m model) selected() row {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor]
	}
	return row{kind: group}
}

// next is the nearest row that can be picked, step away from the cursor
// (0: the cursor's own, or the next below it).
func (m model) next(step int) int {
	i := m.cursor + step
	dir := step
	if dir == 0 {
		dir = 1
	}
	for i >= 0 && i < len(m.rows) {
		if m.rows[i].kind != group {
			return i
		}
		i += dir
	}
	if step == 0 {
		return m.cursor
	}
	return m.cursor
}

// build is the rows: with nothing typed, worktrees with signals by rank,
// then in progress, then a count of the resting; with something typed, the
// worktrees it finds and what can be done with the first.
func (m model) build() []row {
	all := m.worktrees()
	if q := strings.ToLower(strings.TrimSpace(m.query)); q != "" {
		var found []row
		for _, r := range all {
			if strings.Contains(strings.ToLower(r.label+" "+r.branch), q) {
				found = append(found, r)
			}
		}
		var rows []row
		if len(found) > 0 {
			rows = append(rows, row{kind: group, label: "worktrees"})
			rows = append(rows, found...)
			rows = append(rows, row{kind: group, label: "do with " + found[0].label})
			rows = append(rows, actionsFor(found[0])...)
		}
		if strings.HasPrefix("new worktree", q) || strings.HasPrefix(q, "new") {
			rows = append(rows, row{kind: group, label: "everything else"},
				row{kind: action, label: "new worktree", act: func(m model) (model, tea.Cmd) { m.mode = making; return m, nil }})
		}
		return rows
	}
	var signalled, progress []row
	resting := 0
	for _, r := range all {
		switch {
		case len(r.signals) > 0:
			signalled = append(signalled, r)
		case r.detail != "" || len(r.layers) > 1 || r.name == m.data.Here:
			progress = append(progress, r)
		default:
			resting++
		}
	}
	sort.SliceStable(signalled, func(i, j int) bool { return signalled[i].signals[0].Rank < signalled[j].signals[0].Rank })
	var rows []row
	if len(signalled) > 0 {
		rows = append(rows, row{kind: group, label: "signals"})
		rows = m.withLayers(rows, signalled)
	}
	if len(progress) > 0 {
		rows = append(rows, row{kind: group, label: fmt.Sprintf("in progress  %d", len(progress))})
		rows = m.withLayers(rows, progress)
	}
	if resting > 0 {
		rows = append(rows, row{kind: group, label: fmt.Sprintf("resting  %d  — type to find one", resting)})
	}
	return rows
}

// withLayers adds each worktree's row, and under an opened stack its
// layers, top first.
func (m model) withLayers(rows, worktrees []row) []row {
	for _, r := range worktrees {
		rows = append(rows, r)
		if !m.open[r.name] {
			continue
		}
		for i := len(r.layers) - 1; i >= 0; i-- {
			l := r.layers[i]
			rows = append(rows, row{kind: layer, label: fmt.Sprintf("%d  %s", i+1, l.Branch), name: r.name, dir: r.dir, layer: l.Branch, layers: r.layers,
				detail: layerDetail(l)})
		}
	}
	return rows
}

func layerDetail(l stack.Layer) string {
	d := fmt.Sprintf("+%d", l.Ahead)
	if l.Landed {
		d = "landed"
	}
	if l.NeedsRestack {
		d += "  ↓ behind " + l.Parent
	}
	if l.Current {
		d += "  · you are here"
	}
	return d
}

// worktrees is every worktree as a row: this computer's, then each
// machine's; a stack is one row, its layers kept for opening.
func (m model) worktrees() []row {
	var rows []row
	seenStack := map[string]bool{}
	for _, l := range m.data.Local {
		if l.Main && !l.Adopted {
			continue
		}
		r := row{kind: worktree, label: l.Name, branch: l.Branch, name: l.Name, dir: l.Path, signals: l.Signals}
		if len(l.Layers) > 1 {
			key := l.Layers[0].Branch
			if seenStack[key] {
				continue
			}
			seenStack[key] = true
			r.layers = l.Layers
			r.branch = fmt.Sprintf("%s · %d layers", l.Branch, len(l.Layers))
		}
		r.detail = listingDetail(l)
		rows = append(rows, r)
	}
	for _, state := range m.data.Machines {
		for _, l := range state.Worktrees {
			rows = append(rows, row{kind: worktree, label: l.Name, branch: l.Branch, name: l.Name + "@" + state.Name,
				machine: state.Name, signals: l.Signals, detail: listingDetail(l)})
		}
	}
	return rows
}

func listingDetail(l app.Listing) string {
	var parts []string
	if l.Files > 0 || l.Dirty {
		parts = append(parts, "uncommitted")
	}
	if l.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("+%d", l.Ahead))
	}
	for _, layer := range l.Layers {
		if layer.NeedsRestack {
			parts = append(parts, "⟳ restack due")
			break
		}
	}
	return strings.Join(parts, " · ")
}

func actionsFor(r row) []row {
	acts := []row{{kind: action, label: "switch there", act: switchTo(r)}}
	if r.machine == "" {
		acts = append(acts,
			row{kind: action, label: "open in an editor", act: run(r, "open", r.name)},
			row{kind: action, label: "its environment: up", act: run(r, "env", "up", r.name)})
		if len(r.layers) > 1 {
			acts = append(acts, row{kind: action, label: "restack", act: run(r, "stack", "--in", r.name, "restack")})
		}
	}
	if len(r.signals) > 0 && r.machine == "" {
		acts = append(acts, row{kind: action, label: "clear its signals", act: clearSignals(r)})
	}
	if r.machine == "" {
		acts = append(acts, row{kind: action, label: "remove it", act: func(m model) (model, tea.Cmd) {
			m.confirm = "remove " + r.name + "? its branch stays. y removes it"
			m.pending = run(r, "rm", r.name)
			return m, nil
		}})
	}
	return acts
}

func switchTo(r row) func(m model) (model, tea.Cmd) {
	return func(m model) (model, tea.Cmd) {
		if r.machine != "" {
			m.chosen = &Choice{Remote: r.name}
		} else {
			m.chosen = &Choice{Name: r.name}
		}
		return m, tea.Quit
	}
}

func goLayer(r row) func(m model) (model, tea.Cmd) {
	return func(m model) (model, tea.Cmd) {
		m.chosen = &Choice{Argv: []string{"stack", "--in", r.name, "go", r.layer}, Name: r.name}
		return m, tea.Quit
	}
}

func clearSignals(r row) func(m model) (model, tea.Cmd) {
	return run(r, "signal", r.name, "clear")
}

func run(r row, argv ...string) func(m model) (model, tea.Cmd) {
	return func(m model) (model, tea.Cmd) {
		m.chosen = &Choice{Argv: argv, Dir: r.dir}
		return m, tea.Quit
	}
}

func (m model) View() string {
	if m.mode == making {
		return m.formView()
	}
	width := max(m.width-2, 40)
	var b strings.Builder
	head := accent.Render("› ") + ink.Render(m.query) + accent.Render("█")
	b.WriteString(between(head, m.machinesLine(), width) + "\n\n")
	shown := 0
	room := max(m.height-6, 5)
	start := 0
	if m.cursor >= room {
		start = m.cursor - room + 1
	}
	for i := start; i < len(m.rows) && shown < room; i++ {
		b.WriteString(m.line(m.rows[i], i == m.cursor, width) + "\n")
		shown++
	}
	if len(m.rows) == 0 {
		b.WriteString(faint.Render("nothing called "+m.query+" — esc clears") + "\n")
	}
	for ; shown < room; shown++ {
		b.WriteString("\n")
	}
	foot := "type to find a worktree or an action  ·  ⏎ switch  ·  → open a stack  ·  ^x clear its signals  ·  esc close"
	if m.confirm != "" {
		foot = m.confirm
	}
	b.WriteString(faint.Render(cut(foot, width)))
	return b.String()
}

func (m model) machinesLine() string {
	if len(m.data.Machines) < 2 {
		return ""
	}
	var parts []string
	for _, state := range m.data.Machines {
		if state.Answers {
			parts = append(parts, Style("green").Render("●")+" "+muted.Render(state.Name))
		} else {
			parts = append(parts, Style("red").Render("○ "+state.Name))
		}
	}
	return strings.Join(parts, "  ")
}

func (m model) line(r row, on bool, width int) string {
	if r.kind == group {
		return italic.Render(r.label)
	}
	mark := "  "
	name := ink
	if on {
		mark, name = inkB.Render("›")+" ", inkB
	}
	indent := "  "
	if r.kind == layer {
		indent = "      "
	}
	var text string
	switch r.kind {
	case action:
		text = mark + indent + name.Render(r.label)
	default:
		label := r.label
		if r.kind == worktree && len(r.layers) > 1 {
			label = "▸ " + label
			if m.open[r.name] {
				label = "▾ " + r.label
			}
		}
		text = mark + indent + name.Render(pad(label, 22)) + muted.Render(pad(r.branch, 28))
		if len(r.signals) > 0 {
			var said []string
			for _, s := range r.signals {
				one := Style(s.Style).Render(strings.TrimSpace(s.Kind+"  "+s.Text)) + faint.Render("  "+s.From)
				said = append(said, one)
			}
			text += strings.Join(said, faint.Render("  ·  "))
		} else {
			text += faint.Render(r.detail)
		}
		if r.machine != "" {
			text = between(text, muted.Render("@"+r.machine), width)
		}
	}
	text = cut(text, width)
	if on {
		return sel.Render(text + strings.Repeat(" ", max(width-lipgloss.Width(text), 0)))
	}
	return text
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
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}
