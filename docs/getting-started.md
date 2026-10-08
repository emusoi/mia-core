# Getting started

## Install

mia runs on macOS and Linux and needs git and
[tmux](https://github.com/tmux/tmux/wiki/Installing). Environments also need
[podman](https://podman.io/docs/installation) or docker.

```bash
go install github.com/emusoi/mia-core/cmd/mia@latest
```

Add this to your shell's rc file, for `mia switch` and completion
(`bash` and `fish` work too):

```bash
eval "$(mia shell-init zsh)"
```

## The words

| word | means |
|---|---|
| worktree | a checkout of your repository in its own directory |
| name | a worktree's name: a place, like `monduli`, easy to remember; it never changes |
| session | the worktree's tmux session, `mia-<name>` |
| stack | several branches sharing one worktree, each on the one below |
| environment | a container where a worktree's code runs |
| machine | where environments run: this computer, or one you can ssh to |
| plugin | a program named `mia-<name>` that adds to mia |

Anywhere mia takes a worktree, you can give its name, its path, or its branch.

## A first worktree

In any git repository:

```bash
mia new due-dates
```

```
monduli — due-dates in a worktree of its own
/Users/you/src/shop.monduli
```

```bash
mia shell monduli          # its tmux session
mia switch monduli         # cd there
mia ls                     # every worktree
mia dash                   # the dashboard
```

## Settings for a repository

```bash
mia config
```

opens `.git/mia/config.toml`. For example, to make every new worktree ready
to work in:

```toml
clone = ["node_modules"]
setup = ["npm", "install"]
```

mia keeps everything in `.git/mia` and `~/.config/mia`, never in your files.
The one exception is [`mia dev`](environments.md#mains-dev-server), and only
when you ask.
