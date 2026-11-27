# Commands

Every command, grouped as `mia help` groups them. `<worktree>` is a name,
path or branch; leave it out for the one you are in. `mia <command> -h`
shows a command's usage, and every reading command takes `--json`.

## Worktrees

| command | does |
|---|---|
| `mia adopt <worktree>` | take over a worktree mia did not make, one from `git worktree add` or another tool |
| `mia archive [--stop-running] <worktree>` · `mia archive ls` | put a worktree away, uncommitted and untracked work too, under `refs/mia/archive/<name>`; its branch stays. [Worktrees](worktrees.md) |
| `mia chat [--window <index>] [--conversation <id>]` · `mia chat --chat <name>` | a worktree's conversation with its agent, as a chat mia desk draws over the pane; `--chat main` is the main agent's. [Chats](chats.md#shown-as-a-chat) |
| `mia files` | the worktree's files, as an app mia desk draws over the pane. [Automation](api.md#mia-files) |
| `mia fork [worktree] <branch>` | a second try: a new worktree on a new branch from where that one is, its uncommitted and untracked work copied |
| `mia gc [--merged] [--apply\|--ask] [--days N]` | find, or remove, idle worktrees: clean, no session, not starred, untouched for N days (14); `--merged`, those whose branch is merged; `--ask` asks `y` first |
| `mia gc --others [--apply\|--ask] [path…]` | find, or remove, the clean worktrees mia did not make; branches stay |
| `mia git` | how the worktree stands in git, as an app mia desk draws over the pane. [Automation](api.md#mia-git) |
| `mia go <worktree\|project\|project/worktree>` | in tmux, take this client to that worktree's session, started if it is not up. [mia in tmux](tmux.md#the-popup-terminal) |
| `mia ls` | the projects mia knows, from any folder, the one you are in marked |
| `mia move [--dry-run] <worktree>[@<machine>] <machine\|local>` | take a worktree to a machine or back: its branch, uncommitted work, windows and their agents' conversations. [Machines](machines.md#moving-a-worktree) |
| `mia new [--from <ref>] [--name <name>] [--bg] [--shell] [--on <machine>] <branch>` | make a worktree on a branch, and name it. [Worktrees](worktrees.md) |
| `mia new --stack [--in <worktree>] [--worktree] <branch>` | a new [stack](stacks.md) layer, in place or in a worktree of its own |
| `mia new --pr <number> [--remote <remote>]` | a worktree at a pull request's head |
| `mia open [worktree] [--in <cursor\|code\|zed\|idea\|subl\|xcode\|finder\|terminal>]` | open it in your editor, an IDE, Finder or a terminal |
| `mia path [worktree\|stack\|project\|project/worktree]` | print a path, for scripts; with nothing, the main checkout |
| `mia project ls [project]` · `mia project cd <worktree>` | a project's worktrees, here or by name, and the way into one; also `mia p`, `mia pj`, `mia prj` |
| `mia rename <worktree> <name>` | give a worktree another name; its session follows now, its container on its next start |
| `mia resurrect [--dry-run] [--list] [--ask] [<worktree>…]` | open sessions again after tmux went down, or any named: windows, programs, each claude on its own conversation; `--list` shows all remembered |
| `mia review [<range> \| pr <number> \| w]` | the review sawa keeps of the worktree, as an app mia desk draws over the pane. [Automation](api.md#mia-review) |
| `mia rm [--force] [--stop-running] [--branch] <worktree>` | remove a worktree and its name; `--force` even with uncommitted changes, `--stop-running` ending its session, `--branch` its branch too |
| `mia run <worktree>[@machine] <cmd…>` | run a command once in a worktree, here or on a machine |
| `mia setup [worktree]` | run the configured setup again |
| `mia shell [--popup \| --view <name>] [worktree]` | attach to a worktree's session; `--view` at this terminal's own size and window; `name@machine` attaches there |
| `mia shell-init <zsh\|bash\|fish>` | the shell function that lets `mia switch` and `mia cd` change folder, and completion: `eval "$(mia shell-init zsh)"` in your rc |
| `mia star [worktree]` | star it, or unstar it; `mia gc` leaves a starred one alone |
| `mia switch <worktree\|stack\|project\|project/worktree>` | cd there, from any folder, once `mia shell-init` is in your rc; in tmux the client goes to its session too. Also `mia cd` |
| `mia unarchive <name>` | bring an archived worktree back as it was, under its old name when that is free |
| `mia window [ls] [worktree]` | a worktree's windows. [Worktrees](worktrees.md) |
| `mia window new [worktree] [window] [--editor] [--kind <kind>] [-- command]` | a new window: a shell, the editor, a `[launch]` name, or a command; `--kind` [marks it](tmux.md#pane-kinds) |
| `mia window new [worktree] <agent> [--model <m>] [--effort <e>] [--account <a>] [--task <text>]` | a window running an agent, on that model and account, told the task once it is ready. [Agents](agents.md) |
| `mia window new [worktree] [window] --role <role>` | a window running what a [role](agents.md#a-role) says, sent its instructions once ready |
| `mia window open\|select\|close [worktree] <window>` | land in a window, pick it without landing, or close it |
| `mia window read [worktree] <window>` | print what a window shows |
| `mia window send\|press\|key [worktree] <window> -- <text\|keys>` | type a line and Enter, type with no Enter, or press keys by name (`C-c`, `Escape`) |
| `mia window model [worktree] <window> <model> [--effort <e>]` | switch a running agent's model. [Agents](agents.md#an-agents-model) |
| `mia window account [worktree] <window> <account>` | say how to put a running agent on another account. [Agents](agents.md#an-agents-account) |

## Working

| command | does |
|---|---|
| `mia account [ls]` | every agent's accounts, signed in or not, and what runs on each. [Agents](agents.md#an-agents-account) |
| `mia account add\|login <agent> <name> [--in <worktree>]` | make an account and open a window to sign in to it, or sign in to one again |
| `mia account share <agent> (<name> \| --all)` | link the agent's own setup into an account; never a sign-in |
| `mia account use <agent> <name> [--repository <name> \| --global]` | the account a repository, or every repository, starts the agent on; `default` gives it back |
| `mia account rename <agent> <name> <new>` · `mia account rm [--force] <agent> <name>` | rename or remove an account; one a window runs on is refused, and so is `default` |
| `mia account signals <agent> [<name> \| --all] [--dry-run]` | write pi the [extension](tmux.md#pi-says-what-it-does) that tells mia what it does; never over a file of yours |
| `mia asks [n [answer]]` | who waits on you in every project, longest first, with numbered answers; `mia asks 2` goes to the second, `mia asks 2 1` gives its first answer |
| `mia chats [list]` | the [chats](chats.md), the main agent first |
| `mia chats new --agent <agent> [--model <m>] [--effort <e>] [-- <first words…>]` | make a chat, print its name and start it, saying its first words once it is ready |
| `mia chats say <name> -- <text…>` · `press <name> <keys…>` · `interrupt <name>` · `model <name> <model> [--effort <e>]` · `switch <name> --agent <agent>` | what `mia main` does, to that chat; `main` is the main agent |
| `mia chats stop\|start\|remove <name>` | end its session, start it again on its last conversation, or remove it and its name |
| `mia config` | open the repository's configuration in your editor. [Configuration](configuration.md) |
| `mia config show\|validate` | print, or check, the configuration in effect |
| `mia config [--global] <key> [<value>]` | say, or set, one setting: `base staging`, `keys.quizzicle.quiz K`; a setting mia would not load is refused |
| `mia dash` | the radar, under its old name |
| `mia down` · `mia up` | move to the [stack](stacks.md) layer below or above, in place |
| `mia hand [--window <index>] <agent> [--model <m>] [--effort <e>] [--account <a>]` | the conversation in front goes on with another agent, in a window beside. [Conversations](conversations.md#with-another-agent) |
| `mia main [status]` | the [main agent](main.md): whether it runs, and what it is doing |
| `mia main start [--new \| --resume <id>] [--restart] [--role <role>] [--agent <agent>] [--model <m>] [--effort <e>] [--account <a>]` | start it on its last conversation, a new one, or an earlier one; `--restart` ends one that runs first |
| `mia main stop` · `interrupt` | end its session, or stop it mid-work |
| `mia main say <text…>` · `press <keys…>` | type a message and submit it, or type keys with no Enter to pick an option. [The main agent](main.md#saying-something-to-it) |
| `mia main model <model> [--effort <e>]` · `switch --agent <agent> [--model <m>] [--effort <e>]` | switch its model while it runs, or have it go on as another agent |
| `mia radar [--new]` | the [radar](tmux.md#the-radar), `prefix space`; `--new` the new-worktree popup, `prefix n` |
| `mia role [ls]` · `mia role show\|path <name>` | the [roles](agents.md#a-role): setups with opening instructions, files of yours |
| `mia shell-integration zsh` | the zsh lines that mark each prompt for tmux. [Prompt marks](tmux.md#prompt-marks) |
| `mia stack [--in <worktree>] [show]` | the stack and its layers. [Stacks](stacks.md) |
| `mia stack go <layer>\|name <name>\|track <branch> <parent>\|sync\|merge <layer> [onto <layer>]\|restack\|diff\|rm […]` | work on the stack |
| `mia tmux [install\|uninstall]` | print, add or take out mia's part of `~/.tmux.conf`. [mia in tmux](tmux.md) |
| `mia tmux keys <kitty\|ghostty>` | the lines a terminal app's config takes so its ⌘ keys send mia's |

## Environments

| command | does |
|---|---|
| `mia dev [<worktree> [--follow] \| off]` | lend the main checkout's running dev server a worktree's files. [Environments](environments.md#mains-dev-server) |
| `mia env up [--wait] [--recreate] [worktree]` | start a worktree's environment; wait until it is ready, or apply a changed config. [Environments](environments.md) |
| `mia env down [--all]` · `mia env rm` | stop it, or every one; remove it |
| `mia env shell` · `exec <cmd…>` · `run <worktree> <cmd…>` | a shell inside, a command inside, or a command where its code runs |
| `mia env status\|ports\|service` | what it is doing, its ports, its services; `service start\|stop\|restart\|logs <id>` |
| `mia env browse [--direct]` | open it in the browser; `--direct` plain HTTP, no setup |
| `mia env setup\|dotfiles\|tools\|image` | run setup inside, copy your dotfiles or tools in, build its image |
| `mia env host [machine]` | where it runs, or move it there |
| `mia env sync [auto\|manual\|now\|status\|conflicts\|preview\|restart]` | the file sync to an environment on a machine. [Environments](environments.md#sync) |
| `mia gateway status\|start\|stop\|install\|uninstall\|setup\|trust\|untrust` | what makes `<name>.mia` reach an environment. [Environments](environments.md) |
| `mia machine list` | the machines, and how each stood when last asked: `ok`, or what is wrong and the fix. [Machines](machines.md) |
| `mia machine add <name> <ssh-target>` · `rm <name>` | add a machine, or forget one |
| `mia machine shell <name>` | a shell on it |
| `mia machine ship <name>` | put this mia on it. [Machines](machines.md#shipping-mia) |
| `mia machine link <name>` | give it this repository, its base branch pushed |
| `mia machine attach <worktree>@<name>` | attach to that worktree's session there |
| `mia machine open <name> <port>` | open that port of it in the browser |

## Records

| command | does |
|---|---|
| `mia api <query>` | mia's state as JSON, for apps and scripts; `mia api capabilities` lists the queries. [Automation](api.md) |
| `mia conversation resume <id> [--worktree <name>] [--in <worktree>] [--model <m>] [--effort <e>] [--account <a>]` | a window with the agent taking a conversation up again. [Conversations](conversations.md#going-on-with-one) |
| `mia conversation continue (<id> \| --from <window>) --with <agent> [--worktree <name>] [--model <m>] [--effort <e>] [--account <a>] [--dry-run]` | go on with a conversation as another agent, from a handoff it is told to read. [Conversations](conversations.md#with-another-agent) |
| `mia doctor` | check tools, configuration, gateway, machines, plugins and environments, and say the fix |
| `mia mcp` | an MCP server on stdio that lets an agent search and read past conversations; it changes nothing. [Conversations](conversations.md#for-agents-mia-mcp) |
| `mia plugin [ls] \| enable <name> \| disable <name>` | programs named `mia-<name>` on your PATH that add verbs. [Plugins](plugins.md) |
| `mia recall [--all] <text…>` | the agent conversations that mention it, newest first, and how to resume each; `--all` every project and machine. [Conversations](conversations.md#finding-one) |
| `mia serve` | this machine's [server](serve.md): `mia api` over a socket, and events when an answer changes |
| `mia serve install\|uninstall\|start\|stop\|status` | the server as a [launchd or systemd service](serve.md#as-a-service) |
| `mia signal <worktree\|.> <kind> [text] [--style s] [--rank n] [--from source] [--for d \| --until seen] [--yes keys]` | say something about a worktree, shown as you ask. [Signals](tmux.md#signals) |
| `mia signal <worktree> clear [--from source]` · `mia signal ls` | clear a worktree's signals, or list those standing |
| `mia why <file:line\|commit>` | the claude conversation that made a commit, or a line's last commit, and how to resume it |

`mia help vocabulary` says what things are called; `mia help tmux` prints
what `mia tmux install` adds; `mia version` prints the version.
