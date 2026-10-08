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

## The image

`mia env up` uses the first of:

1. `[env] image` in the config;
2. the `image` in `.devcontainer/devcontainer.json`;
3. an image for the project's language (`go.mod`, `Cargo.toml`,
   `package-lock.json`, `pyproject.toml`, `Gemfile`, …);
4. `mcr.microsoft.com/devcontainers/base:ubuntu`.

The worktree is mounted at the same path inside the container.

## Setup

| key | runs |
|---|---|
| `[env] image_setup` | once, baked into an image by `mia env image` |
| `[env] setup` | in each new container |

```toml
[env]
image_setup = ["sh", "-c", "apt-get update && apt-get install -y socat"]
setup = ["npm", "install"]
```

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
`mia env host local` moves it back.
