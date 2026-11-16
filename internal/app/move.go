package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/runtime"
)

// side is one end of a move: this computer or a machine, and where the
// repository is there, as a shell there can reach it.
type side struct {
	machine runtime.Machine
	repo    string // absolute here; under the home there
	mia     string // the mia that answers there
}

func (s side) here() bool { return s.machine.SSH == "" }

// sh runs a script on the side, with a UTF-8 locale, and returns its output.
func (s side) sh(script string, args ...string) (string, error) {
	return runtime.Run(s.machine.SSH, append([]string{"sh", "-c", utf8 + script, "sh"}, args...)...)
}

// feed runs a script on the side with data on its standard input.
func (s side) feed(data []byte, script string, args ...string) error {
	cmd := runtime.Command(s.machine.SSH, append([]string{"sh", "-c", utf8 + script, "sh"}, args...)...)
	cmd.Stdin = bytes.NewReader(data)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// bytesOf runs a script on the side and returns what it printed, untouched.
func (s side) bytesOf(script string, args ...string) ([]byte, error) {
	return runtime.Command(s.machine.SSH, append([]string{"sh", "-c", utf8 + script, "sh"}, args...)...).Output()
}

// Pane is one pane of a window: where it is and what runs in it.
type Pane struct {
	Dir     string // under the worktree
	Command string // what ran there; empty for a shell
	// Conversation is the claude session the pane ran, so it can be
	// resumed exactly; "" for anything else, or when claude did not say.
	Conversation string `json:",omitempty"`
	// ID is tmux's id of the pane, the same for as long as its server runs.
	ID string `json:",omitempty"`
}

// Window is one window of a session, as a move recreates it: its first
// pane, and the others it was split into, laid out as they were.
type Window struct {
	Name string
	Pane
	Splits []Pane `json:",omitempty"`
	Layout string `json:",omitempty"` // tmux's, for a window that is split
	// Auto is a window tmux names after what runs in it; one a person
	// named keeps that name when it is opened again.
	Auto bool `json:",omitempty"`
}

// panes is every pane of the window, the first first.
func (w *Window) panes() []*Pane {
	all := []*Pane{&w.Pane}
	for i := range w.Splits {
		all = append(all, &w.Splits[i])
	}
	return all
}

// What says the window in a line: its name and what each pane runs.
func (w Window) What() string {
	what := strings.TrimSpace(w.Name + " " + w.Command)
	if w.Auto && w.Command != "" {
		what = w.Command // its name only says the same
	}
	for _, p := range w.Splits {
		if p.Command == "" {
			p.Command = "shell"
		}
		what += " | " + p.Command
	}
	return what
}

// Moved is what a move made, for saying so.
type Moved struct {
	Name, Path string
	Windows    []Window
	Carried    []string // programs whose conversation came along
}

var shells = map[string]bool{"zsh": true, "bash": true, "sh": true, "fish": true, "dash": true, "-zsh": true, "-bash": true}

// Move takes a worktree from one side to the other: its branch, its
// uncommitted work and its session's windows, each restarted there; a
// claude, codex or pi window has its conversation carried too.
// Only once all of that is there is the worktree here removed; its branch
// stays where it was.
func (a *App) Move(name, from, to string, say io.Writer) (Moved, error) {
	src, err := a.sideOf(from)
	if err != nil {
		return Moved{}, err
	}
	dst, err := a.sideOf(to)
	if err != nil {
		return Moved{}, err
	}
	if src.here() == dst.here() {
		return Moved{}, fmt.Errorf("a worktree moves between this computer and a machine; %s to %s is two moves", from, to)
	}
	path, err := src.sh(`cd "$1" && `+src.mia+` path "$2"`, src.repo, name)
	if err != nil {
		return Moved{}, err
	}
	path = strings.TrimSpace(path)
	branch, err := src.sh(`git -C "$1" symbolic-ref --short HEAD`, path)
	if err != nil {
		return Moved{}, fmt.Errorf("%s has no branch checked out; check one out to move it", name)
	}
	branch = strings.TrimSpace(branch)

	fmt.Fprintf(say, "reading %s: its changes and its windows\n", name)
	work, err := src.uncommitted(path)
	if err != nil {
		return Moved{}, err
	}
	windows := src.windows(name, path)
	if stays := work.staysBehind(); stays != "" {
		fmt.Fprintf(say, "%s — they go when %s is removed %s\n", stays, name, src.where())
	}

	fmt.Fprintf(say, "taking %s across\n", branch)
	if err := a.carryBranch(src, dst, branch); err != nil {
		return Moved{}, err
	}
	out, err := dst.sh(`cd "$1" && `+dst.mia+` new "$2"`, dst.repo, branch)
	if err != nil {
		return Moved{}, fmt.Errorf("make the worktree: %w", err)
	}
	moved := Moved{Name: madeName(out), Windows: windows}
	if moved.Name == "" {
		return moved, fmt.Errorf("make the worktree: no name in %q", strings.TrimSpace(out))
	}
	// it keeps its name across the move when that name is free there
	if moved.Name != name {
		if _, err := dst.sh(`cd "$1" && `+dst.mia+` rename "$2" "$3"`, dst.repo, moved.Name, name); err == nil {
			moved.Name = name
		} else {
			fmt.Fprintf(say, "%s is taken there; it is %s now\n", name, moved.Name)
		}
	}
	if p, err := dst.sh(`cd "$1" && `+dst.mia+` path "$2"`, dst.repo, moved.Name); err == nil {
		moved.Path = strings.TrimSpace(p)
	}
	if !work.none() {
		fmt.Fprintln(say, "putting back its uncommitted changes")
		if err := dst.putUncommitted(moved.Path, work); err != nil {
			return moved, fmt.Errorf("%w (%s is untouched %s)", err, name, src.where())
		}
	}
	moved.Carried = a.carry(src, dst, path, moved.Path, windows)
	ridden := a.ride(src, dst, path, moved.Path, say)
	moved.Carried = append(moved.Carried, ridden...)
	fmt.Fprintf(say, "opening its windows there: %d\n", len(windows))
	if err := dst.open(moved.Name, moved.Path, windows); err != nil {
		return moved, err
	}
	fmt.Fprintf(say, "removing %s where it was; %s stays there\n", name, branch)
	if _, err := src.sh(`cd "$1" && `+src.mia+` rm --force --stop-running "$2"`, src.repo, name); err != nil {
		return moved, fmt.Errorf("it is there now, but removing it here failed: %w", err)
	}
	for _, name := range ridden {
		_, _ = src.sh(`cd "$1" && shift && `+riders[name], src.repo, path, moved.Path, "done", dst.label())
	}
	return moved, nil
}

// uncommitted is a worktree's work that no commit holds, as it crosses.
type uncommitted struct {
	staged  []byte // a patch from HEAD to the index
	changed []byte // a patch from the index to the files
	files   []byte // a tar of the files git does not know and does not ignore
	// ignored is what git ignores there, a folder as one: it does not cross.
	ignored []string
}

func (u uncommitted) none() bool {
	return len(bytes.TrimSpace(u.staged)) == 0 && len(bytes.TrimSpace(u.changed)) == 0 && len(u.files) == 0
}

// uncommitted reads the worktree's uncommitted work: what is staged and
// what is changed beyond that apart, so the index is the same on the other
// side, not everything unstaged.
func (s side) uncommitted(path string) (uncommitted, error) {
	var u uncommitted
	var err error
	if u.staged, err = s.bytesOf(`cd "$1" && git diff --binary --cached HEAD`, path); err != nil {
		return u, err
	}
	if u.changed, err = s.bytesOf(`cd "$1" && git diff --binary`, path); err != nil {
		return u, err
	}
	untracked, err := s.sh(`cd "$1" && git ls-files --others --exclude-standard`, path)
	if err != nil {
		return u, err
	}
	if strings.TrimSpace(untracked) != "" {
		if u.files, err = s.bytesOf(`cd "$1" && git ls-files --others --exclude-standard -z | COPYFILE_DISABLE=1 tar --null -cf - -T -`, path); err != nil {
			return u, err
		}
	}
	ignored, err := s.sh(`cd "$1" && git -c core.quotepath=off ls-files --others --ignored --exclude-standard --directory`, path)
	if err != nil {
		return u, err
	}
	if ignored = strings.TrimSpace(ignored); ignored != "" {
		u.ignored = strings.Split(ignored, "\n")
	}
	return u, nil
}

// staysBehind says what of the worktree does not cross, "" when all of it does.
func (u uncommitted) staysBehind() string {
	if len(u.ignored) == 0 {
		return ""
	}
	named := u.ignored
	more := ""
	if len(named) > 4 {
		named, more = named[:4], fmt.Sprintf(" and %d more", len(u.ignored)-4)
	}
	return fmt.Sprintf("not crossing, since git ignores them: %s%s", strings.Join(named, ", "), more)
}

// putUncommitted gives a clean worktree at the same commit that work.
func (s side) putUncommitted(path string, u uncommitted) error {
	if len(bytes.TrimSpace(u.staged)) > 0 {
		if err := s.feed(u.staged, `cd "$1" && git apply --binary --index`, path); err != nil {
			return fmt.Errorf("apply the staged changes: %w", err)
		}
	}
	if len(bytes.TrimSpace(u.changed)) > 0 {
		if err := s.feed(u.changed, `cd "$1" && git apply --binary`, path); err != nil {
			return fmt.Errorf("apply the uncommitted changes: %w", err)
		}
	}
	if len(u.files) > 0 {
		if err := s.feed(u.files, `cd "$1" && tar -xf -`, path); err != nil {
			return fmt.Errorf("copy the new files: %w", err)
		}
	}
	return nil
}

// where says the side in a sentence: "here", or "on fedora".
func (s side) where() string {
	if s.here() {
		return "here"
	}
	return "on " + s.machine.Name
}

// label is what the other side calls this one: the machine's name, or this
// computer's host name.
func (s side) label() string {
	if !s.here() {
		return s.machine.Name
	}
	host, _ := os.Hostname()
	return strings.ToLower(strings.SplitN(host, ".", 2)[0])
}

func (a *App) sideOf(where string) (side, error) {
	if where == "" || where == runtime.Local {
		self, err := os.Executable()
		if err != nil {
			return side{}, err
		}
		return side{machine: runtime.Machine{Name: runtime.Local}, repo: a.Root, mia: shellQuote(self)}, nil
	}
	machine, err := a.Runtimes.Get(where)
	if err != nil {
		return side{}, err
	}
	dir, err := a.RemoteDir()
	if err != nil {
		return side{}, err
	}
	return side{machine: machine, repo: dir, mia: remoteMia}, nil
}

// carryBranch puts the branch on the other side: pushed to a machine, or
// fetched from one and moved forward here, never over work only here.
func (a *App) carryBranch(src, dst side, branch string) error {
	if dst.here() {
		if err := git.Fetch(a.Root, src.machine.Name); err != nil {
			return err
		}
		theirs := git.RevParse(a.Root, "refs/remotes/"+src.machine.Name+"/"+branch)
		ours := git.RevParse(a.Root, "refs/heads/"+branch)
		switch {
		case theirs == "":
			return fmt.Errorf("%s has no branch %s to bring here", src.machine.Name, branch)
		case ours == "", ours == theirs, git.IsAncestor(a.Root, ours, theirs):
			return git.UpdateRef(a.Root, "refs/heads/"+branch, theirs)
		case git.IsAncestor(a.Root, theirs, ours):
			return fmt.Errorf("%s here has commits %s does not; merge them first", branch, src.machine.Name)
		}
		return fmt.Errorf("%s moved here and on %s since they parted; merge %s/%s first", branch, src.machine.Name, src.machine.Name, branch)
	}
	if _, err := a.Link(dst.machine); err != nil {
		return err
	}
	return git.Push(a.Root, dst.machine.Name, "refs/heads/"+branch+":refs/heads/"+branch)
}

// windows is the session's windows: name, folder under the worktree, and
// the command running there when it is not just a shell.
// conversationOf is the claude session the program in the foreground of a
// pane's process runs: claude writes it to sessions/<pid>.json.
func (s side) conversationOf(panePID string) string {
	out, err := s.sh(`pid="$(ps -o tpgid= -p "$1" | tr -d ' ')"; cat "${CLAUDE_CONFIG_DIR:-$HOME/.claude}/sessions/$pid.json" 2>/dev/null`, panePID)
	var running struct {
		SessionID string `json:"sessionId"`
	}
	if err != nil || json.Unmarshal([]byte(out), &running) != nil {
		return ""
	}
	return running.SessionID
}

func (s side) windows(name, path string) []Window {
	out, err := s.sh(`tmux list-panes -s -t "=mia-$1" -F '#{window_id}	#{window_name}	#{window_layout}	#{pane_current_path}	#{pane_current_command}	#{pane_pid}	#{pane_id}	#{automatic-rename}' 2>/dev/null`, name)
	if err != nil {
		return nil
	}
	var windows []Window
	window := "" // tmux lists a window's panes together, in order
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 8 {
			continue
		}
		p := Pane{ID: f[6]}
		if rel, err := filepath.Rel(path, f[3]); err == nil && !strings.HasPrefix(rel, "..") {
			p.Dir = rel
		}
		if !shells[f[4]] {
			args, err := s.sh(`ps -o args= -p "$(ps -o tpgid= -p "$1" | tr -d ' ')"`, f[5])
			p.Command = strings.TrimSpace(args)
			if err != nil || p.Command == "" {
				p.Command = f[4]
			}
			if words := strings.Fields(p.Command); len(words) > 0 && filepath.Base(words[0]) == "claude" {
				p.Conversation = s.conversationOf(f[5])
			}
		}
		if f[0] != window {
			window = f[0]
			windows = append(windows, Window{Name: f[1], Pane: p, Auto: f[7] == "1"})
			continue
		}
		last := &windows[len(windows)-1]
		last.Splits, last.Layout = append(last.Splits, p), f[2]
	}
	return windows
}

// open gives the worktree's session there the windows it had, in order,
// each pane started in its folder with its command typed in, so its shell
// stays when the command ends.
func (s side) open(name, path string, windows []Window) error {
	if _, err := s.sh(`tmux has-session -t "=mia-$1" 2>/dev/null || tmux new-session -d -s "mia-$1" -c "$2"`, name, path); err != nil {
		return err
	}
	run := func(pane, command string) error {
		if command == "" {
			return nil
		}
		_, err := s.sh(`tmux send-keys -t "$1" "$2" Enter`, pane, command)
		return err
	}
	for i, w := range windows {
		dir := filepath.Join(path, w.Dir)
		var first string
		var err error
		if i == 0 {
			if first, err = s.sh(`tmux display-message -p -t "=mia-$1:" '#{pane_id}'`, name); err == nil {
				err = run(strings.TrimSpace(first), "cd "+shellQuote(dir))
			}
		} else {
			first, err = s.sh(`tmux new-window -t "=mia-$1:" -c "$2" -P -F '#{pane_id}'`, name, dir)
		}
		first = strings.TrimSpace(first)
		if err == nil && !w.Auto {
			_, err = s.sh(`tmux rename-window -t "$1" "$2"`, first, w.Name) // tmux names it no more
		}
		if err == nil {
			err = run(first, w.Command)
		}
		if err != nil {
			return err
		}
		for _, p := range w.Splits {
			// evened out after each, so there is room for the next
			pane, err := s.sh(`tmux split-window -d -t "$1" -c "$2" -P -F '#{pane_id}' && tmux select-layout -t "$1" tiled >/dev/null`, first, filepath.Join(path, p.Dir))
			if err != nil {
				break // no room for more: what fits is there
			}
			if err := run(strings.TrimSpace(pane), p.Command); err != nil {
				return err
			}
		}
		if len(w.Splits) > 0 {
			_, _ = s.sh(`tmux select-layout -t "$1" "$2"`, first, w.Layout) // as it was, where it still fits
		}
	}
	return nil
}

// carry brings each window's conversation along when its program has a
// carrier, unpacking it under the home on the other side; the window then
// restarts with what the carrier says, so the agent picks up where it was.
func (a *App) carry(src, dst side, from, to string, windows []Window) []string {
	toPath := to
	if !dst.here() {
		if home, err := dst.machine.Home(); err == nil && !filepath.IsAbs(to) {
			toPath = filepath.Join(strings.TrimSpace(home), to)
		}
	}
	var carried []string
	again := map[string]string{} // by program: one folder, one conversation each
	for i := range windows {
		for _, p := range windows[i].panes() {
			program, _ := agentOf(p.Command)
			script, ok := carriers[program]
			if !ok {
				continue // a shell, or a program with no conversation to carry
			}
			if _, done := again[program]; !done {
				again[program] = ""
				packed, err := src.bytesOf(script, from, toPath, "pack")
				if err != nil || len(packed) == 0 || dst.feed(packed, `cd && tar -xf -`) != nil {
					continue
				}
				command, _ := src.sh(script, from, toPath, "command")
				again[program] = strings.TrimSpace(command)
				carried = append(carried, program)
			}
			if again[program] != "" {
				p.Command = again[program]
			}
		}
	}
	return carried
}

// ride carries what rides with the worktree: each rider packs on the side
// it leaves and lands on the other, and is told later when the worktree is
// gone from where it was.
func (a *App) ride(src, dst side, from, to string, say io.Writer) []string {
	var ridden []string
	for name, script := range riders {
		toPath := to
		if !dst.here() {
			if home, err := dst.machine.Home(); err == nil && !filepath.IsAbs(to) {
				toPath = filepath.Join(strings.TrimSpace(home), to)
			}
		}
		packed, err := src.bytesOf(`cd "$1" && shift && `+script, src.repo, from, toPath, "pack", dst.label())
		if err != nil {
			fmt.Fprintf(say, "%s stays behind: %v\n", name, err)
			continue
		}
		if len(packed) == 0 {
			continue // nothing of it here
		}
		if err := dst.feed(packed, script, from, toPath, "land", src.label()); err != nil {
			fmt.Fprintf(say, "%s stays behind: %v\n", name, err)
			continue
		}
		fmt.Fprintf(say, "taking its %s review along\n", name)
		ridden = append(ridden, name)
	}
	return ridden
}

func madeName(out string) string {
	made := ""
	for _, line := range strings.Split(out, "\n") {
		if name, _, ok := strings.Cut(strings.TrimSpace(line), " — "); ok && name != "" && !strings.ContainsAny(name, " \t") {
			made = name
		}
	}
	return made
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
