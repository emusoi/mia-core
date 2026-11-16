# The main agent

One agent you talk to, which runs the others: it gives a task a worktree
and an agent of its own, tells an agent something, answers what it may,
brings back what waits on you, and says where things stand. You have one,
on the machine that is always on, and it is no repository's.

```bash
mia main start        # its folder and session when they are not there, and the agent in it
mia main              # whether it runs, and what it is doing
mia main say start goba on the login bug, and tell me when it asks something
mia main interrupt    # stop it mid-work
mia main stop         # end its session; its folder and conversations stay
```

Each answers from any folder. `mia main start` twice starts one: an agent
that already runs is left as it is. Stopped and started again, it goes on
with its last conversation.

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

## For an app

`mia api main` is where it stands, `mia api main turns` its conversation
with what it did as lines of their own, and `mia api standing` every
agent in every repository: see [Automation](api.md#the-main-agent).
