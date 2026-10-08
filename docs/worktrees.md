# Worktrees

## Make one

```bash
mia new due-dates
```

makes the branch (from `base` if set), a worktree next to the repository
(`~/src/shop.monduli`), and a name. It copies `clone` files from the main
checkout and runs `setup`. If the branch is already checked out somewhere,
you get that worktree.

| flag | does |
|---|---|
| `--shell` | attach to its session after |
| `--on <machine>` | start its environment on that machine |
| `--pr <number>` | check out a pull request |
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
mia window close monduli logs
```

Name windows you start often under `[launch]` in the config, then
`mia window new monduli logs`.

## Names, stars, setup

```bash
mia rename monduli billing     # a name you choose
mia star monduli               # keep it at the top of the dashboard
mia setup monduli              # run setup again
mia run monduli make test      # run a command in it
```

## Remove

```bash
mia rm monduli
```

removes the worktree, its session, its environment and its name. It refuses
when there are uncommitted changes (`--force` discards them) or a session is
running (`--stop-running` stops it).

```bash
mia gc             # worktrees untouched for 14 days
mia gc --apply     # remove them
```

## Pull requests

mia never pushes. `mia pr` drafts a pull request and prints the commands for
you to run:

```bash
mia pr monduli             # preview
mia pr monduli draft       # save to .git/mia/pr/<branch>.md
```
