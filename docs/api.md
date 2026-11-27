# Automation

Scripts, plugins and apps read mia with `mia api <query>`, which always
prints JSON. Every other read command takes `--json` as well. When a
[`mia serve`](serve.md) of the same build runs, `mia api` asks it and prints
what it answers, the same bytes and exit code. A client can also talk to the
server directly and be told when an answer changes.

| query | answers | asked from |
|---|---|---|
| [`capabilities`](#discover-the-interface) | the API version, its queries and its feature markers | anywhere |
| [`worktrees [<worktree>…]`](#worktrees) | the repository's worktrees, their windows and signals | a repository |
| [`machines`](#machines) | this computer and each machine added, and whether it answers | anywhere |
| [`here`](#here) | how busy this computer is, and the machines it knows | anywhere |
| [`repos`](#repos) | every repository mia has named a worktree in | anywhere |
| [`stack [<worktree>]`](#stack-and-base) | the layers of a worktree's stack | a repository |
| [`base`](#stack-and-base) | the repository's base branch | a repository |
| [`status`](#status) | how the worktree a folder is in stands in git | a git folder |
| [`env [<worktree>]`](#env) | a worktree's environment: container, ports, services | a repository |
| [`window <index>`](#window) | whether a window's agent works, and what it asks | a repository |
| [`sessions`, `signals`](#sessions-and-signals) | the sessions mia remembers, the signals that stand | a repository |
| [`conversations`](#conversations) | the agent conversations of the repository | a repository |
| [`conversation <id>`](#one-conversation) | one conversation as turns | a repository |
| [`main`](#the-main-agent) | the main agent, its turns and its conversations | anywhere |
| [`chats`, `chat <name>`](#chats) | every chat, one chat, a chat's turns | anywhere |
| [`standing`](#standing) | who waits on you, who works, what finished unread, on every machine | anywhere |
| [`agents`](#agents) | every agent mia knows, with its models and efforts | anywhere |
| [`setups`](#setups) | the agent, model and effort setups used, and the roles | anywhere |
| [`accounts`](#accounts) | every account of every agent | anywhere |
| [`apps`](#apps) | every app with a manifest | anywhere |
| [`plugins`](#plugins) | the plugins there are, enabled or not | anywhere |

"Anywhere" means any folder. In a repository, `agents`, `plugins` and `apps`
also read that repository's configuration, and `setups` answers for that
repository. `machines` and `here` also answer outside a repository, with no
repository's worktrees in them.

## Discover the interface

```bash
mia api capabilities
```

```json
{
  "version": 1,
  "queries": ["capabilities", "worktrees", "machines", "…"],
  "features": ["direct-http", "project-config", "doctor", "…"]
}
```

`version` goes up only when a field changes meaning or goes away, never
when one is added. `queries` is every query this mia answers. `features`
are markers that newer builds add. Check for a feature before you rely on
it. The sections below name the feature each one depends on. Answers are
not wrapped in a version envelope.

The shape of every answer is kept in `internal/cli/testdata/api`: each
field by name and type, whether it may be left out or null, the values of a
closed set such as a turn's `kind`, and the inputs each query takes. These
are read off the Go types. A test fails, naming the field, when a field is
removed, renamed, changes type or may newly be left out, or when a query
needs an input it did not need before. A field or value that is added
passes. Record it with
`go test ./internal/cli -run TestTheAPIAnAppWasBuiltOnStillAnswersTheSame -update`.

## Exit status

| code | means |
|---|---|
| `0` | it worked |
| `1` | it failed: the reason is on stderr, as `mia: …` |
| `3` | not found: no such worktree, or no such query |
| `64` | usage: the query was typed wrong; stderr gives its usage |

A query that fails prints no JSON. A command that runs another command
passes on that command's exit status where it can: `mia run`, `mia env run`,
`mia env exec` and interactive shells.

## Worktrees

`mia api worktrees` is every worktree of the repository, its main checkout
included. Name worktrees after it to get only those.

```json
[{"name":"monduli","path":"/home/user/src/shop.monduli","branch":"fix-login","dirty":true,
  "session":true,"adopted":true,"where":"local","seen":"2026-11-21T17:40:02-05:00",
  "ahead":2,"files":3,"added":41,"deleted":7,
  "last_work":"2026-11-21T18:00:26-05:00","last_subject":"The login keeps the session",
  "windows":[{"index":1,"name":"claude","command":"claude","quiet":"2s","active":true,
              "kind":"agent","agent":"Fixing the login test","working":true},
             {"index":2,"name":"notes","command":"notes","quiet":"3m10s","active":false,"app":"notes"}],
  "signals":[{"worktree":"monduli","kind":"busy","rank":60,"from":"claude",
              "at":"2026-11-21T18:00:20-05:00","pane":"%12","pid":48211}]}]
```

| field | is |
|---|---|
| `main` | true of the main checkout; left out otherwise |
| `adopted` | mia keeps the worktree: made with `mia new` or taken over with `mia adopt`. False for one made by hand and for a main checkout never adopted |
| `session` | its tmux session runs |
| `where` | where its environment runs, as the mia that keeps it says it. `local` is that mia's own computer, so beside `"machine":"fedora"` it means fedora |
| `machine`, `unreachable` | the machine whose mia keeps the worktree, left out for this one. `unreachable` is set when that machine did not answer and this is its last answer |
| `starred`, `seen` | you starred it; when you last switched to it |
| `ahead`, `behind`, `files`, `added`, `deleted` | its branch against the base: commits, and the files and lines it changes |
| `last_work`, `last_subject` | its last commit's time and subject |
| `layers`, `stack` | of a branch in a [stack](stacks.md): its layers, as [`stack`](#stack-and-base) gives them, and the stack's name |
| `dev` | when [`mia dev`](environments.md#mains-dev-server) lends the main checkout's dev server: `has <worktree>'s files` on the main checkout, `on main's dev server` on the worktree |
| `windows` | its session's windows, when it runs |
| `signals` | what anything outside mia last said about it, best first: see [Signals](tmux.md#signals) |

Each window has `index`, `name`, `command`, whether it is `active`, and
`quiet`, how long since it last wrote (`3s`, `1m12s`). The rest are left
out when they do not apply:

| field | is |
|---|---|
| `kind` | what its first pane is for: `agent`, `shell`, `editor`, `server`, `tests` or `logs`. mia marks every pane it starts with the tmux pane option `@mia_kind`. A window opened by hand has none. See [Pane kinds](tmux.md#pane-kinds) |
| `agent`, `working` | the title an agent gives its pane, and whether its spinner says it is at work now |
| `app` | the [app](apps.md) a manifest says it runs. When only the pane's title named the app, `command` is the app's name too. An agent's window never has one |
| `pid`, `panes` | its process, and how many panes it is split into |

## Machines

`mia api machines` is this computer, as `local`, then every machine
`mia machine add` added, all asked at once. A machine linked to this
repository also reports its worktrees, in the same shape as
[`worktrees`](#worktrees), and its branches are fetched. One that does not
answer keeps the worktrees it last reported, each marked `unreachable`.

```json
[{"name":"local","ssh":"","engine":"","answers":true,"reach":{"state":"ok"},
  "health":{"load":[1.2,1.4,1.5],"cores":10,"mem_used":11.8,"mem_total":32,
            "disk_used":402.1,"disk_total":926.4,"ports":{"/home/user/src/shop":[5173]}}},
 {"name":"fedora","ssh":"user@fedora","engine":"podman","answers":false,
  "detail":"fedora: ssh: connect to host fedora port 22: Operation timed out",
  "reach":{"state":"unreachable","detail":"fedora cannot be reached over ssh: …","fix":"turn fedora on, or check its address with ssh user@fedora"},
  "worktrees":[{"name":"goba","path":"/home/user/src/shop.goba","dirty":false,"session":true,"adopted":true,"unreachable":true}]}]
```

`health` is how busy it is: load averages, cores, memory and the disk the
repository is on in GB, and the TCP ports listened on, by the folder of
the process that listens. It is left out when the machine did not answer.

`reach` says what not answering means, with the one step that fixes it.
The same `reach` is in [`standing`](#standing) and in
`mia machine ls --json`. Feature `machine-reach`.

| `state` | is | `fix` |
|---|---|---|
| `ok` | it answered | |
| `unreachable` | ssh could not connect: it is off, its name does not resolve, or it timed out | turn it on, or check its address |
| `needs_login` | ssh got there and was not let in: a key refused, a host key not accepted, a password wanted. mia never asks for one | `ssh <target>` once in a terminal |
| `no_mia` | it answers and has no mia at `~/.local/bin/mia` | `mia machine ship <name>` |
| `old_mia` | its mia does not know the query; `detail` says its version | `mia machine ship <name>` |
| `not_set_up` | its mia runs and this repository is not there. It has `answers` true | `mia machine link <name>` |
| `failed` | its mia answered with another error; `detail` has its line | |

`reach.warnings` are things that work now and will stop working, each with
a `name`, `detail` and `fix`. The one there is today is `linger`: a Linux
machine whose user services stop when you log out (`loginctl show-user`
says `Linger=no`). Each machine's own mia checks this at most every ten
minutes.

A machine that does not answer within 8 seconds is not waited for, and is
not asked again for 30 seconds by any mia. One that is `unreachable` or
`needs_login` twice running is asked once a minute until it answers.

## Here

`mia api here` is this computer alone, for a client that asks each machine
itself: its `health`, as above, and the names of the `machines` it can move
worktrees to. Nothing is asked over ssh.

```json
{"health":{"load":[1.2,1.4,1.5],"cores":10,"mem_used":11.8,"mem_total":32,"disk_used":402.1,"disk_total":926.4},
 "machines":["fedora"]}
```

## Repos

`mia api repos` is every repository mia has named a worktree in, by the
name of its main checkout's folder. One gone from disk, a bare one and one
in a temporary folder are left out. `mia ls --json` prints the same.

```json
[{"name":"mia-core","path":"/home/user/src/mia-core"},{"name":"shop","path":"/home/user/src/shop"}]
```

## Stack and base

`mia api stack [<worktree>]` is the layers of the stack the worktree's
branch is in, bottom first, the same answer as `mia stack show --json`.
With no worktree it is the one the folder is in. A branch in no stack, or a
detached `HEAD`, gives `[]`.

```json
[{"branch":"api","parent":"main","landed":false,"ahead":2,"needs_restack":false,"current":true,"worktree":"monduli"},
 {"branch":"ui","parent":"api","landed":false,"ahead":1,"needs_restack":true,"current":false}]
```

`current` is the layer the worktree is on. `base_moved` is set on the
bottom layer when the base moved on under it. `worktree` is where the layer
is checked out, and is left out when it is checked out nowhere.

`mia api base` is the repository's base branch: `{"branch":"main"}`.

## Status

`mia api status`, typed in any folder of a git repository, is how that
worktree stands in git. It needs no repository of mia's.

```json
{"branch":"feature","head":"5d1c0a9e…","upstream":"origin/feature","gone":false,"to_push":1,"to_pull":0,
 "staged":[],"changed":[{"path":"main.go","size":2048,"written":"2026-11-21T18:00:26-05:00"}],"untracked":[],"stashes":0}
```

| field | is |
|---|---|
| `branch`, `head` | the branch, `""` when detached, and the commit it is at |
| `upstream`, `gone` | what it follows, and whether that is no longer there |
| `to_push`, `to_pull` | commits ahead of and behind the upstream |
| `staged`, `changed`, `untracked` | its files, each with `path`; `from` when renamed; `state`, one of `new`, `deleted`, `renamed`, `conflict`, left out for a plain change; and `size` and `written`, when the file was last written, both left out of a file that is not there |
| `stashes` | how many stashes the repository keeps |

## Env

`mia api env [<worktree>]` is the [environment](environments.md) a
worktree's code runs in.

```json
{"worktree":"/home/user/src/shop.monduli","name":"monduli","container":"mia-monduli",
 "image":{"ref":"mcr.microsoft.com/devcontainers/base:ubuntu","source":"default base"},
 "state":"running","listening":[5173],"placement":{},"machine":"local",
 "ports":{"listening":[5173],"declared":[5173],"page":5173},
 "services":[{"id":"web","run":["npm","run","dev"],"running":true,"healthy":true}]}
```

| field | is |
|---|---|
| `worktree`, `name` | the worktree's path and its name |
| `image.source` | where the image came from: `[env] image`, `devcontainer.json`, `detected stack`, `default base`, `compose file`, or `` built by `mia env image` `` |
| `state` | what the container engine says, such as `running` or `exited`; `""` when there is no container |
| `placement`, `machine` | where it runs: `{}` here, else `{"runtime":"<machine>","staging":"<folder there>"}`; and that machine's name, `local` for this one |
| `mirror` | of one on another machine, how the file sync stands |
| `ports` | the ports listened on, those the config declares, and the `page` to open |
| `services` | each service, whether it runs, and `healthy` when it has a check |
| `error`, `portsError`, `servicesError` | what could not be read, when something could not |

## Window

`mia api window <index> [--worktree <name>]` is one window of the worktree
the folder is in, or of the one named: whether its agent is `working`, and
what its screen `asks` when it waits on you, with the `options` and the one
`selected`, the first being 1.

```json
{"working":false,"asks":{"text":"Do you want to make this edit?","options":["Yes","No"],"selected":1}}
```

`working` comes from the spinner an agent puts in its pane's title, else
from the worktree's [standing](#standing), as a `busy` signal says it.
agy's pane has no such title, so for agy's window it comes from its
conversation: at work until the last step it wrote is a reply that calls
no tool.

No file says when a window or a worktree's files change, so
[`GET /events`](serve.md#get-events) asks `window` and
[`status`](#status) again every second for whoever listens.

## Sessions and signals

`mia api sessions`, `mia api signals` and `mia api plugins` print the same
bytes as `mia resurrect --list --json`, `mia signal ls --json` and
`mia plugin ls --json`. A client can ask them of [mia serve](serve.md)
instead of starting those commands.

`sessions` is every session the repository remembers, the last seen first.
`state` is `running`, `down` (it went down with tmux) or `closed` (it was
closed while tmux ran). Each window is given as `mia resurrect` opens it
again, in Go's own field names: `Name`, `Dir`, `Command`, and where they
apply `Conversation`, `ID`, `Account`, `Kind`, `Label`, `Splits`, `Layout`
and `Auto`.

```json
[{"name":"monduli","state":"down","seen":"2026-11-21T18:00:26-05:00",
  "windows":[{"Name":"claude","Dir":"","Command":"claude","Conversation":"23f98c02-…","Kind":"agent"},
             {"Name":"","Dir":"web","Command":"","Auto":true}]}]
```

`signals` is every signal of the repository's worktrees whose setter is
still there, best first. Signals whose setter has gone are dropped.

```json
[{"worktree":"goba","kind":"asks","text":"wants to use Bash","rank":10,"from":"claude",
  "at":"2026-11-21T18:00:26-05:00","pane":"%7","pid":51234,"started":"2026-11-21T17:12:00-05:00","yes":"1"}]
```

`until` is when it goes by itself, `until_seen` that it goes when the
worktree is next switched to, and `yes` the keys that approve it, pressed
with no Enter. See [Signals](tmux.md#signals).

## Plugins

`mia api plugins` is every plugin there is, enabled or not, by name.

```json
[{"name":"hello","path":"/home/user/.config/mia/plugins/mia-hello","enabled":true,
  "manifest":{"protocol":1,"verbs":[{"name":"hello"}]}},
 {"name":"pr","path":"/home/user/.local/bin/mia-pr","enabled":false,"manifest":{"protocol":0}}]
```

A manifest is read only from an enabled plugin. `by_config` is set when
`plugins = [...]` in the config enables it. `problem` says why an enabled
plugin cannot be used: it is not installed, or its manifest did not read.
See [Plugins](plugins.md#the-manifest).

## Conversations

`mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]`
is the agent conversations of the repository, from its main checkout and
every worktree, removed ones too, the last written to first. mia reads the
agents' own transcripts, so nothing is recorded while they work. opencode's
come from opencode itself (`opencode api GET /api/session`,
`opencode session export`), and only when it is installed. A conversation
is read again only once it has changed. Feature `conversations`.

```json
[{"id":"23f98c02-c52b-4de9-8c69-b322eb153bff","agent":"claude","account":"default",
  "cwd":"/home/user/src/shop.monduli","worktree":"monduli","gone":false,
  "asked":"make the list faster","said":"It is faster now: one read for each file.",
  "started":"2026-11-02T10:00:00Z","active":"2026-11-02T10:41:07Z","turns":4,
  "running":true,"window":{"index":2,"name":"claude"},"model":"opus","effort":"high"}]
```

| field | is |
|---|---|
| `agent` | `claude`, `codex`, `pi`, `opencode`, `agy`, or an agent a plugin adds |
| `account` | the [account](#accounts) whose folder it is in, `default` for the agent's own. For a plugin's agent, the account its running window was started on. Left out for an agent mia keeps no accounts of |
| `worktree`, `gone` | the worktree `cwd` is in, `""` when it is none of this repository's; and whether that folder is gone |
| `asked`, `said` | the first thing you asked and the last thing the agent said, each on one line, cut at 200 characters |
| `started`, `active` | when it began and when it was last written to, in UTC |
| `turns` | how many things you asked |
| `running`, `window` | a window of the worktree's session has it open now, and which. `window` is left out otherwise |
| `model`, `effort` | of a running one, what mia started its window with or last switched it to. mia keeps these per window and does not read them from the agent, so a switch made by hand inside the agent is not seen. For opencode, `model` is otherwise the `provider/model` opencode says the conversation runs on |
| `continues`, `continued_by` | `{"agent":…,"id":…}`: the conversation this one was [handed over](conversations.md#with-another-agent) from, and the one that went on from it. Left out when neither applies. Feature `conversation-thread` |

A conversation taken up in another worktree, or on another account, was
copied there, so it is listed once for each folder it lives in: the same
`id`, told apart by `worktree`, `cwd` and `account`.

Which conversation a window has open, claude says itself. For any other
agent it is the newest of that agent's in the worktree, on the account the
window was started on. A conversation the agent has open but has not
written to yet is listed all the same: `running`, with its `window`, and
no `asked`, `said` or `turns`.

When `mia conversation continue` hands a conversation to another agent,
the two are one thread. The first gets `continued_by`, and the second gets
`continues`. The second is known only once that agent has written its first
turn. Until then neither field is there. Handed on more than once,
`continued_by` is the last one that began.

### One conversation

`mia api conversation <id> [--worktree <name>] [--after <n>] [--follow]` is
one conversation as turns, in order. Each turn has a number `n` that only
grows. `--after <n>` gives only the turns after `n`, so a view that polls
asks for what came after the last turn it has. For a conversation that is
in several worktrees, `--worktree` picks the copy. With none, it is the
one last written to.

```json
[{"n":1,"kind":"asked","at":"2026-11-02T10:00:00Z","text":"make the list faster"},
 {"n":2,"kind":"thought","at":"2026-11-02T10:00:02Z","text":"The sort runs on every keypress.","seconds":2},
 {"n":3,"kind":"said","at":"2026-11-02T10:00:04Z","text":"I will **look** at where the time goes."},
 {"n":4,"kind":"tool","at":"2026-11-02T10:00:05Z","text":"Edited view.lua +12 −3","name":"Edit","file":"/home/user/src/shop.monduli/view.lua","status":"ok"},
 {"n":5,"kind":"question","at":"2026-11-02T10:00:09Z","text":"Which one?","options":["Fast","Small"],"answer":"Fast"},
 {"n":6,"kind":"answer","at":"2026-11-02T10:00:30Z","text":"Fast","question":5},
 {"n":7,"kind":"commit","at":"2026-11-02T10:01:00Z","hash":"abc1234","subject":"The list is faster"}]
```

| kind | is |
|---|---|
| `asked` | what you said: `text` |
| `said` | what the agent said: `text`, as markdown |
| `thought` | what the agent thought, where its transcript keeps it: `text`, and `seconds` where the transcript says how long. Redacted or encrypted thinking is never given |
| `tool` | what the agent did, in a line (`Ran go test ./...`, `Read 3 files`), with the tool's `name`, `file` when it is about one file, and `status`, `ok` or `error`, once the transcript says how it went. `status` is left out while the tool runs and where the transcript does not say |
| `question` | the agent asked you to choose: `text`, `options`, and `answer` once you gave it |
| `answer` | what you chose: `text`, and `question`, the `n` of the turn it answers |
| `commit` | the agent made a commit: `hash` and `subject` |

What the tools gave back and what the system told the agent are left out.
The main agent's turns and a chat's also have [`did`](#what-the-main-agent-did)
and [`switch`](#another-agent-mid-conversation) turns.

`--follow` is for a view that shows the conversation live instead of
polling. It prints the turns as one line of JSON, then keeps running: each
time there are new turns it prints one more line with only those, and
nothing while nothing changes. A turn that grew, such as a reply opencode
is still writing or a tool whose `status` came in, comes again under the
same `n`, so replace turns by `n`. It ends with exit 0 when stdin ends,
when its reader goes away, or on SIGINT or SIGTERM. Feature
`conversation-follow`.

### Conversations from other commands

`mia recall <text…> --json` is the conversations that mention the text, up
to 10, newest first. In a repository it searches that repository. With
`--all`, or outside a repository, it searches every project on every
machine. Each result has the conversation's `session` (its id), `agent`,
`cwd`, `worktree`, `account`, `at`, and `n`, the turn to open it at (0 for
words a subagent wrote), `yours` (you said them), `line` (the words around
them), `project`, `root`, `resume` (the command that goes on with it),
`machine` when it is not this one, and `gone` when its folder is no more.

`mia why <file:line|commit> --json` is the claude conversations that made
or first saw a commit, in Go's field names: `Commit`, `Subject` and
`Conversations`, each with `session`, `cwd`, `file`, `at`, `asked`, `made`
and `account`.

`mia conversation resume <id> --json` says what it opened: the `worktree`,
the `window`, the `command`, `already` when a window had it open so nothing
was opened, `copied` when it was copied in from another folder or account
first, and the `model`, `effort` and `account` it runs on. Resumed with
`--model` or `--effort` while a window has it open, it is refused.

`mia conversation continue <id> --with <agent> [--worktree <name>]
[--dry-run] --json` hands a conversation to another agent, as
[Conversations](conversations.md#with-another-agent) describes. `--from <window>` names the
conversation by the window that has it open instead. `mia hand --json`
answers the same. Feature `conversation-thread`.

```json
{"from":{"agent":"claude","id":"23f98c02-c52b-4de9-8c69-b322eb153bff","worktree":"monduli","window":{"index":2,"name":"claude"}},
 "to":{"agent":"codex","window":{"index":3,"name":"codex"}},
 "handoff":"/home/user/src/shop/.git/mia/handoffs/monduli/claude-23f98c02-….md",
 "full":"/home/user/src/shop/.git/mia/handoffs/monduli/claude-23f98c02-…-full.md",
 "turns":212,"left_out":158,"command":"codex",
 "instruction":"Read /home/user/src/shop/.git/mia/handoffs/monduli/claude-23f98c02-….md — …",
 "sent":true,"account":"default","dry_run":false}
```

| field | is |
|---|---|
| `from` | the conversation, its worktree, and the `window` that has it open, if any |
| `to` | the agent it goes on with and the `window` opened for it; `worktree` when that is another worktree. `id` is the agent's own conversation, which is known only later, through `continued_by` |
| `resumes` | when the conversation had gone on from this agent's own, that one is taken up again, and the handoff holds only what was said since |
| `handoff`, `turns`, `left_out`, `full` | the file the conversation was written to, of `turns` turns. Over the budget, `left_out` turns of its middle are not in it and `full` has them all |
| `upto` | for a fork, the turn it was made from |
| `instruction`, `sent`, `problem` | the agent's first message, and whether mia typed it. An agent that asked something first, or ended, was typed nothing, and `problem` says which. That is still exit 0 |
| `dry_run` | nothing was written or opened. `to.window` has the name it would have and no `index` |

When it is refused (no such conversation, a window with none or several
open, the agent it is already with), it prints no JSON, says why on stderr
and exits 1.

## The main agent

`mia api main` is where the [main agent](main.md) stands. It starts and
writes nothing. Feature `main-agent`.

```json
{"name":"main","running":true,"agent":"claude","machine":"local",
 "home":"/home/user/.config/mia/main","session":"mia-main",
 "window":{"index":1,"name":"claude"},
 "conversation":"23f98c02-c52b-4de9-8c69-b322eb153bff",
 "working":true,"since":"2026-11-02T10:00:00Z","model":"opus","effort":"high","account":"default"}
```

| field | is |
|---|---|
| `agent` | the one that runs; when none runs, the one `[main] agent` names. `mia main start --agent` can start another (feature `main-start-agent`) |
| `machine` | `local`, or the machine `[main] machine` names |
| `home`, `session` | its folder and its tmux session, whether or not they exist yet |
| `window`, `since` | its window and when its session started; left out when it is not running |
| `conversation` | the one it has open (claude says which), or for another agent the last one written to in its folder. `""` when there is none. After `mia main start --new` (feature `main-start-new`) it is `""` until the new conversation's first turn is written. An id that changed means the conversation changed |
| `working` | it is at work now: claude's pane title says so, and for another agent, its window wrote in the last three seconds |
| `asks` | the question its window shows: `text`, `options`, and `selected`, the option Enter alone would answer, the first being 1. Left out when it asks nothing. `working` is false while it asks. Feature `main-asks` |
| `model`, `effort`, `account` | what mia started it with or last switched it to, and the account it runs on; left out when it is not running or mia chose none |

While it asks, `mia main say` and `mia main model` type nothing and exit 1
with a line that says what it asks. `mia main press <n>` answers it.
`mia main status --json` and `mia main start --json` print this same
state.

When `[main] machine` names another machine, `mia api main`, its
`conversations` and its `turns` are that machine's answers, asked over
ssh. A machine that does not answer is an error naming it, exit 1, and is
not asked again for 30 seconds.

### Its turns and conversations

`mia api main turns [--conversation <id>] [--after <n>] [--follow]` is the
conversation it has open, as [`conversation`](#one-conversation) gives
one; `[]` when there is none. `--follow` follows the one open when it
starts. It reads a main agent on this machine only, and is refused, exit 1,
when `[main] machine` names another. `--conversation` reads one of its
[earlier conversations](main.md#earlier-conversations) instead.

`mia api main conversations` lists its conversations, as
[`conversations`](#conversations) does: every one made in its folder,
with every agent it ran as, the last written to first. `running` is true
of the one it has open now. `worktree` is its folder's name. An id that is
none of these is refused, exit 1. `mia main start --resume <id>` starts it
going on with one. Feature `main-conversations`.

### What the main agent did

A `did` turn is a shell command the main agent ran that is one of mia's
verbs and changes something. Nothing is logged for it: it is the `tool`
turn of the same `n`, read again. `mia api conversation` on the same
conversation gives the plain `tool` turn.

```json
[{"n":7,"kind":"asked","at":"2026-11-02T10:00:00Z","text":"start goba on the login bug"},
 {"n":8,"kind":"did","at":"2026-11-02T10:00:04Z","text":"Started goba on shop · claude","name":"Bash",
  "verb":"window new","repository":"shop","worktree":"goba"},
 {"n":9,"kind":"did","at":"2026-11-02T10:02:11Z","text":"Moved sinza to fedora","name":"Bash",
  "verb":"move","repository":"shop","worktree":"sinza","machine":"fedora"}]
```

`verb` is the verb as typed. `repository` is the one the command ran in,
by its main checkout's folder name. `worktree` and `machine` are there
when the command names them, and `worktree` also when the command was
typed inside it.

| `verb` | `text` |
|---|---|
| `new` | `Made a worktree for fix-login on shop`, with `, on fedora` after `--on`; `worktree` only when `--name` gave it |
| `agent run`, `window new` | `Started goba on shop · claude`; a window that runs no agent stays a `tool` |
| `agent send`, `window send` | `Told kijenge: rebase onto main` |
| the same, or `window press`, with a number | `Answered kijenge: 1` |
| `agent stop` | `Stopped the agent in goba` |
| `move` | `Moved sinza to fedora`, or `Moved sinza back here` |
| `rm`, `archive` | `Removed goba on shop`, `Archived goba on shop` |

Only `mia …` alone, or `cd <folder> && mia …`, is read, split as a shell
splits it. A command with a pipe, a redirection, a variable, a
substitution or a third command stays a `tool`, since what it ran may not
be what its words say. So does a verb that only reads (`mia api …`,
`mia ls`, `mia window read`), one with `--dry-run`, and one typed in a way
its usage does not allow.

### Another agent mid-conversation

After [`mia main switch`](main.md#another-agent-mid-conversation), its
turns are those of each conversation it went on from, each followed by a
`switch` turn, then those of the one it has open, numbered on from them.
`--after` and `--follow` count them the same way. Feature `chat-switch`.

```json
[{"n":11,"kind":"said","at":"2026-11-02T10:05:00Z","text":"goba is on it"},
 {"n":12,"kind":"switch","at":"2026-11-02T10:06:00Z","from":"claude","to":"codex","note":"context handed over · 11 turns"},
 {"n":13,"kind":"asked","at":"2026-11-02T10:06:30Z","text":"and the tests?"}]
```

`note` says what was handed over: `context handed over`, or
`took its own conversation up again` when it went back to an agent it had
gone on from, with how many turns and how many were left out. Until the
first thing is said to the new agent, the last switch has `"tone":"warn"`
and the note `context goes with your next message`. Your own words are
given without the handed-over conversation in front of them.

## Chats

`mia api chats` is every [chat](chats.md): the main agent first, as the
chat named `main`, then the others, the last said in first. It starts and
writes nothing. Feature `chats`.

```json
[{"name":"main","main":true,"agent":"claude","model":"opus","effort":"high",
  "title":"mia","last":"goba is on it","at":"2026-11-02T10:00:04Z",
  "asks":false,"working":false,"running":true,"machine":"local"},
 {"name":"kilwa","main":false,"agent":"codex","model":"","effort":"",
  "title":"what changed in shop this week?","last":"Three things…",
  "at":"2026-11-01T18:20:00Z","asks":false,"working":false,"running":false,"machine":"local"}]
```

| field | is |
|---|---|
| `name` | what `mia chats …` and `mia api chat` take; `main` for the main agent |
| `agent`, `model`, `effort` | the agent it goes on with, and the model and effort it runs on, or is started with when it is not running; `""` when mia chose none |
| `title` | the first thing asked in it, on one line; `mia` for the main agent |
| `last`, `at` | the last thing said in its conversation and when; for one with nothing yet, `""` and when it was made |
| `asks`, `working`, `running` | as [`main`](#the-main-agent) says them |
| `machine` | `local`, or the machine `[main] machine` names |

`mia api chat <name>` is where one chat stands, in the shape of
[`main`](#the-main-agent); `mia api chat main` is `mia api main`. A name
that is no chat's is refused, exit 1.

`mia api chat turns <name> [--after <n>] [--follow]` is its conversation,
as [`main turns`](#its-turns-and-conversations) gives it, `did` and
`switch` turns included. `--follow` reads chats on this machine only, and
follows on across a switch.

## Agents

`mia api agents` is every agent mia knows, plugins' agents included, with
what each can be started with. Feature `agent-models`.

```json
[{"name":"claude",
  "models":[{"id":"opus","label":"Opus"},{"id":"sonnet","label":"Sonnet"},{"id":"haiku","label":"Haiku"},{"id":"fable","label":"Fable"}],
  "efforts":["low","medium","high","xhigh","max"],
  "model":"opus","effort":"","live":true,"again":false,"account_env":"CLAUDE_CONFIG_DIR","installed":true},
 {"name":"opencode","models":[{"id":"provider/model","label":"model"}],
  "efforts":[],"model":"","effort":"","live":false,"again":true,"account_env":"","installed":true},
 {"name":"cursor-agent","models":[],"efforts":[],"model":"","effort":"","live":false,"again":false,"account_env":"","installed":false}]
```

| field | is |
|---|---|
| `models` | what it can be started with: `id` as the agent takes it, `label` to show. A model the config names is listed too. Empty for an agent mia knows no models of |
| `efforts` | its effort levels; empty when mia knows none |
| `model`, `effort` | what `[agents.<name>]` starts it with; `""` for the agent's own default |
| `live` | it can be switched to another model while it runs, by what mia types into it |
| `again` | it takes a model only as it starts, so `mia window model` starts it again in its window, on the conversation it has open. Feature `agent-again` |
| `account_env` | the variable that names the folder of each of its [accounts](#accounts); `""` for an agent mia keeps no accounts of |
| `installed` | its program is on this machine's `PATH` |

opencode's and agy's models are those their own accounts can use, so mia
asks them (`opencode models`, `agy models`) and keeps the answer for an
hour, in `~/.config/mia/cache/models/`. When one cannot say (not
installed, not signed in, no answer in 5 seconds) its `models` are empty,
and that is kept for a minute. opencode is started with a model as
`opencode mini --model <provider/model>`, has no efforts, and is switched
by being started again on its conversation; one with no conversation yet
cannot be switched. agy is started as `agy --model <id> --effort <level>`
and switched the same way.

Start one with `mia window new <worktree> <agent> --model <id> --effort
<level>`, and switch one that runs with `mia window model <worktree>
<window> <id> [--effort <level>]`. Take a conversation up again with a
model using `mia conversation resume <id> --model <id> --effort <level>`
(feature `resume-model`). For the main agent, use `mia main start --model …`
and `mia main model …`. A model or effort not in the agent's list is
refused, and the refusal shows the list.

## Apps

`mia api apps` is every app with a [manifest](apps.md), by name, read as
the manifests are now. Feature `apps`. An older mia has neither this query
nor a window's `app`.

```json
[{"name":"lazygit","label":"Git","icon":"/home/user/.config/mia/apps/lazygit/icon.svg",
  "match":{"commands":["lazygit"],"via":[]},"keys":[{"key":"c","label":"commit"}]},
 {"name":"notes","match":{"commands":[],"title":"^notes","via":["node","python3"]},"keys":[]}]
```

`label` is left out when the manifest has none. `icon` is the file's full
path, and is left out when the manifest names none or the file is not
there. `match.title` is left out for an app told by its command alone.
`commands`, `via` and `keys` are always lists. A manifest that does not
read, or is named as an agent, is left out and named in a line on stderr.
The others are listed all the same, and the exit status is 0.

## Setups

`mia api setups [--repository <name> | --all]` is the setups you have
used, each an agent, a model and an effort taken together, with how often
and where they were used last, so a picker can offer the ones you use. In a
repository it answers for that repository. Outside one, or with `--all`,
it answers for every repository. `--repository` names one by its main
checkout's folder name. Feature `setups`.

```json
[{"agent":"claude","model":"opus","effort":"high","uses":7,"sessions":6,
  "last":{"repository":"shop","worktree":"goba","machine":"local","at":"2026-11-15T10:00:00Z"},"usual":true},
 {"agent":"claude","model":"sonnet","effort":"medium","uses":2,"sessions":2,
  "last":{"repository":"shop","worktree":"themi","machine":"fedora","at":"2026-11-12T16:20:00Z"},"usual":false},
 {"agent":"codex","model":"","effort":"low","uses":0,"sessions":0,"usual":false},
 {"agent":"claude","model":"opus","effort":"max","uses":0,"sessions":0,"usual":false,
  "role":"reviewer","body":"Review the branch against main…"}]
```

| field | is |
|---|---|
| `agent`, `model`, `effort` | the setup; `""` for a model or effort mia did not choose, never both |
| `uses`, `sessions` | how many times an agent was started with it or switched to it, and in how many worktrees |
| `last` | where it was used last: `repository`, `worktree`, `machine` (`local` for this one) and `at`; left out of one never used |
| `usual` | the one used in the most of the repository's worktrees, the later one of a tie; none when every repository is asked |
| `role`, `body`, `account` | of a [role](agents.md#a-role): its name, the first message its agent is sent, and the account it starts on when it names one. Feature `roles` |

The usual setup comes first, then the rest, the last used first. After
those come the setups the config starts agents with, `[main]` and
`[agents.<name>]`, that were not used, with `uses` 0, so someone who has
chosen nothing yet still has those. One that names nothing, or names
something mia cannot start the agent with, is left out. Last come the
roles, by name, each as `mia window new <worktree> --role <role>` would
start it. A role file that does not read is left out; `mia role ls` says
why.

Every `mia window new … --model`, `mia window model`,
`mia conversation resume`, `mia main start` and `mia main model` records a
use in `setups.json` in the config folder of the machine whose mia started
it. The last 500 are kept. The main agent's uses are under the repository
`main`, worktree `main`.

## Accounts

`mia api accounts` is every [account](agents.md#an-agents-account) of every
agent mia keeps accounts of, by agent, each agent's own first. It starts
and writes nothing. Feature `accounts`.

```json
[{"agent":"claude","name":"default","home":"/home/user/.claude","default":true,
  "logged_in":"unknown","sessions":["main","goba"],"user_default":false,"shared":[]},
 {"agent":"claude","name":"work","home":"/home/user/.config/mia/accounts/claude/work","default":false,
  "logged_in":"unknown","sessions":["themi"],"user_default":true,"shared":["settings.json","CLAUDE.md","skills"]},
 {"agent":"pi","name":"default","home":"/home/user/.pi/agent","default":true,
  "logged_in":"no","sessions":[],"user_default":true,"shared":[]}]
```

| field | is |
|---|---|
| `name`, `default` | `default` for the agent's own folder, else the name it was added under |
| `home` | its folder: the agent's own, or mia's `accounts/<agent>/<name>`. `""` for the default account of a plugin's agent, whose folder mia does not know |
| `logged_in` | `yes` when the file the agent writes on sign-in is there; `no` when it is not and the agent keeps a sign-in nowhere else; `unknown` when mia cannot tell (claude on macOS keeps it in the Keychain, codex may use a keyring, and a plugin's agent). Only whether the file exists is checked |
| `sessions` | the worktrees with a window that runs the agent on this account now, on this computer: `main` for the main agent, `accounts` for a window signing in |
| `user_default` | the one the user-wide `[agents.<agent>] account` starts sessions on, or `default` when it names none. Feature `account-user-default`, which `mia account use <agent> <name> --global` sets |
| `shared` | what of the agent's own folder it links to, by name; empty for `default` |

`mia api accounts --projects` is which account each repository starts
each agent on: every repository mia has named a worktree in, the one
asked from, and any `mia account use` named.

```json
[{"repository":"shop","agent":"claude","account":"work","from":"mia"},
 {"repository":"shop","agent":"codex","account":"default","from":"default"},
 {"repository":"mia-core","agent":"claude","account":"personal","from":"config"}]
```

`from` is `mia` when `mia account use` set it, `config` when
`[agents.<name>] account` in the repository's or the user-wide config
says it, and `default` when nothing does.

`mia account add --json` and `mia account login --json` (feature
`account-login`) say what they opened: the `account`, in the shape above,
the `worktree`, `session` and `window` it signs in from, the `command`, and
`shared`, what was linked of the agent's own folder before it started
(`linked` now, `already` linked, and `kept`, names the account has of its
own that mia links nothing over). `mia account share --json` is a list of
`shared`, one for each account.

`mia account signals <agent> [<name> | --all] [--dry-run] --json`
(feature `account-signals`) is a list, one for each account, of what was
done with the file that [makes pi report what it does](tmux.md#pi-says-what-it-does):
`did` is `written`, `updated`, `already`, or `kept` for a file there that
is not mia's. With `dry_run` nothing was written.

```json
[{"agent":"pi","name":"default","file":"/home/user/.pi/agent/extensions/mia-signal.ts","did":"written","dry_run":false}]
```

Accounts belong to each machine. To show them for several machines, ask
`mia api accounts` on each, over ssh, and keep them apart by machine. Two
accounts on two machines are the same only by name, which is what
`mia move` goes by.

## Standing

`mia api standing [--here] [--seen <repository>/<worktree>[@<machine>]]`
is every agent in every repository on every machine: which wait on you,
which work, and which finished with nobody having looked. It is read from
each worktree's best [signal](tmux.md#signals), as `mia signal ls` reads them.
`mia asks --json` is its `waiting` list, the longest waiting first.
Feature `standing`.

```json
{"waiting":[{"repository":"shop","root":"/home/user/src/shop","worktree":"goba",
   "machine":"local","agent":"claude","window":{"index":1,"name":"claude"},
   "kind":"asks","line":"wants to use Bash",
   "options":["Yes","Yes, and don't ask again for mia commands","No"],"yes":"1",
   "since":"2026-11-02T10:00:00Z"}],
 "working":[{"repository":"shop","root":"/home/user/src/shop","worktree":"kijenge",
   "machine":"local","agent":"claude","kind":"busy","line":"","since":"2026-11-02T10:01:00Z"}],
 "done_unread":[{"repository":"mia-core","root":"/home/user/src/mia-core","worktree":"themi",
   "machine":"fedora","agent":"codex","kind":"done","line":"","since":"2026-11-02T09:40:00Z"}],
 "counts":{"waiting":1,"working":1,"done_unread":1},
 "machines":[{"name":"local","answers":true,"reach":{"state":"ok"}},
   {"name":"fedora","answers":true,"reach":{"state":"ok","warnings":[
     {"name":"linger","detail":"user services stop at logout, and the agents with them","fix":"loginctl enable-linger"}]}}]}
```

| group | holds the worktrees whose best signal |
|---|---|
| `waiting` | calls for you (rank 50 or below) and stays until answered: an agent's `asks` |
| `done_unread` | calls for you and lasts until the worktree is seen: `done`, `failed` |
| `working` | does not call for you and was set by an agent: claude's `busy` |

A worktree is in one group at most, and in none when it has no signal or
only a note. Each group is oldest first.

| field | is |
|---|---|
| `repository`, `root` | the repository's name and its main checkout on its machine, where a command about the worktree is run |
| `worktree`, `machine` | the worktree, and the machine whose mia keeps it; `local` is the one asked |
| `agent` | the agent that set the signal; `""` when something else did, such as a test watcher or you |
| `window` | the window it was set from, where an answer is typed; left out when there is none |
| `kind`, `line` | the signal's kind and its line of text |
| `options` | of a waiting one, the numbered choices its window shows, the first being 1 |
| `yes` | the keys its setter says approve it, pressed with no Enter |
| `since` | when it was set |

Every machine `mia machine list` has is asked for its own standing, at
once, by its own mia. `machines` names each one asked: one that did not
answer has `answers` false, a `detail` and a [`reach`](#machines), and no
entries. That is not an error. `--here` asks no other machine.
`--seen` first marks that worktree seen, as switching to it does, and
answers with the standing after.

## JSON commands

These commands print JSON with `--json`, beside what `mia api` gives:

| command | prints |
|---|---|
| `mia ls --json` | the projects, as [`repos`](#repos) |
| `mia p ls [project] --json` | a project's worktrees, as [`worktrees`](#worktrees) |
| `mia window ls [worktree] --json` | a worktree's windows, as in [`worktrees`](#worktrees) |
| `mia new … --json`, `mia adopt … --json` | the worktree's record |
| `mia stack show --json` | as [`stack`](#stack-and-base) |
| `mia stack sync --json` | `base`, the one fetched; `current`, the layer it is on; `dropped`, the landed layers; `restacked`, how many were rebased |
| `mia resurrect --list --json`, `mia signal ls --json`, `mia plugin ls --json` | as [`sessions`, `signals`](#sessions-and-signals) and [`plugins`](#plugins) |
| `mia asks --json` | the waiting worktrees, as in [`standing`](#standing) |
| `mia recall … --json`, `mia why … --json` | see [Conversations from other commands](#conversations-from-other-commands) |
| `mia conversation resume … --json`, `mia conversation continue … --json`, `mia hand … --json` | the same |
| `mia main status --json`, `mia main start --json` | as [`main`](#the-main-agent) |
| `mia account … --json` | see [Accounts](#accounts) |
| `mia role ls --json` | the roles |
| `mia machine ls --json` | the machines, with `reach` |
| `mia config show --json` | the effective global, project and local configuration |
| `mia doctor --json` | `checks`, each a `name`, `status` and `detail`, with a machine's `state` and `fix`; and an overall `ok`. A failure still exits non-zero |
| `mia gateway status --json` | `running`, `address`, `pac`, `ca`, `trust`, `differentBuild`, `routes` |
| `mia env host [worktree] --json` | `worktree`, `machine`, and `staging` when it runs on another machine |
| `mia env sync <worktree> status --json` | the sync session, its problems and how many conflicts |
| `mia env sync <worktree> conflicts --json` | the same, with `conflictPaths` |
| `mia env sync <worktree> preview --json` | the ignore rules, and the secret paths left out or let through |
| `mia dev --json` | whether main is lent, and its `worktree`, `path` and `stashed`. `mia dev off --json` prints `{"lent":false}`. `--follow` cannot be combined with `--json` |
| `mia gc --merged --json` | the clean merged worktrees it would remove, without removing them |

```json
{"running":true,"address":"127.0.0.1:7842","pac":"http://127.0.0.1:7842/proxy.pac","ca":"/home/user/.config/mia/gateway/ca.pem","routes":["monduli.mia"]}
```

## Apps mia desk draws

`mia chat`, `mia git`, `mia files` and `mia review` run in a pane and have
mia desk draw a view over that pane, using desk's UI protocol. mia
itself draws nothing in them. Each one refuses stdin that is not a
terminal (exit 1), and `q` or `⌃C` in the plain terminal leaves. Each is
listed in capabilities: `chat`, `git-app`, `files-app`, `review-app`.
Failures, such as a command git refuses, show up as the view's status, cut
to 200 characters, and the app stays up. A toast reports what was just
done. Each row's menu (`→`, or the pointer's other button) lists what its
keys do. What loses or sends something is asked first, in a modal whose
title says exactly what: press the same key again, or `⏎`, to do it, and
`esc` to leave it.

### mia chat

`mia chat [--window <index>] [--conversation <id>]` follows a conversation
of the worktree as a chat. With no flag it is the conversation a window
has open, else the last one written to, which is then only shown.
`mia chat --chat <name>` is a [chat](chats.md#shown-as-a-chat) of mia's,
from any folder. What you send, choose and stop in the chat is typed into
the agent's window, as `mia window send`, `press` and `key` type it. What
you send shows at once as an `asked` turn. A question the window's screen
asks that the transcript does not, such as a permission, is a question in
the chat until it is answered.

A line over the chat names the agent, the model, effort and account it
runs on. Its buttons:

| button | key | does |
|---|---|---|
| Models | `m` | lists the setups used here and every installed agent's models. A model of this agent switches the window, as `mia window model` does. Another agent's hands the conversation over: written out as a handoff, that agent started in a window of its own, the old window closed once the new one runs, and the handoff sent with the first thing you say. While the agent is at work, the pick is kept to happen next: pick it again to drop it, or `tab` on it to stop the agent and do it now. `⇧⏎` marks rows so the next message goes to each of them too |
| Fork | `f` | starts the chosen agent and model in a window of its own on the conversation up to the last reply. A reply's menu, **Continue from here with…**, forks from that reply |
| Earlier | `e` | the worktree's conversations. `⏎` shows the one a window has open, or takes another up again as `mia conversation resume` does. `c` on another agent's conversation hands it to this chat's agent: the first press says what carries over, the second does it |

That a new window's agent has not yet been sent its handoff is kept on the
window, as the tmux option `@mia_context`, so any chat of that window sends
it once. When a [mia serve](serve.md) of its build answers, the chat
listens to [`window`](#window) instead of looking every second.

### mia git

`mia git`, in any folder of a git repository, shows how that worktree
stands in git. Its head line gives the branch and where it stands against
its base (`3 ahead of main, none behind`): the layer under it in a
[stack](stacks.md), else the repository's base branch. Then come what is to
push and pull, and how many files are staged, changed and untracked.

On the left is a list: **Staged**, **Changed** and **Untracked** files,
each with its lines added and deleted; then the commits ahead of the base,
up to 20; then the **Stashes**, up to 10. Up to 400 files are listed. With
more than 30, the list is split into tabs: **Changes**, **Commits**,
**Stashes**. On the right is the diff of the row the keys are on: what is
staged, what is not, an untracked file's lines, a commit's (`git show`), or
a stash's. Diffs past 1500 lines are cut short with a note of how many are
left.

| key | where | does |
|---|---|---|
| `space` | a file | stages it, or unstages one that is staged |
| `space` | a line of the diff | stages the hunk that line is in, or unstages it in a staged file |
| `⏎` | a line added or deleted | stages or unstages that line alone |
| `d` | a changed file, a line of its diff, an untracked file | discards the unstaged change (`git restore`), the hunk, or deletes the file (`git clean`). Asked first. A staged file is refused |
| `e` | a file, a line of its diff | opens `$EDITOR` (else `nvim`) at that line, and comes back when it exits |
| `y` | a file, a commit | copies its path, or the commit's hash |
| `u` | the last commit | `git reset --soft HEAD~1`, leaving its changes staged. Asked first. Refused when the upstream has the commit |
| `space` `p` `d` | a stash | applies it, pops it, drops it (asked first) |
| `a` | | stages everything (`git add --all`) |
| `c` | | the commit screen |
| `b` | | the branches |
| `s` | | stashes everything, untracked files too, as `mia git: <branch> <date time>` |
| `f` | | fetches the remote the branch follows, else `origin` (`git fetch --prune`) |
| `P` | | pulls only when the branch can fast-forward (`git pull --ff-only --no-rebase`) |
| `p` | | pushes to what the branch follows, or sets it upstream on `origin`. Asked first. Never by force |
| `r` | | reads the standing again |

The commit screen has a text area for the message, a **Commit** button
(`⌘⏎` in the area), and **Amend the last one**. The commit is
`git commit -m` with the message as typed, and hooks run. Amending is
refused when the upstream has the commit, when the commit is the base's,
or with nothing staged and no message. The branches (`b`) are a modal with
a search field: `⏎` switches, a remote's branch is checked out to track
it, and words that match no branch offer **Create <name> from <current>**.
Fetch, pull and push run one at a time, with a progress bar. git has no
terminal there, so a remote that wants a password fails and says so.

It reads the repository again on its own: `git status` every 2 seconds,
or, when a mia serve of its build answers, each time the server says
[`status`](#status) changed. The view is sent again only when it would look
different.

Not yet: merge, rebase, resolving conflicts, force push, deleting or
renaming a branch, tags, and colour by syntax.

### mia files

`mia files` shows the worktree's files, in any folder of a repository. In
a folder that is not in a repository it shows that folder's tree, with no
marks. It hands editing to your editor. Of the worktree, it writes only
what `a`, `R` and `D` ask for.

On the left is a tree of the files git tracks and the untracked ones not
ignored. A folder opens and closes with `⏎`. The folders leading to each
changed file start open. A file changed against the base (the one
`mia git` uses) is marked `A`, `M` or `D`. On the right is the file the
keys are on, with line numbers, read 1500 lines at a time as the keys move
through it. A binary file, one over 2 MB, and one that is not a file are
said to be so.

| key | does |
|---|---|
| `e` | opens `$EDITOR` (else `nvim`) at the line the keys are on, in the worktree's top folder, and comes back when it exits |
| `f` | finds a file by the letters of its path, in order |
| `/` | finds in files: as written (`git grep -F`, case-blind when all lower case), after a `/` as a regular expression, after an `=` as whole words; up to 500 hits |
| `a` | asks a name for a new file in the folder the keys are on and opens it in the editor; a name ending in `/` makes a folder |
| `R` | renames or moves the path the keys are on, with `git mv` where git tracks it |
| `D` | deletes the path the keys are on, asked first, saying whether git can bring it back |
| `y` | copies the path |
| `c` | lists only the changed files, or all again |
| `d` | on a changed file, its diff against the base in place of its lines |
| `B` | blame: each line's commit and age; `⏎` on a line shows that commit |
| `:` | goes to a line by its number |
| `r` | reads the files again |

It reads the repository again on its own, as `mia git` does.

Not yet: editing in place, and reading a diff or a commit past 1500 lines.

### mia review

`mia review [<range> | pr <number> | w]` shows a code review of the
worktree. The review is sawa's: `sw`, the local code-review tool, keeps the
files, what was read, the findings, the gates and the verdict.
`mia review` asks `sw` for all of it as JSON and has `sw` do everything,
as you. With no argument, it opens the review sawa has of the branch
against its base, else the last one of this worktree, and with none it
starts one (`sw open -r <base>..HEAD`). `pr 14` is a pull request, `w` the
working tree, and anything else a range.

Its head gives the change's title, how the findings stand and the verdict,
then sawa's summary and the findings on the review as a whole. On the left
are the files, by folder or in sawa's reading order. On the right is the
diff of the file the keys are on, each finding under the line it is about.

| key | does |
|---|---|
| `n` `N` | the next or previous finding shown |
| `⏎` | opens a finding: its words, the edit it suggests, its replies, and **Reply**, **Reply and resolve**, **Reopen** and **Apply** (`sw apply`) |
| `esc` | closes what is open, keeping what was typed |
| `c` | on a line of the diff, comments on it as a finding (`issue:` or `suggestion:` in front sets its type) |
| `space` | marks the file read and shows the next unread one |
| `e` | opens the file in the editor at the line the keys are on |
| `f` | which findings show: open ones, those waiting on you, or resolved ones too |
| `g` | sawa's gates; `⏎` on one records **Pass**, **Warn** or **Fail** |
| `v` | the verdicts: approve, request changes, comment only |
| `y` | copies the file's path, or a finding's text |
| `o` `u` | reading order or by folder; resolved findings shown or not |
| `[` `]` | in a stack, the review of the layer under or over |
| `r` | has sawa look at the change again |

Every `sw` runs with `-t <review>` and without `SAWA_AGENT`, `SAWA_AUTHOR`
and `SAWA_SNAPSHOT`, so what is written is yours. It reads the review
again when sawa's record changes, every 2 seconds, so a finding an agent
files comes in while you read.

Not yet: editing a finding you filed, filing one on the review as a whole,
sawa's map and patterns, and a pull request's comments on GitHub.
