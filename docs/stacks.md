# Stacks

A feature too large for one pull request is a **stack**: several branches,
each built on the one below, all in one worktree. Each branch is a **layer**,
and each layer lands as its own pull request, bottom first.

```
main
 └─ schema        ← the bottom layer: lands first
     └─ api
         └─ ui    ← the top
```

Because the layers share a worktree, moving between them is a `git checkout`
in place. The session, the environment and everything running in them stay
put.

## Building one

Start on the branch the stack grows from, and add layers on top:

    mia new schema --stack     # a branch on top of the one you are on, checked out here
    mia new api --stack
    mia new ui --stack

Each new layer is a branch off the current one, checked out in place.
`--in <worktree>` adds it to another worktree's stack. `--worktree` gives the
new layer a worktree of its own instead, so two layers can run and be tested
side by side.

A layer added in the middle of a stack is inserted: the layers above it move
on top of it.

## Moving between layers

    mia up                     # the layer above
    mia down                   # the layer below
    mia stack go api           # a layer by name

These refuse when tracked files have uncommitted changes, rather than carry
them to another branch. At either end they say so and do nothing.

## Seeing it

    mia stack

```
payments — 1/3 layers landed
  schema   landed
▸ api      4 commit(s)
  ui       2 commit(s) · needs restack   in kijenge
```

`▸` is the layer checked out here. A layer is *landed* once the base branch
contains it, and *needs restack* when the layer below has moved on without
it. `mia stack --json` gives the same as data; the dashboard shows a stack as
one row that unfolds into its layers, and `S` opens the stack panel.

    mia stack name payments    # name it
    mia switch payments        # and go to it by name, from anywhere
    mia stack diff             # this layer against the one below

## Keeping it in shape

When a lower layer changes — you amend it, or it is rebased onto a new base —
the layers above have to follow:

    mia stack restack

rebases every layer that is not landed onto the layer below, bottom up, and
leaves you on the layer you started on. It refuses when the tree has any
uncommitted change, untracked files included, since it rewrites history under
them. If a rebase stops for a conflict, resolve it, `git rebase --continue`,
and run `mia stack restack` again.

### Folding layers together

    mia stack merge ui onto api

fast-forwards `api` to `ui`: the layers in between and `ui` fold into it and
disappear, and worktrees that had them checked out move to `api`. Without
`onto`, a layer merges into the one below it. mia refuses to merge into the
base branch — the bottom layer lands through a pull request — and refuses
when a layer is behind the one below (restack first) or the target has
uncommitted changes.

## Landing it

mia never pushes. It prints the commands, bottom-up, for you to run:

    mia stack pr

```
# bottom-up; run these yourself
git push -u origin api && gh pr create --base schema --head api
git push -u origin ui && gh pr create --base api --head ui
```

Landed layers are left out. `mia pr` drafts the text of one layer's pull
request, against the layer below it.

## Forgetting it

    mia stack rm

forgets the stack — the edges between the layers and its name. The branches
and worktrees stay as ordinary ones. To take them too:

    mia stack rm --worktrees --branches

`--worktrees` removes each layer's worktree (not the main checkout), with the
same refusals as `mia rm`. `--branches` deletes branches no worktree has
checked out, and refuses one whose work exists on no other branch. `--force`
gets past both.

Stacks are kept in `.git/mia/stack.json`.
