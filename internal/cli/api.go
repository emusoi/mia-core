package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/stack"
)

func cmdAPI(a *app.App, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mia api <"+apiQueries+"> [worktree]")
		return exitUsage
	}
	target := ""
	if len(args) > 1 {
		target = args[1]
	}
	switch args[0] {
	case "capabilities":
		return emit(apiCapabilities())

	case "worktrees":
		listings, err := a.List(args[1:]...)
		if err != nil {
			return fail(err)
		}
		return emit(listings)

	case "machines":
		return emit(a.Machines())

	case "here":
		return emit(a.Here())

	case "env":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		manager, err := managerFor(a)
		if err != nil {
			return fail(err)
		}
		return emit(manager.Snapshot(record))

	case "stack":
		record, err := a.RecordOrHere(target)
		if err != nil {
			return fail(err)
		}
		branch := git.CurrentBranch(record.Path)
		if branch == "" {
			return emit([]stack.Layer{})
		}
		// as `mia stack show --json` says them: each with the worktree it is in
		layers, err := a.Layers(record.Path, branch)
		if err != nil {
			return fail(err)
		}
		return emit(append([]stack.Layer{}, layers...))

	case "conversations":
		rest := args[1:]
		want := app.Wanted{Worktree: takeValue(&rest, "--worktree"), Agent: takeValue(&rest, "--agent")}
		if limit := takeValue(&rest, "--limit"); limit != "" {
			n, err := strconv.Atoi(limit)
			if err != nil || n < 0 {
				return usageErr(conversationsUsage)
			}
			want.Limit = n
		}
		if len(rest) > 0 {
			return usageErr(conversationsUsage)
		}
		heads, problems := a.Conversations(want)
		for _, problem := range problems {
			fmt.Fprintln(os.Stderr, "mia:", problem) // the others are listed all the same
		}
		return emit(append([]app.Head{}, heads...))

	case "conversation":
		rest := args[1:]
		after, worktree := 0, takeValue(&rest, "--worktree")
		if value := takeValue(&rest, "--after"); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return usageErr(conversationUsage)
			}
			after = n
		}
		if len(rest) != 1 {
			return usageErr(conversationUsage)
		}
		turns, err := a.ConversationIn(rest[0], worktree, after)
		if err != nil {
			return fail(err)
		}
		return emit(turns)

	case "base":
		return emit(baseBranch{Branch: a.BaseBranch(a.Root)})

	default:
		fmt.Fprintf(os.Stderr, "mia: no such query %q — try one of %s\n", args[0], apiQueries)
		return exitNotFound
	}
}

// baseBranch is what `mia api base` says.
type baseBranch struct {
	Branch string `json:"branch"`
}

var apiQueryList = []string{"capabilities", "worktrees", "machines", "here", "repos", "env", "stack", "base", "conversations", "conversation", "main", "standing", "agents", "setups", "accounts"}

const (
	setupsUsage        = "mia api setups [--repository <name> | --all]"
	conversationsUsage = "mia api conversations [--worktree <name>] [--agent <agent>] [--limit <n>]"
	conversationUsage  = "mia api conversation <id> [--worktree <name>] [--after <n>]"
)

var apiQueries = strings.Join(apiQueryList, "|")

// apiVersion is what `mia api capabilities` says the JSON speaks; it goes
// up when a field changes meaning or goes, not when one is added.
const apiVersion = 1

type capabilities struct {
	Version  int      `json:"version"`
	Queries  []string `json:"queries"`
	Features []string `json:"features"`
}

func apiCapabilities() capabilities {
	return capabilities{
		Version: apiVersion,
		Queries: apiQueryList,
		Features: []string{
			"direct-http", "project-config", "doctor", "environment-readiness", "environment-drift",
			"local-sessions", "conversations", "main-agent", "standing", "remote-sync-preview", "sync-conflicts", "safe-cleanup", "stack-sync", "merged-gc", "compose-environments", "agent-models", "setups", "resume-model", "main-start-agent", "accounts", "main-asks", "main-start-new", "main-conversations", "machine-reach", "roles",
		},
	}
}

// cmdAPISetups is the setups the person has used, from any folder: in a
// repository, that repository's unless --all or --repository says
// otherwise; outside one, every repository's.
func cmdAPISetups(args []string) int {
	all, repository := take(&args, "--all"), takeValue(&args, "--repository")
	if len(args) > 0 || (all && repository != "") {
		return usageErr(setupsUsage)
	}
	a := app.Outside()
	here, _ := os.Getwd()
	if in, err := app.Open(here); err == nil {
		a = in
		if !all && repository == "" {
			repository = filepath.Base(in.Root)
		}
	} else if a.Config, err = config.Load(""); err != nil {
		return fail(err)
	}
	a.Plugins = pluginHost(a.Config)
	return emit(a.Setups(repository))
}
