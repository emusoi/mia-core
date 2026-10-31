package cli

import (
	"fmt"
	"github.com/emusoi/mia-core/internal/run"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
)

func cmdPR(a *app.App, args []string, asJSON bool) int {
	options := app.PRDraftOptions{}
	if branch := takeValue(&args, "--branch"); branch != "" {
		options.Branch = &branch
	}
	hasTitle := slices.Contains(args, "--title")
	if title := takeValue(&args, "--title"); hasTitle {
		options.Title = &title
	}
	if remove := takeValue(&args, "--remove-after-merge"); remove != "" {
		value, err := strconv.ParseBool(remove)
		if err != nil {
			return usageErr("mia pr [worktree] draft --remove-after-merge <true|false>")
		}
		options.RemoveAfterMerge = &value
	}
	bodyStdin := take(&args, "--body-stdin")
	sub := "show"
	for i, arg := range args {
		if arg == "show" || arg == "draft" {
			sub = arg
			args = append(args[:i:i], args[i+1:]...)
			break
		}
	}
	if len(args) > 1 || (sub == "show" && (options.Branch != nil || options.Title != nil || options.RemoveAfterMerge != nil || bodyStdin)) {
		return usageErr("mia pr [worktree] [show|draft [--branch <branch>] [--title <title>] [--body-stdin] [--remove-after-merge <true|false>]]")
	}
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	if name, machine, ok := strings.Cut(target, "@"); ok && name != "" {
		if sub != "show" {
			return usageErr("mia pr <worktree>@<machine> — a draft is saved once the branch is home: mia pr draft --branch <branch>")
		}
		return landFrom(a, machine, name)
	}
	record, err := a.RecordOrHere(target)
	if err != nil {
		return fail(err)
	}
	var draft app.PRDraft
	if sub == "draft" {
		if bodyStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fail(err)
			}
			body := string(data)
			options.Body = &body
		}
		draft, err = a.DraftPR(record, options)
	} else {
		draft, err = a.PRDraft(record)
	}
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(draft)
	}
	fmt.Printf("%s\n\n%s\n", draft.Title, draft.Body)
	if draft.Saved {
		fmt.Printf("draft saved to %s\n", draft.Path)
	} else {
		fmt.Printf("preview — `mia pr %s draft` saves it to %s\n", record.Name, draft.Path)
	}
	fmt.Println("\nrun these yourself:")
	if !draft.Saved {
		fmt.Printf("mia pr %s draft\n", record.Name)
	}
	fmt.Printf("%s\n%s\n", draft.PushCommand, draft.CreateCommand)
	if !git.HasRemote(record.Path, "origin") {
		fmt.Println("\nthis repository has no remote called origin yet — `git remote add origin <url>` (or `gh repo create`) first")
	}
	if draft.RemoveAfterMerge && draft.RemoveCommand != "" {
		fmt.Printf("\nafter merge:\n%s\n", draft.RemoveCommand)
	}
	return exitOK
}

// landFrom brings a worktree's branch home from the machine that keeps it,
// then says what to run to open its pull request from here.
func landFrom(a *app.App, machine, name string) int {
	listing, err := a.RemoteWorktree(machine, name)
	if err != nil {
		return fail(err)
	}
	if listing.Branch == "" {
		return fail(fmt.Errorf("%s on %s has no branch checked out", name, machine))
	}
	head, err := a.BringHome(machine, listing.Branch)
	if err != nil {
		return fail(err)
	}
	base := a.BaseBranch(a.Root)
	fmt.Printf("%s is here now, at %.7s, as %s has it\n\nrun these yourself:\n", listing.Branch, head, machine)
	fmt.Printf("git -C %s push -u origin %s\n", run.ShellJoin([]string{a.Root}), run.ShellJoin([]string{listing.Branch}))
	fmt.Printf("cd %s && gh pr create --base %s --head %s --fill\n", run.ShellJoin([]string{a.Root}), run.ShellJoin([]string{base}), run.ShellJoin([]string{listing.Branch}))
	return exitOK
}
