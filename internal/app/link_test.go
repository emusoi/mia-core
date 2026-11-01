package app

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestABaseThatOnlyTracksARemoteGoesToAMachineAsABranch(t *testing.T) {
	origin := gittest.New(t)
	here := filepath.Join(t.TempDir(), "here")
	origin.GitIn(filepath.Dir(here), "clone", "-q", origin.Root, here)

	if branch, refs := baseRefs(here, "main"); branch != "main" || !slices.Equal(refs, []string{"refs/heads/main:refs/heads/main"}) {
		t.Errorf("a branch goes as itself: %s %v", branch, refs)
	}
	branch, refs := baseRefs(here, "origin/main")
	want := []string{"refs/remotes/origin/main:refs/heads/main", "refs/remotes/origin/main:refs/remotes/origin/main"}
	if branch != "main" || !slices.Equal(refs, want) {
		t.Errorf("origin/main becomes main there, and stays origin/main too: %s %v", branch, refs)
	}
}
