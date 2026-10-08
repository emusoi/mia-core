package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/container"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/resolve"

	"github.com/emusoi/mia-core/internal/run"
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
	case "gateway":
		return cmdGateway(rest)
	case "machine", "machines", "runtime":
		return cmdRuntime(rest, asJSON)
	case "plugin", "plugins":
		return cmdPlugin(configHere(), rest, asJSON)
	}

	here, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	a, err := app.Open(here)
	if err != nil {
		return fail(err)
	}
	a.Events = func(e app.Event) { sendEvent(a, e) }

	switch verb {
	case "new":
		return cmdNew(a, rest, asJSON)
	case "config":
		return cmdConfig(a)
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
	case "ls", "list":
		return cmdList(a, asJSON)
	case "path", "switch":
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
	case "api":
		return cmdAPI(a, rest)
	case "dash":
		return cmdDash(a, rest)
	case "stack":
		return cmdStack(a, rest, asJSON)
	case "up":
		return cmdUp(a)
	case "down":
		return cmdDown(a)
	case "pr":
		return cmdPR(a, rest, asJSON)
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
	case "run", "env":
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
	if pr := takeValue(&args, "--pr"); pr != "" {
		number, err := strconv.Atoi(strings.TrimPrefix(pr, "#"))
		if err != nil || number <= 0 || len(args) > 0 {
			return usageErr("mia new --pr <number> [--shell]")
		}
		record, err := a.NewFromPR(number)
		if record.Path == "" {
			return fail(err)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "mia: %v\n", err)
		}
		fmt.Printf("%s — pull request #%d in a worktree of its own\n%s\n", record.Name, number, record.Path)
		if andShell {
			return exitFor(a.Shell(record.Name, false))
		}
		return exitOK
	}
	on := takeValue(&args, "--on")
	if len(args) != 1 {
		return usageErr("mia new [--pr <number>] [--on <machine>] [--stack [--in <worktree>] [--worktree]] [--shell] <branch>")
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
	before, _ := a.Store.Load()
	record, err := a.New(args[0])
	if err != nil {
		if record.Path == "" {
			return fail(err)
		}
		fmt.Fprintf(os.Stderr, "mia: %v\n", err)
	}
	if asJSON {
		return emit(record)
	}
	where := "in a worktree of its own"
	if slices.ContainsFunc(before, func(one model.Record) bool { return one.Path == record.Path }) {
		where = "is already checked out there"
	}
	fmt.Printf("%s — %s %s\n%s\n", record.Name, git.CurrentBranch(record.Path), where, record.Path)
	if on != "" && on != "-" && on != "local" {
		placed, code := placeOn(a, record, on)
		if code != exitOK {
			return code
		}
		record = placed
	}
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
	width := 0
	for _, l := range listings {
		width = max(width, len(l.Name))
	}
	for _, l := range listings {
		marks := []string{}
		if l.Dirty {
			marks = append(marks, "dirty")
		}
		if l.Session {
			marks = append(marks, "session")
		}
		if l.Ahead > 0 || l.Behind > 0 {
			marks = append(marks, fmt.Sprintf("+%d -%d", l.Ahead, l.Behind))
		}
		if !l.Adopted && !l.Main {
			marks = append(marks, "not adopted")
		}
		if l.Stack != "" {
			marks = append(marks, "in "+l.Stack)
		}
		branch := l.Branch
		if branch == "" {
			branch = "(detached)"
		}
		star := "  "
		if l.Starred {
			star = "★ "
		}
		fmt.Printf("%s%-*s  %s %s\n", star, width, l.Name, padded(branch, 28), strings.Join(marks, " · "))
	}
	return exitOK
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
	days := 14
	if value := takeValue(&args, "--days"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return usageErr("mia gc [--apply] [--days N]")
		}
		days = n
	}
	if len(args) > 0 {
		return usageErr("mia gc [--apply] [--days N]")
	}
	found, err := a.Abandoned(time.Duration(days)*24*time.Hour, time.Now())
	if err != nil {
		return fail(err)
	}
	if asJSON && !apply {
		return emit(append([]app.Abandoned{}, found...))
	}
	if len(found) == 0 {
		fmt.Printf("nothing abandoned — every worktree is starred, dirty, in a session, or touched within %d days\n", days)
		return exitOK
	}
	for _, one := range found {
		fmt.Printf("%-12s %-28s %6s  last commit %s\n", one.Record.Name, one.Branch, one.Size, one.LastWork.Format("2006-01-02"))
	}
	if !apply {
		command := "mia gc --apply"
		if days != 14 {
			command += " --days " + strconv.Itoa(days)
		}
		fmt.Printf("\nnothing was removed — `%s` removes the above; star one to keep it\n", command)
		return exitOK
	}
	for _, one := range found {
		if err := a.Collect(one); err != nil {
			fmt.Fprintf(os.Stderr, "mia: %s: %v\n", one.Record.Name, err)
			continue
		}
		fmt.Printf("removed %s\n", one.Record.Name)
	}
	return exitOK
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
	if len(args) != 1 {
		return usageErr("mia rm [--force] [--stop-running] <worktree>")
	}
	removal, err := a.PlanRemoval(args[0])
	if err != nil {
		return fail(err)
	}
	if err := removal.Blocked(force, stopRunning); err != nil {
		return fail(err)
	}

	if manager, managerErr := managerFor(a); managerErr == nil {
		if err := manager.Remove(removal.Record); err != nil {
			fmt.Fprintf(os.Stderr, "mia: %v\n", err)
		}
	}

	if err := a.Remove(removal, force, stopRunning); err != nil {
		return fail(err)
	}
	fmt.Printf("removed %s\n", removal.Record.Name)
	return exitOK
}

func cmdShell(a *app.App, args []string) int {
	popup := take(&args, "--popup")
	target := ""
	if len(args) == 1 {
		target = args[0]
	} else if len(args) > 1 {
		return usageErr("mia shell [--popup] [worktree]")
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
	if err := a.Shell(target, popup); err != nil {
		return fail(err)
	}
	return exitOK
}

func cmdRun(a *app.App, args []string) int {
	if len(args) < 2 {
		return usageErr("mia run <worktree> <command...>")
	}
	if err := a.Run(args[0], args[1:]); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, "mia:", err)
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
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fail(err)
	}
	fmt.Println(string(data))
	return exitOK
}

func exitFor(err error) int {
	if err != nil {
		return fail(err)
	}
	return exitOK
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
	fmt.Fprintln(to, "mia help vocabulary — what things are called.  mia help tmux — the key that pops the dashboard over tmux.")
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

func cmdConfig(a *app.App) int {
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
}

const starterConfig = `# mia — this repository's configuration. Every key is optional, and an
# unknown key is refused rather than ignored. ` + "`mia help vocabulary`" + ` explains the terms.
#
# base = "main"                    # what a new branch starts from
# prefix = "you/"                  # put in front of every branch name
# setup = ["npm", "install"]       # run once in a new worktree
# editor = "nvim"
# [popup]                          # the tmux popup ⇥ opens over the list
# width = "80%"
# height = "75%"
#
# [launch]                         # named windows: n in the windows panel, or ` + "`mia window new <name>`" + `
# test = "go test ./..."
# logs = "tail -f log/dev.log"
#
# [keys]                           # rebind anything ` + "`?`" + ` lists; in ~/.config/mia/config.toml it applies everywhere
# machines = "B"                   # actions: open shell env star stack blocks land pr
# merge = "m"                      #   begin layer apart claim worktree merge name restack delete new config
# down = ["j", "down"]             # moves: down up first last pagedown pageup filter help popup refresh
# quiet = "Q"                      #   tabnext tabprev unfold fold back quit · sections: quiet
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
	fmt.Print(`# In ~/.tmux.conf — prefix g opens the dashboard over the current window,
# and it is gone again when you leave it:

bind-key g display-popup -B -E -e MIA_POPUP=1 -e "MIA_TMUX_PANE=#{pane_id}" -d "#{pane_current_path}" -w 80% -h 70% "mia dash"

# prefix j picks another worktree and switches to its session in place:
bind-key j display-popup -B -E -e MIA_POPUP=1 -e "MIA_TMUX_PANE=#{pane_id}" -d "#{pane_current_path}" -w 60% -h 50% "mia dash --switch"

# prefix m: a menu where each entry opens one thing in its own popup,
# on the worktree of the pane you are in:
bind-key m display-menu -T " mia " \
  "dashboard"     g "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 80% -h 70% 'mia dash'" \
  "switch"        j "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 60% -h 50% 'mia dash --switch'" \
  "" \
  "new"           n "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 70% -h 40% 'mia dash --do new'" \
  "branch"        b "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 70% -h 40% 'mia dash --do branch'" \
  "pull request"  p "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 60% -h 20% 'mia dash --do pr'"

# Then: tmux source-file ~/.tmux.conf
`)
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

func cmdWindow(a *app.App, args []string, asJSON bool) int {
	sub, record, args, err := subject(a, args, "ls", "ls", "open", "select", "new", "close")
	if err != nil {
		return fail(err)
	}
	editor := take(&args, "--editor")
	var argv []string
	if i := slices.Index(args, "--"); i >= 0 {
		argv, args = args[i+1:], args[:i]
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
		if command, ok := a.Config.Launch[name]; ok && len(argv) == 0 && !editor {
			shell := a.Config.Dotfiles.Shell
			if shell == "" {
				host, err := a.SessionHostOf(record)
				if err != nil {
					return fail(err)
				}
				if host.SSH == "" {
					shell = os.Getenv("SHELL")
				}
				if shell == "" {
					shell = "/bin/sh"
				}
			}
			argv = []string{shell, "-lc", command + "; exec " + run.ShellJoin([]string{shell}) + " -l"}
		}
		if editor {
			command := a.Config.EditorCommand()
			if command == "" {
				return fail(fmt.Errorf("no editor: set `editor` in the config or $EDITOR"))
			}
			argv = append(strings.Fields(command), ".")
			if name == "" {
				name = "editor"
			}
		}
		if name == "" {
			name = "shell"
		}
		name, err := a.NewWindow(record, name, argv)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s: window %s opened\n", record.Name, name)
		if len(argv) > 0 {
			time.Sleep(1500 * time.Millisecond)
			if text := a.WindowText(record, name); text != "" {
				fmt.Println(text)
			}
		}
		return exitOK
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
	return usageErr("mia window [ls|open|select|new|close] [worktree] [window] [--editor] [-- command]")
}

func configHere() config.Config {
	if here, err := os.Getwd(); err == nil {
		if a, err := app.Open(here); err == nil {
			return a.Config
		}
	}
	c, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mia: %v\n", err)
	}
	return c
}
