package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/run"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/store"
)

// An agent is started with a model and an effort, and switched to others
// while it runs, as its own table says. What mia keeps of a window is what
// it started the agent there with or last switched it to: not read from
// the agent, which can have been switched by hand since.

// AgentModels is an agent with what it can be started with.
type AgentModels struct {
	Name    string         `json:"name"`
	Models  []plugin.Model `json:"models"`
	Efforts []string       `json:"efforts"`
	// Model and Effort are the ones the config starts it with; "" for none.
	Model  string `json:"model"`
	Effort string `json:"effort"`
	// Live: it can be switched to another while it runs.
	Live bool `json:"live"`
	// Again: it takes a model only as it starts, and is switched by being
	// started again on the conversation it has open.
	Again bool `json:"again"`
	// AccountEnv is the variable that names the folder of each of its
	// accounts; "" for an agent mia keeps no accounts of.
	AccountEnv string `json:"account_env"`
	// Installed: its program is on this machine's PATH. One that is not can
	// be named in a config, and is not offered to start.
	Installed bool `json:"installed"`
}

// AgentModels is every agent mia knows, a plugin's too.
func (a *App) AgentModels() []AgentModels {
	all := []AgentModels{}
	for _, agent := range a.agents() {
		set := a.Config.Agents[agent.Name]
		all = append(all, AgentModels{Name: agent.Name, Models: modelsOf(agent, set), Efforts: effortsOf(agent, set),
			Model: set.Model, Effort: set.Effort, Live: agent.to != nil, Again: agent.again, AccountEnv: agent.AccountEnv, Installed: installed(agent)})
	}
	return all
}

// installed reports whether an agent's program is on PATH: its name, or the
// first word of one of its command lines.
func installed(agent Agent) bool {
	for _, name := range append([]string{agent.Name}, agent.Commands...) {
		if word, _, _ := strings.Cut(name, " "); word != "" {
			if _, err := exec.LookPath(word); err == nil {
				return true
			}
		}
	}
	return false
}

// modelsOf is the models an agent can be given: its own, those it says
// it has when it is one that is asked, and one the config names that it
// has not. None for one mia cannot start with one.
func modelsOf(agent Agent, set ...config.Agent) []plugin.Model {
	models := []plugin.Model{}
	if takes, _ := agent.takes(); !takes {
		return models
	}
	models = append(models, agent.Models...)
	if agent.lists != nil {
		models = append(models, agent.lists()...)
	}
	for _, one := range set {
		if one.Model != "" && !slices.ContainsFunc(models, func(m plugin.Model) bool { return m.ID == one.Model }) {
			models = append(models, plugin.Model{ID: one.Model, Label: one.Model})
		}
	}
	return models
}

func effortsOf(agent Agent, set ...config.Agent) []string {
	efforts := []string{}
	if _, takes := agent.takes(); !takes {
		return efforts
	}
	efforts = append(efforts, agent.Efforts...)
	for _, one := range set {
		if one.Effort != "" && !slices.Contains(efforts, one.Effort) {
			efforts = append(efforts, one.Effort)
		}
	}
	return efforts
}

// manyModels is how many models are more than a refusal names one by one.
const manyModels = 12

// allowed refuses a model or an effort the agent does not have, and says
// what it has.
func allowed(agent Agent, model, effort string, set ...config.Agent) error {
	if model != "" {
		models := modelsOf(agent, set...)
		if len(models) == 0 {
			return fmt.Errorf("mia knows no models of %s", agent.Name)
		}
		ids := make([]string, len(models))
		for i, m := range models {
			ids[i] = m.ID
		}
		if !slices.Contains(ids, model) && len(ids) > manyModels {
			return fmt.Errorf("%s has no model %s — `mia api agents` lists the %d it has", agent.Name, model, len(ids))
		}
		if !slices.Contains(ids, model) {
			return fmt.Errorf("%s has no model %s — it has %s", agent.Name, model, strings.Join(ids, ", "))
		}
	}
	if effort != "" {
		efforts := effortsOf(agent, set...)
		if len(efforts) == 0 {
			return fmt.Errorf("mia knows no efforts of %s", agent.Name)
		}
		if !slices.Contains(efforts, effort) {
			return fmt.Errorf("%s has no effort %s — it has %s", agent.Name, effort, strings.Join(efforts, ", "))
		}
	}
	return nil
}

// choose is the words that start an agent with a model and an effort,
// put after the one that runs it, and what mia keeps of them. One not
// given is the first set's that has one, unless the command line already
// says one.
func choose(agent Agent, command, model, effort string, set ...config.Agent) ([]string, Chosen, error) {
	if err := allowed(agent, model, effort, set...); err != nil {
		return nil, Chosen{}, err
	}
	if agent.with == nil {
		return nil, Chosen{}, nil
	}
	said := strings.Fields(command)
	// taken: the agent has words for it, and the command line does not say them
	taken := func(words []string) bool { return len(words) > 0 && !slices.Contains(said, words[0]) }
	for _, one := range set {
		if model == "" && one.Model != "" && taken(agent.with(one.Model, "")) {
			model = one.Model
		}
		if effort == "" && one.Effort != "" && taken(agent.with("", one.Effort)) {
			effort = one.Effort
		}
	}
	return agent.with(model, effort), Chosen{Model: model, Effort: effort}, nil
}

// AgentCommand is the word that runs an agent, by its name: the name
// itself for one mia does not know.
func (a *App) AgentCommand(name string) string {
	if agent, known := agentNamed(a.agents(), name); known {
		return agent.start()
	}
	return name
}

// AgentOf is the name of the agent a command line runs; "" for none mia
// knows.
func (a *App) AgentOf(command string) string {
	name, _ := agentOf(a.agents(), command)
	return name
}

// CommandWith is a command line with the agent it runs started with a
// model and an effort, those given or the configured ones, and on an
// account: the one given, or the one the repository starts the agent's
// sessions on. One that runs no agent is left as it is, and refused a
// model, an effort or an account.
func (a *App) CommandWith(command, model, effort, account string) (string, Chosen, error) {
	agent, with, chosen, err := a.withFor(command, model, effort, account)
	if err != nil || agent.Name == "" {
		return command, chosen, err
	}
	if len(with) > 0 {
		command = put(command, with)
	}
	return withAccount(agent, chosen.Account, command), chosen, nil
}

// put puts words into a command line after those that run its agent.
func put(command string, with []string) string {
	rest := strings.TrimLeft(command, " \t")
	var head []string
	for range agentAt(strings.Fields(rest)) {
		word, after, _ := strings.Cut(rest, " ")
		head, rest = append(head, word), strings.TrimLeft(after, " \t")
	}
	return strings.TrimSpace(strings.Join(head, " ") + " " + run.ShellJoin(with) + " " + rest)
}

// ArgvWith is CommandWith of a command given word by word: an account is
// set by env before it.
func (a *App) ArgvWith(argv []string, model, effort, account string) ([]string, Chosen, error) {
	agent, with, chosen, err := a.withFor(strings.Join(argv, " "), model, effort, account)
	if err != nil || agent.Name == "" {
		return argv, chosen, err
	}
	if len(with) > 0 {
		at := agentAt(argv)
		argv = slices.Concat(argv[:at], with, argv[at:])
	}
	if chosen.Account != "" {
		argv = slices.Concat([]string{"env", agent.AccountEnv + "=" + homeOf(agent, chosen.Account)}, argv)
	}
	return argv, chosen, nil
}

// withFor is the agent a command line runs, the words that start it with
// a model and an effort, and what is chosen of the three; no agent for a
// command line that runs none.
func (a *App) withFor(command, model, effort, account string) (Agent, []string, Chosen, error) {
	name, _ := agentOf(a.agents(), command)
	if name == "" {
		if model != "" || effort != "" || account != "" {
			return Agent{}, nil, Chosen{}, fmt.Errorf("`%s` runs no agent mia knows — a model and an account are an agent's", command)
		}
		return Agent{}, nil, Chosen{}, nil
	}
	agent, _ := agentNamed(a.agents(), name)
	with, chosen, err := choose(agent, command, model, effort, a.Config.Agents[name])
	if err != nil {
		return agent, nil, Chosen{}, err
	}
	if account == "" && agent.AccountEnv != "" && strings.Contains(command, agent.AccountEnv+"=") {
		return agent, with, chosen, nil // the command line says its own
	}
	chosen.Account, err = a.account(agent, account, a.placeAccount(agent))
	return agent, with, chosen, err
}

// agentAt is how many words come before what an agent is given: the one
// that runs it, behind env, node or bun.
func agentAt(words []string) int {
	return min(len(words), launcher(words)+1)
}

// Chosen is the model and effort mia started a window's agent with, or
// last switched it to, and the account it started it with; "" for one it
// did not choose.
type Chosen struct {
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	Account string `json:"account,omitempty"`
	// Fresh is when the main agent was started with a new conversation,
	// and zero when it went on with its last: only one begun since is its.
	Fresh time.Time `json:"fresh,omitzero"`
	// Resumed is the earlier conversation the main agent was started going
	// on with at Fresh: its, until another is begun.
	Resumed string `json:"resumed,omitempty"`
}

// chosenPath keeps them by window, the window as its session's name and
// the process its pane was started with: one that goes and another of the
// same name are told apart.
func (a *App) chosenPath() string { return filepath.Join(a.MiaDir, "chosen.json") }

func chosenKey(sessionName string, pid int) string { return sessionName + ":" + strconv.Itoa(pid) }

// chosen is what mia keeps of every window; nothing when it keeps none.
func (a *App) chosen() map[string]Chosen { return chosenIn(a.MiaDir) }

// chosenIn is what is kept of every window in a folder of mia's.
func chosenIn(dir string) map[string]Chosen {
	kept := map[string]Chosen{}
	if dir == "" {
		return kept
	}
	if data, err := os.ReadFile(filepath.Join(dir, "chosen.json")); err == nil {
		_ = json.Unmarshal(data, &kept) // unreadable is as nothing kept
	}
	return kept
}

// keep remembers what a window of a session was started with or switched
// to, and forgets the windows of that session there are no more.
func (a *App) keep(host session.Host, worktree string, pid int, chosen Chosen) error {
	if a.MiaDir == "" || pid == 0 {
		return nil
	}
	unlock, err := store.Lock(a.MiaDir, "chosen.lock")
	if err != nil {
		return err
	}
	defer unlock()
	kept, name := a.chosen(), session.Name(worktree)
	live := map[string]bool{}
	for _, pane := range host.Panes(worktree) {
		live[chosenKey(name, pane.PID)] = true
	}
	for key := range kept {
		if strings.HasPrefix(key, name+":") && !live[key] {
			delete(kept, key)
		}
	}
	kept[chosenKey(name, pid)] = chosen
	data, err := json.MarshalIndent(kept, "", "  ")
	if err != nil {
		return err
	}
	temp := fmt.Sprintf("%s.%d", a.chosenPath(), os.Getpid())
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, a.chosenPath())
}

// KeepChosen remembers what the agent in a worktree's new window was
// started with, and writes it down as a setup used there.
func (a *App) KeepChosen(record model.Record, window, agent string, chosen Chosen) error {
	host := a.sessionOf(record)
	for _, one := range host.WindowsOf(record.Name) {
		if one.Name == window {
			if err := a.keep(host, record.Name, one.PID, chosen); err != nil {
				return err
			}
			return a.used(agent, record.Name, machineOf(record), chosen)
		}
	}
	return nil
}

// SwitchWindow switches the agent in a worktree's window to another
// model, and effort when one is given, by typing what the agent takes for
// it, as the main agent is said something to; one that takes a model only
// as it starts is started again in its window, on its conversation.
func (a *App) SwitchWindow(record model.Record, window, model, effort string) (Chosen, error) {
	host, window, err := a.windowNamed(record, window)
	if err != nil {
		return Chosen{}, err
	}
	panes := host.Panes(record.Name)
	at := slices.IndexFunc(panes, func(p session.Pane) bool { return p.Window == window || strconv.Itoa(p.Index) == window })
	if at < 0 {
		return Chosen{}, fmt.Errorf("%s has no window called %s", record.Name, window)
	}
	pane := panes[at] // a window is its first pane
	command := pane.Command
	for _, one := range session.WindowsFrom(panes) {
		if one.Index == pane.Index {
			command = one.Command // claude's binary named for its version is claude
		}
	}
	var front func(int) string
	if host.SSH == "" {
		front = foregroundOf
	}
	name := agentBehind(a.agents(), command, pane.Title, pane.Kind, pane.PID, front)
	if name == "" {
		return Chosen{}, fmt.Errorf("window %s of %s runs no agent mia knows", window, record.Name)
	}
	agent, _ := agentNamed(a.agents(), name)
	was := a.chosen()[chosenKey(session.Name(record.Name), pane.PID)]
	pid := pane.PID
	var chosen Chosen
	if agent.to == nil && agent.again {
		chosen, pid, err = a.startAgain(host, record, pane, agent, model, effort, was)
	} else {
		chosen, err = switchTo(host, record.Name, window, agent, model, effort, was, a.Config.Agents[name])
	}
	if err != nil {
		return Chosen{}, err
	}
	if err := a.keep(host, record.Name, pid, chosen); err != nil {
		return chosen, err
	}
	return chosen, a.used(name, record.Name, machineOf(record), chosen)
}

// startAgain switches an agent that takes a model only as it starts: it
// is ended, and started in its pane again on the conversation it had
// open, with the model. The window stays, and what is kept on it; said
// are what it runs then, as far as mia knows, and the process its pane
// was started with.
func (a *App) startAgain(host session.Host, record model.Record, pane session.Pane, agent Agent, model, effort string, was Chosen) (Chosen, int, error) {
	if err := allowed(agent, model, effort, a.Config.Agents[agent.Name]); err != nil {
		return Chosen{}, 0, err
	}
	heads, _ := a.Conversations(Wanted{Worktree: record.Name, Agent: agent.Name})
	at := slices.IndexFunc(heads, func(h Head) bool { return h.Running && h.Window != nil && h.Window.Index == pane.Index })
	if at < 0 {
		return Chosen{}, 0, fmt.Errorf("%s in window %s of %s has no conversation yet to be started again on — close it, and start it with --model", agent.Name, pane.Window, record.Name)
	}
	command, err := againCommand(agent, heads[at].ID, heads[at].Cwd, record.Path, model, effort, was.Account)
	if err != nil {
		return Chosen{}, 0, err
	}
	argv, err := a.LaunchArgv(record, command)
	if err != nil {
		return Chosen{}, 0, err
	}
	if err := host.RestartPane(record.Name, pane.ID, record.Path, argv); err != nil {
		return Chosen{}, 0, err
	}
	pid := 0
	for _, one := range host.Panes(record.Name) {
		if one.ID == pane.ID {
			pid = one.PID
		}
	}
	was.Model = model
	if effort != "" {
		was.Effort = effort
	}
	return was, pid, nil
}

// againCommand is the command line that starts an agent again on a
// conversation with a model, and an effort when one is given, typed in
// the worktree's folder: on the account it was on, and in the folder the
// conversation was made in when that is another.
func againCommand(agent Agent, id, cwd, dir, model, effort, account string) (string, error) {
	command, err := agent.resume(id)
	if err != nil {
		return "", fmt.Errorf("agent %s: %w", agent.Name, err)
	}
	command = withAccount(agent, account, put(command, agent.with(model, effort)))
	if cwd != "" && cwd != dir {
		command = "cd " + shellQuote(cwd) + " && " + command
	}
	return command, nil
}

// typedApart is how long an agent is given to take one line typed into it
// before the next.
const typedApart = 500 * time.Millisecond

// switchTo types into a window what its agent takes to switch to a model,
// and an effort when one is given, and says what it is then as far as
// mia knows: was, with what was switched.
func switchTo(host session.Host, worktree, window string, agent Agent, model, effort string, was Chosen, set ...config.Agent) (Chosen, error) {
	if agent.to == nil {
		return Chosen{}, fmt.Errorf("%s cannot be switched to another model while it runs — restart it with --model", agent.Name)
	}
	if err := allowed(agent, model, effort, set...); err != nil {
		return Chosen{}, err
	}
	for i, line := range agent.to(model, effort) {
		if i > 0 {
			time.Sleep(typedApart)
		}
		if err := host.Paste(worktree, window, line); err != nil {
			return Chosen{}, err
		}
		if err := host.Keys(worktree, window, []string{"Enter"}); err != nil {
			return Chosen{}, err
		}
	}
	was.Model = model
	if effort != "" {
		was.Effort = effort
	}
	return was, nil
}
