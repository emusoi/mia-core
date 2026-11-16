package cli

import "sort"

type Verb struct {
	Name    string
	Usage   string
	Summary string
	Reads   bool
	Group   string
}

var Groups = []string{"Worktrees", "Working", "Environments", "Records"}

var groupOf = map[string]string{
	"new": "Worktrees", "ls": "Worktrees", "path": "Worktrees", "adopt": "Worktrees", "rm": "Worktrees", "gc": "Worktrees",
	"star": "Worktrees", "open": "Worktrees", "shell": "Worktrees", "window": "Worktrees", "rename": "Worktrees", "run": "Worktrees", "setup": "Worktrees", "switch": "Worktrees", "shell-init": "Worktrees",
	"dash": "Working", "stack": "Working", "up": "Working", "down": "Working",
	"config": "Working",
	"env":    "Environments", "dev": "Environments", "machine": "Environments", "gateway": "Environments",
	"api": "Records", "signal": "Records", "radar": "Working", "tmux": "Working", "move": "Worktrees", "resurrect": "Worktrees", "why": "Records", "recall": "Records", "conversation": "Records", "archive": "Worktrees", "fork": "Worktrees", "unarchive": "Worktrees", "plugin": "Records", "doctor": "Records", "main": "Working",
}

var verbs = []Verb{
	{"new", "mia new [--on <machine>] [--stack [--in <worktree>] [--worktree]] [--from <ref>] [--name <name>] [--bg] [--shell] <branch> | --pr <number> [--remote <remote>]", "create a worktree and name it, or one at a pull request's head", false, ""},
	{"ls", "mia ls", "what exists", true, ""},
	{"setup", "mia setup [worktree]", "run the configured setup in a worktree again", false, ""},
	{"path", "mia path [worktree|stack]", "print a worktree's path, for scripts; no argument is the main checkout", true, ""},
	{"switch", "mia switch <worktree|stack>", "cd there — with `eval \"$(mia shell-init zsh)\"` in your rc", true, ""},
	{"shell-init", "mia shell-init <zsh|bash|fish>", "the shell function that makes `mia switch` change directory, and completion", true, ""},
	{"gc", "mia gc [--merged] [--apply] [--days N] | --others [--apply] [path...]", "worktrees nobody has touched: clean, no session, not starred, quiet for N days; --merged: those whose branch is merged; --others: clean ones mia did not make", false, ""},
	{"adopt", "mia adopt <worktree>", "take over a worktree created by hand", false, ""},
	{"rm", "mia rm [--force] [--stop-running] [--branch] <worktree>", "remove a worktree, its session and its name", false, ""},
	{"window", "mia window [ls|open|select|new|close|read|send|press|key] [worktree[@machine]] [window] [--editor] [-- command|text|keys]", "the windows of a worktree's session: land in one, pick one without landing, add a shell, close one, read or type into one, type into one without Enter, press a key by name (C-c, Escape)", false, ""},
	{"shell", "mia shell [--popup | --view <name>] [worktree]", "attach to a worktree's session; --view at this terminal's own size and window", false, ""},
	{"run", "mia run <worktree>[@machine] <cmd…>", "run a command once in a worktree, here or on a machine", false, ""},
	{"dev", "mia dev [<worktree> [--follow]|off]", "lend the main checkout's running dev server a worktree's files; the one time mia writes into a working tree", false, ""},
	{"env", "mia env <up [--wait] [--recreate]|down [--all]|setup|dotfiles|sync [auto|manual|now|status|conflicts|preview|restart]|shell|exec|run|ports|image|tools|host|browse [--direct]|service|status|rm> [worktree]", "the environment a worktree's code runs in", false, ""},
	{"stack", "mia stack [--in <worktree>] [show|go <layer>|name <name>|track <branch> <parent>|sync|merge <layer> [onto <layer>]|rm [--worktrees] [--branches] [--force]|restack|diff]", "a feature too large for one pull request", false, ""},
	{"up", "mia up", "move to the layer above, in place", false, ""},
	{"down", "mia down", "move to the layer below, in place", false, ""},
	{"machine", "mia machine <list|add <name> <ssh-target>|shell <name>|ship <name>|link <name>|attach <worktree>@<name>|rm <name>>", "the machines worktrees and environments run on", false, ""},
	{"gateway", "mia gateway <status|start|stop|install|uninstall|setup|trust|untrust>", "what makes <name>.mia reach an environment", false, ""},
	{"move", "mia move [--dry-run] <worktree>[@<machine>] <machine|local>", "take a worktree to another machine or back: its branch, uncommitted work and windows; --dry-run only says what would cross", false, ""},
	{"resurrect", "mia resurrect [--dry-run] [--list] [--ask] [<worktree>...]", "open sessions again — each that went down with tmux, or any one named, closed on purpose or not: its windows, their programs, a claude resuming the very conversation it had; --list shows every session mia remembers", false, ""},
	{"fork", "mia fork [worktree] <branch>", "a new worktree on a new branch from where a worktree is, its uncommitted and untracked work copied: a second try beside the first", false, ""},
	{"archive", "mia archive [--stop-running] <worktree> · ls", "put a worktree away: everything in it, uncommitted and untracked work too, kept under refs/mia/archive; its branch stays", false, ""},
	{"unarchive", "mia unarchive <name>", "bring an archived worktree back as it was, under its old name when free", false, ""},
	{"recall", "mia recall <text…>", "the agent conversations here that mention it — claude's, codex's, pi's — newest first, and how to resume each", true, ""},
	{"conversation", "mia conversation resume <id> [--in <worktree>]", "open a window with the agent taking a conversation up again, in the worktree it was made in or another; `mia api conversations` lists them", false, ""},
	{"main", "mia main [status|start|stop|say <text…>|interrupt]", "the main agent: the one you talk to, which runs the others — one for you, in a session of its own, no repository's; say types a message into it, interrupt stops it mid-work", false, ""},
	{"why", "mia why <file:line|commit>", "the claude conversation that made a commit, or the line's last commit: what you asked, and how to resume it", true, ""},
	{"radar", "mia radar [--new]", "the popup that shows and finds everything; --new makes a worktree", false, ""},
	{"tmux", "mia tmux [install|uninstall|menu <client>]", "mia's part of the tmux status line, its two popups and its menu, printed, added to ~/.tmux.conf, or taken out; menu draws prefix m", true, ""},
	{"signal", "mia signal <worktree> <kind> [text] [--style s] [--rank n] [--from source] [--for d|--until seen] · clear · ls", "what anything outside mia says about a worktree, shown as its setter asks", false, ""},
	{"api", "mia api <query>", "structured state, for clients and scripts", true, ""},
	{"doctor", "mia doctor", "check tools, configuration, gateway, machines and environments", true, ""},
	{"plugin", "mia plugin [ls|enable <name>|disable <name>]", "programs named mia-<name> that add verbs; installed on PATH, run once enabled", true, ""},
	{"dash", "mia dash", "the radar, under its old name", false, ""},
	{"rename", "mia rename <worktree> <name>", "give a worktree the name you want; its session follows now, its container on its next start", false, ""},
	{"open", "mia open [worktree] [--in <cursor|code|zed|idea|subl|xcode|finder|terminal>]", "open the worktree in your editor, IDE, Finder or a terminal", false, ""},
	{"star", "mia star [worktree]", "mark the worktree you are on right now, or unmark it", false, ""},
	{"config", "mia config [show|validate|[--global] <key> [<value>]]", "the repository's configuration, in your editor; show or validate the effective one; with a key, say or set one setting — base, keys.<action>", false, ""},
}

func Verbs() []Verb {
	out := append([]Verb(nil), verbs...)
	for i := range out {
		out[i].Group = groupOf[out[i].Name]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Grouped() map[string][]Verb {
	by := map[string][]Verb{}
	for _, verb := range Verbs() {
		by[verb.Group] = append(by[verb.Group], verb)
	}
	return by
}
