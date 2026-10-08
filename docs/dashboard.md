# The dashboard

    mia dash

The dashboard is every worktree of the repository in one list, grouped by
what it needs from you, with the keys to act on them. Every key runs a `mia`
command — `?` shows which — so nothing it does is out of reach of a script.

## Over tmux

The dashboard is best as a tmux popup over whatever you are doing, gone again
when you leave it. `mia help tmux` prints the lines for `~/.tmux.conf`:

```
bind-key g display-popup -B -E -e MIA_POPUP=1 -e "MIA_TMUX_PANE=#{pane_id}" -d "#{pane_current_path}" -w 80% -h 70% "mia dash"
bind-key j display-popup -B -E -e MIA_POPUP=1 -e "MIA_TMUX_PANE=#{pane_id}" -d "#{pane_current_path}" -w 60% -h 50% "mia dash --switch"
```

- `prefix g` opens the dashboard over the current window.
- `prefix j` is a type-to-pick list: type part of a name or branch, `⏎`, and
  your tmux client moves to that worktree's session.
- `prefix m` (in the full output) is a menu: dashboard, switch, a new
  worktree, a new branch, a pull request.

`mia dash --popup` opens the same popup from a shell inside tmux.

## What it shows

```
╭── Worktrees / shop ─────────────────────────────────────────────╮
│                                                                  │
│   main  1                                                        │
│   shop        main                          2h                   │
│                                                                  │
│   in progress  2                                                 │
│ ± monduli     you/due-dates  uncommitted   12m                   │
│   kijenge     you/search     +1 −4          3h                   │
│                                                                  │
│   quiet  4                                                       │
╰── ⏎ attach   n new   / find   ? keys ───────────────────────────╯
```

**Sections**, top to bottom:

| section | holds |
|---|---|
| *main* / *starred* | the repository's own checkout, always first, and every starred worktree |
| *in progress* | worktrees with uncommitted changes or commits ahead of the base |
| *quiet* | everything else; folded, `e` opens it |
| *not mia's* | git worktrees mia has no record of; `a` adopts one |

[Plugins](plugins.md) can add sections of their own between these.

**Columns**: the name, the branch (without your `prefix`), a status —
`uncommitted`, or `+ahead −behind` — and how long since the last commit.
`±` marks uncommitted changes, `○` a detached checkout.

**Stacks** are one row: their name, `3 layers · 1 landed`, and the status of
the top layer. `⏎` or `l` unfolds a stack into its layers, top first; layers
with no worktree of their own are dimmed, and `W` gives one a worktree.

**The side pane** appears when the terminal is wide enough. Its tabs (`[`
and `]`, or a digit) show the row's *facts* — branch, drift, changed files,
the last commit, where its environment runs — its session's *blocks*
(windows), and any tabs plugins add. `i` shows it full screen.

The list refreshes itself every couple of seconds.

## Keys

`?` lists every key that works on the selected row, with the command it runs.
These are the defaults.

### On a worktree

| key | does | runs |
|---|---|---|
| `⏎` | attach to its session | `mia shell <worktree>` |
| `s` | a shell in its session | `mia shell <worktree>` |
| `o` | open it in an editor, IDE, Finder or a terminal | `mia open <worktree> --in …` |
| `*` | star or unstar | `mia star <worktree>` |
| `E` | its environment | the environment panel |
| `S` | its stack | the stack panel |
| `t` | its session's windows | the windows panel |
| `Y` | the pull request mia would draft | `mia pr <worktree>` |
| `N` | start a stack here | `mia new --stack --in <worktree> …` |
| `D` | delete it | `mia rm <worktree>`, asking first |
| `a` | adopt a worktree mia does not know | `mia adopt <worktree>` |

When `D` is refused for uncommitted changes or a running session, the
dashboard offers the way past — discard, or stop what is running — and asks
again.

### On a stack

| key | does |
|---|---|
| `⏎` `l` `h` | unfold and fold |
| `n` | a new layer on top |
| `N` | a new layer in a worktree of its own |
| `=` | name the stack |
| `R` | restack every layer |
| `M` | merge a layer down onto a lower one |
| `W` | give a layer without one a worktree |
| `D` | forget the stack (branches and worktrees stay) |
| `*` | star it (its top worktree) |

### Anywhere

| key | does | runs |
|---|---|---|
| `n` | a new worktree, staying on the list | `mia new <branch>` |
| `+` | a new worktree, and go there | `mia new --shell <branch>` |
| `P` | a pull request in a worktree of its own | `mia new --shell --pr <number>` |
| `b` | the machines environments can run on | the machines panel |
| `z` / `Z` | list / remove the abandoned | `mia gc` / `mia gc --apply` |
| `c` | this repository's config, in your editor | `mia config` |

### Moving

| key | moves |
|---|---|
| `j` `k`, arrows | down, up |
| `g` `G` | first, last |
| `ctrl+d` `ctrl+u` | half a page |
| `space` | the next row that needs you |
| `/` | find, by name or branch, into folded stacks; `esc` clears |
| `[` `]`, `1`–`9` | side tabs |
| `K` `J` | scroll the side pane |
| `i` | the side pane, full screen |
| `⇥` | open the row's session in a tmux popup over the list |
| `r` | refresh now |
| `?` | the keys |
| `q` `esc` | back, or quit |

## How actions run

- An action that only changes something (star, new, restack) runs in place:
  a spinner in the footer, then a one-line result, or the full output for
  reports like `Y`.
- An action that lands somewhere (`⏎`, `s`, `+`) leaves the list for that
  session; leaving the session brings you back to a refreshed list.
- Your editor (the config with `c`, an environment shell) opens in a tmux
  popup over the list.

## The other panels

| panel | key | does |
|---|---|---|
| environment | `E` | `u` up, `x` down, `s` a shell inside, `b` browse, `h` move to another machine, `R` remove |
| stack | `S` | each layer bottom first: `⏎` go to it, `n`/`N` add a layer, `W` a worktree for it, `M` merge down, `=` name, `R` restack, `D` forget |
| windows | `t` | the session's windows with a preview: `⏎` land in one, `n` a new one (a shell, or a `[launch]` name), `e` your editor, `x` close |
| machines | `b` | `local` and every registered machine, whether it answers: `⏎` a shell there, `a` add one, `x` forget one |

`mia dash env`, `mia dash stack`, `mia dash blocks` and `mia dash machines`
open a panel directly, optionally for a worktree: `mia dash env monduli`.

## Rebinding keys

Every action, move and section toggle has a name — `?` shows them — and
`[keys]` in the config rebinds it:

```toml
[keys]
machines = "B"             # an action
down = ["j", "down"]       # a move, with every key it should answer to
quiet = "Q"                # the quiet section's toggle
"gh.open" = "O"            # a plugin's key, by <plugin>.<id>
```

In `~/.config/mia/config.toml` it applies to every repository; a
repository's own config wins. `[popup]` sets the size of popups the
dashboard opens (default `80%` × `75%`).

## For editor plugins

`mia dash --pick-to <file>` runs the dashboard for an editor: instead of
landing in a session, it writes what was picked — a worktree's path, or
`edit <file>` — to `<file>` and exits. `mia api dashboard` is the whole panel
as JSON. See [plugins](plugins.md).
