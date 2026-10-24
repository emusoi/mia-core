package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestConfigShowPrintsTheEffectiveProjectConfigAsJSON(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write(".mia.toml", "base = \"project-main\"\n")
	t.Chdir(repo.Root)

	out, err := os.CreateTemp(t.TempDir(), "config-show")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := Run([]string{"config", "show", "--json"})
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Fatalf("config show exited %d", code)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Base string `json:"base"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("config show was not JSON: %v\n%s", err, data)
	}
	if got.Base != "project-main" {
		t.Fatalf("base = %q, want project-main; output: %s", got.Base, data)
	}
}
