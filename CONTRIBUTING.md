# Contributing

## Test

```bash
go test ./...
scripts/loop.sh            # the CLI end to end, in a throwaway repository
```

The tests use real git and tmux.

## Try it safely

`make dev` builds `miadev` beside your installed `mia`, so the two never
overwrite each other. `miadev` uses the same config and worktrees, but
leaves the gateway to `mia`.

Never try mia against a repository you work in. Make a throwaway lab:

```bash
LAB=$(./scripts/lab.sh)
cd "$LAB/service"
```

It has a Python project (`service`), a TypeScript project (`web`), and
configs to copy into `.git/mia/config.toml`. Delete it when done.

## Style

- `gofmt` and `go vet`; no new dependency for what a few lines can do.
- Every dashboard action is a `mia` command.
- mia writes nothing into the working tree; everything goes in `.git/mia`.
