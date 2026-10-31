package cli

import (
	"sync"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/panel"
)

func TestAReloadNeverSeesTheListingForgottenUnderIt(t *testing.T) {
	var c listingCache
	list := func() ([]app.Listing, []app.MachineState, error) {
		time.Sleep(time.Millisecond)
		return []app.Listing{{Name: "main", Main: true}}, nil, nil
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				c.forget()
			}
		}
	}()
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				got, _, err := c.get(list)
				if err != nil || len(got) != 1 {
					t.Errorf("a reload saw %d worktrees, %v", len(got), err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
}

func TestNewSaysWhereANewBranchStarts(t *testing.T) {
	p := panel.Panel{Actions: map[string]panel.Action{"new": {Key: "n", Label: "new"}, "branch": {Key: "+", Label: "new worktree"}}}
	namesBase(p, "", []app.Listing{{Name: "lido-app", Branch: "staging", Main: true}, {Name: "temeke", Branch: "auth"}})
	if got := p.Actions["new"].Label; got != "new worktree from staging" {
		t.Errorf("the main checkout's branch, as mia new starts from it: %q", got)
	}
	namesBase(p, "production", nil)
	if got := p.Actions["branch"].Label; got != "new worktree from production, and go there" {
		t.Errorf("a configured base wins: %q", got)
	}
}

func TestASettingEditsOneLineAndKeepsTheRest(t *testing.T) {
	for _, c := range []struct{ in, table, key, value, want string }{
		{"# a comment\nbase = \"main\"\nprefix = \"you/\"\n", "", "base", `"staging"`, "# a comment\nbase = \"staging\"\nprefix = \"you/\"\n"},
		{"prefix = \"you/\"\n\n[popup]\nwidth = \"100%\"\n", "", "base", `"staging"`, "prefix = \"you/\"\n\nbase = \"staging\"\n\n[popup]\nwidth = \"100%\"\n"},
		{"# base = \"main\"\n", "", "base", `"staging"`, "# base = \"main\"\nbase = \"staging\"\n"},
		{"[keys]\nmachines = \"B\"\n", "keys", "quizzicle.quiz", `"K"`, "[keys]\nmachines = \"B\"\n\"quizzicle.quiz\" = \"K\"\n"},
		{"[keys]\n\"quizzicle.quiz\" = \"K\"\n\n[popup]\n", "keys", "quizzicle.quiz", `"J"`, "[keys]\n\"quizzicle.quiz\" = \"J\"\n\n[popup]\n"},
		{"base = \"main\"\n", "plugin.agent", "default", `"codex"`, "base = \"main\"\n\n[plugin.agent]\ndefault = \"codex\"\n"},
		{"", "keys", "share.pull", `"Y"`, "[keys]\n\"share.pull\" = \"Y\"\n"},
	} {
		if got := setting(c.in, c.table, c.key, c.value); got != c.want {
			t.Errorf("setting(%q, %s.%s)\n got %q\nwant %q", c.in, c.table, c.key, got, c.want)
		}
	}
	for dotted, want := range map[string][2]string{"base": {"", "base"}, "popup.width": {"popup", "width"},
		"keys.quizzicle.quiz": {"keys", "quizzicle.quiz"}, "plugin.agent.default": {"plugin.agent", "default"}} {
		if table, key := settingPath(dotted); table != want[0] || key != want[1] {
			t.Errorf("settingPath(%s) = %s, %s", dotted, table, key)
		}
	}
	if v, ok := lookup("[keys]\n\"share.pull\" = \"Y\"\n", "keys", "share.pull"); !ok || v != `"Y"` {
		t.Errorf("lookup = %q %v", v, ok)
	}
	if literal("staging") != `"staging"` || literal(`["npm", "i"]`) != `["npm", "i"]` || literal("true") != "true" {
		t.Error("a value is a string unless it reads as a list, a bool or a number")
	}
	if config.Check(setting("", "", "bsae", `"main"`)) == nil {
		t.Error("a misspelt setting is refused before it is written")
	}
	p := panel.Panel{Actions: map[string]panel.Action{"base": {Key: "B"}}}
	namesBase(p, "", []app.Listing{{Branch: "dash-needs-you", Main: true}})
	if got := p.Actions["base"].Choices; len(got) != 1 || got[0] != "dash-needs-you" {
		t.Errorf("base offers what is there: %v", got)
	}
}
