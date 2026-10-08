# mia

Worktrees, sessions, stacks and environments — and plugins for the rest.

*Mia* is Swahili for 100.

The full documentation is in [docs/](docs/README.md) and at https://mia.emusoi.app.

mia gives every branch a worktree, every worktree a name and a tmux session,
optionally a container with a real hostname, here or on another machine you can ssh to.
Everything else — plans, agents, editors, trackers — is a plugin.

- **Worktrees with names.** `mia new due-dates` makes a worktree, names it
  (`monduli`), and gives it a tmux session. The name is stable for the
  worktree's life; the branch inside it is free to change.
- **Portable environments.** One description builds a container on this
  laptop or on another machine you can ssh to, and one command moves it between them.
- **Reachable.** Every environment has a real hostname. Whatever you would type
  on localhost, type with the worktree's name instead —
  `localhost:5173` → `monduli.mia:5173`.
- **Stacks.** A feature too large for one pull request is several branches
  sharing one worktree; `mia up` and `mia down` move between them in place.

The repository is byte-identical with and without mia. Everything mia keeps
lives in `.git/mia/`.

## Install

Needs Go 1.26.8+, git and tmux, on macOS or Linux. Environments need podman or
docker; an environment on another machine needs mutagen to keep the tree in step.

    go install github.com/emusoi/mia-core/cmd/mia@latest
    eval "$(mia shell-init zsh)"   # in your rc: `mia switch` changes directory, plus completion

## Getting started

    cd your-repo
    mia new due-dates          # a worktree, named for you, with a tmux session
    mia shell due-dates        # attach to its session; a worktree answers to its name or branch
    mia dash                   # the dashboard: every worktree, grouped by what it needs

    mia env up                 # a container, from whatever the project already says
    mia env browse             # https://monduli.mia:5173/ — the port the app chose

The first time, `.mia` names need two settings changed on your machine, so mia
asks rather than doing it:

    mia gateway setup          # prints both; `mia gateway trust` does the first

### On another machine

    mia machine add build me@build.example.com
    mia env host build
    mia env up

The URL does not change. Neither does your branch or your session.

### Stacks

    mia new schema --stack     # a layer on the branch you are on, same worktree
    mia up / mia down          # move between layers in place — nothing restarts
    mia stack                  # the whole stack
    mia stack merge ui onto api
    mia stack pr               # the `gh pr create` lines, bottom-up, for you to run

mia never merges into the base branch and never pushes.

### Tidying

    mia gc                     # worktrees nobody has touched for 14 days
    mia gc --apply             # remove them; `mia star` one to keep it

## Configuration

One TOML file per repository, `.git/mia/config.toml` (`mia config` opens it),
and an optional user-wide `~/.config/mia/config.toml` underneath it. Unknown
keys are refused, not ignored. See [`examples/config.toml`](examples/config.toml).

Three setups, one for each place a toolchain can live:

    setup = [...]              the host, in a new worktree; `mia setup` again
    [env] image_setup = [...]  the image, at build; survives `mia env rm`
    [env] setup = [...]        the container, on a fresh `mia env up`

## The dashboard

    mia dash                   # `mia help tmux` prints the key that pops it over tmux

`?` lists every key, and every key is a CLI verb — the keys view shows the
command under each. `[keys]` in the config rebinds any of them. `mia api
dashboard` is the same view as JSON.

## Plugins

A plugin is any executable named `mia-<name>` on `PATH` or in
`~/.config/mia/plugins`, in any language. It does nothing until
`mia plugin enable <name>`. Its manifest (`mia-<name> manifest`, JSON) can
declare verbs (`mia <verb>`), dashboard rows, sections, side tabs, keys and
panels, events it wants to hear, and a long-running `serve` mode. Its settings
live under `[plugin.<name>]` in the config. `mia plugin ls` shows what is
installed and what is wrong with it. [docs/plugins.md](docs/plugins.md) is the protocol.

## Scripting

`mia api <query>` returns JSON (`worktrees`, `dashboard`, `env`, `stack`, …),
and every reading verb takes `--json`. Clients and plugins use nothing else.

`mia help vocabulary` is the glossary. `MIA_TRACE=1` prints every git, tmux and
ps call with its time.

## Building and testing

    make build
    make test                  # go test ./...

See [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/architecture.md](docs/architecture.md).

## License

MIT
