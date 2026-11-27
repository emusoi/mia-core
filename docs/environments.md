# Environments

An environment is a container a worktree's code runs in, so each worktree
can run its own copy of the app without clashing with the others. It needs
podman or docker; mia uses podman when both are there.

```bash
mia env up                 # start it
mia env shell              # a shell inside
mia env browse             # open the app in your browser
mia env down               # stop it
mia env rm                 # throw it away
```

Each takes a worktree name, or works on the one you are in.

## What a container is

One worktree has at most one container, named `mia-<name>`. It is a plain
Linux box with your code in it:

- **Your files, live.** The folder that holds your worktrees is mounted at
  the same path inside, so an edit on your computer is there at once, and
  `git` works inside too.
- **Nothing running by itself.** The container only waits. Your shells,
  commands and [services](#services) run in it through mia.
- **No published ports.** Each container listens on its own ports, so two
  apps on port 5173 do not collide. You reach them at
  [`<name>.mia`](#mia).
- **Kept until removed.** `mia env down` stops it and keeps everything, and
  `mia env up` starts it again. `mia env rm` throws it away with its private
  volumes, and the next `mia env up` starts fresh.

## Setting one up

Start with nothing and add only what turns out to be missing. Every key
here goes in the config (`mia config` opens it; `.mia.toml` shares it with
the project). [Configuration](configuration.md#env) lists them all.

**1. Bring it up.**

```bash
mia env up
```

mia picks the image from the first of:

1. `[env] image`;
2. the `image` in `.devcontainer/devcontainer.json` or `.devcontainer.json`;
3. one for the project's language, from `go.mod`, `Cargo.toml`,
   `pnpm-lock.yaml`, `package-lock.json`, `yarn.lock`, `pyproject.toml`,
   `requirements.txt` or `Gemfile`;
4. `mcr.microsoft.com/devcontainers/base:ubuntu`.

It says which one it pulled and why. `mia env status` says it again later.

**2. Look around.**

```bash
mia env shell
```

The shell opens in a window named `env` in the worktree's session; leave it
and the same command brings you back to it as it was. `--once` is a plain
shell that ends when you leave. Install, run the app and run the tests as
you would on a fresh computer. Whatever you had to type goes into one of
two places.

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
`container_only` keeps a folder in a volume of the container's own, so the
Linux `node_modules` never mixes with your computer's. `cache` is a volume
shared by all the repository's containers, so the second install is fast.

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
mia env browse --direct    # plain HTTP, nothing to set up
mia env browse             # HTTPS, once the gateway is set up
```

**7. Reach your computer, if you need to.** For a database running on your
computer rather than in the container:

```toml
[env]
host_ports = [5432]
```

Inside, `127.0.0.1:5432` is your computer's 5432. It needs `socat` in the
image, and shows in `mia env service` as `host-5432`.

Check it all from scratch:

```bash
mia env rm && mia env up
```

### When the configuration changes

A change to the image, setup, volumes, passed variables or services does
not rebuild a container that exists. `mia env up` and `mia doctor` say when
they differ; apply them with:

```bash
mia env up --recreate
```

That replaces the container and runs setup again, keeping its volumes and
its sync to another machine.

### A whole configuration

For a web app:

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

## `<name>.mia`

mia's gateway, on `127.0.0.1:7842`, carries a browser to the ports inside
each container. `mia env up` starts it.

With nothing set up, `mia env browse --direct` opens
`http://<name>.localhost:7842/`, which reaches `[env] page`, or the first
port listening when there is no page. It cannot choose another port.

For any port, over HTTPS, open `https://<name>.mia:<port>/`:

```bash
mia env browse             # the page
mia env browse 8080        # another port
mia env ports              # what is listening, with its address
```

That needs two things on your computer, once:

```bash
mia gateway setup          # says both
mia gateway trust          # trust mia's certificate
```

`trust` adds the certificate to your login keychain on macOS, after asking;
elsewhere it says where the certificate is, for your browser. The other is
pointing your system's automatic proxy setting at
`http://127.0.0.1:7842/proxy.pac`. Only names ending in `.mia` go through
mia; everything else is untouched. mia's certificate can sign `.mia` names
and nothing else, and `mia gateway untrust` takes it back out.

The app is sent `Host: localhost:<port>`, which dev servers accept without
setup. `[env] pass_host = true` sends it `<name>.mia:<port>` instead.

`mia gateway status` says whether it runs and the names it serves. On Linux,
`mia gateway install` makes it a user service that starts with the machine.

## Services

```toml
[[service]]
id = "web"
run = ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
health = ["sh", "-c", "curl -fsS http://127.0.0.1:5173 >/dev/null"]
autostart = true
restart = "on-failure"
```

`autostart` starts it with `mia env up`. `restart = "on-failure"` starts it
again when it exits non-zero; `"on-unhealthy"` also when `health` fails
three times in a row, checked every 10 seconds once `grace` seconds (60 by
default) have passed.

```bash
mia env service              # each service: running, healthy, NOT ANSWERING or stopped
mia env service restart web  # also start, stop
mia env service logs web     # its last 40 lines
mia env service logs -f web  # and follow
```

```bash
mia env up --wait
```

`--wait` waits until every autostart service runs, and answers its health
check when it has one, and every port in `[env] ports` listens. It gives up
after a minute, or the largest `grace` when that is longer, exits non-zero
and leaves the environment running for you to look at.

## Your files

```toml
[tools]                    # in any config
copy = ["~/.gitconfig"]
credentials = ["~/.netrc"]

[dotfiles]                 # in ~/.config/mia/config.toml
env = ["~/.zshrc"]
shell = "zsh"
packages = ["zsh", "fzf"]
```

`[tools] copy` and `[dotfiles] env` are put in the container's home on every
`mia env up`. Credentials can act as you wherever the environment runs, so
they are placed only by `mia env tools --credentials`, or on every `up` with
`[tools] carry_credentials = true`. `[env] pass = ["NAME"]` passes
environment variables from your shell. [Configuration](configuration.md#dotfiles)
has the rest.

## Compose projects

An app that needs Postgres, Redis or other services beside it can use its
Compose file:

```toml
[env.compose]
file = "compose.yaml"
service = "app"            # "app" when left out
```

The project is named `mia-<worktree>`, so containers and networks of two
worktrees never share names. `service` is where setup, shells,
`[[service]]` processes and the gateway go; the others start and stop with
the project. `up` runs `compose up`, `--recreate` takes it down first,
`down` stops it, and `rm` removes its containers and networks. Its volumes,
databases included, are kept by all of them.

Use `expose`, not `ports`, and no `network_mode: host`: mia refuses either
anywhere in the project, as published ports would collide and the host's
network defeats the isolation. If your usual file publishes ports, point
`file` at a `compose.mia.yaml` that does not.

Compose owns images, volumes and each service's variables, so `image`,
`image_setup`, `container_only`, `cache` and `pass` cannot be set with it.
`setup`, ports, `host_ports`, dotfiles and services work in the chosen
service. Switching between Compose and one container needs `mia env rm`
first. `mia doctor` checks the Compose plugin.

## Main's dev server

No container at all: if your dev server already runs in the main checkout,
lend it a worktree's files.

```bash
mia dev monduli            # main's dev server now shows monduli
mia dev monduli --follow   # and keeps copying as you edit
mia dev                    # whose files main has
mia dev off                # main gets its own back
```

This is the one time mia writes into a working tree. It puts main's own
changes aside in a stash, then copies the worktree's files over main,
uncommitted ones included. Ignored files such as `node_modules` and `.env`
stay as they are, so the server keeps running and reloads. `mia dev off`
resets main and brings the stash back. Do not commit in main while it is
lent.

## Other machines

Two ways to use another machine. The whole worktree can live there, git,
session, agents and all, made with `mia new --on` or taken there with
[`mia move`](machines.md#moving-a-worktree); its environment then runs
there as on any computer. Or the worktree stays here and only its
environment runs there:

```bash
mia machine add fedora user@fedora
mia env host fedora        # its environment goes there
mia env up
mia env host               # where it runs now
mia env host local         # and back
```

`mia env host` checks the machine first (its container engine, Compose if
used, and [mutagen](https://mutagen.io/documentation/introduction/installation)
here) and changes nothing when that fails. Then it removes the environment
where it was, private volumes included, and the next `mia env up` starts it
fresh on the machine. The address stays `<name>.mia`. An image built with
`mia env image` is built per machine: run it again there.

The machine gets a copy of the worktree under `~/.mia/<repository>/<name>`,
kept in step by mutagen (see [Sync](#sync)). The copy has no `.git`, since
syncing git's own files both ways would corrupt them. So by default the
worktree's shells and agent windows run on that machine, beside the
environment, and cannot commit. For agents that use git, keep them here:

```toml
[env]
session = "local"
```

`mia env shell` always opens inside the environment, wherever it runs.
`[dotfiles] machine` files, such as `~/.tmux.conf`, go to the machine's home
on `mia env up` and `mia env dotfiles`.

## Sync

The copy on the other machine follows yours live, both ways. It leaves out
`.git`, dependencies (`node_modules`, `.venv`, `.yarn/cache`, …), build
output (`target`, `dist`, `build`, …) and, unless `sync_secrets = true`,
`.env*`, `.envrc`, `.netrc`, `.npmrc`, `.pypirc`, keys and certificates.
Leave out more, such as a tree the box never needs or an iOS `Pods` tree
whose links are too long for it:

```toml
[env]
sync_ignore = ["packages/mobile/ios/Pods", "secrets/*"]
```

```bash
mia env sync monduli status      # its mode and health
mia env sync monduli preview     # what is left out, and what would cross; never contents
mia env sync monduli conflicts   # paths in conflict, or "no conflicts"
mia env sync monduli manual      # settle what is moving, then hold changes
mia env sync monduli now         # move them once, keeping the mode
mia env sync monduli auto        # follow live again
mia env sync monduli restart     # start it again with the current rules
```

`status`, `conflicts` and `preview` take `--json`. Changed rules take a
`restart`; until then `up`, `auto`, `manual` and `now` refuse. A restart keeps
the files on both sides but forgets the sync's history, so `conflicts`
lists what then differs. Leaving a file out stops it moving from then on;
it does not delete a copy already on the machine.

`sync_secrets = true` lets every secret-like file cross, not one. Set it
only for a machine you trust; `mia doctor` warns while it is on.

## The commands

| command | does |
|---|---|
| `mia env up [--wait] [--recreate]` | start it, making it first if needed, with tools, dotfiles and autostart services; starts the gateway |
| `mia env down [--all]` | stop it, or every worktree's in the repository; nothing is removed |
| `mia env rm` | remove it and its private volumes |
| `mia env shell [--once]` | a shell inside, in the session's `env` window |
| `mia env exec <cmd…>` | run a command inside, at the terminal |
| `mia env run <worktree> <cmd…>` | run a command once inside, or in the worktree when no environment runs, and say which |
| `mia env setup` | run `[env] setup` again |
| `mia env image` | build the repository's image from `image_setup` |
| `mia env tools [--credentials]` | carry `[tools]` files in again |
| `mia env dotfiles` | carry `[dotfiles] machine` files to the machine it runs on |
| `mia env status` | its state, machine, image and where it came from, and its addresses |
| `mia env ports` | the ports listening inside |
| `mia env browse [--direct] [port]` | open it in the browser |
| `mia env service …` | its services: list, `start`, `stop`, `restart`, `logs [-f]` |
| `mia env host [machine]` | where it runs, or move it |
| `mia env sync …` | its copy on another machine |

`status`, `ports` and `host` take `--json`. `mia doctor` checks the
container engine, the gateway, each machine and each environment's
configuration.
