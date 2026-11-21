package cli

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

const (
	mainUsage      = "mia main [status|start [--new|--resume <conversation>] [--restart] [--role <role>] [--agent <agent>] [--model <model>] [--effort <effort>] [--account <account>]|stop|say <text…>|press <keys…>|interrupt|model <model> [--effort <effort>]]"
	mainTurnsUsage = "mia api main [conversations|turns [--conversation <id>] [--after <n>] [--follow]]"
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
	var agent, model, effort, account string
	if sub == "start" || sub == "model" {
		model, effort = takeValue(&args, "--model"), takeValue(&args, "--effort")
	}
	var fresh, restart bool
	var resume, role string
	if sub == "start" {
		agent, account, resume, role = takeValue(&args, "--agent"), takeValue(&args, "--account"), takeValue(&args, "--resume"), takeValue(&args, "--role")
		// --restart ends a conversation: only for another one, new or an
		// earlier one, said in so many words
		if fresh, restart = take(&args, "--new"), take(&args, "--restart"); (restart && !fresh && resume == "") || (fresh && resume != "") {
			return usageErr(mainUsage)
		}
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
		if fresh {
			ask = append(ask, "--new")
		}
		if resume != "" {
			ask = append(ask, "--resume", resume)
		}
		if restart {
			ask = append(ask, "--restart")
		}
		if role != "" {
			ask = append(ask, "--role", role) // that machine's role of the name
		}
		if agent != "" {
			ask = append(ask, "--agent", agent)
		}
		if account != "" {
			ask = append(ask, "--account", account) // that machine's account of the name
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
			return sayMainOf(asker{out: os.Stdout, errs: os.Stderr}, machine.Name, out)
		case out != "":
			fmt.Println(out)
		}
		return exitOK
	}
	switch sub {
	case "status":
		return sayMain(a.Main(), asJSON)
	case "start":
		body := ""
		if role != "" {
			// a role is a setup with a body: what is given here is said over it
			read, err := app.ReadRole(role)
			if err != nil {
				return fail(err)
			}
			if !restart && a.Main().Running {
				return fail(fmt.Errorf("the main agent runs already, and role %s opens a conversation — `mia main stop` first, or `mia main start --new --restart --role %s`", role, role))
			}
			agent, model, effort, account, body = cmp.Or(agent, read.Agent), cmp.Or(model, read.Model), cmp.Or(effort, read.Effort), cmp.Or(account, read.Account), read.Body
		}
		start := a.StartMain
		switch {
		case fresh:
			start = func(agent, model, effort, account string) (app.MainState, error) {
				return a.StartMainNew(agent, model, effort, account, restart)
			}
		case resume != "":
			start = func(agent, model, effort, account string) (app.MainState, error) {
				return a.StartMainResuming(resume, agent, model, effort, account, restart)
			}
		}
		state, err := start(agent, model, effort, account)
		if err != nil {
			return fail(err)
		}
		if body != "" {
			// started, it has settled already: it is ready, or asks something
			if err := a.SayToMain(body); err != nil {
				return fail(fmt.Errorf("the main agent is started, and role %s's opening was not sent: %w", role, err))
			}
			state = a.Main()
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
func sayMainOf(who asker, machine, said string) int {
	var state app.MainState
	if err := json.Unmarshal([]byte(said), &state); err != nil {
		return who.fail(fmt.Errorf("%s's mia did not say where the main agent stands: %w", machine, err))
	}
	state.Machine = machine
	return who.emit(state)
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
	if state.Asks != nil {
		doing = "asking"
	}
	fmt.Printf("%s is the main agent, %s, in window %s of %s since %s\n%s\n", state.Agent, doing, state.Window.Name, state.Session, state.Since.Local().Format("Jan 2 15:04"), state.Home)
	if state.Asks != nil {
		fmt.Println(state.Asks.Said(state.Agent, "mia main press"))
	}
	return exitOK
}

// cmdAPIMain is the queries that are about no repository's worktrees but
// the person's: the main agent, and where every agent stands.
func cmdAPIMain(who asker, args []string) int {
	a, err := atMain()
	if err != nil {
		return who.fail(err)
	}
	query, rest := args[0], args[1:]
	if query == "standing" {
		here := take(&rest, "--here")
		seen := takeValue(&rest, "--seen")
		if len(rest) > 0 {
			return who.usage(standingUsage)
		}
		if seen != "" {
			if err := a.SeeStanding(seen); err != nil {
				return who.fail(err)
			}
		}
		standing, err := a.Standing(!here)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(standing)
	}
	machine, elsewhere, err := a.MainElsewhere()
	if err != nil {
		return who.fail(err)
	}
	if len(rest) == 0 {
		if elsewhere {
			out, err := a.OnMainMachine(machine, true, "api", "main")
			if err != nil {
				return who.fail(err)
			}
			return sayMainOf(who, machine.Name, out)
		}
		return who.emit(a.Main())
	}
	if len(rest) == 1 && rest[0] == "conversations" {
		if elsewhere {
			out, err := a.OnMainMachine(machine, true, "api", "main", "conversations")
			if err != nil {
				return who.fail(err)
			}
			fmt.Fprintln(who.out, out) // its conversations, as its mia said them
			return exitOK
		}
		heads, problems := a.MainConversations()
		for _, problem := range problems {
			fmt.Fprintln(who.errs, "mia:", problem) // the others are listed all the same
		}
		return who.emit(append([]app.Head{}, heads...))
	}
	conversation, follow := takeValue(&rest, "--conversation"), take(&rest, "--follow")
	after := 0
	if value := takeValue(&rest, "--after"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return who.usage(mainTurnsUsage + followNote)
		}
		after = n
	}
	if len(rest) != 1 || rest[0] != "turns" {
		return who.usage(mainTurnsUsage + followNote)
	}
	if elsewhere && follow {
		return who.fail(fmt.Errorf("--follow reads a main agent on this machine, and it lives on %s — follow it with mia there", machine.Name))
	}
	if follow {
		return who.follow(func(stop <-chan struct{}, say func([]app.Turn) error) error {
			return a.FollowMain(conversation, after, stop, say)
		})
	}
	if elsewhere {
		ask := []string{"api", "main", "turns", "--after", strconv.Itoa(after)}
		if conversation != "" {
			ask = append(ask, "--conversation", conversation)
		}
		out, err := a.OnMainMachine(machine, true, ask...)
		if err != nil {
			return who.fail(err)
		}
		fmt.Fprintln(who.out, out) // its turns, as its mia said them
		return exitOK
	}
	turns, err := a.MainTurns(conversation, after)
	if err != nil {
		return who.fail(err)
	}
	return who.emit(turns)
}
