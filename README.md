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
mia dash                   # every worktree, and the keys to act on them
mia env up                 # its container, at https://monduli.mia:<port>
mia new api --stack        # a branch on top of this one, same worktree
```

## Build and test

```bash
make build
make test
make site                  # the website, from docs/
```

See [CONTRIBUTING.md](CONTRIBUTING.md). MIT licensed.
