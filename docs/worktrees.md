# Worktrees

## Make one

```bash
mia new due-dates
```

makes the branch (from origin's branch of that name if there is one, else
from `base` if set), a worktree next to the repository
(`~/src/shop.monduli`), and a name. It copies `clone` files from the main
checkout and runs `setup`. If the branch is already checked out somewhere,
you get that worktree.

| flag | does |
|---|---|
| `--shell` | attach to its session after |
| `--bg` | return at once and run `setup` in a `setup` window of its session; a failed setup leaves a red signal |
| `--from <ref>` | start the new branch at a commit, tag or branch instead |
| `--name <name>` | call it that instead of the next free name |
| `--on <machine>` | make the whole worktree on that machine: its git, session and agent live there |
| `--stack` | a [stack](stacks.md) layer instead |

`mia adopt <path>` takes over a worktree you made with `git worktree add`.

## Find and go

```bash
mia ls                     # every worktree; ★ starred, dirty, session, +ahead −behind
mia shell monduli          # attach to its session
mia switch monduli         # cd there
mia open monduli           # open it in your editor
mia path monduli           # print its path
```

## Sessions and windows

Each worktree has a tmux session, `mia-<name>`. It keeps running when you
close the terminal.

```bash
mia window                           # its windows
mia window new monduli               # a new shell
mia window new monduli --editor      # your editor
mia window new monduli logs -- tail -f log/dev.log
mia window read monduli logs         # what it shows
mia window send monduli logs -- q    # type a line into it
mia window close monduli logs
```

Name windows you start often under `[launch]` in the config, then
`mia window new monduli logs`.

## Names

Each worktree gets the name of a place, like `monduli`. A place is easier to
remember than a branch or a number ("the fix is in monduli"), short enough to
say and type, and it stays the same while the branch inside changes.

mia's own places are neighbourhoods of Arusha, Dar es Salaam and Nairobi. Use
places you know instead — the streets you grew up on, cities you have lived
in — in `~/.config/mia/config.toml`:

```toml
names = ["kariakoo", "brooklyn", "shinjuku", "camden"]
```

mia hands out yours first, then its own. A name is lowercase letters, digits
and hyphens, because it becomes a hostname and a tmux session.

## Stars, setup

```bash
mia rename monduli billing     # a name you choose
mia star monduli               # never idle: kept out of mia gc
mia setup monduli              # run setup again
mia run monduli make test      # run a command in it
```

## Remove

```bash
mia rm monduli
```

removes the worktree, its session, its environment and its name. It refuses
when there are uncommitted changes (`--force` discards them) or a session is
running (`--stop-running` stops it). `--branch` deletes the branch too, once
it is merged into the base; otherwise it is kept and `mia rm` says so. If environment cleanup fails, it keeps
the worktree and records so cleanup and removal can be retried.

```bash
mia gc                  # idle worktrees: clean, no session, not starred, untouched for 14 days
mia gc --apply          # remove them
mia gc --merged         # clean worktrees whose branch is an ancestor of the base
mia gc --merged --apply
mia gc --others         # worktrees mia did not make, an agent's or another tool's
mia gc --others --apply # remove the clean ones
```

The radar folds idle ones and those mia did not make away under `idle` and
`not mia's`, and removes them from there too; `^d` removes the one
selected. Branches always stay. `--merged` does not fetch and cannot be
combined with `--days`; update the base first if needed. It detects
ancestor merges, not squash merges. Removal also stops and removes the
recorded environment; a cleanup that fails keeps the worktree for retry.

## Put away, or try again

```bash
mia archive monduli            # put it away as it is
mia archive ls                 # what is put away
mia unarchive monduli          # and back, as it was
mia fork monduli second-try    # a second worktree beside it
```

`archive` keeps everything in the worktree that git does not ignore,
uncommitted and untracked work too, as a commit under
`refs/mia/archive/<name>`, then removes the worktree; its branch stays,
and the stash is never touched. `unarchive` makes it again on that branch,
under its old name when that is free, with the work uncommitted as it was.

`fork` starts a new branch where a worktree is, not at base, and copies
its uncommitted and untracked work in; the first is left as it is.

## After a crash

Every 30 seconds or so, while tmux shows mia's status line, mia notes each
session's windows, and each pane a window is split into: their folders and
what runs in them. When tmux or the
computer goes down, `mia resurrect` opens each one again, its programs
started anew and each claude resuming the very conversation it had; codex,
pi, opencode, gemini and cursor-agent go on with their folder's last. An
agent that ended before tmux did comes back too: its window, back at a
shell, keeps it until it runs something else. mia
forgets none: a session you closed while tmux was running stays closed,
but `mia resurrect <worktree>` brings it back, and `mia resurrect --list`
shows every session it remembers — running, down or closed — and when it
was last seen. `--dry-run` says what it would open.

## Which conversation was that

```bash
mia why internal/app/app.go:212   # the conversation behind this line
mia why 882b0e5                   # or behind a commit
mia recall retry loop             # every conversation that mentions it
```

Both read claude's own transcripts of this repository and its worktrees,
so nothing is recorded as you work, and both say what you asked there and
how to `claude --resume` it.

## On another machine

```bash
mia new fix-retries --on fedora
```

The worktree lives on fedora, made by fedora's own mia, so its session and
agent keep working while this computer sleeps. The repository sits at the
same path under fedora's home as under yours, and fedora becomes a git
remote of this one: mia pushes the base branch (and the branch, if it
exists here) before making the worktree. fedora never needs GitHub;
`git fetch fedora` brings its commits back. mia ships itself there first,
built for fedora, whenever this mia is a build fedora has not had.

The radar lists fedora's worktrees under `on fedora`, as fedora's mia last
answered; tmux's status line has it asked again every 30 seconds, fetching
fedora's branches as it does. ⏎ on one hands this terminal over to its
session there. When fedora does not answer, its rows stay, marked not
answering, as it last reported them.

While a terminal is there, fedora's radar lists this computer's worktrees
too, told again every 30 seconds, and ⏎ on one hands the terminal back.

`mia shell <worktree>@fedora` is its session there. Its branches arrive
here as `fedora/<branch>` on every refresh; pushing them anywhere is yours.

## Moving a worktree

```bash
mia move monduli fedora          # from here to fedora
mia move monduli@fedora local    # and back
```

The branch goes across, and so does what is not committed: changed files
as a patch, new files as they are. The session's windows open again
there, in the same folders, each with the command that was running in it
typed in again — claude, nvim, a dev server. Only once all of that is
there is the worktree removed where it was; its branch stays.

A running program starts again rather than continuing. One that keeps
state can bring it along through an extension named `mia-carry-<program>`
on your PATH: mia asks it to `pack <from> <to>` — a tar of files under the
home, which is unpacked on the other side — and how to `command` it to
start again. `scripts/mia-carry-claude` carries claude's conversation and
restarts it with `claude --continue`.
