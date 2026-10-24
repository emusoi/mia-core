# mia

*Mia* is Swahili for 100.

mia gives every branch a worktree, every worktree a name and a tmux session,
and, when it needs one, a container you open at `https://<name>.mia`.
Several branches can share a worktree as a stack. Everything else is a plugin.

Documentation: [docs/](docs/README.md) or https://mia.emusoi.app.

## Install

macOS or Linux, with git and tmux.

```bash
go install github.com/emusoi/mia-core/cmd/mia@latest
eval "$(mia shell-init zsh)"     # in your shell's rc file
```

## Use

```bash
mia new due-dates          # a worktree named for you, e.g. monduli
mia shell monduli          # its tmux session
mia radar                  # find any worktree, switch to it, or make one
mia env up                 # its container, at https://monduli.mia:<port>
mia env browse --direct    # open its page; no proxy or certificate setup
mia new api --stack        # a branch on top of this one, same worktree
mia gc --others            # list worktrees mia did not make; --apply removes the clean ones
```

In tmux, `mia tmux install` adds the radar on `prefix space`, a new worktree
on `prefix n`, and mia's part of the status line.

## Build and test

```bash
make build
make test
make site                  # the website, from docs/
```

See [CONTRIBUTING.md](CONTRIBUTING.md). MIT licensed.
