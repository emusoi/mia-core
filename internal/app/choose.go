package app

import (
	"encoding/json"
	"fmt"
	"os"
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
}

// AgentModels is every agent mia knows, a plugin's too.
func (a *App) AgentModels() []AgentModels {
	all := []AgentModels{}
	for _, agent := range a.agents() {
		set := a.Config.Agents[agent.Name]
		all = append(all, AgentModels{Name: agent.Name, Models: modelsOf(agent, set), Efforts: effortsOf(agent, set),
			Model: set.Model, Effort: set.Effort, Live: agent.to != nil})
	}
	return all
}

// modelsOf is the models an agent can be given: its own, and one the
// config names that it has not. None for one mia cannot start with one.
func modelsOf(agent Agent, set ...config.Agent) []plugin.Model {
	models := []plugin.Model{}
	if agent.with == nil {
		return models
	}
	models = append(models, agent.Models...)
	for _, one := range set {
		if one.Model != "" && !slices.ContainsFunc(models, func(m plugin.Model) bool { return m.ID == one.Model }) {
			models = append(models, plugin.Model{ID: one.Model, Label: one.Model})
		}
	}
	return models
}

func effortsOf(agent Agent, set ...config.Agent) []string {
	efforts := []string{}
	if agent.with == nil {
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
	says := func(words []string) bool { return len(words) > 0 && slices.Contains(said, words[0]) }
	for _, one := range set {
		if model == "" && one.Model != "" && !says(agent.with(one.Model, "")) {
			model = one.Model
		}
		if effort == "" && one.Effort != "" && !says(agent.with("", one.Effort)) {
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

// CommandWith is a command line with the agent it runs started with a
// model and an effort, those given or the configured ones. One that runs
// no agent is left as it is, and refused a model or an effort.
func (a *App) CommandWith(command, model, effort string) (string, Chosen, error) {
	with, chosen, err := a.withFor(command, model, effort)
	if err != nil || len(with) == 0 {
		return command, chosen, err
	}
	return put(command, with), chosen, nil
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

// ArgvWith is CommandWith of a command given word by word.
func (a *App) ArgvWith(argv []string, model, effort string) ([]string, Chosen, error) {
	with, chosen, err := a.withFor(strings.Join(argv, " "), model, effort)
	if err != nil || len(with) == 0 {
		return argv, chosen, err
	}
	at := agentAt(argv)
	return slices.Concat(argv[:at], with, argv[at:]), chosen, nil
}

func (a *App) withFor(command, model, effort string) ([]string, Chosen, error) {
	name, _ := agentOf(a.agents(), command)
	if name == "" {
		if model != "" || effort != "" {
			return nil, Chosen{}, fmt.Errorf("`%s` runs no agent mia knows — a model is an agent's", command)
		}
		return nil, Chosen{}, nil
	}
	agent, _ := agentNamed(a.agents(), name)
	return choose(agent, command, model, effort, a.Config.Agents[name])
}

// agentAt is how many words come before what an agent is given: the one
// that runs it, behind node or bun when ps shows it so.
func agentAt(words []string) int {
	if len(words) > 1 && (filepath.Base(words[0]) == "node" || filepath.Base(words[0]) == "bun") {
		return 2
	}
	return min(len(words), 1)
}

// Chosen is the model and effort mia started a window's agent with, or
// last switched it to; "" for one it did not choose.
type Chosen struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// chosenPath keeps them by window, the window as its session's name and
// the process its pane was started with: one that goes and another of the
// same name are told apart.
func (a *App) chosenPath() string { return filepath.Join(a.MiaDir, "chosen.json") }

func chosenKey(sessionName string, pid int) string { return sessionName + ":" + strconv.Itoa(pid) }

// chosen is what mia keeps of every window; nothing when it keeps none.
func (a *App) chosen() map[string]Chosen {
	kept := map[string]Chosen{}
	if a.MiaDir == "" {
		return kept
	}
	if data, err := os.ReadFile(a.chosenPath()); err == nil {
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
// started with.
func (a *App) KeepChosen(record model.Record, window string, chosen Chosen) error {
	host := a.sessionOf(record)
	for _, one := range host.WindowsOf(record.Name) {
		if one.Name == window {
			return a.keep(host, record.Name, one.PID, chosen)
		}
	}
	return nil
}

// SwitchWindow switches the agent in a worktree's window to another
// model, and effort when one is given, by typing what the agent takes for
// it, as the main agent is said something to.
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
	if (command == "node" || command == "bun") && host.SSH == "" {
		command = foreground()[pane.PID]
	}
	name := agentIn(a.agents(), command, pane.Title)
	if name == "" {
		return Chosen{}, fmt.Errorf("window %s of %s runs no agent mia knows", window, record.Name)
	}
	agent, _ := agentNamed(a.agents(), name)
	was := a.chosen()[chosenKey(session.Name(record.Name), pane.PID)]
	chosen, err := switchTo(host, record.Name, window, agent, model, effort, was, a.Config.Agents[name])
	if err != nil {
		return Chosen{}, err
	}
	return chosen, a.keep(host, record.Name, pane.PID, chosen)
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
