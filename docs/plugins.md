# Writing a plugin

A plugin is an executable named `mia-<name>`, in any language, on `PATH` or in
`~/.config/mia/plugins/`. mia runs it only after `mia plugin enable <name>` (or
`plugins = ["<name>"]` in a repository's config). Everything below is protocol
1. A plugin that is missing, slow, broken or speaks another protocol is shown
in `mia plugin ls` with its problem and never stops mia.

The smallest plugin adds one verb:

```sh
#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"verbs":[{"name":"hello"}]}' ;;
hello) shift; echo "hello $* from $MIA_WORKTREE" ;;
esac
```

    mia plugin enable hello
    mia hello there            # hello there from monduli

## The manifest

`mia-<name> manifest` prints one JSON object, within 2 seconds:

| field | what it declares |
|---|---|
| `protocol` | `1` |
| `help` | one line for `mia plugin ls` |
| `verbs` | `[{name, usage, help}]` — `mia <name> …` runs `mia-<name> <name> …` |
| `rows` | `{every: "30s"}` — answer `rows` for the dashboard (default every 10s) |
| `sections` | `[{id, label, rank, collapsed}]` — dashboard sections rows can be placed in |
| `tabs` | `[{id, label}]` — side tabs a row can carry |
| `keys` | `[{id, key, label, help, verb, args, panel, input, confirm, report, lands, popup, global}]` |
| `events` | names to hear, or `["*"]` |
| `serve` | `true` — run `mia-<name> serve` while the dashboard is open |

A built-in verb always wins over a plugin's.

## What a plugin is given

Every call runs with these in the environment:

| variable | |
|---|---|
| `MIA_PLUGIN` | the plugin's name |
| `MIA_PLUGIN_PROTOCOL` | `1` |
| `MIA_PLUGIN_DATA` | a folder of its own, `.git/mia/plugins/<name>/` |
| `MIA_PLUGIN_CONFIG` | its `[plugin.<name>]` settings as JSON, `{}` when there are none |
| `MIA_REPO` | the repository's main checkout |
| `MIA_WORKTREE` | the worktree mia was run in, when it was run in one |

A verb gets the terminal: stdin, stdout and stderr are yours, the arguments
are passed untouched (`--json` and `-h` included), and your exit status is
mia's. Read mia through `mia api <query>`, `mia path <worktree>` and the
other verbs; `mia env run <worktree> <cmd…>` runs a command where the
worktree's code runs.

## Rows

With `rows` in the manifest, mia calls `mia-<name> rows` once per dashboard
refresh, at most every `every`, with every worktree on stdin:

```json
{"protocol": 1, "repo": "/src/shop", "worktrees": [{"name": "monduli", "path": "/src/shop.monduli", "branch": "fix", "dirty": false, "main": false, "adopted": true, "layers": []}]}
```

and reads back, within 2 seconds, what to add to each worktree's row:

```json
{"rows": {"monduli": {"status": "#12 open", "note": "", "facts": ["pr      #12 Fix it"], "section": "review", "tabs": {"pr": ["#12 Fix it", "review  approved"]}}}}
```

- `facts` are added to the row's facts, `label  value` with two spaces.
- `tabs` fill the side tabs the manifest named, in its order; a tab with no
  lines is not shown.
- `section` moves the row into a section the manifest declared, but only when
  that section ranks above where core put it. Core's ranks are *starred* 10,
  *in progress* 30, *quiet* 80, *not mia's* 90; the main checkout and starred
  worktrees never move. Plugins declaring the same id share one section.
- `status` replaces the row's status column when this plugin placed the row;
  otherwise it is added as a fact.

The answer is cached in `MIA_PLUGIN_DATA/rows.json`. If a call fails or is
late, the last answer stays and the problem shows as a fact on the first row.

## Keys and panels

Each key becomes a dashboard action named `<name>.<id>`. It either runs a verb
(`verb` and `args`) or opens a panel (`panel`). In `args`, `{row}` is the
worktree under the cursor, `{input}` what was typed at the `input` prompt, and
`{tab}` the side tab showing. A key is offered on mia's worktrees unless it is
`global`. One that clashes with a built-in key is left unbound, says so, and
can be bound by name: `[keys] "<name>.<id>" = "X"`.

A panel is drawn by `mia-<name> panel <id>`, given `{"protocol", "repo",
"worktree"}` on stdin, answering in the same panel JSON the dashboard uses
(`mia api dashboard` shows its shape): `title`, `sections` of `rows` with
`id` and `cells`, and `actions` whose `verb`s are mia verbs. `mia api
plugin:<name>:<id> [worktree]` returns it too.

## Events

`mia-<name> event <event>` is run without waiting, in a session of its own,
with the event on stdin; its output goes to `MIA_PLUGIN_DATA/events.log`.

```json
{"protocol": 1, "event": "env.up", "repo": "/src/shop", "worktree": "monduli", "path": "/src/shop.monduli", "branch": "fix", "detail": {}}
```

| event | when | `detail` |
|---|---|---|
| `worktree.created` | a worktree is made or adopted | |
| `worktree.removed` | a worktree is removed | |
| `session.started` | its tmux session starts | |
| `env.up` | its environment comes up | the environment |
| `env.down` | its environment stops or is removed | `{"removed": bool}` |
| `stack.changed` | a layer is added, merged, restacked, named, moved to | `{"change": "…"}` |

## Serve

With `"serve": true`, the dashboard keeps `mia-<name> serve` running and
speaks JSON-RPC 2.0 to it, one message per line on stdin and stdout:

- requests `rows` (params: the rows input) and `panel` (params:
  `{"id", "input"}`), answered with the same JSON as the one-shot calls;
- notifications `event` (params: the event);
- from the plugin, the notification `{"jsonrpc": "2.0", "method": "refresh"}`
  redraws the dashboard at once.

A serve that exits is started again after 1s, doubling to a minute. Its
stderr goes to `MIA_PLUGIN_DATA/serve.log`. Outside the dashboard the one-shot
commands still run, so a serving plugin answers both.

## Settings

`[plugin.<name>]` in `.git/mia/config.toml` or `~/.config/mia/config.toml` is
yours: mia does not check it, and hands it over as `MIA_PLUGIN_CONFIG`.

```toml
[plugin.notify]
command = "ping-me"                 # given the title and the message
events = ["env.up", "env.down"]
```
