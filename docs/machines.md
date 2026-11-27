# Machines

A machine is another computer you reach with ssh, such as a desktop that
stays on while your laptop sleeps. `local` is this computer. A worktree
can live on a machine, its session and agents running there, and move
between here and there with its work and its windows.

## Adding one

```bash
mia machine add fedora user@fedora.lan   # a name, and the ssh target
mia machine list                         # every machine, and how each stood when last asked
mia machine shell fedora                 # a shell there
mia machine ship fedora                  # this mia, there
mia machine rm fedora                    # forget it
```

`mia machine add` checks that ssh reaches the machine without asking for
a password, and that it has podman or docker — an
[environment](environments.md) is always a container, so one without is
refused. Then it ships mia there.

`mia machine list` says each machine's ssh target, its container engine,
and how it stood when last asked — `ok`, `unreachable`, `needs_login`,
`no_mia`, `old_mia` — with what to do about it, without asking again.
`mia doctor` checks every machine. `mia machine rm` forgets the machine;
an environment recorded there does not start until it is moved.

## Shipping mia

A machine runs its own mia, at `~/.local/bin/mia` there, and mia keeps it
the same build as yours. `mia machine ship <name>` puts this build there,
built for the machine's system when it is another one, and does nothing
when the machine has it already. mia ships itself whenever a worktree is
made or moved there, so you rarely need to. For a machine of another
system, mia's source and Go must be on this computer to build it.

## A worktree on a machine

```bash
mia new fix-retries --on fedora
```

```
kijenge — fix-retries in a worktree of its own
/home/user/src/shop.kijenge
kijenge@fedora — fix-retries on fedora
```

The worktree is made on fedora by fedora's own mia, so its session and
agents keep working while this computer sleeps. The repository sits at the
same path under fedora's home as under yours, so it must be under your
home here. fedora becomes a git remote of this repository, named for it:
mia pushes the base branch there (and the branch, when it exists here) and
copies this repository's `.git/mia/config.toml`, so setup and agents are
the same. fedora never needs GitHub or this computer; `git fetch fedora`
brings its commits back, and pushing them anywhere is yours.

`mia machine link fedora` does that first part alone: the repository
there, the base branch pushed.

## Working on one there

A worktree on a machine is named `<worktree>@<machine>`:

```bash
mia shell kijenge@fedora                     # its session
mia run kijenge@fedora make test             # a command in it, once
mia window new kijenge@fedora claude         # every mia window verb
mia machine attach kijenge@fedora            # its session, in this terminal, over ssh
```

The machine's own mia answers each. In tmux, `mia shell` hands this
terminal over to the session there; detached there, it comes back to the
session it left.

In tmux:

- `project/` in [the one list](tmux.md#the-one-list) lists a project's
  worktrees on every machine too, and Enter on one opens that machine's
  session in a window here, named `worktree@machine`. A past conversation
  on a machine opens the same way.
- `new <branch> on <machine>` in the one list makes a worktree on that
  machine, and `@` moves this worktree to one or brings one back.
- The [status line](tmux.md#the-status-line) shows each machine's name
  with a green `●` while it answers and a red `○` when it does not, and a
  signal from a worktree there as `worktree@machine`. It never waits on a
  machine: it reads their last answers and has them asked again every 30
  seconds, fetching their branches as it does. A machine that does not
  answer keeps its rows as it last reported them.

## Moving a worktree

```bash
mia move monduli fedora              # from here to fedora
mia move monduli@fedora local        # and back
mia move --dry-run monduli fedora    # what would cross; nothing moves
```

In tmux, `@` typed into the one list moves the worktree you are in, or
brings one back.

The branch goes across, and so does everything not committed: what is
staged stays staged, changes not staged stay so, and new files arrive as
they are. A file git ignores crosses too when it is a file of at most 1
MiB — a `.env`, a local setting — with its mode. A folder git ignores
(`build/`, `node_modules/`) and an ignored file over 1 MiB do not cross:
the move names them, and they go with the worktree where it was.

The worktree is made on the other side as `mia new` makes one, its
`clone` files copied and `setup` run, under the same name and so in a
folder of the same name; when the name is another worktree's there, it
takes a free one and says so. Then the session's windows open again
there, in the same folders, split as they were, each with the command that
was running in it typed in again — claude, nvim, a dev server. Only once
all of that is there is the worktree removed where it was, its session and
environment with it; the branch stays on both sides.

A running program starts again rather than going on. Each agent's
conversation crosses with it: for claude, codex and pi, the worktree's
latest conversation is copied into the agent's folder on the other side,
its paths rewritten, and the window starts `claude --continue`, `codex
resume <id>` or `pi --continue` there. An agent a
[plugin adds](plugins.md#an-agent) carries its own. A window's agent that
runs on an [account](agents.md#an-agents-account) starts on the account of
the same name there, its conversation in that account's folder; with none
of that name there, on the default one, and the move says so. A sign-in
never crosses: sign in to the account on each machine. A sawa review of
the worktree goes along when `sw` is installed.

`--dry-run` checks the machine answers, then says the branch, how many
files are staged, changed and new, which ignored files cross and which
stay, and the windows that would open:

```
reading monduli: its changes and its windows
would take due-dates across, with 1 file(s) staged, 3 changed and not staged, 2 new, 1 ignored file(s) carried
would open 3 window(s) there: claude · nvim · shell
nothing was moved
```

A move is refused before anything crosses when the worktree has no branch
checked out; when it is from one machine to another, which is two moves,
through here; and, coming back, when the branch here has commits the
machine's has not, or both moved on since they parted: merge them first.

When the new worktree on the other side already has an ignored file the
move brings — a `clone` file, or one `setup` wrote — and it differs, the
move stops before writing over it, and the worktree where it was is left
untouched: move that file aside there, or remove it, and move again.

## Environments and the main agent

An [environment](environments.md#other-machines) runs on a machine apart
from its worktree: `mia env host fedora` moves it there while the worktree
stays here. [The main agent](main.md#on-which-machine) can live on a
machine too.
