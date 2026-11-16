package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const (
	mainUsage      = "mia main [status|start [--agent <agent>] [--model <model>] [--effort <effort>]|stop|say <text…>|press <keys…>|interrupt|model <model> [--effort <effort>]]"
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
	// what is said to it and the keys pressed in it are taken as they are typed
	typed := sub == "say" || sub == "press"
	asJSON := !typed && take(&args, "--json")
	var agent, model, effort string
	if sub == "start" || sub == "model" {
		model, effort = takeValue(&args, "--model"), takeValue(&args, "--effort")
	}
	if sub == "start" {
		agent = takeValue(&args, "--agent")
	}
	if sub == "model" {
		if model != "" || len(args) != 1 || strings.HasPrefix(args[0], "-") {
			return usageErr("mia main model <model> [--effort <effort>]")
		}
		model, args = args[0], nil
	}
	if !slices.Contains([]string{"status", "start", "stop", "say", "press", "interrupt", "model"}, sub) || (!typed && len(args) > 0) {
		return usageErr(mainUsage)
	}
	text := ""
	if typed {
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if text = strings.TrimSpace(strings.Join(args, " ")); text == "" {
			return usageErr(map[string]string{"say": "mia main say <text…>", "press": "mia main press <keys…>"}[sub])
		}
	}
	a, err := atMain()
	if err != nil {
		return fail(err)
	}
	if machine, elsewhere, err := a.MainElsewhere(); err != nil {
		return fail(err)
	} else if elsewhere {
		// it lives on another machine: that machine's mia does it, and says it
		ask, state := []string{"main", sub}, asJSON && (sub == "status" || sub == "start")
		switch {
		case typed:
			ask = append(ask, "--", text)
		case sub == "model":
			ask = append(ask, model)
		case model != "":
			ask = append(ask, "--model", model)
		}
		if agent != "" {
			ask = append(ask, "--agent", agent)
		}
		if effort != "" {
			ask = append(ask, "--effort", effort)
		}
		if state {
			ask = append(ask, "--json")
		}
		out, err := a.OnMainMachine(machine, false, ask...)
		switch {
		case err != nil:
			return fail(err)
		case state:
			return sayMainOf(machine.Name, out)
		case out != "":
			fmt.Println(out)
		}
		return exitOK
	}
	switch sub {
	case "status":
		return sayMain(a.Main(), asJSON)
	case "start":
		state, err := a.StartMain(agent, model, effort)
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
		return exitFor(a.SayToMain(text))
	case "press":
		return exitFor(a.PressInMain(text))
	case "model":
		chosen, err := a.SwitchMain(model, effort)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("the main agent is switched to %s\n", said(chosen))
		return exitOK
	}
	return exitFor(a.InterruptMain())
}

// sayMainOf prints where the main agent stands as another machine's mia
// said it, with that machine's name where it said local.
func sayMainOf(machine, said string) int {
	var state app.MainState
	if err := json.Unmarshal([]byte(said), &state); err != nil {
		return fail(fmt.Errorf("%s's mia did not say where the main agent stands: %w", machine, err))
	}
	state.Machine = machine
	return emit(state)
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
	machine, elsewhere, err := a.MainElsewhere()
	if err != nil {
		return fail(err)
	}
	if len(rest) == 0 {
		if elsewhere {
			out, err := a.OnMainMachine(machine, true, "api", "main")
			if err != nil {
				return fail(err)
			}
			return sayMainOf(machine.Name, out)
		}
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
	if elsewhere {
		out, err := a.OnMainMachine(machine, true, "api", "main", "turns", "--after", strconv.Itoa(after))
		if err != nil {
			return fail(err)
		}
		fmt.Println(out) // its turns, as its mia said them
		return exitOK
	}
	turns, err := a.MainTurns(after)
	if err != nil {
		return fail(err)
	}
	return emit(turns)
}
