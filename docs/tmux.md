# mia in tmux

mia lives in your tmux: a worktree is a tmux session, a tab is a window,
an agent is a program in one. mia adds a part of the status line, one
list, a popup terminal, a menu and a few keys. It draws no screens of its
own.

```bash
mia tmux            # print what it adds
mia tmux install    # add it to ~/.tmux.conf, and have tmux read it
mia tmux uninstall  # take it out again
mia tmux keys kitty # the lines kitty (or ghostty) takes for the ⌘ keys
```

| prefix key | ⌘ key | does |
|---|---|---|
| `k` | ⌘K | [the one list](#the-one-list) |
| `K` | ⌘O | [the popup terminal](#the-popup-terminal), in the folder you are in |
| `space` | ⌘⇧J | [the radar](#the-radar): who needs you, to answer |
| `n` | ⌘N | [a new worktree](#a-new-worktree) |
| `m` | | [mia's menu](#the-menu) |
| `a` | ⌘J | the next worktree here that needs you, best rank first, round again at the end |
| `e` | ⌘. | this window: stop its agent, move it to another agent, a new agent window, its model, rename, close, fork into a new worktree, lazygit, the editor |
| `b` / `B` | ⌘[ / ⌘] | back and forward through where you have been |

The ⌘ keys need the terminal app to send them: `mia tmux keys kitty`
prints the lines for its config. They take over the app's own ⌘T, ⌘W,
⌘N, ⌘K and ⌘1–9 (a new shell window, close it, the windows by number).

## The one list

`prefix k` opens one list in the worktree you are in. What you type says
what Enter does, and the first row is what it does:

| typed | Enter |
|---|---|
| nothing | who waits for you (their answers numbered), who works, then the projects, the one here first |
| `project/` | that project's worktrees, those on a machine too |
| a name | goes to that worktree or project, in any project |
| a digit | answers whoever has waited longest |
| `pi fix the test` | starts that agent here on that task (a second one is `pi-2`) |
| `> codex` | hands this conversation to another agent; `> opencode son` lists it on each model the words mean |
| `@` | moves this worktree to a machine, or brings one back |
| `new <branch>` | makes a worktree here and goes to it |
| `rm <name>` | removes one, refusing what `mia rm` refuses; a session of shells alone goes with it |
| `stack` | the layers of the stack here |
| `nvim`, `git`, `shell` | the editor, lazygit or a shell, in a window |
| any other words | past conversations that mention them, in every project and on every machine; Enter resumes one |
| `?` | says all of this |

A worktree or conversation on another machine opens as that machine's
session in a window here, named `worktree@machine`. Past conversations are
[`mia recall`](commands.md)'s: a small index per repository keeps it to a
fraction of a second, and `mia mcp` gives agents the same search.

## The popup terminal

`prefix K` is one shell in a popup over the window, kept between openings,
brought to the folder you were in. `mia cd <worktree>` typed there takes
the client under it to that worktree and closes the popup.

## The status line

The right end of tmux's status line, for the pane you are in: the top
signal (below), the worktree and its branch, its place in a stack with `⟳`
when a layer has fallen behind the one under it, its uncommitted files, and a dot for each machine,
red when it stops answering. It never waits on a machine; it reads their
last answers and has them asked again every 30 seconds.

## Pane kinds

Every pane mia starts is marked with what it is for, in tmux's pane option
`@mia_kind`, so tmux, mia and the apps read it the same:

| kind | when |
|---|---|
| `agent` | it runs an agent mia knows: `claude`, `codex`, `pi`, or one a plugin adds |
| `editor` | it runs `nvim`, `vim`, `vi`, `hx` or `emacs`, or was opened with `--editor` |
| `tests` | its command starts as a test runner does: `go test`, `cargo test`, `npm test`, `pytest`, `vitest`… |
| `logs` | its command follows logs: `tail -f`, `journalctl`, `kubectl logs`, `docker logs`, `stern` |
| `server` | its command is a dev server mia keeps (`npm run dev`, `pnpm dev`…) or what a `[[service]]` of the config runs |
| `shell` | anything else, and a window with no command |

`mia window new … --kind <kind>` says it instead. A pane opened by hand in
tmux has no kind. `tmux show-options -p -t <pane> @mia_kind` reads one.

A pane is titled, on its border, by what it is for: an agent by its name
(`claude`), an editor by its file (`nvim · main.go`) or, opened on a
folder, by the folder it is in, a server by its name and port (`api
:8080`, as `PORT=`, `--port` or `-p` says it), tests `tests`, logs `logs`,
and a shell `shell · <folder>`, following it as it goes. mia keeps the
fixed part in the pane option `@mia_label`; the rest is tmux's own format,
so a plain `tmux attach` shows the same titles as the apps.

A window tmux names, one opened with `prefix c`, is named as its pane is
titled (`shell · config`, `nvim · config`), so two in one folder are told
apart by what runs in them. `mia window new` with no name calls the window
so as well, with `-2`, `-3` after it when that name is taken. A window you
rename keeps its name: tmux turns its own naming off for it, and mia only
ever names a window it is opening.

## Prompt marks

Add this to your zshrc:

```zsh
eval "$(mia shell-integration zsh)"
```

mia only prints it; your zshrc is yours. It marks where each prompt is
and where each command's output starts, with the command's exit status
(OSC 133), and says the folder the shell is in (OSC 7). In tmux's copy
mode, `send -X previous-prompt` and `send -X next-prompt` then jump from
one command to the next, and with `-o` to its output — bind them to keys
of your own in your tmux config.

It does nothing where there is no terminal, and nothing more when your
zshrc is read twice.

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
(`space`), a new worktree (`n`), bringing back sessions that went down
(`b`), cleaning up what is idle (`c`) and what mia did not make (`o`) with
how many of each there are, then the
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
outlives its process: a done is news after the agent has gone. `mia
signal ls` goes by the same rule as `mia ls`, the radar and standing: one
whose setter is gone is not listed, and is forgotten. One signal per
source per worktree. Lower ranks come first (50 if none is
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

### pi says what it does

pi has no hooks; it loads extensions. mia has one that says the same
three things, and writes it into pi's folder when you ask, never by
itself:

```bash
mia account signals pi --dry-run   # what would be written, and where
mia account signals pi             # ~/.pi/agent/extensions/mia-signal.ts
mia account signals pi work        # an account's folder; --all is every one
```

```
pi account default: ~/.pi/agent/extensions/mia-signal.ts written — pi says what it does with mia signal from its next start, or /reload in one that runs
```

The file begins with a line that says it is mia's. Run again, one that is
as mia has it is left; an older one of mia's is written again; a file of
that name without that line is yours, is left as it is, and the command
fails saying so. Nothing else in the folder is read or written, and a pi
that was never started there is given no folder.

| pi | signal |
|---|---|
| a run begins (`agent_start`) | `busy --style dim --rank 60 --from pi` |
| an extension's dialog waits for you during a run (`ui_prompt_start`) | `asks <its title> --style red --rank 10 --from pi`, and `busy` again once answered |
| pi goes on with nothing by itself (`agent_settled`) | `done --style green --rank 30 --from pi --until seen` |
| you stopped the run, or the session ends in one | `clear --from pi` |

It runs `mia signal .` in pi's folder and nothing else. Where there is no
`mia`, or the folder is no worktree of mia's, the first call says so and no
other is made. pi waits for no call, each is ended after three seconds,
and print and JSON modes say nothing. pi asks no permission of its own
before a tool, so `asks` is what an extension asks — a question tool, a
permission gate. On a pi older than `agent_settled`, `done` is said a
second and a half after `agent_end` with no other run begun.
