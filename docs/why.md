# Why mia

With coding agents, everyone is working on a different part of the same app:
a feature here, a fix there, an agent on each. One checkout can't hold them.
You stash, switch, reinstall, restart the dev server, and the ports collide.
Your laptop runs out of memory by the third app. Keeping track of what is
where becomes the work.

*Mia* is Swahili for 100, and that is the point: work on a hundred things at
the same time.

## Build at the same time

Every piece of work gets its own worktree and its own tmux session. You and
your agents work side by side and never step on each other.

## Test at the same time

Every worktree can run the whole app in its own container, at its own
address: `monduli.mia` and `kijenge.mia` open side by side.

## Out of room? Move it

Out of memory? Send an environment to a cloud box or your home server over
ssh. You keep editing here, the code syncs there, and the address stays the
same. It works as if it were the same machine.

## Names you remember

Worktrees are named after places, yours first, so a hundred of them stay
easy to tell apart.

## Not another multiplexer

Plenty of people are building new terminal multiplexers for agents, and
rightly so. mia isn't one. It uses tmux and git as they are, and adds the
part they lack: a place, a name and an environment for each piece of work.
