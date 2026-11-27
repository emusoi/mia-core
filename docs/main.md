# The main agent

The main agent is the one agent you talk to. It runs the others: it gives a
task a worktree and an agent of its own, tells an agent something, answers
what it may, brings back what waits on you, and says where things stand.
You have one. It lives on one machine, in a folder of mia's that no
repository holds.

```bash
mia main start                              # start it, going on with its last conversation
mia main                                    # whether it runs, and what it is doing
mia main say start goba on the login bug, and tell me when it asks something
mia main press 2                            # a key with no Enter: pick an option it puts to you
mia main model sonnet --effort high         # switch its model while it runs
mia main switch --agent codex               # go on with another agent, the conversation carried over
mia main interrupt                          # stop it mid-work
mia main stop                               # end its session; its folder and conversations stay
```

Every `mia main` verb works from any folder. The main agent is also the
chat named `main` among the [chats](chats.md), which are plain
conversations with any agent, each one a main agent of its own.

## Starting it

`mia main start` makes its folder and session when they are not there and
starts the agent in it. Stopped and started again, it goes on with its last
conversation. When one already runs, `mia main start` leaves it as it is.

```bash
mia main start --model opus --effort high   # with a model and an effort
mia main start --agent codex                # as another agent, this once
mia main start --role reviewer              # as a role says, and sent the role's words
mia main start --new                        # with a new conversation
mia main start --resume <id>                # going on with one of its earlier conversations
```

mia watches the agent for up to three seconds before it says it started:
until it asks something, ends, or has drawn the same screen for a second.
One that asks first is said to ([When it asks first](#when-it-asks-first)).
One that ends at once is said to have, with the last lines it printed, and
its window is closed, exit 1.

`mia main` alone says where it stands:

```
claude is the main agent, idle, in window claude of mia-main since Nov 2 10:00
/Users/user/.config/mia/main
```

## Where it lives

| | |
|---|---|
| its folder | `~/.config/mia/main` (under `$XDG_CONFIG_HOME/mia` when that is set) |
| its session | `mia-main` in tmux, the agent in a window named for it: `tmux attach -t mia-main` |
| its name | `main` is held in the name registry, so no worktree is given it |
| its environment | `MIA_MAIN=1`, and no `MIA_WORKTREE`: it is no worktree |
| its notes | `CLAUDE.md` for claude, `AGENTS.md` (or the agent's own notes file) for another, written by mia at every start: who it is, the verbs it runs the others with, and what it may answer |

The window is the agent and closes when the agent ends, so a window named
for an agent means the agent runs. `mia resurrect` does not bring the
session back after tmux goes down; `mia main start` does.

## On which machine

It lives on one machine and is reached from the others. On each of the
others, the user-wide config names that machine as `mia machine list`
does:

```toml
[main]
machine = "fedora"
```

Every `mia main …`, `mia chats …` and `mia api main …` there is then run by
fedora's own mia over ssh, and what it says is printed: `mia main say` on a
laptop types into the agent on fedora. A machine that does not answer is an
error that names it. Left out, or `local`, the main agent is this
machine's. Everything named on the other machine, a role or an account, is
that machine's.

## Which agent

claude, unless the user-wide config names another:

```toml
[main]
agent = "codex"
```

Any agent mia knows can be the main one, one a [plugin adds](plugins.md#an-agent)
too. An agent whose conversations mia cannot read starts with a new
conversation each time, and has no turns to show.

`mia main start --agent <name>` starts another agent for this start only:
`[main] agent` is not changed, and the next plain `mia main start` starts
that one again. It goes on with its own last conversation in the folder.
While an agent runs, `--agent` is refused: `mia main stop` first.

`mia main start --role <name>` starts it as a [role](agents.md#a-role)
says: the role's agent, model, effort and account, the `[main]` ones where
the role names none, and flags given on the line over both. Once started
it is sent the role's words, as `mia main say` sends them. A role opens a
conversation, so a main agent that runs is refused: `mia main stop` first,
or `mia main start --new --restart --role <name>`.

To change agent for good, mid-conversation, see the next section.

## Another agent mid-conversation

`mia main switch --agent <name> [--model <model>] [--effort <effort>]`
has it go on with another agent. No agent can load another's conversation,
so mia writes what was said out as a handoff in its folder, under
`.mia/handoffs/`: who said what, and each thing done, a line each. It stops
the agent it was with, starts the other in its place with a new
conversation, and puts the handoff in front of the next thing said to it,
by `mia main say` or from an app.

```
the main agent goes on with codex, 12 turns with claude carried over in front of the next thing said: /home/user/.config/mia/main/.mia/handoffs/main/claude-c1.md
```

From then on it is that agent: started again, it goes on with that agent's
conversation, with the model and effort it was switched with. Switched back
to an agent it went on from, that agent takes its own conversation up
again and is handed only what was said since. With nothing said yet, the
other agent is only started.

It is not switched while it asks something or is at work, and says so:
`mia main interrupt` stops it. Switching to the agent it already runs as,
with a model or an effort, is `mia main model`.

## Its model

It starts with `[main] model` and `[main] effort`, or its agent's own under
[`[agents]`](configuration.md#agents), unless `mia main start --model
<model> --effort <effort>` says others. A model or an effort its agent does
not have is refused, with those it has.

`mia main model <model> [--effort <effort>]` switches one that runs. mia
types what the agent takes, as claude's `/model sonnet` and `/effort high`,
each with Enter after:

```
the main agent is switched to sonnet, effort high
```

An agent that cannot switch while it runs is refused; restart it with
`--model`. mia does not restart it for you. `mia main start --model` on one
that runs is refused and points at `mia main model`. mia knows the model
and effort it started it with or last switched it to, not one you switched
by hand in the agent.

## Its account

It starts on `[main] account`, or its agent's own under
[`[agents]`](configuration.md#agents), or the agent's own folder, unless
`mia main start --account <name>` names another of the
[accounts](agents.md#an-agents-account) `mia account ls` lists. It goes on with
the last conversation of its folder on that account. An agent takes its
account when it starts, so one that runs is refused another: `mia main
stop` first.

## A new conversation

`mia main start --new` starts it with no conversation to go on with, with
the model, effort and account any start gives it. The conversation it had
is not touched; it stays where its agent keeps it. The next plain `mia main
start` goes on with the last one written to, which is the new one once
something is said in it.

While one runs, `--new` is refused, exit 1, and nothing is stopped. The line
says what ending it would cut short:

```
mia: the main agent is running — `mia main start --new --restart` ends its conversation and starts a fresh one
mia: the main agent is working — `mia main start --new --restart` stops it mid-turn, ends its conversation and starts a fresh one
mia: the main agent asks something: Do you want to proceed? — `mia main start --new --restart` leaves it unanswered, ends its conversation and starts a fresh one
```

`mia main start --new --restart` stops the one that runs, working or asking
as it may be, and starts it again with a new conversation, as the same
agent with the model, effort and account it had, unless flags say others.
Another agent named takes its own, as at any start. What it is to start
with is checked first, so a bad model leaves the running one as it was.
`--restart` alone is not a verb: `mia main stop` and `mia main start` go on
with the same conversation.

## Earlier conversations

The conversations it left stay where its agents keep them. Those made in
its folder, with every agent it ran as, are the main agent's own.

```bash
mia api main conversations                  # each of them, the last written to first
mia api main turns --conversation <id>      # one of them as turns
mia main start --resume <id>                # start going on with that one
mia main start --resume <id> --restart      # the one that runs ended first
```

`mia main start --resume <id>` goes on with that conversation as the agent
that had it, whatever `[main] agent` says, and on the account it is kept
on. The id has to be one `mia api main conversations` lists:

```
mia: no conversation 5f0c of the main agent's — `mia api main conversations` lists them
mia: conversation 5f0c is claude's, not codex's — it is gone on with as claude
mia: conversation 5f0c is kept on claude's account default, not work — it is gone on with there
mia: mia does not know how cursor-agent takes a conversation up again by its id
```

One that runs is refused as for `--new`, and `--restart` ends it the same
way. The conversation it had becomes one of the earlier ones. `--new` and
`--resume` together are refused.

## Saying something to it

`mia main say <text…>` pastes the text into the agent as one message (a line
break in it is not an Enter) and presses Enter after. The text is taken as
typed: `--json` or `-h` in it are words.

`mia main press <keys…>` types keys into its window with no Enter after and
not as a paste: `mia main press 1` picks the first option when it asks you
to choose. `mia main interrupt` presses Escape, or the agent's own key for
stopping.

With no main agent running, `say`, `press`, `model` and `interrupt` say so,
and that `mia main start` starts it. The same is said when it ends as it is
typed into.

## When it asks first

While the agent's screen shows a question with options (claude's trust
question in a new folder, a permission, a choice it puts to you), anything
typed answers it, and Enter above all: claude's trust question has
**No, exit** selected, so an Enter there ends it. So `mia main say` and
`mia main model` type nothing then. They say what it asks, exit 1:

```
mia: claude asks first: Is this a project you created or one you trust? — answer it: `mia main press <n>` (1 No, exit · 2 Yes, I trust this folder)
```

mia never answers it for you. `mia main press <n>` does: a digit, where the
agent numbers its options; where it draws them with no numbers and takes no
digit, as with claude's trust question, mia moves the cursor to option n and
presses Enter once it sees the cursor there, and presses nothing more when
it does not. A question is a run of options at the foot of the screen with
the agent's cursor on one, and, for unnumbered ones, a hint under them such
as `Enter to confirm`.

## What it may answer

Its notes say it in a few lines. A permission prompt for work the task
already allows: yes. Never an option that answers later prompts too. A
product choice, a change of scope, anything destructive: it asks you, and
says which agent waits.

Your own answer to another agent does not have to go through it. `mia asks`
lists who waits on you in every project, and `mia asks 2 1` gives the second
its first answer. `mia window send <worktree> <window> -- <text>` and
`mia window press <worktree> <window> -- 1` type into an agent's window
directly.

## Shown as a chat

`mia chat --chat main` follows its conversation as a chat that mia desk
draws over the pane it runs in, from any folder; `mia chat --chat <name>`
does the same for any [chat](chats.md). It is the view `mia chat` gives a
worktree's window (see [Automation](api.md#conversations)): a quiet line
with the agent, its model, effort and account, the conversation under it,
and a field to type in. `q` or `⌃C` in the plain terminal leaves.

What it follows is the chat's whole thread: the conversations it went on
from, each switch between them shown as a band, then the one its agent has
open. When its agent opens another, the view follows that one in place.

| in the view | does |
|---|---|
| sending | `mia main say`, `press` and `interrupt`; the agent shows as at work until a reply comes and the conversation is still, its window rests, or 90 s pass |
| Models | a model of its own agent switches it as `mia main model` does; another agent goes on with the conversation as `mia main switch` does |
| Earlier | its own earlier conversations, the open one marked: one read back, or gone on with as `mia main start --resume <id> --restart` does, unless it is at work or asks something |
| Fork, or a reply's *Continue from here with…* | makes a new chat for the agent picked, told nothing yet, with the thread up to that reply handed to it as a handoff; this chat stays as it is |
| models marked with `⇧⏎` | the next message also goes to each marked agent, each in a new chat that forks this one's thread, and a band here says which chats they went to |

A forked chat opens on the band that says where it went on from; the turns
before it stay with the chat it was forked from. Continuing another agent's
earlier conversation with this chat's agent, which a window's chat offers,
is not offered here.

With `[main] machine` naming another machine, that machine's mia runs the
view over `ssh -t`, with this terminal as its own, so it is drawn here.

## For an app

`mia api main` is where it stands, with `asks` when it asks something;
`mia api main turns [--after <n>] [--follow]` is its conversation as turns,
each mia verb it ran said as what it did; `mia api main conversations` is
its [earlier ones](#earlier-conversations); `mia api standing` is every
agent in every repository. See [Automation](api.md#the-main-agent) for the
fields, the `switch` and `did` turns, and the capabilities that say a mia
has each.
