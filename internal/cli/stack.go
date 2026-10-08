package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/stack"
)

func stackStore(a *app.App) stack.Store { return a.Stacks() }

func cmdStack(a *app.App, args []string, asJSON bool) (code int) {
	in := takeValue(&args, "--in")
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	record, err := a.RecordOrHere(in)
	if err != nil {
		return fail(err)
	}
	branch := git.CurrentBranch(record.Path)
	if branch == "" {
		return fail(fmt.Errorf("%s is detached, and a stack is made of branches", record.Name))
	}
	base := a.BaseBranch(record.Path)
	store := stackStore(a)
	defer func() {
		if code == exitOK && slices.Contains([]string{"restack", "rm", "forget", "merge", "name", "go"}, sub) {
			a.Emit("stack.changed", record, stackChange{Change: sub})
		}
	}()

	switch sub {
	case "show":
		layers, err := a.Layers(record.Path, branch)
		if err != nil {
			return fail(err)
		}
		return showStack(a, record.Path, layers, asJSON)

	case "restack":
		return restack(a, record.Path, branch, base)

	case "rm", "forget":
		worktrees := take(&args, "--worktrees")
		branches := take(&args, "--branches")
		force := take(&args, "--force")
		gone, err := a.RemoveStack(record.Path, branch, worktrees, branches, force)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("stack forgotten: %s\n", strings.Join(gone.Layers, ", "))
		if len(gone.Worktrees) > 0 {
			fmt.Printf("worktrees removed: %s\n", strings.Join(gone.Worktrees, ", "))
		}
		if len(gone.Branches) > 0 {
			fmt.Printf("branches deleted: %s\n", strings.Join(gone.Branches, ", "))
		}
		if !worktrees && !branches {
			fmt.Println("the layers stay as branches and worktrees — `--worktrees` and `--branches` take those too")
		}
		return exitOK

	case "merge":
		rest := args[1:]
		if len(rest) == 0 {
			return usageErr("mia stack merge <layer|worktree> [onto <layer>]")
		}
		top, dir := rest[0], record.Path
		if namesAWorktree(a, rest[0]) {
			r, err := a.RecordOrHere(rest[0])
			if err != nil {
				return fail(err)
			}
			top, dir = git.CurrentBranch(r.Path), r.Path
		}
		target := ""
		if len(rest) >= 3 && rest[1] == "onto" {
			target = rest[2]
		} else if len(rest) == 2 {
			target = rest[1]
		}
		if target == "" {
			layers, err := a.Layers(dir, top)
			if err != nil {
				return fail(err)
			}
			for _, layer := range layers {
				if layer.Branch == top {
					target = layer.Parent
				}
			}
		}
		merged, err := a.MergeDown(dir, top, target)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s now holds %s\n", merged.Target, strings.Join(merged.Layers, ", "))
		for _, name := range merged.Moved {
			fmt.Printf("%s moved to %s\n", name, merged.Target)
		}
		return exitOK

	case "name":
		if len(args) != 2 {
			return usageErr("mia stack name <name>")
		}
		bottom, err := a.StackName(record, args[1])
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s is the stack on %s — `mia switch %s` goes there\n", args[1], bottom, args[1])
		return exitOK

	case "go":
		if len(args) != 2 {
			return usageErr("mia stack go <layer>")
		}
		layers, err := store.Of(record.Path, branch, base)
		if err != nil {
			return fail(err)
		}
		for _, layer := range layers {
			if layer.Branch != args[1] {
				continue
			}
			if !layer.Current {
				if err := toLayer(record.Path, branch, layer.Branch); err != nil {
					return fail(err)
				}
			}
			fmt.Println(layer.Branch)
			return exitOK
		}
		return fail(fmt.Errorf("%s is not a layer of %s's stack", args[1], record.Name))

	case "diff":
		layers, err := store.Of(record.Path, branch, base)
		if err != nil {
			return fail(err)
		}
		for _, layer := range layers {
			if layer.Current {
				return Run([]string{"run", record.Name, "git", "diff", layer.Parent + "..." + layer.Branch})
			}
		}
		return fail(fmt.Errorf("%s is not in a stack", branch))

	case "pr":
		layers, err := store.Of(record.Path, branch, base)
		if err != nil {
			return fail(err)
		}
		fmt.Println("# bottom-up; run these yourself")
		for _, layer := range layers {
			if layer.Landed {
				continue
			}
			branch, parent := run.ShellJoin([]string{layer.Branch}), run.ShellJoin([]string{layer.Parent})
			fmt.Printf("git push -u origin %s && gh pr create --base %s --head %s\n", branch, parent, branch)
		}
		if !git.HasRemote(record.Path, "origin") {
			fmt.Println("\nthis repository has no remote called origin yet — `git remote add origin <url>` (or `gh repo create`) first")
		}
		return exitOK

	default:
		return usageErr("mia stack [--in <worktree>] [show|go <layer>|name <name>|restack|diff|pr]")
	}
}

func showStack(a *app.App, _ string, layers []stack.Layer, asJSON bool) int {
	rows := append([]stack.Layer{}, layers...)
	landed := 0
	for _, layer := range layers {
		if layer.Landed {
			landed++
		}
	}
	if asJSON {
		return emit(rows)
	}
	if len(rows) == 0 {
		fmt.Println("not a stack — `mia new <name> --stack` adds a layer on top of this branch")
		return exitOK
	}
	if name := a.Stacks().NameOf(rows[0].Branch); name != "" {
		fmt.Printf("%s — ", name)
	}
	fmt.Printf("%d/%d layers landed\n", landed, len(rows))
	width := 0
	for _, r := range rows {
		width = max(width, len(r.Branch))
	}
	for _, r := range rows {
		here := "  "
		if r.Current {
			here = "▸ "
		}
		state := fmt.Sprintf("%d commit(s)", r.Ahead)
		switch {
		case r.Landed:
			state = "landed"
		case r.Ahead == 0:
			state = "not started"
		}
		if r.NeedsRestack {
			state += " · needs restack"
		}
		where := ""
		if r.Worktree != "" {
			where = "  in " + r.Worktree
		}
		fmt.Printf("%s%-*s  %s%s\n", here, width, r.Branch, state, where)
	}
	return exitOK
}

func restack(a *app.App, dir, branch, base string) int {
	moved := 0
	err := a.Restack(dir, branch, base, func(layer stack.Layer) {
		moved++
		fmt.Printf("restacked %s onto %s\n", layer.Branch, layer.Parent)
	})
	if err != nil {
		return fail(err)
	}
	if moved == 0 {
		fmt.Println("every layer already sits on its parent — nothing to restack")
	}
	return exitOK
}

type stackChange struct {
	Change string `json:"change"`
}
