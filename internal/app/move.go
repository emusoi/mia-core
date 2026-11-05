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

// Window is one window of a session, as a move recreates it.
type Window struct {
	Name    string
	Dir     string // under the worktree
	Command string // what ran there; empty for a shell
	// Conversation is the claude session the window ran, so it can be
	// resumed exactly; "" for anything else, or when claude did not say.
	Conversation string `json:",omitempty"`
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
	patch, err := src.bytesOf(`cd "$1" && git diff --binary HEAD`, path)
	if err != nil {
		return Moved{}, err
	}
	untracked, err := src.sh(`cd "$1" && git ls-files --others --exclude-standard`, path)
	if err != nil {
		return Moved{}, err
	}
	var files []byte
	if strings.TrimSpace(untracked) != "" {
		if files, err = src.bytesOf(`cd "$1" && git ls-files --others --exclude-standard -z | COPYFILE_DISABLE=1 tar --null -cf - -T -`, path); err != nil {
			return Moved{}, err
		}
	}
	windows := src.windows(name, path)

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
	if len(bytes.TrimSpace(patch)) > 0 {
		fmt.Fprintln(say, "putting back its uncommitted changes")
		if err := dst.feed(patch, `cd "$1" && git apply --binary`, moved.Path); err != nil {
			return moved, fmt.Errorf("apply the uncommitted changes there (%s is untouched here): %w", name, err)
		}
	}
	if len(files) > 0 {
		if err := dst.feed(files, `cd "$1" && tar -xf -`, moved.Path); err != nil {
			return moved, fmt.Errorf("copy the new files there (%s is untouched here): %w", name, err)
		}
	}
	moved.Carried = a.carry(src, dst, path, moved.Path, windows)
	ridden := a.ride(src, dst, path, moved.Path)
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
	out, err := s.sh(`tmux list-windows -t "=mia-$1" -F '#{window_name}	#{pane_current_path}	#{pane_current_command}	#{pane_pid}' 2>/dev/null`, name)
	if err != nil {
		return nil
	}
	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		w := Window{Name: f[0]}
		if rel, err := filepath.Rel(path, f[1]); err == nil && !strings.HasPrefix(rel, "..") {
			w.Dir = rel
		}
		if !shells[f[2]] {
			args, err := s.sh(`ps -o args= -p "$(ps -o tpgid= -p "$1" | tr -d ' ')"`, f[3])
			w.Command = strings.TrimSpace(args)
			if err != nil || w.Command == "" {
				w.Command = f[2]
			}
			if words := strings.Fields(w.Command); len(words) > 0 && filepath.Base(words[0]) == "claude" {
				w.Conversation = s.conversationOf(f[3])
			}
		}
		windows = append(windows, w)
	}
	return windows
}

// open gives the worktree's session there the windows it had, in order,
// each started in its folder with its command typed in, so its shell stays
// when the command ends.
func (s side) open(name, path string, windows []Window) error {
	if _, err := s.sh(`tmux has-session -t "=mia-$1" 2>/dev/null || tmux new-session -d -s "mia-$1" -c "$2"`, name, path); err != nil {
		return err
	}
	for i, w := range windows {
		dir := filepath.Join(path, w.Dir)
		if i == 0 {
			if _, err := s.sh(`tmux rename-window -t "=mia-$1:" "$2" && tmux send-keys -t "=mia-$1:$2" "$3" Enter`, name, w.Name, "cd "+shellQuote(dir)); err != nil {
				return err
			}
		} else if _, err := s.sh(`tmux new-window -t "=mia-$1:" -n "$2" -c "$3"`, name, w.Name, dir); err != nil {
			return err
		}
		if w.Command != "" {
			if _, err := s.sh(`tmux send-keys -t "=mia-$1:$2" "$3" Enter`, name, w.Name, w.Command); err != nil {
				return err
			}
		}
	}
	return nil
}

// carry brings each window's conversation along when its program has a
// carrier, unpacking it under the home on the other side; the window then
// restarts with what the carrier says, so the agent picks up where it was.
func (a *App) carry(src, dst side, from, to string, windows []Window) []string {
	var carried []string
	for i, w := range windows {
		words := strings.Fields(w.Command)
		if len(words) == 0 {
			continue // a shell: nothing to carry
		}
		program := filepath.Base(words[0])
		script, ok := carriers[program]
		if !ok {
			continue
		}
		toPath := to
		if !dst.here() {
			if home, err := dst.machine.Home(); err == nil && !filepath.IsAbs(to) {
				toPath = filepath.Join(strings.TrimSpace(home), to)
			}
		}
		packed, err := src.bytesOf(script, from, toPath, "pack")
		if err != nil || len(packed) == 0 {
			continue
		}
		if err := dst.feed(packed, `cd && tar -xf -`); err != nil {
			continue
		}
		if again, err := src.sh(script, from, toPath, "command"); err == nil && strings.TrimSpace(again) != "" {
			windows[i].Command = strings.TrimSpace(again)
		}
		carried = append(carried, program)
	}
	return carried
}

// ride carries what rides with the worktree: each rider packs on the side
// it leaves and lands on the other, and is told later when the worktree is
// gone from where it was.
func (a *App) ride(src, dst side, from, to string) []string {
	var ridden []string
	for name, script := range riders {
		toPath := to
		if !dst.here() {
			if home, err := dst.machine.Home(); err == nil && !filepath.IsAbs(to) {
				toPath = filepath.Join(strings.TrimSpace(home), to)
			}
		}
		packed, err := src.bytesOf(`cd "$1" && shift && `+script, src.repo, from, toPath, "pack", dst.label())
		if err != nil || len(packed) == 0 {
			continue
		}
		if err := dst.feed(packed, script, from, toPath, "land", src.label()); err != nil {
			continue
		}
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
