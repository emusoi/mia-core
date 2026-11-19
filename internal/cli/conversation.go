package cli

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/app"
)

const conversationVerbUsage = "mia conversation resume <id> [--worktree <name>] [--in <worktree>] [--model <model>] [--effort <effort>] [--account <account>]  ·  mia conversation continue <id> --with <agent> [--worktree <name>] [--model <model>] [--effort <effort>] [--account <account>] [--dry-run]  ·  mia conversation continue --from <window> --with <agent> [--worktree <name>] …"

func cmdConversation(a *app.App, args []string, asJSON bool) int {
	from, in, with := takeValue(&args, "--worktree"), takeValue(&args, "--in"), takeValue(&args, "--with")
	model, effort, account := takeValue(&args, "--model"), takeValue(&args, "--effort"), takeValue(&args, "--account")
	window, dry := takeValue(&args, "--from"), take(&args, "--dry-run")
	if len(args) > 0 && args[0] == "continue" {
		// one conversation, said one way: its id, or the window that has it open
		if with == "" || in != "" || (len(args) == 2) == (window != "") || len(args) > 2 {
			return usageErr(conversationVerbUsage)
		}
		want := app.Continue{Worktree: from, Window: window, With: with, Model: model, Effort: effort, Account: account, DryRun: dry}
		if len(args) == 2 {
			want.ID = args[1]
		}
		return continueConversation(a, want, asJSON)
	}
	if len(args) != 2 || args[0] != "resume" || with != "" || window != "" || dry {
		return usageErr(conversationVerbUsage)
	}
	done, err := a.ResumeConversation(args[1], from, in, model, effort, account)
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
		copied = ", on a copy of the conversation made for this folder and account"
	}
	fmt.Printf("%s: window %s opened — %s%s\n", done.Worktree, done.Window.Name, done.Command, copied)
	return exitOK
}

// continueConversation is mia conversation continue: one conversation
// written out as a handoff, and another agent started on it.
func continueConversation(a *app.App, want app.Continue, asJSON bool) int {
	done, err := a.ContinueConversation(want)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(done)
	}
	in, window, would := done.From.Worktree, done.To.Window.Name, ""
	if done.DryRun {
		would = "would be "
	}
	fmt.Printf("%s: %s's conversation %s, %d turns, %swritten out to %s\n", in, done.From.Agent, done.From.ID, done.Turns, would, done.Handoff)
	if done.LeftOut > 0 {
		fmt.Printf("%s: %d turns of its middle %sleft out of that, to keep it short, and every turn %swritten to %s\n", in, done.LeftOut, would, would, done.Full)
	}
	lost := fmt.Sprintf("not carried over: what %s thought and what came back from its tools — %s has what was said and each thing done, in a line", done.From.Agent, done.To.Agent)
	if done.DryRun {
		fmt.Printf("%s: a window %s would open — %s — and be sent:\n%s\n%s\n", in, window, done.Command, done.Instruction, lost)
		return exitOK
	}
	fmt.Printf("%s: window %s opened — %s\n", in, window, done.Command)
	if !done.Sent {
		fmt.Printf("%s: window %s was not sent its first message: %s\nsay it there yourself:\n%s\n%s\n", in, window, done.Problem, done.Instruction, lost)
		return exitFailed
	}
	fmt.Printf("%s: window %s was told to read it and go on from where it stops\n%s\n", in, window, lost)
	return exitOK
}
