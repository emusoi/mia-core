# The dashboard

```bash
mia dash
```

Every worktree of the repository, grouped:

| section | holds |
|---|---|
| main / starred | the repository's checkout and starred worktrees |
| in progress | uncommitted changes, or commits ahead |
| quiet | everything else; `e` shows it |
| not mia's | worktrees mia has not adopted |

A stack is one row; `⏎` unfolds it. The right side shows the selected row's
facts and session windows when the terminal is wide enough.

`?` lists every key and the command it runs.

## Keys

| key | does |
|---|---|
| `⏎` | attach to the worktree's session |
| `n` | new worktree |
| `/` | find |
| `o` | open in an editor |
| `*` | star |
| `E` | environment |
| `S` | stack |
| `t` | session windows |
| `Y` | draft a pull request |
| `D` | delete |
| `a` | adopt |
| `c` | config |
| `j` `k` | move |
| `q` | back, or quit |

Change any key under `[keys]` in the config.

## In tmux

`mia help tmux` prints lines for `~/.tmux.conf`: `prefix g` opens the
dashboard in a popup, `prefix j` jumps to a worktree by typing its name.
