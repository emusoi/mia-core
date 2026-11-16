package cli

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/app"
)

func cmdConversation(a *app.App, args []string, asJSON bool) int {
	const usage = "mia conversation resume <id> [--in <worktree>]"
	in := takeValue(&args, "--in")
	if len(args) != 2 || args[0] != "resume" {
		return usageErr(usage)
	}
	done, err := a.ResumeConversation(args[1], in)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(done)
	}
	if done.Already {
		fmt.Printf("%s already has it open, in window %d %s — nothing opened\n", done.Worktree, done.Window.Index, done.Window.Name)
		return exitOK
	}
	copied := ""
	if done.Copied {
		copied = ", on a copy of the conversation made for this folder"
	}
	fmt.Printf("%s: window %s opened — %s%s\n", done.Worktree, done.Window.Name, done.Command, copied)
	return exitOK
}
