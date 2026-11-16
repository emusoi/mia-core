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
| `agent` | a coding agent: see [An agent](#an-agent) |

Every call gets `MIA_REPO`, `MIA_WORKTREE`, `MIA_PLUGIN_DATA` (a folder of
its own) and `MIA_PLUGIN_CONFIG` (its `[plugin.<name>]` settings as JSON).
Plugins read mia with `mia api <query>`.

## Showing something

A plugin draws nothing in mia. What it wants you to see it says with
[`mia signal`](tmux.md#signals) — a kind, a line of text, a style and a
rank — and mia shows it in the status line and the radar, where everyone's
signals are. What it does on demand is a verb: typing its name in the
radar runs it there and keeps its output up until a key.

## An agent

mia knows claude, codex and pi. A plugin adds another by declaring it:

```json
{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],"title":"^echo-agent "}}
```

`name` is what its conversations are listed under; the plugin's own when
left out. A window runs the agent when its command line starts with the
name or one of `commands`, or when its pane's title matches `title`, a
regular expression, for an agent whose command line does not say. A name
mia already has is not taken over.

mia then asks the plugin for what it has built in for the others:

| call | prints |
|---|---|
| `mia-<name> agent conversations <main checkout> <worktree>…` | a JSON list, one for each conversation made in those folders: `id`, `cwd`, `asked`, `said`, `started`, `active`, `turns` |
| `mia-<name> agent turns <id> [--after <n>]` | the conversation's turns, one JSON object a line, as [`mia api conversation`](api.md#conversations) gives them |
| `mia-<name> agent search <text> <main checkout> <worktree>…` | a JSON list of the conversations that mention it: `id`, `cwd`, `at`, `yours`, `line`, `n` |
| `mia-<name> agent resume <id>` | the command line that takes the conversation up again |
| `mia-<name> agent carry <from> <to> pack\|command [<id>]` | for `mia move`: `pack` prints a tar of files under the home that hold the conversation of the folder `<from>`, rewritten for `<to>`, and nothing when there is none; `command` prints what to type in the window there |

A worktree that was removed is not among the folders, but it was at
`<main checkout>.<name>`: list its conversations too and mia shows them as
gone. Each call has five seconds. One that fails or prints something else
is named once on standard error, and the other agents' conversations are
listed all the same. `mia move` asks the plugin on the machine the
worktree leaves, so it has to be installed there too.

The smallest one, with two conversations:

```sh
#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],"title":"^echo-agent "}}' ;;
agent)
	case "$2" in
	conversations)
		for folder; do :; done
		printf '[{"id":"e1","cwd":"%s","asked":"say it back","said":"say it back","started":"2026-11-02T10:00:00Z","active":"2026-11-02T10:05:00Z","turns":1},\n' "$folder"
		printf ' {"id":"e2","cwd":"%s","asked":"and again","said":"and again","started":"2026-11-03T10:00:00Z","active":"2026-11-03T10:05:00Z","turns":1}]\n' "$folder" ;;
	turns)
		echo '{"n":1,"kind":"asked","at":"2026-11-02T10:00:00Z","text":"say it back"}'
		echo '{"n":2,"kind":"said","at":"2026-11-02T10:00:01Z","text":"say it back"}' ;;
	search) echo '[]' ;;
	resume) echo "echo-agent $3" ;;
	carry) ;;
	esac ;;
esac
```

```bash
mia api conversations --agent echo
mia conversation resume e2     # a window running `echo-agent e2`
```

## Name collisions

Each verb must be one non-empty word and unique inside a plugin. If two
enabled plugins declare the same verb, `mia <verb>` is refused until all but
one are disabled. Declaring a core command's name is also reported. Check
with `mia plugin ls` or `mia doctor`; neither silently picks an owner.

## Events

`mia-<name> event <event>` runs in the background with the event on stdin:
`worktree.created`, `worktree.removed`, `session.started`, `env.up`,
`env.down`, `stack.changed`, and `poll`: every 30 seconds or so while
tmux shows mia's status line, for a plugin that looks at something outside
— a CI, a review — and says what it saw with `mia signal`. It comes often;
a plugin that asks a service keeps its own pace.
