package tui

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/emusoi/mia-core/internal/panel"
)

var (
	accent  = lipgloss.Color("12")
	dimmed  = lipgloss.NewStyle().Faint(true)
	strong  = lipgloss.NewStyle().Bold(true)
	hot     = lipgloss.NewStyle().Bold(true).Foreground(accent)
	keyCap  = lipgloss.NewStyle().Bold(true)
	problem = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	good    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	bar     = lipgloss.NewStyle().Background(lipgloss.Color("8"))
	rail    = lipgloss.NewStyle().Faint(true)
	tones   = map[string]lipgloss.Style{
		panel.GlyphWaiting:  hot,
		panel.GlyphWorking:  strong,
		panel.GlyphDone:     dimmed,
		panel.GlyphDirty:    lipgloss.NewStyle(),
		panel.GlyphDetached: dimmed,
		panel.GlyphHere:     strong,
	}
)

type Chosen struct {
	Draft  string
	Tab    string
	Argv   []string
	Action panel.Action
	RowID  string
	Panel  string
	Open   map[string]bool
}

type Reload func() (panel.Panel, error)

type Hooks struct {
	Peek    func(rowID, want string) (tab string, lines []string, ok bool)
	Popup   func(Chosen) error
	Close   func()
	Refresh func()
	Perform func(Chosen) Outcome
	Load    func(name, target string) (panel.Panel, Reload, Hooks, error)
	Picks   func(Chosen) bool
	Drafts  map[string]string
	Keep    func(map[string]string)
	Switch  bool
	Do      string
	Here    string
	Watch   func(nudge func())
}

type nudged struct{}

type Outcome struct {
	Output string
	Err    error
	Retry  *panel.Action
}

const peekEvery = 250 * time.Millisecond

const refreshEvery = 2 * time.Second

type mode int

const (
	browsing mode = iota
	confirming
	filtering
	typing
	helping
	working
	reporting
)

type model struct {
	gen      int64
	panel    panel.Panel
	rows     []panel.Row
	expanded map[string]bool
	cursor   int
	offset   int
	width    int
	height   int
	mode     mode
	query    string
	input    string
	pending  *pending
	chosen   Chosen
	reload   Reload
	hooks    Hooks
	popup    *Chosen
	reopen   bool
	peeking  bool
	tab      int
	live     []string
	liveFor  string
	liveName string
	liveSame int
	problem  string
	label    string
	spin     int
	report   []string
	notice   string
	doTitle  string
	stale    bool
	scroll   int
	fill     bool
	picking  bool
	pickCol  int
	pick     [3]string
	focus    string
	made     string
	choice   string
	details  bool
	stack    []model
}

type pending struct {
	action panel.Action
	row    panel.Row
}

type reloaded struct {
	panel panel.Panel
	err   error
	once  bool
	gen   int64
}

type tick struct{ gen int64 }

var generations atomic.Int64

type peekTick struct{}

type viewDone struct{ err error }

type spinTick struct{}

type performed struct {
	chosen  Chosen
	outcome Outcome
}

type loaded struct {
	name   string
	panel  panel.Panel
	reload Reload
	hooks  Hooks
	err    error
}

type peeked struct {
	row   string
	tab   string
	lines []string
	ok    bool
}

func Open(title string, reload Reload, hooks Hooks, open map[string]bool) (Chosen, error) {
	m := newModel(panel.Panel{Version: panel.Version, Title: title})
	m.reload = reload
	m.hooks = hooks
	for id, isOpen := range open {
		m.expanded[id] = isOpen
	}
	m.mode, m.label = working, loadingLabel
	m.fill = os.Getenv("MIA_POPUP") != ""
	return run(m)
}

const loadingLabel = "loading"

func (m model) loading() bool { return m.mode == working && m.label == loadingLabel }

var defaultKeys = panel.Moves

func (m model) keysFor(name string) []string {
	if keys, ok := m.panel.Keys[name]; ok {
		return keys
	}
	return defaultKeys[name]
}

func (m model) shown(names ...string) string {
	var firsts []string
	for _, name := range names {
		if keys := m.keysFor(name); len(keys) > 0 {
			firsts = append(firsts, keys[0])
		}
	}
	return strings.Join(firsts, " ")
}

func (m model) nav(key string) string {
	for name := range defaultKeys {
		for _, k := range m.keysFor(name) {
			if k == key {
				return name
			}
		}
	}
	return ""
}

func run(m model) (Chosen, error) {
	program := tea.NewProgram(m, tea.WithAltScreen())
	if m.hooks.Watch != nil {
		m.hooks.Watch(func() { program.Send(nudged{}) })
		defer m.hooks.Watch(nil)
	}
	final, err := program.Run()
	if err != nil {
		return Chosen{}, err
	}
	root := final.(model)
	if len(root.stack) > 0 {
		root = root.stack[0]
	}
	chosen := final.(model).chosen
	chosen.Open = root.expanded
	return chosen, nil
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	gen := m.gen
	if m.reload != nil {
		if m.mode == working && m.label == loadingLabel {
			reload := m.reload
			cmds = append(cmds,
				func() tea.Msg { p, err := reload(); return reloaded{panel: p, err: err, gen: gen} },
				tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTick{} }))
		} else {
			cmds = append(cmds, tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tick{gen: gen} }))
		}
	}
	return tea.Batch(cmds...)
}

func (m model) peekCmd() tea.Cmd {
	if m.hooks.Peek == nil || m.cursor >= len(m.rows) {
		return nil
	}
	if m.side() {
		if m.attention() != m.cursor {
			return nil
		}
		tabs := m.tabs()
		if current := min(m.tab, len(tabs)-1); current >= 0 && (tabs[current] == "facts" || tabs[current] == "blocks") {
			return nil
		}
	}
	row := m.rows[m.cursor].ID
	who := m.selected()
	target := m.rows[m.cursor].TargetID()
	if who.Worktree != "" {
		target = who.Worktree
	}
	return func() tea.Msg {
		tab, lines, ok := m.hooks.Peek(target, who.Name)
		return peeked{row: row, tab: tab, lines: lines, ok: ok}
	}
}

func (m model) viewCmd() tea.Cmd {
	chosen, show := *m.popup, m.hooks.Popup
	return func() tea.Msg {
		return viewDone{err: show(chosen)}
	}
}

func (m model) viewChosen() (Chosen, bool) {
	if m.cursor >= len(m.rows) {
		return Chosen{}, false
	}
	row := m.rows[m.cursor]
	for _, id := range []string{"open", "shell"} {
		action, ok := m.panel.Actions[id]
		if !ok || !action.Lands || !available(row, id, action) {
			continue
		}
		return Chosen{Argv: m.argvFor(action, row), Action: action, RowID: row.TargetID()}, true
	}
	return Chosen{}, false
}

func (m model) nextPeek() tea.Cmd {
	every := peekEvery
	if m.liveSame >= 4 {
		every = 4 * peekEvery
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return peekTick{} })
}

func newModel(p panel.Panel) model {
	m := model{panel: p, expanded: map[string]bool{}, width: 80, height: 24, gen: generations.Add(1)}
	m.rows = m.visibleRows()
	return m
}

func (m model) visibleRows() []panel.Row {
	var rows []panel.Row
	for _, section := range m.panel.Sections {
		if section.Collapsed && !m.expanded[section.ID] && m.query == "" {
			continue
		}
		rows = append(rows, m.rowsOf(section)...)
	}
	return rows
}

func (m model) rowsOf(section panel.Section) []panel.Row {
	var rows []panel.Row
	for _, row := range section.Rows {
		if m.matches(row) {
			rows = m.emit(rows, row, 0, "", m.query == "" || m.matchesOne(row))
		}
	}
	return rows
}

func (m model) emit(rows []panel.Row, row panel.Row, depth int, rail string, all bool) []panel.Row {
	shown := row
	shown.Depth = depth
	shown.Cells = append([]string{}, row.Cells...)
	name := shown.Cells[0]
	if len(row.Children) > 0 {
		if m.isOpen(row) {
			name = "▾ " + name
		} else {
			name = "▸ " + name
		}
	}
	if depth > 0 {
		name = rail + " " + name
	}
	shown.Cells[0] = name
	rows = append(rows, shown)
	if len(row.Children) == 0 || !m.isOpen(row) {
		return rows
	}
	var kept []panel.Row
	for _, child := range row.Children {
		if all || m.matches(child) {
			kept = append(kept, child)
		}
	}
	for i, child := range kept {
		bar := "│"
		if i == len(kept)-1 {
			bar = "└"
		}
		rows = m.emit(rows, child, depth+1, strings.Repeat("  ", depth)+" "+bar, all || m.matchesOne(child))
	}
	return rows
}

func (m model) fold(row panel.Row, open bool) (tea.Model, tea.Cmd) {
	m.expanded["row:"+row.ID] = open
	m.rows = m.visibleRows()
	if m.cursor >= len(m.rows) {
		m.cursor = max(len(m.rows)-1, 0)
	}
	return m.keepCursorVisible(), m.peekCmd()
}

func (m model) isOpen(row panel.Row) bool {
	if m.query != "" {
		for _, child := range row.Children {
			if m.matchesOne(child) {
				return true
			}
		}
	}
	return m.expanded["row:"+row.ID]
}

func (m model) matches(row panel.Row) bool {
	if m.matchesOne(row) {
		return true
	}
	for _, child := range row.Children {
		if m.matchesOne(child) {
			return true
		}
	}
	return false
}

func (m model) matchesOne(row panel.Row) bool {
	if m.query == "" {
		return true
	}
	text := strings.ToLower(row.ID + " " + strings.Join(row.Cells, " "))
	return strings.Contains(text, strings.ToLower(m.query))
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(message)
	m = next.(model)
	if !m.peeking && m.mode == browsing && m.peekCmd() != nil {
		m.peeking = true
		cmd = tea.Batch(cmd, m.nextPeek())
	}
	return m, cmd
}

func (m model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.keepCursorVisible()
		if m.popup != nil && m.hooks.Close != nil {
			m.reopen = true
			m.hooks.Close()
		}
		return m, nil

	case tick:
		if msg.gen != m.gen {
			return m, nil
		}
		return m, m.refreshCmd()

	case peekTick:
		m.peeking = false
		if cmd := m.peekCmd(); cmd != nil {
			m.peeking = true
			return m, tea.Batch(cmd, m.nextPeek())
		}
		return m, nil

	case viewDone:
		if msg.err != nil {
			m.problem = msg.err.Error()
		}
		if m.reopen && msg.err == nil && m.popup != nil {
			m.reopen = false
			return m, m.viewCmd()
		}
		m.reopen, m.popup = false, nil
		if m.hooks.Refresh != nil {
			m.hooks.Refresh()
		}
		return m, m.reloadCmd()

	case peeked:
		if msg.ok {
			if msg.row == m.liveFor && strings.Join(msg.lines, "\n") == strings.Join(m.live, "\n") {
				m.liveSame++
			} else {
				m.liveSame = 0
			}
			m.live, m.liveFor, m.liveName = msg.lines, msg.row, msg.tab
		} else if m.liveFor == msg.row {
			m.live, m.liveFor, m.liveName = nil, "", ""
		}
		return m, nil

	case nudged:
		if len(m.stack) > 0 || m.mode == working || m.reload == nil {
			return m, nil
		}
		reload := m.reload
		gen := m.gen
		return m, func() tea.Msg { p, err := reload(); return reloaded{panel: p, err: err, once: true, gen: gen} }

	case reloaded:
		if msg.gen != m.gen {
			return m, nil
		}
		gen := m.gen
		next := tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tick{gen: gen} })
		if msg.once {
			next = nil
		}
		first := m.mode == working && m.label == loadingLabel
		if first {
			m.mode, m.label = browsing, ""
			if m.hooks.Switch {
				m.mode = filtering
			}
		}
		if msg.err != nil {
			m.problem, m.stale = msg.err.Error(), true
			return m, next
		}
		if m.stale {
			m.problem, m.stale = "", false
		}
		if first && m.hooks.Here != "" {
			m.focus = m.hooks.Here
		}
		m = m.replace(msg.panel)
		if action, ok := m.panel.Actions[m.hooks.Do]; first && ok {
			var row panel.Row
			if m.cursor < len(m.rows) {
				row = m.rows[m.cursor]
			}
			if action.NeedsRow() && !offers(row, m.hooks.Do) {
				m.mode, m.report = reporting, []string{nameOf(row) + " has nothing to " + action.Label}
				return m, next
			}
			began, cmd := m.run(m.hooks.Do, row)
			if b, ok := began.(model); ok && b.pending != nil {
				b.doTitle = b.popupTitle()
				began = b
			}
			return began, tea.Batch(cmd, next)
		}
		return m, next

	case spinTick:
		if m.mode != working {
			return m, nil
		}
		m.spin++
		return m, tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTick{} })

	case performed:
		m.made = ""
		busy := m.mode != working && m.mode != browsing
		if !busy {
			m.mode, m.label = browsing, ""
		}
		if m.hooks.Refresh != nil {
			m.hooks.Refresh()
		}
		again := m.reloadCmd()
		if msg.outcome.Err == nil {
			if name := made(msg.chosen, msg.outcome.Output); name != "" {
				m.focus, m.made = name, name
			}
		}
		if m.hooks.Do != "" && !busy && msg.outcome.Err != nil && msg.chosen.Action.Input != "" {
			row := panel.Row{ID: msg.chosen.RowID}
			for _, r := range m.rows {
				if r.ID == row.ID {
					row = r
				}
			}
			m.mode, m.pending = typing, &pending{action: msg.chosen.Action, row: row}
			m.problem = lastLine(msg.outcome.Output, msg.outcome.Err.Error())
			return m, nil
		}
		if m.hooks.Do != "" && !busy {
			said := "✓ " + lastLine(msg.outcome.Output, "done")
			if m.made != "" && msg.outcome.Err == nil {
				said = "✓ made " + m.made
			}
			if msg.outcome.Err != nil {
				said = "✗ " + lastLine(msg.outcome.Output, msg.outcome.Err.Error())
				if msg.chosen.Action.Lines {
					said += " · draft kept, nothing resent"
				}
			} else if msg.chosen.Action.Lines {
				delete(m.hooks.Drafts, msg.chosen.Draft)
				if m.hooks.Keep != nil {
					m.hooks.Keep(m.hooks.Drafts)
				}
			}
			m.mode, m.report = reporting, []string{said}
			return m, nil
		}
		if msg.outcome.Err != nil {
			if msg.outcome.Retry != nil && !busy {
				row := panel.Row{ID: msg.chosen.RowID}
				for _, r := range m.rows {
					if r.ID == row.ID {
						row = r
					}
				}
				m.mode, m.pending = confirming, &pending{action: *msg.outcome.Retry, row: row}
				return m, nil
			}
			if msg.chosen.Action.Report && !busy && strings.TrimSpace(msg.outcome.Output) != "" {
				m.mode, m.report = reporting, strings.Split(strings.TrimRight(msg.outcome.Output, "\n"), "\n")
				return m, again
			}
			m.problem = lastLine(msg.outcome.Output, msg.outcome.Err.Error())
			if msg.chosen.Action.Lines {
				m.problem += " · draft kept, nothing resent"
			}
			return m, again
		}
		if msg.chosen.Action.Report && !busy {
			m.mode, m.report = reporting, strings.Split(strings.TrimRight(msg.outcome.Output, "\n"), "\n")
			return m, again
		}
		if msg.chosen.Action.Lines {
			delete(m.hooks.Drafts, msg.chosen.Draft)
			if m.hooks.Keep != nil {
				m.hooks.Keep(m.hooks.Drafts)
			}
		}
		m.notice = "✓ " + lastLine(msg.outcome.Output, "mia "+strings.Join(msg.chosen.Argv, " "))
		if m.made != "" {
			m.notice = "✓ made " + m.made + " · ⏎ attaches"
		}
		return m, again

	case loaded:
		m.mode, m.label = browsing, ""
		if msg.err != nil {
			m.problem = msg.err.Error()
			return m, nil
		}
		parent := m
		parent.stack = nil
		next := newModel(msg.panel)
		next.width, next.height = m.width, m.height
		next.reload, next.hooks = msg.reload, msg.hooks
		next.stack = append(append([]model{}, m.stack...), parent)
		return next, next.Init()

	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) refreshCmd() tea.Cmd {
	if m.reload == nil {
		return nil
	}
	reload, gen := m.reload, m.gen
	return func() tea.Msg { p, err := reload(); return reloaded{panel: p, err: err, gen: gen} }
}

func (m model) reloadCmd() tea.Cmd {
	if m.reload == nil {
		return nil
	}
	reload := m.reload
	gen := m.gen
	return func() tea.Msg { p, err := reload(); return reloaded{panel: p, err: err, gen: gen, once: true} }
}

func lastLine(output, fallback string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return fallback
}

func (m model) start(chosen Chosen) (tea.Model, tea.Cmd) {
	if chosen.Tab == "" {
		chosen.Tab = m.selected().Name
	}
	if m.hooks.Picks != nil && m.hooks.Picks(chosen) {
		m.chosen = chosen
		return m, tea.Quit
	}
	if chosen.Action.Lands && (chosen.Action.Popup || m.hooks.Picks != nil) && m.hooks.Popup != nil && m.popup == nil {
		m.popup = &chosen
		return m, m.viewCmd()
	}
	if chosen.Action.Lands || m.hooks.Perform == nil {
		m.chosen = chosen
		return m, tea.Quit
	}
	m.mode, m.label, m.problem = working, "mia "+strings.Join(chosen.Argv, " "), ""
	perform := m.hooks.Perform
	return m, tea.Batch(
		func() tea.Msg { return performed{chosen: chosen, outcome: perform(chosen)} },
		tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTick{} }),
	)
}

func (m model) open(name, target string) (tea.Model, tea.Cmd) {
	if m.hooks.Load == nil {
		m.chosen = Chosen{Panel: name, RowID: target}
		return m, tea.Quit
	}
	m.mode, m.label, m.problem = working, "opening "+strings.TrimSuffix(name, "-panel"), ""
	load := m.hooks.Load
	return m, tea.Batch(
		func() tea.Msg {
			p, reload, hooks, err := load(name, target)
			return loaded{name, p, reload, hooks, err}
		},
		tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return spinTick{} }),
	)
}

func (m model) replace(p panel.Panel) model {
	held := ""
	if m.cursor < len(m.rows) {
		held = m.rows[m.cursor].ID
	}
	if m.focus != "" {
		held, m.focus = m.focus, ""
		for _, section := range p.Sections {
			for _, row := range section.Rows {
				if row.ID == held && section.Collapsed {
					m.expanded[section.ID] = true
				}
			}
		}
	}
	m.panel = p
	m.rows = m.visibleRows()
	m.cursor = 0
	if held == "" {
		m.cursor = max(m.nextNeed(-1, needsYou), 0)
	}
	for i, row := range m.rows {
		if row.ID == held {
			m.cursor = i
		}
	}
	return m.keepCursorVisible()
}

func (m model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste && (m.mode == browsing || m.mode == helping) {
		var next tea.Model = m
		var cmds []tea.Cmd
		for _, r := range msg.Runes {
			var cmd tea.Cmd
			next, cmd = next.(model).key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			cmds = append(cmds, cmd)
		}
		return next, tea.Batch(cmds...)
	}
	key := msg.String()
	dismissing := key == "esc" && m.mode == browsing && (m.notice != "" || (!m.stale && m.problem != ""))
	m.notice = ""
	if !m.stale && m.mode == browsing {
		m.problem = ""
	}
	if dismissing {
		return m, nil
	}

	switch m.mode {
	case working:
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if key == "esc" && strings.HasPrefix(m.label, "mia ") {
			m.mode, m.notice, m.label = browsing, "… still running: "+m.label, ""
		}
		return m, nil

	case reporting:
		if key == "enter" && m.hooks.Do != "" && m.made != "" {
			if action, ok := m.panel.Actions["open"]; ok {
				m.chosen = Chosen{Argv: action.Command(m.made), Action: action, RowID: m.made}
			}
			return m, tea.Quit
		}
		if key == "q" || key == "esc" || key == "enter" || key == "ctrl+c" {
			if m.hooks.Do != "" {
				return m, tea.Quit
			}
			m.mode, m.report = browsing, nil
		}
		return m, nil

	case confirming:
		if key == "y" || key == "Y" {
			action, row := m.pending.action, m.pending.row
			m.mode, m.pending = browsing, nil
			return m.start(Chosen{Argv: m.argvFor(action, row), Action: action, RowID: row.TargetID()})
		}
		m.mode, m.pending = browsing, nil
		if m.hooks.Do != "" {
			return m, tea.Quit
		}
		return m, nil

	case typing:
		choices := m.choices()
		if m.picking {
			return m.pickKey(key)
		}
		if m.pending.action.Lines {
			if opts := m.pending.row.Options; len(opts) > 0 && m.input == "" && len(key) == 1 {
				for _, option := range opts {
					if strings.HasPrefix(option, key+" ") {
						m.input = key
						return m, nil
					}
				}
			}
			switch key {
			case "enter":
				if m.answersOption() {
					break
				}
				m.input += "\n"
				return m, nil
			case "tab":
				if combined(choices) {
					m.picking, m.pickCol = true, 0
					words := strings.Fields(m.choice)
					m.pick = [3]string{"", "-", ""}
					for i := range min(len(words), 3) {
						m.pick[i] = words[i]
					}
					return m, nil
				}
				if len(choices) > 0 {
					m.choice = choices[(slices.Index(choices, m.choice)+1)%len(choices)]
					if m.hooks.Drafts == nil {
						m.hooks.Drafts = map[string]string{}
					}
					m.hooks.Drafts["choice:"+m.pending.action.Input] = m.choice
					if m.hooks.Keep != nil {
						m.hooks.Keep(m.hooks.Drafts)
					}
				}
				return m, nil
			case "ctrl+s":
				key = "enter"
			case "esc":
				m.keepDraft()
				m.mode, m.pending, m.input = browsing, nil, ""
				if m.hooks.Do != "" {
					return m, tea.Quit
				}
				return m, nil
			case "ctrl+c":
				m.keepDraft()
				return m, tea.Quit
			}
		}
		switch key {
		case "tab":
			if len(choices) > 0 {
				m.input = choices[(slices.Index(choices, m.input)+1)%len(choices)]
			}
		case "enter":
			if strings.TrimSpace(m.input) == "" {
				return m, nil
			}
			action, row := m.pending.action, m.pending.row
			if action.Confirm != "" {
				m.mode = confirming
				return m, nil
			}
			m.keepDraft()
			if !action.Lines && len(choices) > 0 && slices.Contains(choices, strings.TrimSpace(m.input)) {
				if m.hooks.Drafts == nil {
					m.hooks.Drafts = map[string]string{}
				}
				m.hooks.Drafts["choice:"+action.Input] = strings.TrimSpace(m.input)
				if m.hooks.Keep != nil {
					m.hooks.Keep(m.hooks.Drafts)
				}
			}
			draft := m.draftKey(row)
			m.mode, m.pending = browsing, nil
			return m.start(Chosen{Argv: m.argvFor(action, row), Action: action, RowID: row.TargetID(), Draft: draft})
		case "esc", "ctrl+c":
			m.mode, m.pending, m.input = browsing, nil, ""
			if m.hooks.Do != "" {
				return m, tea.Quit
			}
		default:
			if !m.pending.action.Lines && len(choices) > 0 && len(key) == 1 && key[0] >= '1' && key[0] <= '9' && int(key[0]-'1') < len(choices) {
				m.input = choices[key[0]-'1']
				return m, nil
			}
			m.input = edited(m.input, msg)
		}
		return m, nil

	case filtering:
		switch key {
		case "up", "ctrl+p":
			m.cursor = max(m.cursor-1, 0)
			return m.keepCursorVisible(), nil
		case "down", "ctrl+n":
			m.cursor = min(m.cursor+1, max(len(m.rows)-1, 0))
			return m.keepCursorVisible(), nil
		case "enter":
			m.mode = browsing
			if m.hooks.Switch {
				open, ok := m.panel.Actions["open"]
				if !ok || m.cursor >= len(m.rows) {
					return m, nil
				}
				target := m.rows[m.cursor].TargetID()
				return m.start(Chosen{Argv: open.Command(target), Action: open, RowID: target})
			}
			return m, nil
		case "esc", "ctrl+c":
			if m.hooks.Switch {
				return m, tea.Quit
			}
			m.mode, m.query = browsing, ""
		default:
			m.query = edited(m.query, msg)
		}
		m.rows, m.cursor = m.visibleRows(), 0
		return m.keepCursorVisible(), nil

	case helping:
		n := m.nav(key)
		if n == "help" || n == "back" {
			m.mode, m.scroll = browsing, 0
			return m, nil
		}
		if n == "down" || n == "up" || n == "pagedown" || n == "pageup" {
			step := map[string]int{"down": 1, "up": -1, "pagedown": m.bodyHeight() / 2, "pageup": -m.bodyHeight() / 2}[n]
			m.scroll = min(max(m.scroll+step, 0), max(len(m.help())-m.bodyHeight(), 0))
			return m, nil
		}
		m.scroll = 0
		m.mode = browsing
		return m.key(msg)
	}

	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		if n := int(key[0] - '1'); n < len(m.tabs()) {
			m.tab = n
		}
		return m, nil
	}
	name := m.nav(key)
	if name != "scrollup" && name != "scrolldown" && name != "" && name != "help" {
		m.scroll = 0
	}
	switch name {
	case "back", "quit":
		if m.details && name == "back" {
			m.details = false
			return m, nil
		}
		if m.query != "" && name == "back" {
			m.query = ""
			m.rows, m.cursor = m.visibleRows(), 0
			return m.keepCursorVisible(), nil
		}
		if n := len(m.stack); n > 0 && name == "back" {
			back := m.stack[n-1]
			back.stack = m.stack[:n-1]
			back.width, back.height = m.width, m.height
			back.gen = generations.Add(1)
			return back, tea.Batch(back.Init(), back.reloadCmd())
		}
		return m, tea.Quit
	case "down":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
		return m.keepCursorVisible(), m.peekCmd()
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m.keepCursorVisible(), m.peekCmd()
	case "filter":
		m.mode = filtering
		return m, nil
	case "help":
		m.mode = helping
		return m, nil
	case "scrollup", "scrolldown":
		m.scroll = m.scrolled(name == "scrolldown")
		return m, nil
	case "next":
		if i := m.nextNeed(m.cursor, needsYou); i >= 0 {
			m.cursor = i
		}
		return m.keepCursorVisible(), m.peekCmd()
	case "details":
		m.details = !m.details
		return m, m.peekCmd()
	case "popup":
		if m.hooks.Popup == nil || m.popup != nil {
			return m, nil
		}
		chosen, ok := m.viewChosen()
		if !ok {
			return m, nil
		}
		m.popup = &chosen
		return m, m.viewCmd()
	case "refresh":
		if m.hooks.Refresh != nil {
			m.hooks.Refresh()
		}
		if m.reload != nil {
			return m, m.reloadCmd()
		}
		return m, nil
	case "tabnext", "tabprev":
		tabs := m.tabs()
		if len(tabs) > 1 {
			if name == "tabnext" {
				m.tab = (m.tab + 1) % len(tabs)
			} else {
				m.tab = (m.tab + len(tabs) - 1) % len(tabs)
			}
		}
		return m, nil
	case "first":
		m.cursor = 0
		return m.keepCursorVisible(), m.peekCmd()
	case "last":
		m.cursor = max(len(m.rows)-1, 0)
		return m.keepCursorVisible(), m.peekCmd()
	case "pagedown":
		m.cursor = min(m.cursor+max(m.bodyHeight()/2, 1), max(len(m.rows)-1, 0))
		return m.keepCursorVisible(), m.peekCmd()
	case "pageup":
		m.cursor = max(m.cursor-max(m.bodyHeight()/2, 1), 0)
		return m.keepCursorVisible(), m.peekCmd()
	}
	toggled := false
	for _, section := range m.panel.Sections {
		if key == section.ToggleKey && key != "" {
			m.expanded[section.ID] = !m.expanded[section.ID]
			toggled = true
		}
	}
	if toggled {
		m.rows = m.visibleRows()
		if len(m.rows) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
		}
		return m.keepCursorVisible(), nil
	}
	return m.activate(key)
}

func edited(text string, msg tea.KeyMsg) string {
	switch msg.String() {
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(text)
		return text[:len(text)-size]
	case "ctrl+u":
		return text[:strings.LastIndex(text, "\n")+1]
	case "ctrl+w", "alt+backspace":
		trimmed := strings.TrimRight(text, " ")
		return trimmed[:strings.LastIndexAny(trimmed, " \n")+1]
	}
	if msg.Type == tea.KeyRunes || msg.String() == " " {
		return text + string(msg.Runes)
	}
	return text
}
func (m model) activate(key string) (tea.Model, tea.Cmd) {
	var row panel.Row
	if len(m.rows) > 0 {
		row = m.rows[m.cursor]
	}
	if key == "enter" && len(row.Children) > 0 {
		return m.fold(row, !m.isOpen(row))
	}
	if m.nav(key) == "unfold" && len(row.Children) > 0 {
		return m.fold(row, true)
	}
	if m.nav(key) == "fold" {
		if len(row.Children) > 0 {
			return m.fold(row, false)
		}
		if row.Depth > 0 {
			for i := m.cursor; i >= 0; i-- {
				if len(m.rows[i].Children) > 0 {
					m.cursor = i
					return m.fold(m.rows[i], false)
				}
			}
		}
	}
	for _, id := range m.bound(key, row) {
		return m.run(id, row)
	}
	for _, id := range slices.Sorted(maps.Keys(m.panel.Actions)) {
		if action := m.panel.Actions[id]; action.Matches(key) && len(m.rows) > 0 {
			m.notice = key + " " + action.Label + " is not for " + nameOf(row)
			return m, nil
		}
	}
	return m, nil
}

func (m model) run(id string, row panel.Row) (tea.Model, tea.Cmd) {
	action := m.panel.Actions[id]
	{
		if action.Panel != "" {
			return m.open(action.Panel, row.TargetID())
		}
		if action.Input != "" {
			m.mode, m.pending, m.input = typing, &pending{action: action, row: row}, ""
			if action.Lines {
				m.input = m.hooks.Drafts[m.draftKey(row)]
				if choices := m.choices(); len(choices) > 0 {
					if last := m.hooks.Drafts["choice:"+action.Input]; slices.Contains(choices, last) {
						m.choice = last
					} else if !slices.Contains(choices, m.choice) {
						m.choice = choices[0]
					}
				}
			}
			if choices := m.choices(); !action.Lines && !action.Prefill && len(choices) > 0 {
				m.input = choices[0]
				if last := m.hooks.Drafts["choice:"+action.Input]; slices.Contains(choices, last) {
					m.input = last
				}
			}
			if action.Prefill {
				m.input = nameOf(row)
				if choices := row.Choices[id]; len(choices) > 0 {
					m.input = choices[0]
				}
			}
			return m, nil
		}
		if action.Confirm != "" {
			m.mode, m.pending = confirming, &pending{action: action, row: row}
			return m, nil
		}
		return m.start(Chosen{Argv: m.argvFor(action, row), Action: action, RowID: row.TargetID()})
	}
}

func (m model) bound(key string, row panel.Row) []string {
	var onRow, anywhere []string
	for id, action := range m.panel.Actions {
		switch {
		case !action.Matches(key):
		case action.NeedsRow() && offers(row, id):
			onRow = append(onRow, id)
		case !action.NeedsRow():
			anywhere = append(anywhere, id)
		}
	}
	byRank := func(ids []string) {
		sort.Slice(ids, func(i, j int) bool {
			if a, b := m.panel.Rank(ids[i]), m.panel.Rank(ids[j]); a != b {
				return a < b
			}
			return ids[i] < ids[j]
		})
	}
	byRank(onRow)
	byRank(anywhere)
	return append(onRow, anywhere...)
}

func offers(row panel.Row, id string) bool {
	for _, offered := range row.Actions {
		if offered == id {
			return true
		}
	}
	return false
}

func available(row panel.Row, id string, action panel.Action) bool {
	return !action.NeedsRow() || offers(row, id)
}
func Render(p panel.Panel, cursor int) string {
	m := newModel(p)
	m.cursor, m.width, m.height = cursor, 96, 200
	return m.View()
}

func (m model) side() bool {
	composing := m.mode == typing && m.pending != nil && m.pending.action.Lines
	cols := 4
	unfiltered := m.unfiltered()
	for _, w := range m.rawColumns() {
		cols += w + 2
	}
	fits := m.width >= cols+40+11
	return m.width >= 90 && fits && !m.hooks.Switch && m.hooks.Do == "" && !composing && len(unfiltered) > 0
}

func wrap(line string, width int) []string {
	width = max(width, 1)
	var out []string
	runes := []rune(line)
	for lipgloss.Width(string(runes)) > width {
		fits, used := 0, 0
		for fits < len(runes) {
			if used += lipgloss.Width(string(runes[fits])); used > width {
				break
			}
			fits++
		}
		fits = max(fits, 1)
		cut := fits
		for i := fits - 1; i > 0; i-- {
			if runes[i] == ' ' {
				cut = i + 1
				break
			}
		}
		out = append(out, string(runes[:cut]))
		runes = runes[cut:]
	}
	return append(out, string(runes))
}

func combined(choices []string) bool {
	return slices.ContainsFunc(choices, func(c string) bool { return strings.Contains(c, " ") })
}

func (m model) pickNames() []string {
	if names := m.pending.action.Pick; len(names) > 0 {
		return names[:min(len(names), 3)]
	}
	return []string{"first", "second", "third"}
}

func (m model) pickLabel(c int, v string) string {
	switch {
	case v == "-" && m.pickNames()[c] == "on":
		return "this machine"
	case v == "-":
		return "its own"
	}
	return v
}

func (m model) pickColumns() [3][]string {
	var cols [3][]string
	add := func(i int, v string) {
		if !slices.Contains(cols[i], v) {
			cols[i] = append(cols[i], v)
		}
	}
	for _, choice := range m.choices() {
		w := strings.Fields(choice)
		add(0, w[0])
		if w[0] != m.pick[0] {
			continue
		}
		model := "-"
		if len(w) > 1 {
			model = w[1]
		}
		add(1, model)
		if model != m.pick[1] {
			continue
		}
		effort := "-"
		if len(w) > 2 {
			effort = w[2]
		}
		add(2, effort)
	}
	return cols
}

func (m model) pickKey(key string) (tea.Model, tea.Cmd) {
	cols := m.pickColumns()
	switch key {
	case "left", "h", "shift+tab":
		m.pickCol = max(m.pickCol-1, 0)
	case "right", "l", "tab":
		m.pickCol = min(m.pickCol+1, len(m.pickNames())-1)
	case "up", "k", "down", "j":
		options := cols[m.pickCol]
		if len(options) == 0 {
			return m, nil
		}
		i := slices.Index(options, m.pick[m.pickCol])
		if key == "up" || key == "k" {
			i = max(i-1, 0)
		} else {
			i = min(i+1, len(options)-1)
		}
		m.pick[m.pickCol] = options[i]
		if m.pickCol == 0 {
			m.pick[1], m.pick[2] = "-", "-"
		}
		if m.pickCol == 1 {
			m.pick[2] = "-"
		}
	case "enter":
		parts := m.pick[:len(m.pickNames())]
		for len(parts) > 1 && (parts[len(parts)-1] == "-" || parts[len(parts)-1] == "") {
			parts = parts[:len(parts)-1]
		}
		if choice := strings.Join(parts, " "); slices.Contains(m.choices(), choice) {
			m.choice = choice
		}
		m.picking = false
	case "esc":
		m.picking = false
	}
	return m, nil
}

func (m model) picker() []string {
	cols := m.pickColumns()
	names := m.pickNames()
	lines := []string{"", dimmed.Render("  " + m.pending.action.Label + " · " + strings.Join(names, ", ")), ""}
	height := max(len(cols[0]), len(cols[1]), len(cols[2]))
	row := ""
	for c := range len(names) {
		head := names[c]
		if c == m.pickCol {
			head = strong.Render(head)
		} else {
			head = dimmed.Render(head)
		}
		row += "  " + head + strings.Repeat(" ", max(16-len(names[c]), 0))
	}
	lines = append(lines, row)
	for i := range height {
		row = ""
		for c := range len(names) {
			cell := ""
			if i < len(cols[c]) {
				v := cols[c][i]
				mark, style := "○ ", dimmed
				if v == m.pick[c] || (c == 2 && m.pick[c] == "" && v == "-") {
					mark, style = "● ", lipgloss.NewStyle()
					if c == m.pickCol {
						mark, style = hot.Render("● "), strong
					}
				}
				cell = mark + style.Render(m.pickLabel(c, v))
			}
			row += "  " + cell + strings.Repeat(" ", max(16-lipgloss.Width(cell), 0))
		}
		lines = append(lines, row)
	}
	return lines
}

func (m model) popupTitle() string {
	action := m.pending.action
	title := strings.ToUpper(action.Label[:1]) + action.Label[1:]
	if action.Label == "new" {
		title = "New worktree"
	}
	title = strings.Replace(title, "{tab}", m.tabWord(), 1)
	if action.NeedsRow() {
		title += " · " + nameOf(m.pending.row)
	}
	return title
}

func made(chosen Chosen, output string) string {
	if chosen.Action.Verb != "new" {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	name, _, ok := strings.Cut(first, " — ")
	if !ok || strings.ContainsAny(name, " \t") {
		return ""
	}
	return name
}

func (m model) answersOption() bool {
	for _, option := range m.pending.row.Options {
		if strings.HasPrefix(option, m.input+" ") && m.input != "" {
			return true
		}
	}
	return false
}

func (m model) draftKey(row panel.Row) string {
	action := m.pending.action
	if !action.NeedsRow() {
		return action.Input + ":"
	}
	who := ""
	if slices.Contains(action.Args, "{tab}") {
		who = m.selected().Name
	}
	return action.Input + ":" + row.TargetID() + "/" + who
}

func (m *model) keepDraft() {
	if !m.pending.action.Lines {
		return
	}
	if m.hooks.Drafts == nil {
		m.hooks.Drafts = map[string]string{}
	}
	key := m.draftKey(m.pending.row)
	if strings.TrimSpace(m.input) == "" {
		delete(m.hooks.Drafts, key)
	} else {
		m.hooks.Drafts[key] = m.input
	}
	if m.hooks.Keep != nil {
		m.hooks.Keep(m.hooks.Drafts)
	}
}

func (m model) hasDraft(row panel.Row) bool {
	for key := range m.hooks.Drafts {
		if strings.Contains(key, ":"+row.TargetID()+"/") {
			return true
		}
	}
	return false
}

func (m model) composer() []string {
	head := m.pending.action.Label
	head = strings.Replace(head, "{tab}", m.tabWord(), 1)
	if m.pending.action.NeedsRow() {
		head += " · " + nameOf(m.pending.row)
	}
	switch {
	case m.choice != "" && len(m.choices()) > 0:
		head += "  →  " + m.choice
	}
	if m.hooks.Do != "" {
		switch {
		case m.choice != "" && len(m.choices()) > 0:
			head = "→  " + m.choice
		default:
			head = ""
		}
	}
	lines := []string{""}
	if head != "" {
		lines = append(lines, dimmed.Render("  "+head), "")
	}
	i := m.attention()
	if i >= 0 && m.pending.action.NeedsRow() && m.rows[i].ID == m.pending.row.ID {
		pane := m.paneOf(i, m.inner()-6, 12)
		for _, line := range lastOf(pane[min(4, len(pane)):], 8) {
			lines = append(lines, rail.Render("  │ ")+line)
		}
		lines = append(lines, "")
	}
	if opts := m.pending.row.Options; len(opts) > 0 {
		lines = append(lines, dimmed.Render("  a digit picks, ⏎ presses it: ")+strings.Join(opts, "   "), "")
	}
	for _, line := range strings.Split(m.input+"█", "\n") {
		for _, part := range wrap(line, max(m.inner()-8, 20)) {
			lines = append(lines, hot.Render("  ┃ ")+part)
		}
	}
	if m.problem != "" {
		lines = append(lines, "")
		for _, part := range wrap(m.problem, max(m.inner()-8, 20)) {
			lines = append(lines, problem.Render("  "+part))
		}
	}
	return lines
}

var needsYou = map[string]bool{panel.GlyphWaiting: true, panel.GlyphUnseen: true, panel.GlyphDirty: true}

var tabNeedsYou = map[string]bool{panel.GlyphWaiting: true, panel.GlyphUnseen: true}

func (m model) nextNeed(after int, wanted map[string]bool) int {
	for step := 1; step <= len(m.rows); step++ {
		i := (after + step) % len(m.rows)
		if i >= 0 && wanted[m.rows[i].Glyph] {
			return i
		}
	}
	return -1
}

func (m model) attention() int {
	if m.panel.ID != "dashboard" && m.panel.ID != "" {
		if m.cursor < len(m.rows) {
			return m.cursor
		}
		return -1
	}
	for i, row := range m.rows {
		if row.Glyph == panel.GlyphWaiting {
			return i
		}
	}
	if m.cursor < len(m.rows) {
		return m.cursor
	}
	return -1
}

func (m model) listWidth() int {
	w := 4
	for _, width := range m.rawColumns() {
		w += width + 2
	}
	note := 0
	for _, row := range m.panel.Rows() {
		if row.Note != "" {
			note = max(note, lipgloss.Width(row.Note)+2)
		}
	}
	if len(m.panel.Rows()) == 0 {
		w = max(w, lipgloss.Width(m.panel.Empty)+4)
	}
	if m.mode == typing && m.pending != nil && m.pending.action.Lines {
		w = max(w, 80)
	}
	if m.mode == reporting {
		for _, line := range m.report {
			w = max(w, lipgloss.Width(line)+10)
		}
	}
	return max(w+note, 28)
}

func (m model) sideWidth() int {
	return max(min(m.inner()-m.listWidth()-5, 64), 40)
}

func (m model) tabs() []string {
	return m.tabsOf(m.attention())
}

func (m model) tabsOf(i int) []string {
	if i < 0 {
		return nil
	}
	row := m.rows[i]
	var tabs []string
	for _, one := range row.Tabs {
		tabs = append(tabs, one.Label())
	}
	if len(tabs) == 0 && (len(row.Preview) > 0 || (i == m.cursor && m.liveFor == row.ID)) {
		if m.panel.ID == "dashboard" {
			tabs = append(tabs, "preview")
		} else {
			tabs = append(tabs, "screen")
		}
	}
	tabs = append(tabs, "facts")
	if len(row.Blocks) > 0 {
		tabs = append(tabs, "blocks")
	}
	return tabs
}

func (m model) scrolled(down bool) int {
	i := m.attention()
	if m.details {
		i = m.cursor
	}
	if i < 0 || i >= len(m.rows) {
		return 0
	}
	row := m.rows[i]
	length := len(row.Preview)
	for _, one := range row.Tabs {
		length = max(length, len(one.Preview))
	}
	if i == m.cursor && m.liveFor == row.ID {
		length = max(length, len(m.live))
	}
	step := max(m.bodyHeight()/2, 1)
	if down {
		step = -step
	}
	return min(max(m.scroll+step, 0), length)
}

func (m model) sidePane(width, height int) []string {
	return m.paneOf(m.attention(), width, height)
}

func (m model) paneOf(i, width, height int) []string {
	if i < 0 || i >= len(m.rows) {
		return nil
	}
	row := m.rows[i]
	tone := dimmed
	if row.Glyph == panel.GlyphWaiting {
		tone = hot
	}
	head := strong.Render(nameOf(row))
	if row.Note != "" {
		head += "  " + tone.Render(row.Note)
	}
	tabs := m.tabsOf(i)
	current := min(m.tab, max(len(tabs)-1, 0))
	strip := ""
	for j, tab := range tabs {
		if j > 0 {
			strip += dimmed.Render(" · ")
		}
		if j == current {
			strip += strong.Render(tab)
		} else {
			strip += dimmed.Render(tab)
		}
	}
	lines := []string{"", fit(head, width), fit(strip, width), ""}
	switch tab := tabs[current]; tab {
	case "facts":
		if len(row.Facts) == 0 && len(row.Cells) > 1 {
			lines = append(lines, wrap(strings.Join(row.Cells[1:], "  "), width)...)
		}
		for _, fact := range row.Facts {
			label, value, _ := strings.Cut(fact, "  ")
			for i, part := range wrap(strings.TrimSpace(value), max(width-8, 10)) {
				if i > 0 {
					label = ""
				}
				lines = append(lines, fit(dimmed.Render(fmt.Sprintf("%-7s", label))+" "+part, width))
			}
		}
	case "blocks":
		for _, block := range row.Blocks {
			name, rest, _ := strings.Cut(block, "  ")
			command, quiet, _ := strings.Cut(rest, "  ")
			lines = append(lines, fit(strong.Render(fmt.Sprintf("%-16s", name))+" "+fmt.Sprintf("%-10s", command)+" "+dimmed.Render(quiet), width))
		}
		lines = append(lines, "", dimmed.Render("t opens them"))
	default:
		preview := row.Preview
		for _, one := range row.Tabs {
			if one.Label() == tab {
				preview = one.Preview
				lines[1] = fit(strong.Render(nameOf(row))+"  "+tone.Render(one.Note), width)
			}
		}
		if i == m.cursor && m.liveFor == row.ID && (m.selected().Label() == tab || len(row.Tabs) == 0) && len(m.live) > 0 {
			preview = m.live
		}
		room := max(height-len(lines), 1)
		end := max(len(preview)-m.scroll, min(room, len(preview)))
		shown := preview[max(end-room, 0):end]
		if end < len(preview) {
			shown = append(shown[min(1, len(shown)):], dimmed.Render(fmt.Sprintf("↓ %d newer · J", len(preview)-end)))
		}
		for _, line := range shown {
			lines = append(lines, fit(line, width))
		}
	}
	return lines
}

func (m model) choices() []string {
	if m.pending == nil {
		return nil
	}
	for id, action := range m.panel.Actions {
		if action.Key == m.pending.action.Key && action.Label == m.pending.action.Label {
			if choices := m.pending.row.Choices[id]; len(choices) > 0 {
				return choices
			}
		}
	}
	return m.pending.action.Choices
}

func nameOf(row panel.Row) string {
	if len(row.Cells) == 0 {
		return row.ID
	}
	return strings.TrimLeft(row.Cells[0], "▸▾│└ ")
}

func (m model) tabWord() string {
	if who := m.selected().Name; who != "" {
		return who
	}
	return "the tab"
}

func (m model) selectedTab() string {
	return m.selected().Name
}

func (m model) selected() panel.Tab {
	i := m.attention()
	if i < 0 || i != m.cursor {
		return panel.Tab{}
	}
	tabs := m.tabs()
	current := min(m.tab, len(tabs)-1)
	if current < 0 {
		return panel.Tab{}
	}
	for _, one := range m.rows[i].Tabs {
		if one.Label() == tabs[current] {
			return one
		}
	}
	return panel.Tab{}
}

func (m model) argvFor(action panel.Action, row panel.Row) []string {
	who := m.selected()
	target := row.TargetID()
	if who.Worktree != "" && slices.Contains(action.Args, "{tab}") {
		target = who.Worktree
	}
	return action.Argv(target, strings.TrimSpace(m.input), who.Name, m.choice)
}

func (m model) keepCursorVisible() model {
	_, cursorLine := m.body()
	if cursorLine < 0 {
		m.offset = 0
		return m
	}
	visible := m.bodyHeight()
	lines, _ := m.body()
	margin := 0
	if len(lines) > visible {
		margin = 1
	}
	if cursorLine < m.offset+margin {
		m.offset = max(cursorLine-margin, 0)
	}
	if m.offset == 1 {
		m.offset = 0
	}
	if cursorLine >= m.offset+visible-margin {
		m.offset = min(cursorLine-visible+1+margin, max(len(lines)-visible, 0))
	}
	return m
}

func (m model) rawColumns() []int {
	widths := cellWidths(m.everyRow())
	for i := range widths {
		widths[i] = min(widths[i], 32)
	}
	return widths
}

func (m model) unfiltered() []panel.Row {
	m.query = ""
	return m.visibleRows()
}

func (m model) everyRow() []panel.Row {
	m.query = ""
	open := map[string]bool{}
	for _, section := range m.panel.Sections {
		open[section.ID] = true
	}
	m.expanded = open
	return m.visibleRows()
}

func cellWidths(rows []panel.Row) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row.Cells {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], lipgloss.Width(cell))
		}
	}
	return widths
}

func (m model) columns() []int {
	widths := m.rawColumns()
	note := 0
	for _, row := range m.rows {
		if row.Note != "" {
			note = max(note, lipgloss.Width(row.Note)+2)
		}
	}
	if len(widths) < 2 {
		return widths
	}
	avail := m.width - 4 - 6 - 2
	if m.side() {
		avail -= m.sideWidth() + 5
	}
	total := 4 + note
	for _, w := range widths {
		total += w + 2
	}
	over := total - avail
	for _, shrink := range []struct{ col, floor int }{{5, 0}, {4, 0}, {1, 24}, {0, 18}} {
		if over <= 0 || shrink.col >= len(widths) {
			continue
		}
		give := min(over, widths[shrink.col]-shrink.floor)
		if give > 0 {
			widths[shrink.col] -= give
			over -= give
		}
	}
	return widths
}

func (m model) body() (lines []string, cursorLine int) {
	cursorLine = -1
	if len(m.panel.Rows()) == 0 {
		return []string{dimmed.Render("  " + m.panel.Empty)}, -1
	}
	widths := m.columns()
	lines = append(lines, "")
	index := 0
	for _, section := range m.panel.Sections {
		rows := m.rowsOf(section)
		if m.query != "" && len(rows) == 0 {
			continue
		}
		folded := section.Collapsed && !m.expanded[section.ID] && m.query == ""
		shown := len(section.Rows)
		if m.query != "" {
			shown = 0
			for _, row := range section.Rows {
				if m.matches(row) {
					shown++
				}
			}
		}
		lines = append(lines, heading(section, folded, shown))
		if folded {
			lines = append(lines, "")
			continue
		}
		for _, row := range rows {
			selected := index == m.cursor
			if selected {
				cursorLine = len(lines)
			}
			lines = append(lines, m.renderRow(row, widths, selected))
			if selected && !m.side() && m.liveFor == row.ID && len(m.live) > 0 {
				for _, line := range lastOf(m.live, 4) {
					lines = append(lines, fit(rail.Render("      │ ")+line, m.inner()))
				}
			}
			index++
		}
		lines = append(lines, "")
	}
	if m.query != "" && len(m.rows) == 0 {
		lines = append(lines, dimmed.Render("  nothing matches "+m.query+" — esc clears the filter"))
	}
	return lines, cursorLine
}

func heading(section panel.Section, folded bool, shown int) string {
	mark := "  "
	if folded {
		mark = "▸ "
	}
	return dimmed.Render(fmt.Sprintf("  %s%s  %d", mark, section.Label, shown))
}

const widest = 160

func (m model) boxWidth() int {
	if m.fill {
		return max(m.width, 24)
	}
	want := m.listWidth() + 6
	if m.side() {
		want += 5 + 64
	}
	if m.problem != "" {
		want = max(want, lipgloss.Width(m.problem)+16)
	}
	return max(min(min(m.width-4, widest), want), 24)
}

func (m model) inner() int { return m.boxWidth() - 6 }

const tallest = 30

func (m model) bodyHeight() int {
	below := len(m.problemLines())
	if m.fill {
		return max(m.height-2-below, 3)
	}
	return max(min(m.height-4, tallest)-below, 3)
}

func (m model) problemLines() []string {
	room := m.boxWidth() - 8
	if m.problem == "" || m.mode == typing || lipgloss.Width(m.problem)+2 <= room {
		return nil
	}
	return wrap(m.problem, room)
}

func (m model) View() string {
	lines, _ := m.body()
	if m.loading() {
		lines = m.loadingMark()
	}
	if m.mode == helping {
		lines = m.help()
	}
	if m.mode == reporting {
		lines = []string{""}
		for _, line := range m.report {
			for i, part := range wrap(strings.ReplaceAll(line, "\t", "    "), max(m.inner()-6, 20)) {
				if i > 0 {
					part = "    " + part
				}
				lines = append(lines, "  "+part)
			}
		}
	}
	if m.mode == typing && m.pending.action.Lines {
		lines = m.composer()
		if m.picking {
			lines = m.picker()
		}
	}
	if m.details && m.mode == browsing {
		lines = nil
		for _, line := range m.paneOf(m.cursor, m.inner()-4, m.bodyHeight()) {
			lines = append(lines, "  "+line)
		}
	}
	visible := m.bodyHeight()
	start := min(m.offset, max(len(lines)-1, 0))
	if m.mode == helping {
		start = min(m.scroll, max(len(lines)-visible, 0))
	}
	if m.mode == reporting {
		start = 0
	}
	end := min(start+visible, len(lines))
	rows := lines[start:end]
	height := len(rows)
	if len(lines) > visible || m.mode == helping {
		height = visible
	}
	height = max(height, 3)
	if m.fill {
		height = visible
	}
	side := m.side() && m.mode != helping && m.mode != reporting && !(m.details && m.mode == browsing)
	inner := m.inner()
	leftW := inner - 2
	var right []string
	if side {
		leftW = inner - m.sideWidth() - 5
		height = min(max(height, 18), visible)
		right = m.sidePane(m.sideWidth(), height)
	}
	scroll := len(lines) > height && m.mode == browsing
	thumbAt, thumbLen := 0, height
	if scroll {
		thumbLen = max(height*height/len(lines), 1)
		thumbAt = min(start*height/len(lines), height-thumbLen)
	}
	if scroll && start > 1 {
		rows[0] = fit(dimmed.Render(fmt.Sprintf("  ↑ %d more", start-1)), leftW)
	}
	if scroll && end < len(lines) {
		rows[len(rows)-1] = fit(dimmed.Render(fmt.Sprintf("  ↓ %d more", len(lines)-end)), leftW)
	}
	var b strings.Builder
	b.WriteString(m.frameTop() + "\n")
	for i := 0; i < height; i++ {
		line := ""
		if i < len(rows) {
			line = fit(rows[i], leftW)
		}
		if pad := leftW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		bar := " "
		if scroll {
			bar = dimmed.Render("╎")
			if i >= thumbAt && i < thumbAt+thumbLen {
				bar = strong.Render("┃")
			}
		}
		if side {
			extra := ""
			if i < len(right) {
				extra = right[i]
			}
			if pad := m.sideWidth() - lipgloss.Width(extra); pad > 0 {
				extra += strings.Repeat(" ", pad)
			}
			line += " " + bar + " " + dimmed.Render("│") + "  " + extra
		} else {
			line += " " + bar + " "
		}
		b.WriteString(dimmed.Render("│") + "  " + line + " " + dimmed.Render("│") + "\n")
	}
	for _, part := range m.problemLines() {
		line := "    " + problem.Render(part)
		if pad := m.boxWidth() - 2 - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		b.WriteString(dimmed.Render("│") + line + dimmed.Render("│") + "\n")
	}
	b.WriteString(m.frameBottom())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, b.String())
}

func (m model) frameTop() string {
	name, tail, _ := strings.Cut(m.panel.Title, "  —  ")
	if m.hooks.Do != "" && m.pending != nil {
		name = m.popupTitle()
	} else if m.doTitle != "" {
		name = m.doTitle
	}
	left := dimmed.Render("╭── ") + strong.Render(name) + " "
	if m.query != "" {
		left += dimmed.Render("/ ") + m.query + " "
	}
	right := ""
	if tail != "" {
		right = " " + hot.Render(tail) + " "
	}
	fill := m.boxWidth() - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if fill < 0 {
		right, fill = "", m.boxWidth()-lipgloss.Width(left)-2
	}
	return left + dimmed.Render(strings.Repeat("─", max(fill, 0))) + right + dimmed.Render("─╮")
}

func (m model) frameBottom() string {
	footer := strings.TrimLeft(m.footer(), " ")
	left := dimmed.Render("╰── ") + footer + " "
	if footer == "" {
		left = dimmed.Render("╰")
	}
	fill := m.boxWidth() - lipgloss.Width(left) - 1
	if fill < 0 {
		left = dimmed.Render("╰── ") + fit(footer, m.boxWidth()-6) + " "
		fill = m.boxWidth() - lipgloss.Width(left) - 1
	}
	return left + dimmed.Render(strings.Repeat("─", max(fill, 0))+"╯")
}

func (m model) footer() string {
	switch m.mode {
	case confirming:
		text := strings.ReplaceAll(m.pending.action.ConfirmFor(nameOf(m.pending.row)), "{tab}", m.tabWord())
		text = strings.ReplaceAll(text, "{input}", strings.TrimSpace(m.input))
		return hot.Render("  "+text) + "  " + keyCap.Render("y") + dimmed.Render(" yes  ") + keyCap.Render("n") + dimmed.Render(" no")
	case typing:
		if m.picking {
			return "  " + keycap("←→ column") + "   " + keycap("↑↓ choose") + "   " + keycap("⏎ apply") + "   " + keycap("esc back")
		}
		if m.pending.action.Lines {
			line := "  " + keycap("^s send") + "   " + keycap("⏎ newline")
			if len(m.choices()) > 1 {
				line += "   " + keycap("⇥ "+m.pending.action.Input)
			}
			return line + "   " + keycap("esc keep draft")
		}
		line := hot.Render("  "+m.pending.action.Input+": ") + m.input + "█"
		if choices := m.choices(); len(choices) > 0 {
			var offered []string
			for i, choice := range choices {
				offered = append(offered, keyCap.Render(fmt.Sprint(i+1))+" "+choice)
			}
			line += "   " + strings.Join(offered, "  ") + dimmed.Render("   ⇥ next")
		}
		if m.problem != "" {
			line += "   " + problem.Render(strings.TrimPrefix(m.problem, "mia: "))
		}
		return line
	case filtering:
		if m.hooks.Switch {
			return hot.Render("  › ") + m.query + "█" + "   " + keycap("⏎ switch") + "   " + keycap("↑↓ move") + "   " + keycap("esc stay")
		}
		return hot.Render("  / ") + m.query + "█" + dimmed.Render("   ⏎ keep · esc clear")
	case helping:
		return "  " + keycap("? hide") + "   " + keycap("j k scroll")
	case working:
		if m.loading() {
			return ""
		}
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		line := "  " + hot.Render(frames[m.spin%len(frames)]) + " " + dimmed.Render(m.label)
		if strings.HasPrefix(m.label, "mia ") {
			line += "   " + keycap("esc leave it running")
		}
		return line
	case reporting:
		if m.hooks.Do != "" && m.made != "" {
			return "  " + keycap("⏎ attach") + "   " + keycap("esc close")
		}
		if m.hooks.Do != "" {
			return "  " + keycap("⏎ close")
		}
		return "  " + keyCap.Render("⏎") + dimmed.Render(" back")
	}
	shown := m.problem != "" && m.problemLines() == nil
	if len(m.stack) > 0 && !shown {
		return "  " + keycap("q back") + "   " + keycap("? keys")
	}
	if shown {
		return problem.Render("  " + m.problem)
	}
	if m.notice != "" {
		return good.Render("  "+m.notice) + "   " + keycap("? keys")
	}
	return "  " + strings.Join(m.hints(), "   ")
}

func (m model) hints() []string {
	var hints []string
	if m.cursor < len(m.rows) {
		row := m.rows[m.cursor]
		if len(row.Children) > 0 {
			word := "unfold"
			if m.isOpen(row) {
				word = "fold"
			}
			hints = append(hints, keycap("⏎ "+word))
		} else {
			for _, id := range []string{"open", "adopt", "claim", "go", "shell"} {
				if action, ok := m.panel.Actions[id]; ok && offers(row, id) && action.Key != "" {
					label := id
					if words := strings.Fields(action.Label); len(words) > 0 {
						label = words[0]
					}
					hints = append(hints, keycap(action.Key+" "+label))
					break
				}
			}
		}
	}
	if action, ok := m.panel.Actions["new"]; ok && action.Key != "" && !action.NeedsRow() {
		hints = append(hints, keycap(action.Key+" new"))
	}
	if len(m.rows) > 0 {
		hints = append(hints, keycap(m.shown("filter")+" find"))
	}
	return append(hints, keycap("? keys"))
}

func keycap(hint string) string {
	key, label, ok := strings.Cut(hint, " ")
	if !ok {
		return dimmed.Render(hint)
	}
	return keyCap.Render(key) + " " + dimmed.Render(label)
}

func (m model) help() []string {
	var row panel.Row
	if m.cursor < len(m.rows) {
		row = m.rows[m.cursor]
	}
	entry := func(key, what, more string) string {
		line := "  " + keyCap.Render(fmt.Sprintf("%-7s", key)) + fmt.Sprintf("%-28s", fit(what, 28))
		if more != "" {
			line += " " + dimmed.Render(more)
		}
		return line
	}
	type item struct {
		rank int
		key  string
		line string
	}
	var global, onRow []item
	for id, action := range m.panel.Actions {
		line := entry(action.Key, action.Label, action.CLI())
		if action.Help != "" {
			for _, part := range wrap(action.Help, max(m.inner()-13, 20)) {
				line += "\n         " + dimmed.Render(part)
			}
		}
		it := item{m.panel.Rank(id), action.Key, line}
		switch {
		case !action.NeedsRow():
			global = append(global, it)
		case offers(row, id):
			onRow = append(onRow, it)
		}
	}
	byRank := func(items []item) []string {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].rank != items[j].rank {
				return items[i].rank < items[j].rank
			}
			return items[i].key < items[j].key
		})
		var lines []string
		for _, it := range items {
			lines = append(lines, it.line)
		}
		return lines
	}
	lines := []string{"",
		dimmed.Render("  moving"),
		entry(m.shown("down", "up"), "move", m.shown("first", "last")+" first and last · "+m.shown("pagedown", "pageup")+" half a page"),
		entry(m.shown("filter"), "find", "by name or branch, into folded stacks · esc clears"),
		entry("space", "next", "the next row that needs you: uncommitted, or flagged by a plugin"),
		entry("K J", "scroll the side pane", "up and down the tab showing"),
		entry(m.shown("details"), "details", "the side pane full screen · i or esc returns"),
		entry(m.shown("tabprev", "tabnext"), "side tab", "or a digit · plugin tabs, facts, blocks"),
		entry("⏎ "+m.shown("unfold", "fold"), "unfold, fold", "a stack row · "+m.shown("fold")+" on a layer returns to its stack"),
		entry(m.shown("popup"), "popup", "the tab's thing in a tmux popup over the list · prefix d returns"),
		entry(m.shown("refresh"), "refresh", "now; the list also refreshes itself"),
	}
	for _, section := range m.panel.Sections {
		if section.ToggleKey != "" {
			lines = append(lines, entry(section.ToggleKey, "show or hide "+section.Label, ""))
		}
	}
	lines = append(lines, entry(m.shown("back"), "back, or quit", ""), entry(m.shown("help"), "hide this", ""))
	if len(onRow) > 0 {
		lines = append(lines, "", dimmed.Render("  on ")+strong.Render(nameOf(row))+dimmed.Render("  —  key · what · the command underneath"))
		lines = append(lines, byRank(onRow)...)
	}
	if len(global) > 0 {
		lines = append(lines, "", dimmed.Render("  anywhere"))
		lines = append(lines, byRank(global)...)
	}
	var flat []string
	for _, line := range lines {
		flat = append(flat, strings.Split(line, "\n")...)
	}
	return flat
}

func lastOf(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func fit(line string, width int) string {
	line = strings.ReplaceAll(line, "\t", "    ")
	if width <= 1 || lipgloss.Width(line) <= width {
		return line
	}
	runes := []rune(line)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func (m model) renderRow(row panel.Row, widths []int, selected bool) string {
	glyph := row.Glyph
	if glyph == "" {
		glyph = " "
	}
	inner := m.inner() - 2
	if m.side() {
		inner = m.inner() - m.sideWidth() - 5
	}
	cells := make([]string, len(row.Cells))
	for i, cell := range row.Cells {
		if i < len(widths) {
			if strings.TrimSpace(cell) == "" {
				cell = strings.Repeat(" ", widths[i])
			} else {
				cell = fit(cell, widths[i])
				cell += strings.Repeat(" ", max(widths[i]-lipgloss.Width(cell), 0))
			}
		}
		cells[i] = cell
	}
	plain := "  " + glyph + " " + strings.Join(cells, "  ")
	note := row.Note
	if slices.Contains(row.Cells, note) {
		note = ""
	}
	if m.hasDraft(row) {
		note = strings.TrimSpace(note + "  ✎ draft")
	}
	room := inner - lipgloss.Width(plain) - lipgloss.Width(note)
	if note != "" && room < 2 {
		note = ""
	}
	if selected {
		line := plain
		if note != "" {
			line += strings.Repeat(" ", room) + note
		}
		line = fit(line, inner)
		if pad := inner - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return bar.Render(line)
	}
	if row.Dim {
		line := plain
		if note != "" {
			line += strings.Repeat(" ", room) + note
		}
		return fit(dimmed.Render(line), inner)
	}
	tone := tones[row.Glyph]
	name := strong.Render(cells[0])
	if runes := []rune(cells[0]); row.Depth > 0 && len(runes) > 2*row.Depth+1 {
		n := 2*row.Depth + 1
		name = dimmed.Render(string(runes[:n])) + string(runes[n:])
	}
	styled := "  " + tone.Render(glyph) + " " + name
	if len(cells) > 1 {
		styled += "  " + cells[1]
	}
	for _, cell := range cells[min(2, len(cells)):] {
		styled += "  " + dimmed.Render(cell)
	}
	if note != "" {
		styled += strings.Repeat(" ", room) + dimmed.Render(note)
	}
	return fit(styled, inner)
}
func RenderAt(p panel.Panel, cursor, height int) string {
	m := newModel(p)
	m.cursor, m.width, m.height = cursor, 96, height
	return m.keepCursorVisible().View()
}
