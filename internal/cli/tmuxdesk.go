package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/run"
)

// mia in tmux with no window of its own (design/tmux/PLAN.md): a popup terminal on one shell
// that lasts (`mia tmux shell`), `mia go` and so `mia cd` taking the tmux client to a
// worktree's session, tmux's own menus for what is done to the window in front (move its
// conversation to another agent, start another agent beside it), `mia asks` for who waits on
// you, and back and forward through the sessions a client has been in.

// popupSession is tmux's session that holds the popup terminal's shell.
const popupSession = "mia-shell"

// tmuxSaid is what tmux answers a format with, about the pane it is asked from.
func tmuxSaid(format string, target ...string) (string, error) {
	args := []string{"display-message", "-p"}
	if len(target) == 1 && target[0] != "" {
		args = append(args, "-t", target[0])
	}
	out, err := run.Local("tmux").Capture(append(args, format)...)
	return strings.TrimSpace(out), err
}

// outerClient is the tmux client a command typed here moves: the one it is typed in, or typed
// in the popup terminal, the client the popup is over (true: it is the popup's).
func outerClient() (client string, popup bool, err error) {
	if os.Getenv("TMUX") == "" {
		return "", false, fmt.Errorf("not in tmux: `mia cd` changes folder here, and a session is gone to from inside tmux")
	}
	said, err := tmuxSaid("#{session_name}\t#{client_name}", os.Getenv("TMUX_PANE"))
	if err != nil {
		return "", false, err
	}
	session, client, _ := strings.Cut(said, "\t")
	if session != popupSession {
		return client, false, nil
	}
	over, err := run.Local("tmux").Capture("show-options", "-v", "-t", "="+popupSession+":", "@mia_client")
	if err != nil || strings.TrimSpace(over) == "" {
		return "", true, fmt.Errorf("the popup terminal does not know which client it is over")
	}
	return strings.TrimSpace(over), true, nil
}

// cmdGo is mia go <words>: the tmux client goes to the session of the worktree the words mean
// (as `mia cd` means them), started if it is not up. Typed in the popup terminal, the client
// the popup is over goes, and the popup shuts. `mia cd` in tmux does this after it changes
// folder.
func cmdGo(args []string) int {
	quiet := take(&args, "--quiet") // `mia cd`'s: a folder that is no worktree of mia's is only a folder
	if len(args) != 1 {
		return usageErr("mia go <worktree|project|project/worktree>")
	}
	client, popup, err := outerClient()
	if err != nil {
		if quiet {
			return exitOK
		}
		return fail(err)
	}
	var here *app.App
	if dir, err := os.Getwd(); err == nil {
		here, _ = app.Open(dir)
	}
	repos, err := app.Repositories()
	if err != nil {
		return fail(err)
	}
	path, err := whereTo(here, args[0], repos)
	if err != nil {
		if quiet {
			return exitOK
		}
		return fail(err)
	}
	a, err := app.Open(path)
	if err != nil {
		if quiet {
			return exitOK
		}
		return fail(err)
	}
	if err := a.SwitchClient(path, client); err != nil {
		if quiet {
			return exitOK
		}
		return fail(err)
	}
	if popup {
		_, _ = run.Local("tmux").Combined("display-popup", "-C", "-c", client)
	}
	return exitOK
}

// cmdTmuxShell is mia tmux shell <client> [folder]: what the popup terminal runs. Its shell is
// one session that lasts, made the first time; it is told which client it is over, and at rest
// (no program run in it) it is brought to the folder the popup was opened in; then this
// process becomes tmux attached to it.
func cmdTmuxShell(args []string) int {
	popup := take(&args, "--popup")
	if len(args) < 1 || len(args) > 2 {
		return usageErr("mia tmux shell [--popup] <client> [folder]")
	}
	tmux := run.Local("tmux")
	if popup { // (The key runs this through run-shell: display-popup does not fill #{…} in its command.)
		self, err := os.Executable()
		if err != nil {
			return fail(err)
		}
		// (What it answers is how the popup ended, shut by `mia cd` among them: nothing to say.)
		_, _ = tmux.Combined(append([]string{"display-popup", "-c", args[0], "-E", "-w", "84", "-h", "18", "-T", " mia ", self, "tmux", "shell"}, args...)...)
		return exitOK
	}
	target := "=" + popupSession + ":"
	if _, err := tmux.Combined("has-session", "-t", "="+popupSession); err != nil {
		home, _ := os.UserHomeDir()
		if out, err := tmux.Combined("new-session", "-d", "-s", popupSession, "-c", home); err != nil {
			return fail(fmt.Errorf("the popup terminal's session: %s", strings.TrimSpace(out)))
		}
		_, _ = tmux.Combined("set-option", "-t", target, "status", "off")
	}
	_, _ = tmux.Combined("set-option", "-t", target, "@mia_client", args[0])
	if len(args) == 2 && args[1] != "" {
		if said, err := tmuxSaid("#{pane_current_path}\t#{pane_current_command}", target); err == nil {
			dir, runs, _ := strings.Cut(said, "\t")
			if line := comeLine(dir, runs, args[1]); line != "" {
				_, _ = tmux.Combined("send-keys", "-t", target, "-l", line)
				_, _ = tmux.Combined("send-keys", "-t", target, "Enter")
			}
		}
	}
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return fail(err)
	}
	env := slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(e, "TMUX=") })
	return fail(syscall.Exec(bin, []string{"tmux", "attach-session", "-t", "=" + popupSession}, env))
}

// comeLine is what is typed into a shell in dir, running runs, to bring it to the folder to:
// nothing where it is there already or runs a program. In zsh what was typed and not sent is
// pushed aside and comes back (^Q, push-line); another shell's line is cleared (^U). A space
// first keeps it out of the shell's history.
func comeLine(dir, runs, to string) string {
	shell := strings.TrimPrefix(runs, "-")
	if !slices.Contains([]string{"zsh", "bash", "fish", "sh"}, shell) || dir == to {
		return ""
	}
	aside := "\x15"
	if shell == "zsh" {
		aside = "\x11"
	}
	return aside + " cd -- '" + strings.ReplaceAll(to, "'", `'\''`) + "'"
}

// agentKnown is an agent as `mia api agents` says it: its name, its models, whether it is here.
type agentKnown struct {
	Name   string `json:"name"`
	Models []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"models"`
	Installed bool `json:"installed"`
}

// apiSaid is what this mia answers an api query with, asked as a client would: of the server
// when it runs, so nothing is worked out twice.
func apiSaid(self string, into any, query ...string) error {
	out, err := exec.Command(self, append([]string{"api"}, query...)...).Output()
	if err != nil {
		return fmt.Errorf("mia api %s: %w", strings.Join(query, " "), err)
	}
	return json.Unmarshal(out, into)
}

// cmdTmuxAgents is mia tmux agents <client> <hand|new> <window> [agent]: tmux's own menu of
// the agents installed, a letter each. Chosen, its models are the next menu, the first "as it
// is set"; chosen there, `hand` moves the window's conversation to that agent and `new` starts
// it in a window beside.
func cmdTmuxAgents(args []string) int {
	if len(args) < 3 || len(args) > 4 || (args[1] != "hand" && args[1] != "new") {
		return usageErr("mia tmux agents <client> <hand|new> <window> [agent]")
	}
	client, what, window := args[0], args[1], args[2]
	self, err := os.Executable()
	if err != nil {
		return tmuxSay(args[:1], err)
	}
	var agents []agentKnown
	if err := apiSaid(self, &agents, "agents"); err != nil {
		return tmuxSay(args[:1], err)
	}
	agents = slices.DeleteFunc(agents, func(a agentKnown) bool { return !a.Installed })
	if out, err := run.Local("tmux").Combined(append([]string{"display-menu", "-c", client}, agentsMenu(run.ShellJoin([]string{self}), client, what, window, args[3:], agents)...)...); err != nil {
		return tmuxSay(args[:1], fmt.Errorf("tmux display-menu: %s", strings.TrimSpace(out)))
	}
	return exitOK
}

// agentsMenu is display-menu's arguments: with no agent chosen the agents, each opening its
// models; with one, its models, each doing the deed.
func agentsMenu(mia, client, what, window string, chosen []string, agents []agentKnown) []string {
	title := map[string]string{"hand": " move this conversation to ", "new": " new agent window "}[what]
	here := `cd "#{pane_current_path}" && ` + mia
	if len(chosen) == 0 {
		argv := []string{"-T", title}
		taken := map[string]bool{}
		for _, a := range agents {
			argv = append(argv, a.Name, menuKey(a.Name, taken), fmt.Sprintf("run-shell -b '%s tmux agents %s %s %s %s'", mia, client, what, window, a.Name))
		}
		return argv
	}
	// (The window it makes is the session's last: you are put in it, and what the verb says of
	// its doing is not shown over the pane, only what goes wrong.)
	const front = ` >/dev/null && tmux select-window -t :{end}`
	deed := func(model string) string {
		with := ""
		if model != "" {
			with = " --model " + model
		}
		if what == "hand" {
			return fmt.Sprintf("run-shell -b '%s hand --window %s %s%s%s'", here, window, chosen[0], with, front)
		}
		return fmt.Sprintf("run-shell -b '%s window new %s%s%s'", here, chosen[0], with, front)
	}
	argv := []string{"-T", " " + chosen[0] + " ", "as it is set", "Enter", deed("")}
	taken := map[string]bool{}
	for _, a := range agents {
		if a.Name != chosen[0] {
			continue
		}
		if len(a.Models) > 0 {
			argv = append(argv, "")
		}
		for _, m := range a.Models {
			argv = append(argv, m.Label, menuKey(m.ID, taken), deed(m.ID))
		}
	}
	return argv
}

// menuKey is the first letter of name no row before took, lower case: the key that chooses it.
func menuKey(name string, taken map[string]bool) string {
	for _, r := range strings.ToLower(name) {
		if key := string(r); r >= 'a' && r <= 'z' && !taken[key] {
			taken[key] = true
			return key
		}
	}
	return ""
}

// cmdHand is mia hand [--window <index>] <agent> [--model m] [--effort e] [--account a]: the
// conversation of the window in front (or the one named) goes on with another agent, as
// `mia conversation continue --from <window> --with <agent>` does it.
func cmdHand(a *app.App, args []string, asJSON bool) int {
	const form = "mia hand [--window <index>] <agent> [--model <model>] [--effort <effort>] [--account <account>]"
	window := takeValue(&args, "--window")
	model, effort, account := takeValue(&args, "--model"), takeValue(&args, "--effort"), takeValue(&args, "--account")
	if len(args) != 1 {
		return usageErr(form)
	}
	if window == "" {
		if os.Getenv("TMUX") == "" {
			return usageErr(form + "   (outside tmux, say which window)")
		}
		said, err := tmuxSaid("#{window_index}", os.Getenv("TMUX_PANE"))
		if err != nil {
			return fail(err)
		}
		window = said
	}
	return continueConversation(a, app.Continue{Window: window, With: args[0], Model: model, Effort: effort, Account: account}, asJSON)
}

// cmdAsks is mia asks [n [answer]]: who waits on you, in every project, the longest first,
// each with its question and its numbered answers. `mia asks 2` goes to the second (in tmux);
// `mia asks 2 1` gives its first answer, typed into the window that asks.
func cmdAsks(args []string, asJSON bool) int {
	const form = "mia asks [n [answer]]"
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	var standing app.Standing
	if err := apiSaid(self, &standing, "standing"); err != nil {
		return fail(err)
	}
	waiting := standing.Waiting
	slices.SortStableFunc(waiting, func(x, y app.Stand) int { return x.Since.Compare(y.Since) })
	if len(args) == 0 {
		if asJSON {
			return emit(waiting)
		}
		if len(waiting) == 0 {
			fmt.Println("nobody is waiting on you")
			return exitOK
		}
		for i, w := range waiting {
			fmt.Printf("%d  %s  %s  %s\n", i+1, w.Worktree, w.Repository, w.Agent)
			fmt.Printf("   %s\n", w.Line)
			for n, option := range w.Options {
				fmt.Printf("     %d  %s\n", n+1, option)
			}
		}
		return exitOK
	}
	if len(args) > 2 {
		return usageErr(form)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < 1 || n > len(waiting) {
		return fail(fmt.Errorf("%d are waiting — `mia asks` lists them", len(waiting)))
	}
	w := waiting[n-1]
	if len(args) == 1 {
		if err := os.Chdir(w.Root); err != nil {
			return fail(err)
		}
		return cmdGo([]string{w.Worktree})
	}
	answer, err := strconv.Atoi(args[1])
	if err != nil || answer < 1 || answer > max(len(w.Options), 1) || w.Window == nil {
		return fail(fmt.Errorf("%s has %d answers — `mia asks` shows them", w.Worktree, len(w.Options)))
	}
	press := exec.Command(self, "window", "press", w.Worktree, strconv.Itoa(w.Window.Index), "--", strconv.Itoa(answer))
	press.Dir, press.Stdout, press.Stderr = w.Root, os.Stdout, os.Stderr
	if err := press.Run(); err != nil {
		return exitFailed
	}
	fmt.Printf("%s: %s\n", w.Worktree, optionOr(w.Options, answer))
	return exitOK
}

func optionOr(options []string, n int) string {
	if n >= 1 && n <= len(options) {
		return options[n-1]
	}
	return "answered " + strconv.Itoa(n)
}

// trail is the sessions a tmux client has been in, oldest first, and where in them it is:
// kept in mia's config folder, a file a client.
type trail struct {
	Sessions []string `json:"sessions"`
	At       int      `json:"at"`
}

func trailPath(client string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, client)
	return filepath.Join(config.Dir(), "trails", safe+".json")
}

func readTrail(client string) trail {
	var t trail
	if data, err := os.ReadFile(trailPath(client)); err == nil {
		_ = json.Unmarshal(data, &t)
	}
	if t.At < 0 || t.At >= len(t.Sessions) {
		t.At = max(len(t.Sessions)-1, 0)
	}
	return t
}

func writeTrail(client string, t trail) error {
	path := trailPath(client)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// trailKeep is the most sessions a client's trail holds: the oldest go.
const trailKeep = 50

// seen is the trail after the client came to session: nothing new where it is the one it is
// at (a walk brought it there); else what was ahead is let go and it is the newest.
func (t trail) seen(session string) trail {
	if len(t.Sessions) > 0 && t.Sessions[t.At] == session {
		return t
	}
	next := append(slices.Clone(t.Sessions[:min(t.At+1, len(t.Sessions))]), session)
	if len(next) > trailKeep {
		next = next[len(next)-trailKeep:]
	}
	return trail{Sessions: next, At: len(next) - 1}
}

// cmdTmuxSeen is mia tmux seen <client> <session>: tmux's hook, each time a client comes to a
// session. The popup terminal's own session is no place been.
func cmdTmuxSeen(args []string) int {
	if len(args) != 2 {
		return usageErr("mia tmux seen <client> <session>")
	}
	if args[1] == popupSession {
		return exitOK
	}
	return exitFor(writeTrail(args[0], readTrail(args[0]).seen(args[1])))
}

// cmdTmuxWalk is mia tmux walk <client> <back|forward>: the client goes to the session before
// or after the one it is at in its trail, past any that is gone; at the end, a line says so.
func cmdTmuxWalk(args []string) int {
	if len(args) != 2 || (args[1] != "back" && args[1] != "forward") {
		return usageErr("mia tmux walk <client> <back|forward>")
	}
	client, by := args[0], 1
	if args[1] == "back" {
		by = -1
	}
	t := readTrail(client)
	tmux := run.Local("tmux")
	for at := t.At + by; at >= 0 && at < len(t.Sessions); at += by {
		if _, err := tmux.Combined("has-session", "-t", "="+t.Sessions[at]); err != nil {
			continue
		}
		t.At = at
		if err := writeTrail(client, t); err != nil {
			return tmuxSay(args[:1], err)
		}
		if out, err := tmux.Combined("switch-client", "-c", client, "-t", "="+t.Sessions[at]); err != nil {
			return tmuxSay(args[:1], fmt.Errorf("%s", strings.TrimSpace(out)))
		}
		return exitOK
	}
	_, _ = tmux.Combined("display-message", "-c", client, "mia: nowhere further "+args[1])
	return exitOK
}
