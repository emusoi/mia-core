# Commands

`<worktree>` is a name, path or branch; leave it out for the one you are in.
`mia <command> -h` shows a command's usage.

## Worktrees

| command | does |
|---|---|
| `mia new <branch>` | make a worktree |
| `mia adopt <path>` | take over a worktree |
| `mia ls` | list worktrees |
| `mia shell [worktree]` | attach to its session |
| `mia switch <worktree>` | cd there |
| `mia path [worktree]` | print its path |
| `mia open [worktree]` | open it in your editor |
| `mia window …` | its session's windows |
| `mia window new <worktree> <agent> [--model <m>] [--effort <e>] [-- command]` | a window running an agent, started with a model and an effort |
| `mia window model <worktree> <window> <model> [--effort <e>]` | switch the agent in a window to another model while it runs |
| `mia window new <worktree> [window] --role <role>` | a window running the agent a role says, with its model, effort and account, sent the role's body once it is ready |
| `mia role [ls]`, `mia role show <role>`, `mia role path <role>` | the roles: setups with opening instructions, files of yours in `~/.config/mia/roles/<role>.md` |
| `mia window new <worktree> [window] --kind <kind> [-- command]` | a window marked as another [kind](tmux.md#pane-kinds) than its command says: `agent`, `shell`, `editor`, `server`, `tests` or `logs` |
| `mia window new <worktree> <agent> --account <a>` | a window running an agent on another account than the repository's, this once |
| `mia window account <worktree> <window> <a>` | says how to put a running agent on another account: end it, then resume its conversation with `--account` |
| `mia account [ls]` | every agent's accounts: its own folder, `default`, and the ones added — signed in or not, and what runs on each |
| `mia account add <agent> <name> [--in <worktree>]` | a new account's folder, and a window running the agent on it to sign in to with the agent's own login |
| `mia account login <agent> <name> [--in <worktree>]` | a window running the agent on an account it has, `default` too, to sign in to it again: one signed out, or whose sign-in ran out |
| `mia account share <agent> <name>`, `mia account share <agent> --all` | link the agent's own [setup](worktrees.md#its-setup-shared) into an account, or every one of mia's: what is missing there, never a sign-in; `add` and `login` do it first |
| `mia account signals <agent> [<name> \| --all] [--dry-run]` | write pi the [extension](tmux.md#pi-says-what-it-does) that tells mia what it does, in its own folder, an account's or every one's; never over a file of yours, and `--dry-run` only says |
| `mia account use <agent> <name> [--repository <r>]` | the account a repository starts the agent's sessions on from now on; `default` gives it back to the config |
| `mia account use <agent> <name> --global` | the user-wide one, `[agents.<agent>] account` in `~/.config/mia/config.toml`, for every repository that says none; `default` takes the line out |
| `mia account rename <agent> <name> <new>`, `mia account rm [--force] <agent> <name>` | rename or remove one; one a window runs on is refused, and so is `default` |
| `mia rename <worktree> <name>` | rename it |
| `mia star [worktree]` | star or unstar it |
| `mia setup [worktree]` | run setup again |
| `mia run <worktree>[@machine] <cmd…>` | run a command in it, here or on a machine |
| `mia rm <worktree>` | remove it |
| `mia fork [worktree] <branch>` | a new worktree on a new branch from where that worktree is, its uncommitted and untracked work copied; the first stays as it is |
| `mia archive [--stop-running] <worktree>` | put it away: everything in it, uncommitted and untracked work too, kept under `refs/mia/archive/<name>`; the worktree goes, its branch stays |
| `mia archive ls` | what is archived |
| `mia unarchive <name>` | bring it back as it was, under its old name when that is free |
| `mia recall <text…>` | the agent conversations of this repository and its worktrees that mention it — claude's, codex's, pi's — newest first: the words around it, and how to resume each |
| `mia conversation resume <id> [--worktree <name>] [--in <worktree>] [--model <m>] [--effort <e>] [--account <a>]` | open a window with the agent taking the conversation up again, in the worktree it was made in or in another, started with a model and an effort, on the repository's account or the one named — copied into that account's folder when it was made on another; `--worktree` picks the copy of one that is in several |
| `mia conversation continue <worktree\|.> --with <agent> [--from <window\|agent\|id>] [--model <m>] [--effort <e>] [--account <a>] [--dry-run]` | go on with a worktree's conversation [with another agent](worktrees.md#with-another-agent): it is written out as a handoff in the repository's git folder, and a window opens with that agent told to read it and go on from where it stops; `--dry-run` only says |
| `mia why <file:line\|commit>` | the claude conversation that made a commit, or the line's last commit: what you asked, and how to resume it |
| `mia resurrect [--dry-run] [--list] [<worktree>...]` | after tmux or the computer went down, open each session again — or any one named, closed on purpose or not: its windows in their folders, their programs restarted, each claude resuming its own conversation; `--list` shows every session remembered |
| `mia gc [--apply\|--ask] [--days N]` | find, or remove, idle worktrees: clean, no session, not starred, untouched for N days (14); `--ask` shows them and removes on `y` |
| `mia gc --merged [--apply]` | find, or remove, clean worktrees whose branch the base already holds |
| `mia gc --others [--apply\|--ask] [path…]` | find, or remove, the clean worktrees mia did not make; branches stay |

## Stacks

| command | does |
|---|---|
| `mia up`, `mia down` | move between layers |
| `mia stack` | show the stack |
| `mia stack go\|name\|sync\|merge\|restack\|diff\|rm` | work on it |

## Environments

| command | does |
|---|---|
| `mia env up [--wait] [--recreate]` | start; optionally wait for readiness or apply changed configuration |
| `mia env down [--all]\|rm` | stop one or all, or remove |
| `mia env shell` | a shell inside |
| `mia env exec <cmd…>` | run a command inside |
| `mia env run <worktree> <cmd…>` | run where its code runs |
| `mia dev [worktree\|off]` | lend main's dev server a worktree's files |
| `mia env browse [--direct]` | open it in the browser, with optional no-setup HTTP |
| `mia env status\|ports\|service` | what it is doing |
| `mia env host [machine]` | where it runs, or move it after a target preflight |
| `mia env sync [worktree] [auto\|manual\|now\|status\|conflicts\|preview\|restart]` | control and inspect remote file synchronization |
| `mia machine list\|add\|shell\|rm` | machines; the list says how each stood when last asked — `ok`, or what is wrong and the fix — and asks none |
| `mia gateway status\|start\|stop\|setup\|trust` | the `<name>.mia` gateway |

## Other

| command | does |
|---|---|
| `mia radar [--new]` | find and switch to any worktree, or make one — `prefix space` and `prefix n` in tmux |
| `mia dash` | the radar, under its old name |
| `mia config [show\|validate]` | edit local overrides, or inspect the effective configuration |
| `mia config [--global] <key> [<value>]` | say, or set, one setting: `base staging`, `keys.quizzicle.quiz K`, `plugin.agent.default codex`; one line edited, comments kept, and a setting mia would not load refused |
| `mia plugin ls\|enable\|disable` | plugins |
| `mia doctor` | check tools, config, gateway, machines, plugins and environment drift; of a machine that does not answer, what is wrong and the fix |
| `mia api <query>` | state as JSON |
| `mia api capabilities` | API version, queries and feature markers |
| `mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]` | the agent conversations of the repository and its worktrees, the last written to first |
| `mia api accounts [--projects]` | every agent's accounts, or which one each repository starts each agent's sessions on |
| `mia api conversation <id> [--worktree <name>] [--after <n>]` | one conversation as turns, for a chat view; `--worktree` picks the copy of one that is in several |
| `mia main [status]` | the [main agent](main.md): whether it runs, and what it is doing |
| `mia main start [--agent <name>] [--model <m>] [--effort <e>] [--account <a>]` | make its folder and session when they are not there and start the agent, going on with its last conversation; one that runs is left as it is. `--agent` starts another than `[main] agent`, this once |
| `mia main start --resume <id> [--restart] [--model <m>] [--effort <e>]` | start it going on with one of its [earlier conversations](main.md#earlier-conversations), as the agent that had it; one that runs is refused, unless `--restart` says to end it |
| `mia main start --new [--restart] [--agent <name>] [--model <m>] [--effort <e>] [--account <a>]` | start it with a [new conversation](main.md#a-new-conversation), going on with none; the one it had stays. One that runs is refused, unless `--restart` says to end it and start it again as it was |
| `mia main stop` | end its session; its folder and conversations stay |
| `mia main say <text…>` | type a message into it and submit it; refused while it [asks something](main.md#when-it-asks-first) |
| `mia main press <keys…>` | type keys into it with no Enter after: `mia main press 1` picks an option when it is the main agent that asks, moving its cursor there and pressing Enter when its options have no numbers |
| `mia main interrupt` | stop it mid-work |
| `mia api main` | where the main agent stands, from any folder |
| `mia api main turns [--conversation <id>] [--after <n>]` | its conversation as turns, each of mia's verbs it ran said as what it did; `--conversation` reads one of its earlier ones |
| `mia api main conversations` | the conversations made in its folder, with every agent it was run as, the one it has open marked `running` |
| `mia api setups [--repository <name> \| --all]` | the agent · model · effort setups used, the repository's usual first, from any folder |
| `mia api standing [--here] [--seen <repository>/<worktree>]` | every agent in every repository, and on every machine: which wait on you, which work, which finished unseen |
| `mia help vocabulary` | the words |
| `mia shell-integration zsh` | prints what marks each prompt and its output for tmux: `eval "$(mia shell-integration zsh)"` in your zshrc — see [Prompt marks](tmux.md#prompt-marks) |
| `mia help tmux` | what `mia tmux install` adds: the radar, a new worktree, the menu, the status line |

Reading commands take `--json`.
