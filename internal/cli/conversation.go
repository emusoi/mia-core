package cli

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/app"
)

const conversationVerbUsage = "mia conversation resume <id> [--worktree <name>] [--in <worktree>] [--model <model>] [--effort <effort>] [--account <account>]  ·  mia conversation continue <worktree|.> --with <agent> [--from <window|agent|id>] [--model <model>] [--effort <effort>] [--account <account>] [--dry-run]"

func cmdConversation(a *app.App, args []string, asJSON bool) int {
	from, in, with := takeValue(&args, "--worktree"), takeValue(&args, "--in"), takeValue(&args, "--with")
	model, effort, account := takeValue(&args, "--model"), takeValue(&args, "--effort"), takeValue(&args, "--account")
	of, dry := takeValue(&args, "--from"), take(&args, "--dry-run")
	if len(args) == 2 && args[0] == "continue" && from == "" && in == "" && with != "" {
		return continueConversation(a, app.Continue{Worktree: args[1], From: of, With: with,
			Model: model, Effort: effort, Account: account, DryRun: dry}, asJSON)
	}
	if len(args) != 2 || args[0] != "resume" || with != "" || of != "" || dry {
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

// continueConversation is mia conversation continue: a worktree's
// conversation written out as a handoff, and another agent started on it.
func continueConversation(a *app.App, want app.Continue, asJSON bool) int {
	if want.Worktree == "." {
		want.Worktree = "" // the worktree this runs in
	}
	done, err := a.ContinueConversation(want)
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(done)
	}
	if done.DryRun {
		fmt.Printf("%s: %s's conversation %s, %d turns, would be written out to %s\n", done.Worktree, done.From, done.Conversation, done.Turns, done.Handoff)
		fmt.Printf("%s: a window %s would open — %s — and be sent:\n%s\n", done.Worktree, done.Window.Name, done.Command, done.Instruction)
		return exitOK
	}
	fmt.Printf("%s: %s's conversation %s, %d turns, written out to %s\n", done.Worktree, done.From, done.Conversation, done.Turns, done.Handoff)
	fmt.Printf("%s: window %s opened — %s\n", done.Worktree, done.Window.Name, done.Command)
	if !done.Sent {
		fmt.Printf("%s: window %s was not sent its first message: %s\nsay it there yourself:\n%s\n", done.Worktree, done.Window.Name, done.Problem, done.Instruction)
		return exitFailed
	}
	fmt.Printf("%s: window %s was told to read it and go on from where it stops\n", done.Worktree, done.Window.Name)
	return exitOK
}
