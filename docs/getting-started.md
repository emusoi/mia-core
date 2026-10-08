# Getting started

## Install

mia runs on macOS and Linux, and needs Go 1.26.8 or newer to build, git, and
[tmux](https://github.com/tmux/tmux/wiki/Installing).

```bash
go install github.com/emusoi/mia-core/cmd/mia@latest
```

Then add the shell function to your shell's rc file, so `mia switch` can
change directory and the shell completes worktree names:

```bash
eval "$(mia shell-init zsh)"
```

(`bash` and `fish` work too.)

Environments also need [podman](https://podman.io/docs/installation) or
docker; you can skip that until you want one.

## Your first worktree

In any git repository:

```bash
mia new due-dates
```

```
monduli — due-dates in a worktree of its own
/Users/you/src/shop.monduli
```

mia made a branch `due-dates`, checked it out in a new directory next to the
repository, and gave the worktree a name: `monduli`. From now on that name
reaches it:

```bash
mia shell monduli          # its tmux session
mia switch monduli         # cd there
mia ls                     # every worktree of this repository
```

Make a few more. Work in one, leave it, come back days later: the session,
and whatever was running in it, is still there.

## The dashboard

```bash
mia dash
```

Every worktree, grouped by what it needs: the main checkout, the ones in
progress, the quiet ones. `⏎` attaches, `n` makes a new one, `?` lists every
key and the command it runs. `q` leaves.

It is best as a popup over tmux. `mia help tmux` prints two lines for
`~/.tmux.conf`; with them, `prefix g` opens the dashboard and `prefix j`
jumps to another worktree by typing part of its name. See
[the dashboard](dashboard.md).

## A repository's config

```bash
mia config
```

opens `.git/mia/config.toml` — inside `.git`, so it is never committed and
never shows up as a change. Two lines make every new worktree ready to work
in:

```toml
setup = ["npm", "install"]
clone = ["node_modules"]
```

`clone` copies the main checkout's `node_modules` (copy-on-write, so it is
instant and free), and `setup` brings it up to date. See
[configuration](configuration.md).

## An environment

When a worktree's code needs its own container — its own database, its own
toolchain version, its own ports:

```bash
mia env up
```

mia picks an image from what the project already says (a
`devcontainer.json`, or its language), starts the container with the
worktree mounted, and prints where the app is:

```
monduli  running  mcr.microsoft.com/devcontainers/typescript-node:22
          https://monduli.mia:5173/
```

That URL needs the gateway set up once, which changes two settings on your
computer:

```bash
mia gateway setup
```

prints both steps. After that, whatever you would open on `localhost`, open
with the worktree's name instead. See [environments](environments.md).

## A stack

When a feature is too big for one pull request:

```bash
mia new schema --stack
mia new api --stack
mia up / mia down          # move between layers, in place
mia stack pr               # the commands to open one pull request per layer
```

See [stacks](stacks.md).

## Next

- [Concepts](concepts.md) — what the words mean.
- [Commands](commands.md) — every command, in one place.
- [Plugins](plugins.md) — adding what core does not do.
