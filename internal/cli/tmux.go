package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/run"
)

// handOver gives this terminal to the machine's tmux: the client here
// detaches and ssh attaches the worktree's session there, so every tmux key
// — a new window, a split — acts on that machine, as if it were this one.
// Detaching there brings back the session this terminal was in.
func handOver(a *app.App, target string) error {
	_, on, _ := strings.Cut(target, "@")
	if _, err := a.Runtimes.Get(on); err != nil {
		return err
	}
	back, err := run.Local("tmux").Capture("display-message", "-p", "#{socket_path}\t#{session_name}")
	if err != nil {
		return err
	}
	socket, session, _ := strings.Cut(strings.TrimSpace(back), "\t")
	self, err := os.Executable()
	if err != nil {
		return err
	}
	away := "cd " + run.ShellJoin([]string{a.Root}) + " && " + run.ShellJoin([]string{self, "away", target, session, "--back", socket})
	out, err := run.Local("tmux").Combined("detach-client", "-E", away)
	if err != nil {
		return fmt.Errorf("hand over to %s: %s", on, strings.TrimSpace(out))
	}
	return nil
}

const tmuxMarker = "# mia: status line and popups"

// tmuxSnippet is what mia adds to a tmux config: its part of the status
// line, refreshed every 5s, and its keys.
func tmuxSnippet(self string) string {
	mia := run.ShellJoin([]string{self})
	in := `cd \"#{pane_current_path}\" && ` + mia
	var keys strings.Builder
	for i, k := range tmuxKeys(mia, in) {
		// (Each on a prefix key, and on a key of its own the terminal app can send: `mia tmux keys`.)
		if k.does == "" { // retired: its number stays taken, bound to nothing
			continue
		}
		if k.prefix != "" {
			fmt.Fprintf(&keys, "bind %s %s\n", k.prefix, k.does)
		}
		fmt.Fprintf(&keys, "set -s user-keys[%d] \"\\e[99;%d~\"\nbind -n User%d %s\n", userKeysFrom+i, i+1, userKeysFrom+i, k.does)
	}
	return tmuxMarker + ` — mia tmux uninstall removes this block
set -g status-interval 5
set -g status-right-length 160
set -ga status-right ' #(` + mia + ` status-line "#{pane_current_path}") '
` + keys.String() + `set-hook -g client-session-changed "run-shell -b \"` + mia + ` tmux seen '#{client_name}' '#{session_name}'\""
# mia: end
`
}

// userKeysFrom is the first of tmux's user keys mia takes, clear of a config's own few.
const userKeysFrom = 40

// tmuxKey is one thing mia does in tmux: its prefix key ("" for none), the key the terminal
// app sends it by (`mia tmux keys`), what it is called, and the tmux command it runs. The
// zero tmuxKey is a retired one: its place kept, nothing bound, no terminal key sent.
type tmuxKey struct{ prefix, cmd, name, does string }

// tmuxKeys is every key mia binds, in the order their user keys are numbered: never reordered,
// only added to, so a terminal app's config keeps meaning what it meant. A key that goes stays
// in its place, retired, so every key after it keeps its number: a terminal app's config
// (kitty's) sends \e[99;N~ by the place.
func tmuxKeys(mia, in string) []tmuxKey {
	keys := []tmuxKey{
		{"k", "cmd+k", "the one list: go somewhere, start an agent, hand on, answer, resume", `run-shell -b "` + mia + ` tmux list --popup '#{client_name}' '#{window_index}' '#{pane_current_path}'"`},
		{"K", "cmd+o", "the popup terminal", `run-shell -b "` + mia + ` tmux shell --popup '#{client_name}' '#{pane_current_path}'"`},
		{}, // retired: prefix Space, cmd+shift+j — the radar; the one list has who needs you
		{}, // retired: prefix n, cmd+n — a new worktree; `new <branch>` in the one list
		{}, // retired: prefix m — mia's menu; `back`, `clean` and the menu's verbs in the one list
		{"a", "cmd+j", "the next that needs you", `run-shell -b "` + in + ` tmux next '#{client_name}'"`},
		{}, // retired: prefix e, cmd+. — this window's menu; `stop`, `fork` and `>` in the one list
		{"b", "cmd+[", "back through where you have been", `run-shell -b "` + mia + ` tmux walk '#{client_name}' back"`},
		{"B", "cmd+]", "forward again", `run-shell -b "` + mia + ` tmux walk '#{client_name}' forward"`},
		{"", "cmd+t", "a new shell window", `new-window -c "#{pane_current_path}"`},
		{"", "cmd+w", "close this window", `confirm-before -p "close #W? (y/n)" kill-window`},
		{"", "cmd+shift+]", "the next window", `next-window`},
		{"", "cmd+shift+[", "the window before", `previous-window`},
	}
	for n := 1; n <= 9; n++ {
		keys = append(keys, tmuxKey{"", fmt.Sprintf("cmd+%d", n), fmt.Sprintf("window %d", n), fmt.Sprintf("select-window -t :%d", n)})
	}
	return keys
}

// terminalKeys is the lines a terminal app's config takes so its ⌘ keys send mia's: kitty's
// or ghostty's.
func terminalKeys(app string) (string, error) {
	var b strings.Builder
	for i, k := range tmuxKeys("mia", "mia") {
		if k.cmd == "" {
			continue
		}
		switch app {
		case "kitty":
			fmt.Fprintf(&b, "map %s send_text all \\x1b[99;%d~\n", k.cmd, i+1)
		case "ghostty":
			fmt.Fprintf(&b, "keybind = %s=text:\\x1b[99;%d~\n", strings.ReplaceAll(strings.ReplaceAll(k.cmd, "[", "left_bracket"), "]", "right_bracket"), i+1)
		default:
			return "", fmt.Errorf("mia tmux keys <kitty|ghostty>")
		}
	}
	return b.String(), nil
}

// cmdTmux is mia tmux [install|uninstall]: print, add or remove mia's part
// of ~/.tmux.conf. install shows what it adds before it adds it.
func cmdTmux(args []string) int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	if len(args) == 2 && args[0] == "keys" {
		lines, err := terminalKeys(args[1])
		if err != nil {
			return usageErr(err.Error())
		}
		fmt.Print(lines)
		return exitOK
	}
	snippet := tmuxSnippet(self)
	if len(args) == 0 {
		fmt.Print(snippet)
		return exitOK
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}
	conf := filepath.Join(home, ".tmux.conf")
	current, err := os.ReadFile(conf)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	kept := withoutBlock(string(current))
	switch args[0] {
	case "install":
		fmt.Printf("adding to %s:\n\n%s\n", conf, snippet)
		if kept != "" && !strings.HasSuffix(kept, "\n") {
			kept += "\n"
		}
		kept += "\n" + snippet
	case "uninstall":
		fmt.Printf("removing mia's block from %s\n", conf)
	default:
		return usageErr("mia tmux [install|uninstall|keys <kitty|ghostty>]")
	}
	if err := os.WriteFile(conf, []byte(kept), 0o644); err != nil {
		return fail(err)
	}
	if os.Getenv("TMUX") != "" {
		if out, err := run.Local("tmux").Combined("source-file", conf); err != nil {
			return fail(fmt.Errorf("tmux source-file %s: %s", conf, strings.TrimSpace(out)))
		}
		fmt.Println("tmux has read it: prefix k the one list (somewhere to go, `pi fix it` to start an agent on it, `> codex` to hand this conversation on, a number to answer, words to find a past conversation, ? for the rest), K the popup terminal, a the next that needs you, b and B back and forward; `mia tmux keys kitty` prints the ⌘ keys for your terminal app")
	}
	return exitOK
}

// withoutBlock is the config with mia's block taken out, whatever else is
// in it kept as it was.
func withoutBlock(conf string) string {
	start := strings.Index(conf, tmuxMarker)
	if start < 0 {
		return conf
	}
	end := strings.Index(conf[start:], "# mia: end")
	if end < 0 {
		return conf
	}
	end += start + len("# mia: end")
	if end < len(conf) && conf[end] == '\n' {
		end++
	}
	return strings.TrimRight(conf[:start], "\n") + "\n" + conf[end:]
}

// cmdTmuxNext is mia tmux next <client>: prefix a. The client goes to the
// next worktree here that needs you, after the one it is in, round again
// at the end; best rank first.
func cmdTmuxNext(a *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia tmux next <client>")
	}
	local, err := a.List()
	if err != nil {
		return tmuxSay(args, err)
	}
	here := ""
	if record, err := a.RecordOrHere(""); err == nil {
		here = record.Name
	}
	next := nextNeedingYou(local, here)
	if next == "" {
		_, _ = run.Local("tmux").Combined("display-message", "-c", args[0], "mia: nothing here needs you")
		return exitOK
	}
	if err := a.SwitchClient(next, args[0]); err != nil {
		return tmuxSay(args, err)
	}
	return exitOK
}

// nextNeedingYou is the worktree after here among those that need you — whose best signal
// calls for you — in rank order then by name; "" when none does but here.
// ponytail: this computer's alone; a machine's is reached from the one list.
func nextNeedingYou(local []app.Listing, here string) string {
	var waiting []app.Listing
	for _, l := range local {
		if len(l.Signals) > 0 && l.Signals[0].Calls() {
			waiting = append(waiting, l)
		}
	}
	slices.SortFunc(waiting, func(x, y app.Listing) int {
		if c := x.Signals[0].Rank - y.Signals[0].Rank; c != 0 {
			return c
		}
		return strings.Compare(x.Name, y.Name)
	})
	at := slices.IndexFunc(waiting, func(l app.Listing) bool { return l.Name == here })
	for i := 1; i <= len(waiting); i++ {
		if l := waiting[(at+i+len(waiting))%len(waiting)]; l.Name != here {
			return l.Name
		}
	}
	return ""
}

// tmuxSay shows what went wrong on the client that pressed a key: a
// background run-shell has nowhere else to say it.
func tmuxSay(args []string, err error) int {
	if len(args) == 1 {
		_, _ = run.Local("tmux").Combined("display-message", "-c", args[0], "mia: "+err.Error())
	}
	return fail(err)
}

// askYes asks on the terminal and is true only for y.
func askYes(question string) bool {
	fmt.Print(question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line) == "y"
}

// pressKey keeps a popup's last words up until ⏎.
func pressKey() {
	fmt.Print("\n⏎ close")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
