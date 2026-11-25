package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
)

// The one list (design/tmux, version B): tmux's prefix k opens it over whatever is in front, and
// what is typed in it says what is wanted, each row what Enter then does.
//
//	nothing        who waits on you, who works, the worktrees of the project here
//	words          the worktrees and projects they mean, then the past conversations that mention them
//	agent words…   that agent started in the worktree here, told the words
//	> agent        the conversation of the window in front goes on with that agent
//	a number       that answer, given to whoever has waited longest

// listRow is one row: a mark, its words, what is said at the right, and what Enter does.
type listRow struct {
	mark  string
	asks  bool // its mark is the amber dot
	words string
	right string
	do    func(l listed) error
	into  string // what Enter types in place of doing anything: a project opened to its worktrees
}

// listed is what the list was opened over: the tmux client, its window, and the folder there.
type listed struct {
	self, client, window, folder string
}

// place is a worktree or a project to go to.
type place struct{ name, project, branch, path, root string }

// awayTree is a worktree of the project here that is on a machine.
type awayTree struct{ name, branch, machine string }

// layer is one branch of the stack the worktree here is on.
type layer struct {
	Branch       string `json:"branch"`
	Current      bool   `json:"current"`
	Ahead        int    `json:"ahead"`
	NeedsRestack bool   `json:"needs_restack"`
}

type listModel struct {
	at       listed
	here     string   // the project the folder is in, "" in none
	root     string   // and its main checkout
	tree     string   // the worktree the folder is, by its name
	main     bool     // and it is the main checkout, which stays where it is
	machines []string // the machines mia knows other than this one
	query    string
	cursor   int
	width    int
	height   int
	stack    []layer
	away     []awayTree // the worktrees of the project here that are on a machine
	places   []place
	stand    app.Standing
	agents   []agentKnown
	threads  []thread
	asked    string // what threads answers
	chosen   *listRow
}

type (
	placesMsg   []place
	standMsg    app.Standing
	agentsMsg   []agentKnown
	machinesMsg []string
	stackMsg    []layer
	awayMsg     []awayTree
	threadsMsg  struct {
		text  string
		found []thread
	}
	pausedMsg string
)

func (m listModel) Init() tea.Cmd {
	self := m.at.self
	return tea.Batch(
		func() tea.Msg { return placesMsg(everyPlace(m.root)) },
		func() tea.Msg {
			var s app.Standing
			_ = apiSaid(self, &s, "standing")
			slices.SortStableFunc(s.Waiting, func(x, y app.Stand) int { return x.Since.Compare(y.Since) })
			return standMsg(s)
		},
		func() tea.Msg {
			var agents []agentKnown
			_ = apiSaid(self, &agents, "agents")
			return agentsMsg(slices.DeleteFunc(agents, func(a agentKnown) bool { return !a.Installed }))
		},
		func() tea.Msg {
			var machines []struct {
				Name string `json:"name"`
				SSH  string `json:"ssh"`
			}
			_ = apiSaid(self, &machines, "machines")
			var names []string
			for _, machine := range machines {
				if machine.SSH != "" {
					names = append(names, machine.Name)
				}
			}
			return machinesMsg(names)
		},
		func() tea.Msg { return awayMsg(treesAway(m.root)) },
		func() tea.Msg {
			var layers []layer
			_ = apiSaid(self, &layers, "stack")
			return stackMsg(layers)
		},
	)
}

// treesAway is the worktrees of the repository at root that are on a machine: each machine's
// mia asked, three seconds given to it, in the folder the repository has there.
func treesAway(root string) []awayTree {
	a, err := app.Open(root)
	if err != nil {
		return nil
	}
	dir, err := a.RemoteDir()
	if err != nil {
		return nil
	}
	var away []awayTree
	for _, machine := range otherMachines() {
		out, err := runtime.RunWithin(3*time.Second, machine.SSH, "sh", "-c", `cd "$HOME/$1" 2>/dev/null && "$HOME/.local/bin/mia" api worktrees`, "sh", dir)
		var there []struct {
			Name   string `json:"name"`
			Branch string `json:"branch"`
			Main   bool   `json:"main"`
		}
		if err != nil || json.Unmarshal([]byte(out), &there) != nil {
			continue
		}
		for _, t := range there {
			if !t.Main {
				away = append(away, awayTree{t.Name, t.Branch, machine.Name})
			}
		}
	}
	return away
}

// everyPlace is every worktree of every project mia knows, and each project; the repository at
// root with them, when mia has named no worktree in it yet.
func everyPlace(root string) []place {
	repos, _ := app.Repositories()
	if root != "" && !slices.ContainsFunc(repos, func(r app.Repository) bool { return r.Path == root }) {
		repos = append(repos, app.Repository{Name: filepath.Base(root), Path: root})
	}
	var places []place
	for _, repo := range repos {
		a, err := app.Open(repo.Path)
		if err != nil {
			continue
		}
		resolver, err := a.Resolver()
		if err != nil {
			continue
		}
		for _, r := range resolver.Records {
			places = append(places, place{name: r.Name, project: repo.Name, branch: headBranch(r.Path), path: r.Path, root: repo.Path})
		}
		places = append(places, place{name: repo.Name, project: "project", path: repo.Path, root: repo.Path})
	}
	return places
}

func (m listModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case placesMsg:
		m.places = msg
	case standMsg:
		m.stand = app.Standing(msg)
	case agentsMsg:
		m.agents = msg
	case machinesMsg:
		m.machines = msg
	case stackMsg:
		m.stack = msg
	case awayMsg:
		m.away = msg
	case threadsMsg:
		if msg.text == m.words() {
			m.threads, m.asked = msg.found, msg.text
		}
	case pausedMsg: // typing stopped for a moment: the conversations that mention it are looked for
		text := string(msg)
		if text != m.words() || text == m.asked {
			return m, nil
		}
		root := m.root
		return m, func() tea.Msg {
			found, _ := recalled("", text, 6, otherMachines()...)
			if root != "" && !slices.ContainsFunc(found, func(t thread) bool { return t.Root == root }) {
				here, _ := recalled(root, text, 6) // the repository here, when it is no project mia knows yet
				found = append(here, found...)
			}
			return threadsMsg{text, found}
		}
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			rows := m.rows()
			if m.cursor < len(rows) && rows[m.cursor].into != "" {
				m.query, m.cursor = rows[m.cursor].into, 0
				return m, nil
			}
			if m.cursor < len(rows) {
				m.chosen = &rows[m.cursor]
			}
			return m, tea.Quit
		case tea.KeyUp, tea.KeyCtrlP:
			m.cursor = max(m.cursor-1, 0)
		case tea.KeyDown, tea.KeyCtrlN, tea.KeyTab:
			m.cursor = min(m.cursor+1, max(len(m.rows())-1, 0))
		case tea.KeyBackspace:
			if r := []rune(m.query); len(r) > 0 {
				m.query, m.cursor = string(r[:len(r)-1]), 0
			}
		case tea.KeyCtrlU:
			m.query, m.cursor = "", 0
		case tea.KeyRunes, tea.KeySpace:
			m.query, m.cursor = m.query+string(msg.Runes), 0
		}
		if text := m.words(); len(text) >= 3 && text != m.asked {
			return m, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return pausedMsg(text) })
		}
	}
	return m, nil
}

// words is what was typed when it means places and conversations: "" when it is a number, a
// hand-over or an agent's task.
func (m listModel) words() string {
	q := strings.TrimSpace(m.query)
	if verb, _, _ := strings.Cut(q, " "); verb == "rm" || verb == "new" || verb == "stack" || strings.Contains(q, "/") {
		return ""
	}
	if strings.HasPrefix(q, ">") || strings.HasPrefix(q, "@") || q == "?" || m.answer(q) != nil || m.task(q) != nil {
		return ""
	}
	return q
}

// answer is the row a number alone means: that answer to whoever has waited longest.
func (m listModel) answer(q string) *listRow {
	n, err := strconv.Atoi(q)
	if err != nil || len(m.stand.Waiting) == 0 || n < 1 || n > len(m.stand.Waiting[0].Options) {
		return nil
	}
	w := m.stand.Waiting[0]
	return &listRow{mark: "●", asks: true, words: "answer " + w.Worktree + ": " + w.Options[n-1], right: w.Repository, do: func(l listed) error {
		return exec.Command(l.self, "asks", "1", strconv.Itoa(n)).Run()
	}}
}

// task is the row an agent's name and then words mean: it started here, told the words.
func (m listModel) task(q string) *listRow {
	name, words, _ := strings.Cut(q, " ")
	words = strings.TrimSpace(words)
	if words == "" || !slices.ContainsFunc(m.agents, func(a agentKnown) bool { return a.Name == name }) {
		return nil
	}
	return &listRow{mark: "+", words: "start " + name + " here on: " + words, right: "new window", do: func(l listed) error {
		return l.beside(l.folder, "window", "new", name, "--task", words)
	}}
}

func (m listModel) rows() []listRow {
	q := strings.TrimSpace(m.query)
	var rows []listRow
	stands := func(stands []app.Stand, mark string, asks bool) {
		for i, w := range stands {
			rows = append(rows, listRow{mark: mark, asks: asks, words: w.Worktree + "  " + w.Line, right: w.Repository, do: func(l listed) error {
				return l.goTo(w.Root, w.Worktree)
			}})
			for n, option := range w.Options { // its answers under it, each given by Enter
				if asks {
					rows = append(rows, listRow{mark: " ", words: fmt.Sprintf("    %d  %s", n+1, option), do: func(l listed) error {
						return exec.Command(l.self, "asks", strconv.Itoa(i+1), strconv.Itoa(n+1)).Run()
					}})
				}
			}
		}
	}
	places := func(found []place) {
		for _, p := range found {
			if p.project == "project" { // opened to its worktrees
				mark, count := " ", 0
				if p.name == m.here {
					mark = "▸"
				}
				for _, other := range m.places {
					if other.project == p.name {
						count++
					}
				}
				rows = append(rows, listRow{mark: mark, words: p.name, right: strings.TrimSuffix(fmt.Sprintf("%d worktrees", count), strings.Repeat("s", btoi(count == 1))), into: p.name + "/"})
				continue
			}
			rows = append(rows, listRow{mark: " ", words: p.name + "  " + p.branch, right: p.project, do: func(l listed) error { return l.goTo(p.path, p.path) }})
		}
	}
	verb, rest, _ := strings.Cut(q, " ")
	rest = strings.TrimSpace(rest)
	project, within, drilled := strings.Cut(q, "/")
	drilled = drilled && slices.ContainsFunc(m.places, func(p place) bool { return p.project == "project" && p.name == project })
	switch {
	case q == "?": // what can be typed here, said in the list itself and nowhere else
		for _, h := range [][2]string{{"a name", "go to that worktree or project"}, {"pi fix the test", "start that agent here on it"},
			{"> codex gpt", "hand this conversation to that agent, on that model"}, {"1", "give that answer to whoever waits"},
			{"words said before", "find the conversation, and resume it"}, {"nvim · lazygit · shell", "open it in a new window"}, {"@ fedora", "move this worktree to that machine, or bring one back"},
			{"new a-branch", "a new worktree on that branch, in the project here"}, {"rm a-name", "remove that worktree, its session and its name"},
			{"stack", "the layers of the stack this worktree is on, to go to one"}} {
			rows = append(rows, listRow{mark: " ", words: h[0], right: h[1]})
		}
	case q == "":
		stands(m.stand.Waiting, "●", true)
		stands(m.stand.Working, "◌", false)
		// (The projects, the one here first: Enter opens one to its worktrees.)
		projects := slices.DeleteFunc(slices.Clone(m.places), func(p place) bool { return p.project != "project" })
		slices.SortStableFunc(projects, func(x, y place) int {
			return cmp.Compare(btoi(y.name == m.here), btoi(x.name == m.here))
		})
		places(projects)
	case drilled: // a project's worktrees, narrowed by what is typed after the /
		var its []place
		names := []string{}
		for _, p := range m.places {
			if p.project == project {
				its, names = append(its, p), append(names, p.name+" "+p.branch)
			}
		}
		if strings.TrimSpace(within) != "" {
			var narrowed []place
			for _, i := range closest(strings.TrimSpace(within), names) {
				narrowed = append(narrowed, its[i])
			}
			its = narrowed
		}
		places(its)
		if project == m.here { // and those of it that are on a machine: its session there, in a window here
			for _, t := range m.away {
				if strings.TrimSpace(within) == "" || len(closest(strings.TrimSpace(within), []string{t.name + " " + t.branch})) > 0 {
					rows = append(rows, listRow{mark: "⇄", words: t.name + "  " + t.branch, right: "on " + t.machine, do: func(l listed) error {
						return l.openIn(m.root, run.ShellJoin([]string{l.self, "machine", "attach", t.name + "@" + t.machine}), t.name+"@"+t.machine)
					}})
				}
			}
		}
	case verb == "stack":
		for _, layer := range m.stack {
			mark, right := " ", fmt.Sprintf("%d ahead", layer.Ahead)
			if layer.Current {
				mark = "▸"
			}
			if layer.NeedsRestack {
				right = "needs restack"
			}
			rows = append(rows, listRow{mark: mark, words: layer.Branch, right: right, do: func(l listed) error {
				to := exec.Command(l.self, "stack", "go", layer.Branch)
				to.Dir = l.folder
				if out, err := to.CombinedOutput(); err != nil {
					return fmt.Errorf("%s", strings.TrimSpace(string(out)))
				}
				return nil
			}})
		}
	case verb == "new" && rest != "" && m.root != "":
		branch := strings.ReplaceAll(rest, " ", "-")
		rows = append(rows, listRow{mark: "+", words: "new worktree on " + branch + " in " + m.here, right: "and go there", do: func(l listed) error {
			return l.script(m.root, "make a worktree on "+branch,
				`p=$(`+run.ShellJoin([]string{l.self, "new", branch})+` 2>/dev/null | tail -1) && [ -d "$p" ] && `+run.ShellJoin([]string{l.self, "tmux", "go", l.client})+` "$p"`)
		}})
	case verb == "rm" && rest != "":
		var trees []place
		names := []string{}
		for _, p := range m.places {
			if p.project != "project" && p.path != p.root {
				trees, names = append(trees, p), append(names, p.name)
			}
		}
		for _, i := range closest(rest, names) {
			p := trees[i]
			rows = append(rows, listRow{mark: "−", words: "remove worktree " + p.name + "  " + p.branch, right: p.project, do: func(l listed) error {
				if p.root == m.root && p.name == m.tree {
					l.leave()
				}
				// (A session of shells alone goes with it — every worktree gone to has one; one
				// an agent runs in is refused, as `mia rm` refuses it, for the person to stop.)
				windows := exec.Command(l.self, "window", "ls", p.name, "--json")
				windows.Dir = p.root
				if out, _ := windows.Output(); strings.Contains(string(out), `"kind": "agent"`) {
					return l.apart(p.root, "remove "+p.name+": an agent runs in it", "rm", p.name)
				}
				return l.apart(p.root, "remove "+p.name, "rm", "--stop-running", p.name)
			}})
		}
	case strings.HasPrefix(q, ">"):
		names := make([]string, len(m.agents))
		for i, a := range m.agents {
			names[i] = a.Name
		}
		// (`> agent` and then some of a model's name: that agent on each model the words mean.)
		want, model, _ := strings.Cut(strings.TrimSpace(q[1:]), " ")
		at := []int{}
		for i := range names {
			at = append(at, i)
		}
		if want != "" {
			at = closest(want, names)
		}
		for _, i := range at {
			name := names[i]
			if model = strings.TrimSpace(model); model != "" {
				ids := make([]string, len(m.agents[i].Models))
				for j, known := range m.agents[i].Models {
					ids[j] = known.ID
				}
				found := closest(model, ids)
				for _, j := range found[:min(len(found), 8)] {
					rows = append(rows, listRow{mark: "→", words: "move this conversation to " + name + ", on " + ids[j], do: func(l listed) error {
						return l.beside(l.folder, "hand", "--window", l.window, name, "--model", ids[j])
					}})
				}
				continue
			}
			rows = append(rows, listRow{mark: "→", words: "move this conversation to " + name, do: func(l listed) error {
				return l.beside(l.folder, "hand", "--window", l.window, name)
			}})
		}
	case strings.HasPrefix(q, "@"): // the worktree here to a machine, and one that is on a machine back
		want := strings.TrimSpace(q[1:])
		for _, to := range m.machines {
			if m.tree != "" && !m.main && (want == "" || len(closest(want, []string{to})) > 0) {
				words := "move " + m.tree + " to " + to
				rows = append(rows, listRow{mark: "⇄", words: words, right: "branch, uncommitted work, windows", do: func(l listed) error {
					l.leave() // its session here ends with the move: the client is not to end with it
					return l.apart(m.root, words, "move", m.tree, to)
				}})
			}
		}
		for _, t := range m.away {
			if want == "" || len(closest(want, []string{t.machine, t.name})) > 0 {
				words := "bring " + t.name + " back from " + t.machine
				rows = append(rows, listRow{mark: "⇄", words: words, right: t.branch, do: func(l listed) error {
					return l.apart(m.root, words, "move", t.name+"@"+t.machine, "local")
				}})
			}
		}
	case m.answer(q) != nil:
		rows = append(rows, *m.answer(q))
	case m.task(q) != nil:
		rows = append(rows, *m.task(q))
	default:
		names := make([]string, len(m.places))
		for i, p := range m.places {
			names[i] = p.name
		}
		var found []place
		for _, i := range closest(q, names) {
			if len(found) < 8 {
				found = append(found, m.places[i])
			}
		}
		places(found)
		for _, d := range listDoes {
			if len(q) >= 2 && slices.ContainsFunc(d.names, func(name string) bool { return strings.HasPrefix(name, strings.ToLower(q)) }) {
				rows = append(rows, listRow{mark: "+", words: d.words, right: "new window", do: func(l listed) error { return l.open(d.runs) }})
			}
		}
		if m.asked == q {
			for _, t := range m.threads {
				who := t.Agent
				if t.Yours {
					who = "you"
				}
				whose := t.Agent
				if t.Machine != "" {
					whose += " on " + t.Machine
				}
				in := t.Project
				if t.Worktree != "" && t.Worktree != t.Project {
					in += "/" + t.Worktree
				}
				if t.Gone {
					whose += " · removed"
				}
				rows = append(rows, listRow{mark: "↺", words: who + ": " + t.Line, right: in + " · " + whose + " · " + t.At.Local().Format("Jan 2"), do: func(l listed) error {
					if t.Gone {
						return fmt.Errorf("its worktree is there no more, and %s resumes a conversation only in its folder: `mia recall` prints it", t.Agent)
					}
					if t.Machine != "" { // on a machine: its worktree's session there, in a window here
						at := slices.IndexFunc(m.places, func(p place) bool { return p.project == "project" && p.name == t.Project })
						if at < 0 {
							return fmt.Errorf("%s is on %s, and no project here is called that to reach it from", t.Project, t.Machine)
						}
						return l.openIn(m.places[at].root, run.ShellJoin([]string{l.self, "machine", "attach", t.Worktree + "@" + t.Machine}), t.Worktree+"@"+t.Machine)
					}
					if err := l.goTo(t.Root, t.Cwd); err != nil {
						return err
					}
					return l.beside(t.Root, "conversation", "resume", t.Session)
				}})
			}
		}
	}
	return rows
}

// listDoes is what is opened in a window of its own by its name: what it is called, the row,
// and the command ("" a shell).
var listDoes = []struct {
	names []string
	words string
	runs  string
}{
	{[]string{"nvim", "vim", "editor", "edit"}, "open the editor here", "${EDITOR:-nvim} ."},
	{[]string{"lazygit", "git"}, "open lazygit here", "lazygit"},
	{[]string{"shell", "zsh", "terminal"}, "open a shell here", ""},
}

var (
	listDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	listAmber = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	listOn    = lipgloss.NewStyle().Reverse(true)
)

func (m listModel) View() string {
	width := max(m.width, 40)
	var b strings.Builder
	where := m.here
	if m.tree != "" && m.tree != m.here {
		where += " / " + m.tree
	}
	typed := " " + listDim.Render("❯ ") + m.query + listOn.Render(" ")
	b.WriteString(typed + strings.Repeat(" ", max(width-lipgloss.Width(typed)-lipgloss.Width(where)-1, 1)) + listDim.Render(where) + "\n\n")
	rows := m.rows()
	// (As many rows as there is room for, the chosen one kept among them.)
	fits, first := max(m.height-2, 3), 0
	if m.cursor >= fits {
		first = m.cursor - fits + 1
	}
	shown := rows[min(first, len(rows)):min(first+fits, len(rows))]
	if len(rows) == 0 && strings.TrimSpace(m.query) != "" {
		b.WriteString(" " + listDim.Render("nothing by that name"))
	}
	for i, r := range shown {
		i += first
		room := width - 4 - lipgloss.Width(r.right) - 2
		words := []rune(r.words)
		if len(words) > room {
			words = append(words[:max(room-1, 0)], '…')
		}
		pad := strings.Repeat(" ", max(room-len(words), 0)+2)
		if i == m.cursor {
			b.WriteString(listOn.Render(" " + r.mark + " " + string(words) + pad + r.right + " "))
		} else {
			mark := listDim.Render(r.mark)
			if r.asks {
				mark = listAmber.Render(r.mark)
			}
			b.WriteString(" " + mark + " " + string(words) + pad + listDim.Render(r.right) + " ")
		}
		if i < first+len(shown)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// goTo takes the client to the session of the worktree at (or named) worktree of the
// repository at root, started if it is not up.
func (l listed) goTo(root, worktree string) error {
	a, err := app.Open(root)
	if err != nil {
		return err
	}
	return a.SwitchClient(worktree, l.client)
}

// open opens a window in the client's session, in the folder the list was opened in, running
// runs ("" a shell).
func (l listed) open(runs string) error { return l.openIn(l.folder, runs, "") }

// openIn is open in another folder, the window called name when one is given (a session on a
// machine, which tmux would call `mia` after what attaches to it).
func (l listed) openIn(dir, runs, name string) error {
	tmux := run.Local("tmux")
	session, err := tmux.Capture("display-message", "-p", "-c", l.client, "#{session_id}")
	if err != nil {
		return err
	}
	argv := []string{"new-window", "-t", strings.TrimSpace(session) + ":", "-c", dir}
	if name != "" {
		argv = append(argv, "-n", name)
	}
	if runs != "" {
		argv = append(argv, runs)
	}
	if out, err := tmux.Combined(argv...); err != nil {
		return fmt.Errorf("tmux new-window: %s", strings.TrimSpace(out))
	}
	return nil
}

// leave takes the client to the session it was in before, or the next: for when the session it
// is in is about to end, which would end the client with it.
func (l listed) leave() {
	tmux := run.Local("tmux")
	if _, err := tmux.Combined("switch-client", "-c", l.client, "-l"); err != nil {
		_, _ = tmux.Combined("switch-client", "-c", l.client, "-n")
	}
}

// apart runs this mia in dir and lets it go on by itself, as beside does, where it opens no
// window to go to: how it ended is said on the client's status line.
func (l listed) apart(dir, doing string, verb ...string) error {
	return l.script(dir, doing, run.ShellJoin(append([]string{l.self}, verb...))+" >/dev/null 2>&1", "`mia "+strings.Join(verb, " ")+"` in a shell says why")
}

// script is apart of a line of shell: why is how to learn what went wrong, when there is one.
func (l listed) script(dir, doing, line string, why ...string) error {
	said := "tmux display-message -d 4000 -c " + run.ShellJoin([]string{l.client}) + " " // four seconds: long enough to be read
	failed := "mia: could not " + doing
	if len(why) > 0 {
		failed += " — " + why[0]
	}
	script := "{ " + line + "; } && " + said + run.ShellJoin([]string{"mia: done: " + doing}) + " || " + said + run.ShellJoin([]string{failed})
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir, cmd.SysProcAttr = dir, &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// beside runs this mia in dir and lets it go on by itself (an agent takes seconds to come up,
// and the list is gone by then), and puts the client in the window it opens: the first there
// is within three seconds that there was not, in whichever session (the worktree's own, when
// the client is in one mia did not make).
func (l listed) beside(dir string, verb ...string) error {
	tmux := run.Local("tmux")
	windows := func() []string {
		out, _ := tmux.Capture("list-windows", "-a", "-F", "#{window_id}")
		return strings.Fields(out)
	}
	before := windows()
	cmd := exec.Command(l.self, verb...)
	cmd.Dir, cmd.SysProcAttr = dir, &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	for range 30 {
		time.Sleep(100 * time.Millisecond)
		for _, w := range windows() {
			if !slices.Contains(before, w) {
				_, err := tmux.Combined("switch-client", "-c", l.client, "-t", w)
				return err
			}
		}
	}
	return fmt.Errorf("`mia %s` opened no window — run it in a shell to see why", strings.Join(verb, " "))
}

// cmdTmuxList is mia tmux list [--popup] <client> <window> <folder>: the one list, over the
// client. The key runs it with --popup through run-shell, which fills in the three.
func cmdTmuxList(args []string) int {
	popup := take(&args, "--popup")
	if len(args) != 3 {
		return usageErr("mia tmux list [--popup] <client> <window> <folder>")
	}
	self, err := os.Executable()
	if err != nil {
		return tmuxSay(args[:1], err)
	}
	if popup {
		_, _ = run.Local("tmux").Combined(append([]string{"display-popup", "-c", args[0], "-E", "-w", "88", "-h", "20", "-T", " mia ", self, "tmux", "list"}, args...)...)
		return exitOK
	}
	// (Where it is opened is the worktree of the session the client is in, when mia made that
	// session: the pane in front says another folder when its shell went elsewhere, and none
	// at all under an editor.)
	if at, err := run.Local("tmux").Capture("display-message", "-p", "-c", args[0], "#{session_name}\t#{session_path}"); err == nil {
		name, path, _ := strings.Cut(strings.TrimSpace(at), "\t")
		if _, err := app.Open(path); err == nil && strings.HasPrefix(name, "mia-") && !strings.HasPrefix(args[2], path+string(filepath.Separator)) {
			args[2] = path
		}
	}
	_ = os.Chdir(args[2]) // what is asked of mia is asked of the worktree there
	m := listModel{at: listed{self: self, client: args[0], window: args[1], folder: args[2]}}
	if a, err := app.Open(args[2]); err == nil {
		m.root, m.here = a.Root, filepath.Base(a.Root)
		if resolver, err := a.Resolver(); err == nil {
			for _, r := range resolver.Records {
				if r.Path == args[2] || strings.HasPrefix(args[2], r.Path+string(filepath.Separator)) {
					m.tree, m.main = r.Name, r.Path == a.Root
				}
			}
		}
		repos, _ := app.Repositories()
		for _, repo := range repos {
			if repo.Path == a.Root {
				m.here = repo.Name
			}
		}
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return tmuxSay(args[:1], err)
	}
	if row := final.(listModel).chosen; row != nil && row.do != nil {
		if err := row.do(m.at); err != nil {
			return tmuxSay(args[:1], err)
		}
	}
	return exitOK
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cmdTmuxGo is mia tmux go <client> <folder>: the client to the session of the worktree at the
// folder, started if it is not up; for what runs with no terminal.
func cmdTmuxGo(args []string) int {
	if len(args) != 2 {
		return usageErr("mia tmux go <client> <folder>")
	}
	a, err := app.Open(args[1])
	if err != nil {
		return tmuxSay(args[:1], err)
	}
	if err := a.SwitchClient(args[1], args[0]); err != nil {
		return tmuxSay(args[:1], err)
	}
	return exitOK
}
