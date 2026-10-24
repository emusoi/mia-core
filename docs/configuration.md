# Configuration

Three TOML files, all optional, later files taking precedence:

1. `~/.config/mia/config.toml`: yours, for every repository.
2. `.mia.toml` in the main checkout: committed project configuration, shared
   by every worktree and by everyone who clones the repository.
3. `.git/mia/config.toml`: this repository's local overrides; `mia config`
   opens it. It is never committed.

Inspect the merged result with `mia config show` (TOML) or
`mia config show --json`. `mia config validate` checks all three files and
the service declarations. Unknown keys are an error.
[examples/config.toml](../examples/config.toml) uses every key; copy it to
`.mia.toml` for project settings or `.git/mia/config.toml` for local ones.
Keep credentials out of the committed file.

## Worktrees

| key | meaning |
|---|---|
| `base` | the branch new branches start from |
| `prefix` | added to new branch names, like `"you/"` |
| `setup` | a command run in each new worktree |
| `clone` | files copied from the main checkout into new worktrees |
| `commit_times` | date files by their last commit |
| `editor` | your editor; otherwise `$EDITOR` |
| `names` | places to name worktrees after, used before mia's own |
| `[launch]` | named windows: `logs = "tail -f log/dev.log"` |

## `[env]`

| key | meaning |
|---|---|
| `image` | the container image |
| `image_setup` | baked into the image |
| `setup` | run in each new container |
| `ports` | ports the app serves |
| `page` | the port `mia env browse` opens |
| `host_ports` | ports on your computer the container can reach |
| `pass_host` | send the original `Host` header |
| `container_only` | paths kept inside the container, like `node_modules` |
| `cache` | container paths shared by the repository's environments |
| `pass` | environment variables passed from your shell |

## `[[service]]`

`id`, `run`, `health`, `autostart`, `restart` (`"on-failure"` or
`"on-unhealthy"`), `grace` (seconds).

## `[tools]` and `[dotfiles]`

| key | meaning |
|---|---|
| `[tools] copy` | files copied into environments |
| `[tools] credentials` | copied only with `mia env tools --credentials` |
| `[dotfiles] env` | files for every environment's home |
| `[dotfiles] shell` | the shell in environments |
| `[dotfiles] packages` | packages installed in environments |
| `[dotfiles] setup` | run in every environment |
| `[dotfiles] machine` | files for other machines' home |

## Keys, popups, plugins

```toml
[keys]
machines = "B"
down = ["j", "down"]

[popup]
width = "80%"
height = "75%"

plugins = ["gh"]

[plugin.gh]
# that plugin's own settings
```
