# Chats

A chat is a plain conversation with any agent — claude, codex, opencode,
pi, agy, or one a plugin adds — that no repository and no worktree holds,
and that you come back to and go on with. Each is a [main agent](main.md)
of its own: a folder and a session of its own, the same notes, and the
same verbs, so it does what you ask on your behalf, runs commands and
drives the other agents through mia as the main agent does. The main
agent is the chat named `main` in every listing.

```bash
mia chats                                   # each, the main agent first: name, agent, what it is doing, its first words
mia chats new --agent codex -- what changed in mia-desk this week?   # prints its name, starts it, says its first words
mia chats new --agent claude --model opus --effort high              # one with nothing said yet
mia chats say goba -- and the tests?        # say something to it, Enter after
mia chats press goba 2                      # a key with no Enter: answer what it asks
mia chats model goba sonnet --effort high   # switch its model while it runs
mia chats switch goba --agent claude --model opus   # go on with another agent, the conversation carried over
mia chats interrupt goba                    # stop it mid-work
mia chats stop goba                         # end its session; its folder and conversations stay
mia chats start goba                        # start it again, going on with its last conversation
mia chats remove goba                       # end it and take its folder and its name
```

Each answers from any folder. `main` is the main agent in each of them
but `remove`: `mia chats say main -- …` is `mia main say …`.

## Where it lives

| | |
|---|---|
| its folder | `~/.config/mia/chats/<name>`, which mia makes and no repository holds |
| its session | `mia-chat-<name>` in tmux, with the agent in a window named for it |
| its name | given by the name registry as a worktree's is, and held there while it is: no worktree is given it |
| its environment | `MIA_CHAT=<name>`, and neither `MIA_MAIN` nor `MIA_WORKTREE` |
| its notes | `CLAUDE.md` for claude, `AGENTS.md` for another, written by mia at every start: the main agent's, but for who it is |
| what mia keeps of it | `.mia/chat.json` in its folder: the agent it goes on with, the model and effort it is started with, its first words |

It lives where the main agent does: with `[main] machine` naming another
machine, every `mia chats …` and `mia api chats`, `mia api chat …` is that
machine's mia's, asked over ssh, as [the main agent's are](main.md#on-which-machine).

## A new chat

`mia chats new --agent <agent> [--model <model>] [--effort <effort>] [-- <first words…>]`
makes its folder, takes a name from the registry and prints it, the one
line it prints, then starts the agent there. The model and the effort
are checked against the agent's before anything is made, and are what it
is started with each time; left out, its agent's own under
[`[agents]`](configuration.md#agents). What follows `--` is said to it
once it is ready, as `mia chats say` says it, and is its title in a list.

An agent that asks something first — claude, in a folder it has not seen,
asks whether you trust it — is asked it in every new chat: the first
words wait, nothing is typed over the question, and a line on stderr
says what it asks. `mia chats press <name> <n>` answers it, and once the
agent is ready the first words are said. mia never answers it for you.

## Saying something to it

`say`, `press`, `interrupt` and `model` are [the main agent's](main.md#saying-something-to-it):
`say` pastes the text as one message and presses Enter after, and types
nothing while it asks something; `press` types keys with no Enter; with
none running, each says so and that `mia chats start <name>` starts it.
What follows the name is taken as it is typed: `--json` or `-h` in it are
words.

## Another agent mid-conversation

`mia chats switch <name> --agent <agent> [--model <model>] [--effort <effort>]`
has it go on with another agent as [`mia main switch`](main.md#another-agent-mid-conversation)
has the main agent: what was said is written out as a handoff in its
folder, the agent it was with stopped, the other started in its place and
handed the conversation in front of the first thing said to it. It is
that agent from then on, and its turns hold a `switch` turn where it
went on.

## For an app

`mia api chats` is every chat, the main agent first and the others the
last said in first, `mia api chat <name>` where one stands as
`mia api main` says it, and `mia api chat turns <name> [--after <n>] [--follow]`
its conversation as `mia api main turns` gives it: see
[Automation](api.md#chats). `mia serve` answers them and says when they
change, as tmux says the chat's panes did or its transcript is written.
