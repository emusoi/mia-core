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
| `mia conversation resume <id> [--in <worktree>]` | open a window with the agent taking the conversation up again, in the worktree it was made in or in another |
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
| `mia machine list\|add\|shell\|rm` | machines |
| `mia gateway status\|start\|stop\|setup\|trust` | the `<name>.mia` gateway |

## Other

| command | does |
|---|---|
| `mia radar [--new]` | find and switch to any worktree, or make one — `prefix space` and `prefix n` in tmux |
| `mia dash` | the radar, under its old name |
| `mia config [show\|validate]` | edit local overrides, or inspect the effective configuration |
| `mia config [--global] <key> [<value>]` | say, or set, one setting: `base staging`, `keys.quizzicle.quiz K`, `plugin.agent.default codex`; one line edited, comments kept, and a setting mia would not load refused |
| `mia plugin ls\|enable\|disable` | plugins |
| `mia doctor` | check tools, config, gateway, machines, plugins and environment drift |
| `mia api <query>` | state as JSON |
| `mia api capabilities` | API version, queries and feature markers |
| `mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]` | the agent conversations of the repository and its worktrees, the last written to first |
| `mia api conversation <id> [--after <n>]` | one conversation as turns, for a chat view |
| `mia help vocabulary` | the words |
| `mia help tmux` | what `mia tmux install` adds: the radar, a new worktree, the menu, the status line |

Reading commands take `--json`.
