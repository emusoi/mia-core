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
over stdin and stdout: `rows` and `panel` requests, `event` notifications.
The plugin sends `{"jsonrpc":"2.0","method":"refresh"}` to redraw the
dashboard.
