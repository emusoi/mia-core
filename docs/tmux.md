# mia in tmux

mia lives in your tmux: a part of the status line, and two popups.

```bash
mia tmux            # print what it adds
mia tmux install    # add it to ~/.tmux.conf, and have tmux read it
mia tmux uninstall  # take it out again
```

| key | opens |
|---|---|
| `prefix space` | the radar |
| `prefix n` | a new worktree |

## The status line

The right end of tmux's status line, for the pane you are in: the top
signal (below), the worktree and its branch, its place in a stack with `⟳`
when a restack is due, its uncommitted files, and a dot for each machine,
red when it stops answering. It never waits on a machine; it reads their
last answers and has them asked again every 30 seconds.

## The radar

Worktrees with signals first, by rank; then those in progress; the rest
are counted. A stack is one row: `→` opens it into its layers, and `⏎` on
a layer moves there, in place. Type to find any worktree and what can be
done with it. `⏎` switches there; a worktree on another machine opens in a
window of its own.

## A new worktree

Type the branch. Under it, `from · on`: from your base branch, on this
computer or a machine. From inside a stack it starts from the layer you
are on, `as a layer, in place` or `in its own worktree`. `⇥` moves between
the parts, `← →` changes one, `⏎` makes it while you watch; `esc` leaves
it running. The first time in a repository it shows what it guessed —
the base branch, the setup from the lockfile — and saves that as the
repository's config.

## Signals

Anything outside mia can say something about a worktree: an agent's hook,
a test watcher, you. mia gives it no meaning; it shows it as its setter
asks.

```bash
mia signal <worktree> <kind> [text] [--style red] [--rank 10] [--from claude] [--for 10m | --until seen]
mia signal <worktree> clear [--from claude]
mia signal ls
```

One signal per source per worktree. Lower ranks come first (50 if none is
given). `--for` ends it after a while, `--until seen` when you next switch
to the worktree. A machine's signals arrive with its worktrees.

A claude hook, in `~/.claude/settings.json`; it reads its event on stdin:

```json
"Notification": [{"hooks": [{"type": "command", "command":
  "mia signal \"$MIA_WORKTREE\" asks \"$(jq -r .message)\" --style red --rank 10 --from claude"}]}],
"Stop": [{"hooks": [{"type": "command", "command":
  "mia signal \"$MIA_WORKTREE\" done --style green --rank 30 --from claude --until seen"}]}]
```
