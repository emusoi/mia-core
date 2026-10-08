package panel

import (
	"fmt"

	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/model"
)

func Env(record model.Record, e env.Environment, ports env.Ports, services []env.Status) Panel {
	name := record.Name
	p := Panel{
		Columns: []Column{{Name: "what"}, {Name: "value"}},
		Version: Version,
		ID:      "env",
		Title:   "Environment / " + name,
		Actions: envActions(name),
		Hints:   []string{"u up", "x down", "s shell", "b browse", "h host", "R remove", "q back"},
	}

	state := e.State
	if state == "" {
		state = "none — `u` builds one from what the project already says"
	}
	facts := []Row{
		{ID: "state", Cells: []string{"state", state}},
		{ID: "on", Cells: []string{"on", e.Machine}},
	}
	if e.Image.Ref != "" {
		facts = append(facts, Row{ID: "image", Cells: []string{"image", e.Image.Ref + "  (" + string(e.Image.Source) + ")"}})
	}
	if e.Mirror != "" {
		facts = append(facts, Row{ID: "tree", Cells: []string{"tree", e.Mirror}})
	}
	p.Sections = append(p.Sections, Section{ID: "environment", Label: "environment", Rows: facts})

	var serving []Row
	for _, port := range ports.Listening {
		url := fmt.Sprintf("https://%s%s:%d/", name, env.HostSuffix, port)
		serving = append(serving, Row{ID: fmt.Sprintf("port-%d", port), Cells: []string{url}})
	}
	if len(serving) > 0 {
		p.Sections = append(p.Sections, Section{ID: "serving", Label: "serving", Rows: serving})
	}

	var running []Row
	for _, service := range services {
		state := "stopped"
		if service.Running {
			state = "running"
			if service.Healthy != nil {
				if *service.Healthy {
					state += " · healthy"
				} else {
					state += " · NOT ANSWERING"
				}
			}
		}
		running = append(running, Row{ID: service.ID, Cells: []string{service.ID, state}})
	}
	if len(running) > 0 {
		p.Sections = append(p.Sections, Section{ID: "services", Label: "services", Rows: running})
	}
	return p
}

func envActions(name string) map[string]Action {
	return map[string]Action{
		"up":     {Key: "u", Label: "up", Verb: "env", Args: []string{"up", name}, Help: "build or start the container from what the project already says"},
		"down":   {Key: "x", Label: "down", Verb: "env", Args: []string{"down", name}, Confirm: "Stop the environment for " + name + "?", Help: "stop the container; it starts again with u"},
		"shell":  {Popup: true, Lands: true, Key: "s", Label: "shell inside", Verb: "env", Args: []string{"shell", name}, Help: "a shell inside the container, in the worktree"},
		"browse": {Key: "b", Label: "browse", Verb: "env", Args: []string{"browse", name}, Help: "open https://" + name + ".mia:<port>/ in the browser"},
		"host":   {Lands: true, Key: "h", Label: "host on…", Verb: "env", Args: []string{"host", name, "{input}"}, Input: "machine", Help: "move the environment to a machine from `mia machine list`; the URL does not change"},
		"remove": {Key: "R", Label: "remove", Verb: "env", Args: []string{"rm", name},
			Confirm: "Remove the environment for " + name + "? The worktree stays.", Help: "delete the container and its own volumes, and stop syncing a staging copy; the worktree and shared caches stay"},
	}
}
