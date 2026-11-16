package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/runtime"
)

// cmdMove is mia move <worktree>[@<machine>] <machine|local>: the worktree,
// its uncommitted work and its windows, taken to the other side.
func cmdMove(a *app.App, args []string) int {
	dry := take(&args, "--dry-run")
	if len(args) != 2 {
		return usageErr("mia move [--dry-run] <worktree>[@<machine>] <machine|local>")
	}
	name, from, _ := strings.Cut(args[0], "@")
	if from == "" {
		from = runtime.Local
	}
	moved, err := a.Move(name, from, args[1], dry, os.Stdout)
	if err != nil {
		return fail(err)
	}
	if dry {
		there := "there"
		if args[1] == runtime.Local {
			there = "here"
		}
		var what []string
		for _, w := range moved.Windows {
			what = append(what, w.What())
		}
		fmt.Printf("would open %d window(s) %s: %s\nnothing was moved\n", len(what), there, strings.Join(what, " · "))
		return exitOK
	}
	where, there := args[1], "there"
	if where == runtime.Local {
		where, there = "this computer", "here"
	}
	fmt.Printf("%s — moved to %s, %d window(s) open %s", moved.Name, where, len(moved.Windows), there)
	if len(moved.Carried) > 0 {
		fmt.Printf(", carrying %s", strings.Join(moved.Carried, ", "))
	}
	fmt.Println()
	if args[1] != runtime.Local {
		fmt.Printf("%s@%s — on %s\n", moved.Name, args[1], args[1])
	}
	return exitOK
}
