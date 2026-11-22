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
	chatsUsage     = "mia chats [list|new --agent <agent> [--model <model>] [--effort <effort>] [-- <first words…>]|say <name> -- <text…>|press <name> <keys…>|interrupt <name>|stop <name>|start <name>|model <name> <model> [--effort <effort>]|switch <name> --agent <agent> [--model <model>] [--effort <effort>]|remove <name>]"
	chatQueryUsage = "mia api chat <name>"
	chatTurnsUsage = "mia api chat turns <name> [--after <n>] [--follow]"
)

// atChat is mia at a chat, as atMain is at the main agent.
func atChat(name string) (*app.App, error) {
	a, err := app.AtChat(name)
	if err != nil {
		return nil, err
	}
	a.Plugins = pluginHost(a.Config)
	return a, nil
}

// cmdChats is mia chats: plain conversations of the person's with any
// agent, each a main agent of its own, the main agent among them as the
// chat named main. Its arguments come as they were typed, so that what is
// said is not read for flags.
func cmdChats(args []string) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "-h" || sub == "--help" {
		verbUsage(os.Stdout, "chats")
		return exitOK
	}
	rest := args[min(1, len(args)):]
	// what follows -- is said as it is typed, and read for no flag
	flags, words := rest, ""
	if at := slices.Index(rest, "--"); at >= 0 {
		flags, words = rest[:at], strings.TrimSpace(strings.Join(rest[at+1:], " "))
	}
	flags = slices.Clone(flags)
	var name, agent, model, effort string
	switch sub {
	case "new":
		agent, model, effort = takeValue(&flags, "--agent"), takeValue(&flags, "--model"), takeValue(&flags, "--effort")
		if agent == "" || len(flags) > 0 {
			return usageErr("mia chats new --agent <agent> [--model <model>] [--effort <effort>] [-- <first words…>]")
		}
	case "say", "press":
		// said or pressed: all after the name, -- or not
		if len(rest) < 2 {
			return usageErr(map[string]string{"say": "mia chats say <name> -- <text…>", "press": "mia chats press <name> <keys…>"}[sub])
		}
		name, words = rest[0], strings.Join(rest[1:], " ")
		if rest[1] == "--" {
			words = strings.Join(rest[2:], " ")
		}
		if words = strings.TrimSpace(words); words == "" {
			return usageErr(map[string]string{"say": "mia chats say <name> -- <text…>", "press": "mia chats press <name> <keys…>"}[sub])
		}
	case "model":
		effort = takeValue(&flags, "--effort")
		if len(flags) != 2 || words != "" || strings.HasPrefix(flags[1], "-") {
			return usageErr("mia chats model <name> <model> [--effort <effort>]")
		}
		name, model = flags[0], flags[1]
	case "switch":
		agent, model, effort = takeValue(&flags, "--agent"), takeValue(&flags, "--model"), takeValue(&flags, "--effort")
		if agent == "" || len(flags) != 1 || words != "" {
			return usageErr("mia chats switch <name> --agent <agent> [--model <model>] [--effort <effort>]")
		}
		name = flags[0]
	case "interrupt", "stop", "start", "remove":
		if len(flags) != 1 || words != "" {
			return usageErr("mia chats " + sub + " <name>")
		}
		name = flags[0]
	case "list", "ls":
		if len(rest) > 0 {
			return usageErr(chatsUsage)
		}
	default:
		return usageErr(chatsUsage)
	}
	a, err := atMain()
	if err != nil {
		return fail(err)
	}
	if machine, elsewhere, err := a.MainElsewhere(); err != nil {
		return fail(err)
	} else if elsewhere {
		// they live where the main agent does: that machine's mia does it, and says it
		out, err := a.OnMainMachine(machine, false, append([]string{"chats"}, args...)...)
		if err != nil {
			return fail(err)
		}
		if out != "" {
			fmt.Println(out)
		}
		return exitOK
	}
	switch sub {
	case "list", "ls":
		for _, chat := range a.Chats() {
			fmt.Println(chatLine(chat))
		}
		return exitOK
	case "new":
		c, err := a.NewChat(agent, model, effort, words)
		if err != nil {
			return fail(err)
		}
		made := c.Main().Name
		fmt.Println(made) // the one line it prints: what follows is said beside it
		state, err := c.StartMain("", "", "", "")
		if err != nil {
			return fail(fmt.Errorf("chat %s is made, and not started: %w", made, err))
		}
		if sent, err := c.SayWaiting(); err != nil {
			return fail(fmt.Errorf("chat %s is started, and its first words were not said: %w", made, err))
		} else if !sent && state.Asks != nil {
			fmt.Fprintf(os.Stderr, "mia: %s — its first words are said once it is answered\n", state.Asks.Said(state.Agent, "mia chats press "+made))
		}
		return exitOK
	case "remove":
		if err := a.RemoveChat(name); err != nil {
			return fail(err)
		}
		fmt.Printf("chat %s is removed; its conversations stay with its agent\n", name)
		return exitOK
	}
	c, err := atChat(name)
	if err != nil {
		return fail(err)
	}
	switch sub {
	case "start":
		state, err := c.StartMain("", "", "", "")
		if err != nil {
			return fail(err)
		}
		return sayWaiting(c, state)
	case "stop":
		if err := c.StopMain(); err != nil {
			return fail(err)
		}
		fmt.Printf("chat %s is stopped; its folder and conversations stay\n", name)
		return exitOK
	case "say":
		return exitFor(c.SayToMain(words))
	case "press":
		return exitFor(c.PressInMain(words))
	case "switch":
		return saySwitched(c.SwitchChat(agent, model, effort))
	case "model":
		chosen, err := c.SwitchMain(model, effort)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("chat %s is switched to %s\n", name, said(chosen))
		return exitOK
	}
	return exitFor(c.InterruptMain())
}

// sayWaiting says a chat's first words once it has started, when it has
// them, and where it stands: they wait while it asks something first.
func sayWaiting(c *app.App, state app.MainState) int {
	sent, err := c.SayWaiting()
	if err != nil {
		return fail(err)
	}
	if sent {
		state = c.Main()
	}
	fmt.Println(chatLine(app.ChatState{Name: state.Name, Agent: state.Agent, Running: state.Running, Working: state.Working, Asks: state.Asks != nil}))
	if state.Asks != nil {
		fmt.Println(state.Asks.Said(state.Agent, "mia chats press "+state.Name))
	}
	return exitOK
}

// chatLine is a chat on one line: its name, its agent, what it is doing
// and its first words.
func chatLine(chat app.ChatState) string {
	doing := "stopped"
	switch {
	case chat.Asks:
		doing = "asks"
	case chat.Working:
		doing = "working"
	case chat.Running:
		doing = "idle"
	}
	return strings.TrimSpace(fmt.Sprintf("%-10s %-9s %-8s %s", chat.Name, chat.Agent, doing, chat.Title))
}

// cmdAPIChats is mia api chats, every chat and the main agent first, and
// mia api chat, one of them: where it stands, or its turns.
func cmdAPIChats(who asker, args []string) int {
	query, rest := args[0], args[1:]
	turns := query == "chat" && len(rest) > 0 && rest[0] == "turns"
	if turns {
		rest = rest[1:]
	}
	follow := turns && take(&rest, "--follow")
	after := 0
	if value := takeValue(&rest, "--after"); value != "" {
		if !turns {
			return who.usage(map[string]string{"chats": "mia api chats", "chat": chatQueryUsage}[query])
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return who.usage(chatTurnsUsage + followNote)
		}
		after = n
	}
	switch {
	case query == "chats" && len(rest) > 0:
		return who.usage("mia api chats")
	case query == "chat" && len(rest) != 1 && turns:
		return who.usage(chatTurnsUsage + followNote)
	case query == "chat" && len(rest) != 1:
		return who.usage(chatQueryUsage + "\n       " + chatTurnsUsage)
	}
	a, err := atMain()
	if err != nil {
		return who.fail(err)
	}
	machine, elsewhere, err := a.MainElsewhere()
	if err != nil {
		return who.fail(err)
	}
	if elsewhere {
		if follow {
			return who.fail(fmt.Errorf("--follow reads a chat on this machine, and they live on %s — follow it with mia there", machine.Name))
		}
		ask := append([]string{"api"}, args...)
		out, err := a.OnMainMachine(machine, true, ask...)
		switch {
		case err != nil:
			return who.fail(err)
		case query == "chat" && !turns:
			return sayMainOf(who, machine.Name, out)
		case query == "chats":
			var chats []app.ChatState
			if err := json.Unmarshal([]byte(out), &chats); err != nil {
				return who.fail(fmt.Errorf("%s's mia did not say its chats: %w", machine.Name, err))
			}
			for i := range chats {
				chats[i].Machine = machine.Name
			}
			return who.emit(chats)
		}
		fmt.Fprintln(who.out, out) // its turns, as its mia said them
		return exitOK
	}
	if query == "chats" {
		return who.emit(a.Chats())
	}
	c, err := atChat(rest[0])
	if err != nil {
		return who.fail(err)
	}
	switch {
	case follow:
		return who.follow(func(stop <-chan struct{}, say func([]app.Turn) error) error {
			return c.FollowMain("", after, stop, say)
		})
	case turns:
		turns, err := c.MainTurns("", after)
		if err != nil {
			return who.fail(err)
		}
		return who.emit(turns)
	}
	return who.emit(c.Main())
}
