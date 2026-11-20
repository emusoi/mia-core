package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/session"
)

// manifest writes apps/<name>/app.toml in mia's folder and says the app's folder.
func manifest(t *testing.T, name, text string) string {
	t.Helper()
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "apps", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAnAppIsWhatItsManifestSays(t *testing.T) {
	gittest.Isolate(t)
	dir := manifest(t, "lazygit", "label = \"Git\"\nicon = \"icon.svg\"\n[match]\ncommands = [\"lazygit\"]\ntitle = \"^lazygit\"\nvia = [\"node\", \"python3\"]\n[[keys]]\nkey = \"c\"\nlabel = \"commit\"\n")
	if err := os.WriteFile(filepath.Join(dir, "icon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest(t, "btop", "icon = \"gone.svg\"\nlater = \"a key of a mia to come\"\n[match]\ncommands = [\"btop\"]\n")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(dir), "empty"), 0o755); err != nil { // a folder with no manifest is no app
		t.Fatal(err)
	}
	apps, problems := Outside().Apps()
	if len(problems) != 0 || len(apps) != 2 {
		t.Fatalf("apps = %+v, problems = %v", apps, problems)
	}
	got, err := json.Marshal(apps)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"btop","match":{"commands":["btop"],"via":[]},"keys":[]},` +
		`{"name":"lazygit","label":"Git","icon":"` + filepath.Join(dir, "icon.svg") + `","match":{"commands":["lazygit"],"title":"^lazygit","via":["node","python3"]},"keys":[{"key":"c","label":"commit"}]}]`
	if string(got) != want {
		t.Errorf("apps =\n%s\nwant\n%s", got, want)
	}
}

func TestAManifestThatDoesNotReadIsLeftOutAndSaid(t *testing.T) {
	gittest.Isolate(t)
	manifest(t, "good", "[match]\ncommands = [\"good\"]\n")
	for name, text := range map[string]string{
		"notoml":  "label = \n",
		"nomatch": "label = \"Nothing says what it is\"\n",
		"regex":   "[match]\ntitle = \"(\"\nvia = [\"node\"]\n",
		"novia":   "[match]\ntitle = \"^app\"\n",
		"typed":   "[match]\ncommands = \"one\"\n",
		"pi":      "[match]\ntitle = \"^π\"\nvia = [\"node\"]\n", // an agent's name
		"claude":  "[match]\ncommands = [\"claude\"]\n",
	} {
		manifest(t, name, text)
	}
	apps, problems := Outside().Apps()
	if len(apps) != 1 || apps[0].Name != "good" {
		t.Errorf("apps = %+v", apps)
	}
	if len(problems) != 7 {
		t.Fatalf("problems = %q", problems)
	}
	for _, name := range []string{"notoml", "nomatch", "regex", "novia", "typed", "pi", "claude"} {
		if !strings.Contains(strings.Join(problems, "\n"), filepath.Join("apps", name, "app.toml")+": ") {
			t.Errorf("no line names %s: %q", name, problems)
		}
	}
}

// A window says the app it runs, and is called by it when only its title
// said so; an agent's window is no app's.
func TestAWindowSaysTheAppAManifestTellsItIs(t *testing.T) {
	gittest.Isolate(t)
	manifest(t, "lazygit", "[match]\ncommands = [\"lazygit\"]\n")
	manifest(t, "notes", "[match]\ntitle = \"^notes\"\nvia = [\"node\", \"python3\"]\n")
	manifest(t, "greedy", "[match]\ncommands = [\"codex\", \"node\"]\ntitle = \".\"\nvia = [\"claude\", \"pi\", \"zsh\"]\n")
	windows := session.WindowsWith([]session.Pane{
		{Index: 1, Window: "lazygit", Command: "lazygit", Title: "host"},
		{Index: 2, Window: "node", Command: "node", Title: "notes - shop"},
		{Index: 3, Window: "codex", Command: "codex", Title: "host"},
		{Index: 4, Window: "node", Command: "node", Title: "π - shop"},
		{Index: 5, Window: "2.1.316", Command: "2.1.316", Title: "✳ Ship it"},
		{Index: 6, Window: "vim", Command: "vim", Title: "notes - shop"},
	}, Outside().apps())
	for i, want := range []string{
		`{"index":1,"name":"lazygit","command":"lazygit","quiet":"0s","active":false,"app":"lazygit","panes":1}`,
		`{"index":2,"name":"notes","command":"notes","quiet":"0s","active":false,"app":"notes","panes":1}`,
		`{"index":3,"name":"codex","command":"codex","quiet":"0s","active":false,"panes":1}`,
		`{"index":4,"name":"pi","command":"pi","quiet":"0s","active":false,"panes":1}`,
		`{"index":5,"name":"claude","command":"claude","quiet":"0s","active":false,"agent":"Ship it","panes":1}`,
		`{"index":6,"name":"vim","command":"vim","quiet":"0s","active":false,"panes":1}`,
	} {
		if got, _ := json.Marshal(windows[i]); string(got) != want {
			t.Errorf("window %d = %s, want %s", i+1, got, want)
		}
	}
}

func TestAnAgentIsInstalledWhenItsProgramIsOnPath(t *testing.T) {
	if !installed(Agent{Name: "sh"}) || !installed(Agent{Name: "nowhere-x", Commands: []string{"sh -c"}}) {
		t.Fatal("a program on PATH, by its name or the first word of its command, is installed")
	}
	if installed(Agent{Name: "no-such-agent-here"}) {
		t.Fatal("a program PATH does not have is not installed")
	}
}
