package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/plugin"
)

// AgentTimeout is how long a plugin has to answer about its agent.
var AgentTimeout = 5 * time.Second

// pluginAgents is the agent of each enabled plugin that declares one. What
// mia has built in for claude, codex, pi, opencode and agy it asks such a plugin for:
//
//	mia-<name> agent conversations <main checkout> <worktree>…   a JSON list: id, cwd, asked, said, started, active, turns
//	mia-<name> agent turns <id> [--after N]                      a turn a line, as JSON, in mia's own shape
//	mia-<name> agent search <text> <main checkout> <worktree>…   a JSON list: id, cwd, at, yours, line, n
//	mia-<name> agent resume <id>                                 the command line that takes it up again
//	mia-<name> agent carry <from> <to> pack|command [id]         as carriers do
func (a *App) pluginAgents() []Agent {
	enabled, _ := a.Plugins.Enabled()
	var found []Agent
	for _, p := range enabled {
		declared := p.Manifest.Agent
		if declared == nil {
			continue
		}
		name := declared.Name
		if name == "" {
			name = p.Name
		}
		its := pluginTalks{plugin: p, agent: name, context: plugin.Context{Repo: a.Root, MiaDir: a.MiaDir, Settings: a.Config.Plugin[p.Name]}}
		agent := Agent{Name: name, Commands: declared.Commands, talks: its,
			resume: func(id string) (string, error) {
				out, err := its.ask("resume", id)
				line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
				if err == nil && line == "" {
					err = fmt.Errorf("resume said nothing")
				}
				return line, err
			},
			// wherever the script runs, the plugin there answers; none there carries nothing
			carrier: `p=$(command -v ` + shellQuote(plugin.Prefix+p.Name) + ` || echo "${XDG_CONFIG_HOME:-$HOME/.config}/mia/plugins/"` + shellQuote(plugin.Prefix+p.Name) + `)
[ -x "$p" ] || exit 0
exec "$p" agent carry "$@"`,
		}
		agent.Models, agent.Efforts, agent.AccountEnv = declared.Models, declared.Efforts, declared.AccountEnv
		if len(declared.WithModel) > 0 || len(declared.WithEffort) > 0 {
			agent.with = words(declared.WithModel, declared.WithEffort)
		}
		if declared.SwitchModel != "" {
			agent.to = typing(declared.SwitchModel, declared.SwitchEffort)
		}
		if declared.Title != "" {
			agent.Title, _ = regexp.Compile(declared.Title) // the manifest was read only if it compiles
		}
		found = append(found, agent)
	}
	return found
}

// pluginTalks is the conversations of an agent a plugin adds: each answer is the
// plugin's.
type pluginTalks struct {
	plugin  plugin.Plugin
	agent   string
	context plugin.Context
}

func (p pluginTalks) ask(args ...string) ([]byte, error) {
	return p.plugin.Ask(p.context, AgentTimeout, append([]string{"agent"}, args...)...)
}

func (p pluginTalks) heads(in folders) ([]Head, error) {
	out, err := p.ask(append([]string{"conversations", in.Root}, in.Paths...)...)
	if err != nil {
		return nil, err
	}
	var listed []struct {
		ID      string    `json:"id"`
		Cwd     string    `json:"cwd"`
		Asked   string    `json:"asked"`
		Said    string    `json:"said"`
		Started time.Time `json:"started"`
		Active  time.Time `json:"active"`
		Turns   int       `json:"turns"`
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		return nil, fmt.Errorf("conversations is not a JSON list: %w", err)
	}
	var heads []Head
	for _, l := range listed {
		if l.ID != "" && in.holds(l.Cwd) {
			heads = append(heads, Head{ID: l.ID, Agent: p.agent, Cwd: l.Cwd, Asked: oneLine(l.Asked), Said: oneLine(l.Said),
				Started: l.Started, Active: l.Active, Turns: l.Turns})
		}
	}
	return heads, nil
}

// A plugin is asked for a conversation by its id alone: which copy of one
// it gives is its own to say.
func (p pluginTalks) turns(in folders, id, _ string, after int) ([]Turn, bool, error) {
	heads, err := p.heads(in)
	if err != nil {
		return nil, false, err
	}
	found := false
	for _, h := range heads {
		found = found || h.ID == id
	}
	if !found {
		return nil, false, nil
	}
	args := []string{"turns", id}
	if after > 0 {
		args = append(args, "--after", strconv.Itoa(after))
	}
	out, err := p.ask(args...)
	if err != nil {
		return nil, true, err
	}
	var turns []Turn
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var turn Turn
		if err := json.Unmarshal(line, &turn); err != nil || turn.Kind == "" {
			return nil, true, fmt.Errorf("turns: a line is not a turn: %.60s", line)
		}
		if turn.N > after {
			turns = append(turns, turn)
		}
	}
	return turns, true, nil
}

// A plugin is asked for all of it again each time: what it says of a turn
// may grow under its n.
func (p pluginTalks) follow(in folders, id, cwd string, after int) (func() ([]Turn, bool, error), time.Duration) {
	return polling(func() ([]Turn, bool, error) { return p.turns(in, id, cwd, after) }), time.Second
}

func (p pluginTalks) search(in folders, text string, limit int) ([]Recalled, error) {
	out, err := p.ask(append([]string{"search", text, in.Root}, in.Paths...)...)
	if err != nil {
		return nil, err
	}
	var hits []struct {
		ID    string    `json:"id"`
		Cwd   string    `json:"cwd"`
		At    time.Time `json:"at"`
		Yours bool      `json:"yours"`
		Line  string    `json:"line"`
		N     int       `json:"n"`
	}
	if err := json.Unmarshal(out, &hits); err != nil {
		return nil, fmt.Errorf("search is not a JSON list: %w", err)
	}
	var found []Recalled
	for _, h := range hits {
		if h.ID != "" && in.holds(h.Cwd) {
			found = append(found, Recalled{Conversation: Conversation{Session: h.ID, Cwd: h.Cwd, At: h.At}, Agent: p.agent, N: h.N, Yours: h.Yours, Line: oneLine(h.Line)})
		}
	}
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found, nil
}
