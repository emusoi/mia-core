package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/gittest"
)

func writeRole(t *testing.T, name, text string) {
	t.Helper()
	path, err := RolePath(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A role's front matter says what it starts, all of it optional; a file
// with none is all body. One that cannot be read says why, and hides no
// other.
func TestARoleIsFrontMatterAndABody(t *testing.T) {
	gittest.Isolate(t)
	writeRole(t, "reviewer", "---\nagent: claude\nmodel: opus\neffort: \"high\"\naccount: work\n---\nYou review.\n\nRead the diff.\n")
	writeRole(t, "plain", "Say what you see.\n")
	writeRole(t, "empty-front", "---\n---\nOnly a body.")
	writeRole(t, "unclosed", "---\nagent: claude\nYou review.\n")
	writeRole(t, "unknown-key", "---\nagent: claude\ncolour: red\n---\nbody")
	writeRole(t, "no-colon", "---\nagent claude\n---\nbody")
	writeRole(t, "two-words", "---\nmodel: opus high\n---\nbody")

	for name, want := range map[string]Role{
		"reviewer":    {Name: "reviewer", Agent: "claude", Model: "opus", Effort: "high", Account: "work", Body: "You review.\n\nRead the diff."},
		"plain":       {Name: "plain", Body: "Say what you see."},
		"empty-front": {Name: "empty-front", Body: "Only a body."},
	} {
		got, err := ReadRole(name)
		want.Path, _ = RolePath(name)
		if err != nil || got != want {
			t.Errorf("role %s = %+v, %v", name, got, err)
		}
	}
	for name, says := range map[string]string{
		"unclosed":    "no closing ---",
		"unknown-key": "says colour",
		"no-colon":    "line 2 of its front matter is no key: value",
		"two-words":   `its model is "opus high"`,
		"missing":     "there is no role missing — `mia role path missing`",
		"../escape":   "no role's name",
		".hidden":     "no role's name",
	} {
		if _, err := ReadRole(name); err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("role %s: %v, want it to say %q", name, err, says)
		}
	}
	roles, problems := Roles()
	var names []string
	for _, role := range roles {
		names = append(names, role.Name)
	}
	if strings.Join(names, " ") != "empty-front plain reviewer" || len(problems) != 4 {
		t.Errorf("roles = %v, problems = %v", names, problems)
	}
}

// A role is a setup: listed with the others, after them, by name — what it
// leaves out is the default setup's, the one used most here. One whose
// agent mia does not know, or whose model that agent has not, is refused
// when used.
func TestRolesAreListedWithTheSetupsAndRefusedWhatTheAgentHasNot(t *testing.T) {
	gittest.Isolate(t)
	history := []Use{{"claude", "sonnet", "medium", "mia-phone", "goba", "local", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), ""}}
	data, _ := json.Marshal(history)
	if err := os.MkdirAll(filepath.Dir(usesPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(usesPath(), data, 0o644); err != nil {
		t.Fatal(err)
	}
	writeRole(t, "reviewer", "---\nagent: claude\nmodel: opus\naccount: work\n---\nYou review.")
	writeRole(t, "plain", "Say what you see.")
	writeRole(t, "broken", "---\nagent: claude\n")
	a := &App{Root: "/src/mia-phone", Config: config.Config{Agents: map[string]config.Agent{"claude": {Effort: "high"}}}}

	type row struct{ agent, model, effort, role, body, account string }
	var rows []row
	for _, s := range a.Setups("mia-phone") {
		rows = append(rows, row{s.Agent, s.Model, s.Effort, s.Role, s.Body, s.Account})
	}
	want := []row{
		{"claude", "sonnet", "medium", "", "", ""},
		{"claude", "", "high", "", "", ""}, // [agents.claude]'s
		{"claude", "sonnet", "medium", "plain", "Say what you see.", ""},
		{"claude", "opus", "", "reviewer", "You review.", "work"},
	}
	if len(rows) != len(want) {
		t.Fatalf("setups = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("setup %d = %+v, want %+v", i, rows[i], want[i])
		}
	}

	// the agent named is the default before the one used most
	if setup, err := a.RoleSetup(Role{Name: "plain"}, "codex"); err != nil || setup.Agent != "codex" || setup.Model != "" {
		t.Errorf("a role with no agent, in a window named codex: %+v, %v", setup, err)
	}
	for role, says := range map[Role]string{
		{Name: "ghost", Agent: "nobody"}:               "role ghost: mia knows no agent nobody",
		{Name: "big", Agent: "claude", Model: "huge"}:  "role big: claude has no model huge",
		{Name: "lazy", Agent: "claude", Effort: "nap"}: "role lazy: claude has no effort nap",
	} {
		if _, err := a.RoleSetup(role, ""); err == nil || !strings.HasPrefix(err.Error(), says) {
			t.Errorf("role %s: %v", role.Name, err)
		}
	}
}
