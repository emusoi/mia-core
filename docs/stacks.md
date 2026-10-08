# Stacks

A stack is several branches in one worktree, each built on the one below.
Each branch is a layer, and each layer becomes its own pull request.

```
main
 └─ schema
     └─ api
         └─ ui
```

## Build

```bash
mia new schema --stack     # a branch on top of this one, checked out here
mia new api --stack
```

`--worktree` gives the new layer its own worktree. `--in <worktree>` adds to
another worktree's stack.

## Move

```bash
mia up                     # the layer above
mia down                   # the layer below
mia stack go api           # a layer by name
```

These refuse when tracked files have uncommitted changes.

## Look

```bash
mia stack                  # the layers, landed or not, and which needs a restack
mia stack name payments    # name it; then mia switch payments
mia stack diff             # this layer against the one below
```

## Keep it in shape

```bash
mia stack restack          # rebase every layer onto the one below
mia stack merge ui onto api    # fold layers into a lower one
```

mia never merges into your base branch.

## Land

```bash
mia stack pr
```

prints `git push` and `gh pr create` for each layer, bottom first, for you to
run.

## Forget

```bash
mia stack rm                       # forget the stack; branches and worktrees stay
mia stack rm --worktrees --branches    # remove those too
```

`--branches` refuses a branch with work nowhere else; `--force` deletes it
anyway.
