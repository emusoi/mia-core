package app_test

import (
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestAPullRequestAlreadyOpenIsReusedNotRefetched(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	path := repo.Worktree("pr", "pr-7")
	record, err := a.NewFromPR(7)
	if err != nil || record.Path != path {
		t.Fatalf("NewFromPR = %+v, %v; want the open worktree %s without a fetch (there is no remote)", record, err, path)
	}
}
