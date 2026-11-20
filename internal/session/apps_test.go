package session

import (
	"regexp"
	"testing"
)

func TestAPaneIsAnAppByItsCommandOrByItsTitleUnderARuntime(t *testing.T) {
	apps := Apps{
		Known: []App{
			{Name: "git", Commands: []string{"lazygit", "tig"}},
			{Name: "notes", Title: regexp.MustCompile(`^notes`), Via: []string{"node", "python3"}},
			{Name: "runner", Commands: []string{"node"}},
		},
		Agent: func(command, title string) bool { return command == "codex" || title == "an agent's" },
	}
	for _, c := range []struct {
		command, title, app string
		byTitle             bool
	}{
		{"lazygit", "host", "git", false},
		{"tig", "notes - shop", "git", false},
		{"node", "notes - shop", "notes", true}, // its title says more than node does
		{"python3", "notes", "notes", true},
		{"node", "shop", "runner", false},
		{"zsh", "notes - shop", "", false}, // it quit and left its title on the shell
		{"vim", "host", "", false},
		{"node", "an agent's", "", false},
		{"codex", "notes", "", false},
	} {
		if app, byTitle := apps.of(c.command, c.title); app != c.app || byTitle != c.byTitle {
			t.Errorf("of(%q, %q) = %q, %v, want %q, %v", c.command, c.title, app, byTitle, c.app, c.byTitle)
		}
	}
	if app, _ := (Apps{}).of("lazygit", "host"); app != "" {
		t.Errorf("with no manifests a pane is %q", app)
	}
}

func TestAWindowToldByItsTitleIsCalledByItsApp(t *testing.T) {
	apps := Apps{Known: []App{
		{Name: "git", Commands: []string{"lazygit"}},
		{Name: "notes", Title: regexp.MustCompile(`^notes`), Via: []string{"node"}},
	}}
	w := WindowsWith([]Pane{
		{Index: 1, Window: "node", Command: "node", Title: "notes - shop"},
		{Index: 2, Window: "scratch", Command: "node", Title: "notes - shop"},
		{Index: 3, Window: "lazygit", Command: "lazygit"},
		{Index: 4, Window: "node", Command: "node", Title: "shop"},
	}, apps)
	for i, want := range []Window{
		{Name: "notes", Command: "notes", App: "notes"},
		{Name: "scratch", Command: "notes", App: "notes"}, // a name someone gave it is kept
		{Name: "lazygit", Command: "lazygit", App: "git"},
		{Name: "node", Command: "node"},
	} {
		if w[i].Name != want.Name || w[i].Command != want.Command || w[i].App != want.App {
			t.Errorf("window %d = %s running %s, app %q; want %s running %s, app %q", i+1, w[i].Name, w[i].Command, w[i].App, want.Name, want.Command, want.App)
		}
	}
}

func TestAWindowSaysHowManyPanesItIsSplitInto(t *testing.T) {
	panes := []Pane{{Index: 1, Window: "a", Command: "zsh"}, {Index: 1, Window: "a", Command: "zsh"}, {Index: 2, Window: "b", Command: "zsh"}}
	got := WindowsFrom(panes)
	if len(got) != 2 || got[0].Panes != 2 || got[1].Panes != 1 {
		t.Fatalf("panes per window: %+v", got)
	}
}
