# Worktrees

A worktree is a checkout of your repository in a folder of its own, with a
name and a tmux session. What runs in it is on other pages: the
[agents](agents.md), their [conversations](conversations.md), and
[another machine](machines.md) to run it on.

## Make one

```bash
mia new due-dates
```

```
monduli — due-dates in a worktree of its own
/Users/you/src/shop.monduli
```

makes the branch (from origin's branch of that name if there is one, else
from `base`), a worktree next to the repository (`~/src/shop.monduli`), and
a name. It copies the `clone` files from the main checkout and runs
`setup` ([Configuration](configuration.md#worktrees)). If the branch is
already checked out somewhere, you get that worktree.

| flag | does |
|---|---|
| `--shell` | attach to its session after |
| `--bg` | return at once and run `setup` in a `setup` window of its session; a failed setup leaves a red [signal](tmux.md#signals) |
| `--from <ref>` | start the new branch at a commit, tag or branch instead |
| `--name <name>` | call it that instead of the next free name |
| `--on <machine>` | make it on [another machine](machines.md#a-worktree-on-a-machine): its git, session and agents live there |
| `--stack` | a [stack](stacks.md) layer instead |

```bash
mia new --pr 482               # a worktree at pull request 482's head, called pr-482
mia adopt ../shop.scratch      # take over a worktree made with git worktree add
```

`--pr` looks for the pull request on `upstream`, then `origin`, then any
other remote, or on the one `--remote <remote>` names. Its
branch tracks nothing, so nothing done there can be pushed to the author's
fork by a slip. Run again after the pull request moved, it brings the
worktree up to the new head when that loses nothing, and otherwise says so
and leaves it.

## Find and go

```bash
mia p ls                   # this project's worktrees; ★ starred, dirty, session, +ahead −behind
mia p ls shop              # another project's, from any folder
mia ls                     # every project mia knows
mia shell monduli          # attach to its session
mia go monduli             # in tmux: take this client to its session, started if it is not up
mia switch monduli         # cd there
mia open monduli           # open it in your editor; --in code, zed, finder, terminal…
mia path monduli           # print its path, for scripts
```

A worktree answers to its name, its path or its branch. `go`, `switch` and
`path` also take a project, or `project/worktree`, from any folder.
`mia switch` (also `mia cd`) changes your shell's folder only with
`eval "$(mia shell-init zsh)"` in your rc; `bash` and `fish` work too.

In tmux, the [one list](tmux.md#the-one-list) (`prefix k`) finds and goes
to any of these by typing part of its name.

## Sessions and windows

Each worktree has a tmux session, `mia-<name>`. It keeps running when you
close the terminal.

```bash
mia window monduli                   # its windows
mia window new monduli               # a new shell
mia window new monduli --editor      # your editor
mia window new monduli logs -- tail -f log/dev.log
mia window open monduli logs         # go to it
mia window select monduli logs       # make it the session's current window, without going there
mia window read monduli logs         # what it shows
mia window send monduli logs -- q    # type a line into it, and Enter
mia window press monduli agent -- 2  # type without Enter: an agent's menu
mia window key monduli agent -- C-c  # a key by tmux's name: stop what runs, keep the window
mia window close monduli logs
```

Name windows you start often under `[launch]` in the config, `logs = "tail
-f log/dev.log"`, then `mia window new monduli logs`. A window named for an
agent runs that agent: see [Agents](agents.md#starting-one).

## Names

Each worktree gets the name of a place, like `monduli`. A place is easier
to remember than a branch or a number ("the fix is in monduli"), short
enough to say and type, and it stays the same while the branch inside
changes.

mia's own places are neighbourhoods of Arusha, Dar es Salaam and Nairobi.
Use places you know instead — the streets you grew up on, cities you have
lived in — in `~/.config/mia/config.toml`:

```toml
names = ["kariakoo", "brooklyn", "shinjuku", "camden"]
```

mia hands out yours first, then its own. A name is lowercase letters,
digits and hyphens, because it becomes a hostname and a tmux session.

## Stars, setup

```bash
mia rename monduli billing     # a name you choose; its session follows at once
mia star monduli               # never idle: kept out of mia gc; again to unstar
mia setup monduli              # run setup again
mia run monduli make test      # run a command in it once
```

## Remove

```bash
mia rm monduli
```

removes the worktree, its session, its environment and its name. It
refuses when there are uncommitted changes (`--force` discards them) or a
session is running (`--stop-running` stops it). `--branch` deletes the
branch too, once it is merged into the base; otherwise the branch is kept
and `mia rm` says so. If removing the environment fails, the worktree is
kept and the failure recorded, so the removal can be run again.

```bash
mia gc                  # idle worktrees: clean, no session, not starred, untouched for 14 days
mia gc --apply          # remove them
mia gc --days 30        # untouched for 30 days instead
mia gc --merged         # clean worktrees whose branch is an ancestor of the base
mia gc --merged --apply
mia gc --others         # worktrees mia did not make, an agent's or another tool's
mia gc --others --apply # remove the clean ones
```

Branches always stay. `--merged` does not fetch, so update the base first,
and it sees merges, not squash merges; it cannot be combined with `--days`.
`prefix m` in tmux offers the same clean-up, and asks `y` first.

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
and the stash is never touched. A running session is refused unless
`--stop-running` is given. `unarchive` makes it again on that branch,
under its old name when that is free, with the work uncommitted as it was
and what was staged staged again. What git ignores is not kept.

`fork` starts a new branch where a worktree is, not at the base, and
copies its uncommitted and untracked work in; the first is left as it is.

## After a crash

While tmux runs mia's status line, every 30 seconds or so, mia notes each
session's windows and each pane a window is split into: their folders and
what runs in them. When tmux or the computer goes down:

```bash
mia resurrect              # open again every session that went down with tmux
mia resurrect monduli      # this one, even if you closed it yourself
mia resurrect --list       # every session mia remembers: running, down or closed, and when last seen
mia resurrect --dry-run    # what it would open
```

Each window opens in its folder, split as it was, its program started
anew. Each claude resumes the very conversation it had; codex, pi,
opencode, agy and cursor-agent go on with their folder's last. Each agent
starts on the [account](agents.md#an-agents-account) it ran on, or the
default one when that account was removed since. An agent, an editor or a
dev server that ended before tmux did comes back too; a command that
finishes, like a build, is not run again. `keep` in the config names more
programs to bring back.

A session you closed while tmux was running stays closed until you name
it. One whose worktree was removed is said to be gone.
