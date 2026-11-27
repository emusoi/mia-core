# Stacks

A stack is a feature too big for one pull request, cut into branches that
each build on the one below. Each branch is a layer. The layers usually
share one worktree, and you move between them in place.

```
main
 └─ schema
     └─ api
         └─ ui
```

mia keeps which layer sits on which in `.git/mia/stack.json`. Merging the
bottom layer into your base, pushing and opening pull requests are yours:
mia never does them.

## Adding a layer

```bash
mia new schema --stack     # a branch on top of the one checked out here, checked out here
mia new api --stack        # and another on top of that
```

The new branch takes your `prefix` and starts where the current one is.
Added in the middle of a stack, it goes between this layer and the one that
was above it.

| flag | does |
|---|---|
| `--worktree` | give the new layer a worktree of its own instead |
| `--in <worktree>` | add to that worktree's stack, not the one here |

A branch made another way, such as a pull request stacked on another,
joins with `mia stack track <branch> <parent>`.

## Moving between layers

```bash
mia up                     # the layer above
mia down                   # the layer below
mia stack go api           # a layer by name
```

Each checks the layer out in this worktree, and refuses while tracked files
have uncommitted changes, which would otherwise go with you.

## Looking at it

```bash
mia stack                  # or mia stack show
mia stack diff             # this layer against the one below
```

```
payments — 1/3 layers landed
  schema  landed
▸ api     4 commit(s)  in monduli
  ui      2 commit(s) · needs restack  in monduli
```

`▸` is the layer you are on. A layer is `landed` once its branch is merged
into the base, `not started` with no commits of its own, and `needs
restack` when the layer under it has commits it lacks. The bottom layer
says `main moved on` when the base has; a restack takes that too, but
nothing is out of step. `--json` gives the same, and `mia api stack` the
stack for an app.

Name a stack to go to it from anywhere:

```bash
mia stack name payments
mia cd payments            # cd to the worktree that has it checked out
```

### In tmux

In the [one list](tmux.md#the-one-list) (`prefix k`), type `stack`: a row
per layer, `▸` on the one you are on, each with how many commits it is
ahead or `needs restack`. Enter goes to that layer, as `mia stack go`
does. The status line shows `⟳` beside the branch when a layer has fallen
behind the one under it.

## Keeping it in shape

```bash
mia stack restack          # rebase each layer onto the one below
```

Layers already in line and landed layers are left alone. It refuses with
uncommitted changes, and stops at a conflict: resolve it, `git rebase
--continue`, then `mia stack restack` again.

```bash
mia stack sync
```

`sync` fetches the base's upstream, fast-forwards a local base, forgets
the layers that landed and restacks the rest. If the layer you are on
landed, you move to the nearest one that did not. The worktree here and the
base's worktree must be clean, and a base that cannot fast-forward is left
for you. Without an upstream, use `restack`, which fetches nothing.
`--json` says the base, the layer you end on, the layers dropped and how
many were restacked. Branches and worktrees of dropped layers stay.

## Folding layers together

```bash
mia stack merge ui             # into the layer below it
mia stack merge ui onto schema # into a lower one, taking every layer between
```

The lower layer fast-forwards to the top one, the folded layers' branches
are deleted, and a worktree that had one of them checked out moves to the
lower layer. `ui` may also be the name of the worktree it is checked out in. The layers must be in line first (`mia stack restack`), and
where the lower layer is checked out must be clean. It never merges into
the base: the bottom layer lands through a pull request.

## Forgetting a stack

```bash
mia stack rm                            # forget it; branches and worktrees stay
mia stack rm --worktrees --branches     # remove those too
```

`--worktrees` removes each layer's worktree but the main checkout, refusing
what `mia rm` refuses. `--branches` deletes each layer's branch not checked
out anywhere, refusing one with work no other branch has. `--force` does
both anyway.

Every `mia stack` verb but `track` takes `--in <worktree>` to act on
another worktree's stack.
