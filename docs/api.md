# Automation

`mia` commands are the interface for scripts, `mia-ui` and plugins. Read
commands that support structured output take `--json`; `mia api` always emits
JSON.

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
| `mia api <query>` | worktrees, machines, here, repositories, environments, stacks, base, conversations, the main agent, standing, agents and apps |
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
| `running`, `window` | a window of that worktree's session runs this agent with this conversation; `window` is left out otherwise |
| `model`, `effort` | of a running one, what mia started its window's agent with or last switched it to — see [Agents](#agents); left out when mia chose none |
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

`mia api conversation <id> [--worktree <name>] [--after <n>]` is one
conversation as turns, in order, each with a number `n` that only grows.
`--after` gives the turns with a greater `n`: a view that polls asks for
what came after the last it has, and mia reads only what was written
since. Of a conversation that is in several worktrees, `--worktree` reads
the copy in the one named, as the list names it; with none it is the last
written to, the first of them in the list.

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

`mia api main turns [--after <n>]` is that conversation as
[`mia api conversation`](#conversations) gives one, an empty list when
there is none, with one kind more:

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
  "model":"opus","effort":"","live":true,"account_env":"CLAUDE_CONFIG_DIR"},
 {"name":"opencode","models":[],"efforts":[],"model":"","effort":"","live":false,"account_env":""}]
```

| field | is |
|---|---|
| `models` | what it can be started with: `id` as the agent takes it, `label` to show. Empty for an agent mia knows none of |
| `efforts` | its effort levels, how hard it thinks; empty when it has none mia knows |
| `model`, `effort` | what the config starts it with, `[agents.<name>]`; empty when it says none, and the agent's own default is used |
| `live` | it can be switched to another model while it runs |
| `account_env` | the variable that names the folder of each of its [accounts](#accounts); empty for an agent mia keeps no accounts of |

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
A mia that can is one whose `mia api capabilities` has `resume-model`.

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
that says neither, or names an agent mia does not start with a model and
an effort — opencode, or one no enabled plugin adds — is left out.

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
