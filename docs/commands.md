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
| `mia run <worktree> <cmd…>` | run a command in it |
| `mia rm <worktree>` | remove it |
| `mia gc [--apply]` | find, or remove, untouched worktrees |

## Stacks

| command | does |
|---|---|
| `mia up`, `mia down` | move between layers |
| `mia stack` | show the stack |
| `mia stack go\|name\|merge\|restack\|diff\|rm` | work on it |

## Environments

| command | does |
|---|---|
| `mia env up\|down\|rm` | start, stop, remove |
| `mia env shell` | a shell inside |
| `mia env exec <cmd…>` | run a command inside |
| `mia env run <worktree> <cmd…>` | run where its code runs |
| `mia dev [worktree\|off]` | lend main's dev server a worktree's files |
| `mia env browse` | open it in the browser |
| `mia env status\|ports\|service` | what it is doing |
| `mia env host [machine]` | where it runs, or move it |
| `mia machine list\|add\|shell\|rm` | machines |
| `mia gateway status\|start\|stop\|setup\|trust` | the `<name>.mia` gateway |

## Other

| command | does |
|---|---|
| `mia radar [--new]` | find and switch to any worktree, or make one — `prefix space` and `prefix n` in tmux |
| `mia config` | edit the repository's config |
| `mia config [--global] <key> [<value>]` | say, or set, one setting: `base staging`, `keys.quizzicle.quiz K`, `plugin.agent.default codex`; one line edited, comments kept, and a setting mia would not load refused. `B` in the dashboard sets `base` |
| `mia plugin ls\|enable\|disable` | plugins |
| `mia api <query>` | state as JSON |
| `mia help vocabulary` | the words |
| `mia help tmux` | what `mia tmux install` adds: the radar, a new worktree, the status line |

Reading commands take `--json`.
