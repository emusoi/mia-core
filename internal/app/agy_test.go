package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/session"
)

// fakeAgy puts a program called agy first on PATH that says the models in
// store/models, as agy models does, and fails as one signed in to nothing
// does when there are none; each time it is run it notes how in
// store/asked. agy's own folder is made, as agy makes it the first time it
// runs.
func fakeAgy(t *testing.T) (store string) {
	t.Helper()
	store, bin := t.TempDir(), t.TempDir()
	script := `#!/bin/sh
echo "$*" >> "` + store + `/asked"
case "$1" in
models) echo "Fetching available models..."; [ -f "` + store + `/models" ] && exec cat "` + store + `/models"; echo "Error: not signed in" >&2; exit 1 ;;
esac
echo "not asked as mia asks: $*" >&2
exit 2
`
	if err := os.WriteFile(filepath.Join(bin, "agy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".gemini", "antigravity-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	return store
}

// What agy models prints is a line saying it fetches them, then a model a
// line, its id and its name a tab apart: each is a model once, and
// anything else is none.
func TestAgysModelsAreTheLinesThatAreAnIDAndItsName(t *testing.T) {
	listed := agyListed("Fetching available models...\ngemini-4-neon-high\tGemini 4 Neon (High)\r\n  gemini-4-flash\tGemini 4 Flash  \n\ngemini-4-neon-high\tGemini 4 Neon (High)\nError: not signed in\nlonely-id\n\tno id\nan id\twith a space\n")
	want := []plugin.Model{{ID: "gemini-4-neon-high", Label: "Gemini 4 Neon (High)"}, {ID: "gemini-4-flash", Label: "Gemini 4 Flash"}}
	if !slices.Equal(listed, want) {
		t.Errorf("listed = %+v", listed)
	}
	for _, none := range []string{"", "Fetching available models...\n", "Error: not signed in\n"} {
		if listed := agyListed(none); listed == nil || len(listed) != 0 {
			t.Errorf("%q lists %+v", none, listed)
		}
	}
}

// agy is asked for its models once and what it said is kept, as
// opencode's are; one that cannot say has none, and it is not asked where
// it has kept nothing. It is started with a model and an effort, each a
// word of its own, and switched by being started again.
func TestAgyIsAskedForItsModelsOnceAndStartedWithOneAndAnEffort(t *testing.T) {
	gittest.Isolate(t)
	cache := t.TempDir()
	if models := (agyModels{cache: cache}).models(); len(models) != 0 {
		t.Fatalf("with no agy: %+v", models)
	}
	store := fakeAgy(t)
	asked := func() int {
		data, _ := os.ReadFile(filepath.Join(store, "asked"))
		_ = os.Remove(filepath.Join(store, "asked"))
		return strings.Count(string(data), "models\n")
	}
	if models := (agyModels{cache: cache}).models(); len(models) != 0 || asked() != 1 {
		t.Fatalf("one that fails has none: %+v", models)
	}
	if err := os.Remove(filepath.Join(cache, "agy.json")); err != nil {
		t.Fatalf("that it had none is kept: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store, "models"), []byte("gemini-4-neon-high\tGemini 4 Neon (High)\ngemini-4-flash\tGemini 4 Flash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := []plugin.Model{{ID: "gemini-4-neon-high", Label: "Gemini 4 Neon (High)"}, {ID: "gemini-4-flash", Label: "Gemini 4 Flash"}}
	for range 2 {
		if models := (agyModels{cache: cache}).models(); !slices.Equal(models, want) {
			t.Fatalf("models = %+v", models)
		}
	}
	if n := asked(); n != 1 {
		t.Errorf("asked %d times, and then what it said was kept", n)
	}

	repo := gittest.New(t)
	a, err := Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	all := a.AgentModels()
	at := slices.IndexFunc(all, func(m AgentModels) bool { return m.Name == "agy" })
	if at < 0 {
		t.Fatalf("agy is not among the agents: %+v", all)
	}
	if its := all[at]; !slices.Equal(its.Models, want) || !slices.Equal(its.Efforts, []string{"low", "medium", "high", "xhigh", "max"}) || !its.Installed || its.Live || !its.Again {
		t.Errorf("agy = %+v", its)
	}
	argv, chosen, err := a.ArgvWith([]string{"agy"}, "gemini-4-flash", "high", "")
	if err != nil || strings.Join(argv, " ") != "agy --model gemini-4-flash --effort high" || chosen != (Chosen{Model: "gemini-4-flash", Effort: "high"}) {
		t.Errorf("started with = %q, %+v, %v", argv, chosen, err)
	}
	if _, _, err := a.ArgvWith([]string{"agy"}, "gemini-9", "", ""); err == nil || err.Error() != "agy has no model gemini-9 — it has gemini-4-neon-high, gemini-4-flash" {
		t.Errorf("a model it does not list: %v", err)
	}
}

// A window runs agy when what it runs is agy: by its program's name, behind
// env, or behind the shell mia starts it by. agy gives its pane no title
// of its own.
func TestAgysWindowIsKnownByWhatItRuns(t *testing.T) {
	for command, want := range map[string]string{
		"agy": "agy",
		"/Users/user/.local/bin/agy --model gemini-4-flash": "agy",
		"env A=1 agy --conversation c1":                     "agy",
		"/bin/sh /opt/bin/agy -c":                           "agy",
		"agyx":                                              "",
		"sh -c agy":                                         "",
	} {
		if got, _ := agentOf(builtin, command); got != want {
			t.Errorf("agentOf(%q) = %q, want %q", command, got, want)
		}
	}
	front := func() map[int]string {
		return map[int]string{7: "/bin/sh -lc agy '--model' 'gemini-4-flash'; exec '/bin/sh' -l"}
	}
	if got := agentBehind(builtin, "sh", "host", session.KindAgent, 7, front); got != "agy" {
		t.Errorf("behind the shell mia started it by: %q", got)
	}
	if got := agentIn(builtin, "agy", "host"); got != "agy" {
		t.Errorf("by its program: %q", got)
	}
}
