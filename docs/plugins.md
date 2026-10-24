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
| `verbs` | commands: `mia <verb>` runs `mia-<name> <verb>`; the radar finds them by name |
| `events` | events to hear, or `["*"]` |

Every call gets `MIA_REPO`, `MIA_WORKTREE`, `MIA_PLUGIN_DATA` (a folder of
its own) and `MIA_PLUGIN_CONFIG` (its `[plugin.<name>]` settings as JSON).
Plugins read mia with `mia api <query>`.

## Showing something

A plugin draws nothing in mia. What it wants you to see it says with
[`mia signal`](tmux.md#signals) — a kind, a line of text, a style and a
rank — and mia shows it in the status line and the radar, where everyone's
signals are. What it does on demand is a verb: typing its name in the
radar runs it there and keeps its output up until a key.

## Name collisions

Each verb must be one non-empty word and unique inside a plugin. If two
enabled plugins declare the same verb, `mia <verb>` is refused until all but
one are disabled. Declaring a core command's name is also reported. Check
with `mia plugin ls` or `mia doctor`; neither silently picks an owner.
Dashboard key clashes are separate: they are shown on the dashboard and can
be rebound with `[keys]`.

## Events

`mia-<name> event <event>` runs in the background with the event on stdin:
`worktree.created`, `worktree.removed`, `session.started`, `env.up`,
`env.down`, `stack.changed`, and `poll`: every 30 seconds or so while
tmux shows mia's status line, for a plugin that looks at something outside
— a CI, a review — and says what it saw with `mia signal`. It comes often;
a plugin that asks a service keeps its own pace.
