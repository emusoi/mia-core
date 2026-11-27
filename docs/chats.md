# Chats

A chat is a plain conversation with any agent that no repository or
worktree holds, and that you come back to and go on with. Each one is a
[main agent](main.md) of its own: a folder and a session of its own, the
same notes, and the same verbs. It does what you ask, runs commands, and
drives the other agents through mia as the main agent does. The main agent
is the chat named `main`.

```bash
mia chats                                   # each chat, the main agent first
mia chats new --agent codex -- what changed in mia-core this week?
mia chats new --agent claude --model opus --effort high   # one with nothing said yet
mia chats say sinza -- and the tests?       # say something to it, Enter after
mia chats press sinza 2                     # a key with no Enter: answer what it asks
mia chats model sinza sonnet --effort high  # switch its model while it runs
mia chats switch sinza --agent claude --model opus   # go on with another agent, the conversation carried over
mia chats interrupt sinza                   # stop it mid-work
mia chats stop sinza                        # end its session; its folder and conversations stay
mia chats start sinza                       # start it again, going on with its last conversation
mia chats remove sinza                      # end it, and take its folder and its name
```

`say`, `press`, `model`, `switch`, `interrupt`, `stop` and `start` do to a
chat what the `mia main` verbs of the same names do to the main agent: see
[Saying something to it](main.md#saying-something-to-it),
[When it asks first](main.md#when-it-asks-first),
[Its model](main.md#its-model) and
[Another agent mid-conversation](main.md#another-agent-mid-conversation).
`main` names the main agent in each of them but `remove`:
`mia chats say main -- …` is `mia main say …`. Every `mia chats` verb works
from any folder.

`mia chats` (or `mia chats ls`) prints a line for each: its name, its
agent, what it is doing (`idle`, `working`, `asks` or `stopped`) and its
first words, `mia` for the main agent.

```
main       claude    idle     mia
sinza      codex     stopped  what changed in mia-core this week?
```

## A new chat

`mia chats new --agent <agent> [--model <model>] [--effort <effort>] [-- <first words…>]`
checks the model and effort against the agent's, takes a name from the
registry, makes the chat's folder, prints the name (the one line it
prints) and starts the agent there. The model and effort are what it starts with
each time; left out, its agent's own under
[`[agents]`](configuration.md#agents). The words after `--` are said to it
once it is ready, and are its title in a list.

An agent that asks something first is asked it in every new chat, since
each has a new folder: claude asks whether you trust it. The first words
wait, nothing is typed over the question, and a line on stderr says what
it asks. `mia chats press <name> <n>` answers it, and the first words are
said once the agent is ready.

## What differs from the main agent

| | the main agent | a chat |
|---|---|---|
| its folder | `~/.config/mia/main` | `~/.config/mia/chats/<name>` |
| its session | `mia-main` | `mia-chat-<name>` |
| its name | `main`, held in the registry | given by the registry as a worktree's is, and held while the chat is there |
| its environment | `MIA_MAIN=1` | `MIA_CHAT=<name>`; neither `MIA_MAIN` nor `MIA_WORKTREE` |
| its notes | who the main agent is, and what it may answer | the same, but that it is one of your chats |
| its agent, model, effort | `[main]`, or as last switched | what `mia chats new` gave it, or as last switched, kept in `.mia/chat.json` in its folder |
| its account | `[main] account`, or `--account` | its agent's own under `[agents]`, or the agent's own folder |
| a new or earlier conversation | `mia main start --new`, `--resume <id>` | none on the command line; Earlier in [the view](#shown-as-a-chat) goes on with one |
| removed | never: `mia main stop` stops it | `mia chats remove <name>` ends it and takes its folder and name; its conversations stay with its agent |

`mia chats start <name>` takes no flags: it starts the chat's agent going on
with its last conversation, says its first words if they still wait, and
prints its line.

Chats live where the main agent does. With
[`[main] machine`](main.md#on-which-machine) naming another machine, every
`mia chats …`, `mia api chats` and `mia api chat …` is that machine's mia's,
asked over ssh.

## Shown as a chat

`mia chat --chat <name>` follows a chat's conversation as a chat that mia
desk draws over the pane, from any folder: the same view, with the same
Models, Earlier and Fork, as
[the main agent's](main.md#shown-as-a-chat). What it sends, presses and
stops is done as `mia chats say`, `press` and `interrupt` do it.

## For an app

`mia api chats` is every chat, the main agent first, then the others, the
last said in first. `mia api chat <name>` is where one stands, as
`mia api main` says it, and `mia api chat turns <name> [--after <n>] [--follow]`
its conversation as turns. `mia serve` answers them and says when they
change. See [Automation](api.md#chats) for the fields.
