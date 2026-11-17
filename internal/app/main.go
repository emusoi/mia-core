package app

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/store"
)

// The main agent is the one a person talks to, which runs the others: one
// for the person, on one machine, in a session of its own that is no
// worktree's. Its folder is mia's, under the user-wide config, and what it
// did is read from its own conversation.

// mainName is its session's, mia-main, held in the name registry so that
// no worktree is given it.
const mainName = "main"

// mainNotes is what its folder tells it: who it is, and what it may answer.
//
//go:embed main.md
var mainNotes string

// AtMain is mia at the main agent's folder, outside any repository: it
// knows the machines, the repositories and the main agent, and nothing one
// repository keeps.
func AtMain() (*App, error) {
	home := paths.In(mainName)
	if home == "" {
		return nil, errors.New("cannot find a home directory to keep the main agent in")
	}
	cfg, err := config.Load("")
	if err != nil {
		return nil, err
	}
	nameStore, err := globalNames()
	if err != nil {
		return nil, err
	}
	state := filepath.Join(home, ".mia")
	return &App{
		Root:     git.Resolve(home),
		MiaDir:   state,
		Config:   cfg,
		Store:    store.Store{Dir: state},
		Names:    nameStore,
		Runtimes: runtime.Store{Path: config.RuntimesPath()},
	}, nil
}

// MainState is the main agent as an app shows it.
type MainState struct {
	Running bool `json:"running"`
	// Agent is the one that runs, or the configured one when none does.
	Agent   string `json:"agent"`
	Machine string `json:"machine"`
	Home    string `json:"home"`
	Session string `json:"session"`
	// Window is the agent's in the session; left out when it is not running.
	Window *WindowRef `json:"window,omitempty"`
	// Conversation is the one it has open, or would go on with; "" when it
	// has had none, or is an agent mia cannot read the conversations of.
	Conversation string `json:"conversation"`
	// Working: it is doing something now, as its pane says.
	Working bool `json:"working"`
	// Asks is the question its window shows, waiting on the person: left
	// out when it asks nothing. It is not working while it asks.
	Asks *Asks `json:"asks,omitempty"`
	// Since is when its session was started.
	Since time.Time `json:"since,omitzero"`
	// Model and Effort are what mia started it with or last switched it to;
	// left out when it is not running, or mia did not choose them.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Account is the account of its agent it runs on, default for the
	// agent's own; left out when it is not running, or for an agent mia
	// keeps no accounts of.
	Account string `json:"account,omitempty"`
}

// mainHere, in the environment, makes mia answer for the main agent of the
// machine it runs on whatever the config says: what one machine sets when
// it asks another, so two that name each other do not ask in a circle.
const mainHere = "MIA_MAIN_HERE"

// MainElsewhere is the machine the config says the main agent lives on,
// when that is not this one: everything about the main agent is then asked
// of that machine's mia.
func (a *App) MainElsewhere() (runtime.Machine, bool, error) {
	name := a.Config.Main.Machine
	if name == "" || name == runtime.Local || os.Getenv(mainHere) != "" {
		return runtime.Machine{}, false, nil
	}
	machine, err := a.Runtimes.Get(name)
	if err != nil {
		return runtime.Machine{}, false, fmt.Errorf("[main] machine = %q: %w", name, err)
	}
	return machine, true, nil
}

// sshFailed is how ssh exits when it is ssh that failed, and not the
// command it ran.
const sshFailed = 255

// OnMainMachine runs mia with args on the machine the main agent lives on,
// as standing asks a machine, and gives what it printed. A machine that
// does not answer is an error that names it; polled, one that did not is
// not asked again for a while, as with standing.
func (a *App) OnMainMachine(machine runtime.Machine, polled bool, args ...string) (string, error) {
	const unanswered = "%s, the machine the main agent lives on, does not answer: %s"
	if last, off := a.unreachable(machine.Name); polled && off {
		return "", fmt.Errorf(unanswered, machine.Name, last.Detail)
	}
	out, err := askMia(machine, mainHere+"=1 ", args...)
	if err == nil {
		a.keepAnswer(machine.Name, Reach{State: ReachOK}, "")
		return out, nil
	}
	detail := strings.TrimPrefix(run.FirstLine(err.Error()), machine.SSH+": ")
	if run.ExitCode(err) != sshFailed {
		return "", fmt.Errorf("on %s: %s", machine.Name, strings.TrimPrefix(detail, "mia: ")) // its mia answered, and said why not
	}
	a.keepAnswer(machine.Name, Diagnose(machine, err, false), detail)
	return "", fmt.Errorf(unanswered, machine.Name, detail)
}

func (a *App) mainHost() session.Host {
	host := session.Here()
	host.Shell = a.Config.Dotfiles.Shell
	return host
}

// mainAgent is the agent the config names as the main one, from the table.
func (a *App) mainAgent() (Agent, error) {
	name := a.Config.Main.Agent
	if name == "" {
		name = "claude"
	}
	agent, known := agentNamed(a.agents(), name)
	if !known {
		return Agent{}, fmt.Errorf("[main] agent = %q, and mia knows no agent of that name — a plugin that adds it has to be enabled", name)
	}
	return agent, nil
}

// mainWindow is the window of its session an agent was started in: each is
// named for its agent, and goes when the agent ends.
func (a *App) mainWindow(host session.Host) (session.Window, session.Pane, bool) {
	panes := host.Panes(mainName)
	for _, window := range session.WindowsFrom(panes) {
		if _, known := agentNamed(a.agents(), window.Name); !known {
			continue
		}
		at := slices.IndexFunc(panes, func(p session.Pane) bool { return p.Index == window.Index })
		return window, panes[at], true
	}
	return session.Window{}, session.Pane{}, false
}

// workingFor is how lately a pane must have written for an agent that
// gives its pane no title to count as working.
const workingFor = 3 * time.Second

// Main is where the main agent stands. Nothing is started or written.
func (a *App) Main() MainState {
	state, _ := a.mainNow()
	return state
}

// mainNow is Main, and what the agent itself says it has open, when it says.
func (a *App) mainNow() (MainState, holds) {
	host := a.mainHost()
	state := MainState{Machine: runtime.Local, Home: a.Root, Session: session.Name(mainName)}
	agent, _ := a.mainAgent()
	state.Agent = agent.Name
	if state.Agent == "" {
		state.Agent = a.Config.Main.Agent
	}
	window, pane, running := a.mainWindow(host)
	var fresh time.Time
	var resumed string
	if running {
		agent, _ = agentNamed(a.agents(), window.Name)
		state.Running, state.Agent = true, agent.Name
		state.Window = &WindowRef{Index: window.Index, Name: window.Name}
		state.Since = host.Created(mainName).UTC()
		// claude says it in its pane's title; of another, writing is the sign
		state.Working = session.Spinning(pane.Title) ||
			(session.AgentTitle(pane.Title) == "" && !pane.Activity.IsZero() && time.Since(pane.Activity) < workingFor)
		if asks, on := askingIn(host, mainName, window.Name); on {
			state.Asks, state.Working = &asks, false
		}
		kept := a.chosen()[chosenKey(session.Name(mainName), pane.PID)]
		state.Model, state.Effort, fresh, resumed = kept.Model, kept.Effort, kept.Fresh, kept.Resumed
		if agent.AccountEnv != "" {
			state.Account = cmp.Or(kept.Account, DefaultAccount)
		}
	}
	var said holds
	if agent.talks != nil {
		said = agent.holding(pane.PID)
	}
	if state.Conversation = said.ID; said.ID == "" {
		state.Conversation = a.mainConversation(agent, state.Account, fresh, resumed)
	}
	return state, said
}

// mainConversation is the conversation of the agent's folder last written
// to, which is the one it has open when it does not say: of the account
// named, when one is. Started with a new conversation at fresh, it
// is one begun since, and until there is one none, or the one it was
// started going on with, resumed: not the one it left.
func (a *App) mainConversation(agent Agent, account string, fresh time.Time, resumed string) string {
	if agent.talks == nil {
		return ""
	}
	in, _ := a.where()
	heads, _ := agent.talks.heads(in)
	if account != "" {
		heads = slices.DeleteFunc(heads, func(h Head) bool { return h.Account != "" && h.Account != account })
	}
	if !fresh.IsZero() {
		heads = slices.DeleteFunc(heads, func(h Head) bool { return h.Started.Before(fresh.Truncate(time.Second)) })
	}
	if len(heads) == 0 {
		return resumed
	}
	return slices.MaxFunc(heads, func(x, y Head) int { return x.Active.Compare(y.Active) }).ID
}

// mainSet is what the main agent is started with when nothing else is
// said: [main]'s, and failing that its agent's own. Started as another
// agent than [main] names, only that agent's own.
func (a *App) mainSet(agent Agent) []config.Agent {
	set := []config.Agent{a.Config.Agents[agent.Name]}
	if configured, err := a.mainAgent(); err == nil && configured.Name == agent.Name {
		set = slices.Insert(set, 0, config.Agent{Model: a.Config.Main.Model, Effort: a.Config.Main.Effort})
	}
	return set
}

// mainAccount is the account the main agent is started on when none is
// given: [main]'s, for the agent [main] names, and failing that its
// agent's own in [agents]; "" for the default.
func (a *App) mainAccount(agent Agent, account string) (string, error) {
	set := []string{a.Config.Agents[agent.Name].Account}
	if configured, err := a.mainAgent(); err == nil && configured.Name == agent.Name {
		set = slices.Insert(set, 0, a.Config.Main.Account)
	}
	return a.account(agent, account, set...)
}

// StartMain makes the main agent's folder and session when they are not
// there and starts the agent in it, going on with its last conversation
// on the account it starts on when it has one, with a model and an
// effort: those given, or the configured ones. One that runs already is
// left as it is. Named, the agent is that one of the table for this
// start, whatever [main] says.
func (a *App) StartMain(name, model, effort, account string) (MainState, error) {
	return a.startMain(name, model, effort, account, false, "", false)
}

// StartMainNew starts the main agent with a new conversation: it goes on
// with none, and the one it had stays where its agent keeps them. One
// that runs is refused, in a line that says what would be cut short,
// unless restart says to end it: it is then stopped and started as the
// agent it was, with the model, the effort and the account it had, where
// none is given. Another agent named takes none of them.
func (a *App) StartMainNew(name, model, effort, account string, restart bool) (MainState, error) {
	return a.startMain(name, model, effort, account, true, "", restart)
}

// StartMainResuming starts the main agent going on with one of its
// earlier conversations, as the agent that had it and on the account it
// is kept on: one of those MainConversations lists, and no other. One
// that runs is refused as for a new conversation, unless restart says to
// end it; the conversation it had stays where its agent keeps them.
func (a *App) StartMainResuming(id, name, model, effort, account string, restart bool) (MainState, error) {
	return a.startMain(name, model, effort, account, false, id, restart)
}

// endRefused is why a main agent that runs is not given another
// conversation unasked: what ending it would cut short. flag is what was
// asked for, and then what follows.
func endRefused(state MainState, flag, then string) error {
	how := "`mia main start " + flag + " --restart` %s its conversation and " + then
	switch {
	case state.Asks != nil:
		return fmt.Errorf("the main agent asks something: %s — "+how, state.Asks.Text, "leaves it unanswered, ends")
	case state.Working:
		return fmt.Errorf("the main agent is working — "+how, "stops it mid-turn, ends")
	}
	return fmt.Errorf("the main agent is running — "+how, "ends")
}

// noMainConversation is said of an id that is none of the main agent's.
const noMainConversation = "no conversation %s of the main agent's — `mia api main conversations` lists them"

// MainConversations is the conversations made in the main agent's folder,
// with every agent it has been run as, the last written to first, and the
// one it has open now marked as running. One it says it has and has
// written nothing of yet is listed too, first, with nothing asked or said.
func (a *App) MainConversations() ([]Head, []string) {
	heads, problems := a.Conversations(Wanted{})
	if state, said := a.mainNow(); state.Running && state.Conversation != "" {
		if !slices.ContainsFunc(heads, func(h Head) bool { return h.ID == state.Conversation && h.Agent == state.Agent }) && said.ID != "" {
			worktree, _ := a.worktreeOf(a.Root, nil)
			heads = slices.Insert(heads, 0, unsaid(state.Agent, said, state.Account, a.Root, worktree))
		}
		for i := range heads {
			if heads[i].ID == state.Conversation && heads[i].Agent == state.Agent {
				heads[i].Running, heads[i].Window = true, state.Window
				heads[i].Model, heads[i].Effort = state.Model, state.Effort
			}
		}
	}
	return heads, problems
}

// resumeOf is how the main agent goes on with one of its earlier
// conversations: the agent that had it, the account it is kept on, and
// the command. Named, the agent has to be the one that had it.
func (a *App) resumeOf(id, name, account string) (agent Agent, on, command string, err error) {
	heads, problems := a.Conversations(Wanted{})
	at := slices.IndexFunc(heads, func(h Head) bool { return h.ID == id })
	if at < 0 {
		if len(problems) > 0 {
			return Agent{}, "", "", fmt.Errorf("no conversation %s of the main agent's — %s", id, strings.Join(problems, "; "))
		}
		return Agent{}, "", "", fmt.Errorf(noMainConversation, id)
	}
	head := heads[at] // of one kept on two accounts, the last written to
	if name != "" && name != head.Agent {
		return Agent{}, "", "", fmt.Errorf("conversation %s is %s's, not %s's — it is gone on with as %s", id, head.Agent, name, head.Agent)
	}
	if account != "" && head.Account != "" && account != head.Account {
		return Agent{}, "", "", fmt.Errorf("conversation %s is kept on %s's account %s, not %s — it is gone on with there", id, head.Agent, head.Account, account)
	}
	agent, _ = agentNamed(a.agents(), head.Agent)
	if agent.resume == nil {
		return Agent{}, "", "", fmt.Errorf("mia does not know how %s takes a conversation up again by its id", head.Agent)
	}
	if command, err = agent.resume(id); err != nil {
		return Agent{}, "", "", fmt.Errorf("agent %s: %w", agent.Name, err)
	}
	return agent, cmp.Or(account, head.Account), command, nil
}

func (a *App) startMain(name, model, effort, account string, fresh bool, resume string, restart bool) (MainState, error) {
	host := a.mainHost()
	restarting := false
	flag, then, resumed := "--new", "starts a fresh one", ""
	if resume != "" {
		taken, on, command, err := a.resumeOf(resume, name, account)
		if err != nil {
			return MainState{}, err
		}
		name, account, resumed = taken.Name, on, command
		flag, then = "--resume "+resume, "goes on with that one"
	}
	if window, pane, running := a.mainWindow(host); (fresh || resume != "") && running {
		if !restart {
			return MainState{}, endRefused(a.Main(), flag, then)
		}
		restarting = true
		if name == "" || name == window.Name {
			was := a.chosen()[chosenKey(session.Name(mainName), pane.PID)]
			name, model, effort, account = window.Name, cmp.Or(model, was.Model), cmp.Or(effort, was.Effort), cmp.Or(account, was.Account)
		}
	}
	agent, err := a.mainAgent()
	if name != "" {
		var known bool
		agent, known = agentNamed(a.agents(), name)
		if err = nil; !known {
			err = fmt.Errorf("mia knows no agent %s — `mia api agents` lists those it does", name)
		}
	}
	if err != nil {
		return MainState{}, err
	}
	if err := allowed(agent, model, effort, a.mainSet(agent)...); err != nil {
		return MainState{}, err
	}
	given := account
	if account, err = a.mainAccount(agent, account); err != nil {
		return MainState{}, err
	}
	if !host.Available() {
		return MainState{}, errors.New("tmux not found — the main agent runs in a session: https://github.com/tmux/tmux/wiki/Installing")
	}
	if window, _, running := a.mainWindow(host); running && name != "" && resume == "" && !restarting {
		return MainState{}, fmt.Errorf("the main agent runs already, as %s — `mia main stop` first", window.Name)
	}
	home := paths.In(mainName)
	if err := os.MkdirAll(home, 0o755); err != nil {
		return MainState{}, err
	}
	a.Root = git.Resolve(home) // there now, so as an agent's transcript says it
	if err := a.Names.Claim(mainName, a.Root); err != nil {
		return MainState{}, fmt.Errorf("the main agent's session is mia-%s: %w", mainName, err)
	}
	notes := filepath.Join(a.Root, agent.notesFile())
	if was, _ := os.ReadFile(notes); string(was) != mainNotes {
		if err := os.WriteFile(notes, []byte(mainNotes), 0o644); err != nil {
			return MainState{}, err
		}
	}
	if _, _, running := a.mainWindow(host); running && restarting {
		// asked for in so many words, and everything it is started with is known to be good
		if err := host.Kill(mainName); err != nil {
			return MainState{}, err
		}
	} else if running {
		if model != "" || effort != "" {
			return MainState{}, errors.New("the main agent runs already — `mia main model <model>` switches it")
		}
		if given != "" {
			return MainState{}, errors.New("the main agent runs already, and takes its account when it starts — `mia main stop` first")
		}
		return a.Main(), nil
	}
	on := ""
	if agent.AccountEnv != "" {
		on = cmp.Or(account, DefaultAccount)
	}
	command := agent.start()
	if resumed != "" {
		command = resumed
	} else if last := a.mainConversation(agent, on, time.Time{}, ""); !fresh && last != "" && agent.resume != nil {
		if command, err = agent.resume(last); err != nil {
			return MainState{}, fmt.Errorf("agent %s: %w", agent.Name, err)
		}
	}
	with, chosen, err := choose(agent, command, model, effort, a.mainSet(agent)...)
	if err != nil {
		return MainState{}, err
	}
	if len(with) > 0 {
		command = put(command, with)
	}
	chosen.Account = account
	kept := chosen
	if fresh || resume != "" {
		// the one it is told to go on with is its, until another is begun
		kept.Fresh, kept.Resumed = time.Now(), resume
	}
	command = withAccount(agent, account, command)
	if err := host.Ensure(mainName, a.Root); err != nil {
		return MainState{}, err
	}
	shell := a.Config.Dotfiles.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
	}
	if shell == "" {
		shell = "/bin/sh"
	}
	// by a login shell, for its PATH, and with nothing after: the window is
	// the agent, and goes when it ends. It is told it is the main agent's by
	// MIA_MAIN, and has no MIA_WORKTREE: it is no worktree's.
	if err := host.StartWindowWith("MIA_MAIN=1", mainName, a.Root, agent.Name, session.What{Kind: session.KindAgent, Label: agent.Name}, []string{shell, "-lc", command}); err != nil {
		return MainState{}, err
	}
	said, ended := settle(host, mainName, agent.Name)
	_, pane, running := a.mainWindow(host)
	if ended || !running {
		if said != "" {
			return MainState{}, fmt.Errorf("%s ended as soon as it started, saying:\n%s", agent.Name, said)
		}
		return MainState{}, fmt.Errorf("%s ended as soon as it started — `%s` in %s shows why", agent.Name, command, a.Root)
	}
	if kept != (Chosen{}) {
		if err := a.keep(host, mainName, pane.PID, kept); err != nil {
			return MainState{}, err
		}
		if err := a.used(agent.Name, mainName, runtime.Local, chosen); err != nil {
			return MainState{}, err
		}
	}
	return a.Main(), nil
}

// StopMain ends the main agent's session. Its folder and conversations stay.
func (a *App) StopMain() error {
	return a.mainHost().Kill(mainName)
}

var errMainDown = errors.New("the main agent is not running — `mia main start`")

// SayToMain types text into the main agent as one message and submits it:
// pasted, so a line break in it is not an Enter, and Enter after. While it
// asks something nothing is typed: the Enter would answer it.
func (a *App) SayToMain(text string) error {
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return errMainDown
	}
	return a.mainGone(host, say(host, mainName, window.Name, window.Name, "mia main press", text))
}

// say types text into an agent's window as one message and submits it:
// pasted, so a line break in it is not an Enter, and Enter after. While it
// asks something nothing is typed: the Enter would answer it, and press is
// how it is answered.
func say(host session.Host, worktree, window, agent, press, text string) error {
	if asks, on := askingIn(host, worktree, window); on {
		return askedFirst(agent, asks, press)
	}
	if err := host.Paste(worktree, window, text); err != nil {
		return err
	}
	return host.Keys(worktree, window, []string{"Enter"})
}

// mainGone is err, or errMainDown when the agent ended as it was typed
// into: a window that is gone is not to be pasted into.
func (a *App) mainGone(host session.Host, err error) error {
	if _, _, running := a.mainWindow(host); err != nil && !running {
		return errMainDown
	}
	return err
}

// PressInMain presses keys in the main agent's window with no Enter after
// them and not as a paste, as PressInWindow does in a worktree's: a digit
// that picks an answer when it is the main agent that asks, or moves its
// cursor there and presses Enter when its options have no numbers.
func (a *App) PressInMain(keys string) error {
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return errMainDown
	}
	return pressAnswer(host, mainName, window.Name, keys)
}

// SwitchMain switches the main agent to another model, and effort when
// one is given, as SwitchWindow does a worktree's agent.
func (a *App) SwitchMain(model, effort string) (Chosen, error) {
	host := a.mainHost()
	window, pane, running := a.mainWindow(host)
	if !running {
		return Chosen{}, errMainDown
	}
	if asks, on := askingIn(host, mainName, window.Name); on {
		return Chosen{}, askedFirst(window.Name, asks, "mia main press")
	}
	agent, _ := agentNamed(a.agents(), window.Name)
	was := a.chosen()[chosenKey(session.Name(mainName), pane.PID)]
	chosen, err := switchTo(host, mainName, window.Name, agent, model, effort, was, a.mainSet(agent)...)
	if err != nil {
		return Chosen{}, a.mainGone(host, err)
	}
	if err := a.keep(host, mainName, pane.PID, chosen); err != nil {
		return chosen, err
	}
	return chosen, a.used(agent.Name, mainName, runtime.Local, chosen)
}

// InterruptMain presses the key that stops the main agent mid-work.
func (a *App) InterruptMain() error {
	host := a.mainHost()
	window, _, running := a.mainWindow(host)
	if !running {
		return errMainDown
	}
	agent, _ := agentNamed(a.agents(), window.Name)
	return host.Keys(mainName, window.Name, []string{agent.stopKey()})
}

// MainTurns is one of the main agent's conversations as turns, those
// after turn number after, with each of mia's verbs it ran that changed
// something said as what it did: the one named, which has to be one made
// in its folder, or with no id the one it has open.
func (a *App) MainTurns(id string, after int) ([]Turn, error) {
	if id == "" {
		if id = a.Main().Conversation; id == "" {
			return []Turn{}, nil
		}
	}
	in, _ := a.where()
	turns, found, err := a.turnsIn(in, id, "", "", after)
	if err == nil && !found {
		if state, said := a.mainNow(); state.Running && said.ID == id {
			return []Turn{}, nil // it has it open, and nothing is written of it yet
		}
		err = fmt.Errorf(noMainConversation, id)
	}
	if err != nil {
		return nil, err
	}
	places := a.places()
	for i := range turns {
		turns[i] = a.did(turns[i], places)
	}
	return turns, nil
}
