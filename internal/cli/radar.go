package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/radar"
	"github.com/emusoi/mia-core/internal/run"
)

// cmdRadar is mia radar [--new]: the popup that shows and finds everything,
// or straight to making a new worktree. What it picks runs once it closes.
func cmdRadar(a *app.App, args []string) int {
	newOnly := take(&args, "--new")
	pickTo := takeValue(&args, "--pick-to")
	data, err := radarData(a)
	if err != nil {
		return fail(err)
	}
	if a.PollDue(pollEvery) {
		pollInBackground(a.Root)
	}
	choice, err := radar.Run(data, newOnly)
	if err != nil || choice == nil {
		return exitFor(err)
	}
	if pickTo != "" && choice.Name != "" && len(choice.Argv) == 0 {
		// picked for someone else — an editor — to go to: say where, do not switch
		path, err := a.RecordOrHere(choice.Name)
		if err != nil {
			return fail(err)
		}
		return exitFor(os.WriteFile(pickTo, []byte(path.Path+"\n"), 0o600))
	}
	return exitFor(carryOut(a, *choice))
}

func radarData(a *app.App) (radar.Data, error) {
	local, err := a.List()
	if err != nil {
		return radar.Data{}, err
	}
	data := radar.Data{Repo: filepath.Base(a.Root), Root: a.Root, MiaDir: a.MiaDir, Base: a.BaseBranch(a.Root),
		Prefix: a.Config.Prefix, Local: local, Machines: a.KnownMachines()}
	if record, err := a.RecordOrHere(""); err == nil {
		data.Here, data.HereDir = record.Name, record.Path
		data.Branch = git.CurrentBranch(record.Path)
	}
	data.Away = a.ReadNote()
	for _, s := range a.Sessions() {
		if s.State != "running" {
			if data.Down == nil {
				data.Down = map[string]string{}
			}
			data.Down[s.Name] = s.State
		}
	}
	if plugins, err := enabledPlugins(a); err == nil {
		for _, p := range plugins {
			for _, v := range p.Manifest.Verbs {
				data.Verbs = append(data.Verbs, radar.Verb{Name: v.Name, Help: v.Help})
			}
		}
	}
	if _, err := os.Stat(filepath.Join(a.MiaDir, "config.toml")); os.IsNotExist(err) {
		data.First = &radar.Guess{Base: data.Base, Setup: radar.GuessSetup(a.Root)}
	}
	return data, nil
}

// carryOut does what the radar picked: a mia command, then the switch.
func carryOut(a *app.App, choice radar.Choice) error {
	if len(choice.Argv) > 0 {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(self, choice.Argv...)
		cmd.Dir = choice.Dir
		if cmd.Dir == "" {
			cmd.Dir = a.Root
		}
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err = cmd.Run()
		if choice.Wait {
			fmt.Print("\n⏎ back")
			_, _ = fmt.Scanln()
		}
		if err != nil {
			return err
		}
	}
	switch {
	case choice.Return != "":
		return goBack(choice.Return)
	case choice.Remote != "":
		if os.Getenv("TMUX") == "" {
			return cmdErr(cmdAttachOn(a, []string{choice.Remote}))
		}
		return handOver(a, choice.Remote)
	case choice.Name != "":
		os.Unsetenv("TMUX_PANE") // from a popup, switch the client rather than nest
		return a.Shell(choice.Name, false)
	}
	return nil
}

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

func cmdErr(code int) error {
	if code != exitOK {
		return fmt.Errorf("exit %d", code)
	}
	return nil
}

const tmuxMarker = "# mia: status line and popups"

// tmuxSnippet is what mia adds to a tmux config: its part of the status
// line, refreshed every 5s, the two popups, and its menu.
func tmuxSnippet(self string) string {
	mia := run.ShellJoin([]string{self})
	in := `cd \"#{pane_current_path}\" && ` + mia
	var keys strings.Builder
	for i, k := range tmuxKeys(mia, in) {
		// (Each on a prefix key, and on a key of its own the terminal app can send: `mia tmux keys`.)
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
// app sends it by (`mia tmux keys`), what it is called, and the tmux command it runs.
type tmuxKey struct{ prefix, cmd, name, does string }

// tmuxKeys is every key mia binds, in the order their user keys are numbered: never reordered,
// only added to, so a terminal app's config keeps meaning what it meant.
func tmuxKeys(mia, in string) []tmuxKey {
	keys := []tmuxKey{
		{"k", "cmd+k", "the one list: go somewhere, start an agent, hand on, answer, resume", `run-shell -b "` + mia + ` tmux list --popup '#{client_name}' '#{window_index}' '#{pane_current_path}'"`},
		{"K", "cmd+o", "the popup terminal", `run-shell -b "` + mia + ` tmux shell --popup '#{client_name}' '#{pane_current_path}'"`},
		{"Space", "cmd+shift+j", "who needs you", `display-popup -E -d "#{pane_current_path}" -w 72 -h 60% -T " mia " "` + mia + ` radar"`},
		{"n", "cmd+n", "a new worktree", `display-popup -E -d "#{pane_current_path}" -w 110 -h 14 -T " new worktree " "` + mia + ` radar --new"`},
		{"m", "", "mia's menu", `run-shell -b "` + in + ` tmux menu '#{client_name}'"`},
		{"a", "cmd+j", "the next that needs you", `run-shell -b "` + in + ` tmux next '#{client_name}'"`},
		{"e", "cmd+.", "what is done to this window", `run-shell -b "` + mia + ` tmux act '#{client_name}' '#{window_index}'"`},
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

// cmdTmuxAct is mia tmux act <client> <window>: tmux's own menu of what is done to the window
// in front, a key each, in three parts: its agent, the window, its worktree.
func cmdTmuxAct(args []string) int {
	if len(args) != 2 {
		return usageErr("mia tmux act <client> <window>")
	}
	self, err := os.Executable()
	if err != nil {
		return tmuxSay(args[:1], err)
	}
	mia := run.ShellJoin([]string{self})
	in := `cd "#{pane_current_path}" && ` + mia
	argv := []string{"display-menu", "-c", args[0], "-T", " this window ",
		"Stop the agent", "s", "send-keys Escape",
		"Move to another agent", "a", fmt.Sprintf("run-shell -b '%s tmux agents %s hand %s'", mia, args[0], args[1]),
		"New agent window", "n", fmt.Sprintf("run-shell -b '%s tmux agents %s new %s'", mia, args[0], args[1]),
		"Its model", "m", fmt.Sprintf(`command-prompt -p "model" "run-shell -b '%s window model %s %%%%'"`, in, args[1]),
		"",
		"Rename", "r", `command-prompt -I "#W" "rename-window -- '%%'"`,
		"Close", "x", `confirm-before -p "close #W? (y/n)" kill-window`,
		"",
		"Fork into a new worktree", "f", fmt.Sprintf(`command-prompt -p "branch" "run-shell -b '%s fork %%%%'"`, in),
		"lazygit", "g", `new-window -c "#{pane_current_path}" lazygit`,
		"Editor", "o", `new-window -c "#{pane_current_path}" "${EDITOR:-nvim} ."`,
	}
	if out, err := run.Local("tmux").Combined(argv...); err != nil {
		return tmuxSay(args[:1], fmt.Errorf("tmux display-menu: %s", strings.TrimSpace(out)))
	}
	return exitOK
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
		fmt.Println("tmux has read it: prefix k the one list (somewhere to go, `pi fix it` to start an agent on it, `> codex` to hand this conversation on, a number to answer, words to find a past conversation), K the popup terminal, space who needs you, a the next of them, e this window's menu, n a new worktree, m mia's menu, b and B back and forward; `mia tmux keys kitty` prints the ⌘ keys for your terminal app")
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
