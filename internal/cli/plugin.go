package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/panel"
	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/plugin"
)

func pluginHost(c config.Config) plugin.Host {
	return plugin.Host{
		Dirs:       append([]string{paths.In("plugins")}, filepath.SplitList(os.Getenv("PATH"))...),
		StatePath:  paths.In("plugins.json"),
		Configured: c.Plugins,
	}
}

func pluginContext(a *app.App, name string) plugin.Context {
	c := plugin.Context{Repo: a.Root, MiaDir: a.MiaDir, Settings: a.Config.Plugin[name]}
	if record, err := a.RecordOrHere(""); err == nil {
		c.Worktree = record.Name
	}
	return c
}

func cmdPlugin(c config.Config, args []string, asJSON bool) int {
	host := pluginHost(c)
	sub := "ls"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "ls", "list":
		if len(args) > 0 {
			return usageErr("mia plugin ls")
		}
		plugins, err := host.List()
		if err != nil {
			return fail(err)
		}
		if asJSON {
			return emit(plugins)
		}
		if len(plugins) == 0 {
			fmt.Println("no plugins — put an executable mia-<name> on PATH or in " + paths.In("plugins"))
			return exitOK
		}
		for _, p := range plugins {
			fmt.Println(pluginLine(p))
		}
		return exitOK

	case "enable", "disable":
		if len(args) != 1 {
			return usageErr("mia plugin " + sub + " <name>")
		}
		name := strings.TrimPrefix(args[0], plugin.Prefix)
		var err error
		if sub == "enable" {
			err = host.Enable(name)
		} else {
			err = host.Disable(name)
		}
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s %sd\n", name, sub)
		return exitOK
	}
	return usageErr(usageOf("plugin"))
}

func pluginLine(p plugin.Plugin) string {
	state := "off"
	if p.Enabled {
		state = "on"
	}
	line := padded(p.Name, 16) + padded(state, 5)
	switch {
	case p.Problem != "":
		return line + "⚠ " + p.Problem
	case !p.Enabled:
		return line + p.Path
	}
	var names []string
	for _, v := range p.Manifest.Verbs {
		names = append(names, v.Name)
	}
	if len(names) > 0 {
		line += "verbs: " + strings.Join(names, ", ") + "  "
	}
	return line + p.Manifest.Help
}

func runPluginVerb(a *app.App, verb string, args []string) (int, bool) {
	plugins, err := pluginHost(a.Config).Enabled()
	if err != nil {
		return fail(err), true
	}
	p, ok := plugin.Owner(plugins, verb)
	if !ok {
		return 0, false
	}
	code, err := p.Run(pluginContext(a, p.Name), append([]string{verb}, args...))
	if err != nil {
		return fail(err), true
	}
	return code, true
}

func pluginRows(a *app.App, listings []app.Listing) []plugin.Rows {
	plugins, err := enabledPlugins(a)
	if err != nil || len(plugins) == 0 {
		return nil
	}
	input := struct {
		Protocol  int           `json:"protocol"`
		Repo      string        `json:"repo"`
		Worktrees []app.Listing `json:"worktrees"`
	}{plugin.Protocol, a.Root, listings}
	return plugin.AllRows(plugins, func(p plugin.Plugin) plugin.Context { return pluginContext(a, p.Name) }, input)
}

func pluginPanel(a *app.App, owner, id, target string) (panel.Panel, error) {
	plugins, err := enabledPlugins(a)
	if err != nil {
		return panel.Panel{}, err
	}
	i := slices.IndexFunc(plugins, func(p plugin.Plugin) bool { return p.Name == owner })
	if i < 0 {
		return panel.Panel{}, fmt.Errorf("plugin %s is not enabled", owner)
	}
	input := struct {
		Protocol int          `json:"protocol"`
		Repo     string       `json:"repo"`
		Worktree *app.Listing `json:"worktree,omitempty"`
	}{Protocol: plugin.Protocol, Repo: a.Root}
	if target != "" {
		if listings, err := a.List(target); err == nil && len(listings) == 1 {
			input.Worktree = &listings[0]
		}
	}
	data, err := plugins[i].Panel(pluginContext(a, owner), id, input)
	if err != nil {
		return panel.Panel{}, err
	}
	var p panel.Panel
	if err := json.Unmarshal(data, &p); err != nil {
		return panel.Panel{}, fmt.Errorf("%s's panel %s is not panel JSON: %w", owner, id, err)
	}
	p.ID, p.Version = panel.PluginPanel(owner, id), panel.Version
	return p, nil
}

func sendEvent(a *app.App, e app.Event) {
	plugins, err := enabledPlugins(a)
	if err != nil {
		return
	}
	event := plugin.Event{
		Protocol: plugin.Protocol,
		Event:    e.Name,
		Repo:     a.Root,
		Worktree: e.Record.Name,
		Path:     e.Record.Path,
		Branch:   git.CurrentBranch(e.Record.Path),
		Detail:   e.Detail,
	}
	for _, p := range plugins {
		if !p.Wants(e.Name) {
			continue
		}
		if err := p.Send(pluginContext(a, p.Name), event); err != nil {
			fmt.Fprintf(os.Stderr, "mia: plugin %s missed %s: %v\n", p.Name, e.Name, err)
		}
	}
}

var (
	servers   = map[string]*plugin.Server{}
	nudgeMu   sync.Mutex
	nudgeOpen func()
)

func watchNudge(nudge func()) {
	nudgeMu.Lock()
	nudgeOpen = nudge
	nudgeMu.Unlock()
}

func nudgeDashboard() {
	nudgeMu.Lock()
	nudge := nudgeOpen
	nudgeMu.Unlock()
	if nudge != nil {
		go nudge()
	}
}

func startServers(a *app.App) func() {
	plugins, err := pluginHost(a.Config).Enabled()
	if err != nil {
		return func() {}
	}
	for _, p := range plugins {
		if p.Manifest.Serve {
			servers[p.Name] = p.Serve(pluginContext(a, p.Name), nudgeDashboard)
		}
	}
	return func() {
		for name, server := range servers {
			server.Stop()
			delete(servers, name)
		}
	}
}

func enabledPlugins(a *app.App) ([]plugin.Plugin, error) {
	plugins, err := pluginHost(a.Config).Enabled()
	for i := range plugins {
		plugins[i].Server = servers[plugins[i].Name]
	}
	return plugins, err
}
