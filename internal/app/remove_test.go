package app_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/model"
)

func TestRemovalRefusesALiveSessionUntilToldToStopIt(t *testing.T) {
	r := app.Removal{Record: model.Record{Name: "monduli"}, Session: true}
	err := r.Blocked(false, false)
	if err == nil || !strings.Contains(err.Error(), "running") || !strings.Contains(err.Error(), "--stop-running") {
		t.Errorf("a live session was not refused with its way past: %v", err)
	}
	if err := r.Blocked(false, true); err != nil {
		t.Errorf("told to stop it, still refused: %v", err)
	}
	both := app.Removal{Record: model.Record{Name: "monduli"}, Session: true, Dirty: true}
	if err := both.Blocked(true, false); err == nil || !strings.Contains(err.Error(), "running") {
		t.Errorf("--force alone should still refuse the session: %v", err)
	}
	if err := both.Blocked(true, true); err != nil {
		t.Errorf("both flags, still refused: %v", err)
	}
}

func TestStarTogglesOnTheRecord(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	record, err := a.New("mine")
	if err != nil {
		t.Fatal(err)
	}
	if on, err := a.Star(record.Name); err != nil || !on {
		t.Fatalf("first star: %v %v", on, err)
	}
	listings, _ := a.List()
	starred := false
	for _, l := range listings {
		starred = starred || (l.Name == record.Name && l.Starred)
	}
	if !starred {
		t.Error("the listing does not show the star")
	}
	if on, _ := a.Star(record.Name); on {
		t.Error("second star did not unstar")
	}
}

func TestNewStartsFromTheConfiguredBase(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	base := git.CurrentBranch(repo.Root)
	baseHead := git.Head(repo.Root)
	if err := git.CreateBranch(repo.Root, "elsewhere"); err != nil {
		t.Fatal(err)
	}
	repo.Write("far.txt", "x\n")
	repo.Commit("far away")
	a.Config.Base = base
	record, err := a.New("from-base")
	if err != nil {
		t.Fatal(err)
	}
	if git.Head(record.Path) != baseHead {
		t.Errorf("new branch starts at %s, want %s (%s)", git.Head(record.Path), baseHead, base)
	}
}

func TestSetupRunsAgainAndAFailedSetupIsReported(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	a.Config.Setup = []string{"sh", "-c", "exit 3"}
	record, err := a.New("needs-setup")
	if err == nil || record.Path == "" {
		t.Fatalf("a failed setup must report and still hand back the worktree: %v %+v", err, record)
	}
	a.Config.Setup = []string{"sh", "-c", "touch setup-ran"}
	if err := a.Setup(record.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(record.Path, "setup-ran")); err != nil {
		t.Error("setup did not run in the worktree")
	}
}

func TestNewAndRemoveTellWhatHappenedOnce(t *testing.T) {
	repo := gittest.New(t)
	a := open(t, repo)
	var told []string
	a.Events = func(e app.Event) { told = append(told, e.Name+" "+e.Record.Name) }

	record, err := a.New("mine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.New("mine"); err != nil {
		t.Fatal(err)
	}
	removal, err := a.PlanRemoval(record.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remove(removal, false, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"worktree.created " + record.Name, "worktree.removed " + record.Name}
	if strings.Join(told, ", ") != strings.Join(want, ", ") {
		t.Errorf("told %v, want %v", told, want)
	}
}

func TestARecordWhosePathCannotBeReadIsKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	repo := gittest.New(t)
	a := open(t, repo)
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(filepath.Join(locked, "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	away := model.Record{Path: filepath.Join(locked, "tree"), Name: "away"}
	if err := a.Store.Put(away); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	if _, err := a.List(); err != nil {
		t.Fatal(err)
	}
	records, _ := a.Store.Load()
	if !slices.ContainsFunc(records, func(r model.Record) bool { return r.Path == away.Path }) {
		t.Error("a worktree mia could not stat was forgotten as if it were gone")
	}
}
