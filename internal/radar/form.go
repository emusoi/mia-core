package radar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/emusoi/mia-core/internal/mark"
)

const (
	inPlace  = "a layer, in place"
	ownTree  = "a layer, own worktree"
	thisHere = "local"
)

// form is the new-worktree popup: the branch first, then where it starts,
// what it is and where it runs, each one ⇥ away; then the making of it.
type form struct {
	branch string
	focus  int // 0 the branch, 1 from, 2 as, 3 on
	from   []string
	fromAt int
	as     []string
	asAt   int
	on     []string
	onAt   int
	down   map[string]bool // machines that did not answer last

	started bool
	began   time.Time
	ticks   int
	showLog bool
	seen    map[string]time.Time // when each step was first seen running, and done
	log     string
	lines   []string
	done    bool
	err     error
	made    string
}

func newForm(data Data) form {
	f := form{branch: data.Prefix, from: []string{data.Base}, as: []string{inPlace, ownTree}, on: []string{thisHere}, down: map[string]bool{}}
	for _, state := range data.Machines[min(1, len(data.Machines)):] {
		f.on = append(f.on, state.Name)
		f.down[state.Name] = !state.Answers
	}
	if data.Here != "" && data.Branch != "" && data.Branch != data.Base {
		f.from = append(f.from, data.Branch)
		for _, l := range data.Local {
			if l.Name == data.Here && len(l.Layers) > 1 {
				f.fromAt = 1 // inside a stack, the next thing is usually a layer on it
			}
		}
	}
	return f
}

func (f form) layered() bool { return f.fromAt > 0 }

// offers says whether a line can be chosen on now: from when there is more
// than one place to start, as for a layer, on for a worktree of its own.
func (f form) offers(line int) bool {
	switch line {
	case 1:
		return len(f.from) > 1
	case 2:
		return f.layered()
	case 3:
		return !f.layered() && len(f.on) > 1
	}
	return true
}

// move is the next line that can be chosen on, step away.
func (f form) move(step int) int {
	for i := 1; i <= 4; i++ {
		next := (f.focus + step*i + 8) % 4
		if f.offers(next) {
			return next
		}
	}
	return 0
}

// argv is the mia command the popup makes, and the directory it runs in.
func (f form) argv(data Data) ([]string, string) {
	branch := strings.TrimSpace(f.branch)
	if f.layered() {
		if f.as[f.asAt] == ownTree {
			return []string{"new", "--stack", "--worktree", branch}, data.HereDir
		}
		return []string{"new", "--stack", branch}, data.HereDir
	}
	if on := f.on[f.onAt]; on != thisHere {
		return []string{"new", "--on", on, branch}, data.Root
	}
	return []string{"new", branch}, data.Root
}

func (m model) formKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.form
	if f.started {
		switch msg.String() {
		case "l":
			f.showLog = !f.showLog
			return m, nil
		case "esc", "q":
			return m, tea.Quit // what is being made keeps going
		case "enter":
			if f.done && f.err == nil && f.made != "" {
				if strings.Contains(f.made, "@") {
					m.chosen = &Choice{Remote: f.made}
				} else {
					m.chosen = &Choice{Name: f.made}
				}
				return m, tea.Quit
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "tab", "down":
		f.focus = f.move(1)
	case "shift+tab", "up":
		f.focus = f.move(-1)
	case "left", "right":
		if f.focus == 0 {
			break
		}
		step := 1
		if msg.String() == "left" {
			step = -1
		}
		switch f.focus {
		case 1:
			f.fromAt = (f.fromAt + step + len(f.from)) % len(f.from)
		case 2:
			f.asAt = (f.asAt + step + len(f.as)) % len(f.as)
		case 3:
			f.onAt = (f.onAt + step + len(f.on)) % len(f.on)
		}
	case "backspace":
		if f.focus == 0 && f.branch != "" {
			runes := []rune(f.branch)
			f.branch = string(runes[:len(runes)-1])
		}
	case "enter":
		if strings.TrimSpace(f.branch) == "" || strings.HasSuffix(f.branch, "/") {
			return m, nil
		}
		return m.start()
	default:
		if msg.Type == tea.KeyRunes {
			f.focus = 0 // typing always goes to the branch
			f.branch += string(msg.Runes)
		}
	}
	return m, nil
}

type tick struct{}
type finished struct{ err error }

// start makes the worktree in a process of its own that writes to a log
// the popup follows, so leaving the popup leaves it running.
func (m model) start() (tea.Model, tea.Cmd) {
	f := &m.form
	if m.data.First != nil {
		if err := writeConfig(m.data.MiaDir, *m.data.First); err != nil {
			f.started, f.done, f.err = true, true, err
			return m, nil
		}
	}
	self, err := os.Executable()
	if err != nil {
		f.started, f.done, f.err = true, true, err
		return m, nil
	}
	argv, dir := f.argv(m.data)
	logFile, err := os.CreateTemp("", "mia-new-*.log")
	if err != nil {
		f.started, f.done, f.err = true, true, err
		return m, nil
	}
	cmd := exec.Command(self, argv...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		f.started, f.done, f.err = true, true, err
		return m, nil
	}
	f.started, f.log, f.began, f.seen = true, logFile.Name(), time.Now(), map[string]time.Time{}
	wait := func() tea.Msg {
		err := cmd.Wait()
		logFile.Close()
		return finished{err}
	}
	return m, tea.Batch(wait, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tick{} }))
}

func (m model) follow(msg tea.Msg) (model, tea.Cmd) {
	f := &m.form
	switch msg := msg.(type) {
	case tick:
		f.ticks++
		if data, err := os.ReadFile(f.log); err == nil {
			f.lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		if !f.done {
			return m, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tick{} })
		}
	case finished:
		if data, err := os.ReadFile(f.log); err == nil {
			f.lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		}
		os.Remove(f.log)
		f.done, f.err = true, msg.err
		f.made = madeName(f.lines)
	}
	return m, nil
}

// madeName is the last "name — " line mia new printed: name, or name@machine
// for one made on a machine.
func madeName(lines []string) string {
	made := ""
	for _, line := range lines {
		if name, _, ok := strings.Cut(strings.TrimSpace(line), " — "); ok && name != "" && !strings.ContainsAny(name, " \t") {
			made = name
		}
	}
	return made
}

func (m model) formView() string {
	f := m.form
	width := max(m.width-2, 40)
	var b strings.Builder
	if f.started {
		return m.makingView(width)
	}
	b.WriteString(italic.Render("branch") + "\n")
	cursor := ""
	if f.focus == 0 {
		cursor = accent.Render("█")
	}
	prompt := "  "
	if f.focus == 0 {
		prompt = accent.Render("› ")
	}
	b.WriteString(prompt + inkB.Render(f.branch) + cursor + "\n\n")
	choices := func(line int, label string, options []string, at int, note string) {
		marker := "  "
		if f.focus == line {
			marker = accent.Render("› ")
		}
		text := marker + muted.Render(pad(label, 8))
		if !f.offers(line) && note != "" {
			b.WriteString(text + faint.Render(note) + "\n")
			return
		}
		for i, option := range options {
			shown := option
			if f.down[option] {
				shown = "○ " + option
			}
			switch {
			case i == at && f.focus == line:
				text += accent.Bold(true).Render("["+shown+"]") + "  "
			case i == at:
				text += inkB.Render(" "+shown+" ") + "  "
			case f.down[option]:
				text += Style("red").Render(" "+shown+" ") + "  "
			default:
				text += soft.Render(" "+shown+" ") + "  "
			}
		}
		b.WriteString(text + "\n")
	}
	choices(1, "from", f.from, f.fromAt, "")
	if f.layered() {
		choices(2, "as", f.as, f.asAt, "")
		choices(3, "on", nil, 0, "local — a layer lives with its stack")
	} else {
		choices(2, "as", nil, 0, "a new worktree")
		choices(3, "on", f.on, f.onAt, "local")
	}
	b.WriteString("\n" + faint.Render("  "+f.explain(m.data)) + "\n")
	if m.data.First != nil {
		g := m.data.First
		setup := "none found"
		if len(g.Setup) > 0 {
			setup = strings.Join(g.Setup, " ")
		}
		b.WriteString("\n" + accent.Render("first time in "+m.data.Repo+"  ") + muted.Render(fmt.Sprintf("base %s  ·  setup %s", g.Base, setup)) + "\n")
		b.WriteString(faint.Render("  guessed from the repository; saved as its config when the worktree is made — mia config changes it") + "\n")
	}
	for used := strings.Count(b.String(), "\n"); used < m.height-2; used++ {
		b.WriteString("\n")
	}
	b.WriteString(keys("⏎", "make it") + keys("↑↓", "a line") + keys("←→", "choose") + keys("esc", "cancel"))
	return b.String()
}

func (f form) explain(data Data) string {
	switch {
	case f.layered() && f.as[f.asAt] == ownTree:
		return "a layer on " + f.from[f.fromAt] + ", in a worktree of its own"
	case f.layered():
		return "a layer on " + f.from[f.fromAt] + ", here, in this worktree and session"
	case f.on[f.onAt] != thisHere:
		return "a worktree on " + f.on[f.onAt] + ": its git, session and processes live there"
	}
	return "a new worktree from " + f.from[f.fromAt] + ", or the branch itself if it exists"
}

// writeConfig saves the first-time guesses as the repository's config.
func writeConfig(miaDir string, g Guess) error {
	path := filepath.Join(miaDir, "config.toml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	var b strings.Builder
	b.WriteString("# what mia guessed the first time it made a worktree here; `mia config` changes it\n")
	fmt.Fprintf(&b, "base = %q\n", g.Base)
	if len(g.Setup) > 0 {
		quoted := make([]string, len(g.Setup))
		for i, word := range g.Setup {
			quoted[i] = fmt.Sprintf("%q", word)
		}
		fmt.Fprintf(&b, "setup = [%s]\n", strings.Join(quoted, ", "))
	}
	if err := os.MkdirAll(miaDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// GuessSetup is the install command the repository's lockfile asks for.
func GuessSetup(root string) []string {
	for _, c := range []struct {
		file string
		argv []string
	}{
		{"pnpm-lock.yaml", []string{"pnpm", "install", "--frozen-lockfile"}},
		{"package-lock.json", []string{"npm", "ci"}},
		{"yarn.lock", []string{"yarn", "install"}},
		{"bun.lock", []string{"bun", "install"}},
		{"bun.lockb", []string{"bun", "install"}},
		{"uv.lock", []string{"uv", "sync"}},
		{"poetry.lock", []string{"poetry", "install"}},
		{"Gemfile.lock", []string{"bundle", "install"}},
	} {
		if _, err := os.Stat(filepath.Join(root, c.file)); err == nil {
			return c.argv
		}
	}
	return nil
}

type step struct {
	name, detail string
	state        string // wait, run, done, fail
	live         string // the last thing it printed, while it runs
}

// steps reads what mia new printed so far as the steps of making a
// worktree: shipping mia to a machine, the worktree, its setup, its session.
func (f form) steps() []step {
	var steps []step
	setupAt, setupWith, name := -1, "", madeName(f.lines)
	shipping := false
	for i, line := range f.lines {
		if strings.HasPrefix(line, "shipping this mia to") {
			shipping = true
		}
		if who, with, ok := strings.Cut(line, ": setting up with "); ok && setupAt < 0 {
			setupAt, setupWith = i, with
			if name == "" {
				name = who
			}
		}
	}
	failed := f.done && f.err != nil
	if shipping {
		state := "done"
		if setupAt < 0 && name == "" && !f.done {
			state = "run"
		}
		steps = append(steps, step{name: "ship mia", detail: "this mia, built for that machine", state: state})
	}
	worktree := step{name: "worktree", state: "run"}
	if setupAt >= 0 || name != "" {
		worktree.state, worktree.detail = "done", strings.Split(name, "@")[0]
	}
	steps = append(steps, worktree)
	if setupAt >= 0 {
		setup := step{name: "setup", detail: setupWith, state: "run"}
		if madeName(f.lines) != "" || (f.done && !failed) {
			setup.state = "done"
		}
		for i := len(f.lines) - 1; i > setupAt; i-- {
			if line := strings.TrimSpace(f.lines[i]); line != "" && !strings.Contains(line, " — ") {
				setup.live = line
				break
			}
		}
		steps = append(steps, setup)
	}
	session := step{name: "session", detail: "opens when you switch there", state: "wait"}
	if f.done && !failed {
		session.state, session.detail = "done", "ready — ⏎ opens it"
	}
	steps = append(steps, session)
	if failed {
		for i := range steps {
			if steps[i].state == "run" || (steps[i].state == "wait" && i == len(steps)-1 && worktree.state == "run") {
				steps[i].state = "fail"
				break
			}
		}
	}
	return steps
}

var spinner = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// makingView is the making of a worktree: mia's mark at work on the left,
// the steps on the right with how long each took, the keys on a line of
// their own.
func (m model) makingView(width int) string {
	f := m.form
	now := time.Now()
	steps := f.steps()
	for _, s := range steps {
		if _, ok := f.seen[s.name]; !ok && s.state != "wait" {
			f.seen[s.name] = now
		}
		if _, ok := f.seen[s.name+"/done"]; !ok && s.state == "done" {
			f.seen[s.name+"/done"] = now
		}
	}
	argv, _ := f.argv(m.data)
	where := "this computer"
	for i, a := range argv {
		if a == "--on" && i+1 < len(argv) {
			where = argv[i+1]
		}
	}
	var right []string
	right = append(right, muted.Render("making ")+inkB.Render(strings.TrimSpace(f.branch))+muted.Render("   from ")+ink.Render(f.from[f.fromAt])+"   "+m.colour(where).Render("◆ "+where), "")
	for _, s := range steps {
		glyph := ghost.Render("·")
		nameStyle, detailStyle := ghost, ghost
		switch s.state {
		case "done":
			glyph, nameStyle, detailStyle = Style("green").Render("✓"), inkB, ink
		case "run":
			glyph, nameStyle, detailStyle = accent.Render(spinner[f.ticks%len(spinner)]), inkB, ink
		case "fail":
			glyph, nameStyle, detailStyle = Style("red").Render("✗"), Style("red").Bold(true), Style("red")
		}
		took := ""
		if began, ok := f.seen[s.name]; ok && s.name != "session" {
			end, finished := f.seen[s.name+"/done"]
			if !finished {
				end = now
			}
			took = fmt.Sprintf("%.1fs", end.Sub(began).Seconds())
		}
		right = append(right, between(glyph+"  "+nameStyle.Render(pad(s.name, 11))+detailStyle.Render(s.detail), muted.Render(took), width-25))
		if s.state == "run" && s.live != "" {
			right = append(right, "   "+line_.Render("│ ")+ghost.Render(cut(s.live, width-32)))
		}
	}
	right = append(right, "")
	switch {
	case f.done && f.err != nil:
		right = append(right, Style("red").Render("✗ "+cut(f.err.Error(), width-28)))
	case f.done && f.made != "":
		right = append(right, Style("green").Render("✓ made ")+inkB.Render(f.made)+muted.Render(fmt.Sprintf("  —  ready in %.0fs", now.Sub(f.began).Seconds())))
	}
	grid := mark.Grid(float64(f.ticks) * 0.15)
	var b strings.Builder
	rows := max(len(grid), len(right))
	if f.showLog {
		rows = 0
		lines := f.lines
		if room := max(m.height-4, 4); len(lines) > room {
			lines = lines[len(lines)-room:]
		}
		for _, l := range lines {
			b.WriteString(muted.Render(cut(l, width)) + "\n")
		}
	}
	for i := 0; i < rows; i++ {
		left := strings.Repeat(" ", 19)
		if i < len(grid) {
			left = grid[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		b.WriteString(left + "    " + r + "\n")
	}
	for used := strings.Count(b.String(), "\n"); used < m.height-2; used++ {
		b.WriteString("\n")
	}
	b.WriteString(line_.Render(strings.Repeat("─", width)) + "\n")
	switch {
	case !f.done:
		b.WriteString(keys("esc", "back to your window — it keeps going") + keys("l", "the whole log"))
	case f.made != "" && f.err == nil:
		b.WriteString(keys("⏎", "switch there") + keys("l", "the whole log") + keys("esc", "close"))
	default:
		b.WriteString(keys("l", "the whole log") + keys("esc", "close"))
	}
	return b.String()
}
