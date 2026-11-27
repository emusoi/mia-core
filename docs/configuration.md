# Configuration

mia reads three TOML files, all optional. A later file wins over an earlier
one:

1. `~/.config/mia/config.toml`: yours, for every repository.
2. `.mia.toml` in the main checkout: the project's, committed and shared by
   every worktree and everyone who clones it. Keep credentials out of it.
3. `.git/mia/config.toml`: this repository's, on this computer only.

```bash
mia config                    # open the repository's own file in your editor
mia config show               # the three merged, as TOML
mia config show --json
mia config validate           # load and check all three
mia config base main          # set one key in the repository's own file
mia config --global agents.claude.model opus    # one in yours
mia config prefix             # what the repository's own file sets it to
```

`mia config <key> <value>` edits that one line, keeps the file's comments,
and refuses a value mia would not load. A dotted key names its table:
`env.page` is `page` under `[env]`, `agents.claude.model` is `model` under
`[agents.claude]`, `plugin.gh.token` is `token` under `[plugin.gh]`. Without
`--global` it writes `.git/mia/config.toml`, made with a commented starter
the first time; `.mia.toml` you edit by hand.

A key mia does not know is an error, never ignored, and so is a value it
cannot use: a name that is not a hostname, an `env.session` other than
`local` or `runtime`, a `[[service]]` without `id` or `run`.

When files merge, a key set later replaces the earlier value whole, lists
included. Tables of names (`[launch]`, `[agents]`, `[plugin]`) merge name by
name, but each named entry is replaced whole: an `[agents.claude]` in the
repository's file that says only `account` drops the `model` your own file
gave claude.

`mia config` opens `.git/mia/config.toml` in your editor, the first time
writing there a starter whose every line is a comment; `mia config base
staging` sets one key without it.

[examples/config.toml](../examples/config.toml) and
[examples/compose.toml](../examples/compose.toml) are files to copy from.

## Worktrees

Top-level keys, before any table.

| key | default | meaning |
|---|---|---|
| `base` | unset | the branch new branches start from, and the one stacks and "landed" are judged against. Unset, a new branch starts from what the main checkout has checked out, and stacks go by origin's default branch, else `main`, else `master` |
| `prefix` | `""` | put in front of every new branch name, like `"you/"` |
| `setup` | none | a command, as a list (`["npm", "install"]`), run once in each new worktree on this computer. `mia setup` runs it again; `mia new --bg` runs it in a `setup` window instead |
| `clone` | none | paths from the main checkout, globs allowed, copied into each new worktree before `setup`, copy-on-write where the disk allows: `["node_modules", "dist/*"]`. What the worktree already has is kept |
| `commit_times` | `false` | date a new worktree's files by their last commit, not the checkout, so tools that compare times see what the main checkout sees |
| `editor` | `$EDITOR`, then `$VISUAL` | what `mia config` and `mia window new --editor` open |
| `keep` | none | programs `mia resurrect` starts again even if they had ended, by their first words: `["tail -f"]`. Agents, `nvim`, `vim`, `hx`, `npm run dev`, `npm start`, `pnpm dev`, `pnpm run dev`, `bun dev`, `bun run dev` and `yarn dev` are kept already. Never name one that finishes, like a build |
| `names` | none | names to give worktrees before mia's own place names, best in your own file. Lowercase letters, digits and hyphens, since a name becomes a hostname and a tmux session |
| `plugins` | none | [plugins](plugins.md) enabled by the config, beside those `mia plugin enable` turned on; `mia plugin disable` refuses one named here |
| `menu` | none | the plugins' verbs [the one list](tmux.md#the-one-list) runs by name, each in a new window: `["type", "quiz"]` |

## `[launch]`

Named windows, each a command:

```toml
[launch]
logs = "tail -f log/dev.log"
claude = "claude --verbose"
```

`mia window new monduli logs` opens that window. A name that is an agent's
says how that agent is started: a window named for it, and a conversation
handed over to it, run that command.

## `[agents]`

What an agent is started with when nothing else says, each under its name:

```toml
[agents.claude]
model = "opus"
effort = "high"
account = "work"
```

| key | default | meaning |
|---|---|---|
| `model` | the agent's own | one of its models, as `mia api agents` lists them, or another the agent takes, which then becomes one of its choices |
| `effort` | the agent's own | one of its efforts, the same way |
| `account` | the agent's own folder | the [account](agents.md#an-agents-account) its sessions start on, one `mia account ls` lists |

It is used for every window that starts the agent unless the command already
says one, and for the main agent when `[main]` says none. `mia account use
<agent> <name>` says an account for this repository over both files, kept
by mia and written into neither; `--global` writes yours, and `default`
takes it out.

## `[main]`

In your own file, as you have one [main agent](main.md):

| key | default | meaning |
|---|---|---|
| `agent` | `claude` | which agent it is: any agent mia knows, one a plugin adds too. `mia main start --agent <name>` starts another, that once |
| `machine` | this computer (`local`) | the machine it lives on, by the name `mia machine list` gives. `mia main …` and `mia api main` then run there over ssh |
| `model`, `effort` | its agent's under `[agents]` | what it starts with when `mia main start` says none |
| `account` | its agent's under `[agents]` | the account it starts on when `mia main start` says none |

## `[env]`

The [environment](environments.md) a worktree's code runs in.

| key | default | meaning |
|---|---|---|
| `image` | see below | the container image. Unset: `.devcontainer/devcontainer.json`'s (or `.devcontainer.json`'s) `image`, else one for the project's language, else `mcr.microsoft.com/devcontainers/base:ubuntu` |
| `image_setup` | none | a command baked into an image of the repository's own by `mia env image`, used from then on |
| `setup` | none | a command run once in each new container |
| `ports` | none | ports the app serves; `mia env up --wait` waits for them to listen |
| `page` | the first listening port | the port `mia env browse` opens |
| `pass_host` | `false` | send the browser's `Host` (`monduli.mia:5173`) to the app; otherwise it gets `localhost:<port>`, which dev servers accept without setup |
| `host_ports` | none | ports on your computer the container reaches at `127.0.0.1`; needs `socat` in the image |
| `container_only` | none | paths in the worktree kept in a volume of the container's own, like `node_modules`; globs allowed |
| `cache` | none | absolute paths inside the container shared by all the repository's environments, like `/root/.npm` |
| `pass` | none | environment variables passed from your shell at `mia env up` |
| `session` | `"runtime"` | with the environment on another machine, `"runtime"` runs the worktree's shells and agents there, beside it; `"local"` keeps them in the checkout here |
| `sync_ignore` | none | relative paths or globs the copy on another machine leaves out, beyond the built-in ones |
| `sync_secrets` | `false` | let `.env*`, keys and credential files cross to another machine |

### `[env.compose]`

| key | default | meaning |
|---|---|---|
| `file` | unset | a Compose file in the worktree; set, the environment is that project instead of one container |
| `service` | `"app"` | the service setup, shells, `[[service]]` processes and the gateway use |

With `file` set, `image`, `image_setup`, `container_only`, `cache` and
`pass` are refused: Compose owns images, volumes and service environment.
See [Compose projects](environments.md#compose-projects).

## `[[service]]`

A program an environment keeps running. One block each:

| key | default | meaning |
|---|---|---|
| `id` | required | its name, unique |
| `run` | required | the command, as a list |
| `health` | none | a command that succeeds while it answers |
| `autostart` | `false` | start it with `mia env up` |
| `restart` | never | `"on-failure"`: start it again when it exits non-zero. `"on-unhealthy"` (needs `health`): also when `health` fails three times in a row, checked every 10 seconds |
| `grace` | `60` | seconds after a start before `health` counts; `mia env up --wait` waits at least the largest |

## `[tools]`

Files from your home carried into an environment's home.

| key | default | meaning |
|---|---|---|
| `copy` | none | copied on every `mia env up` and by `mia env tools`: `["~/.gitconfig"]` |
| `credentials` | none | placed only by `mia env tools --credentials`, mode 0600, never over a file mia did not place |
| `carry_credentials` | `false` | place `credentials` on every `mia env up` too |

## `[dotfiles]`

Your own setup, in your own file, carried wherever an environment runs:

| key | default | meaning |
|---|---|---|
| `env` | none | files put in every environment's home on `mia env up`; a path outside your home goes to the same path |
| `shell` | tmux's own, the image's in an environment | the shell of every session mia makes, on any machine (tmux's `default-shell`), and of `mia env shell` when the environment has it |
| `packages` | none | installed in the environment with `apt-get` when missing |
| `setup` | none | commands run with `sh -lc` in the environment on every `mia env up`, after the packages and files; keep them quick when already done |
| `machine` | none | files put in the home of the machine an environment runs on, where its tmux runs, on `mia env up` and `mia env dotfiles`; a running tmux there reads its config again |

## `[plugin.<name>]`

A plugin's own settings, any keys it reads, handed to it as they are:

```toml
[plugin.gh]
# that plugin's own settings
```

The plugin reads them as JSON in `MIA_PLUGIN_CONFIG`.

## `[keys]` and `[popup]`

These set the old dashboard's keys and popup size. They are still accepted,
so a config that has them loads, and nothing reads them.
