# Commands

Every command, with its flags. A `<worktree>` is a name, a path or a branch;
leaving it out means the worktree you are in. `-h` after any command prints
its usage.

## Worktrees

| command | does |
|---|---|
| `mia new [flags] <branch>` | create a worktree for a branch, new or existing, and name it. `--shell` attaches after; `--on <machine>` starts its environment there; `--pr <number>` fetches a pull request; `--stack [--in <worktree>] [--worktree]` adds a [stack layer](stacks.md) |
| `mia adopt <worktree>` | take over a worktree made by hand |
| `mia ls` | every worktree of this repository |
| `mia path [worktree\|stack]` | print a worktree's path; with nothing, the main checkout |
| `mia switch <worktree\|stack>` | `cd` there (needs `mia shell-init`) |
| `mia shell [--popup] [worktree]` | attach to its tmux session |
| `mia window [ls\|open\|select\|new\|close] [worktree] [window] [--editor] [-- command]` | the session's windows |
| `mia open [worktree] [--in <opener>]` | open it in cursor, code, zed, idea, subl, Xcode, Finder or Terminal |
| `mia rename <worktree> <name>` | give it another name |
| `mia star [worktree]` | star or unstar it |
| `mia setup [worktree]` | run the configured `setup` again |
| `mia run <worktree> <cmd…>` | run a command in it, on this machine |
| `mia rm [--force] [--stop-running] <worktree>` | remove it, its session and its name |
| `mia gc [--apply] [--days N]` | list, or remove, worktrees nobody has touched |
| `mia shell-init <zsh\|bash\|fish>` | the shell function and completion |

## Stacks and pull requests

| command | does |
|---|---|
| `mia up` / `mia down` | move to the layer above or below, in place |
| `mia stack [--in <worktree>]` | the stack and its layers |
| `mia stack go <layer>` | move to a layer |
| `mia stack name <name>` | name the stack |
| `mia stack merge <layer> [onto <layer>]` | fold layers into a lower one |
| `mia stack restack` | rebase every layer onto the one below |
| `mia stack diff` | this layer against the one below |
| `mia stack pr` | the push and `gh pr create` lines, bottom-up |
| `mia stack rm [--worktrees] [--branches] [--force]` | forget the stack |
| `mia pr [worktree] [show\|draft …]` | draft a pull request and print the commands to open it; `draft` takes `--title`, `--body-stdin`, `--branch`, `--remove-after-merge true\|false` |

## Environments

| command | does |
|---|---|
| `mia env up` / `down` / `rm` | start, stop or remove the container |
| `mia env shell [--once]` | a shell inside it |
| `mia env exec <cmd…>` | run a command inside it, interactively |
| `mia env run <worktree> <cmd…>` | run a command where its code runs, and exit with its status |
| `mia env setup` | run `[env] setup` again |
| `mia env ports [--json]` | what is listening |
| `mia env browse [port]` | open it in the browser |
| `mia env status [--json]` | state, machine, image, URLs |
| `mia env service [start\|stop\|restart\|logs <id>]` | its services |
| `mia env image` | bake `[env] image_setup` into an image |
| `mia env tools [--credentials]` | carry `[tools]` files in |
| `mia env dotfiles` | copy `[dotfiles] machine` files to its machine |
| `mia env host [machine]` | where it runs, or move it |
| `mia env sync [auto\|manual\|now]` | how its staging copy follows the worktree |
| `mia machine [list\|add <name> <ssh-target>\|shell <name>\|rm <name>]` | the machines environments can run on |
| `mia gateway [status\|start\|stop\|setup\|trust\|untrust\|install\|uninstall]` | what makes `<name>.mia` work |

## The rest

| command | does |
|---|---|
| `mia dash [--popup\|--switch] [env\|stack\|blocks\|machines [worktree]]` | the [dashboard](dashboard.md), or one of its panels |
| `mia config` | this repository's config, in your editor |
| `mia plugin [ls\|enable <name>\|disable <name>]` | [plugins](plugins.md) |
| `mia api <query> [worktree]` | state as JSON: `worktrees`, `dashboard`, `env-panel`, `stack-panel`, `blocks-panel`, `machines-panel`, `plugin:<name>:<panel>`, `env`, `stack`, `base` |
| `mia help [vocabulary\|tmux]` | the command list, the glossary, or the tmux bindings |
| `mia version` | the version and the commit it was built from |

Commands that read take `--json`: `ls`, `new`, `adopt`, `gc`, `window ls`,
`stack`, `pr`, `env status`, `env ports`, `machine list`, `plugin ls`.

## Exit status

| status | means |
|---|---|
| `0` | done |
| `1` | it failed; the message says why |
| `3` | no worktree, query or name like that |
| `64` | the command was used wrong; the usage is printed |

`mia run` and `mia env run` exit with the command's own status.

## Environment variables

| variable | effect |
|---|---|
| `XDG_CONFIG_HOME` | where `mia/` lives instead of `~/.config/mia` |
| `EDITOR`, `VISUAL` | the editor, when `editor` is not configured |
| `MIA_TRACE=1` | print every git, tmux and ps call mia makes, with its time |
| `MIA_TMUX_SPAWN=1` | talk to tmux with one process per call instead of one control client |

mia sets `MIA_WORKTREE` to the worktree's name in windows it starts, and the
`MIA_PLUGIN_*` variables for [plugins](plugins.md).
