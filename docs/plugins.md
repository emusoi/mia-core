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
{"rows": {"monduli": {"status": "#12 open", "facts": ["pr      #12"], "section": "review", "tabs": {"pr": ["#12 Fix it"]}}}}
```

The answer is cached for `rows.every` (default 10s). Core sections rank
starred 10, in progress 30, quiet 80, not mia's 90; a plugin section moves a
row only if it ranks higher.

## Sides and corners

```json
{"sides":   [{"id": "waiting", "side": "top", "label": "waiting", "rank": 10, "every": "3s"}],
 "corners": [{"id": "agents", "at": "bl", "every": "3s"}]}
```

`side` is `top`, `left`, `right` or `bottom`; `at` is `bl` or `br`. A side's
tabs come from every plugin, ordered by `rank`; the lowest is the default.
`mia-<name> side <id>` and `mia-<name> corner <id>` answer:

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

## Keys and panels

```json
{"keys": [{"id": "open", "key": "O", "label": "open the PR", "verb": "gh-open", "args": ["{row}"]}]}
```

`{row}` is the selected worktree. A key with `"panel": "<id>"` opens a panel
the plugin prints from `mia-<name> panel <id>`, in the same JSON as
`mia api dashboard`.

## Events

`mia-<name> event <event>` runs in the background with the event on stdin:
`worktree.created`, `worktree.removed`, `session.started`, `env.up`,
`env.down`, `stack.changed`.

## Serve

With `"serve": true`, the dashboard runs `mia-<name> serve` and talks JSON-RPC
over stdin and stdout: `rows`, `panel`, `side` and `corner` requests (the
last two with `{"id": "<id>"}`), `event` notifications.
The plugin sends `{"jsonrpc":"2.0","method":"refresh"}` to redraw the
dashboard.
