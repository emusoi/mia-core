# Architecture

mia is one Go binary and the plugins you choose to give it. It
keeps almost no state of its own: git, tmux and the container engine already
know most of what there is to know, and mia asks them.

## The rule everything hangs from

**A worktree's path is its identity, and everything else is derived from it
every time it is asked for.**

- Records in `.git/mia/` say which paths mia adopted and what name each got.
  Names live in one user-wide registry, so a name reaches the same checkout
  from any repository.
- A **branch** is cargo: read off the worktree when needed. A detached
  checkout is an ordinary state, never a failure.
- **Sessions** are tmux sessions named after the worktree.
- **Environments** are containers named after the worktree.
- **Routes** for `<name>.mia` are computed from the records at request time,
  never written down.
- A record whose directory is gone is dropped as the list is read.

There is nothing to keep in sync, so there is no doctor and no repair command.

## Packages

| package | lines | holds |
|---|---:|---|
| `cli` | 3.3k | verbs, help, the dashboard loop |
| `tui` | 2.3k | the one renderer, bubbletea |
| `app` | 1.6k | the operations: list, new, adopt, remove, stacks, windows |
| `env`, `container`, `runtime`, `mirror` | 2.2k | environments in podman or docker, here or on another machine over ssh |
| `panel` | 1.1k | the view model: sections, rows, actions — every action names a CLI verb |
| `session` | 0.9k | tmux: panes, windows, attach, popup, capture |
| `gateway` | 0.8k | the daemon: the `<name>.mia` proxy and its certificate authority |
| `git`, `names`, `stack`, `store`, `model` | 1.6k | the records and the git they derive from |
| `plugin` | 0.8k | finding, enabling and talking to `mia-<name>` programs |

About 15k lines of Go and 8k of tests.

## Actions are verbs

The dashboard draws a `panel.Panel`: sections of rows, and actions that are
each `{verb, args}` — a `mia` command line. Nothing in the list can do what the
CLI cannot, and the CLI is what scripts use. `mia api dashboard` prints the
panel as JSON; `?` in the dashboard prints the command under every key.
Rebinding a key is config, not code.

An editor plugin runs the same binary in a float and reads back a pick
(`mia dash --pick-to`), so what the terminal shows is what the editor shows.

## Plugins

A plugin is a program named `mia-<name>`; core finds it, runs it only once it
is enabled, and talks to it in JSON: one-off calls with the context on stdin,
or JSON-RPC over stdio while the dashboard is open. Plugin sections share ids
and sit by rank beside core's own; plugin keys are actions named
`<plugin>.<id>`; events are sent without waiting. Plugins read core through
`mia api` and keep their data under `.git/mia/plugins/<name>/`.

## Environments

A container per worktree, on this machine or on a machine registered with
`mia machine add <name> <ssh-target>`. The image is `[env] image` if the config
names one, else the project's `devcontainer.json`, else one picked for the
language it is written in, with `[env] image_setup` baked on top. On another machine, mia keeps a live-synced staging copy of
the worktree, computed from the machine, repository and name — never stored, so
it cannot be inherited across a move.

## The gateway

The one long-lived process. It serves a proxy auto-config file that sends
`*.mia` (and nothing else) to itself, terminates TLS with a local certificate
authority, and forwards to whichever container holds that name, on whatever
port the app is listening on. It holds no truth: killing it loses nothing.

## tmux

Long-running commands talk to tmux through one control-mode client
(`tmux -C`) instead of a process per question; `MIA_TMUX_SPAWN=1` forces the
old path. Popups, windows, scrollback and capture are tmux's own. `MIA_TRACE=1`
prints every git, tmux and ps call with its time.

## Speed

Deriving everything each time costs a few processes per refresh, so the
expensive ones are cut: worktrees and heads are read from `.git` files, one
`list-panes -a` covers every session, and every fact keyed by a commit (ahead,
behind, landed) is cached on disk and never recomputed.
