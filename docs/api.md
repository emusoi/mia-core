# Automation

`mia` commands are the interface for scripts, `mia-ui` and plugins. Read
commands that support structured output take `--json`; `mia api` always emits
JSON. [`mia serve`](serve.md) answers the same queries over a socket, and
tells a client that listens when an answer changes, so it need not ask
again; `mia api` asks it when it runs.

## Discover the interface

```bash
mia api capabilities
```

The answer names the API version, available queries and feature markers
introduced by newer builds:

```json
{
  "version": 1,
  "queries": ["capabilities", "worktrees", "machines"],
  "features": ["direct-http", "project-config", "doctor"]
}
```

The actual answer includes every query and feature, not this shortened list.
`capabilities`, `machines`, `here`, `repos`, `main`, `standing`,
`agents` and `apps` answer from any folder; the other queries are about a repository and need one. Check capabilities before using a feature. Existing response shapes are not
wrapped in a version envelope.

What the apps read is kept: `internal/cli/testdata/api` holds the shape of
each `mia api` answer — every field by name, its type, whether it may be
left out or null, and the values of a closed set such as a turn's `kind` or
a machine's `reach.state` — and the inputs its usage names, read off the Go
types rather than off a live answer. A test fails, naming the field, when one
is removed or renamed, changes type, may newly be left out, loses a value, or
when a query comes to need an input it did not. A field, value or optional
flag that is added passes and says to keep it with
`go test ./internal/cli -run TestTheAPIAnAppWasBuiltOnStillAnswersTheSame -update`.

## JSON commands

| command | returns |
|---|---|
| `mia api <query>` | worktrees, machines, here, repositories, environments, stacks, base, conversations, the main agent, standing, agents, apps, a worktree's git status and a window's agent |
| `mia config show --json` | effective global, project and local configuration |
| `mia doctor --json` | every check and an overall `ok`; failures still exit non-zero |
| `mia gateway status --json` | `running`, `address`, `pac`, `ca`, `trust`, `differentBuild`, `routes` |
| `mia env host [worktree] --json` | `worktree`, `machine`, and remote `staging` when present |
| `mia dev --json` | whether main is lent, and its `worktree`, `path`, `stashed` |
| `mia env sync <worktree> status --json` | sync session, problems and conflict count |
| `mia env sync <worktree> conflicts --json` | the same session with `conflictPaths` |
| `mia env sync <worktree> preview --json` | ignore rules and secret paths excluded or allowed |
| `mia gc --merged --json` | clean merged worktrees offered for removal, without removing them |
| `mia stack sync --json` | fetched base, current layer, dropped landed layers and rebase count |

In `mia api worktrees`, and in each machine's `worktrees` in `mia api
machines`, two fields mean less than their names say. `adopted` is whether
mia keeps the worktree: `true` for one made with `mia new` or taken over
with `mia adopt`, `false` for one made by hand and for a main checkout
that was never adopted. `where` is where the worktree's environment runs,
as the mia that keeps the worktree says it: `local` is that mia's own
computer, so beside `"machine": "fedora"` it means on fedora.

Each of a worktree's `windows` has a `kind`: what its first pane is for,
one of `agent`, `shell`, `editor`, `server`, `tests` or `logs`. mia marks
every pane it starts with one, as tmux's pane option `@mia_kind`, and
keeps it when the session comes back after tmux went down or crossed to
another machine. A window opened by hand in tmux has none: `kind` is left
out. See [Pane kinds](tmux.md#pane-kinds) for how each is decided.

A window that runs an app with a [manifest](apps.md) has `app`, the app's
name; it is left out for any other. When only the pane's title told the
app, under a command its manifest lists in `via`, the window's `command`
is the app's name as well, as it is `pi` for pi; told by its command, the
window keeps the `command` it has. An agent's window never has `app`.

```json
{"index":2,"name":"notes","command":"notes","quiet":"3s","active":false,"app":"notes"}
```

`mia api stack [worktree]` and `mia stack show --json` are the same answer:
the layers of the stack the worktree's branch is in, bottom first, each with
`branch`, `parent`, `landed`, `ahead`, `needs_restack`, `current`,
`base_moved` when the base moved on under the bottom layer, and `worktree`,
the name of the worktree the layer is checked out in, left out when it is
checked out in none.

```json
[{"branch":"api","parent":"main","landed":false,"ahead":2,"needs_restack":false,"current":true,"worktree":"monduli"},
 {"branch":"ui","parent":"api","landed":false,"ahead":1,"needs_restack":true,"current":false}]
```

`mia api status`, typed in a worktree, is how it stands in git, as
[mia git](#mia-git) shows it: its `branch` (`""` detached), the commit it
is at, `head`, what it follows, `upstream`, `gone` when that is no
longer there, and the commits `to_push` and `to_pull`; its files
`staged`, `changed` and `untracked`, each with its `path`, `from` when it
was renamed, `state` (`new`, `deleted`, `renamed`, `conflict`; left out
for one only changed), and its `size` and when it was last `written` in
the worktree, left out of one that is not there; and how many `stashes`
the repository keeps. It is git's own folder, no repository of mia's.

```json
{"branch":"feature","head":"5d1c…","upstream":"origin/feature","gone":false,"to_push":1,"to_pull":0,
 "staged":[],"changed":[{"path":"main.go","size":2048,"written":"2026-11-21T18:00:26-05:00"}],"untracked":[],"stashes":0}
```

`mia api window <index> [--worktree <name>]` is a window of the worktree
the folder is in, or of the one named: whether its agent is `working`, as
[mia chat](#apps) says it, and what its screen `asks` when it asks
something — its `text`, its `options` and the one `selected`.

```json
{"working":false,"asks":{"text":"Do you want to make this edit?","options":["Yes","No"],"selected":1}}
```

[`GET /events`](serve.md#get-events) asks these two again each time the
server looks, every second, as no file says when they change: `mia git`
and `mia files` listen to `status` and `mia chat` to `window` when a
server of their build answers.

## Conversations

`mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]`
is the agent conversations of the repository — its main checkout and every
worktree, one that was removed too — the last written to first. mia reads
the agents' own transcripts, so nothing is recorded as they work. opencode
keeps its conversations in a database of its own, so mia asks opencode
for them as JSON (`opencode api GET /api/session`, `opencode session
export <id>`), and only when opencode is installed and has a data folder;
each is read again only once it has changed.

```json
[{"id":"23f98c02-c52b-4de9-8c69-b322eb153bff","agent":"claude",
  "cwd":"/Users/user/src/app.monduli","worktree":"monduli","gone":false,
  "asked":"make the list faster","said":"It is faster now: one read for each file.",
  "started":"2026-11-02T10:00:00Z","active":"2026-11-02T10:41:07Z","turns":4,
  "running":true,"window":{"index":2,"name":"claude"}}]
```

| field | is |
|---|---|
| `agent` | whose it is: `claude`, `codex`, `pi`, `opencode`, or the name of an agent a plugin adds |
| `account` | the agent's [account](worktrees.md#an-agents-account) whose folder it is in, `default` for the agent's own; of a plugin's agent, the one its running window was started on. Left out for an agent mia keeps no accounts of |
| `worktree` | the worktree `cwd` is or is in; a removed one by what followed the main checkout's path; empty when none of this repository's |
| `gone` | the folder is no more |
| `asked`, `said` | the first thing the person asked and the last thing the agent said, each on one line, cut at 200 characters |
| `started`, `active` | when the first thing in it was said and when it was last written to, both in UTC as RFC 3339, to the millisecond at most |
| `turns` | how many things the person asked |
| `running`, `window` | a window of that worktree's session runs this agent with this conversation — one mia started too, which to tmux has the shell it was started by in front; `window` is left out otherwise |
| `model`, `effort` | of a running one, what mia started its window's agent with or last switched it to — see [Agents](#agents); left out when mia chose none. Of opencode's, running or not, `model` is otherwise the `provider/model` opencode says the conversation runs on |
| `continues`, `continued_by` | `{"agent":…,"id":…}`, another conversation of this list: the one this was begun from by a [handoff](worktrees.md#with-another-agent), and the one that went on from this. Left out of one that is neither |

A conversation taken up in another worktree, or on another account, was
copied there, so it is listed once for each folder it lives in: the same
`id`, told apart by `worktree`, `cwd` and `account`, each with its own
turns from where they parted.

`mia conversation continue` hands a conversation to another agent, and
the two are one thread: the first has `continued_by`, the conversation
the other agent began from the handoff has `continues`, and a chat reads
the first's turns, then the second's. A mia that says them is one whose
capabilities have `conversation-thread`. The second is known once that
agent has written its first turn — it is the first of that agent's in
the worktree, begun no earlier than the handoff, whose first message
names the handoff's file — and until then neither field is there; the
`to.window` the command gave is where it is starting. Of a conversation
handed on more than once, `continued_by` is the last that began.

Which conversation a window has open, claude says itself. Of any other
agent it is the newest of that agent's in the worktree on the account
the window was started on, so two of one agent in one worktree and
account are told apart only by which was written to last. One an agent
says it has and has written nothing of yet — just started, or cleared —
is listed all the same, `running` with its `window`, nothing in `asked`
and `said` and no `turns`; `mia api conversation` reads it as no
turns, `[]`, and it is the same of the main agent's.

`mia api conversation <id> [--worktree <name>] [--after <n>] [--follow]` is one
conversation as turns, in order, each with a number `n` that only grows.
`--after` gives the turns with a greater `n`: a view that polls asks for
what came after the last it has, and mia reads only what was written
since. Of a conversation that is in several worktrees, `--worktree` reads
the copy in the one named, as the list names it; with none it is the last
written to, the first of them in the list.

`--follow` is for a view that shows it live, in place of polling. The
turns come in one line of JSON, the same list without the indent, and
mia keeps running: each time the conversation gains turns, one more line
gives only those. Nothing is printed while nothing changes. A turn that
grew — a reply opencode is still writing — comes again under the same
`n`, so a view replaces the turn it has by `n`. A transcript is looked
at every 150 ms and read on from where it stopped once it changed;
opencode's conversations are exported again every second. It ends with
exit 0 when stdin ends, when a line cannot be written because its reader
has gone, or on SIGINT or SIGTERM: a view keeps stdin open as long as it
reads. `mia api capabilities` lists `conversation-follow`.

`mia chat [--window <index>] [--conversation <id>]` is that, shown: run
in a pane of a worktree's session, it follows a conversation of the
worktree and has mia desk draw it as a chat over the pane, by desk's UI
protocol — the whole view once, the last turns that fit a message, then
every change as a patch (a turn put in or taken out, the status, a modal
put over the chat or taken away, a toast), and the view again when the
pane comes into sight. With no flag it is the conversation a window has open, else
the last written to, which is only shown. What is sent, chosen and
stopped in the chat is typed into that window as `mia window send`,
`press` and `key` type; what was sent comes back as a turn once the
agent has written it. Whether the agent is at work is looked at once a
second. `q` or `⌃C` in the plain terminal leaves, and stdin that is no
terminal is refused, exit 1. `mia api capabilities` lists `chat`.

The view fills its pane: one quiet line over the chat says the agent, the
model and effort mia started it with or last switched it to, and the
account it is on, with two buttons at its right, `models` and `earlier`.
Of opencode, where mia chose no model, it is the one opencode says the
conversation runs on. The agent is the one the window runs, with a
conversation or none yet: the one tmux names, or the one behind node,
bun or the shell mia started the window by, as `ps` says it.
The chat's field types every letter, so `tab` goes from it to the
buttons, where `m` and `e` (or `⏎`) press them. A window named by
`--window` that has no conversation mia can read yet is a chat all the
same, and its conversation is looked for each second; of an agent mia
reads no conversations of (gemini, cursor-agent) the status says once
that its replies are in its terminal. When a [mia serve](serve.md) of
its build answers, the chat does not look each second: it listens to
`mia api window` for its window and looks when the server says the agent
went to work or stopped or the screen asks another thing — its
conversation, while it has none, among what it looks for then.

- **What is sent** is said at once as an `asked` turn after the last, and
  taken out (`unturn`) in the patch that puts in the transcript's own
  turn of the same words. When the agent was at work and another turn
  comes first, the turn is taken out and the status says it waits. A
  send the window refuses — it asks something first — takes the turn
  out, gives the words back to the chat's field (`draft`) and says why
  as the status.
- **Questions.** An answer chosen is pressed in the window as
  `mia window press` presses it, for every agent: its digit where the
  options are numbered, the cursor moved to it and Enter where they are
  not. A press refused is said as the status, the question open again.
  What the window's screen asks that the transcript does not — a
  permission, a folder to trust — is a question of the chat's own after
  the last turn, looked for each second and taken out once answered.
- **How a thing went.** The status is said alone, as a change. What went
  well — a model switched, a conversation taken up or handed over, an id
  copied — is a toast, which goes by itself; what did not is the status,
  or, of something asked in a modal, a line at the modal's foot.
- **Models** (`m`), a modal over the chat, which stays in sight behind
  it: a search line with the keys over one list in a room of its own —
  the setups used in this repository, then the models of each agent
  installed under its head, the one that runs checked, an item's menu
  (`→`, or the pointer's other button) its efforts. A pick takes the
  modal away. opencode's are those it lists, each with its
  provider beside it, and none when it lists none: one row then, its
  model chosen inside it. `⏎` on a model of the chat's own agent switches
  the window as `mia window model` does — opencode by starting it again
  on the conversation, in the same window. On another agent's it hands the
  conversation over, in one step: its turns written out as
  `mia conversation continue` writes a handoff, that agent started on
  the model in a window of its own and told nothing, the old agent's
  window closed once the new one runs — never before, and not when it is
  the chat's own window — and the chat goes on in the new window, the
  turns so far over a line `continued by <agent>`. The first thing then
  sent has the handoff in front of it in the same message, under `The
  conversation so far, with <agent>:` and over a rule; past 48 KB only
  its last turns, with where the whole file is. The chat shows the
  person's own words alone, of the transcript's turn too. No agent's own
  files, session or sign-in is read, copied or converted. `esc` takes
  the modal away and leaves the chat, its status and what runs as they
  were. That the new window's agent was not told yet is kept on the
  window itself, as its tmux window option `@mia_context` — whose
  conversation, its id and the handoff's file — so a `mia chat` started
  for that window after, in place of the one that handed it over, shows
  the turns so far over the same line and says the handoff with the
  first thing sent; whichever chat sends first takes it off the window,
  so it is said once, and a send that did not go keeps it there.
- **Earlier** (`e`), a modal over the chat: the worktree's conversations,
  what was first asked with the agent and how long ago, the one a window
  has open marked, and beside the list the one under the keys read back.
  `⏎` on one a window has open shows it, on another takes it up again as
  `mia conversation resume` does and follows it. `c` on a conversation
  of another agent hands it to this chat's agent as
  `mia conversation continue` does, told to read it: the first press
  says at the modal's foot what that carries over, the second does it.
  Each has a menu (`→`, or the pointer's other button): **Resume** (or
  **Go to it**) as `⏎`, **Continue with** this chat's agent on another
  agent's, at once, and **Copy its id**. `esc` takes the modal away.

A turn desk was told stays as it was told, so a turn of the chat's own —
what was sent, what the screen asks — has a place after any the
transcript has (past 2^30) and is taken out by it, never said again
otherwise; what is typed in the chat's field stays. A chat of another
window or conversation is another piece (`chat`, `chat-1`, …).

```json
[{"n":1,"kind":"asked","at":"2026-11-02T10:00:00Z","text":"make the list faster"},
 {"n":2,"kind":"said","at":"2026-11-02T10:00:04Z","text":"I will **look** at where the time goes."},
 {"n":3,"kind":"tool","at":"2026-11-02T10:00:05Z","text":"Edited view.lua +12 −3","name":"Edit","file":"/Users/user/src/app.monduli/view.lua"},
 {"n":4,"kind":"question","at":"2026-11-02T10:00:09Z","text":"Which one?","options":["Fast","Small"],"answer":"Fast"},
 {"n":5,"kind":"answer","at":"2026-11-02T10:00:30Z","text":"Fast","question":4},
 {"n":6,"kind":"commit","at":"2026-11-02T10:01:00Z","hash":"abc1234","subject":"The list is faster"}]
```

| kind | is |
|---|---|
| `asked` | what the person said: `text` |
| `said` | what the agent said: `text`, markdown as written |
| `tool` | what the agent did, in a line — `Ran go test ./...`, `Read 3 files` — with the tool's `name`, and `file` when it is one file's |
| `question` | the agent asked the person to choose: `text`, `options`, and `answer` once it is given |
| `answer` | the person chose: `text`, and `question`, the `n` of the turn it answers — so a view that already has the question learns of it |
| `commit` | the agent made a commit: `hash` and `subject` |

What the agent thought, what its tools gave back and what the system told
it are left out. `mia recall <text…> --json` gives each conversation that
mentions the text with its `agent`, its `worktree` and `n`, the turn to
open it at; `n` is 0 for words that were a subagent's. A conversation that
was copied to another worktree is given once for each folder that mentions
it, and `n` counts in that one's. Each says its `account`, as the list
does; so does each conversation `mia why --json` gives.

## The main agent

`mia api main` is where the [main agent](main.md) stands. It answers from
any folder, and starts and writes nothing.

```json
{"running":true,"agent":"claude","machine":"local",
 "home":"/Users/user/.config/mia/main","session":"mia-main",
 "window":{"index":1,"name":"claude"},
 "conversation":"23f98c02-c52b-4de9-8c69-b322eb153bff",
 "working":true,"since":"2026-11-02T10:00:00Z"}
```

| field | is |
|---|---|
| `running` | the agent is in a window of its session |
| `agent` | the one that runs — another than `[main] agent` when `mia main start --agent` started it, which a mia can when its capabilities have `main-start-agent`; when none runs, the one `[main] agent` names |
| `machine` | the machine it lives on, as `mia api machines` calls it: `local` when that is the one asked, and the name `[main] machine` gives when it is another, whose mia answered |
| `home`, `session` | its folder and its tmux session, whether or not they are there yet |
| `window` | the agent's window; left out when it is not running |
| `conversation` | the one it has open — claude says which — or, of another agent and when it is not running, the last written to in its folder; empty when there is none, or mia cannot read that agent's. Started with a [new conversation](main.md#a-new-conversation) — `mia main start --new`, which a mia has when its capabilities have `main-start-new` — it is that one, and empty until an agent that does not say which it has open has written the first of it: another id than before, or none, is a conversation that changed |
| `working` | it is doing something now: claude's pane title says so, and of another agent, that its window wrote in the last three seconds |
| `asks` | the question its window shows, waiting on you: `text`, `options` and `selected`, the option its cursor is on, the first being 1, what Enter alone would answer; left out when it asks nothing, and `working` is false while it asks. A mia has it when its capabilities have `main-asks` |
| `since` | when its session was started; left out when it is not running |
| `model`, `effort` | what mia started it with or last switched it to, as [Agents](#agents) says; left out when it is not running, or mia chose none |
| `account` | the [account](#accounts) it runs on, `default` for its agent's own; left out when it is not running, or for an agent mia keeps no accounts of |

```json
{"running":true,"agent":"claude","working":false,
 "asks":{"text":"Is this a project you created or one you trust?",
         "options":["No, exit","Yes, I trust this folder"],"selected":1}}
```

While it asks, `mia main say` and `mia main model` type nothing and fail,
exit 1, with a line that says what it asks and how to answer; `mia main
press <n>` answers it, as [The main agent](main.md#when-it-asks-first)
says. `mia main start --json` is the same state, `asks` and all, once the
agent has settled.

When `[main] machine` names another machine, `mia api main`,
`mia api main conversations` and `mia api main turns` are that machine's answers, asked over ssh: `home`,
`session` and `window` are its own. One that does not answer is an error
in a line that names it, exit 1, and is not asked again for 30 seconds.

`mia api main turns [--after <n>] [--follow]` is that conversation as
[`mia api conversation`](#conversations) gives one, an empty list when
there is none, and `--follow` follows it as it does there — the one
open when it starts; with none open, `[]` and nothing more. `--follow`
reads a main agent on this machine only, and is refused, exit 1, when
`[main] machine` names another. It has one kind more:

```json
[{"n":7,"kind":"asked","at":"2026-11-02T10:00:00Z","text":"start goba on the login bug"},
 {"n":8,"kind":"did","at":"2026-11-02T10:00:04Z","text":"Started goba on mia-desk · claude","name":"Bash",
  "verb":"window new","repository":"mia-desk","worktree":"goba"},
 {"n":9,"kind":"did","at":"2026-11-02T10:02:11Z","text":"Moved sakina to fedora","name":"Bash",
  "verb":"move","repository":"mia-desk","worktree":"sakina","machine":"fedora"},
 {"n":10,"kind":"tool","at":"2026-11-02T10:02:15Z","text":"Ran mia api standing","name":"Bash"}]
```

`mia api main turns --conversation <id> [--after <n>]` is one of its
[earlier conversations](main.md#earlier-conversations) in the same way,
and `mia api main conversations` lists them: the heads
[`mia api conversations`](#conversations) gives, of the conversations
made in the main agent's folder with every agent it was run as, the last
written to first. `running` is true of the one it has open now, with its
`window`; `worktree` is its folder's name. An id that is none of these is
refused, exit 1. `mia main start --resume <id> [--restart]` starts it
going on with one. A mia has all three when its capabilities have
`main-conversations`.

A `did` is a shell command the main agent ran that is one of mia's verbs
and changes something. Nothing is logged for it: it is the `tool` turn of
the same `n`, read again. `verb` is the verb as typed; `repository` is the
one the command ran in, by its main checkout's folder name; `worktree` and
`machine` are there when the command names them, the worktree also when
the command was typed inside it.

| `verb` | `text` |
|---|---|
| `new` | `Made a worktree for fix-login on mia-desk`, with `, on fedora` after `--on`; `worktree` only when `--name` gave it |
| `agent run`, `window new` | `Started goba on mia-desk · claude`; a window that runs no agent stays a `tool` |
| `agent send`, `window send` | `Told olasiti: rebase onto main` |
| the same, or `window press`, with a number | `Answered kijenge: 1` |
| `agent stop` | `Stopped the agent in goba` |
| `move` | `Moved sakina to fedora`, or `Moved sakina back here` |
| `rm`, `archive` | `Removed goba on mia-desk`, `Archived goba on mia-desk` |

Only a command that is `mia …` alone, or `cd <folder> && mia …`, is read:
its words, quotes and backslashes as a shell splits them. One with a pipe,
a redirection, a variable, a substitution or a third command stays a
`tool`, since what it ran is not what its words say. So does a verb that
only reads — `mia api …`, `mia ls`, `mia window read` — one with
`--dry-run`, and one typed in a way its usage does not have.
`mia api conversation <id>` gives the same conversation with no `did` in it.

## Agents

`mia api agents` is every agent mia knows, one a plugin adds too, with
what it can be started with. It answers from any folder; in a
repository, with that repository's config.

```json
[{"name":"claude",
  "models":[{"id":"opus","label":"Opus"},{"id":"sonnet","label":"Sonnet"},
            {"id":"haiku","label":"Haiku"},{"id":"fable","label":"Fable"}],
  "efforts":["low","medium","high","xhigh","max"],
  "model":"opus","effort":"","live":true,"again":false,"account_env":"CLAUDE_CONFIG_DIR","installed":true},
 {"name":"opencode",
  "models":[{"id":"openai/gpt-5","label":"gpt-5"},{"id":"ollama/qwen3:4b","label":"qwen3:4b"}],
  "efforts":[],"model":"","effort":"","live":false,"again":true,"account_env":"","installed":true},
 {"name":"cursor-agent","models":[],"efforts":[],"model":"","effort":"","live":false,"again":false,"account_env":"","installed":false}]
```

| field | is |
|---|---|
| `models` | what it can be started with: `id` as the agent takes it, `label` to show. Empty for an agent mia knows none of |
| `efforts` | its effort levels, how hard it thinks; empty when it has none mia knows |
| `model`, `effort` | what the config starts it with, `[agents.<name>]`; empty when it says none, and the agent's own default is used |
| `live` | it can be switched to another model while it runs, by what is typed into it |
| `again` | it takes a model only as it starts: `mia window model` ends it and starts it again in its window, on the conversation it has open |
| `account_env` | the variable that names the folder of each of its [accounts](#accounts); empty for an agent mia keeps no accounts of |
| `installed` | its program is on this machine's `PATH` |

opencode's models are those of the providers it is signed in to, which
only it knows: mia asks it, `opencode models`, and lists each
`provider/model` it prints under that id, labelled by what comes after
the provider. What it said is kept for an hour, in the repository's
`mia/cache/models/opencode.json`, so asking for the agents does not
start opencode each time. When it cannot say — not installed, nothing
kept by it yet, not signed in, no answer in 5 s — its `models` are
empty, and that is kept for a minute. opencode's own interface has no
word to be started with a model; its minimal one has, so with a model
mia starts it as `opencode mini --model <provider/model>`, and with none
as `opencode`. It has no effort mia can choose: its help names a variant
of a model only for `opencode run`, and lists none. Neither has it a
line to type that switches it, so it is switched by being started again,
as `opencode mini --model <provider/model> --session <id>` in the pane
it ran in: the window, its number and what is kept on it stay. One with
no conversation yet is refused: close it and start it with `--model`.

agy's models, the Antigravity CLI's, are those its account may use:
mia asks it, `agy models`, and lists each line it prints that is an id
and a name a tab apart, kept as opencode's are, in
`mia/cache/models/agy.json`, and asked only where agy has made its
folder, `~/.gemini/antigravity-cli`. It is started with one and an
effort as `agy --model <id> --effort <level>`, its efforts `low` to
`max`.

gemini has a word for a model, `--model`, and neither lists its models
nor names any in its help: its `models` are those the config names,
`[agents.gemini] model = "…"`, which it is then started with.

A model the config names that the list has not is in the list too.
Start one with `mia window new <worktree> <agent> --model <id> --effort
<level>` and switch one that runs with `mia window model <worktree>
<window> <id> [--effort <level>]`; take a conversation up again with one
with `mia conversation resume <id> --model <id> --effort <level>`; the
main agent with `mia main start --model …` and `mia main model …`. A
model or an effort not in its list is refused, with the list.

To restart an agent with another model, stop it — `mia window key
<worktree> <window> -- C-c` — and resume its conversation with the model.
`mia conversation resume --json` says the `model` and `effort` it was
started with, or those of the window that has it open already; resumed
with `--model` or `--effort` while a window has it open, it is refused.
A mia that can is one whose `mia api capabilities` has `resume-model`;
one that lists opencode's models and starts it again to switch it has
`agent-again`.

`mia conversation resume --json` also says the `account` the agent was
started on, and `copied` when the conversation was copied into that
account's folder first.

`mia conversation continue <id> --with <agent> [--worktree <name>]
[--dry-run] --json` — a mia has it in this form when its capabilities
have `conversation-thread` — goes on with one conversation [with another
agent](worktrees.md#with-another-agent), and says what it did. `<id>` is
the conversation's `id` as `mia api conversations` lists it, and
`--worktree` its `worktree` there, needed only for one listed in two.
`--from <window>` in place of the id is the one that window has open.

```json
{"from":{"agent":"claude","id":"23f98c02-c52b-4de9-8c69-b322eb153bff","worktree":"monduli","window":{"index":2,"name":"claude"}},
 "to":{"agent":"codex","window":{"index":3,"name":"codex"}},
 "handoff":"/src/app/.git/mia/handoffs/monduli/claude-23f98c02-c52b-4de9-8c69-b322eb153bff.md",
 "full":"/src/app/.git/mia/handoffs/monduli/claude-23f98c02-c52b-4de9-8c69-b322eb153bff-full.md",
 "turns":212,"left_out":158,"command":"codex",
 "instruction":"Read /src/app/.git/mia/handoffs/monduli/claude-23f98c02-….md — a conversation the person had with claude …",
 "sent":true,"account":"default","dry_run":false}
```

`from` is the conversation and the worktree it is in, with the `window`
that has it open when one does. `to` is the agent it goes on with and
the `window` opened for it; the conversation that agent begins has no id
yet, so `to` has none: `continued_by` in [`mia api
conversations`](#conversations) says it once the agent has written its
first turn. `handoff` is the file the conversation was written out to,
of `turns` turns; when it was over the budget, `left_out` is how many of
its middle are not in it and `full` the file that has every one, both
left out otherwise. `model`, `effort` and `account` are as a resume says
them. `sent` is whether mia typed `instruction`, the agent's first
message, into the window: an agent that asked something first, or ended
as it started, is typed nothing, `problem` says which, and the window is
open for `instruction` to be said there — with `--json` that is exit 0,
`sent` false. With `--dry-run` nothing was written or opened: `dry_run`
is true, `to.window` has the name it would have and no `index`, and
`sent` is false. Refused — no such conversation, a window with none or
several open, the agent it is already with — it prints no JSON, says why
on standard error and exits 1.

The `model` and `effort` that `mia api conversations` and `mia api main`
give a running window are what mia started it with or last switched it
to, kept by mia for the window: not read from the agent. One switched by
hand in the agent is not known, and a window mia chose nothing for has
neither.

## Apps

`mia api apps` is every app with a [manifest](apps.md), by name. It
answers from any folder, and reads the manifests as they are then. Check
capabilities for `apps`: an older mia has neither this nor a window's
`app`.

```json
[{"name":"lazygit","label":"Git","icon":"/Users/user/.config/mia/apps/lazygit/icon.svg",
  "match":{"commands":["lazygit"],"via":[]},
  "keys":[{"key":"c","label":"commit"}]},
 {"name":"notes","match":{"commands":[],"title":"^notes","via":["node","python3"]},"keys":[]}]
```

`label` is left out when the manifest has none, `icon` when it names none
or the file is not there — it is the file's whole path otherwise — and
`match.title` when the app is told by its command alone. `commands`, `via`
and `keys` are always lists, empty when there are none. A manifest that
does not read, or is named as an agent, is left out and said in a line on
stderr; the others are listed all the same and the exit status is 0.

### mia git

`mia git` is an app of mia's own: run in a pane, in any folder of a
repository, it has mia desk draw how that worktree stands in git over the
pane, by desk's UI protocol, as `mia chat` draws a conversation. It needs
no mia worktree: it is about the repository of the folder it is run in.
`mia api capabilities` lists `git-app`.

Over the view is the branch, where it stands against its base — `3 ahead
of main, none behind`; the base is the layer under it in a
[stack](stacks.md) mia knows, else the repository's base branch, and for
the base branch itself what it follows — then, for a branch that follows
one of a remote, what is to push and to pull (`2 to push, 1 to pull`),
and how many files are staged, changed and untracked. Under it, side by
side:

- A list, `files`: the files under **Staged**, **Changed** and
  **Untracked**, each head with its count and a group with none left out,
  each file with its lines added and deleted and `new`, `deleted`,
  `renamed` or `conflict`; then the commits ahead of the base, newest
  first, 20 at most, each with its age; then, when there are any, the
  **Stashes**, newest first, 10 at most. An item's id says its group:
  `s:<path>`, `c:<path>`, `u:<path>`, `k:<short hash>`, `t:<the stash's
  commit, 10 characters>`. At most 400 files are listed, and a head then
  says how many of its group are shown. With more than 30 files the list
  is behind `tabs` — **Changes**, **Commits**, **Stashes**, each with its
  count, one with nothing left out — and shows the chosen one's groups
  alone; with fewer it is the one list. Each row carries a menu of what
  can be done to it, opened beside it with `→` and under the pointer's
  other button, each choice with the key that does the same at its
  right: on a file Stage or Unstage, Discard… (Delete… on an untracked
  one), Open in editor, Copy path; on a commit Copy its hash, and Undo…
  on the last one where it can be undone; on a stash Apply, Pop, Drop….
- The diff of the row under the keys, a code `diff`: of a staged file
  what is staged (`git diff --cached`), of a changed one what is not
  (`git diff`), of an untracked one its lines as added, of a commit its
  own (`git show`), of a stash what it changed and its untracked files.
  Over a commit's diff are who wrote it and when, its subject, and the
  files it changed, each with its lines added and deleted, the first 6
  and then how many more. Each hunk has a head that says which of how many it
  is, each line its number in the old file and the new, and the lines
  git left out between two hunks are a gap. A tab is said as the spaces
  to the next fourth column. A diff longer than a message is said on in
  patches of `lines`, and past 1500 lines it ends in a gap that says how
  many more there are.

| key | where | does |
|---|---|---|
| `space` | a file | stages it, or unstages one that is staged |
| `space` | a line of the diff | stages the hunk that line is in (`git apply --cached`), or unstages it in a staged file's (`--reverse`); in an untracked file's, stages the file |
| `⏎` | a line added or deleted | stages that line alone, or unstages it in a staged file's: a patch of the hunk with that one change, the others written as the index has them |
| `d` | a changed file, a line of its diff, an untracked file | discards what is changed and not staged (`git restore`), the hunk that line is in (`git apply --reverse`), or deletes the file (`git clean`). Asked first. A staged file is refused: unstage it first |
| `e` | a file, a line of its diff | hands the file to `$EDITOR` (else `nvim`), at that line of it, as [`mia files`](#mia-files) does, and takes the pane again when it leaves. A deleted file is refused |
| `y` | a file, a commit | copies the file's path, or the commit's hash, to the clipboard (the window's `copy`) |
| `u` | the last commit | takes it off the branch and leaves what it changed staged (`git reset --soft HEAD~1`). Asked first. Refused on an older commit, and on one the branch's upstream has |
| `space` `p` `d` | a stash | applies it and keeps it, pops it, drops it (asked first) |
| `a` | | stages every file (`git add --all`) |
| `c` | | the commit screen |
| `b` | | the branches |
| `s` | | stashes every change and every untracked file (`git stash push --include-untracked`) under `mia git: <branch> <date time>` |
| `f` | | fetches the remote the branch follows, else `origin` (`git fetch --prune`) |
| `P` | | pulls where the branch can only be moved on (`git pull --ff-only --no-rebase`); it never merges and never rebases, and says git's reason where it cannot |
| `p` | | pushes the branch to what it follows, or with `--set-upstream` to `origin` under its own name where it follows nothing. Asked first. Never by force |
| `r` | | reads the standing again |

The foot says the keys of where the keys are: a file's on a file, a
hunk's in its diff, `u` on the last commit, a stash's on a stash, then
those of the worktree whole; `a` and `s` only when there is something to
stage or stash. The window says the piece the keys are on as it changes
(`focus`), and the keys are said again alone, by a patch's `keys`; what
`⏎` does on a line of the diff is the code's own `does`, `stage line` or
`unstage line`.

**Asked first**: the first press does nothing and puts up a modal,
`sure`, whose title says exactly what is lost or sent — `Discard hunk 2
of 2 of talk.txt (+1 -1)?`, `Push 2 commits to origin/talk?` — over a
line of what that means, with two buttons: the one that does it, `yes`,
whose key is the key that asked, so the key pressed twice does it as `⏎`
does; and `no`, which with `esc` takes the modal away and does nothing.
What it says is checked again as it is confirmed: where the file or the
count is another by then, nothing is done and the modal is said again in
the new words. When it goes the keys are back on the piece they were on.

**What was just done** — `committed a41c9e2`, `staged 3 files`,
`discarded a hunk of talk.txt`, `on flags`, `copied src/talk.rs`, the
line git wrote of a fetch — is a toast, said once. The status is for
what was refused or failed and is yours to act on, and stays until the
next thing is done; it and the keys are said by a patch's `status` and
`keys` where nothing else of the view is another.

**With a remote** (`f`, `P`, `p`) git runs beside the app, one at a time:
a `progress` bar at the head's end says `fetching origin…` and the keys
are taken meanwhile, another deed with a remote refused in a toast; the
line git wrote last is a toast after, or of one that failed the first
that says why is the status, and the standing is read again. git has no
terminal there: a remote that wants a password typed fails and says so,
and nothing is asked over the view.

After each of these the standing is read again, and the keys stay on the
same file where it still is — a file staged is followed into Staged —
else on the row after it. It is read again by itself when the repository
changes: `git status` is asked every 2 s, with `--no-optional-locks`, and
the file the stash is kept in is looked at — or, when a [mia
serve](serve.md) of its build answers, it is read again when the server
says `mia api status` of the worktree is another, asked once a second for
all who listen; the view is said again only when it would say something
else. A file
already changed that changes again is seen by its size and time; the
base moving on is not seen until something else changes, or `r`.

The commit screen has, in place of the diff alone, `Commit to <branch>`
with what is staged, a text area for the message, a **Commit** button
(`⌘⏎` in the area sends it too), an **Amend the last one** button and
the diff of the row under the keys under them; the list stays, and `space` and `a` work in it. `esc` goes
back to the files and keeps the words typed. The commit is `git commit
-m` with the message as typed: nothing is added to it, and hooks run.
With nothing staged, or no message, nothing is done and the view's status
says why; a hook that fails is said by its first line. Made, a toast
says `committed <short hash>`, back on the files.

**Amend the last one** (`amend`; `tab` to it and `⏎` — the `⌘A` on it is
said and is no key the window takes) makes the last commit again with
what is staged added, under the message typed, or with none under the
one it had (`git commit --amend`). It is refused, with the reason as the
status, when the branch's upstream has that commit already, when the
commit is the base's and not this branch's, and with nothing staged and
no message.

**The branches** (`b`) are a modal, `switch`, over the view: a search
field, `q`, over
a list, `branches` — the repository's own (`l:<name>`), the one the
worktree is on checked, each with how it stands against what it follows
(`↑2 ↓1`, `level`, `gone`, `no upstream`), then under **Remote** the
remotes' that no branch here is named for (`r:<remote>/<name>`), 300 at
most. Words typed narrow them, whatever their case. `⏎` switches (`git
switch`); on a remote's it makes the branch that follows it (`git switch
--track`); and words that are no branch's name offer `Create <name> from
<current>` (`n:<name>`, `git switch --create`) — first where they are in
no name, else last. What git refuses — a file changed that the branch
has otherwise, a branch another worktree is on — is the status, in git's
first line, said in the modal too, and the branches stay up. `esc` goes
back to the files.

What fails — a folder in no repository, git not installed, a hunk git
will not apply — is the view's status, cut to 200 characters, and the app
stays up. `q` or `⌃C` in the plain terminal leaves, as closing the view
does; stdin that is no terminal is refused, exit 1.

Not yet: merging and rebasing, resolving a conflict, a push by force,
deleting and renaming a branch, tags, a commit older than those ahead of
the base, and colour by syntax.

### mia files

`mia files` is an app of mia's own too: run in a pane, in any folder of a
repository, it has mia desk draw that worktree's files over the pane, by
desk's UI protocol. Editing is handed to your editor; of the worktree it
writes only what `a`, `R` and `D` are asked to. In a folder of no
repository the tree is that folder's, with no marks. `mia api
capabilities` lists `files-app`.

Side by side:

- A list, `tree`, under a head with the repository's name and `3 changed
  against main`: the files git tracks and those it does not, ignored ones
  left out (`git ls-files --cached --others --exclude-standard`), as a
  tree — in each folder its folders, then its files, by name; an item's
  id is its path, its `depth` how far in it is, and a folder has `open`.
  Only a folder you opened is listed under: `⏎` on one opens or shuts
  it. As the view comes up the folders on the way to each changed file
  are open, unless that is more than 30 folders. A file changed against
  the base carries `A` (added, or untracked), `M` (changed) or `D`
  (deleted; it is still listed). The base is the one `mia git` stands a
  branch against, and the files are compared, as they are on disk, with
  the commit the branch left it at; with no base, with the last commit.
  At most 400 rows are listed, and a head then says how many more there
  are. `c` lists only the files changed, with the folders on the way to
  them, and the head says `only the 3 changed against main`; `c` again
  lists all.
- The file of the row under the keys: its path, `new against main · 19
  lines`, and a code `file` with its lines numbered, a tab as the spaces
  to the next fourth column. `l` passes the keys to it, `h` back. A
  binary file, one over 2 MB and one that is no file are one gap that
  says so. A UI holds 2000 pieces, the tree's rows among them, so of a
  file of more than 1500 lines 1500 are said at a time, on in patches of
  `lines` where they are more than a message, between a gap that says how
  many lines are above and one that says how many more there are, and
  the line over the code says which: `lines 701–2200 of 5000`. The code
  is `live`: when the keys come within 100 lines of the last line said
  and the file goes on, the view is said again with the lines around the
  keys' line and the keys on it (the code's `selected`), and so within
  100 of the first; any file is read to its end this way.

| key | does |
|---|---|
| `e` | hands the file to the editor, at the line under the keys where they are in the file, else at its first: the view is closed, `$EDITOR` (else `nvim`) is run in the worktree's top folder with `+<line> <path>` and this terminal as its own, and when it leaves the files are read again and the view said again |
| `f` | finds a file |
| `/` | finds in files |
| `a` | asks a name, in a one-line field `name` in a modal, `ask`, over the view (the message's `focus` puts the keys in it), for a new file in the folder under the keys, or the folder of the file under them: `⏎` makes it with the folders on the way to it, puts the keys on it in the tree and hands it to the editor at once; a name that ends in `/` makes a folder, listed while nothing is in it. A path that is there already, one outside the worktree and one in `.git` are refused in the status, said under the field too, and the modal stays; `esc` asks no more |
| `R` | asks where the path under the keys goes, in the same modal, the field holding it as it is: `⏎` renames or moves the file or the folder, with `git mv` where git tracks it and on disk alone where it does not, and refuses a path that is there already |
| `D` | deletes the path under the keys, asked first: pressed once it puts up a modal, `sure`, whose title says what goes — `Delete src/x.rs, 120 lines?`, and of a folder `Delete src/new and its 3 files?` — over whether git can bring it back (`It is not in git: nothing can bring it back.`, `Tracked: git can bring it back.`, `Tracked: git can bring back only what is committed.`, `1 of them not in git, which nothing can bring back.`), and deletes nothing. Its button **Delete** (`yes`, whose key is `D`, so `D` twice does it) deletes the path from disk (not from git's index); **Keep it** (`no`) and `esc` take the modal away. What goes is counted again as it is confirmed, and where it is another the modal says it anew and nothing is deleted |
| `y` | copies the path under the keys, by its steps from the top, to the clipboard (a patch's `copy`), and a toast says `copied <path>` |
| `c` | the tree of the changed files alone, and of all again |
| `d` | on a changed file: its diff against the base in place of its lines — `git diff <base's commit> -- <path>`, hunks under heads with the lines added and deleted, a file git does not track all added — with `diff against main · +15 -3` over it; `d` again shows the file. It stays as the keys go to other changed files |
| `B` | blame: each line of the file says at its `right` its commit by a short name and how long ago it was written (`3f9a2c1 3d`, from `git blame --porcelain`, asked once for a file as it stands), a line not committed `not committed`; `B` again takes them off. `⏎` on a line while they are there shows that commit in place of the file — its name, its date and its subject over what it changed of this file — and `esc` is back in the file on that line |
| `:` | asks a line's number in a one-line field `line` in the modal: `⏎` says the file from around that line with the keys on it |
| `r` | reads the files again |

Each row of the tree carries a menu, opened beside it with `→` and under
the pointer's other button, each choice with the key that does the same
at its right: Open in editor, New file here…, Rename…, Delete…, Copy
path, Diff against the base, Blame; a folder's has the four in the
middle. What was just done — `made <path>`, `now <path>`, `deleted
<path>`, `copied <path>` — is a toast, said once; the status is for a
search and for what was refused, and it and the keys are said by a
patch's `status` and `keys` where nothing else of the view is another.
The foot's keys are those of the piece the window says the keys are on
(`focus`): on a line of the file, those that are about the file.

**Finding a file**, the left side is a field, `q`, over a list, `found`:
the files whose path has the letters typed, in order and whatever their
case — those with them in the file's name first, then the shorter path,
50 at most — narrowed with each change of the words. The file under the
keys is at the right. `⏎` goes back to the tree with every folder on the
way to it open and the keys on it; `esc` goes back to the tree as it was.

**Finding in files**, the field is over a list, `hits`: `⏎` searches for
the words as they are written, not as a pattern (`git grep -n -I
--untracked -F`, and `-i` when they are all in lower case; binary and
ignored files are passed over). Words after a `/` are a regular
expression (`-E` in place of `-F`), and words after an `=` are found
only where they are whole words (`-F -w`); the status says which, `6 in
4 files · as a pattern`, and a pattern git cannot read is the status.
What git finds is said as it finds it,
the hits of a file under a head with its path and their count, each line
by its text with its number at the right, in patches of `item` no more
often than every 100 ms; the status is `searching…` and then `6 in 4
files`. At 500 hits the search is stopped and the status says there are
more. The hit under the keys is at the right: its file with the keys of
the code on the hit's line, 960 lines of it at a time around that line
where it has more. `↑`
`↓` walk the hits from the field and `⏎` goes back to the tree with the
keys on the hit's file; `e`, on a hit or a line, edits there. Other
words typed drop what was found, so `⏎` searches for them; a new search
and `esc` stop one at work.

The files are read again by themselves when the repository changes, as
`mia git` does it: `git status` is asked every 2 s, or the server is
listened to, and the view is said again only when it would say something
else. Listening, a file git ignores that is shown and written again is
not seen until something else changes. What fails — a command git
refuses, an editor that cannot be run — is the view's status, and the
app stays up. `q` or `⌃C` in the plain terminal leaves; stdin that is no
terminal is refused, exit 1.

A key that is not the program's is not said to it, so a delete asked
for once is dropped by the next thing done that is: the keys moved, or
another of these keys.

Not yet: editing in place, and a diff or a commit of more than 1500
lines read past them.
### mia review

`mia review [<range> | pr <number> | w]` is an app of mia's own: run in a
pane of a worktree, it has mia desk draw a code review of that worktree
over the pane, by desk's UI protocol. The review itself is
sawa's — `sw`, the local code-review tool, keeps the files, what was
read, the findings, the gates and the verdict — and `mia review` is a
face on it: everything it shows it asks `sw` for as JSON, and everything
it does it has `sw` do, as you. `mia api capabilities` lists
`review-app`.

With no argument it shows the review sawa already has for this worktree:
the one of the branch against its base (`<base>..HEAD`), else the one of
this worktree last written to — never a pull request's, which is every
worktree's. With none it opens one, `sw open -r <base>..HEAD`; the base
is found as [`mia git`](#mia-git) finds it. An argument is opened as `sw
nvim` takes one: `pr 14`, `w` for the working tree, anything else a
range.

Over the view is the change's title — a pull request's own, a branch's
one commit's subject, else the branch — with, at its right, how the
findings stand (`3 open`; by the filter `1 to answer`, or `3 open, 1
resolved`) and the verdict. Under it are what the review is of (`words →
main · 3 files · +5 -3 · sawa read it 4 min ago · layer 2 of 3`, the age
that of the finding filed last, the layer said only where sawa reviews a
stack) and sawa's summary, cut to 400 characters. Under the summary is a
list, `general`, of the findings on the review as a whole — those on no
file — each by its first line with its label before it and who filed it
and how it stands at its right; `⏎` on one opens it under the list, all
its words and its thread, as a file's is opened under its line.

Under those, side by side:

- A list, `files`, by folder: each folder open over its files, a file
  with its lines added and deleted, how many findings stand on it, and
  checked once read. `o` puts them in reading order instead — the files
  on sawa's path first, in its order, each with the path's one line of
  why — and says so where sawa has no path. At most 400 files are listed.
- The diff of the file under the keys, a code `diff`, from `sw rows
  --changed`: runs of lines under heads, with a gap for the lines left
  out between two, and each finding as a note directly under the line it
  is about: `issue · major` in its tone (bad for a blocker and a major
  one, a warning below that), its first line, who filed it and how it
  stands. A run's head says what it is in as git names it — `@@ fn
  hear(&mut self)`, the words after the `@@` of `git diff <base> <head>
  -- <file>` for the hunk that has the run's first line — and where git
  has none, the line it begins at. A finding on no line shown is over
  them all. Findings resolved or closed are left out until `u` or `f`.
  Past 1500 lines the code ends in a gap that says how many more there
  are.

| key | does |
|---|---|
| `n` `N` | the keys on the next finding that is shown, or the one before, round again: those on the review as a whole first (the list `general`), then across files (the code's `selected`, and the message's `focus`) |
| `⏎` | on a note, or a finding on the review as a whole: opens it — all its words, the edit it suggests as a small code of the lines deleted and added, the replies with who and when, a text area and **Reply** (`⌘⏎` in the area: `sw reply <id> -m`), **Reply and resolve** while it stands (then `sw resolve <id>`; with no words, only that), **Reopen** once it does not (`sw reopen <id>`, with the words typed as its `-m`), and **Apply** while it suggests an edit no one has applied (`sw apply <id>`: sawa makes the edit in the working tree, and refuses where the lines changed since). On a file: the keys on its diff |
| `esc` | shuts what is open and keeps the words typed |
| `c` | on a line of the diff: a comment under it, filed by **Comment** as a finding on that line (`sw add -f <file> -l <line> --side new\|old -m`); `issue:` or `suggestion:` before the words says its type, as in sawa |
| `space` | marks the file under the keys read (`sw read -f`) and shows the next unread one; when every file is read the status says so and the verdicts come up with the keys on them |
| `e` | hands the file to the editor as [`mia files`](#mia-files) does — `$EDITOR +<line> <path>`, else `nvim`, run in the worktree's top folder, the view said again when it leaves — at the line of the file the keys are on in the diff (a deleted line's and a note's being the next line the file has), and at its first on its row in the list |
| `f` | which findings are shown: every open one, then only those an agent says it addressed and that wait on you, then the resolved too, and round again; the header's count, each file's, the notes and `n` follow |
| `g` | sawa's gates in a modal, `gated`, over the view, a list `gates`: each by its name with what must hold (`.sawa/gates.md`), how it was judged, by whom and when at its right, and why on a line under it. `⏎` on one shows what was judged, with a text area for why and **Pass**, **Warn** and **Fail**, which record its result (`sw gate <name> pass\|warn\|fail -m`): sawa lets a person record one as it lets an agent. `esc` goes back to the gates, and again to the summary |
| `v` | the verdicts in a modal, `judge`, over the view — approve, request changes, comment only, and none once one is given; `⏎` gives it (`sw verdict`), `esc` leaves it |
| `y` | copies to the clipboard (a patch's `copy`) the path of the file under the keys in the list, and on a finding — its note in the diff, or its row over the files — what it says |
| `o` `u` | reading order or by folder; resolved findings shown or not |
| `[` `]` | where sawa reviews a stack (`sw stack --json` says its layers): the layer under and the one over, each a review of its own (`sw open -r <parent>..<branch>`), this worktree's own being the one found as above; the header says `layer 2 of 3`, and the status says so at the bottom and at the top |
| `r` | has sawa look at the change again (`sw open` of the same target) |

Each file of the list carries a menu, opened beside it with `→` and
under the pointer's other button, each choice with the key that does the
same at its right: Mark read (where it is not; sawa has no word that
marks one unread), Open in editor, Copy path. A finding on the review
as a whole has Reply, Resolve while it stands or Reopen once it does
not — each with what is typed under it — and Copy its text. A finding
under a line of the diff has no menu: the window's protocol has none on
a line of code, and `⏎` opens it, as the code's `does` says in the foot.

What was just done — `reply sent`, `t3 resolved`, `t3 reopened`, `filed
t7`, `marked read: <path>`, the verdict as sawa words it, a gate's result,
`copied <path>` — is a toast, said once. The status is for what sawa
refused and for what is to be known of how things stand, and it and the
keys are said by a patch's `status` and `keys` where nothing else of the
view is another. The foot's keys are those of the piece the window says
the keys are on (`focus`): `c` only off the list of files.
Every `sw` is run with `-t <review>` and without `SAWA_AGENT`,
`SAWA_AUTHOR` and `SAWA_SNAPSHOT`, so what is written is the person's
whatever the pane's environment says. After each the review is read
again, and it is read again by itself when sawa's record of it changes —
the file `sw state` names as its `store` is looked at every 2 s — so a
finding an agent files or answers comes in while you read; the view is
said again only when it would say something else, and the keys stay on
the line they were on.

What sawa refuses — approving over a blocker, a line the file does not
have, an edit whose lines changed — is the view's status, cut to 200
characters, and the app stays up; so it does with no `sw` on `PATH` and
with nothing to review, which the view says, and `r` asks again. `q` or
`⌃C` in the plain terminal leaves, as closing the view does; stdin that
is no terminal is refused, exit 1.

Not yet: changing the words of a finding you filed (sawa has no verb for
it: `sw type` changes only its type), filing a finding on the review as
a whole, sawa's map and its patterns, and a pull request's comments on
GitHub.

## Setups

`mia api setups` is the setups the person has used — an agent, a model
and an effort, as one — each with how often and where last, for a picker
to offer the ones they use. It answers from any folder: in a repository,
that repository's; outside one, every repository's. `--repository <name>`
asks for the one named, by its main checkout's folder name, and `--all`
for every one.

```json
[{"agent":"claude","model":"opus","effort":"high","uses":7,"sessions":6,
  "last":{"repository":"mia-phone","worktree":"goba","machine":"local","at":"2026-11-15T10:00:00Z"},
  "usual":true},
 {"agent":"claude","model":"sonnet","effort":"medium","uses":2,"sessions":2,
  "last":{"repository":"mia-phone","worktree":"olasiti","machine":"fedora","at":"2026-11-12T16:20:00Z"},
  "usual":false},
 {"agent":"codex","model":"","effort":"low","uses":0,"sessions":0,"usual":false}]
```

| field | is |
|---|---|
| `agent`, `model`, `effort` | the setup; `""` for a model or an effort mia did not choose, so the agent's own — never both |
| `uses` | how many times an agent was started with it or switched to it |
| `sessions` | in how many worktrees |
| `last` | where it was used last: the `repository`, the `worktree`, the `machine` it runs on — `local` for this one — and `at`; left out for one never used |
| `usual` | the one used in the most of the repository's worktrees, the later used of a tie; none when every repository is asked |
| `role`, `body`, `account` | of a [role](worktrees.md#a-role): its name, the first message its agent is sent, and the account it starts on when it says one; left out of a setup that is no role |

The usual one comes first, then the last used first. After them come
what the config starts agents with, `[main]` and `[agents.<name>]`, that
were not used there, with `uses` 0: someone who has chosen nothing yet
still has those. One of them with no model is there for its effort; one
that says neither, or names an agent mia does not start with what it
says — cursor-agent, an effort of opencode's or gemini's, or one no
enabled plugin adds — is left out.

Last come the roles, by name, each with `uses` 0: the agent, model and
effort it starts, those it leaves out the default setup's, as `mia window
new <worktree> --role <role>` would start it. A role file that cannot be
read is left out; `mia role ls` says why. A mia has roles when its
capabilities have `roles`.

Every `mia window new … --model`, `mia window model`, `mia conversation
resume`, `mia main start` and `mia main model` writes down a use, in
`setups.json` in the user-wide config folder of the machine whose mia
started it; a start with no `--model` or `--effort` writes down what the
config started the agent with, and nothing when it says none. The last 500 are kept. The main
agent's are in the repository `main`, its folder's name, worktree `main`.
Each use also keeps the `account` the agent was started on, when it was
not the default, for the history: a setup is an agent, a model and an
effort, whatever account they were used on.

## Accounts

`mia api accounts` is every [account](worktrees.md#an-agents-account) of
every agent mia keeps accounts of, by agent, each agent's own first. It
answers from any folder, and starts and writes nothing. A capabilities
list with `accounts` has it.

```json
[{"agent":"claude","name":"default","home":"/Users/user/.claude","default":true,
  "logged_in":"unknown","sessions":["main","goba"],"user_default":false},
 {"agent":"claude","name":"work","home":"/Users/user/.config/mia/accounts/claude/work","default":false,
  "logged_in":"unknown","sessions":["olasiti"],"user_default":true},
 {"agent":"codex","name":"default","home":"/Users/user/.codex","default":true,
  "logged_in":"yes","sessions":[],"user_default":true},
 {"agent":"pi","name":"default","home":"/Users/user/.pi/agent","default":true,
  "logged_in":"no","sessions":[],"user_default":true}]
```

| field | is |
|---|---|
| `name` | `default` for the agent's own folder, or the name it was added under |
| `home` | its folder: the agent's own, as its variable or its usual place says, or mia's `accounts/<agent>/<name>`; empty for the default of a plugin's agent, whose folder mia does not know |
| `logged_in` | `yes` when the file the agent writes once signed in is there, `no` when it is not and the agent keeps a sign-in nowhere else — or nothing but its shared setup is in the folder yet — and `unknown` when mia cannot tell: claude on macOS, which keeps it in the Keychain, codex, which may keep it in a keyring, and a plugin's agent. Only whether a file is there is read |
| `shared` | what of the agent's own folder it links to, by name, as [its setup is shared](worktrees.md#its-setup-shared); empty for `default`, and for an agent mia shares nothing of |
| `sessions` | the worktrees with a window that runs the agent on it now — `main` for the main agent, `accounts` for a window signing in to it — on this computer |
| `user_default` | true of the one the user-wide config starts the agent's sessions on, `[agents.<agent>] account`, or of `default` when it names none; all of an agent's are false when it names one the agent has not. A repository can start its own on another: `--projects` says. A mia has it when its capabilities have `account-user-default`, which `mia account use <agent> <name> --global` sets |

`mia account add --json` and `mia account login --json`, which opens a window to
sign in to an account the agent has already — a mia has it when its
capabilities have `account-login` — say what they opened:

```json
{"account":{"agent":"claude","name":"work","home":"/Users/user/.config/mia/accounts/claude/work",
  "default":false,"logged_in":"no","sessions":[],"user_default":false},
 "worktree":"accounts","session":"mia-accounts","window":"claude-work","command":"CLAUDE_CONFIG_DIR='/Users/user/.config/mia/accounts/claude/work' claude",
 "shared":{"agent":"claude","name":"work","from":"/Users/user/.claude","linked":["settings.json","CLAUDE.md","skills"],"already":[],"kept":[]}}
```

`shared` is what was linked of the agent's own folder before the agent
started: `linked` now, `already` linked, and `kept`, a name the account
has of its own that mia links nothing over. `mia account share --json`
is a list of the same, one for each account.

`mia account signals <agent> [<name> | --all] [--dry-run] --json` — a mia
has it when its capabilities have `account-signals` — is a list, one for
each account, of what was done with the file that [makes pi say what it
does](tmux.md#pi-says-what-it-does):

```json
[{"agent":"pi","name":"default","file":"/Users/user/.pi/agent/extensions/mia-signal.ts","did":"written","dry_run":false}]
```

`did` is `written`, `updated` — mia's own, of another version —
`already`, or `kept`: a file there that is not mia's, left as it is. With
`dry_run` nothing was written, and `did` is what would be.

`mia api accounts --projects` is which account each repository starts
each agent's sessions on: every repository mia has named a worktree in,
the one asked from, and any `mia account use` named.

```json
[{"repository":"mia-desk","agent":"claude","account":"work","from":"mia"},
 {"repository":"mia-desk","agent":"codex","account":"default","from":"default"},
 {"repository":"mia-phone","agent":"claude","account":"personal","from":"config"}]
```

| `from` | is |
|---|---|
| `mia` | `mia account use` said it, kept in mia's own state |
| `config` | `[agents.<name>] account` in the repository's config, or the user-wide one |
| `default` | nothing says one: the agent's own folder |

Accounts are each machine's own: a folder there, signed in to there.
Each machine answers for itself — `mia api accounts` asked on it, over
ssh — and an app that shows them together keeps them apart by machine.
An account of one name on two machines is the same only by its name,
which is what `mia move` goes by.

## Standing

`mia api standing` is every agent in every repository `mia api repos`
lists, from any folder: which wait on you, which work, and which finished
with nobody having looked. Nothing is worked out anew: it is each
worktree's best [signal](tmux.md#signals), as the radar reads them.

```json
{"waiting":[{"repository":"mia-desk","root":"/Users/user/src/mia-desk","worktree":"goba",
   "machine":"local","agent":"claude","window":{"index":1,"name":"claude"},
   "kind":"asks","line":"wants to use Bash",
   "options":["Yes","Yes, and don't ask again for mia commands","No"],"yes":"1",
   "since":"2026-11-02T10:00:00Z"}],
 "working":[{"repository":"mia-desk","root":"/Users/user/src/mia-desk","worktree":"kijenge",
   "machine":"local","agent":"claude","kind":"busy","line":"","since":"2026-11-02T10:01:00Z"}],
 "done_unread":[{"repository":"api","root":"/home/user/src/api","worktree":"olasiti",
   "machine":"fedora","agent":"codex","kind":"done","line":"","since":"2026-11-02T09:40:00Z"}],
 "counts":{"waiting":1,"working":1,"done_unread":1},
 "machines":[{"name":"local","answers":true,"reach":{"state":"ok"}},
   {"name":"fedora","answers":true,"reach":{"state":"ok","warnings":[
     {"name":"linger","detail":"user services stop at logout, and the agents with them","fix":"loginctl enable-linger"}]}},
   {"name":"box","answers":false,"detail":"box: ssh: connect to host box port 22: Operation timed out",
    "reach":{"state":"unreachable","detail":"box cannot be reached over ssh: ssh: connect to host box port 22: Operation timed out",
      "fix":"turn box on, or check its address with ssh box"}}]}
```

| group | holds the worktrees whose best signal |
|---|---|
| `waiting` | calls for you — ranked 50 or below — and stays until it is answered: an agent's `asks` |
| `done_unread` | calls for you and lasts `--until seen`: `done`, `failed` |
| `working` | does not call for you and was set by an agent: claude's `busy` |

A worktree is in one group at most, and in none when it has no signal or
only a note. Each group has the oldest first.

| field | is |
|---|---|
| `repository`, `root` | the repository's name and its main checkout on its machine, where a verb about the worktree is run |
| `worktree`, `machine` | the worktree's name, and the machine whose mia keeps it: `local` is the one asked |
| `agent` | the agent that set the signal, or the one in the window it was set from; empty when something else set it — a test watcher, you |
| `window` | the window it was set from, where an answer is typed; left out when it was set from none |
| `kind`, `line` | the signal's own: what its setter called it, and the line that says why |
| `options` | of a waiting one, the numbered choices its window shows, the first being 1; left out when it shows none, and for one whose window is not known |
| `yes` | the keys its setter says approve it, pressed with no Enter |
| `since` | when the signal was set |

Every machine `mia machine list` has is asked for its own, at once, by
its own mia, and its entries carry its name. `machines` names each one
asked: one that did not answer — it is off, or its mia is older than this
query — has `answers` false and why, and no entries; that is not an
error. One that has not answered in 8 seconds has not answered, and
does not hold up the rest. It is not asked again for 30 seconds, by this
mia or another. `--here` asks no machine.

`reach` says what a machine's not answering means: a `state`, a `detail`
of one line, and the `fix`, the one command or step that puts it right —
left out when mia knows none. Capability `machine-reach`; a machine in
`mia api machines` and in `mia machine ls --json` has the same `reach`.

| `state` | is | `fix` |
|---|---|---|
| `ok` | it answered | |
| `unreachable` | ssh could not connect: it is off, its name does not resolve, it timed out | turn it on, or check its address |
| `needs_login` | ssh got there and was not let in: a key refused, a host key not accepted, a passphrase or password wanted. mia never asks for one | `ssh <target>` once in a terminal |
| `no_mia` | it answers and has no mia at `~/.local/bin/mia` | `mia machine ship <name>` |
| `old_mia` | its mia answers and does not know the query; `detail` says its version | `mia machine ship <name>` |
| `not_set_up` | its mia runs and the repository asked about is not there: in `mia api machines`, of a machine linked to the repository. It has `answers` true | `mia machine link <name>` |
| `failed` | its mia answered with an error that is none of these; `detail` has its line | |

One that is `unreachable` or `needs_login` twice running is asked once a
minute from then on, not every 30 seconds, until it answers. `warnings`
are what works now and will not go on working, each a `name`, a `detail`
and a `fix`: `linger`, on a Linux machine whose user services stop when
you log out (`loginctl show-user` says `Linger=no`). Each machine's own
mia looks, at most every ten minutes, and says it of itself, so it costs
no asking.

`--seen <repository>/<worktree>` first marks that worktree seen, as
switching to it does, so what it finished is unread no more; the
repository by its name or its `root`, and `<worktree>@<machine>` for
another machine's. The answer is the standing after.

`mia dev <worktree> --json` reports the new lent state; `mia dev off --json`
reports `{"lent": false}`. `--follow` cannot be combined with `--json`.

Examples of placement and gateway state:

```json
{"worktree":"monduli","machine":"build","staging":"/home/me/.mia/shop/monduli"}
```

```json
{"running":true,"address":"127.0.0.1:7842","pac":"http://127.0.0.1:7842/proxy.pac","ca":"/home/me/.config/mia/gateway/ca.pem","routes":["monduli.mia"]}
```

## Exit status

A command that runs another command preserves its exit status where possible,
including `mia run`, `mia env run`, `mia env exec` and interactive shells.
Mia itself uses `1` for failure, `3` for a missing worktree or query and `64`
for usage errors.

## Plugin name collisions

A verb declared by two enabled plugins is refused until all but one are
disabled. A verb that shadows a core command is also reported by
`mia plugin ls` and `mia doctor`.
