# Why mia

With coding agents, everyone is working on a different part of the same app:
a feature here, a fix there, an agent on each. One checkout can't hold them.
You stash, switch, reinstall, restart the dev server, and the ports collide.
Your laptop runs out of memory by the third app. Keeping track of what is
where, and who is waiting on you, becomes the work.

*Mia* is Swahili for 100, and that is the point: work on a hundred things at
the same time.

## Build at the same time

Every piece of work gets its own worktree, its own name and its own tmux
session. You and your agents work side by side and never step on each other.

## One place to start from

In tmux, `prefix k` opens one list. Type a name and you are there. Type
`pi fix the flaky test` and pi starts on it. Press a number and whoever has
waited longest has their answer. Type anything else and you find what was
said about it, in any project, on any machine.

## Nothing said is lost

Every agent's conversations are kept where the agent keeps them, and mia
reads them all: claude's, codex's, pi's, opencode's. Find one by what was
said, go on with it, or hand it to another agent mid-thought.

## Test at the same time

Every worktree can run the whole app in its own container, at its own
address: `monduli.mia` and `kijenge.mia` open side by side.

## Out of room? Move it

Send a worktree to another machine with `mia move`: its branch, its
uncommitted work, its windows and its agents' conversations go with it. Or
send only its environment, and keep editing here while the code runs there.

## Names you remember

Worktrees are named after places, yours first, so a hundred of them stay
easy to tell apart.

## Not another multiplexer

Plenty of people are building new terminal multiplexers for agents, and
rightly so. mia isn't one. It uses tmux and git as they are, and adds the
part they lack: a place, a name and an environment for each piece of work,
and one way into all of them.
