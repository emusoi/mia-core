# Environments

An environment is where a worktree's code runs: a container with the
project's toolchain, its services and its ports. Each worktree has at most
one. Environments need [podman](https://podman.io/docs/installation) or
docker; mia uses podman when both are installed.

    mia env up                 # build or start this worktree's container
    mia env shell              # a shell inside it
    mia env browse             # open https://<name>.mia:<port>/ in the browser
    mia env down               # stop it
    mia env rm                 # remove it

Every `mia env` command takes the worktree as its first word and defaults to
the one you are in. See [the full list](#every-env-command) below.

## What `mia env up` does

1. **Chooses an image**, in this order:
   1. `[env] image` from the config, if set;
   2. the `"image"` key of `.devcontainer/devcontainer.json` or
      `.devcontainer.json` (comments and trailing commas are fine; `build`
      and `dockerFile` are not read);
   3. an image for the language the project is written in, from the first of
      these files it finds:

      | file | image |
      |---|---|
      | `go.mod` | `docker.io/library/golang:1-bookworm` |
      | `Cargo.toml` | `docker.io/library/rust:1-bookworm` |
      | `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock` | `mcr.microsoft.com/devcontainers/typescript-node:22` |
      | `pyproject.toml`, `requirements.txt` | `mcr.microsoft.com/devcontainers/python:3` |
      | `Gemfile` | `docker.io/library/ruby:3` |

   4. otherwise `mcr.microsoft.com/devcontainers/base:ubuntu`.

   If you have baked an image with `mia env image` (see below), that is used
   instead of the base it was baked from.
2. **Starts the container** `mia-<name>`, pulling the image first if needed.
   The worktree's parent directory is mounted at the same absolute path, and
   the container's working directory is the worktree — so paths inside the
   container match the ones on your machine.
3. **Runs `[env] setup`** the first time the container is created.
4. **Starts services** marked `autostart`.
5. **Carries your files**: the `[tools] copy` list, and your `[dotfiles]`.
6. **Starts the gateway** if it is not running, and prints the URLs:

        monduli  running  docker.io/library/golang:1-bookworm
                  https://monduli.mia:8080/

A stopped container is started again as it was. Changes to the image,
`[env] pass` or volumes apply only to a new container: `mia env rm`, then
`mia env up`.

## Three kinds of setup

A toolchain can live in three places, and each has its own setup:

| key | runs | survives |
|---|---|---|
| `setup` (top level) | on your machine, in a new worktree; again with `mia setup` | — |
| `[env] image_setup` | while baking an image with `mia env image` | `mia env rm` |
| `[env] setup` | inside a fresh container; again with `mia env setup` | until `mia env rm` |

A container does not inherit the host's setup. System packages belong in
`image_setup`, so they are baked once; the project's own install
(`npm install`, `go mod download`) belongs in `[env] setup`.

```toml
[env]
image_setup = ["sh", "-c", "apt-get update && apt-get install -y socat"]
setup = ["npm", "install"]
```

### Baking an image

`mia env image` builds `FROM <base>` plus one `RUN` of `image_setup`, and tags
it `mia-<repo>:<hash>`, where the hash covers the base and the setup. Change
either and a new tag is built. `mia env up` uses a baked image when one exists
for the current settings; it never builds one itself.

## Dependencies that should not live on the host

An installed dependency tree belongs to one worktree. `container_only` keeps
it in a volume of the container's own instead of in the worktree on your
disk, so two worktrees never fight over `node_modules` and nothing in it is
indexed or backed up:

```toml
[env]
container_only = ["node_modules", "packages/*/node_modules"]
```

What an installer *downloads* belongs to nobody. `cache` puts it in a volume
shared by every environment of the repository and kept by `mia env rm`, so
only the first worktree pays for the download:

```toml
[env]
cache = ["/root/.npm"]
```

A `cache` path is a path *inside* the container and must be absolute — `~` is
not expanded, and a relative path is reported and skipped.

## Ports and the browser

Containers publish no ports. You reach them through the
[gateway](#the-gateway): whatever port the app listens on inside the
container, `https://<name>.mia:<port>/` reaches it.

| key | does |
|---|---|
| `ports` | ports you expect the app to serve, shown in the environment panel |
| `page` | the port `mia env browse` opens; without it, the first port listening |
| `host_ports` | ports on *your* machine the container should reach, as `localhost:<port>` inside it (a database running on the host, say). Needs `socat` in the image. |
| `pass_host` | send the original `Host` header (`monduli.mia:5173`) instead of `localhost:<port>` |

By default an app sees requests as if they came from `localhost:<port>`:
`Host`, `Origin` and `Referer` are rewritten to localhost, so a dev server that
only trusts localhost keeps working. The original host is still in
`X-Forwarded-Host`.

`mia env ports` lists what is listening right now.

## Services

A service is a process the environment keeps running:

```toml
[[service]]
id = "web"
run = ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true
restart = "on-failure"
grace = 30
```

| key | meaning |
|---|---|
| `id` | its name, for `mia env service start web` |
| `run` | the command |
| `health` | a command that exits 0 when the service is answering |
| `autostart` | start it on `mia env up` |
| `restart` | `on-failure`: start it again 2s after it exits non-zero. `on-unhealthy`: also restart it when `health` fails 3 times in a row, checked every 10s after `grace` seconds (default 60). Absent: never restart. |

    mia env service                 # each service and its state
    mia env service start web
    mia env service logs web        # its last 40 lines

A service's output goes to `/run/mia/<id>.log` inside the container.

## Your own files in the environment

An environment is a fresh machine that knows nothing about your setup.

`[dotfiles]` is your own setup and belongs in `~/.config/mia/config.toml`, so
it follows you into every repository:

```toml
[dotfiles]
env = ["~/.zshrc", "~/.config/starship.toml"]   # into the container's home
shell = "zsh"                                   # what env shell runs, when the image has it
packages = ["zsh", "ripgrep"]                   # installed when missing (apt, as root)
setup = ["sh", "-c", "command -v starship || curl -sS https://starship.rs/install.sh | sh -s -- -y"]
machine = ["~/.tmux.conf"]                      # into the home of another machine environments run on
```

`[tools]` carries files a project's tools need, and belongs in the
repository's config:

```toml
[tools]
copy = ["~/.gitconfig", "~/.config/gh/config.yml"]
credentials = ["~/.netrc"]
carry_credentials = false
```

`copy` goes on every `mia env up` and `mia env tools`. `credentials` go only
with `mia env tools --credentials` (or always, with `carry_credentials =
true`), written with mode `0600` — those files can act as you wherever the
environment runs. mia never overwrites a credential file it did not put there.

`[env] pass` passes environment variables from your shell, by name:

```toml
[env]
pass = ["DATABASE_URL", "STRIPE_TEST_KEY"]
```

A container takes its variables when it is created: after changing one, run
`mia env rm` and `mia env up`.

## Other machines

An environment can run on any computer you can `ssh` to that has podman or
docker. Register it once:

    mia machine add build me@build.example.com
    mia machine list

`add` connects and checks for a container engine. Then move a worktree's
environment there and start it:

    mia env host build
    mia env up

The name, the URL and your branch do not change. The editing still happens on
your machine: mia keeps a **staging copy** of the worktree on the other
machine, at `~/.mia/<repo>/<name>` in its home, and keeps it in step with
[mutagen](https://mutagen.io/documentation/introduction/installation), both
ways. Build output and dependency folders (`.git`, `node_modules`, `.venv`,
`target`, `dist`, `build` and the like) are not synced.

Syncing is live by default. For a large tree, you can sync on demand instead:

    mia env sync manual        # stop syncing on every change
    mia env sync now           # move changes now
    mia env sync auto          # back to live

Move back with `mia env host local`. `mia env host` with no machine says where
the environment runs now.

`mia new <branch> --on build` makes a worktree and starts its environment on
`build` in one go.

Registered machines are kept in `~/.config/mia/runtimes.json`; `mia machine`
is how you change them.

## The gateway

The gateway is the process that makes `https://<name>.mia:<port>/` work. You
only need it for opening environments in a browser; `mia env up` and
`mia env browse` start it when it is not running.

The first time, two settings on your computer need changing, and mia asks
rather than doing it:

    mia gateway setup

prints both:

1. **Trust mia's certificate authority**, so `https://` works without
   warnings: `mia gateway trust` does it on macOS (it asks first). Firefox
   keeps its own store: import `~/.config/mia/gateway/ca.pem` there.
2. **Point the browser at the proxy file** `http://127.0.0.1:7842/proxy.pac`:
   on macOS, System Settings → Network → your connection → Details → Proxies →
   Automatic proxy configuration. Chrome and Safari follow the system setting;
   Firefox has its own.

The proxy file sends only names ending in `.mia` to the gateway. Everything
else goes direct.

How a request is answered: the browser asks for `monduli.mia:5173`; the
gateway finds the worktree named `monduli`, checks that its container is
running, terminates TLS with a certificate for that name, and forwards to
port 5173 inside the container — on this computer, or on another machine over
`ssh`. Routes are worked out from mia's records on every request, never
written down, so the gateway holds nothing you can lose.

When something does not answer, the gateway says why in the browser: no
environment with that name, or nothing listening on that port (and what *is*
listening).

| command | does |
|---|---|
| `mia gateway status` | whether it runs, the proxy file, whether the CA is trusted, and every route |
| `mia gateway start` / `stop` | start or stop it |
| `mia gateway setup` | start it and print the two one-time steps |
| `mia gateway trust` / `untrust` | trust or untrust the CA (macOS; elsewhere it says how) |
| `mia gateway install` / `uninstall` | run it as a systemd user service, so it starts at login (Linux only; on macOS, `mia env up` starts it when needed) |

It listens on `127.0.0.1:7842` and keeps its CA, pid and log in
`~/.config/mia/gateway/`. Certificates for each name are made on demand and
never written to disk; the CA can only sign `.mia` names.

## Every env command

| command | does |
|---|---|
| `mia env up` | build or start the container; run setup, services, carry files, print URLs |
| `mia env down` | stop it |
| `mia env rm` | remove the container and its `container_only` volumes (`cache` volumes stay) |
| `mia env setup` | run `[env] setup` again in the running container |
| `mia env shell [--once]` | a shell inside it, in a window called `env` of the worktree's session; `--once` attaches directly instead |
| `mia env exec <cmd…>` | run a command inside it, interactively |
| `mia env run <worktree> <cmd…>` | run a command where the worktree's code runs — inside the container when it is up, else in the worktree — and exit with its status; for scripts and plugins |
| `mia env ports [--json]` | what is listening, with its URL |
| `mia env browse [port]` | open the app in the browser |
| `mia env status [--json]` | state, machine, image and where it came from, sync, URLs |
| `mia env service [start\|stop\|restart\|logs <id>]` | the declared services |
| `mia env image` | bake `image_setup` into an image |
| `mia env tools [--credentials]` | carry `[tools]` files in now |
| `mia env dotfiles` | copy `[dotfiles] machine` files to the machine the environment runs on |
| `mia env host [machine]` | where it runs, or move it |
| `mia env sync [auto\|manual\|now]` | how the staging copy follows the worktree |

The dashboard's `E` key opens the same things as a panel.
