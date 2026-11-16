package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const accountUsage = "mia account [ls|add <agent> <name> [--in <worktree>]|use <agent> <name> [--repository <name>]|rename <agent> <name> <new name>|rm [--force] <agent> <name>]"

// atAccounts is mia where accounts are looked at from: the repository
// here, whose worktrees a window can sign one in to, or, outside one, the
// main agent's folder. Accounts are the person's, the same from anywhere.
func atAccounts() (*app.App, error) {
	a, err := app.AtMain()
	if here, werr := os.Getwd(); werr == nil {
		if in, oerr := app.Open(here); oerr == nil {
			a, err = in, nil
		}
	}
	if err != nil {
		return nil, err
	}
	a.Plugins = pluginHost(a.Config)
	return a, nil
}

// cmdAccount is mia account: an agent's logins, each a folder the agent
// keeps its sign-in and its conversations in.
func cmdAccount(args []string, asJSON bool) int {
	sub := "ls"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	in, repository := takeValue(&args, "--in"), takeValue(&args, "--repository")
	force := take(&args, "--force")
	if (in != "" && sub != "add") || (force && sub != "rm") || (repository != "" && sub != "use") {
		return usageErr(accountUsage)
	}
	a, err := atAccounts()
	if err != nil {
		return fail(err)
	}
	switch {
	case (sub == "ls" || sub == "list") && len(args) == 0:
		accounts := a.Accounts()
		if asJSON {
			return emit(accounts)
		}
		for _, one := range accounts {
			using := ""
			if len(one.Sessions) > 0 {
				using = "  in " + strings.Join(one.Sessions, ", ")
			}
			fmt.Printf("%-10s %-12s signed in: %-8s %s%s\n", one.Agent, one.Name, one.LoggedIn, one.Home, using)
		}
		return exitOK
	case sub == "add" && len(args) == 2:
		opened, err := a.AddAccount(args[0], args[1], in)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(opened)
		}
		fmt.Printf("%s account %s: %s\n", opened.Account.Agent, opened.Account.Name, opened.Account.Home)
		fmt.Printf("window %s of %s runs `%s` — sign in there with %s's own login; mia signs nothing in\n", opened.Window, opened.Session, opened.Command, opened.Account.Agent)
		if in == "" {
			fmt.Printf("tmux attach -t =%s goes there\n", opened.Session)
		}
		return exitOK
	case sub == "use" && len(args) == 2:
		if repository == "" {
			if a.MiaDir == "" || !strings.HasSuffix(a.MiaDir, "/.git/mia") {
				return fail(fmt.Errorf("not in a repository — name one with --repository <name>, its main checkout's folder name"))
			}
			repository = filepath.Base(a.Root)
		}
		project, err := a.UseAccount(args[0], args[1], repository)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(project)
		}
		said := map[string]string{"mia": "as you said", "config": "as the config says", "default": "its own folder"}[project.From]
		fmt.Printf("%s starts %s's sessions on %s from now on, %s; a window that runs already stays on its own\n", project.Repository, project.Agent, project.Account, said)
		return exitOK
	case sub == "rename" && len(args) == 3:
		renamed, err := a.RenameAccount(args[0], args[1], args[2])
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(renamed)
		}
		fmt.Printf("%s account %s is %s now: %s\n", renamed.Agent, args[1], renamed.Name, renamed.Home)
		if a.KeepsSignInElsewhere(renamed.Agent) {
			fmt.Printf("%s may keep its sign-in under the folder's old path, out of the folder: if it asks, sign in again\n", renamed.Agent)
		}
		return exitOK
	case sub == "rm" && len(args) == 2:
		gone, err := a.RemoveAccount(args[0], args[1], force)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(gone)
		}
		fmt.Printf("%s account %s removed, its folder %s with it\n", gone.Agent, gone.Name, gone.Home)
		fmt.Printf("mia signs nothing out: signing out is %s's own, and a sign-in it kept out of the folder stays there\n", gone.Agent)
		return exitOK
	}
	return usageErr(accountUsage)
}
