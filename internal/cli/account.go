package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
)

const accountUsage = "mia account [ls|add <agent> <name> [--in <worktree>]|login <agent> <name> [--in <worktree>]|share <agent> (<name> | --all)|signals <agent> [<name> | --all] [--dry-run]|use <agent> <name> [--repository <name>|--global]|rename <agent> <name> <new name>|rm [--force] <agent> <name>]"

// atAccounts is mia where accounts are looked at from: the repository
// here, whose worktrees a window can sign one in to, or, outside one, the
// main agent's folder. Accounts are the person's, the same from anywhere.
func atAccounts() (*app.App, error) {
	here, _ := os.Getwd()
	return accountsAt(here)
}

// accountsAt is atAccounts, here being a folder that is given.
func accountsAt(dir string) (*app.App, error) {
	a, err := app.AtMain()
	if dir != "" {
		if in, oerr := app.Open(dir); oerr == nil {
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
	force, global, all, dry := take(&args, "--force"), take(&args, "--global"), take(&args, "--all"), take(&args, "--dry-run")
	if (in != "" && sub != "add" && sub != "login") || (force && sub != "rm") || (repository != "" && sub != "use") ||
		(global && (sub != "use" || repository != "")) || (all && sub != "share" && sub != "signals") || (dry && sub != "signals") {
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
			if one.UserDefault {
				using += "  (user-wide default)"
			}
			if len(one.Shared) > 0 {
				using += fmt.Sprintf("  shared: %d", len(one.Shared))
			}
			fmt.Printf("%-10s %-12s signed in: %-8s %s%s\n", one.Agent, one.Name, one.LoggedIn, one.Home, using)
		}
		return exitOK
	case (sub == "add" || sub == "login") && len(args) == 2:
		add := a.AddAccount
		if sub == "login" {
			add = a.LoginAccount
		}
		opened, err := add(args[0], args[1], in)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(opened)
		}
		fmt.Printf("%s account %s: %s\n", opened.Account.Agent, opened.Account.Name, opened.Account.Home)
		sayShared(opened.Shared)
		fmt.Printf("window %s of %s runs `%s` — sign in there with %s's own login; mia signs nothing in\n", opened.Window, opened.Session, opened.Command, opened.Account.Agent)
		if in == "" {
			fmt.Println(goesThere(opened.Session, opened.Window, os.Getenv("TMUX") != "") + " goes there")
		}
		return exitOK
	case sub == "share" && ((len(args) == 2 && !all) || (len(args) == 1 && all)):
		name := ""
		if !all {
			name = args[1]
		}
		shared, err := a.ShareAccount(args[0], name)
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(shared)
		}
		for _, one := range shared {
			fmt.Printf("%s account %s:\n", one.Agent, one.Name)
			sayShared(one)
		}
		if len(shared) == 0 {
			fmt.Printf("%s has no account of mia's to share its own folder's setup with\n", args[0])
		}
		return exitOK
	case sub == "signals" && ((len(args) == 1) || (len(args) == 2 && !all)):
		name := ""
		if len(args) == 2 {
			name = args[1]
		}
		done, err := a.SignalAccount(args[0], name, all, dry)
		if asJSON && err == nil {
			return emit(done)
		}
		kept := false
		for _, one := range done {
			would := map[bool]string{false: "", true: "would be "}[one.DryRun]
			switch one.Did {
			case app.SignalsWritten, app.SignalsUpdated:
				fmt.Printf("%s account %s: %s %s%s — %s says what it does with mia signal from its next start, or /reload in one that runs\n", one.Agent, one.Name, one.File, would, one.Did, one.Agent)
			case app.SignalsAlready:
				fmt.Printf("%s account %s: %s is mia's already, as it is now — nothing to write\n", one.Agent, one.Name, one.File)
			case app.SignalsKept:
				kept = true
				fmt.Printf("%s account %s: %s is yours, not one mia wrote — left as it is; move it away to have mia's\n", one.Agent, one.Name, one.File)
			}
		}
		if err != nil {
			return fail(err)
		}
		if kept {
			return exitFailed
		}
		return exitOK
	case sub == "use" && len(args) == 2 && global:
		if err := a.HasAccount(args[0], args[1]); err != nil {
			return fail(err)
		}
		if err := setUserAccount(args[0], args[1]); err != nil {
			return fail(err)
		}
		chosen := accountNamed(a.Accounts(), args[0], args[1])
		if asJSON {
			return emit(chosen)
		}
		fmt.Printf("%s's sessions start on %s from now on wherever a repository says no other, as %s says; a window that runs already stays on its own\n", chosen.Agent, chosen.Name, config.GlobalPath())
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
		if err := followUserAccount(renamed.Agent, args[1], renamed.Name); err != nil {
			return fail(err)
		}
		fmt.Printf("%s account %s is %s now: %s\n", renamed.Agent, args[1], renamed.Name, renamed.Home)
		if a.KeepsSignInElsewhere(renamed.Agent) {
			fmt.Printf("%s may keep its sign-in under the folder's old path, out of the folder: if it asks, `mia account login %s %s` signs in again\n", renamed.Agent, renamed.Agent, renamed.Name)
		}
		return exitOK
	case sub == "rm" && len(args) == 2:
		gone, err := a.RemoveAccount(args[0], args[1], force)
		if err != nil {
			return fail(err)
		}
		if err := followUserAccount(gone.Agent, gone.Name, app.DefaultAccount); err != nil {
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

// sayShared says what of the agent's own folder an account links to.
func sayShared(shared app.Sharing) {
	if linked := slices.Concat(shared.Linked, shared.Already); len(linked) > 0 {
		fmt.Printf("shares %s from %s", strings.Join(linked, ", "), shared.From)
		if len(shared.Linked) > 0 && len(shared.Already) > 0 {
			fmt.Printf(" (linked now: %s)", strings.Join(shared.Linked, ", "))
		}
		fmt.Println()
	} else if shared.From != "" {
		fmt.Printf("shares nothing from %s: it has none of the setup mia shares\n", shared.From)
	}
	if len(shared.Kept) > 0 {
		fmt.Printf("keeps its own %s: mia links nothing over what the account has\n", strings.Join(shared.Kept, ", "))
	}
}

// goesThere is the tmux command line that takes a terminal to a window of
// a session, to be pasted into a shell as it is: switch-client from inside
// tmux, attach from outside it. The target has no =, which zsh expands as
// a command's path, and is quoted when it has anything a shell reads.
func goesThere(session, window string, inTmux bool) string {
	verb := "attach"
	if inTmux {
		verb = "switch-client"
	}
	target := session + ":" + window
	if !plainWord.MatchString(target) {
		target = "'" + strings.ReplaceAll(target, "'", `'\''`) + "'"
	}
	return "tmux " + verb + " -t " + target
}

// plainWord is a word no shell changes.
var plainWord = regexp.MustCompile(`^[A-Za-z0-9_.:/+-]+$`)

func accountNamed(all []app.Account, agent, name string) app.Account {
	for _, one := range all {
		if one.Agent == agent && one.Name == name {
			return one
		}
	}
	return app.Account{}
}

// setUserAccount sets the account the user-wide config starts an agent's
// sessions on, [agents.<agent>] account; default takes the line out.
func setUserAccount(agent, name string) error {
	path := config.GlobalPath()
	if path == "" {
		return errors.New("cannot find a home directory to keep the config in")
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	table := "agents." + agent
	edited := unsetting(string(data), table, "account")
	if name != app.DefaultAccount {
		edited = setting(string(data), table, "account", strconv.Quote(name))
	}
	if err := writeConfig(path, edited); err != nil {
		return fmt.Errorf("agents.%s.account = %s: %w", agent, name, err)
	}
	return nil
}

// followUserAccount makes the user-wide config, when it starts an agent's
// sessions on an account renamed or removed, start them on the one it is
// now: its new name, or default.
func followUserAccount(agent, was, now string) error {
	users, err := config.Load("")
	if err != nil || users.Agents[agent].Account != was {
		return nil // one mia cannot read it leaves as it is
	}
	return setUserAccount(agent, now)
}
