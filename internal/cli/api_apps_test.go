package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

// The apps are asked from wherever an app's server was started; one whose
// manifest does not read is left out, said on stderr, and fails nothing.
func TestTheAppsAnswerFromAnywhere(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	if data, code := answered(t, "apps"); strings.TrimSpace(string(data)) != "[]" || code != exitOK {
		t.Fatalf("api apps with none = %s, exit %d", data, code)
	}
	apps := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "apps")
	for name, text := range map[string]string{
		"lazygit": "label = \"Git\"\n[match]\ncommands = [\"lazygit\"]\n[[keys]]\nkey = \"c\"\nlabel = \"commit\"\n",
		"broken":  "[match]\ntitle = \"(\"\nvia = [\"node\"]\n",
		"codex":   "[match]\ncommands = [\"codex\"]\n",
	} {
		if err := os.MkdirAll(filepath.Join(apps, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(apps, name, "app.toml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	said, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = said
	data, code := answered(t, "apps")
	os.Stderr = stderr
	want := `[{"name":"lazygit","label":"Git","match":{"commands":["lazygit"],"via":[]},"keys":[{"key":"c","label":"commit"}]}]`
	var got bytes.Buffer
	if err := json.Compact(&got, data); err != nil {
		t.Fatal(err)
	}
	if got.String() != want || code != exitOK {
		t.Errorf("api apps = %s, exit %d, want %s", data, code, want)
	}
	lines, err := os.ReadFile(said.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(lines)), "\n"); len(got) != 2 || !strings.Contains(got[0], filepath.Join("broken", "app.toml")) || !strings.Contains(got[1], filepath.Join("codex", "app.toml")) {
		t.Errorf("stderr = %q", lines)
	}
	if _, code := answered(t, "apps", "lazygit"); code != exitUsage {
		t.Errorf("api apps with more exited %d", code)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "apps") || !slices.Contains(caps.Features, "apps") {
		t.Errorf("capabilities = %+v", caps)
	}
}
