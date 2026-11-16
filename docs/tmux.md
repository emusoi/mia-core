# mia in tmux

mia lives in your tmux: a part of the status line, two popups and a menu.

```bash
mia tmux            # print what it adds
mia tmux install    # add it to ~/.tmux.conf, and have tmux read it
mia tmux uninstall  # take it out again
```

| key | opens |
|---|---|
| `prefix space` | the radar |
| `prefix n` | a new worktree |
| `prefix m` | mia's menu |
| `prefix a` | the next worktree here that needs you, best rank first, round again at the end |

## The status line

The right end of tmux's status line, for the pane you are in: the top
signal (below), the worktree and its branch, its place in a stack with `⟳`
when a layer has fallen behind the one under it, its uncommitted files, and a dot for each machine,
red when it stops answering. It never waits on a machine; it reads their
last answers and has them asked again every 30 seconds.

## The radar

A popup 72 columns wide, one row per worktree, in four groups:

| group | holds |
|---|---|
| needs you | worktrees with a [signal](#signals) ranked 50 or below, by rank: an agent asks, tests went red, a task is done |
| active | the worktree you are in, an open session, uncommitted work, a stack, or anything touched in 14 days |
| ▸ idle | clean, no session, not starred, untouched for 14 days: what `mia gc` removes. Folded |
| ▸ not mia's | worktrees of the repository mia did not make, such as an agent's or another tool's. Folded, with how many hold changes |

Another machine's worktrees come under `on <machine>`. `⏎` on a fold opens
it, and its last row removes all of it at once (idle ones with `mia gc
--apply`, clean ones mia did not make with `mia gc --others --apply`);
branches stay. `⏎` on a worktree mia did not make adopts it.

A stack is one row: `→` opens it into its layers, and `⏎` on a layer moves
there, in place. Type to find any worktree and what can be done with it.
`⏎` switches there; a worktree on another machine hands this terminal over.

| key | does |
|---|---|
| `⏎` | switch there |
| `?` | the selected worktree in full, its windows as they are now, `←` `→` between them and its diff; `?` again for the list |
| `^d` | remove it, after a `y`; one mia did not make only when nothing in it is uncommitted |
| `^n` | a new worktree |
| `tab` | only what needs you, this computer's (`local`), or one machine's |
| `^x` | clear its signal |
| `^y` | approve what it asks: presses its signal's `--yes` keys in the pane that set it |
| `^r` | reply: type a line, `⏎` sends it with Enter to that pane, `esc` drops it |
| `esc` | close |

## The menu

`prefix m` is tmux's own menu, built when you press it: the radar
(`space`), a new worktree (`n`), cleaning up what is idle (`c`) and what
mia did not make (`o`) with how many of each there are, then the
plugins' verbs your config's `menu` names, such as `menu = ["type", "quiz"]`.
Every verb is still one word away in the radar. Cleaning up shows what would go and asks `y`
first (`mia gc --ask`); branches stay.

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
mia signal <worktree|.> <kind> [text] [--style red] [--rank 10] [--from claude] [--for 10m | --until seen] [--yes 1]
mia signal <worktree> clear [--from claude]
mia signal ls
```

`.` is the worktree the command runs in, which is what a hook means. Set
with `.`, a signal goes when the process behind it does, by its process id
and when it began: the agent whose hook set it or, with no agent above it,
whatever ran the hook's shell. An agent that asked and then died asks no
more, even killed with no word, its pane left with a shell in it. One set
from a tmux pane goes when that pane does too. A `--until seen` signal
outlives its process: a done is news after the agent has gone. One signal
per source per worktree. Lower ranks come first (50 if none is
given). A signal ranked above 50, such as claude's `busy` at 60, says how a
worktree is rather than asking for you: it is a word on its row, under
active. `--for` ends it after a while, `--until seen` when you next switch
to the worktree. A machine's signals arrive with its worktrees.

A note on where a worktree stands, one an agent leaves and that should
outlive it, is a signal with no pane: `TMUX_PANE= mia signal . note "tests
green, ready for review" --rank 60 --from note`. Emptied so, it keeps no
process either. `mia ls` shows each
worktree's best signal at the end of its row.

Set with `.` from inside tmux, a signal keeps the pane it came from, so the
radar can answer there without switching: `^r` types a reply, and `^y`
presses what `--yes` names as the approval. A pane no longer in the
worktree's session is not typed into.

A claude hook, in `~/.claude/settings.json`; it reads its event on stdin:

```json
"Notification": [{"matcher": "idle_prompt", "hooks": [{"type": "command", "command":
  "mia signal . asks \"$(jq -r .message)\" --style red --rank 10 --from claude"}]}],
"PermissionRequest": [{"hooks": [{"type": "command", "command":
  "mia signal . asks \"wants $(jq -r .tool_name)\" --style red --rank 10 --from claude --yes 1"}]}],
"Stop": [{"hooks": [{"type": "command", "command":
  "mia signal . done --style green --rank 30 --from claude --until seen"}]}]
```
