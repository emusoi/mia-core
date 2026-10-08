# Configuration

mia reads two TOML files, and both are optional:

| file | for |
|---|---|
| `~/.config/mia/config.toml` | you: your keys, your dotfiles, settings for every repository |
| `.git/mia/config.toml` | one repository; wins over the first for any key it sets |

`mia config` opens the repository's file in your editor, writing a commented
starter the first time. `XDG_CONFIG_HOME` moves `~/.config` as usual.

**Unknown keys are refused**, not ignored: a typo is an error naming the key,
rather than a setting that silently does nothing. The one exception is
`[plugin.<name>]`, which belongs to that plugin.

[`examples/config.toml`](../examples/config.toml) uses every key.

## Worktrees

| key | type | default | meaning |
|---|---|---|---|
| `base` | string | the repository's default branch | what a new branch starts from, and what *ahead* and *behind* count against |
| `prefix` | string | — | put in front of every new branch name (`"you/"`); left off in the dashboard |
| `setup` | list | — | a command run on your machine in every new worktree; `mia setup` runs it again |
| `clone` | list of globs | — | files copied from the main checkout into a new worktree, copy-on-write, before setup (`["node_modules", "dist/*"]`) |
| `commit_times` | bool | `false` | date tracked files by their last commit, not the checkout, so build tools that compare times see what the main checkout sees |
| `editor` | string | `$EDITOR`, then `$VISUAL` | the editor for `mia config`, editor windows, and plugins that open files |

Commands are lists — `["npm", "install"]` — run without a shell; use
`["sh", "-c", "…"]` when you need one.

## `[launch]`

Named windows, started with `mia window new <worktree> <name>` or `n` in the
windows panel. Values are shell command lines.

```toml
[launch]
test = "go test ./..."
logs = "tail -f log/dev.log"
```

## `[env]`

The container a worktree's code runs in. See [environments](environments.md).

| key | type | meaning |
|---|---|---|
| `image` | string | the image, overriding `devcontainer.json` and the language guess |
| `image_setup` | list | baked into an image by `mia env image`; for system packages |
| `setup` | list | run inside a new container; for the project's own install |
| `ports` | list of ints | ports the app serves, shown in the environment panel |
| `page` | int | the port `mia env browse` opens |
| `host_ports` | list of ints | ports on your machine the container reaches as `localhost` (needs `socat` in the image) |
| `pass_host` | bool | send the original `Host` header instead of `localhost:<port>` |
| `container_only` | list of globs | paths kept in the container's own volumes, not the worktree (`["node_modules"]`) |
| `cache` | list of paths | absolute paths inside the container kept in volumes shared by the repository's environments |
| `pass` | list of names | environment variables passed from your shell |

## `[[service]]`

A process an environment keeps running; repeat the table for each.

| key | type | meaning |
|---|---|---|
| `id` | string | its name |
| `run` | list | the command |
| `health` | list | exits 0 when it is answering |
| `autostart` | bool | start it on `mia env up` |
| `restart` | `"on-failure"` or `"on-unhealthy"` | when to start it again; absent means never |
| `grace` | int | seconds before health is first checked (default 60) |

## `[tools]`

Files carried from your machine into environments.

| key | type | meaning |
|---|---|---|
| `copy` | list of paths | carried on every `mia env up` and `mia env tools` |
| `credentials` | list of paths | carried only with `mia env tools --credentials`, mode `0600` |
| `carry_credentials` | bool | carry credentials on every `mia env up` too |

## `[dotfiles]`

Your own setup, for `~/.config/mia/config.toml`.

| key | type | meaning |
|---|---|---|
| `env` | list of paths | into every environment's home |
| `shell` | string | the shell `mia env shell` and new windows run, when it is installed |
| `packages` | list | installed in an environment when missing (apt, as root) |
| `setup` | list | run in every environment after the files land |
| `machine` | list of paths | into the home of another machine environments run on (your `.tmux.conf`, say) |

## `[keys]` and `[popup]`

```toml
[keys]
machines = "B"
down = ["j", "down"]

[popup]
width = "80%"
height = "75%"
```

See [the dashboard](dashboard.md#rebinding-keys).

## Plugins

| key | type | meaning |
|---|---|---|
| `plugins` | list of names | plugins enabled for this repository, beside those `mia plugin enable` turned on everywhere |
| `[plugin.<name>]` | table | that plugin's own settings, handed to it as JSON |

```toml
plugins = ["gh"]

[plugin.notify]
events = ["env.up"]
```

See [plugins](plugins.md).
