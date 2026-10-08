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
	"config": "Working", "pr": "Working",
	"env": "Environments", "machine": "Environments", "gateway": "Environments",
	"api": "Records", "plugin": "Records",
}

var verbs = []Verb{
	{"new", "mia new [--pr <number>] [--on <machine>] [--stack [--in <worktree>] [--worktree]] [--shell] <branch>", "create a worktree and name it", false, ""},
	{"ls", "mia ls", "what exists", true, ""},
	{"setup", "mia setup [worktree]", "run the configured setup in a worktree again", false, ""},
	{"path", "mia path [worktree|stack]", "print a worktree's path, for scripts; no argument is the main checkout", true, ""},
	{"switch", "mia switch <worktree|stack>", "cd there — with `eval \"$(mia shell-init zsh)\"` in your rc", true, ""},
	{"shell-init", "mia shell-init <zsh|bash|fish>", "the shell function that makes `mia switch` change directory, and completion", true, ""},
	{"gc", "mia gc [--apply] [--days N]", "worktrees nobody has touched: clean, no session, not starred, quiet for N days", false, ""},
	{"adopt", "mia adopt <worktree>", "take over a worktree created by hand", false, ""},
	{"rm", "mia rm [--force] [--stop-running] <worktree>", "remove a worktree, its session and its name", false, ""},
	{"window", "mia window [ls|open|select|new|close] [worktree] [window] [--editor] [-- command]", "the windows of a worktree's session: land in one, pick one without landing, add a shell, close one", false, ""},
	{"shell", "mia shell [--popup] [worktree]", "attach to a worktree's session", false, ""},
	{"run", "mia run <worktree> <cmd…>", "run a command once in a worktree", false, ""},
	{"env", "mia env <up|down|setup|dotfiles|sync|shell|exec|run|ports|image|tools|host|browse|service|status|rm> [worktree]", "the environment a worktree's code runs in", false, ""},
	{"stack", "mia stack [--in <worktree>] [show|go <layer>|name <name>|merge <layer> [onto <layer>]|rm [--worktrees] [--branches] [--force]|restack|diff|pr]", "a feature too large for one pull request", false, ""},
	{"up", "mia up", "move to the layer above, in place", false, ""},
	{"down", "mia down", "move to the layer below, in place", false, ""},
	{"pr", "mia pr [worktree] [show|draft [--branch <branch>] [--title <title>] [--body-stdin] [--remove-after-merge <true|false>]]", "draft a pull request and print the commands you run yourself", false, ""},
	{"machine", "mia machine <list|add <name> <ssh-target>|shell <name>|rm <name>>", "the machines an environment can run on", false, ""},
	{"gateway", "mia gateway <status|start|stop|install|uninstall|setup|trust|untrust>", "what makes <name>.mia reach an environment", false, ""},
	{"api", "mia api <query>", "structured state, for clients and scripts", true, ""},
	{"plugin", "mia plugin [ls|enable <name>|disable <name>]", "programs named mia-<name> that add verbs; installed on PATH, run once enabled", true, ""},
	{"dash", "mia dash [--popup|--switch] [env|stack|blocks|machines [worktree]]", "the dashboard; --switch is a type-to-pick list that moves this tmux client to the worktree you pick", false, ""},
	{"rename", "mia rename <worktree> <name>", "give a worktree the name you want; its session follows now, its container on its next start", false, ""},
	{"open", "mia open [worktree] [--in <cursor|code|zed|idea|subl|xcode|finder|terminal>]", "open the worktree in your editor, IDE, Finder or a terminal", false, ""},
	{"star", "mia star [worktree]", "mark the worktree you are on right now, or unmark it", false, ""},
	{"config", "mia config", "this repository's configuration, in your editor", false, ""},
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
