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

// groupOf is every core verb's group; go is one, though help leaves it out: `mia cd` calls it.
var groupOf = map[string]string{
	"new": "Worktrees", "ls": "Worktrees", "project": "Worktrees", "go": "Worktrees", "asks": "Working", "hand": "Working", "path": "Worktrees", "adopt": "Worktrees", "rm": "Worktrees", "gc": "Worktrees",
	"star": "Worktrees", "open": "Worktrees", "shell": "Worktrees", "window": "Worktrees", "chat": "Worktrees", "git": "Worktrees", "files": "Worktrees", "review": "Worktrees", "rename": "Worktrees", "run": "Worktrees", "setup": "Worktrees", "shell-init": "Worktrees", "shell-integration": "Working",
	"stack": "Working", "up": "Working", "down": "Working",
	"config": "Working",
	"env":    "Environments", "dev": "Environments", "machine": "Environments", "gateway": "Environments",
	"api": "Records", "serve": "Records", "role": "Working", "signal": "Records", "tmux": "Working", "move": "Worktrees", "resurrect": "Worktrees", "why": "Records", "recall": "Records", "mcp": "Records", "conversation": "Records", "archive": "Worktrees", "fork": "Worktrees", "unarchive": "Worktrees", "plugin": "Records", "doctor": "Records", "main": "Working", "chats": "Working", "account": "Working",
}

var verbs = []Verb{
	{"new", "mia new [--on <machine>] [--stack [--in <worktree>] [--worktree]] [--from <ref>] [--name <name>] [--bg] [--shell] <branch> | --pr <number> [--remote <remote>]", "create a worktree and name it, or one at a pull request's head", false, ""},
	{"ls", "mia ls", "the projects mia knows, from any folder, the one you are in marked; a project's worktrees are `mia p ls`", true, ""},
	{"project", "mia project ls [project] | cd <worktree>", "a project's worktrees, here or by name, and the way into one; also `mia p`, `mia pj`, `mia prj`", true, ""},
	{"setup", "mia setup [worktree]", "run the configured setup in a worktree again", false, ""},
	{"asks", "mia asks [n [answer]]", "who waits on you in every project, the longest first, with the question and its numbered answers; `mia asks 2` goes to the second, `mia asks 2 1` gives its first answer", true, ""},
	{"hand", "mia hand [--window <index>] <agent> [--model <model>] [--effort <effort>] [--account <account>]", "the conversation of the window in front goes on with another agent, written out as a handoff and opened in a window beside", false, ""},
	{"path", "mia path [worktree|stack|project|project/worktree]", "print a worktree's path, for scripts, matched loosely by name: one of the repository here, else a project; no argument is the main checkout", true, ""},
	{"shell-init", "mia shell-init <zsh|bash|fish>", "the shell function that makes `mia cd <worktree|stack|project|project/worktree>` change directory, from any folder, and completion", true, ""},
	{"shell-integration", shellIntegrationUsage, "marks each prompt and each command's output, so tmux's previous-prompt and next-prompt jump between commands, and says the folder; mia prints it, your zshrc runs it", true, ""},
	{"gc", "mia gc [--merged] [--apply] [--days N] | --others [--apply] [path...]", "worktrees nobody has touched: clean, no session, not starred, quiet for N days; --merged: those whose branch is merged; --others: clean ones mia did not make", false, ""},
	{"adopt", "mia adopt <worktree>", "take over a worktree created by hand", false, ""},
	{"rm", "mia rm [--force] [--stop-running] [--branch] <worktree>", "remove a worktree, its session and its name", false, ""},
	{"window", windowUsage, "the windows of a worktree's session: land in one, pick one without landing, add a shell or an agent — with a model, an effort and an account — close one, read or type into one, type into one without Enter, press a key by name (C-c, Escape), switch an agent's model while it runs; account says how to put one on another account", false, ""},
	{"chat", chatUsage, "a worktree's conversation with its agent as a chat drawn by mia desk over the pane it is run in, followed as it is written: what is sent, chosen and stopped there is typed into the window that has the conversation open, the one of --window or the one that has --conversation; with no window that has one open, the last written to, only shown; --chat is a chat of mia's, main the main agent, the same chat from any folder", false, ""},
	{"git", gitUsage, "how the worktree it is run in stands in git, drawn by mia desk over the pane: its files staged, changed and untracked and its commits ahead of its base in a list, the diff of the one under the keys beside it; space stages a file or unstages a staged one, and in the diff the hunk under the keys, a stages all, c writes a commit, r asks again; it reads the standing again when the repository changes", false, ""},
	{"files", filesUsage, "the files of the worktree it is run in, drawn by mia desk over the pane: the tree, a folder opened and shut with ⏎ and a file changed against the base marked A, M or D, beside the file under the keys with its lines numbered; e hands it to $EDITOR at the line under the keys, f finds a file by its name, / finds lines in the files, r asks again; it writes nothing, and reads the files again when the repository changes", false, ""},
	{"review", reviewUsage, "the review sawa keeps of the worktree it is run in, drawn by mia desk over the pane: the change's title, its findings that stand and its verdict over its files, by folder or in sawa's reading order, and the diff of the one under the keys beside them with each finding under the line it is about; n and N go through the findings, ⏎ opens one to reply or resolve, c comments on a line, space marks a file read, v gives the verdict, r looks again; each is done by sw as you, and the review is read again when sawa's record changes", false, ""},
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
	{"mcp", "mia mcp", "an MCP server on stdio for an agent to search the conversations had with agents before, in every project, and read one (tools recall and conversation); it changes nothing. Told to an agent by the agent's own means: `claude mcp add mia -- mia mcp`", false, ""},
	{"recall", "mia recall [--all] <text…>", "the agent conversations here (with --all, or in no repository, in every project) that mention it — claude's, codex's, pi's, opencode's, agy's — newest first, and how to resume each", true, ""},
	{"conversation", conversationVerbUsage, "continue goes on with a worktree's conversation with another agent — claude's with codex, pi or any other: no agent loads another's, so the conversation is written out as a handoff in the repository's git folder, who said what and each thing done in a line, and a window opens with the other agent told to read it and go on from where it stops; --from says which by a window, an agent or an id, else the last written to, and --dry-run only says. resume opens a window with the agent taking a conversation up again, in the worktree it was made in or another, started with a model and an effort and on an account as `mia window new` starts one — one made on another account is copied to it; --worktree picks the copy of one that is in several; `mia api conversations` lists them", false, ""},
	{"main", mainUsage, "the main agent: the one you talk to, which runs the others — one for you, in a session of its own, no repository's; start --agent starts another agent than the configured one, this once; say types a message into it, press types keys with no Enter (pick an option: a number moves the cursor there and presses Enter when the options have none), say and model type nothing while it asks something, interrupt stops it mid-work, model switches its model while it runs, switch has it go on with another agent, its conversation written out and said in front of the next thing said to it, that agent from then on", false, ""},
	{"chats", chatsUsage, "plain conversations with any agent, which you come back to and go on with: each a main agent of its own, in a folder and a session of its own and no repository's, that does what you ask and runs the other agents through mia as the main agent does; the main agent is the chat named main. new prints the name the registry gives it, starts it and says its first words once it is ready, held while it asks something first; say, press, interrupt, model, switch, stop and start are the main agent's; remove takes its folder and name, and its conversations stay with its agent", false, ""},
	{"account", accountUsage, "an agent's logins: each a folder of its own the agent keeps its sign-in and conversations in, the agent's usual one called default; add makes one and opens a window to sign in to it with the agent's own login; signals writes pi the extension that tells mia what it does — working, asks, done — as claude's hooks do, only when asked, and never over a file of yours; mia never reads, copies or moves a sign-in", false, ""},
	{"role", roleUsage, "the roles: each a setup — agent, model, effort, account — with opening instructions, a file of yours in roles/<name>.md, listed by mia api setups; mia window new --role and mia main start --role start one and send it its instructions once ready", true, ""},
	{"why", "mia why <file:line|commit>", "the claude conversation that made a commit, or the line's last commit: what you asked, and how to resume it", true, ""},
	{"tmux", "mia tmux [install|uninstall|keys <kitty|ghostty>]", "mia's part of tmux — the one list, the popup terminal, its keys and its part of the status line — printed, added to ~/.tmux.conf, or taken out; keys prints a terminal app's lines for the ⌘ keys", true, ""},
	{"signal", "mia signal <worktree> <kind> [text] [--style s] [--rank n] [--from source] [--for d|--until seen] · clear · ls", "what anything outside mia says about a worktree, shown as its setter asks", false, ""},
	{"api", "mia api <query>", "structured state, for clients and scripts", true, ""},
	{"serve", serveUsage, "the server of this machine, in the foreground or, installed, as a launchd or systemd service: over a socket in mia's config folder it answers `mia api`, which asks it when it is the same build, says when an answer changes and runs verbs; it holds no truth, and killing it loses nothing", false, ""},
	{"doctor", "mia doctor", "check tools, configuration, gateway, machines and environments", true, ""},
	{"plugin", "mia plugin [ls|enable <name>|disable <name>]", "programs named mia-<name> that add verbs; installed on PATH, run once enabled", true, ""},
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
