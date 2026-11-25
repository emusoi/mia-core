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
mia p ls                   # every worktree; ★ starred, dirty, session, +ahead −behind
mia ls                     # every project mia knows, from any folder
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
mia window press monduli agent -- 2  # type without Enter: an agent's menu
mia window key monduli agent -- C-c  # a key by name: stop what runs, keep the window's session
mia window close monduli logs
```

Name windows you start often under `[launch]` in the config, then
`mia window new monduli logs`.

## An agent's model

An agent in a window is started with a model and an effort, and switched
to others while it runs:

```bash
mia window new monduli claude --model sonnet --effort high   # claude, in a window named for it
mia window new monduli claude --model opus -- claude "fix the login page"
mia window model monduli claude haiku --effort low           # typed into it: /model haiku, /effort low
mia window new monduli opencode --model openai/gpt-5         # opencode mini --model openai/gpt-5
mia window model monduli opencode ollama/qwen3:4b            # started again on its conversation, with the model
```

With no command, a window named for an agent runs that agent. With one,
the words that choose go after the word that runs it: `claude --model
sonnet …`. A model or an effort the agent does not have is refused, with
those it has; `mia api agents` lists them. Left out, they are the ones
`[agents.<name>]` in the config gives, for every agent mia starts in a
window — unless its command already says one.

`mia window model` types what the agent takes for it into its window, as
a paste, and Enter. opencode has nothing to type and takes a model only
as it starts — as its minimal interface, `opencode mini --model
<provider/model>`, the only one with a word for it — so mia ends it and
starts it again in the same pane on the conversation it has open, with
`--session <id>`: the window stays. One with no conversation yet is
refused. Its models are those `opencode models` lists; it has no effort
mia can choose. Any other agent that cannot be switched while it runs is
refused: restart it with `--model`. mia does not restart it. What mia
says a window runs is what it started it with or last switched it to,
not what the agent says: switched by hand in the agent, mia does not
know — but where mia chose none, a conversation of opencode's says the
model opencode says it runs on.

## A role

A role is a setup with opening instructions: a file of yours,
`~/.config/mia/roles/<name>.md`, that says which agent, model, effort and
account to start, all optional, and below them what to tell the agent
first.

```markdown
---
agent: claude
model: opus
effort: high
---
You review. Read the diff against main and say what is wrong with it.
```

```bash
mia window new monduli --role reviewer          # claude, opus, effort high, in a window named claude
mia window new monduli review --role reviewer   # the same, in a window named review
mia role ls                                     # every role, and what is wrong with one that cannot be read
mia role show reviewer
mia role path reviewer                          # where to write it
```

The agent starts as `--model`, `--effort` and `--account` would start it,
and is sent the body once it is ready — when it has drawn the same for a
second, asks something or ends, as `mia main start` waits — pasted, so a
line break is not an Enter, and Enter after. Into a question nothing is
sent: the window stays, and mia says what it asks. What the front matter
leaves out is the default setup's: the agent the window is named for,
else the one used most in the repository, else the main agent's, and
`[agents.<name>]`'s model and effort. `--model`, `--effort` and
`--account` given with `--role` say over it. An agent mia does not know,
or a model or an effort it has not, is refused before any window opens.
The front matter is `key: value` lines between two `---` lines, of
`agent`, `model`, `effort` and `account` only; a file that starts with
none is all body. mia writes no role: write one with your editor.

## An agent's account

An agent can have several accounts — two claude logins, two codex ones —
and each session's agent starts on one. An account is a folder of the
agent's own that it keeps its sign-in, its settings and its
conversations in, named to it by a variable:

| agent | variable | its own folder, the account called `default` |
|---|---|---|
| claude | `CLAUDE_CONFIG_DIR` | `~/.claude` |
| codex | `CODEX_HOME` | `~/.codex` |
| pi | `PI_CODING_AGENT_DIR` | `~/.pi/agent` |

An agent a [plugin adds](plugins.md#an-agent) has accounts when its
manifest names its variable, `account_env`. mia leaves the agent's own
folder as it is; every other account is a folder of mia's,
`~/.config/mia/accounts/<agent>/<name>`, made with mode 0700.

```bash
mia account add claude work              # its folder, and a window running claude on it
mia account add codex spare --in monduli # the window in a worktree's session instead
mia account ls                           # every account, signed in or not, and what runs on it
mia account use claude work              # this repository's claude sessions start on work from now on
mia account use claude default           # and back to what the config says
mia account use claude work --global     # every repository that says none starts claude on work
mia account login claude work            # signed out, or run out: a window to sign in to it again
mia account share claude --all           # ~/.claude's settings, CLAUDE.md, skills… linked into each
mia window new monduli claude --account default   # this one on its own folder, this once
mia account rename claude work team
mia account rm claude team
```

`mia account add` makes the folder and opens a window running the agent
on it — in a session of mia's own, `mia-accounts`, unless `--in` names a
worktree — and says what it opened. The person signs in there, with the
agent's own login: mia never reads, copies or moves a sign-in, a
password or a token. Outside tmux it says `tmux attach -t
mia-accounts:<window>` goes there, inside it `tmux switch-client -t …`,
written to be pasted into any shell as it is.

`mia account login` opens the same window on an account the agent has
already, `default` too: one that says `logged_in` `no`, or whose sign-in
ran out. The agent's own folder is started as the agent finds it, its
variable not set to it — claude names its Keychain item after a folder
the variable names.

Each repository starts each agent's sessions on one account: the one
`mia account use` last said for it, kept by mia in
`~/.config/mia/accounts.json` and never written into a config; failing
that, `account` under `[agents.<name>]` in the config, the repository's
own or the user-wide one; failing that, the default. `mia account use
--global` writes the user-wide one, that line of
`~/.config/mia/config.toml` and no other, and `default` takes it out; a
rename or a removal of the account it names is followed there, as it is
in `accounts.json`. `--account` on
`mia window new`, `mia conversation resume` and `mia main start` starts
one on another, that once: the next start in that worktree is on the
repository's again. What mia started a window on is kept for the
window, as its model is.

### Its setup, shared

An account's folder is the agent's whole folder, so a session on one
would start without the person's setup: claude's hooks that tell mia
what it does, its instructions, its skills. `mia account add` and `mia
account login` link that setup of the agent's own folder into the
account's before the agent starts there, and `mia account share <agent>
<name>` or `--all` does it for one there already. Each is a link to the
agent's own, so a change to either is the other's; removing the account
removes the links and not what they lead to.

What is linked is named one by one, and only what the agent's own
folder has:

| agent | linked |
|---|---|
| claude | `settings.json`, `CLAUDE.md`, `skills`, `commands`, `agents`, `plugins`, `output-styles`, `hooks`, `statusline.sh`, `statusline-command.sh` |

Nothing else is linked or read: not the sign-in (`.credentials.json`),
not `.claude.json`, which says the account claude is signed in as, and
not its conversations or state — `projects`, `sessions`, `todos`,
`history.jsonl`, `shell-snapshots`, `statsig`, `ide`, caches and backups.
A name the account has of its own — a file, a folder or a link elsewhere
— is kept, and said; only one missing, or a link that leads nowhere, is
linked. Run again, it links what is missing still and nothing more.
codex and pi have nothing shared yet: `mia account share` says so. What
tells mia what pi does is written apart, when asked: [`mia account
signals pi`](tmux.md#pi-says-what-it-does).

An agent takes its account when it starts, so one that runs is not put
on another: `mia window account <worktree> <window> <account>` says how —
end the agent there, then `mia conversation resume <id> --account
<account>`. mia stops nothing itself.

A conversation lives in the folder of the account it was made on, and
mia looks in every account's folder for one: `mia api conversations`,
`mia recall` and `mia why` say which `account` each is in. Taken up on
another account, a conversation is copied into that account's folder,
as one taken up in another worktree is copied under that folder's name;
the one it was copied from stays.

Whether an account is signed in, `logged_in`, is read from whether the
file the agent writes then is there — claude's `.credentials.json`,
codex's and pi's `auth.json` — never from what is in it. On macOS claude
keeps its sign-in in the Keychain instead, under a name made from the
folder's path — `Claude Code-credentials` for its own folder,
`Claude Code-credentials-<8 hex of the path's SHA-256>` for any other —
and codex can keep its in the system's keyring when its config says so.
mia does not look there, so such an account says `unknown`; one whose
folder is empty but for its shared setup, as it is until the agent first
runs there, says `no`.
For the same reason, a renamed claude account asks to be signed in to
again on macOS, and the old Keychain item stays.

`mia account rm` removes the folder, and the conversations in it — one
that holds any is refused without `--force`. It is refused while a
window runs the agent on it, and for `default`. mia signs nothing out:
a sign-in the agent keeps elsewhere, in the Keychain, stays there until
the person signs out in the agent first.

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
under its old name when that is free, with the work uncommitted as it was,
what was staged staged again. What git ignores is not kept.

`fork` starts a new branch where a worktree is, not at base, and copies
its uncommitted and untracked work in; the first is left as it is.

## After a crash

Every 30 seconds or so, while tmux shows mia's status line or a session of
mia's is in front, mia notes each
session's windows, and each pane a window is split into: their folders and
what runs in them. When tmux or the
computer goes down, `mia resurrect` opens each one again, its programs
started anew and each claude resuming the very conversation it had; codex,
pi, opencode and cursor-agent go on with their folder's last. Each
starts on the account it ran on, or the default one when that account was
removed since. An
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

Both read the agents' own transcripts of this repository and its
worktrees, so nothing is recorded as you work, and both say what you asked
there and how to resume it. `mia why` reads claude's; `mia recall` reads
claude's, codex's, pi's, opencode's and agy's, and those of any agent a
[plugin adds](plugins.md#an-agent).

```bash
mia conversation resume 23f98c02-…              # a window where it was made, the agent taking it up
mia conversation resume 23f98c02-… --in monduli # or in another worktree
mia conversation resume 23f98c02-… --model sonnet --effort high
```

`mia conversation resume` opens a window in the worktree's session running
the agent's own resume — `claude --resume <id>`, `codex resume <id>`,
`pi --session <id>`. A conversation a window already has open is left as it
is, and said so. One whose worktree is gone is refused, with how to bring
the worktree back; `--in` takes it up in another instead. claude and pi
keep a conversation under its folder's name, so for another worktree mia
first copies it under that folder's, its path rewritten, as a move does;
the first stays where it was. From then on the conversation is in both,
each going its own way: `mia conversation resume <id>` takes up the one
last written to, and `--worktree <name>` the one in that worktree.

### With another agent

```bash
mia conversation continue 23f98c02-c52b-4de9-8c69-b322eb153bff --with codex   # that conversation, on with codex
mia conversation continue --from claude --with pi             # the one the window claude has open, here
mia conversation continue 23f98c02-… --with opencode --dry-run   # only says: the file, the window, the message
```

No agent loads another's conversation: each keeps its own format. So to
go on with another, mia writes the conversation out as a handoff,
`.git/mia/handoffs/<worktree>/<agent>-<id>.md` in the repository's git
folder — the worktree stays as it was — and opens a window in the
session of the worktree the conversation is in with the other agent, as
`mia window new <worktree> <agent>` starts one: `[launch]`'s command of
that name when it runs the agent, `--model`, `--effort` and `--account`
or the configured ones. Once the agent is ready it is sent one message:
to read that file, go on from where it stops, and look at the worktree
before changing anything.

Which conversation is said exactly, by its id, as `mia api
conversations` and `mia recall` give it: a worktree can have three open
at once, and none is taken for the one meant. Its worktree, its session
and the agent it was with are the conversation's own. One that was
copied to another worktree is in both under one id, so `--worktree
<name>` says whose. `--from <window>` says it by the window that has it
open instead, by the window's name or number, in the session of the
worktree you are in or of `--worktree`: when that is one window with one
conversation open it is that one, and otherwise the command is refused
and lists the ids of those open there.

A handoff carries over less than the conversation was. It says where it
came from and when, then the conversation in order: what you and the
agent said, as written, and each thing the agent did as one line — `Ran
go test ./...`, `Edited view.lua +2 −1` and the file. What came back
from a tool is not in it, and nothing the agent only thought: the other
agent reads the files and runs the commands again for that, and the
command says so in its last line. A message over 6,000 characters is
cut, and says by how much. A conversation over 60,000 characters keeps
the first thing you asked and the last turns, as many as fit, whole; the
turns between are one line saying how many are left out and naming
`<agent>-<id>-full.md` beside it, which has every turn, for the agent to
look up what it needs. What you typed into the conversation is in these
files, which only you can read, and which are written again each time.
It is the other agent's to read, not its memory: it knows what the file
says and what the worktree shows, no more.

So with the same agent nothing is handed over. `--with` the agent the
conversation is with is refused, and says what goes on with nothing
lost: `mia conversation resume <id>`, the agent's own resume, with
`--model` for another model, or `mia window model` for the window that
has it open.

Each time is also kept, `<agent>-<id>.<when>.json` beside the handoff:
which conversation went on with which agent, in which window, when. The
conversation the other agent then begins is told by its first message,
which names the handoff's file, and from then on `mia api conversations`
says the two are [one thread](api.md#conversations): `continues` on the
new one, `continued_by` on the first.

The agent is typed into as [a role's opening](#a-role) is: never into a
question it asks first, and never into the shell left by one that ended
as it started. Then the window is open, the command fails, and the
message is printed for you to say there.

```
monduli: claude's conversation 23f98c02-…, 41 turns, written out to /src/app/.git/mia/handoffs/monduli/claude-23f98c02-….md
monduli: window codex opened — codex
monduli: window codex was told to read it and go on from where it stops
not carried over: what claude thought and what came back from its tools — codex has what was said and each thing done, in a line
```

The agent is started with a model and an effort as [a window's
is](#an-agents-model): `--model` and `--effort`, or `[agents.<name>]`'s
when left out, the words put after the one that runs it — `claude
--model sonnet --resume <id>`. One it does not have is refused, with
those it has, and nothing is opened. One a window has open already is not
started again with another: `mia window model` switches it, or stop it —
`mia window key <worktree> <window> -- C-c` — and resume it again.

`mia api conversations` lists them all and `mia api conversation <id>`
reads one as turns: see [Automation](api.md#conversations).

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

`mia shell <worktree>@fedora` is its session there, and `mia run` and
every `mia window` verb take `<worktree>@fedora` too: fedora's own mia
answers. Its branches arrive
here as `fedora/<branch>` on every refresh; pushing them anywhere is yours.

## Moving a worktree

```bash
mia move monduli fedora          # from here to fedora
mia move monduli@fedora local    # and back
mia move --dry-run monduli fedora    # what would cross; nothing moves
```

The branch goes across, and so does what is not committed: changed files
as a patch, new files as they are. The session's windows open again
there, in the same folders, each with the command that was running in it
typed in again — claude, nvim, a dev server. Only once all of that is
there is the worktree removed where it was; its branch stays. What is
staged stays staged. A file git ignores crosses too when it is a file of
at most 1 MiB — a `.env`, a local setting — with its mode; one the other
side already has differently is not written over: the move is refused. A
folder git ignores (`build/`, `node_modules/`) and an ignored file over
1 MiB do not cross: the move names them, and they go with the worktree
where it was.

A running program starts again rather than continuing. One that keeps
state can bring it along through an extension named `mia-carry-<program>`
on your PATH: mia asks it to `pack <from> <to>` — a tar of files under the
home, which is unpacked on the other side — and how to `command` it to
start again. `scripts/mia-carry-claude` carries claude's conversation and
restarts it with `claude --continue`.

A window's agent that runs on an account starts again on the account of
the same name there, and its conversation is carried into that
account's folder; with none of that name there, on the default one, and
the move says so. A sign-in never crosses: sign in to the account on
each machine.
