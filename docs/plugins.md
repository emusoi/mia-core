# Plugins

A plugin is a program named `mia-<name>`, in any language, on your `PATH` or
in `~/.config/mia/plugins`. It does nothing until you enable it:

```bash
mia plugin enable hello
mia plugin ls
```

[mia-plugins](https://github.com/emusoi/mia-plugins) has working ones to
use or copy: agents (`mia agent`: which agent is waiting on you), plans
proven by checks, Neovim, and pull request status.

## The smallest plugin

```sh
#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"verbs":[{"name":"hello"}]}' ;;
hello) shift; echo "hello $* from $MIA_WORKTREE" ;;
esac
```

```bash
mia hello there            # hello there from monduli
```

## The manifest

`mia-<name> manifest` prints JSON:

| field | adds |
|---|---|
| `verbs` | commands: `mia <verb>` runs `mia-<name> <verb>` |
| `rows` | facts, a status and a section for each dashboard row |
| `sections` | dashboard sections, placed by `rank` |
| `tabs` | tabs on the dashboard's side pane |
| `keys` | dashboard keys that run a verb or open a panel |
| `sides` | tabs on a side of the dashboard's room |
| `corners` | a line in a bottom corner of the room |
| `events` | events to hear, or `["*"]` |
| `serve` | a long-running process while the dashboard is open |

Every call gets `MIA_REPO`, `MIA_WORKTREE`, `MIA_PLUGIN_DATA` (a folder of
its own) and `MIA_PLUGIN_CONFIG` (its `[plugin.<name>]` settings as JSON).
Plugins read mia with `mia api <query>`.

## Rows

`mia-<name> rows` gets the worktrees as JSON on stdin and answers:

```json
{"rows": {"monduli": {"title": "Fix the login redirect", "status": "#12 open", "facts": ["pr      #12"], "section": "review", "tabs": {"pr": ["#12 Fix it"]}}}}
```

The answer is cached for `rows.every` (default 10s). Core sections rank
needs you 5, starred 10, in progress 30, resting 80 (id `quiet`), not mia's 90;
a plugin section moves a row only if it ranks higher. `answers` — `[{"label": "Yes", "open": {"verb":
"window", "args": ["press", "monduli", "claude", "--", "1"]}}]` — are the
choices of a question the row waits on: the dashboard numbers them on its
entry, `1`, `2`… picks one, and it goes five seconds later unless `u` takes
it back. A plugin puts a row in
needs you with `"section": "needs"`, without declaring it. `title` names the
row's task; the first plugin to give one wins over the last commit's subject.

## Sides and corners

```json
{"sides":   [{"id": "waiting", "side": "top", "label": "waiting", "rank": 10, "every": "3s"}],
 "corners": [{"id": "agents", "at": "bl", "every": "3s"}]}
```

`side` is `top`, `left`, `right` or `bottom`; `at` is `bl` or `br`. A side's
tabs come from every plugin, ordered by `rank`; the lowest is the default.
`mia-<name> side <id>` and `mia-<name> corner <id>` get the same worktrees on
stdin as `rows` and answer:

```json
{"count": "2", "lines": [[{"text": "●", "width": 3, "style": "yellow"}, {"text": "sinza", "width": 11, "style": "bold"}], []]}
{"segments": [{"text": "$5.76", "style": "green"}, {"text": " today", "style": "dim"}]}
```

A line is a list of cells (`[]` is a blank line); on the right side each
line ends flush with the side's right edge. A cell's `width` defaults
to its text's, `align` is `left` or `right`, and `style` is any of `bold dim
underline strike green yellow red blue cyan magenta white`. `count` `""`
shows none. Answers are cached for `every` (default 10s); an error or a
timeout shows as a dim line in the tab and never holds up the dashboard.

A line can also be an object, so the cursor can stop on it and `⏎` open it:

```json
{"cells": [{"text": "LB-9", "width": 7, "style": "bold"}], "open": {"popup": ["wazo", "--local", "issues", "show", "LB-9"], "title": "issues / LB-9"}}
```

`open` is exactly one of:

| open | ⏎ does |
|---|---|
| `{"popup": [argv], "title": "…"}` | runs argv in the repository, in a 110×16 tmux popup with the title in its border, through `less -R` (`q` closes); `"interactive": true` lets the command own the popup |
| `{"verb": "issue", "args": [...]}` | runs the plugin's verb, as a key does; `"report": true` shows its output |
| `{"lands": "<worktree>[:<window>]"}` | lands in that worktree's session, on that window, as the dashboard's `⏎` does |

Lines without `open` are information; the cursor skips them. Without tmux,
a popup's output shows as a page in the dashboard (`j` `k` read, `q` closes);
an interactive one takes the terminal until it ends.

### Views

The dashboard's home shows each side as a view, `tab` going round them at
full width. A side can say more for that:

```json
{"count": "$22.59", "head": "$22.59 spent today", "note": "$882.63 this week",
 "lines": [
   {"label": "this week"},
   {"chart": [{"label": "mon", "value": 52.1}, {"label": "today", "value": 22.59, "on": true}]},
   {"label": "today by worktree", "cells": [{"text": "1"}]},
   {"cells": [{"text": "lemara", "width": 16}, {"meter": {"value": 22.59, "max": 22.59}, "width": 40}, {"text": "$22.59"}]},
   {"cells": [{"text": "WL-55", "width": 9}, {"text": "A provider emails…", "grow": true}], "open": {"lands": "lemara"}}]}
```

| field | the view draws |
|---|---|
| `head`, `note` | the one thing the view says, bold, where home's entry sits, and a faint line under it |
| `"label"` on a line | a section's heading, in italic, its cells after it as the count |
| `"chart"` on a line | bars six rows high, each `label` under its bar, the one `on` in ink |
| `"grow"` on a cell | the cell takes the width the others leave, a title most often |
| `"meter"` on a cell | a bar `width` wide filled `value` of `max`; `red` in its style for a bad one |
| `"keys"` on a line | `[{"key": "w", "label": "new worktree on it", "open": {…}}]`: keys that act on the line while it is selected, shown under the page; `open` as `⏎`'s |

Every field is optional, and the room's narrow sides flatten them: a label
leads its line, a chart is a row of blocks, a meter a short bar.

## Keys and panels

```json
{"keys": [{"id": "open", "key": "O", "label": "open the PR", "verb": "gh-open", "args": ["{row}"]}]}
```

`{row}` is the selected worktree. A key with `"panel": "<id>"` opens a panel
the plugin prints from `mia-<name> panel <id>`, in the same JSON as
`mia api dashboard`.

A key with `"lands": true, "popup": true` opens its verb in a tmux popup the
dashboard's `[popup]` size; `"popup": {"width": "80", "height": "20", "x": "R",
"y": "S"}` opens it that size instead, with `x` and `y` as tmux
`display-popup -x`/`-y` take them (`R` and `S` is the bottom-right corner).

## The tmux menu

```json
{"menu": [{"key": "y", "label": "type while you wait", "verb": "type", "popup": {"width": "80", "height": "20", "x": "R", "y": "S"}}]}
```

An entry in the `prefix m` menu that `mia help tmux` prints: its one-character
key opens `mia <verb> [args]` in a tmux popup of that width and height
(cells, or a percentage), over the pane you are in. Optional `x` and `y`, given
together, place it as tmux `display-popup -x`/`-y` do; without them it is
centered. An entry whose key is
already a core entry's is left out, and `mia help tmux` says so.

## Events

`mia-<name> event <event>` runs in the background with the event on stdin:
`worktree.created`, `worktree.removed`, `session.started`, `env.up`,
`env.down`, `stack.changed`.

## Serve

With `"serve": true`, the dashboard runs `mia-<name> serve` and talks JSON-RPC
over stdin and stdout: `rows`, `panel`, `side` and `corner` requests (the
last two with `{"id": "<id>", "input": <the worktrees>}`), `event` notifications.
The plugin sends `{"jsonrpc":"2.0","method":"refresh"}` to redraw the
dashboard.
