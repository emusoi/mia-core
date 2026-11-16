package app

import (
	"slices"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/plugin"
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
