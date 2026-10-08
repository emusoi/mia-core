# Concepts

mia has a small vocabulary, and it uses each word for exactly one thing.
`mia help vocabulary` prints the short version; this page is the long one.

## The worktree is the identity

A **worktree** is a checkout: a directory with your files in it. It is a
*place*. Everything else mia knows about hangs off a worktree's path — its
name, its tmux session, its environment, its stack. Branches come and go
inside it; the worktree stays.

mia does not invent a second kind of checkout. A worktree is an ordinary
`git worktree`, and the repository's main checkout counts as one too. You can
make one by hand and hand it to mia later with `mia adopt`.

## Names

Every worktree mia makes gets a **name**: a short word like `monduli` or
`longido`, picked from a list of place names when the worktree is created and
kept for its whole life. The name is how you refer to the worktree everywhere:

    mia shell monduli
    mia env up monduli
    https://monduli.mia:5173/

A name is not derived from the branch. Branches get renamed, rebased, merged
and abandoned; a name does not change unless you change it with
`mia rename`. Names are unique across every repository on your machine, so
`monduli` means one checkout no matter which directory you are in.

## Branches are cargo

The **branch** is whatever is checked out in a worktree right now. mia reads
it when it needs it and never records it as the worktree's identity. A
detached checkout is an ordinary state, not an error.

Wherever mia takes a worktree, it accepts any of three things: the name, the
path, or the branch checked out in it.

## Sessions and windows

Each worktree has a **session**: a tmux session called `mia-<name>`, started
the first time you need it. `mia shell monduli` attaches to it; leaving it
(`prefix d`, or `exit` in the last shell) brings you back where you were.

A session holds **windows** — shells, an editor, a dev server, a test watcher.
`mia window` lists and manages them, and named windows you use often can be
declared once in the config under `[launch]`.

A session belongs to the worktree, not to a terminal. Close the terminal and
the session, and everything running in it, keeps going.

## Stacks and layers

A **stack** is several branches sharing *one* worktree, each built on the one
below: `schema` on `main`, `api` on `schema`, `ui` on `api`. Each branch is a
**layer**. `mia up` and `mia down` move between layers in place, so the
session, the environment and everything running in them survive the move.

A stack exists for one reason: a feature too large for one pull request.
Each layer lands as its own pull request, bottom first. mia never merges
anything into your base branch and never pushes. See [stacks](stacks.md).

## Environments

An **environment** is where a worktree's code runs: a container with the
project's interpreters, packages, services and ports. A worktree has at most
one, and does not need one at all — plenty of checkouts are just files.

`mia env up` builds the container from what the project already says (a
`devcontainer.json`, or the language it is written in) plus what the config
adds. The worktree is mounted into it, so you edit on your machine and the
code runs in the container.

## Machines

A **machine** is where environments can run: `local` (this computer), or any
computer you can `ssh` to, registered once with `mia machine add`. Move an
environment with `mia env host <machine>`; its name, its URL and your branch
stay the same.

When an environment runs on another machine, mia keeps a **staging copy** of
the worktree there and keeps it in step with your local files. You never type
its path: it is computed from the machine, the repository and the name every
time it is needed.

## Services

A **service** is a long-running process an environment keeps running: a dev
server, an API, a worker. You declare it once (`[[service]]`: an id, a
command, an optional health check) and `mia env up` can start it, restart it
when it fails, and show its state.

## The gateway and `.mia` names

The **gateway** is mia's one long-running process. It makes
`https://<name>.mia:<port>/` in your browser reach port `<port>` inside that
worktree's environment, wherever it runs. Whatever you would type on
`localhost`, type with the worktree's name instead:

    localhost:5173   →   monduli.mia:5173

A **route** is one name's entry in the gateway. Routes are never written
down: each request is matched against mia's records at that moment, so a
route can never go stale. Killing the gateway loses nothing.

You only need the gateway if you use environments and want to open their apps
in a browser. `mia env up` starts it when it is not running.

## Plugins

Everything else — plans and checks, coding agents, editor integrations, pull
request status, notifications — is a **plugin**: a program called
`mia-<name>`, in any language, that does nothing until you enable it. A plugin
can add verbs, put facts and sections on the dashboard, add keys and panels,
and hear about what happens. See [plugins](plugins.md).

## Where mia keeps things

mia never writes into your working tree. A repository is byte-for-byte the
same with or without mia.

| place | holds |
|---|---|
| `.git/mia/` | this repository's records: which worktrees mia adopted, stacks, the config (`config.toml`), caches, plugin data |
| `~/.config/mia/` | your own: the user-wide `config.toml`, the name registry, registered machines, enabled plugins, the gateway's certificate authority |

`XDG_CONFIG_HOME` moves `~/.config` as usual.

## Words mia does not use

Some words meant something in earlier versions and were retired because they
blurred two ideas into one:

- *workspace* — editors took it; say **worktree**.
- *lane*, *slot* — said **environment** and its number at once; mia has no
  slots, the worktree is the identity.
- *berth* — say **staging copy**.
- *runtime*, *box*, *host* — say **machine**.
