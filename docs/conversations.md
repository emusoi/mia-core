# Conversations

A conversation is what was said to an agent in a worktree, kept where the
agent keeps it: claude's, codex's, pi's, opencode's and agy's transcripts,
and those of any agent a [plugin adds](plugins.md#an-agent). mia records
nothing as you work. It reads those transcripts to find a conversation
again, take it up, or hand it to another agent.

## Finding one

```bash
mia recall retry loop         # the conversations of this repository that mention it
mia recall --all retry loop   # every project, on this computer and every machine
```

```
2026-11-24 16:02  shop.monduli
  you: …why does the retry loop in the uploader never give up…
  cd /Users/you/src/shop.monduli && claude --resume 23f98c02-c52b-4de9-8c69-b322eb153bff
```

Each found one is the folder it was had in, where the words came up —
your own words over the agent's — and the line that goes on with it. The
ten newest come first. The words are looked for as they are typed, in any
case: a phrase, an error message, a name. Run outside any repository,
`mia recall` looks everywhere, as `--all` does.

`--all` also asks each [machine](machines.md) for its own, giving each 3
seconds: one that is off, or whose mia is too old, adds nothing and holds
nothing up. A conversation on a machine says `on <machine>` after its
folder, and its line is to be run there. In tmux, any words typed into
[the one list](tmux.md#the-one-list) are the same search, and Enter
resumes the one chosen, a machine's in a window here named
`worktree@machine`.

What was asked and said in each conversation is kept in a small index per
repository, `.git/mia/cache/recall.gob`, read on only from where a
conversation was written to since, so a search takes a fraction of a
second. Deleting it loses nothing: it is read again from the transcripts.

```bash
mia why internal/app/app.go:212   # the conversation behind this line's last commit
mia why 882b0e5                   # or behind a commit
```

`mia why` reads claude's transcripts for the conversation that made the
commit: what you asked before it, and the `claude --resume` line.

## Going on with one

```bash
mia conversation resume 23f98c02-…              # a window where it was had, the agent taking it up
mia conversation resume 23f98c02-… --in monduli # or in another worktree
mia conversation resume 23f98c02-… --model sonnet --effort high --account work
```

This opens a window in the worktree's session running the agent's own
resume — `claude --resume <id>`, `codex resume <id>`, `pi --session <id>`,
`opencode --session <id>`, `agy --conversation <id>` — started with a
model, an effort and an [account](agents.md#an-agents-account) as `mia
window new` starts one. A conversation a window already has open is left as
it is, and said so.

claude and pi keep a conversation under its folder's name, so for another
worktree mia first copies it under that folder's name, its paths
rewritten; the first stays where it was. A conversation is copied the same
way into another account's folder. From then on it is in both, each going
its own way: `mia conversation resume <id>` takes up the one last written
to, and `--worktree <name>` the one in that worktree.

`mia api conversations` lists every conversation with its id, and `mia api
conversation <id>` reads one as turns: see
[Automation](api.md#conversations).

## With another agent

```bash
mia hand codex                                                # the window in front's conversation, on with codex
mia conversation continue 23f98c02-c52b-4de9-8c69-b322eb153bff --with codex
mia conversation continue --from claude --with pi             # the one window claude has open, here
mia conversation continue 23f98c02-… --with opencode --dry-run   # only says: the file, the window, the message
```

No agent loads another's conversation: each keeps its own format. So mia
writes the conversation out as a handoff,
`.git/mia/handoffs/<worktree>/<agent>-<id>.md` in the repository's git
folder — the worktree stays as it was — and opens a window in that
worktree's session with the other agent, as `mia window new <worktree>
<agent>` starts one, with `--model`, `--effort` and `--account` or the
configured ones. Once the agent is ready it is sent one message: read that
file, go on from where it stops, and look at the worktree before changing
anything.

```
monduli: claude's conversation 23f98c02-…, 41 turns, written out to /Users/you/src/shop/.git/mia/handoffs/monduli/claude-23f98c02-….md
monduli: window codex opened — codex
monduli: window codex was told to read it and go on from where it stops
not carried over: what claude thought and what came back from its tools — codex has what was said and each thing done, in a line
```

Which conversation is said exactly: by its id, as `mia recall` and `mia
api conversations` give it, or by the window that has it open. A worktree
can have three open at once, and none is taken for the one meant.
`--from <window>` names the window by its name or number, in the session
of the worktree you are in or of `--worktree`; `mia hand` takes the window
you are in, or `--window <index>`. When that window has one conversation
open it is that one; otherwise the command is refused and lists the ids
open there. One copied to another worktree is in both under one id, so
`--worktree <name>` says whose.

In tmux, `> codex` typed into [the one list](tmux.md#the-one-list) does
this for the window you are in, and `> opencode son` lists it on each
model the words name.

### What a handoff loses

A handoff carries less than the conversation was. It says where it came
from and when, then the conversation in order: what you and the agent
said, as written, and each thing the agent did as one line — `Ran go test
./...`, `Edited view.lua +2 −1` and the file. What came back from a tool is
not in it, and nothing the agent only thought: the other agent reads the
files and runs the commands again for that, as its last line says.

A message over 6,000 characters is cut, and says by how much. A
conversation over 60,000 characters keeps the first thing you asked and
the last turns, as many as fit; the turns between are one line saying how
many are left out and naming `<agent>-<id>-full.md` beside it, which has
every turn, for the agent to look up what it needs. These files hold what
you typed, can be read by you alone, and are written again each time. The
other agent knows what the file says and what the worktree shows, no more.

So with the same agent nothing is handed over: `--with` the agent the
conversation is already with is refused, and says what goes on with
nothing lost — `mia conversation resume <id>`, with `--model` for another
model, or `mia window model` for the window that has it open.

Handed back to the agent it came from, that agent takes its own earlier
conversation up again, with all it knew, and is handed only what was said
since.

Each handoff is also kept as `<agent>-<id>.<when>.json` beside it: which
conversation went on with which agent, in which window, when. From then
on `mia api conversations` says the two are [one
thread](api.md#conversations): `continues` on the new one, `continued_by`
on the first.

The agent is typed into as [a role's opening](agents.md#a-role) is: never
into a question it asks first, and never into the shell left by one that
ended as it started. Then the window is open, the command fails, and the
message is printed for you to say there. A model or an effort the agent
does not have is refused before anything opens.

The [main agent](main.md#another-agent-mid-conversation) and
[chats](chats.md) go on with another agent the same way.

## When its worktree is gone

A conversation whose worktree was removed is still found and read: `mia
recall` says `its folder … is there no more` in place of the line that
resumes it, and `mia api conversation <id>` reads it. Its agent takes a
conversation up only in its folder, so `mia conversation resume` refuses
it and says how to bring the worktree back — `mia unarchive <name>` for
one archived, `mia new <branch>` for its branch — or takes it up in
another with `--in <worktree>`.

With `--all`, a conversation that went with its worktree to a machine is
offered where it is now, not where its folder was. A worktree whose session
runs on a machine, beside its [environment](environments.md#other-machines),
neither resumes nor hands on a conversation kept here: use a worktree that
is here.

## For agents: mia mcp

```bash
claude mcp add mia -- mia mcp
```

`mia mcp` is an MCP server on stdio that gives an agent the same search.
Add it to an agent by the agent's own means, as above for claude; mia
does not change any agent's settings. It has two tools, and changes
nothing:

| tool | takes | gives |
|---|---|---|
| `recall` | `text`, `limit` (10) | conversations in every project, on this computer and every machine, that mention the words: project, agent, date, the line, and the `root`, `id` and `turn` to read one by |
| `conversation` | `root`, `id`, `turn` | forty turns of what you and the agent said, around that turn or at the end, with no tool output; a message over 1,500 characters is cut |

A conversation found on a machine has no `root` or `id` to read it with
from here.
