package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/git"
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
	return plugin.Context{Repo: a.Root, MiaDir: a.MiaDir, Settings: a.Config.Plugin[name], Worktree: a.HereName()}
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
		for _, collision := range verbConflicts(plugins) {
			fmt.Printf("⚠ verb %s is declared by %s; commands with this name are refused\n", collision.Verb, strings.Join(collision.Plugins, ", "))
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
	owners := plugin.Owners(plugins, verb)
	if len(owners) == 0 {
		return 0, false
	}
	if len(owners) > 1 {
		names := make([]string, len(owners))
		for i, p := range owners {
			names[i] = p.Name
		}
		return fail(fmt.Errorf("verb %q is declared by plugins %s; disable all but one", verb, strings.Join(names, ", "))), true
	}
	p := owners[0]
	code, err := p.Run(pluginContext(a, p.Name), append([]string{verb}, args...))
	if err != nil {
		return fail(err), true
	}
	return code, true
}

var coreAliases = map[string]bool{
	"help": true, "-h": true, "--help": true, "version": true, "--version": true,
	"__complete": true, "list": true, "remove": true, "windows": true,
	"machines": true, "runtime": true, "plugins": true,
}

func coreVerb(name string) bool {
	if _, ok := groupOf[name]; ok {
		return true
	}
	return coreAliases[name]
}

func verbConflicts(plugins []plugin.Plugin) []plugin.VerbCollision {
	active := make([]plugin.Plugin, 0, len(plugins))
	for _, p := range plugins {
		if p.Enabled && p.Problem == "" {
			active = append(active, p)
		}
	}
	conflicts := plugin.VerbCollisions(active)
	for _, p := range active {
		for _, verb := range p.Manifest.Verbs {
			if coreVerb(verb.Name) {
				conflicts = append(conflicts, plugin.VerbCollision{Verb: verb.Name, Plugins: []string{"mia", p.Name}})
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Verb < conflicts[j].Verb })
	return conflicts
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

func enabledPlugins(a *app.App) ([]plugin.Plugin, error) {
	return pluginHost(a.Config).Enabled()
}
