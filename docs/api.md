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
`capabilities`, `machines`, `here`, `repos`, `main` and `standing` answer
from any folder; the other queries are about a repository and need one. Check capabilities before using a feature. Existing response shapes are not
wrapped in a version envelope.

## JSON commands

| command | returns |
|---|---|
| `mia api <query>` | worktrees, machines, here, repositories, environments, stacks, base, conversations, the main agent and standing |
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
the agents' own transcripts, so nothing is recorded as they work.

```json
[{"id":"23f98c02-c52b-4de9-8c69-b322eb153bff","agent":"claude",
  "cwd":"/Users/user/src/app.monduli","worktree":"monduli","gone":false,
  "asked":"make the list faster","said":"It is faster now: one read for each file.",
  "started":"2026-11-02T10:00:00Z","active":"2026-11-02T10:41:07Z","turns":4,
  "running":true,"window":{"index":2,"name":"claude"}}]
```

| field | is |
|---|---|
| `agent` | whose it is: `claude`, `codex`, `pi`, or the name of an agent a plugin adds |
| `worktree` | the worktree `cwd` is or is in; a removed one by what followed the main checkout's path; empty when none of this repository's |
| `gone` | the folder is no more |
| `asked`, `said` | the first thing the person asked and the last thing the agent said, each on one line, cut at 200 characters |
| `started`, `active` | when the first thing in it was said and when it was last written to, both in UTC as RFC 3339, to the millisecond at most |
| `turns` | how many things the person asked |
| `running`, `window` | a window of that worktree's session runs this agent with this conversation; `window` is left out otherwise |

Which conversation a window has open, claude says itself. Of any other
agent it is the newest of that agent's in the worktree, so two of one
agent in one worktree are told apart only by which was written to last.

`mia api conversation <id> [--after <n>]` is one conversation as turns, in
order, each with a number `n` that only grows. `--after` gives the turns
with a greater `n`: a view that polls asks for what came after the last it
has, and mia reads only what was written since.

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
mentions the text with its `agent` and `n`, the turn to open it at; `n` is
0 for words that were a subagent's.

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
| `agent` | the one that runs; when none does, the one `[main] agent` names |
| `machine` | `local`: the machine that was asked, as `mia api machines` calls it |
| `home`, `session` | its folder and its tmux session, whether or not they are there yet |
| `window` | the agent's window; left out when it is not running |
| `conversation` | the one it has open — claude says which — or, of another agent and when it is not running, the last written to in its folder; empty when there is none, or mia cannot read that agent's |
| `working` | it is doing something now: claude's pane title says so, and of another agent, that its window wrote in the last three seconds |
| `since` | when its session was started; left out when it is not running |

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
 "machines":[{"name":"local","answers":true},{"name":"fedora","answers":true},
   {"name":"box","answers":false,"detail":"box: ssh: connect to host box port 22: Operation timed out"}]}
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
error. It is not asked again for 30 seconds. `--here` asks no machine.

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
