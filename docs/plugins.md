# Plugins

A plugin is an executable named `mia-<name>`, written in any language, in
`~/.config/mia/plugins` or on your `PATH` (that folder is looked in first).
It does nothing until you enable it:

```bash
mia plugin enable hello
mia plugin ls
```

`mia plugin disable <name>` turns it off again. A plugin can also be
enabled with `plugins = ["hello"]` in the
[configuration](configuration.md); `mia plugin disable` cannot turn that
one off, and says so.

[mia-plugins](https://github.com/emusoi/mia-plugins) has working ones to
use or copy: which agent is waiting on you, plans proven by checks,
Neovim, and pull request status.

## The smallest plugin

```sh
#!/bin/sh
case "$1" in
manifest) echo '{"protocol":1,"verbs":[{"name":"hello","help":"say hello"}]}' ;;
hello) shift; echo "hello $* from $MIA_WORKTREE" ;;
esac
```

```bash
mia hello there            # hello there from monduli
```

## The manifest

`mia-<name> manifest` prints JSON. It has two seconds to answer. mia keeps
what it said until the file changes.

| field | is |
|---|---|
| `protocol` | `1`. A manifest that speaks another protocol is refused |
| `help` | a line `mia plugin ls` shows beside it |
| `verbs` | the commands it adds, each `{"name":…,"usage":…,"help":…}`. `mia <verb>` runs `mia-<name> <verb>` with the words after it. Only `name` is needed, and it is one word |
| `events` | the [events](#events) it wants, or `["*"]` for all |
| `agent` | a coding agent it adds: see [An agent](#an-agent) |

A manifest that does not read, names an unknown event or declares a verb
twice leaves the plugin enabled but unused, and `mia plugin ls` and
`mia api plugins` give the `problem`.

Every call gets these variables:

| variable | is |
|---|---|
| `MIA_PLUGIN` | the plugin's name |
| `MIA_PLUGIN_PROTOCOL` | `1` |
| `MIA_REPO` | the repository's main checkout |
| `MIA_WORKTREE` | the worktree it was run from; not set outside one |
| `MIA_PLUGIN_DATA` | a folder of its own, in the repository's mia folder (`.git/mia/plugins/<name>`) |
| `MIA_PLUGIN_CONFIG` | its `[plugin.<name>]` settings, as JSON; `{}` when there are none |

A plugin reads mia with [`mia api <query>`](api.md).

## Name collisions

Each verb must be unique inside a plugin. If two enabled plugins declare
the same verb, `mia <verb>` is refused until all but one are disabled. A
verb with the name of one of mia's own commands is reported too, and
mia's command wins. `mia plugin ls` and `mia doctor` both list these; mia
never picks an owner silently.

## Showing something

A plugin draws nothing in mia. To show you something it uses
[`mia signal`](tmux.md#signals): a kind, a line of text, a style and a
rank. mia shows it in the status line and [`mia signal ls`](tmux.md#signals),
with everyone else's signals.

What a plugin does on demand is a verb: `mia <verb>`. Name verbs in
`menu = [...]` in the configuration and typing one's name in
[the one list](tmux.md#the-one-list), `prefix k`, runs it in a new
window.

## Events

`mia-<name> event <event>` runs in the background, cut off from the
terminal, with the event as one line of JSON on stdin. What it prints goes
to `events.log` in its data folder.

```json
{"protocol":1,"event":"worktree.created","repo":"/home/user/src/shop","worktree":"monduli","path":"/home/user/src/shop.monduli","branch":"fix-login"}
```

| event | when | `detail` |
|---|---|---|
| `worktree.created` | a worktree is made or taken over: `mia new`, `mia fork`, `mia adopt` | |
| `worktree.removed` | a worktree is removed | |
| `session.started` | a worktree's session starts | |
| `env.up` | its environment comes up | the environment, as [`mia api env`](api.md#env) gives it |
| `env.down` | its environment stops or is removed | `{"removed":false}` or `{"removed":true}` |
| `stack.changed` | a stack changes | `{"change":…}`: the `mia stack` verb that did it (`restack`, `sync`, `rm`, `merge`, `name`, `go`), `layer` for `mia new --stack`, `moved` for `mia up` and `mia down` |
| `poll` | every 30 seconds or so while tmux shows mia's status line | |

`poll` is for a plugin that watches something outside mia, such as CI or a
review, and reports what it saw with `mia signal`. It comes often, so a
plugin that calls a service should keep its own pace. It has no worktree.

## An agent

mia knows claude, codex, pi, opencode, agy and cursor-agent. A plugin adds
another by declaring it:

```json
{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],"title":"^echo-agent "}}
```

`name` is what its conversations are listed under; when it is left out,
the plugin's own name is used. A window runs the agent when its command
line starts with the name or one of `commands`, or when its pane's title
matches `title`, a regular expression, for an agent whose command line does
not say. A plugin cannot take the name of an agent mia already has.

mia then asks the plugin for what it has built in for the others. Each
call has five seconds. One that fails, or prints something else, is named
once on stderr, and the other agents' conversations are listed all the
same.

| call | prints |
|---|---|
| `mia-<name> agent conversations <main checkout> <worktree>…` | a JSON list, one for each conversation made in those folders: `id`, `cwd`, `asked`, `said`, `started`, `active`, `turns` |
| `mia-<name> agent turns <id> [--after <n>]` | the conversation's turns, one JSON object per line, as [`mia api conversation`](api.md#one-conversation) gives them |
| `mia-<name> agent search <text> <main checkout> <worktree>…` | a JSON list of the conversations that mention it: `id`, `cwd`, `at`, `yours`, `line`, `n` |
| `mia-<name> agent resume <id>` | the command line that takes the conversation up again |
| `mia-<name> agent carry <from> <to> pack\|command [<id>]` | for [`mia move`](machines.md#moving-a-worktree), and for resuming a conversation on another account: `pack` prints a tar of the files under the home folder that hold the conversation of the folder `<from>`, rewritten for `<to>`, or nothing when there is none; `command` prints what to type in the window there |

A removed worktree is not among the folders mia passes, but it was at
`<main checkout>.<name>`. List its conversations too, and mia shows them
as gone. `mia move` asks the plugin on the machine the worktree leaves, so
the plugin has to be installed there too.

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

### Its models

An agent can also say which models and efforts it can be started with,
and how. `mia api agents` then lists them, `mia window new … --model`
starts it with one, and `mia window model` switches it:

```json
{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],
  "models":[{"id":"loud","label":"Loud"},{"id":"soft","label":"Soft"}],
  "efforts":["low","high"],
  "with_model":["--model","%s"],"with_effort":["--effort","%s"],
  "switch_model":"/model %s","switch_effort":"/effort %s"}}
```

| field | is |
|---|---|
| `models` | what it can be started with: `id` as the agent takes it, `label` to show |
| `efforts` | its effort levels; none when left out |
| `with_model`, `with_effort` | the words put after the command that runs it, `%s` being the one chosen: `echo-agent --model loud` |
| `switch_model`, `switch_effort` | the line typed into its window, followed by Enter, to switch it while it runs. Without them it cannot be switched, and `mia window model` says to restart it |

Each is optional. An agent with no `with_model` is started with no model.

### Its accounts

An agent that keeps its sign-in in a folder named by a variable can have
[several accounts](agents.md#an-agents-account), each its own folder:

```json
{"protocol":1,"agent":{"name":"echo","commands":["echo-agent"],"account_env":"ECHO_HOME"}}
```

mia then starts it with `ECHO_HOME` set to the folder of the chosen
account, and with nothing set for its own one, `default`. mia does not
know where that one is or whether anyone signed in, so
[`mia api accounts`](api.md#accounts) gives it no `home` and says
`unknown`. Finding its conversations in every account's folder is up to
the plugin, and `mia move` carries them with the plugin's own `carry`.
