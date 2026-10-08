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
mia env browse
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
On the dashboard, `v` lends the selected worktree; `v` on main gives it back.

## `<name>.mia`

Containers publish no ports. Open the app at `https://<name>.mia:<port>/`
instead of `localhost:<port>`. The gateway makes that work. It needs two
settings on your computer, once:

```bash
mia gateway setup          # prints both
mia gateway trust          # trusts mia's certificate (macOS)
```

The other is pointing your system's automatic proxy setting at
`http://127.0.0.1:7842/proxy.pac`. `mia env up` starts the gateway.

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
mia env service            # each service and its state
mia env service logs web
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
