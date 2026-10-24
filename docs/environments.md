# Environments

An environment is a container where a worktree's code runs. It needs
podman or docker.

```bash
mia env up                 # start it
mia env shell              # a shell inside
mia env browse             # open the app in your browser
mia env down               # stop it
mia env rm                 # remove it
```

## What a container is

One worktree, at most one container, named `mia-<name>`. It is a plain
Linux box with your code in it:

- **Your files, live.** The folder holding your worktrees is mounted at the
  same path inside, so an edit on your computer is there at once and `git`
  works inside too.
- **Nothing running by itself.** The container only waits. Your shell,
  commands and [services](#services) run in it through mia.
- **No published ports.** Each container listens on its own ports, so two
  apps on port 5173 don't collide. You reach them at `<name>.mia`.
- **Kept until removed.** `mia env down` stops it and keeps everything;
  `mia env up` starts it again. `mia env rm` throws it away, and the next
  `mia env up` starts fresh.

## Setting one up

Start with nothing and add only what is missing.

**1. Bring it up.**

```bash
mia env up
```

mia picks the image from the first of:

1. `[env] image` in the config;
2. the `image` in `.devcontainer/devcontainer.json`;
3. an image for the project's language (`go.mod`, `Cargo.toml`,
   `package-lock.json`, `pyproject.toml`, `Gemfile`, …);
4. `mcr.microsoft.com/devcontainers/base:ubuntu`.

It prints which one it pulled and why.

**2. Look around.**

```bash
mia env shell
```

Install, run the app and the tests the way you would on a fresh computer.
Whatever you had to type goes into one of two places.

**3. System packages go in the image.**

```toml
[env]
image_setup = ["sh", "-c", "apt-get update && apt-get install -y postgresql-client socat"]
```

```bash
mia env image              # build it once, on this machine
```

The built image is used from then on and survives `mia env rm`. Change
`image_setup` and build again for a new one.

**4. The project's own install goes in setup.**

```toml
[env]
setup = ["npm", "install"]
container_only = ["node_modules"]
cache = ["/root/.npm"]
```

`setup` runs once in each new container; `mia env setup` runs it again.
`container_only` keeps a folder inside the container, so the Linux
`node_modules` never mixes with your computer's. `cache` is shared by all
the repository's containers, so the second install is fast.

Check it from scratch:

```bash
mia env rm && mia env up
```

Changing the image, setup, volumes, passed environment or services does not
silently rebuild an existing container. `mia env up` warns when they differ.
Apply the new configuration with:

```bash
mia env up --recreate
```

Recreation replaces the container and reruns setup, but keeps its named
volumes and remote sync. `mia env rm` is the destructive operation: it also
removes the worktree's private volumes.

**5. Start the app as a service.**

```toml
[[service]]
id = "web"
run = ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
autostart = true
```

Serve on `0.0.0.0`, not `127.0.0.1`, so the gateway can reach it.

**6. Open it.**

```bash
mia env browse --direct   # HTTP; no proxy or certificate setup
mia env browse            # HTTPS, after the gateway setup below
```

**7. Reach your computer, if you need to.** A database running on your
computer, not in the container:

```toml
[env]
host_ports = [5432]
```

Inside, `127.0.0.1:5432` reaches your computer's 5432. It needs `socat` in
the image.

## A base configuration

All of the above in one file, for a web app. `mia config` opens it; copy
this and change what differs.

```toml
[env]
setup = ["npm", "install"]
container_only = ["node_modules"]
cache = ["/root/.npm"]
ports = [5173]
page = 5173

[[service]]
id = "web"
run = ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true
restart = "on-failure"
```

For Python, the same shape:

```toml
[env]
setup = ["sh", "-c", "pip install -e ."]
cache = ["/root/.cache/pip"]
ports = [8000]
page = 8000

[[service]]
id = "api"
run = ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000", "--reload"]
autostart = true
```

## Compose projects

An app that needs Postgres, Redis or other sidecars can use its existing
Compose file:

```toml
[env.compose]
file = "compose.yaml"
service = "app"
```

The Compose project is named `mia-<worktree>`, so containers and networks
from different worktrees do not share names. `service` is `app` by default.
It is where setup, shells, exec and `[[service]]` processes run, and the
service the gateway reaches. Other services start and stop with the project.

Use `expose`, not published `ports`, and do not use `network_mode: host`.
Mia refuses either anywhere in the resolved Compose config: published ports
would collide and host networking defeats isolation. If your normal dev file
publishes ports, make a `compose.mia.yaml` variant or override that resolves
to `expose` only.

```bash
mia env up --wait
mia env service
mia env down
mia env rm
```

`up` reconciles the project through Compose. `--recreate` forces down/up.
`down` stops containers; `rm` removes containers and networks. Compose
volumes, including databases, are kept by all three. External volumes are
never removed. The actual service container ID is resolved with
`compose ps -q`, so custom `container_name` and supported Compose providers
work. Switch between Compose and the single-container path with `mia env rm`
before changing `env.compose`; the old mode is not discovered automatically.

Compose owns images, service environment and volumes, so
`env.image`, `env.image_setup`, `env.pass`, `env.container_only` and
`env.cache` cannot be combined with it. `env.setup`, ports, host forwarding,
dotfiles and services still work in the selected service. `mia doctor`
checks for a Compose plugin with JSON config and `ps -q` support.

## Main's dev server

No container needed. If your dev server is already running in the main
checkout, lend it a worktree's files:

```bash
mia dev monduli            # main's dev server now shows monduli
mia dev monduli --follow   # and keeps copying as you edit
mia dev                    # whose files main has
mia dev off                # main gets its own files back
```

This is the only time mia writes into a working tree. It puts main's own
changes aside in a stash, then copies the worktree's files over main,
uncommitted ones included. Ignored files such as `node_modules` and `.env`
stay as they are, so the server keeps running and reloads. `mia dev off`
resets main and restores the stash. Don't commit in main while it is lent.
On the dashboard, `V` lends the selected worktree; `V` on main gives it back.

## Direct HTTP

For ordinary local web apps, no browser setup is needed:

```bash
mia env browse --direct
```

This opens `http://<name>.localhost:7842/`, which reaches the configured
`[env] page`, or the first listening port when no page is configured. Direct
browsing cannot choose another port; use HTTPS below for that.

## `<name>.mia`

For HTTPS, open the app at `https://<name>.mia:<port>/` instead of
`localhost:<port>`:

```bash
mia env browse
```

The gateway needs two settings on your computer, once:

```bash
mia gateway setup          # prints both
mia gateway trust          # trusts mia's certificate (macOS)
```

The other is pointing your system's automatic proxy setting at
`http://127.0.0.1:7842/proxy.pac`. `mia env up` starts the gateway.
Containers publish no ports of their own.

## Services

```toml
[[service]]
id = "web"
run = ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true
restart = "on-failure"
```

`autostart` starts it with `mia env up`; `health` and `restart` keep it
running.

```bash
mia env up --wait
```

`--wait` waits for every autostart service to be running and healthy (when
it has a health check), and for every port in `[env] ports` to listen. It
times out after one minute, or the largest configured service `grace` when
that is longer. A timeout exits non-zero and leaves the environment running
for inspection.

```bash
mia env service            # each service and its state
mia env service logs web   # the last 40 lines
mia env service logs -f web
```

## Your files

```toml
[tools]                    # in the repository's config
copy = ["~/.gitconfig"]

[dotfiles]                 # in ~/.config/mia/config.toml
env = ["~/.zshrc"]
shell = "zsh"
```

`[env] pass = ["NAME"]` passes environment variables from your shell.

## Many environments

Stop every recorded environment in this repository without removing their
containers, volumes or sync state:

```bash
mia env down --all
```

`mia doctor` checks local tools and configuration, registered machines,
remote tmux, plugin manifests and environment configuration drift.

## Other machines

```bash
mia machine add build me@build.example.com
mia env host build         # move the environment there
mia env up
```

The URL stays the same. mia keeps a copy of the worktree there in sync with
[mutagen](https://mutagen.io/documentation/introduction/installation).
`mia env host local` moves it back. An image built with `mia env image` is
built per machine: run it again there.

A move preflights the target and aborts if old-environment cleanup fails.
The worktree record keeps its old placement so the move can be retried.
Private container volumes are not migrated: the target starts fresh, and
the old environment and its private volumes are removed after preflight.

### Git and sessions

The remote staging copy deliberately excludes `.git`; syncing Git metadata
two ways would corrupt it. By default, worktree shells and agent windows run
on the runtime beside the environment, so they cannot commit or inspect the
local Git branch.

For agents that need Git, keep their sessions in the local worktree while
the environment still runs remotely:

```toml
[env]
session = "local"
```

`mia shell` and agent windows then use the local checkout. `mia env shell`
always opens a shell inside the environment on its runtime.

### Sync and secrets

Remote sync is two-way-safe. New sessions exclude `.env*`, private keys and
common credential files, in addition to `.git`, dependencies and build
outputs. Add project exclusions with:

```toml
[env]
sync_ignore = ["secrets/*", "*.local"]
```

Preview paths and effective rules without reading file contents:

```bash
mia env sync monduli preview
mia env sync monduli preview --json
```

If the machine is trusted and these files must cross, explicitly set
`sync_secrets = true`. This allows every matching secret pattern, not just
one file. Excluding a file stops future synchronization; it does not delete
a copy already on the machine. Remove old remote copies explicitly.

### Sync operations

```bash
mia env sync monduli status     # mode and health
mia env sync monduli conflicts  # conflicted paths, or "no conflicts"
mia env sync monduli auto       # follow changes live, both ways
mia env sync monduli manual     # pause after settling current changes
mia env sync monduli now        # flush once, preserving the mode
mia env sync monduli restart    # recreate the session with current rules
```

`status`, `conflicts` and `preview` also take `--json`. Changed ignore rules
require `restart`; ordinary `up`, `auto`, `manual` and `now` refuse to keep
using an old rule set. Sessions created before rules were labelled need one
explicit restart. Restart keeps files on both sides but resets Mutagen's
session history; any conflicts are then listed with `conflicts`.
