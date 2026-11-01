// Package radar is mia inside tmux: one popup that shows what there is and
// finds anything by typing, and the popup that makes a new worktree. It
// knows worktrees, stacks, machines and signals; nothing about agents.
package radar

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/session"
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
	// Activity is how much each worktree here moved in the last 24 hours,
	// by its folder: commits in each of eight three-hour spans, oldest first.
	Activity map[string][]int
	// Away is the computer that handed this terminal here, and what it has:
	// picking one of its worktrees hands the terminal back.
	Away *Away
}

// Away is the note a computer leaves on a machine it hands a terminal to.
type Away struct {
	From      string        `json:"from"`
	Repo      string        `json:"repo"`
	Worktrees []app.Listing `json:"worktrees"` // its own, and other machines' as it heard them
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
	// Return: go back to the computer that handed this terminal here, to
	// this worktree of its — name, or name@machine for another machine's.
	Return string
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
	listing *app.Listing
	last    bool   // the worktree switched to last, other than here
	away    string // reached by going back: what to tell the computer that handed us over
}

var (
	ink    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	inkB   = ink.Bold(true)
	muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	soft   = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
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
	commits map[string][]git.Commit // the preview's, by worktree, fetched as the cursor reaches it
	screens map[string]string       // each worktree's session as it looks, by row name
	filter  int                     // 0 all, 1 signals, 2 here, then one per machine
	window  map[string]int          // the window the stage shows, by row name
}

type screened struct {
	name   string
	screen string
}

type look struct{ name string }

type previewed struct {
	dir     string
	commits []git.Commit
}

// fetch asks for what the preview shows of the selected worktree: its
// recent commits once, and its session's screen, again every second while
// it stays selected.
func (m model) fetch() tea.Cmd {
	r := m.selected()
	var cmds []tea.Cmd
	if r.kind != worktree {
		return nil
	}
	if r.dir != "" && r.machine == "" {
		if _, ok := m.commits[r.dir]; !ok {
			dir := r.dir
			cmds = append(cmds, func() tea.Msg { return previewed{dir, git.Recent(dir, 5)} })
		}
	}
	if capture := m.capture(r); capture != nil {
		cmds = append(cmds, capture)
	}
	return tea.Batch(cmds...)
}

// capture reads the worktree's session's active window, here or on its
// machine; nothing when it has no session.
func (m model) capture(r row) tea.Cmd {
	host := session.Here()
	if r.machine != "" {
		found := false
		for _, state := range m.data.Machines {
			if state.Name == r.machine && state.Answers {
				host, found = session.On(state.Machine), true
			}
		}
		if !found {
			return nil
		}
	} else if r.listing == nil || !r.listing.Session {
		return nil
	}
	name, worktree, window := r.name, r.label, ""
	if w, ok := m.shownWindow(r); ok {
		window = strconv.Itoa(w.Index)
	}
	return func() tea.Msg {
		screen, _ := host.Capture(worktree, window, true)
		return screened{name, screen}
	}
}

// Run opens the radar, or the new-worktree popup when newOnly, and returns
// what was picked, nil when nothing was.
func Run(data Data, newOnly bool) (*Choice, error) {
	m := model{data: data, open: map[string]bool{}, form: newForm(data), commits: map[string][]git.Commit{}, screens: map[string]string{}, window: map[string]int{}}
	if newOnly {
		m.mode = making
	}
	m.rows = m.build()
	m.cursor = m.opening()
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return nil, err
	}
	return final.(model).chosen, nil
}

func (m model) Init() tea.Cmd { return m.fetch() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tick, finished:
		return m.follow(msg)
	case previewed:
		m.commits[msg.dir] = msg.commits
		return m, nil
	case screened:
		m.screens[msg.name] = msg.screen
		name := msg.name
		return m, tea.Tick(time.Second, func(time.Time) tea.Msg { return look{name} })
	case look:
		if r := m.selected(); r.name == msg.name && m.mode == browsing {
			return m, m.capture(r) // still selected: look again
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == making {
			return m.formKey(msg)
		}
		next, cmd := m.browseKey(msg)
		if nm, ok := next.(model); ok && cmd == nil {
			return nm, nm.fetch()
		}
		return next, cmd
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
	case "down":
		m.cursor = m.next(1)
	case "ctrl+n":
		return m.newWorktree(strings.TrimSpace(m.query))
	case "tab", "shift+tab":
		step := 1
		if msg.String() == "shift+tab" {
			step = -1
		}
		m.filter = (m.filter + step + len(m.filters())) % len(m.filters())
		m.rows, m.cursor = m.build(), 0
		m.cursor = m.next(0)
	case "right", "left":
		r := m.selected()
		switch {
		case msg.String() == "right" && r.kind == worktree && len(r.layers) > 1 && !m.open[r.name]:
			m.open[r.name] = true
			m.rows = m.build()
		case msg.String() == "left" && (r.kind == layer || m.open[r.name]):
			delete(m.open, r.name)
			m.rows = m.build()
			m.cursor = m.next(0)
		case r.listing != nil && len(r.listing.Windows) > 1:
			// ← → flip the stage between the session's windows
			step := 1
			if msg.String() == "left" {
				step = -1
			}
			n := len(r.listing.Windows)
			cur, _ := m.windowAt(r)
			m.window[r.name] = (cur + step + n) % n
			delete(m.screens, r.name)
			return m, m.capture(r)
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

// opening is where the cursor opens: on the top signal, else on the
// worktree the radar was opened from, else on the first worktree.
func (m model) opening() int {
	for i, r := range m.rows {
		if len(r.signals) > 0 {
			return i
		}
	}
	for i, r := range m.rows {
		if r.last {
			return i
		}
	}
	for i, r := range m.rows {
		if r.kind == worktree && r.name == m.data.Here {
			return i
		}
	}
	return m.next(0)
}

// newWorktree turns the radar into the new-worktree form, the branch
// started with what was typed.
func (m model) newWorktree(branch string) (model, tea.Cmd) {
	m.mode = making
	if branch != "" && !strings.HasPrefix(branch, "new") {
		m.form.branch = m.data.Prefix + strings.TrimPrefix(branch, m.data.Prefix)
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
	if m.filter > 0 {
		f := m.filters()[m.filter]
		var kept []row
		for _, r := range all {
			if f.keep(r) {
				kept = append(kept, r)
			}
		}
		if strings.TrimSpace(m.query) == "" {
			sort.SliceStable(kept, func(i, j int) bool { return rank(kept[i]) < rank(kept[j]) })
			return m.withLayers(nil, kept)
		}
		all = kept
	}
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
			rows = append(rows, m.actionsFor(found[0])...)
		}
		typed := strings.TrimSpace(m.query)
		switch {
		case strings.HasPrefix("new worktree", q) || strings.HasPrefix(q, "new"):
			rows = append(rows, row{kind: group, label: "everything else"},
				row{kind: action, label: "new worktree", act: func(m model) (model, tea.Cmd) { return m.newWorktree("") }})
		case len(found) == 0 && !strings.ContainsAny(typed, " "):
			rows = append(rows, row{kind: group, label: "nothing called " + typed},
				row{kind: action, label: "new worktree  " + typed, act: func(m model) (model, tea.Cmd) { return m.newWorktree(typed) }})
		}
		return rows
	}
	var signalled, progress []row
	elsewhere := map[string][]row{} // by machine: listed always, never resting
	var machines []string
	resting := 0
	for _, r := range all {
		switch {
		case len(r.signals) > 0:
			signalled = append(signalled, r)
		case r.machine != "":
			if _, seen := elsewhere[r.machine]; !seen {
				machines = append(machines, r.machine)
			}
			elsewhere[r.machine] = append(elsewhere[r.machine], r)
		case r.detail != "" || len(r.layers) > 1 || r.name == m.data.Here || (r.listing != nil && r.listing.Session):
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
	for _, machine := range machines {
		label := fmt.Sprintf("on %s  %d", machine, len(elsewhere[machine]))
		for _, state := range m.data.Machines {
			if state.Name == machine && !state.Answers {
				label += "  ○ not answering — as it said last"
			}
		}
		rows = append(rows, row{kind: group, label: label})
		rows = m.withLayers(rows, elsewhere[machine])
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
	last := ""
	var lastSeen time.Time
	for _, l := range m.data.Local {
		if l.Name != m.data.Here && !l.Main && l.Seen.After(lastSeen) {
			last, lastSeen = l.Name, l.Seen
		}
	}
	for _, l := range m.data.Local {
		if l.Main && !l.Adopted {
			continue
		}
		l := l
		r := row{kind: worktree, label: l.Name, branch: l.Branch, name: l.Name, dir: l.Path, signals: l.Signals, listing: &l, last: l.Name == last}
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
	if a := m.data.Away; a != nil {
		for _, l := range a.Worktrees {
			l := l
			machine, back := a.From, l.Name
			if l.Machine != "" {
				machine, back = l.Machine, l.Name+"@"+l.Machine
			}
			rows = append(rows, row{kind: worktree, label: l.Name, branch: l.Branch, name: l.Name + "@" + machine,
				machine: machine, signals: l.Signals, detail: listingDetail(l), listing: &l, away: back})
		}
	}
	for _, state := range m.data.Machines {
		for _, l := range state.Worktrees {
			l := l
			rows = append(rows, row{kind: worktree, label: l.Name, branch: l.Branch, name: l.Name + "@" + state.Name,
				machine: state.Name, signals: l.Signals, detail: listingDetail(l), listing: &l})
		}
	}
	return rows
}

// listingDetail is what tells one worktree from another at a glance:
// commits ahead, uncommitted work, a restack due, and how long ago it moved.
func listingDetail(l app.Listing) string {
	var parts []string
	if l.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("+%d", l.Ahead))
	}
	if l.Dirty {
		parts = append(parts, "✎")
	}
	for _, layer := range l.Layers {
		if layer.NeedsRestack {
			parts = append(parts, "⟳ restack due")
			break
		}
	}
	if !l.LastWork.IsZero() {
		parts = append(parts, ago(time.Since(l.LastWork)))
	}
	return strings.Join(parts, " · ")
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (m model) actionsFor(r row) []row {
	acts := []row{{kind: action, label: "switch there", act: switchTo(r)}}
	if r.away == "" {
		if r.machine != "" {
			acts = append(acts, row{kind: action, label: "move here — its changes and windows too", act: run(r, "move", r.name, "local")})
		} else {
			for _, state := range m.data.Machines {
				if state.Name != "" && state.Name != "local" && state.Answers {
					acts = append(acts, row{kind: action, label: "move to " + state.Name + " — its changes and windows too", act: run(r, "move", r.name, state.Name)})
				}
			}
		}
	}
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
		if r.away != "" {
			m.chosen = &Choice{Return: r.away}
			return m, tea.Quit
		}
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
