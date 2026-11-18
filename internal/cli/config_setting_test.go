package cli

import (
	"testing"

	"github.com/emusoi/mia-core/internal/config"
)

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
		"keys.quizzicle.quiz": {"keys", "quizzicle.quiz"}, "plugin.agent.default": {"plugin.agent", "default"},
		"agents.claude.account": {"agents.claude", "account"}} {
		if table, key := settingPath(dotted); table != want[0] || key != want[1] {
			t.Errorf("settingPath(%s) = %s, %s", dotted, table, key)
		}
	}
	if v, ok := lookup("[keys]\n\"share.pull\" = \"Y\"\n", "keys", "share.pull"); !ok || v != `"Y"` {
		t.Errorf("lookup = %q %v", v, ok)
	}
	if got := unsetting("[agents.claude]\nmodel = \"opus\"\naccount = \"work\"\n\n[main]\naccount = \"x\"\n", "agents.claude", "account"); got != "[agents.claude]\nmodel = \"opus\"\n\n[main]\naccount = \"x\"\n" {
		t.Errorf("unsetting takes out the one line: %q", got)
	}
	if literal("staging") != `"staging"` || literal(`["npm", "i"]`) != `["npm", "i"]` || literal("true") != "true" {
		t.Error("a value is a string unless it reads as a list, a bool or a number")
	}
	if config.Check(setting("", "", "bsae", `"main"`)) == nil {
		t.Error("a misspelt setting is refused before it is written")
	}
}
