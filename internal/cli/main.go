package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const (
	mainUsage      = "mia main [status|start|stop|say <text…>|interrupt]"
	mainTurnsUsage = "mia api main [turns [--after <n>]]"
	standingUsage  = "mia api standing [--here] [--seen <repository>/<worktree>[@<machine>]]"
)

// atMain is mia where the main agent lives, which is no repository.
func atMain() (*app.App, error) {
	a, err := app.AtMain()
	if err != nil {
		return nil, err
	}
	a.Plugins = pluginHost(a.Config)
	return a, nil
}

// cmdMain is mia main: the one agent a person talks to, which runs the
// others. Its arguments come as they were typed, so that what is said to
// it is not read for flags.
func cmdMain(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	if sub == "-h" || sub == "--help" {
		verbUsage(os.Stdout, "main")
		return exitOK
	}
	asJSON := sub != "say" && take(&args, "--json")
	if sub != "say" && len(args) > 0 {
		return usageErr(mainUsage)
	}
	a, err := atMain()
	if err != nil {
		return fail(err)
	}
	switch sub {
	case "status":
		return sayMain(a.Main(), asJSON)
	case "start":
		state, err := a.StartMain()
		if err != nil {
			return fail(err)
		}
		return sayMain(state, asJSON)
	case "stop":
		if err := a.StopMain(); err != nil {
			return fail(err)
		}
		fmt.Println("the main agent is stopped; its folder and conversations stay")
		return exitOK
	case "say":
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		text := strings.TrimSpace(strings.Join(args, " "))
		if text == "" {
			return usageErr("mia main say <text…>")
		}
		return exitFor(a.SayToMain(text))
	case "interrupt":
		return exitFor(a.InterruptMain())
	}
	return usageErr(mainUsage)
}

func sayMain(state app.MainState, asJSON bool) int {
	if asJSON {
		return emit(state)
	}
	if !state.Running {
		fmt.Printf("the main agent is not running — `mia main start` starts %s in %s\n", state.Agent, state.Home)
		return exitOK
	}
	doing := "idle"
	if state.Working {
		doing = "working"
	}
	fmt.Printf("%s is the main agent, %s, in window %s of %s since %s\n%s\n", state.Agent, doing, state.Window.Name, state.Session, state.Since.Local().Format("Jan 2 15:04"), state.Home)
	return exitOK
}

// cmdAPIMain is the queries that are about no repository's worktrees but
// the person's: the main agent, and where every agent stands.
func cmdAPIMain(args []string) int {
	a, err := atMain()
	if err != nil {
		return fail(err)
	}
	query, rest := args[0], args[1:]
	if query == "standing" {
		here := take(&rest, "--here")
		seen := takeValue(&rest, "--seen")
		if len(rest) > 0 {
			return usageErr(standingUsage)
		}
		if seen != "" {
			if err := a.SeeStanding(seen); err != nil {
				return fail(err)
			}
		}
		standing, err := a.Standing(!here)
		if err != nil {
			return fail(err)
		}
		return emit(standing)
	}
	if len(rest) == 0 {
		return emit(a.Main())
	}
	after := 0
	if value := takeValue(&rest, "--after"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return usageErr(mainTurnsUsage)
		}
		after = n
	}
	if len(rest) != 1 || rest[0] != "turns" {
		return usageErr(mainTurnsUsage)
	}
	turns, err := a.MainTurns(after)
	if err != nil {
		return fail(err)
	}
	return emit(turns)
}
