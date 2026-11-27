# Agents

An agent is a coding agent's own program, run in a window of a worktree's
session. mia starts it, with a model, an effort and an account when you
say, tells you when it needs you, and reads its conversations from where
the agent keeps them. It never wraps or replaces the agent.

| agent | conversations read | model switched while it runs | accounts |
|---|---|---|---|
| `claude` | yes | typed in: `/model`, `/effort` | yes |
| `codex` | yes | typed in: `/model … --reasoning=…` | yes |
| `pi` | yes | typed in: `/model`, `/thinking` | yes |
| `opencode` | yes | started again on its conversation | no |
| `agy` | yes | started again on its conversation | no |
| `cursor-agent` | no | no | no |

A [plugin can add an agent](plugins.md#an-agent). `mia api agents` lists
every one with its models and efforts, and whether it is installed.
opencode's and agy's models are what `opencode models` and `agy models`
print, asked again after an hour.

## Starting one

```bash
mia window new monduli claude                          # claude, in a window named claude
mia window new monduli pi --task "fix the flaky test"  # told that once it is ready
mia window new monduli claude -- claude "fix the login page"
```

A window named for an agent runs that agent; a second one beside the
first is `claude-2`. With a command after `--`, the window runs that.
`--task` is pasted into the agent once it is ready — when its screen has
stood still for a second, it asks something, or it ends — never into a
question it asks first.

In tmux, `pi fix the test` typed into the [one list](tmux.md#the-one-list)
does the same in the worktree you are in, and `prefix e` opens a new agent
window. A window named under `[launch]` that runs an agent is started the
same way, with its own command.

## An agent's model

An agent in a window is started with a model and an effort, and switched
to others while it runs:

```bash
mia window new monduli claude --model sonnet --effort high
mia window new monduli claude --model opus -- claude "fix the login page"
mia window model monduli claude haiku --effort low     # typed into it: /model haiku, /effort low
mia window new monduli opencode --model openai/gpt-5   # opencode mini --model openai/gpt-5
mia window model monduli opencode anthropic/claude-sonnet-5
```

With a command, the words that choose go after the word that runs it:
`claude --model sonnet …`. A model or an effort the agent does not have is
refused, with those it has. Left out, they are the ones `[agents.<name>]`
in the [config](configuration.md#agents) gives, unless the command already
says one.

`mia window model` types what the agent takes for it into its window, as a
paste, and Enter. opencode and agy have nothing to type, so mia ends the
agent and starts it again in the same pane on the conversation it has
open, with the model: the window stays. One with no conversation yet is
refused. opencode takes a model only in its minimal interface, so with one
it is started as `opencode mini --model <provider/model>`; it has no
effort mia can choose. Any other agent that cannot be switched while it
runs is refused: restart it with `--model`.

What mia says a window runs is what it started it with or last switched it
to, not what the agent says: switched by hand inside the agent, mia does
not know. Where mia chose none, a conversation of opencode's says the model
opencode says it runs on.

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
and is sent the body once it is ready, as `--task` is. What the front
matter leaves out is the default setup's: the agent the window is named
for, else the one used most in the repository, else the main agent's, and
`[agents.<name>]`'s model and effort. `--model`, `--effort` and `--account`
given with `--role` say over it. An agent mia does not know, or a model or
an effort it has not, is refused before any window opens.

The front matter is `key: value` lines between two `---` lines, of
`agent`, `model`, `effort` and `account` only; a file that starts without
one is all body. mia writes no role: write one with your editor.
`mia main start --role` starts [the main agent](main.md) on one.

## An agent's account

An agent can have several accounts — two claude logins, two codex ones —
and each session's agent starts on one. An account is a folder of the
agent's own, where it keeps its sign-in, its settings and its
conversations, named to it by a variable:

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
mia account use claude work --repository shop   # another repository's, by its main checkout's folder name
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
worktree — and says what it opened. You sign in there, with the agent's
own login: mia never reads, copies or moves a sign-in, a password or a
token. Outside tmux it says `tmux attach -t mia-accounts:<window>` goes
there, inside it `tmux switch-client -t …`, ready to paste.

`mia account login` opens the same window on an account the agent has
already, `default` too: one that says `logged_in` `no`, or whose sign-in
ran out. The agent's own folder is started as the agent finds it, its
variable not set — claude names its Keychain item after a folder the
variable names.

Each repository starts each agent's sessions on one account: the one
`mia account use` last said for it, kept by mia in
`~/.config/mia/accounts.json` and never written into a config; failing
that, `account` under `[agents.<name>]` in the config, the repository's own
or the user-wide one; failing that, the default. `mia account use
--global` writes the user-wide one, that line of `~/.config/mia/config.toml`
and no other, and `default` takes it out. A rename or a removal of the
account it names is followed there, as it is in `accounts.json`.

`--account` on `mia window new`, `mia conversation resume` and `mia main
start` starts one on another, that once: the next start in that worktree
is on the repository's again. What mia started a window on is kept for the
window, as its model is.

An agent takes its account when it starts, so one that runs is not moved
to another: `mia window account <worktree> <window> <account>` says how —
end the agent there, then `mia conversation resume <id> --account
<account>`. mia stops nothing itself.

A conversation lives in the folder of the account it was made on, and mia
looks in every account's folder for one: `mia recall --json` and `mia api
conversations` say which `account` each is in. Taken up on another
account, a conversation is copied into that account's folder; the one it
was copied from stays. See [Conversations](conversations.md#going-on-with-one).

Whether an account is signed in, `logged_in`, is read from whether the file
the agent writes then is there — claude's `.credentials.json`, codex's and
pi's `auth.json` — never from what is in it. On macOS claude keeps its
sign-in in the Keychain instead, under a name made from the folder's path —
`Claude Code-credentials` for its own folder, `Claude
Code-credentials-<8 hex of the path's SHA-256>` for any other — and codex
can keep its in the system's keyring when its config says so. mia does not
look there, so such an account says `unknown`; one whose folder is empty
but for its shared setup, as it is until the agent first runs there, says
`no`. For the same reason, a renamed claude account asks to be signed in
again on macOS, and the old Keychain item stays.

`mia account rm` removes the folder, and the conversations in it — one
that holds any is refused without `--force`. It is refused while a window
runs the agent on it, and for `default`. mia signs nothing out: a sign-in
the agent keeps elsewhere, in the Keychain, stays there until you sign out
in the agent first.

### Its setup, shared

An account's folder is the agent's whole folder, so a session on one would
start without your setup: claude's hooks that tell mia what it does, its
instructions, its skills. `mia account add` and `mia account login` link
that setup of the agent's own folder into the account's before the agent
starts there, and `mia account share <agent> <name>` or `--all` does it for
one there already. Each is a link to the agent's own, so a change to
either is the other's; removing the account removes the links and not what
they lead to.

What is linked is named one by one, and only what the agent's own folder
has:

| agent | linked |
|---|---|
| claude | `settings.json`, `CLAUDE.md`, `skills`, `commands`, `agents`, `plugins`, `output-styles`, `hooks`, `statusline.sh`, `statusline-command.sh` |

Nothing else is linked or read: not the sign-in (`.credentials.json`), not
`.claude.json`, which says the account claude is signed in as, and not its
conversations or state — `projects`, `sessions`, `todos`, `history.jsonl`,
`shell-snapshots`, `statsig`, `ide`, caches and backups. A name the account
has of its own — a file, a folder or a link elsewhere — is kept, and said;
only one missing, or a link that leads nowhere, is linked. Run again, it
links what is still missing and nothing more.

codex and pi have nothing shared yet: `mia account share` says so. What
tells mia what pi does is written apart, when asked: [`mia account signals
pi`](tmux.md#pi-says-what-it-does).
