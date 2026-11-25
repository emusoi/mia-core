package cli

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/resolve"

	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/signal"
)

var version = "dev"

func versionOf(stamped string) string {
	info, ok := debug.ReadBuildInfo()
	if stamped != "dev" || !ok {
		return stamped
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	revision := settings["vcs.revision"]
	if len(revision) < 7 {
		return stamped
	}
	out := "dev " + revision[:7]
	if settings["vcs.modified"] == "true" {
		out += " with uncommitted changes"
	}
	if built := settings["vcs.time"]; built != "" {
		out += ", committed " + built
	}
	return out
}

const (
	exitOK       = 0
	exitFailed   = 1
	exitNotFound = 3
	exitUsage    = 64
)

func Run(args []string) int {
	if len(args) == 0 {
		usage(os.Stdout)
		return exitOK
	}
	verb, rest := args[0], args[1:]
	asJSON := false
	if !passesArgumentsThrough(verb) {
		asJSON = take(&rest, "--json")
		if take(&rest, "-h") || take(&rest, "--help") {
			verbUsage(os.Stdout, verb)
			return exitOK
		}
	}

	if slices.Contains([]string{"ls", "list"}, verb) && len(rest) > 0 {
		return usageErr(usageOf(verb))
	}

	switch verb {
	case "help", "-h", "--help":
		if len(rest) > 0 && rest[0] == "vocabulary" {
			return printVocabulary()
		}
		if len(rest) > 0 && rest[0] == "tmux" {
			return printTmuxHelp()
		}
		usage(os.Stdout)
		return exitOK
	case "version", "--version":
		fmt.Println(versionOf(version))
		return exitOK
	case "shell-init":
		return cmdShellInit(rest)
	case "shell-integration":
		return cmdShellIntegration(rest)
	case "gateway":
		return cmdGateway(rest, asJSON)
	case "serve":
		return cmdServe(rest)
	case "status-line":
		return cmdStatusLine(rest)
	case "tmux":
		if len(rest) > 0 {
			switch rest[0] { // what tmux's keys run, from any folder
			case "shell":
				return cmdTmuxShell(rest[1:])
			case "list":
				return cmdTmuxList(rest[1:])
			case "agents":
				return cmdTmuxAgents(rest[1:])
			case "act":
				return cmdTmuxAct(rest[1:])
			case "seen":
				return cmdTmuxSeen(rest[1:])
			case "walk":
				return cmdTmuxWalk(rest[1:])
			}
		}
		if len(rest) == 0 || (rest[0] != "menu" && rest[0] != "next") {
			return cmdTmux(rest)
		}
	case "go": // the tmux client to a worktree's session, by its name, from any folder
		return cmdGo(rest)
	case "recall": // the conversations that mention it: here, or in every project
		return cmdRecall(rest, asJSON)
	case "asks": // who waits on you, in every project
		return cmdAsks(rest, asJSON)
	case "machine", "machines", "runtime":
		if len(rest) == 0 || (rest[0] != "attach" && rest[0] != "poll" && rest[0] != "link") {
			return cmdRuntime(rest, asJSON)
		}
	case "plugin", "plugins":
		return cmdPlugin(configHere(), rest, asJSON)
	case "doctor":
		return cmdDoctor(rest, asJSON)
	case "main":
		return cmdMain(rest)
	case "chats":
		return cmdChats(rest)
	case "chat":
		if slices.Contains(rest, "--chat") { // a chat of mia's: from any folder
			return cmdChatOfChat(rest)
		}
	case "account", "accounts":
		return cmdAccount(rest, asJSON)
	case "role", "roles":
		return cmdRole(rest, asJSON)
	case "ls", "list": // the projects mia knows, from any folder: a project's worktrees are `mia p ls`
		if len(rest) > 0 {
			return usageErr(usageOf(verb))
		}
		return cmdProjects(asJSON)
	case "project", "prj", "pj", "p": // a project's worktrees: the one here, or one named from anywhere
		return cmdProject(rest, asJSON)
	case "git": // from any folder: one in no repository is shown as that
		return cmdGit(rest)
	case "files": // from any folder too: one in no repository is its own tree
		return cmdFiles(rest)
	case "review": // from any folder too: what is not a worktree is shown as that
		return cmdReview(rest)
	case "api":
		if code, asked := askServer(rest); asked {
			return code
		}
		return answerAPI(atTerminal(), rest)
	}

	here, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	a, err := app.Open(here)
	if err != nil {
		if verb == "tmux" { // prefix m, where a printed error is seen by no one
			return tmuxSay(rest[1:], err)
		}
		// In no repository: the projects mia knows, and the way to one by its name.
		if errors.Is(err, git.ErrNotARepository) {
			switch {
			case (verb == "path" || verb == "switch") && len(rest) == 1:
				return cmdPathAnywhere(nil, rest)
			case verb == "__complete" && len(rest) == 1 && rest[0] == "names":
				return completeProjects()
			}
		}
		return fail(err)
	}
	a.Events = func(e app.Event) { sendEvent(a, e) }
	a.Plugins = pluginHost(a.Config)

	switch verb {
	case "tmux":
		if rest[0] == "next" {
			return cmdTmuxNext(a, rest[1:])
		}
		return cmdTmuxMenu(a, rest[1:])
	case "signal", "signals":
		return cmdSignal(a, rest, asJSON)
	case "radar":
		return cmdRadar(a, rest)
	case "away":
		return cmdAway(a, rest)
	case "move":
		return cmdMove(a, rest)
	case "archive":
		if len(rest) == 1 && (rest[0] == "ls" || rest[0] == "list") {
			all, err := a.Archives()
			if err != nil {
				return fail(err)
			}
			if asJSON {
				return emit(all)
			}
			if len(all) == 0 {
				fmt.Println("nothing archived")
			}
			for _, one := range all {
				kept := ""
				if one.Dirty {
					kept = "  ✎ with uncommitted work"
				}
				fmt.Printf("%-14s %-28s %s%s\n", one.Name, one.Branch, one.At.Local().Format("2006-01-02 15:04"), kept)
			}
			return exitOK
		}
		stop := take(&rest, "--stop-running")
		if len(rest) != 1 {
			return usageErr("mia archive [--stop-running] <worktree>  ·  mia archive ls")
		}
		done, err := a.Archive(rest[0], stop)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s archived — its branch %s stays; `mia unarchive %s` brings it back\n", done.Name, done.Branch, done.Name)
		return exitOK
	case "fork":
		if len(rest) < 1 || len(rest) > 2 {
			return usageErr("mia fork [worktree] <branch>")
		}
		from, branch := "", rest[len(rest)-1]
		if len(rest) == 2 {
			from = rest[0]
		}
		record, err := a.Fork(from, branch)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s — %s, forked with its uncommitted work, at %s\n", record.Name, branch, record.Path)
		return exitOK
	case "unarchive":
		if len(rest) != 1 {
			return usageErr("mia unarchive <name>")
		}
		record, err := a.Unarchive(rest[0])
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s is back at %s\n", record.Name, record.Path)
		return exitOK
	case "conversation", "conversations":
		return cmdConversation(a, rest, asJSON)
	case "why":
		if len(rest) != 1 {
			return usageErr("mia why <file:line|commit>")
		}
		why, err := a.Why(rest[0])
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(why)
		}
		fmt.Printf("%s %s\n", why.Commit[:7], why.Subject)
		if len(why.Conversations) == 0 {
			fmt.Println("  no conversation of claude's here shows it: made by hand, -q, or on another machine")
		}
		for _, c := range why.Conversations {
			how := "first seen in"
			if c.Made {
				how = "made in"
			}
			fmt.Printf("\n  %s %s · %s\n", how, c.Session, c.At.Local().Format("2006-01-02 15:04"))
			if c.Asked != "" {
				asked := strings.Join(strings.Fields(c.Asked), " ")
				if r := []rune(asked); len(r) > 160 {
					asked = string(r[:160]) + "…"
				}
				fmt.Printf("  you asked: %s\n", asked)
			}
			fmt.Printf("  cd %s && claude --resume %s\n", c.Cwd, c.Session)
		}
		return exitOK
	case "resurrect":
		dry := take(&rest, "--dry-run")
		if take(&rest, "--ask") {
			defer pressKey() // from the tmux menu's popup: stay up until ⏎
		}
		if take(&rest, "--list") {
			if asJSON {
				return emit(a.Sessions())
			}
			for _, s := range a.Sessions() {
				var what []string
				for _, w := range s.Windows {
					what = append(what, w.What())
				}
				seen := ""
				if !s.Seen.IsZero() {
					seen = s.Seen.Local().Format("Jan 2 15:04")
				}
				fmt.Printf("%-16s %-8s %-13s %s\n", s.Name, s.State, seen, strings.Join(what, " · "))
			}
			return exitOK
		}
		revived, err := a.Resurrect(dry, rest...)
		for _, r := range revived {
			var what []string
			for _, w := range r.Windows {
				what = append(what, w.What())
			}
			did := "brought back"
			if dry {
				did = "would bring back"
			}
			fmt.Printf("%s %s: %s\n", did, r.Name, strings.Join(what, " · "))
		}
		if err == nil && len(revived) == 0 {
			if len(rest) > 0 {
				fmt.Println("already running")
			} else {
				fmt.Println("nothing went down with tmux — mia resurrect --list shows every session mia remembers")
			}
		}
		return exitFor(err)
	case "machine", "machines", "runtime":
		if rest[0] == "poll" {
			_ = a.Remember() // what resurrect opens again after tmux goes down
			sendEvent(a, app.Event{Name: "poll"})
			// asking leaves each machine's answer where the status line reads
			// it; each machine linked here is told, in turn, what this computer has
			for _, state := range a.Machines() {
				if state.Answers && state.Name != runtime.Local && git.HasRemote(a.Root, state.Name) {
					_ = a.LeaveNote(state.Machine)
				}
			}
			return exitOK
		}
		if rest[0] == "link" {
			if len(rest) != 2 {
				return usageErr("mia machine link <name>")
			}
			machine, err := a.Runtimes.Get(rest[1])
			if err != nil {
				return fail(err)
			}
			dir, err := a.Link(machine)
			if err != nil {
				return fail(err)
			}
			fmt.Printf("%s has this repository at ~/%s, its base branch pushed\n", machine.Name, dir)
			return exitOK
		}
		return cmdAttachOn(a, rest[1:])
	case "new":
		return cmdNew(a, rest, asJSON)
	case "config":
		return cmdConfig(a, rest, asJSON)
	case "rename":
		if len(rest) != 2 || strings.TrimSpace(rest[1]) == "" {
			return usageErr("mia rename <worktree> <name>")
		}
		record, err := a.Rename(rest[0], rest[1])
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s is now %s\n", rest[0], record.Name)
		return exitOK
	case "window", "windows":
		return cmdWindow(a, rest, asJSON)
	case "chat":
		return cmdChat(a, rest)
	case "setup":
		target := ""
		if len(rest) == 1 {
			target = rest[0]
		} else if len(rest) > 1 {
			return usageErr("mia setup [worktree]")
		}
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		if err := a.Setup(record.Name); err != nil {
			return fail(err)
		}
		fmt.Printf("setup ran in %s\n", record.Name)
		return exitOK
	case "star":
		target := ""
		if len(rest) == 1 {
			target = rest[0]
		} else if len(rest) > 1 {
			return usageErr("mia star [worktree]")
		}
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		on, err := a.Star(record.Name)
		if err != nil {
			return fail(err)
		}
		if on {
			fmt.Printf("★ %s\n", record.Name)
		} else {
			fmt.Printf("%s unstarred\n", record.Name)
		}
		return exitOK
	case "hand":
		return cmdHand(a, rest, asJSON)
	case "path", "switch":
		if len(rest) == 1 { // a worktree here, else a project, loosely by name
			return cmdPathAnywhere(a, rest)
		}
		return cmdPath(a, rest)
	case "__complete":
		return cmdComplete(a, rest)
	case "gc":
		return cmdGC(a, rest, asJSON)
	case "adopt":
		return cmdAdopt(a, rest, asJSON)
	case "rm", "remove":
		return cmdRemove(a, rest)
	case "shell":
		return cmdShell(a, rest)
	case "run":
		return cmdRun(a, rest)
	case "env":
		return cmdEnv(a, rest, asJSON)
	case "dev":
		return cmdDev(a, rest, asJSON)
	case "dash":
		return cmdDash(a, rest)
	case "stack":
		return cmdStack(a, rest, asJSON)
	case "up":
		return cmdUp(a)
	case "down":
		return cmdDown(a)
	case "open":
		return cmdOpen(a, rest)
	default:
		if code, ok := runPluginVerb(a, verb, args[1:]); ok {
			return code
		}
		fmt.Fprintf(os.Stderr, "mia: unknown command %q\n\n", verb)
		usage(os.Stderr)
		return exitUsage
	}
}

func passesArgumentsThrough(verb string) bool {
	switch verb {
	case "run", "env", "main", "chats":
		return true
	case "help", "-h", "--help", "version", "--version", "__complete", "list", "remove", "windows", "plugins":
		return false
	}
	_, core := groupOf[verb]
	return !core
}

func cmdNew(a *app.App, args []string, asJSON bool) int {
	asLayer := take(&args, "--stack")
	in := takeValue(&args, "--in")
	own := take(&args, "--worktree")
	andShell := take(&args, "--shell")
	on := takeValue(&args, "--on")
	from := takeValue(&args, "--from")
	bg := take(&args, "--bg")
	name := takeValue(&args, "--name")
	pr, remote := takeValue(&args, "--pr"), takeValue(&args, "--remote")
	const usage = "mia new [--on <machine>] [--stack [--in <worktree>] [--worktree]] [--from <ref>] [--name <name>] [--bg] [--shell] <branch>\n       mia new --pr <number> [--remote <remote>] [--name <name>] [--bg] [--shell]"
	// A pull request to read: a worktree at its head, as pr-<number>. Its branch is yours and
	// tracks nothing, so nothing done there can be pushed to the author's fork by a slip.
	var head string
	if pr != "" {
		number, err := strconv.Atoi(pr)
		if err != nil || number <= 0 || len(args) != 0 || from != "" || asLayer || on != "" {
			return usageErr(usage)
		}
		_, sha, err := git.PullRequestHead(a.Root, number, remote)
		if err != nil {
			return fail(err)
		}
		args, from, head = []string{"pr-" + pr}, sha, sha
		if name == "" {
			name = "pr-" + pr
		}
	}
	if len(args) != 1 || remote != "" && pr == "" || ((from != "" || bg || name != "") && (asLayer || on != "")) {
		return usageErr(usage)
	}
	if strings.HasPrefix(args[0], "-") {
		return usageErr("mia new: unknown flag " + args[0])
	}
	if asLayer {
		code := newLayer(a, in, args[0], own)
		if code == exitOK {
			if record, err := a.RecordOrHere(in); err == nil {
				a.Emit("stack.changed", record, stackChange{Change: "layer"})
			}
		}
		return code
	}
	if on != "" && on != "-" && on != runtime.Local {
		machine, err := a.Runtimes.Get(on)
		if err != nil {
			return fail(err)
		}
		return exitFor(a.NewOn(machine, args[0]))
	}
	before, _ := a.Store.Load()
	setup := a.Config.Setup
	if bg {
		a.Config.Setup = nil // run below, in the session, not here
	}
	record, err := a.NewNamed(args[0], from, name)
	if err != nil {
		if record.Path == "" {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "mia: %v\n", err)
	}
	// Opened before, and the pull request has moved since: brought up to it when that loses
	// nothing (no work of yours on top, nothing uncommitted); otherwise said, and left.
	if head != "" && record.Path != "" && git.Head(record.Path) != head {
		dirty, _ := git.Dirty(record.Path)
		switch {
		case dirty || !git.IsAncestor(record.Path, "HEAD", head):
			fmt.Fprintf(os.Stderr, "mia: pull request %s has moved to %.8s; %s has work of its own, so it was left where it is\n", pr, head, record.Name)
		default:
			if err := git.FastForward(record.Path, head); err != nil {
				fmt.Fprintf(os.Stderr, "mia: %v\n", err)
			}
		}
	}
	if bg && len(setup) > 0 && err == nil && !slices.ContainsFunc(before, func(one model.Record) bool { return one.Path == record.Path }) {
		if err := setupInWindow(a, record, setup); err != nil {
			fmt.Fprintf(os.Stderr, "mia: %s created, but setup did not start: %v\n", record.Name, err)
		} else {
			fmt.Fprintf(os.Stderr, "%s: setting up in its setup window\n", record.Name)
		}
	}
	_ = a.Remember() // its session, when its setup opened one, and what is left of the others
	if asJSON {
		return emit(record)
	}
	where := "in a worktree of its own"
	if slices.ContainsFunc(before, func(one model.Record) bool { return one.Path == record.Path }) {
		where = "is already checked out there"
	}
	fmt.Printf("%s — %s %s\n%s\n", record.Name, git.CurrentBranch(record.Path), where, record.Path)
	if andShell {
		return exitFor(a.Shell(record.Name, false))
	}
	return exitOK
}

func cmdList(a *app.App, asJSON bool) int {
	listings, err := a.List()
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(listings)
	}
	// One line each, cut to the terminal's width so a narrow one never wraps: whether its session
	// runs, its name, its branch (cut to a column), then what is worth knowing of it in a word each.
	sort.SliceStable(listings, func(i, j int) bool { return listings[i].Session && !listings[j].Session })
	width, branchWidth := 0, 0
	for _, l := range listings {
		width = max(width, lipgloss.Width(l.Name))
		branchWidth = max(branchWidth, lipgloss.Width(l.Branch))
	}
	branchWidth = min(max(branchWidth, len("(detached)")), 32)
	cols := 0
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil {
		cols = w
	}
	for _, l := range listings {
		marks := []string{}
		if len(l.Signals) > 0 {
			marks = append(marks, signalMark(l.Signals[0]))
		}
		if l.Dirty {
			marks = append(marks, "dirty")
		}
		if l.Ahead > 0 || l.Behind > 0 {
			marks = append(marks, fmt.Sprintf("+%d -%d", l.Ahead, l.Behind))
		}
		if l.Stack != "" {
			marks = append(marks, "stack")
		}
		if !l.Adopted && !l.Main {
			marks = append(marks, "not adopted")
		}
		branch := l.Branch
		if branch == "" {
			branch = "(detached)"
		}
		star, runs := " ", " "
		if l.Starred {
			star = "★"
		}
		if l.Session {
			runs = "●"
		}
		line := strings.TrimRight(fmt.Sprintf("%s%s %s  %s  %s", star, runs, padded(l.Name, width), padded(ansi.Truncate(branch, branchWidth, "…"), branchWidth), strings.Join(marks, "  ")), " ")
		if cols > 0 {
			line = ansi.Truncate(line, cols-1, "…")
		}
		fmt.Println(line)
	}
	return exitOK
}

// setupInWindow runs setup in a setup window of the worktree's session. The
// window stays, a shell under setup's output; a failed setup leaves a signal.
func setupInWindow(a *app.App, record model.Record, setup []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	script := `mia=$1 name=$2; shift 2; "$@"; s=$?
if [ $s -ne 0 ]; then "$mia" signal "$name" setup "failed: exit $s" --style red --rank 20 --from setup; fi
echo; echo "setup exited $s"; exec "${SHELL:-sh}"`
	_, err = a.NewWindow(record, "setup", session.What{Kind: session.KindShell}, append([]string{"sh", "-c", script, "sh", self, record.Name}, setup...))
	return err
}

// signalMark is a worktree's best signal on its ls row: its kind, and its text.
func signalMark(one signal.Signal) string {
	if one.Text == "" {
		return one.Kind
	}
	return one.Kind + ": " + one.Text
}

func cmdPath(a *app.App, args []string) int {
	if len(args) > 1 {
		return usageErr("mia path [worktree|stack]")
	}
	query := ""
	if len(args) == 1 {
		query = args[0]
	}
	path, err := a.Locate(query)
	if err != nil {
		return fail(err)
	}
	fmt.Println(path)
	return exitOK
}

func cmdGC(a *app.App, args []string, asJSON bool) int {
	apply := take(&args, "--apply")
	ask := take(&args, "--ask")
	merged := take(&args, "--merged")
	days := 14
	if value := takeValue(&args, "--days"); value != "" {
		if merged {
			return usageErr("mia gc --merged [--apply] (cannot be combined with --days)")
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return usageErr("mia gc [--merged] [--apply|--ask] [--days N] | mia gc --others [--apply|--ask] [path...]")
		}
		days = n
	}
	if take(&args, "--others") {
		return gcOthers(a, args, apply, ask)
	}
	if len(args) > 0 {
		return usageErr("mia gc [--merged] [--apply|--ask] [--days N] | mia gc --others [--apply|--ask] [path...]")
	}
	var (
		found []app.Abandoned
		err   error
	)
	if merged {
		found, err = a.Merged()
	} else {
		found, err = a.Abandoned(time.Duration(days)*24*time.Hour, time.Now())
	}
	if err != nil {
		return fail(err)
	}
	if asJSON && !apply {
		return emit(append([]app.Abandoned{}, found...))
	}
	if len(found) == 0 {
		if merged {
			fmt.Println("nothing merged and safe to remove — every merged worktree is starred, dirty, in a session, or already gone")
			return exitOK
		}
		fmt.Printf("nothing abandoned — every worktree is starred, dirty, in a session, or touched within %d days\n", days)
		if ask {
			pressKey()
		}
		return exitOK
	}
	for _, one := range found {
		fmt.Printf("%-12s %-28s %6s  last commit %s\n", one.Record.Name, one.Branch, one.Size, one.LastWork.Format("2006-01-02"))
	}
	if ask {
		apply = askYes(fmt.Sprintf("\nremove these %d? branches stay. y removes them ", len(found)))
		defer pressKey()
		if !apply {
			return exitOK
		}
	}
	if !apply {
		command := "mia gc"
		if merged {
			command += " --merged"
		}
		command += " --apply"
		if days != 14 {
			command += " --days " + strconv.Itoa(days)
		}
		fmt.Printf("\nnothing was removed — `%s` removes the above; star one to keep it\n", command)
		return exitOK
	}
	failed := false
	for _, one := range found {
		if err := a.Collect(one); err != nil {
			fmt.Fprintf(os.Stderr, "mia: %s: %v\n", one.Record.Name, err)
			failed = true
			continue
		}
		fmt.Printf("removed %s\n", one.Record.Name)
	}
	if failed {
		return exitFailed
	}
	return exitOK
}

// gcOthers removes the repository's worktrees mia did not make — an agent's,
// another tool's — when nothing in them is uncommitted. Their branches stay.
func gcOthers(a *app.App, paths []string, apply, ask bool) int {
	listings, err := a.List()
	if err != nil {
		return fail(err)
	}
	var found []app.Listing
	for _, l := range listings {
		if l.Main || l.Adopted || (len(paths) > 0 && !slices.Contains(paths, l.Path)) {
			continue
		}
		if l.Dirty {
			fmt.Printf("%-44s %-28s kept — uncommitted changes\n", l.Path, l.Branch)
			continue
		}
		found = append(found, l)
	}
	for _, l := range found {
		fmt.Printf("%-44s %-28s\n", l.Path, l.Branch)
	}
	if len(found) == 0 {
		fmt.Println("nothing to remove")
		if ask {
			pressKey()
		}
		return exitOK
	}
	if ask {
		apply = askYes(fmt.Sprintf("\nremove these %d? branches stay. y removes them ", len(found)))
		defer pressKey()
		if !apply {
			return exitOK
		}
	}
	if !apply {
		fmt.Println("\nnothing was removed — `mia gc --others --apply` removes the above; branches stay")
		return exitOK
	}
	code := exitOK
	for _, l := range found {
		if err := git.Remove(a.Root, l.Path, false); err != nil {
			fmt.Fprintf(os.Stderr, "mia: %s: %v\n", l.Path, err)
			code = exitFor(err)
			continue
		}
		fmt.Printf("removed %s\n", l.Path)
	}
	return code
}

func cmdAdopt(a *app.App, args []string, asJSON bool) int {
	if len(args) != 1 {
		return usageErr("mia adopt <worktree>")
	}
	record, err := a.Adopt(args[0])
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(record)
	}
	fmt.Printf("%s  %s\n", record.Name, record.Path)
	return exitOK
}

func cmdRemove(a *app.App, args []string) int {
	force := take(&args, "--force") || take(&args, "-f")
	stopRunning := take(&args, "--stop-running")
	dropBranch := take(&args, "--branch")
	if len(args) != 1 {
		return usageErr("mia rm [--force] [--stop-running] [--branch] <worktree>")
	}
	removal, err := a.PlanRemoval(args[0])
	if err != nil {
		return fail(err)
	}
	if err := removal.Blocked(force, stopRunning); err != nil {
		return fail(err)
	}

	branch := git.CurrentBranch(removal.Record.Path)
	if err := a.Remove(removal, force, stopRunning); err != nil {
		return fail(err)
	}
	fmt.Printf("removed %s\n", removal.Record.Name)
	if dropBranch && branch != "" {
		dropped, err := a.DropMergedBranch(branch)
		if err != nil {
			return fail(err)
		}
		if dropped {
			fmt.Printf("deleted branch %s, merged into %s\n", branch, a.BaseBranch(a.Root))
		} else {
			fmt.Printf("kept branch %s: it is not merged into %s\n", branch, a.BaseBranch(a.Root))
		}
	}
	return exitOK
}

func cmdShell(a *app.App, args []string) int {
	popup := take(&args, "--popup")
	view := takeValue(&args, "--view")
	target := ""
	if len(args) == 1 {
		target = args[0]
	} else if len(args) > 1 {
		return usageErr("mia shell [--popup | --view <name>] [worktree]")
	}
	if name, on, ok := strings.Cut(target, "@"); ok && name != "" {
		if os.Getenv("TMUX") != "" {
			return exitFor(handOver(a, name+"@"+on))
		}
		return cmdAttachOn(a, []string{name + "@" + on})
	}
	if target == "" {
		here, err := os.Getwd()
		if err != nil {
			return fail(err)
		}
		resolver, err := a.Resolver()
		if err != nil {
			return fail(err)
		}
		path, err := resolver.Here(here)
		if err != nil {
			return fail(err)
		}
		target = path
	}
	if view != "" {
		return exitFor(a.ShellView(target, view))
	}
	if err := a.Shell(target, popup); err != nil {
		return fail(err)
	}
	return exitOK
}

func cmdRun(a *app.App, args []string) int {
	if len(args) < 2 {
		return usageErr("mia run <worktree>[@machine] <command...>")
	}
	run := func() error { return a.Run(args[0], args[1:]) }
	there := false
	if name, on, ok := strings.Cut(args[0], "@"); ok && name != "" {
		machine, err := a.Runtimes.Get(on)
		if err != nil {
			return fail(err)
		}
		run, there = func() error { return a.RunOn(machine, name, args[1:]) }, true
	}
	if err := run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if !there { // the machine's mia has said it already
				fmt.Fprintln(os.Stderr, "mia:", err)
			}
			return exit.ExitCode()
		}
		return fail(err)
	}
	return exitOK
}

func managerFor(a *app.App) (env.Manager, error) {
	engine, err := container.Find()
	if err != nil {
		return env.Manager{}, err
	}
	return env.Manager{
		Engine:   engine,
		Runtimes: a.Runtimes,
		Repo:     a.Root,
		Settings: a.Config.Env,
		Services: a.Config.Services,
		Dotfiles: a.Config.Dotfiles,
	}, nil
}

func takeValue(args *[]string, flag string) string {
	for i, arg := range *args {
		if arg == flag && i+1 < len(*args) {
			value := (*args)[i+1]
			*args = append((*args)[:i:i], (*args)[i+2:]...)
			return value
		}
	}
	return ""
}

func take(args *[]string, flag string) bool {
	kept := (*args)[:0]
	found := false
	for _, arg := range *args {
		if arg == flag {
			found = true
			continue
		}
		kept = append(kept, arg)
	}
	*args = kept
	return found
}

func emit(value any) int {
	return asker{out: os.Stdout, errs: os.Stderr}.emit(value)
}

func exitFor(err error) int {
	if err == nil {
		return exitOK
	}
	var notFound resolve.NotFound
	if errors.As(err, &notFound) {
		return fail(err)
	}
	fmt.Fprintln(os.Stderr, "mia:", err)
	return run.ExitCode(err)
}

func padded(text string, width int) string {
	return text + strings.Repeat(" ", max(width-lipgloss.Width(text), 0))
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "mia: %v\n", err)
	var notFound resolve.NotFound
	if errors.As(err, &notFound) {
		return exitNotFound
	}
	return exitFailed
}

func usageErr(form string) int {
	fmt.Fprintf(os.Stderr, "usage: %s\n", form)
	return exitUsage
}

const usageColumn = 48

func usage(to *os.File) {
	fmt.Fprintln(to, "mia — worktrees, sessions, stacks and environments")
	width := 0
	for _, verb := range Verbs() {
		if len(verb.Usage) <= usageColumn {
			width = max(width, len(verb.Usage))
		}
	}
	by := Grouped()
	for _, group := range Groups {
		fmt.Fprintf(to, "\n%s\n", group)
		for _, verb := range by[group] {
			if len(verb.Usage) > width {
				fmt.Fprintf(to, "  %s\n  %-*s  %s\n", verb.Usage, width, "", verb.Summary)
				continue
			}
			fmt.Fprintf(to, "  %-*s  %s\n", width, verb.Usage, verb.Summary)
		}
	}
	fmt.Fprintln(to)
	fmt.Fprintln(to, "A worktree answers to its name, its path, or the branch checked out in it.")
	fmt.Fprintln(to, "Every read command takes --json.")
	fmt.Fprintln(to, "mia help vocabulary — what things are called.  mia help tmux — what mia adds to tmux: the radar, a new worktree, the status line.")
}

func usageOf(name string) string {
	for _, verb := range Verbs() {
		if verb.Name == name {
			return verb.Usage
		}
	}
	return "mia " + name
}

func verbUsage(to *os.File, name string) {
	for _, verb := range Verbs() {
		if verb.Name == name {
			fmt.Fprintf(to, "usage: %s\n\n%s\n", verb.Usage, verb.Summary)
			return
		}
	}
	usage(to)
}

func configPath(a *app.App) (string, error) {
	path := filepath.Join(a.MiaDir, "config.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(a.MiaDir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(starterConfig), 0o644); err != nil {
			return "", err
		}
	}
	return path, nil
}

func cmdConfig(a *app.App, args []string, asJSON bool) int {
	sub := ""
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	if sub != "" && sub != "show" && sub != "validate" {
		return cmdConfigKey(a, append([]string{sub}, args...))
	}
	if len(args) > 0 || (sub == "" && asJSON) {
		return usageErr("mia config [show|validate]")
	}
	switch sub {
	case "show":
		if asJSON {
			return emit(a.Config)
		}
		if err := toml.NewEncoder(os.Stdout).Encode(a.Config); err != nil {
			return fail(err)
		}
		return exitOK
	case "validate":
		if err := a.Config.Validate(); err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(struct {
				Valid bool `json:"valid"`
			}{Valid: true})
		}
		fmt.Println("configuration valid")
		return exitOK
	case "":
		path, err := configPath(a)
		if err != nil {
			return fail(err)
		}
		command := run.Local(a.Config.EditorCommand()).Command(path)
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			return fail(err)
		}
		return exitOK
	default:
		return usageErr("mia config [show|validate]")
	}
}

// cmdConfigKey prints one setting, or sets it: mia config [--global] <key>
// [<value>], the key a dotted path — base, popup.width, keys.quizzicle.quiz,
// plugin.agent.default. Setting edits that one line, keeps the file's
// comments, and refuses what mia would not load.
func cmdConfigKey(a *app.App, args []string) int {
	global := len(args) > 0 && args[0] == "--global"
	if global {
		args = args[1:]
	}
	if len(args) == 0 || len(args) > 2 {
		return usageErr("mia config [--global] <key> [<value>]")
	}
	path := config.GlobalPath()
	if !global {
		var err error
		if path, err = configPath(a); err != nil {
			return fail(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fail(err)
	}
	table, key := settingPath(args[0])
	if len(args) == 1 {
		if value, ok := lookup(string(data), table, key); ok {
			fmt.Println(value)
		} else if args[0] == "base" {
			fmt.Println(git.CurrentBranch(a.Root) + "  (unset: the branch the main checkout is on)")
		} else {
			fmt.Println("(unset)")
		}
		return exitOK
	}
	value := args[1]
	if args[0] == "base" && !git.BranchExists(a.Root, value) && git.RevParse(a.Root, value) == "" {
		return fail(fmt.Errorf("no branch %s here", value))
	}
	if err := writeConfig(path, setting(string(data), table, key, literal(value))); err != nil {
		return fail(fmt.Errorf("%s = %s: %w", args[0], value, err))
	}
	fmt.Printf("%s = %s\n", args[0], literal(value))
	return exitOK
}

// writeConfig writes a config file edited, refusing what mia would not
// load.
func writeConfig(path, edited string) error {
	if err := config.Check(edited); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(edited), 0o644)
}

// settingPath is a dotted key's table and key: base is top-level, keys.a.b
// is "a.b" in [keys], plugin.agent.default is default in [plugin.agent],
// and agents.claude.account is account in [agents.claude].
func settingPath(dotted string) (table, key string) {
	parts := strings.Split(dotted, ".")
	switch {
	case len(parts) == 1:
		return "", dotted
	case (parts[0] == "plugin" || parts[0] == "agents") && len(parts) > 2:
		return parts[0] + "." + parts[1], strings.Join(parts[2:], ".")
	}
	return parts[0], strings.Join(parts[1:], ".")
}

var bareKey = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func tomlKey(key string) string {
	if bareKey.MatchString(key) {
		return key
	}
	return strconv.Quote(key)
}

var rawValue = regexp.MustCompile(`^(\[.*\]|true|false|-?[0-9]+(\.[0-9]+)?)$`)

// literal is a value as TOML: a list, a bool or a number as written, else
// a string.
func literal(value string) string {
	if rawValue.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}

// keyOf is the key a "key = value" line sets, unquoted, or "".
func keyOf(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "[") {
		return ""
	}
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	k = strings.TrimSpace(k)
	if uq, err := strconv.Unquote(k); err == nil {
		return uq
	}
	return k
}

// region is where table's keys sit in lines: from its first key line to the
// line before the next table; ok is false when there is no such table.
func region(lines []string, table string) (from, to int, ok bool) {
	from, ok = 0, table == ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !strings.HasPrefix(t, "[") {
			continue
		}
		if ok {
			return from, i, true
		}
		if t == "["+table+"]" {
			from, ok = i+1, true
		}
	}
	return from, len(lines), ok
}

func lookup(config, table, key string) (string, bool) {
	lines := strings.Split(config, "\n")
	from, to, ok := region(lines, table)
	for i := from; ok && i < to; i++ {
		if keyOf(lines[i]) == key {
			_, v, _ := strings.Cut(lines[i], "=")
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// setting is config with key set to the TOML value in table: its line
// replaced, or added to the table — made at the end when it is not there —
// with every other line, comments too, as it was.
func setting(config, table, key, value string) string {
	line := tomlKey(key) + " = " + value
	lines := strings.Split(strings.TrimRight(config, "\n"), "\n")
	if config == "" {
		lines = nil
	}
	from, to, ok := region(lines, table)
	if !ok {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		return strings.Join(append(lines, "["+table+"]", line), "\n") + "\n"
	}
	for i := from; i < to; i++ {
		if keyOf(lines[i]) == key {
			lines[i] = line
			return strings.Join(lines, "\n") + "\n"
		}
	}
	at := to
	if table == "" && to < len(lines) {
		// before the first table, with a blank line between
		lines = append(lines[:at], append([]string{line, ""}, lines[at:]...)...)
		return strings.Join(lines, "\n") + "\n"
	}
	for at > from && strings.TrimSpace(lines[at-1]) == "" {
		at--
	}
	lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

// unsetting is config without key in table: its line taken out, every
// other line as it was.
func unsetting(config, table, key string) string {
	lines := strings.Split(config, "\n")
	from, to, ok := region(lines, table)
	for i := from; ok && i < to; i++ {
		if keyOf(lines[i]) == key {
			return strings.Join(slices.Delete(lines, i, i+1), "\n")
		}
	}
	return config
}

const starterConfig = `# mia — this repository's local configuration. Every key is optional, and an
# unknown key is refused rather than ignored. ` + "`mia help vocabulary`" + ` explains the terms.
# Use .mia.toml in the main checkout for committed settings shared by every worktree.
#
# base = "main"                    # what a new branch starts from
# prefix = "you/"                  # put in front of every branch name
# setup = ["npm", "install"]       # run once in a new worktree
# editor = "nvim"
# keep = ["tail -f"]               # programs a pane keeps after they end, with agents, editors and dev servers
# [popup]                          # the tmux popup f opens over the list
# width = "100%"
# height = "100%"
#
# [launch]                         # named windows: n in the windows panel, or ` + "`mia window new <name>`" + `
# test = "go test ./..."
# logs = "tail -f log/dev.log"
#
# [env]                            # the container a worktree's code runs in
# image = "node:22"                # else devcontainer.json, else picked for the language
# image_setup = ["apt", "install", "-y", "qpdf"]   # baked in, survives ` + "`mia env rm`" + `
# setup = ["npm", "install"]       # inside a fresh container; the top-level setup is the host's
# ports = [5173]
#
# [[service]]                      # what an environment keeps running
# id = "web"
# run = ["npm", "run", "dev"]
# autostart = true
`

func printTmuxHelp() int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	fmt.Print("# mia in tmux — `mia tmux install` adds this to ~/.tmux.conf:\n# prefix space the radar, prefix n a new worktree, prefix m the menu, prefix a the next that needs you, mia's part of the status line.\n\n")
	fmt.Print(tmuxSnippet(self))
	return exitOK
}

func subject(a *app.App, args []string, fallback string, subs ...string) (sub string, record model.Record, rest []string, err error) {
	sub, rest = fallback, args
	target := ""
	if len(rest) > 1 && !slices.Contains(subs, rest[0]) && slices.Contains(subs, rest[1]) && namesAWorktree(a, rest[0]) {
		target, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && slices.Contains(subs, rest[0]) {
		sub, rest = rest[0], rest[1:]
	}
	if target == "" && len(rest) > 0 && namesAWorktree(a, rest[0]) {
		target, rest = rest[0], rest[1:]
	}
	record, err = a.RecordOrHere(target)
	return sub, record, rest, err
}

// onMachine finds <worktree>@<machine> where a window verb names its
// worktree, before any --, and gives the arguments back with the worktree
// alone: what the machine's own mia is asked.
func onMachine(args []string) (machine string, rest []string, ok bool) {
	for i, arg := range args {
		if arg == "--" || i > 1 {
			break
		}
		if name, on, cut := strings.Cut(arg, "@"); cut && name != "" && on != "" {
			rest = slices.Clone(args)
			rest[i] = name
			return on, rest, true
		}
	}
	return "", args, false
}

func cmdWindow(a *app.App, args []string, asJSON bool) int {
	if on, rest, ok := onMachine(args); ok {
		machine, err := a.Runtimes.Get(on)
		if err != nil {
			return fail(err)
		}
		if machine.Name != runtime.Local {
			if asJSON {
				rest = append([]string{"--json"}, rest...)
			}
			if err := a.WindowOn(machine, rest); err != nil {
				var exit *exec.ExitError
				if errors.As(err, &exit) {
					return exit.ExitCode() // the machine's mia has said why
				}
				return fail(err)
			}
			return exitOK
		}
		args = rest
	}
	// what comes after -- is the window's command, or what is typed: no flag of mia's
	var argv []string
	if i := slices.Index(args, "--"); i >= 0 {
		argv, args = args[i+1:], args[:i]
	}
	model, effort, account := takeValue(&args, "--model"), takeValue(&args, "--effort"), takeValue(&args, "--account")
	role, kind := takeValue(&args, "--role"), takeValue(&args, "--kind")
	task := takeValue(&args, "--task") // of new: what the agent started is told, once it is ready
	sub, record, args, err := subject(a, args, "ls", "ls", "open", "select", "new", "close", "read", "send", "press", "key", "model", "account")
	if err != nil {
		return fail(err)
	}
	editor := take(&args, "--editor")
	if (model != "" || effort != "") && sub != "new" && sub != "model" || (account != "" || role != "" || kind != "") && sub != "new" || role != "" && (len(argv) > 0 || editor) {
		return usageErr(windowUsage)
	}
	name := strings.Join(args, " ")
	switch sub {
	case "ls":
		windows := a.Windows(record)
		if asJSON {
			return emit(windows)
		}
		if len(windows) == 0 {
			fmt.Printf("%s has no session\n", record.Name)
			return exitOK
		}
		for _, w := range windows {
			fmt.Printf("%-16s %-12s quiet %s\n", w.Name, w.Command, w.Quiet.Round(time.Second))
		}
		return exitOK
	case "open":
		if name == "" {
			return usageErr("mia window open [worktree] <window>")
		}
		return exitFor(a.OpenWindow(record, name))
	case "select":
		if name == "" {
			return usageErr("mia window select [worktree] <window>")
		}
		return exitFor(a.SelectWindow(record, name))
	case "new":
		// an agent is started with a model and an effort: those given, or the configured ones
		var chosen app.Chosen
		var agent, body, line string // line: the command the window runs, not the shell that starts it
		if role != "" {
			// a role is a setup with a body: what is given here is said over it
			read, err := app.ReadRole(role)
			if err != nil {
				return fail(err)
			}
			setup, err := a.RoleSetup(read, name)
			if err != nil {
				return fail(err)
			}
			name = cmp.Or(name, setup.Agent)
			model, effort, account, body = cmp.Or(model, setup.Model), cmp.Or(effort, setup.Effort), cmp.Or(account, setup.Account), setup.Body
			agent = setup.Agent
		}
		command, launched := a.Config.Launch[name]
		// (A window named for an agent runs it, whether or not a model is said: a bare shell
		// called pi is what nobody asked for.)
		if !launched && len(argv) == 0 && !editor && (model != "" || effort != "" || account != "" || (name != "" && a.AgentOf(name) == name)) {
			command, launched = a.AgentCommand(name), true // the window is named for the agent it runs
		}
		if role != "" && a.AgentOf(command) != agent {
			command, launched = a.AgentCommand(agent), true
		}
		if launched && len(argv) == 0 && !editor {
			var err error
			agent = a.AgentOf(command)
			if command, chosen, err = a.CommandWith(command, model, effort, account); err != nil {
				return fail(err)
			}
			if argv, err = a.LaunchArgv(record, command); err != nil {
				return fail(err)
			}
			line = command
		} else if len(argv) > 0 {
			var err error
			agent = a.AgentOf(strings.Join(argv, " "))
			if argv, chosen, err = a.ArgvWith(argv, model, effort, account); err != nil {
				return fail(err)
			}
		} else if model != "" || effort != "" || account != "" {
			return usageErr(windowUsage)
		}
		if editor {
			command := a.Config.EditorCommand()
			if command == "" {
				return fail(fmt.Errorf("no editor: set `editor` in the config or $EDITOR"))
			}
			argv = append(strings.Fields(command), ".")
		}
		// what the window is for: as told, its agent, its editor, or what its command is for
		told := session.Kind("")
		if kind != "" {
			var err error
			if told, err = session.ParseKind(kind); err != nil {
				return fail(err)
			}
		}
		switch {
		case told != "":
		case agent != "":
			told = session.KindAgent
		case editor:
			told = session.KindEditor
		}
		if line == "" {
			line = strings.Join(argv, " ")
		}
		name, err := a.NewWindow(record, name, a.WhatAs(told, line), argv)
		if err != nil {
			return fail(err)
		}
		if chosen != (app.Chosen{}) {
			if err := a.KeepChosen(record, name, agent, chosen); err != nil {
				fmt.Fprintf(os.Stderr, "mia: %s: window %s opened, and what it was started with is not kept: %v\n", record.Name, name, err)
			}
		}
		if task != "" {
			if err := a.SayOnceReady(record, name, task); err != nil {
				return fail(fmt.Errorf("%s: window %s opened, and it was not told its task: %w", record.Name, name, err))
			}
		}
		fmt.Printf("%s: window %s opened\n", record.Name, name)
		if body != "" {
			err := a.SayOnceReady(record, name, body)
			_ = a.Remember()
			if err != nil {
				return fail(fmt.Errorf("%s: role %s's opening was not sent to window %s: %w", record.Name, role, name, err))
			}
			fmt.Printf("%s: window %s was sent role %s's opening\n", record.Name, name, role)
			return exitOK
		}
		if len(argv) > 0 {
			time.Sleep(1500 * time.Millisecond)
			if text, err := a.ReadWindow(record, name); err != nil {
				// a window is its command, and goes when that ends
				fmt.Printf("%s: window %s has ended already — its command finished, and its output went with the window; `mia run %s <command>` shows a command's output\n", record.Name, name, record.Name)
			} else if text != "" {
				fmt.Println(text)
			}
			_ = a.Remember() // again: a command started through a shell is in front only now
		}
		return exitOK
	case "read":
		if name == "" {
			return usageErr("mia window read [worktree] <window>")
		}
		text, err := a.ReadWindow(record, name)
		if err != nil {
			return fail(err)
		}
		fmt.Println(text)
		return exitOK
	case "send":
		if name == "" || len(argv) == 0 {
			return usageErr("mia window send [worktree] <window> -- <text>")
		}
		return exitFor(a.SendToWindow(record, name, strings.Join(argv, " ")))
	case "press":
		if name == "" || len(argv) == 0 {
			return usageErr("mia window press [worktree] <window> -- <keys>")
		}
		return exitFor(a.PressInWindow(record, name, strings.Join(argv, " ")))
	case "key":
		if name == "" || len(argv) == 0 {
			return usageErr("mia window key [worktree] <window> -- <key>…   (tmux's names: C-c, Escape, Enter, Up)")
		}
		return exitFor(a.KeysInWindow(record, name, argv))
	case "model":
		window, chosen, ok := strings.Cut(name, " ")
		if !ok || chosen == "" || strings.Contains(chosen, " ") || len(argv) > 0 || model != "" {
			return usageErr("mia window model [worktree] <window> <model> [--effort <effort>]")
		}
		now, err := a.SwitchWindow(record, window, chosen, effort)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s: window %s switched to %s\n", record.Name, window, said(now))
		return exitOK
	case "account":
		window, chosen, ok := strings.Cut(name, " ")
		if !ok || chosen == "" || strings.Contains(chosen, " ") || len(argv) > 0 {
			return usageErr("mia window account [worktree] <window> <account>")
		}
		return exitFor(a.AccountOfWindow(record, window, chosen))
	case "close":
		if name == "" {
			return usageErr("mia window close [worktree] <window>")
		}
		if err := a.CloseWindow(record, name); err != nil {
			return fail(err)
		}
		fmt.Printf("%s: window %s closed\n", record.Name, name)
		return exitOK
	}
	return usageErr(windowUsage)
}

const windowUsage = "mia window [ls|open|select|new|close|read|send|press|key|model|account] [worktree[@machine]] [window] [--editor] [--model <model>] [--effort <effort>] [--account <account>] [--role <role>] [--kind <kind>] [--task <text>] [-- command|text|keys]"

// said is a model and an effort as a line says them: sonnet, effort high.
func said(chosen app.Chosen) string {
	if chosen.Effort == "" {
		return chosen.Model
	}
	return chosen.Model + ", effort " + chosen.Effort
}

func configHere() config.Config {
	here, _ := os.Getwd()
	return configAt(here, os.Stderr)
}

// configAt is the configuration of the repository a folder is in, or the
// person's own outside one.
func configAt(dir string, errs io.Writer) config.Config {
	if dir != "" {
		if a, err := app.Open(dir); err == nil {
			return a.Config
		}
	}
	c, err := config.Load("")
	if err != nil {
		fmt.Fprintf(errs, "mia: %v\n", err)
	}
	return c
}
