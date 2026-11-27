# mia in tmux

mia lives in your tmux: a worktree is a tmux session, a tab is a window,
an agent is a program in one. mia adds a part of the status line, one
list, a popup terminal, a radar, a menu and a few keys. It draws no
screens of its own.

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
| `b` / `B` | ⌘[ / ⌘] | back and forward through the sessions you have been in |

The ⌘ keys need the terminal app to send them: `mia tmux keys kitty`
prints the lines for its config. They also give the app ⌘T (a new shell
window), ⌘W (close it), ⌘⇧[ and ⌘⇧] (the window before and after) and
⌘1–9 (a window by number), in place of the app's own.

## The one list

`prefix k` opens one list in the worktree you are in. What you type says
what Enter does, and the first row is what it does:

| typed | Enter |
|---|---|
| nothing | who waits for you (their answers numbered), who works, then the projects, the one here first |
| `project/` | that project's worktrees; for the project here, those on a machine too |
| a name | goes to that worktree, in any project; a project opens to its worktrees |
| a number | gives that answer to whoever has waited longest |
| `pi fix the test` | starts that agent here on that task (a second one is `pi-2`) |
| `> codex` | hands this conversation to another agent; `> opencode son` lists it on each model the words mean |
| `@` | moves this worktree to a machine, or brings one back |
| `new <branch>` | makes a worktree here and goes to it |
| `rm <name>` | removes one, its session of shells with it; one an agent runs in is refused |
| `stack` | the layers of the stack here, to go to one |
| `nvim`, `git`, `shell` | the editor, lazygit or a shell, in a new window |
| any other words | past conversations that mention them, in every project and on every machine; Enter resumes one |
| `?` | says all of this |

A worktree or conversation on another machine opens as that machine's
session in a window here, named `worktree@machine`. Past conversations are
the ones [`mia recall`](conversations.md#finding-one) finds; `mia mcp` gives agents the
same search.

What takes a while — a move, a removal, a new worktree — goes on after the
list closes, and says on the status line when it is done or why it failed.

## The popup terminal

`prefix K` is one shell in a popup over the window, kept between openings
and brought to the folder you were in when nothing runs in it. `mia cd
<worktree>` typed there takes the client under it to that worktree and
closes the popup. Anywhere in tmux, `mia go <worktree>` takes the client
there without changing folder.

## The status line

In a session mia made, the left says the session, `mia-<worktree>`, the
windows follow as `index:name`, and the right is left empty. mia still runs
there every few seconds, unseen: that is what remembers each session's
windows for [`mia resurrect`](worktrees.md) and has the machines asked
again every 30 seconds.

In any other session, the right end is mia's part, for the folder of the
pane you are in:

```
● monduli asks · wants Bash +2  ·  monduli due-dates 2/3 ⟳ ±4  ·  fedora ●
```

The top [signal](#signals) of the repository's worktrees and machines, and
how many more there are; the worktree and its branch; its layer of a
stack, with `⟳` when a layer has fallen behind the one under it; how many
files are uncommitted; and each machine, green while it answers, a red `○`
when it stops. It never waits on a machine: it reads their last answers.

## Pane kinds

Every pane mia starts is marked with what it is for, in tmux's pane option
`@mia_kind`, so tmux, mia and the apps read it the same:

| kind | when |
|---|---|
| `agent` | it runs an agent mia knows: `claude`, `codex`, `pi`, `opencode`, `agy`, `cursor-agent`, or one a plugin adds |
| `editor` | it runs `nvim`, `vim`, `vi`, `hx` or `emacs`, or was opened with `--editor` |
| `tests` | its command starts as a test runner does: `go test`, `cargo test`, `npm test`, `pytest`, `vitest`, `make test`… |
| `logs` | its command follows logs: `tail -f`, `journalctl`, `kubectl logs`, `docker logs`, `stern` |
| `server` | its command is a dev server mia knows (`npm run dev`, `pnpm dev`…) or what a `[[service]]` of the config runs |
| `shell` | anything else, and a window with no command |

`mia window new … --kind <kind>` says it instead. A pane opened by hand in
tmux has no kind. `tmux show-options -p -t <pane> @mia_kind` reads one.

A pane's border says what it is for: an agent its name (`claude`), an
editor its file (`nvim · main.go`), a server its name and port (`api
:8080`, as `PORT=`, `--port` or `-p` says it), tests `tests`, logs `logs`,
and anything else what runs and the folder, `shell · config`, following it
as it goes. mia keeps the fixed part in the pane option `@mia_label`; the
rest is tmux's own format, so a plain `tmux attach` shows the same titles.

A window tmux names, one opened with `prefix c`, is named the same way, so
two in one folder are told apart by what runs in them. `mia window new`
with no name names it so too, with `-2`, `-3` after it when that name is
taken. A window you rename keeps its name.

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

`prefix space` is a popup of the repository's worktrees, the ones that
need you first, so you can answer them without going there. A worktree
needs you when its best [signal](#signals) is ranked 50 or below: an agent
asks, tests went red, a task is done. The cursor opens on the first.

| key | does |
|---|---|
| `^y` | approve what it asks: presses its signal's `--yes` keys in the pane that set it |
| `^r` | reply: type a line, `⏎` sends it with Enter to that pane, `esc` drops it |
| `^x` | clear its signals |
| `⏎` | switch there; a worktree on another machine hands this terminal over to it |
| `?` | the selected worktree in full: its windows as they are now and its diff, `←` `→` between them; `?` again for the list |
| `tab` | only what needs you, only this computer's (`local`), or one machine's |
| `^n` | a new worktree |
| `^d` | remove it, after a `y`; one mia did not make only when nothing in it is uncommitted |
| `esc` | clear what is typed, then close |

`^y` and `^r` type only into the pane the signal was set from, and only
while that pane is still in the worktree's session. Once answered, the
signal is dropped from the radar; the agent's hooks say what it does next.

Under the ones that need you come the rest, in groups:

| group | holds |
|---|---|
| went down with tmux | how many sessions tmux took down, and a row that brings them back (`mia resurrect`) |
| active | the worktree you are in, an open session, uncommitted work, a stack, or anything touched in 14 days |
| on `<machine>` | each machine's worktrees, and whether it answers |
| ▸ idle | clean, no session, not starred, untouched for 14 days: what `mia gc` removes. Folded |
| ▸ not mia's | worktrees of the repository mia did not make, such as an agent's or another tool's. Folded, with how many hold changes |

`⏎` on a fold opens it, and its last row removes all of it at once, after a
`y` (idle ones with `mia gc --apply`, clean ones mia did not make with `mia
gc --others --apply`); branches stay. `⏎` on a worktree mia did not make
adopts it. A stack is one row: `→` opens it into its layers, and `⏎` on a
layer moves there, in place.

Type to find a worktree by name or branch. Under what is found come what
can be done with the first: switch there, bring its session back, move it
to a machine or back here, open it in an editor, start its environment,
restack, clear its signals, archive or remove it. A plugin's verbs are
found by typing too, and a name nothing has offers a new worktree of it.

## The menu

`prefix m` is tmux's own menu, built when you press it:

| key | does |
|---|---|
| `space` | the radar |
| `n` | a new worktree |
| `b` | bring back the sessions that went down with tmux (`mia resurrect --ask`) |
| `c` | clean up what is idle (`mia gc --ask`) |
| `o` | clean up the clean worktrees mia did not make (`mia gc --others --ask`) |

Each clean-up says how many there are, is greyed out at none, shows what
would go and asks `y` first; branches stay. Below them come the plugins'
verbs your config's `menu` names, such as `menu = ["type", "quiz"]`, each
on its first free letter.

## A new worktree

`prefix n` asks for the branch, with your `prefix` before it. Under it:

| line | says |
|---|---|
| `from` | the base branch, or the branch of the worktree you are in |
| `as` | when starting from a layer of a stack: `a layer, in place` or `a layer, own worktree` |
| `on` | `local`, or a machine; a machine that did not answer last is marked `○` |

`⇥` moves between the lines, `← →` changes one, `⏎` makes it while you
watch the steps (`l` shows the log), and `⏎` again goes there. `esc` leaves
it running. Inside a stack, `from` starts on the layer you are on.

The first time in a repository it shows what it guessed — the base
branch, the setup from the lockfile — and saves that as the repository's
config when the worktree is made.

## Signals

Anything outside mia can say something about a worktree: an agent's hook,
a test watcher, you. mia gives it no meaning; it shows it as its setter
asks.

```bash
mia signal <worktree|.> <kind> [text] [--style red] [--rank 10] [--from claude] [--for 10m | --until seen] [--yes 1]
mia signal <worktree> clear [--from claude]
mia signal ls
```

| option | does |
|---|---|
| `--style` | how it is drawn: `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`, `black`, `bold`, `dim`, `italics`, `underline` |
| `--rank` | lower comes first; 50 if none is given. 50 or below needs you; above, such as claude's `busy` at 60, only says how a worktree is |
| `--from` | who set it; one signal per source per worktree, `you` if none |
| `--for` | it ends after that long: `90s`, `10m`, `2h` |
| `--until seen` | it ends when you next switch to the worktree |
| `--yes` | the keys that approve it, which the radar's `^y` presses |

`.` is the worktree the command runs in, which is what a hook means. Set
with `.`, a signal goes when the process behind it does: the agent whose
hook set it or, with no agent above it, whatever ran the hook's shell. An
agent that asked and then died asks no more, even killed with its pane
left open. One set from a tmux pane keeps that pane, so the radar can
answer there, and goes when the pane does. A `--until seen` signal
outlives its process: a done is news after the agent has gone.

A note an agent leaves that should outlive it is a signal with no pane:
`TMUX_PANE= mia signal . note "tests green, ready for review" --rank 60
--from note`. Emptied so, it keeps no process either.

`mia signal ls` lists the signals still standing; `mia p ls` shows each
worktree's best at the end of its row. A machine's signals arrive with its
worktrees.

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
that name without that line is yours and is left as it is. A pi that was
never started in a folder is given nothing there.

| pi | signal |
|---|---|
| a run begins (`agent_start`) | `busy --style dim --rank 60 --from pi` |
| an extension's dialog waits for you during a run (`ui_prompt_start`) | `asks <its title> --style red --rank 10 --from pi`, and `busy` again once answered |
| pi goes on with nothing by itself (`agent_settled`) | `done --style green --rank 30 --from pi --until seen` |
| you stopped the run, or the session ends in one | `clear --from pi` |

It runs `mia signal .` in pi's folder. Where there is no `mia`, or the
folder is no worktree of mia's, the first call says so and no other is
made. pi waits for no call, each is ended after three seconds, and print
and JSON modes say nothing. pi asks no permission of its own before a
tool, so `asks` is what an extension asks — a question tool, a permission
gate. On a pi older than `agent_settled`, `done` is said a second and a
half after `agent_end` with no other run begun.

It also writes pi's reply as it is being written beside pi's session file,
as `<session file>.live`, so a [chat](chats.md) shows it live; the file
goes once pi has written the reply itself.
