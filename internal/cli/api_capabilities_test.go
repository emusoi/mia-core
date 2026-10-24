package cli

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestAPICapabilitiesAdvertiseVersionQueriesAndFeatures(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	out, err := os.CreateTemp(t.TempDir(), "capabilities")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = out
	code := Run([]string{"api", "capabilities"})
	os.Stdout = stdout
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if code != exitOK {
		t.Fatalf("api capabilities exited %d", code)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	var got capabilities
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != apiVersion || !slices.Contains(got.Queries, "worktrees") || !slices.Contains(got.Features, "direct-http") {
		t.Fatalf("capabilities = %+v", got)
	}
}
