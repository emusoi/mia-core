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

Plugins fill the four sides around the box: waiting on you above it, issues and pull requests left, cost right, activity below.
Each side shows one tab at a time; its tab row starts with the side's number
(1 top, 2 left, 3 right, 4 bottom) and lists every tab with a count.
A side is drawn only where it fits — 30 free columns left or right, 6 free
rows (and the side's width) above or below — and too small for any side,
the dash is just the box. In a tmux popup the same holds; the box fills the
popup only when no side fits.

| key | does |
|---|---|
| `1`–`4` | move the cursor into the top, left, right or bottom side; the same digit brings it back |
| `j` `k` | the side's next or previous item |
| `⏎` | open the item: a popup, the plugin's command, or its agent's window |
| `tab` `shift+tab` | the selected side's next or previous tab; with none selected, the side pane's |
| `esc` | the cursor back to the box |

`mia dash --side N` (also with `--popup`) opens with the cursor already in
side N.

The selected side's number is a green chip, its tab and the two dotted lines
that frame it turn green, the item under the cursor takes the box's
selection, the box's border dims, and the bottom-left corner reads
`j k move   ⏎ open   tab tabs   esc back`. Lines that open nothing are
skipped.

`v` opens the sides legend in the bottom-left corner:

| key | does |
|---|---|
| `1`–`4` | turn the top, left, right or bottom side on or off |
| `esc` `v` | close the legend |

What you choose is kept per repository in `.git/mia/dash-sides.json`.

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
| `[` `]` | the side pane's previous or next tab, even with a side selected |
| `f` | the side pane's tab in a tmux popup over the list |
| `q` | back, or quit |

Change any key under `[keys]` in the config.

## In tmux

`mia help tmux` prints lines for `~/.tmux.conf`: `prefix g` opens the
dashboard in a full-size popup, `prefix j` jumps to a worktree by typing its
name, and `prefix m` is a menu of popups for the worktree of the pane you
are in:

| key | opens |
|---|---|
| `g` `j` | the dashboard, the switch list |
| `n` `b` `p` | a new worktree, a branch, a pull request |
| `1`–`4` | the dashboard with the cursor in that side, named for the side's first tab (waiting, issues, cost, activity) |
| a plugin's | what its manifest's `menu` names; one on a key above is left out and said on stderr |

Run it again after enabling a plugin to pick up its entries.
