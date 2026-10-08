# Worktrees

A worktree is a checkout with a name and a tmux session. Making one, moving
between them and getting rid of them is most of what mia is for.

## Making one

    mia new due-dates

1. **The branch.** `prefix` from the config goes in front, unless the name
   already starts with it (`prefix = "you/"` makes `you/due-dates`). The name
   must be a valid git branch name.
2. **Already checked out?** If some worktree already has that branch, mia
   uses it (and takes it over if it had no record): `… is already checked out
   there`.
3. **The name and the directory.** The next free name from the list goes to
   the worktree, and the directory is a sibling of the main checkout:
   `~/src/shop` gets `~/src/shop.monduli`.
4. **`git worktree add`.** A new branch starts from `base` when the config
   sets one, otherwise from whatever the main checkout has checked out. An
   existing branch is checked out as it is.
5. **Ready to work in.** `clone` patterns are copied from the main checkout
   (copy-on-write where the file system can, never overwriting), and with
   `commit_times` every tracked file is dated by its last commit. Then
   `setup` runs, its output on your terminal.

        monduli — you/due-dates in a worktree of its own
        /Users/you/src/shop.monduli

If preparing or setup fails, the worktree still exists and mia says what
failed; `mia setup monduli` runs setup again.

| flag | does |
|---|---|
| `--shell` | attach to its session afterwards |
| `--on <machine>` | put its environment on that machine and start it (see [environments](environments.md#other-machines)) |
| `--pr <number>` | fetch pull request `<number>` into a branch `pr-<number>` and make a worktree for it |
| `--stack [--in <worktree>] [--worktree]` | a new [stack layer](stacks.md) instead |

`--json` prints the record.

### A worktree you made by hand

    git worktree add ../shop.experiment experiment
    mia adopt ../shop.experiment

`adopt` gives it a name and a record. The main checkout is adopted the
first time you use it, named after its directory.

## Finding them

    mia ls

```
★ shop      main                         session
  monduli   you/due-dates                dirty · session · +3 −0
  kijenge   you/search                   +1 −4 · in payments
  experiment experiment                  not adopted
```

`★` marks a starred worktree. The marks are: `dirty` (uncommitted changes,
untracked files included), `session` (its tmux session is running), `+A −B`
(commits ahead of and behind the base), `in <stack>`, and `not adopted` (a
git worktree mia has no record of). `mia ls --json` gives everything, and
records whose directory is gone are dropped as the list is read.

Anywhere mia takes a worktree, it takes its **name**, its **path** (anything
with a `/`, or `.`), or the **branch** checked out in it, in that order. Not
found exits with status 3; a git worktree mia does not know yet says to
`mia adopt` it.

## Going to one

    mia shell monduli          # attach to its tmux session
    mia switch monduli         # cd there, in this shell
    mia open monduli           # open it in your editor or IDE

`mia switch` needs the shell function, once, in your shell's rc file:

```bash
eval "$(mia shell-init zsh)"
```

(`bash` and `fish` work too.) It also gives you completion of verbs,
worktree names and branches. `mia switch <stack>` goes to a stack by its
name, and `mia path <worktree>` prints a path for scripts — with no argument,
the main checkout.

`mia open` tries cursor, code, zed, idea, subl, then on macOS Xcode, Finder
and Terminal, and uses the first one installed. `--in code` picks one.

## Sessions

Every worktree has a tmux session, `mia-<name>`, started the first time you
attach. It belongs to the worktree, not to your terminal: close the terminal
and everything in it keeps running.

`mia shell` switches your tmux client to the session when you are already in
tmux, and attaches otherwise. `mia shell --popup` opens the session in a tmux
popup over what you are doing, sharing the session's windows; leave it with
`prefix d`.

When an environment runs on another machine, the session runs there too, in
the staging copy, so what you start in it runs next to the code.

### Windows

A session holds windows — shells, an editor, a dev server:

    mia window                         # this worktree's windows
    mia window new monduli             # another shell
    mia window new monduli --editor    # your editor on the worktree
    mia window new monduli logs -- tail -f log/dev.log
    mia window open monduli logs       # land in it
    mia window close monduli logs

Windows you open often can be named once in the config, and then started by
name:

```toml
[launch]
test = "go test ./..."
logs = "tail -f log/dev.log"
```

    mia window new monduli test

The `t` key on the dashboard shows the same list as a panel.

## Names

Names come from a list of neighbourhood names from Arusha, Dar es Salaam and
Nairobi, handed out in order and unique across every repository on your
machine. They live in `~/.config/mia/names.json`.

    mia rename monduli billing

gives a worktree the name you want. Its session follows at once; its
container takes the new name the next time it starts. A name must be one
word without `.` or `:`, since it becomes a hostname and a tmux session.

## Starring

    mia star                   # the worktree you are in
    mia star monduli

A starred worktree sits at the top of the dashboard, next to the main
checkout, and is never offered for clean-up. Run it again to unstar.

## Removing

    mia rm monduli

removes the environment, the session, the worktree, its record and its name.
It refuses when there is something to lose, and says how past it:

- uncommitted changes: `mia rm --force monduli` discards them;
- a running session: `mia rm --stop-running monduli` stops what runs in it.

### Tidying up

    mia gc                     # what nobody has touched in 14 days
    mia gc --apply             # remove those
    mia gc --days 30

A worktree is abandoned when it is clean, has no session, is not starred and
its last commit is older than the days given. `mia gc` only lists; `--apply`
removes. Star one to keep it.

## Running things

    mia setup monduli          # run the configured setup again
    mia run monduli make test  # run a command in the worktree, here
    mia env run monduli make test   # …or where its code runs: inside its environment if it is up

## Pull requests

mia never pushes and never opens a pull request. It drafts one and prints the
commands for you to run:

    mia pr                     # a preview for this worktree
    mia pr monduli draft       # save the draft

The title is the latest commit's subject and the body lists every commit
since the base (or since the layer below, for a stack layer). A saved draft
lives in `.git/mia/pr/<branch>.md`; edit it there or pass `--title` and
`--body-stdin`. Then:

    run these yourself:
      git -C /Users/you/src/shop.monduli push -u origin you/due-dates
      cd /Users/you/src/shop.monduli && gh pr create --base main --head you/due-dates --title '…' --body-file …

`--remove-after-merge true` adds the `mia rm` to run once it has merged.

To review someone else's pull request in a worktree of its own:

    mia new --pr 482 --shell
