package cli

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/plugin"
)

func TestTheTmuxMenuHasTheSidesAndThePluginsEntries(t *testing.T) {
	entry := func(key, label, verb string) plugin.Entry {
		e := plugin.Entry{Key: key, Label: label, Verb: verb}
		e.Popup.Width, e.Popup.Height = "80", "20"
		return e
	}
	plugins := []plugin.Plugin{
		{Name: "gh", Manifest: plugin.Manifest{Sides: []plugin.Side{{ID: "prs", Where: "left", Label: "prs", Rank: 20}}}},
		{Name: "issue", Manifest: plugin.Manifest{Sides: []plugin.Side{{ID: "issues", Where: "left", Label: "issues", Rank: 10}}}},
		{Name: "type", Manifest: plugin.Manifest{Menu: []plugin.Entry{entry("y", "type while you wait", "type"), entry("g", "steals the dashboard", "type")}}},
	}
	plugins[2].Manifest.Menu[0].Popup.X, plugins[2].Manifest.Menu[0].Popup.Y = "R", "S"
	menu, problems := tmuxMenu(plugins)
	for _, want := range []string{
		`  "top"           1 "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 64 -h 18 'mia side 1'" \`,
		`  "issues"        2 `,
		`  "dashboard"     g "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 100% -h 100% 'mia dash'" \`,
		`  "type while you wait" y "display-popup -B -E -e MIA_POPUP=1 -e 'MIA_TMUX_PANE=#{pane_id}' -d '#{pane_current_path}' -w 80 -h 20 -x R -y S 'mia type'"`,
	} {
		if !strings.Contains(menu, want) {
			t.Errorf("missing %q in\n%s", want, menu)
		}
	}
	if strings.Contains(menu, "steals") || strings.HasSuffix(strings.TrimSpace(menu), `\`) {
		t.Errorf("a clashing entry is left out and the last line ends the menu:\n%s", menu)
	}
	if len(problems) != 1 || problems[0] != "plugin  type: g is already dashboard in the tmux menu; steals the dashboard is left out" {
		t.Errorf("the clash is told: %v", problems)
	}
}
