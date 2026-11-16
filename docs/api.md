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
`capabilities`, `machines`, `here` and `repos` answer from any folder; the
other queries are about a repository and need one. Check capabilities before using a feature. Existing response shapes are not
wrapped in a version envelope.

## JSON commands

| command | returns |
|---|---|
| `mia api <query>` | worktrees, machines, here, repositories, environments, stacks, base and conversations |
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
| `active` | when it was last written to |
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
