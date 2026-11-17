# The main agent

One agent you talk to, which runs the others: it gives a task a worktree
and an agent of its own, tells an agent something, answers what it may,
brings back what waits on you, and says where things stand. You have one,
on the machine that is always on, and it is no repository's.

```bash
mia main start        # its folder and session when they are not there, and the agent in it
mia main start --model opus --effort high   # started with a model and an effort
mia main start --agent codex                # started as another agent, this once
mia main start --new                        # started with a new conversation, not its last
mia main start --new --restart              # the one that runs ended, and started again with a new one
mia main start --resume <id>                # started going on with one of its earlier conversations
mia main              # whether it runs, and what it is doing
mia main say start goba on the login bug, and tell me when it asks something
mia main press 2      # a key with no Enter: pick an option when it asks you to choose
mia main model sonnet # switch it to another model while it runs
mia main interrupt    # stop it mid-work
mia main stop         # end its session; its folder and conversations stay
```

Each answers from any folder. `mia main start` twice starts one: an agent
that already runs is left as it is. Stopped and started again, it goes on
with its last conversation, unless [`--new`](#a-new-conversation) says
not to.

`mia main start` watches the agent for up to three seconds before it says
it started: until it asks something, ends, or has drawn the same for a
second. One that asks first is said to — claude, in a folder it has not
seen, asks whether you trust it — and one that ended is said to have, with
the last lines it printed, exit 1, its window closed.

## Where it lives

| | |
|---|---|
| its folder | `~/.config/mia/main`, which mia makes and no repository holds |
| its session | `mia-main` in tmux, with the agent in a window named for it: `tmux attach -t mia-main` |
| its name | `main` is taken in the name registry, so no worktree can be given it and its session |
| its environment | `MIA_MAIN=1`, and no `MIA_WORKTREE`: it is no worktree |
| its notes | `CLAUDE.md` for claude, `AGENTS.md` for another, written by mia at every start: who it is, the verbs it runs the others with, and what it may answer |

The window is the agent and goes when the agent ends, so a window named
for an agent is an agent running. `mia resurrect` does not bring the
session back after tmux goes down; `mia main start` does.

## On which machine

It lives on one machine and is reached from the others. On each of the
others, the user-wide config names the machine, as `mia machine list`
does:

```toml
[main]
machine = "fedora"
```

Every `mia main …` there, and `mia api main`, `mia api main conversations`
and `mia api main turns`, is
then run by fedora's own mia over ssh, and what it says is printed:
`mia main say` from a laptop types into the agent on fedora. A machine
that does not answer is an error that names it. Left out, or `local`, the
main agent is this machine's, and so it is on the machine itself.

## Which agent

claude, unless the user-wide config says another:

```toml
[main]
agent = "codex"
```

Any agent mia knows can be the main one, one a [plugin adds](plugins.md#an-agent)
too. How it is started, how it goes on with a conversation, which file it
reads its notes from and which key stops it are that agent's own, kept
where mia keeps everything it knows of an agent. An agent mia cannot read
the conversations of is started anew each time, and has no turns to show.

`mia main start --agent <name>` starts another agent of those mia knows,
for this start: `[main] agent` stays as it is, and the next `mia main
start` with no `--agent` starts that one again. It goes on with its own
last conversation in the folder, and reads its own notes file, which mia
writes as it does at every start. While one runs it is refused: `mia main
stop` first. Its model and effort are its own under `[agents]`, or those
`--model` and `--effort` say: `[main] model` and `effort` are for the
agent `[main]` names. From another machine, it is that machine's mia's,
as every `mia main …` is.

## Its model

It is started with `[main] model` and `[main] effort`, or its agent's own
under [`[agents]`](configuration.md#agents), unless `mia main start
--model <model> --effort <effort>` says others; a model or an effort its
agent does not have is refused, with those it has. One that runs already
is not started again with them: `mia main model <model> [--effort
<effort>]` switches it, pasting into it what its agent takes — claude's
`/model sonnet` and `/effort high` — each with Enter after. An agent that
cannot be switched while it runs is refused, to be restarted with
`--model`; mia does not restart it. From another machine, both are that
machine's mia's, as every `mia main …` is.

`mia api main` says the `model` and `effort` mia started it with or last
switched it to: not read from the agent, so one switched by hand in it is
not known.

## Its account

It is started on `[main] account`, or its agent's own under
[`[agents]`](configuration.md#agents), or the agent's own folder, unless
`mia main start --account <name>` says another of the
[accounts](worktrees.md#an-agents-account) `mia account ls` lists. It
goes on with the last conversation of its folder on that account. One
that runs already is refused another: an agent takes its account when
it starts, so `mia main stop` first. `mia api main` says the `account`
it runs on.

## A new conversation

`mia main start --new` starts it going on with nothing: its agent is run
as it is the first time, with the model, the effort and the account any
start gives it. The conversation it had is not touched: it stays where
its agent keeps them. The next plain `mia main start` goes on with the
last written to, which is the new one once something is said in it.

One that runs is refused, exit 1, and nothing is stopped or started. The
line says what ending it would cut short:

```
mia: the main agent is running — `mia main start --new --restart` ends its conversation and starts a fresh one
mia: the main agent is working — `mia main start --new --restart` stops it mid-turn, ends its conversation and starts a fresh one
mia: the main agent asks something: Do you want to proceed? — `mia main start --new --restart` leaves it unanswered, ends its conversation and starts a fresh one
```

`mia main start --new --restart` is that said in so many words: the one
that runs is stopped, working or asking as it may be, and started again
with a new conversation, as the agent it was and with the model, the
effort and the account it had, unless `--agent`, `--model`, `--effort` or
`--account` say others. Another agent named takes none of them: its own
are as at any start. What it is to be started with is checked before
anything is stopped, so a model its agent has not leaves it running.
`--restart` with no `--new` is not a verb: `mia main stop` and `mia main
start` go on with the same conversation.

`mia api main` says the new one as `conversation`: at once for claude,
which says which it has open, and for another agent empty until it has
written the first of it, never the one it left. A mia starts one when its
capabilities have `main-start-new`.

## Earlier conversations

The conversations it left stay in its agents' folders, and are the main
agent's own: those made in its folder, with every agent it was run as.

```
mia api main conversations                  # each of them, the last written to first
mia api main turns --conversation <id>      # one of them as turns
mia main start --resume <id>                # started going on with that one
mia main start --resume <id> --restart      # the one that runs ended first
```

`mia main start --resume <id>` starts it going on with that conversation,
as the agent that had it — whatever `[main] agent` says — and on the
account it is kept on. The id is one `mia api main conversations` lists,
and no other: a repository's conversation is refused, and so is
`--agent` or `--account` naming another than the conversation's own, and
an agent mia does not know how to take up again by an id:

```
mia: no conversation 5f0c of the main agent's — `mia api main conversations` lists them
mia: conversation 5f0c is claude's, not codex's — it is gone on with as claude
mia: conversation 5f0c is kept on claude's account default, not work — it is gone on with there
mia: mia does not know how gemini takes a conversation up again by its id
```

One that runs is refused as for a new conversation, and `--restart` ends
it in the same way, with the model and the effort it had when it is the
same agent:

```
mia: the main agent is running — `mia main start --resume 5f0c --restart` ends its conversation and goes on with that one
```

The conversation it had is not touched, and is among the earlier ones
from then on. `--new` and `--resume` together are not a verb. A mia has
these when its capabilities have `main-conversations`.

## What it may answer

Its notes say it in a few lines. A permission prompt for work the task
already allows: yes. Never an option that answers later prompts too. A
product choice, a change of scope, anything destructive: it asks you, and
says which agent waits.

Your own answer to another agent does not go through it:
`mia window send <worktree> <window> -- <text>` types into that agent's
window, and `mia window press <worktree> <window> -- 1` picks an option,
the window by its name or its number as `mia api standing` and
`mia api conversations` give it.

## Saying something to it

`mia main say <text…>` pastes the text into the agent as one message —
a line break in it is not an Enter — and presses Enter after. The text is
taken as it is typed: `--json` or `-h` in it are words. `mia main
interrupt` presses Escape, or the agent's own key for stopping.

`mia main press <keys…>` types its keys into the agent's window as they
are, with no Enter after and not as a paste, as `mia window press` does
in a worktree's window: `mia main press 1` picks the first option when
the main agent itself asks you to choose. An app presses the digit; it
does not say it. With no main agent running it is refused, as `say` is.

## When it asks first

While the agent's screen shows a question with options — claude's trust
in a new folder, a permission, a choice it puts to you — anything typed
into it answers it, Enter above all: claude's trust question has
**No, exit** chosen, and an Enter there ends it. So `mia main say` and
`mia main model` type nothing then, and say what it asks, exit 1:

```
mia: claude asks first: Is this a project you created or one you trust? — answer it: `mia main press <n>` (1 No, exit · 2 Yes, I trust this folder)
```

mia never answers it for you. `mia main press <n>` does, as you say: a
digit, where the agent numbers its options; where it draws them with no
numbers and takes no digit, as claude's trust question, mia moves the
cursor to option n and presses Enter once it sees the cursor there, and
presses nothing more when it does not. A question is a run of options at
the foot of the screen with the agent's cursor on one, and, of those with
no numbers, a hint under them such as `Enter to confirm`.
`mia window send` into a worktree's window is refused the same way, and
`mia window press <worktree> <window> -- <n>` answers it.

With no main agent running, `say`, `press`, `model` and `interrupt` say
that it is not running and that `mia main start` starts it, and so does
one that ends as it is typed into.

## For an app

`mia api main` is where it stands, `asks` among it when it asks
something, `mia api main turns` its conversation
with what it did as lines of their own, `mia api main conversations` the
[earlier ones](#earlier-conversations), and `mia api standing` every
agent in every repository: see [Automation](api.md#the-main-agent).
