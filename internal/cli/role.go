package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const roleUsage = "mia role [ls|show <name>|path <name>]"

// cmdRole is mia role: the person's roles, each a setup with the first
// message its agent is sent. They are written by hand; mia only reads them.
func cmdRole(args []string, asJSON bool) int {
	sub := "ls"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch {
	case (sub == "ls" || sub == "list") && len(args) == 0:
		roles, problems := app.Roles()
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "mia:", problem) // the others are listed all the same
		}
		if asJSON {
			return emit(append([]app.Role{}, roles...))
		}
		if len(roles) == 0 && len(problems) == 0 {
			fmt.Println("no roles — `mia role path <name>` says where to write one")
		}
		for _, role := range roles {
			first, _, _ := strings.Cut(role.Body, "\n")
			fmt.Printf("%-14s %s  %s\n", role.Name, roleSaid(role), first)
		}
		return exitOK
	case sub == "show" && len(args) == 1:
		role, err := app.ReadRole(args[0])
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(role)
		}
		fmt.Printf("%s  %s\n%s\n\n%s\n", role.Name, roleSaid(role), role.Path, role.Body)
		return exitOK
	case sub == "path" && len(args) == 1:
		path, err := app.RolePath(args[0])
		if err != nil {
			return fail(err)
		}
		fmt.Println(path)
		return exitOK
	}
	return usageErr(roleUsage)
}

// roleSaid is what a role's front matter says, in a line.
func roleSaid(role app.Role) string {
	var said []string
	for _, one := range []string{role.Agent, role.Model, role.Effort} {
		if one != "" {
			said = append(said, one)
		}
	}
	if role.Account != "" {
		said = append(said, "on "+role.Account)
	}
	if len(said) == 0 {
		return "the default setup"
	}
	return strings.Join(said, " · ")
}
