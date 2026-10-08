# Contributing

## Never test against a real project

Every live test runs in a disposable lab, and nowhere else:

    LAB=$(./scripts/lab.sh)
    cd "$LAB/service"
    mia env up

Testing against a repository somebody works in means a bug writes into work
they care about. `rm -rf` the lab when you are done; nothing in it is precious.

| | |
| --- | --- |
| `service` | a Python project — a dev server, worktrees, commits |
| `web` | a TypeScript project, so image discovery is exercised on both stacks |
| `service-extra` | a sibling whose name starts with `service`, so matching a repository by path prefix is caught |
| `mia.python.toml`, `mia.typescript.toml` | configs to copy into `<repo>/.git/mia/config.toml` |

Neither app has heard of mia. If either needs editing to work through the
gateway, the gateway is wrong.

## The suites

    go test ./...
    scripts/loop.sh            # the CLI end to end, in a throwaway repository

The tests use real git and tmux rather than mocks. Everything that can be a
pure function of text is one — placement, image discovery, panels — so
the parts most likely to be quietly wrong are tested without a machine.

## What needs real hardware

Checked by hand against the lab:

- an environment on another machine (`mia machine add`, `mia env host`, `mia env up`)
- the gateway end to end (`curl --proxy`, a websocket upgrade, a stream)

## Style

- `gofmt`, `go vet`, and no new dependency for what a few lines can do.
- Every dashboard action is a CLI verb. If the list can do it, `mia` can.
- Nothing mia keeps goes in the working tree; it lives in `.git/mia/`.
