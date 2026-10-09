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

## The room

When the terminal has space, the box sits in a room. Dotted lines run from
each terminal corner to the box's corner, and each corner carries one line:
`mia` and the repository top left, the date and time top right, and what
plugins put in the bottom corners (agents waiting, today's spend).

Plugins fill the four sides around the box with information: waiting on
you above it, issues and pull requests left, cost right, activity below.
Each side shows one tab at a time; its tab row lists them all with a count.
A side is drawn only where it fits — 30 free columns left or right, 6 free
rows (and the side's width) above or below — and too small for any side,
the dash is just the box.

`v` opens the sides legend in the bottom-left corner:

| key | does |
|---|---|
| `w` `i` `c` `a` | turn the top, left, right or bottom side on or off |
| `[` `]` | switch the tabs of the side last turned (top if none) |
| `esc` `v` | close the legend |

Outside the legend the sides take no keys. What you choose is kept per
repository in `.git/mia/dash-sides.json`.

## Keys

| key | does |
|---|---|
| `⏎` | attach to the worktree's session |
| `n` | new worktree |
| `/` | find |
| `o` | open in an editor |
| `*` | star |
| `E` | environment |
| `V` | lend main's dev server, or give it back |
| `v` | the sides legend, when the room is drawn |
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
