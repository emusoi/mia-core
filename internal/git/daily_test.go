package git_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

// A status says what the branch follows and how many commits each has
// that the other has not; one whose branch is gone is counted against
// nothing.
func TestAStatusSaysWhatTheBranchFollows(t *testing.T) {
	got := git.ParseStatus("# branch.oid 0123\x00# branch.head talk\x00# branch.upstream origin/talk\x00# branch.ab +2 -1\x00")
	if want := (git.Standing{Branch: "talk", Head: "0123", Upstream: "origin/talk", ToPush: 2, ToPull: 1}); !reflect.DeepEqual(got, want) {
		t.Errorf("standing = %+v", got)
	}
	got = git.ParseStatus("# branch.oid 0123\x00# branch.head talk\x00# branch.upstream origin/talk\x00")
	if !got.Gone || got.Upstream != "origin/talk" || got.ToPush != 0 {
		t.Errorf("a branch whose own is gone = %+v", got)
	}
}

// The patch of one line is that line among the hunk's lines as the index
// has them: staging, a line the hunk deletes stays and one it adds is
// left out; unstaging, the other way round. The file is named as one only
// changed, a new one too; git's note goes with the line it is of.
func TestOneLineOfAHunkIsItsOwnPatch(t *testing.T) {
	files := git.ParseDiff(twoHunks)
	head := "diff --git a/src/talk.rs b/src/talk.rs\n--- a/src/talk.rs\n+++ b/src/talk.rs\n"
	for _, c := range []struct {
		hunk, line int
		reverse    bool
		want       string
	}{
		{0, 3, false, "@@ -2,3 +2,4 @@\n two\n three\n+and a half\n four\n"},
		{0, 1, false, "@@ -2,3 +2,2 @@\n two\n-three\n four\n"},
		{0, 2, true, "@@ -2,3 +2,4 @@\n two\n+THREE\n and a half\n four\n"},
		{0, 1, true, "@@ -2,5 +2,4 @@\n two\n-three\n THREE\n and a half\n four\n"},
		{1, 1, false, "@@ -40,1 +40,2 @@\n forty\n+\n\\ No newline at end of file\n"},
	} {
		if got, err := files[0].LinePatch(c.hunk, c.line, c.reverse); err != nil || got != head+c.want {
			t.Errorf("line %d of hunk %d (reverse %v) = %q, %v\nwant %q", c.line, c.hunk, c.reverse, got, err, head+c.want)
		}
	}
	if _, err := files[0].LinePatch(0, 0, false); err == nil {
		t.Error("a line the same has a patch")
	}
	added := git.ParseDiff("diff --git a/new.txt b/new.txt\nnew file mode 100644\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,2 @@\n+one\n+two\n")
	if got, err := added[0].LinePatch(0, 1, true); err != nil || got != "diff --git a/new.txt b/new.txt\n--- a/new.txt\n+++ b/new.txt\n@@ -1,1 +1,2 @@\n one\n+two\n" {
		t.Errorf("a line of a new file, to unstage = %q, %v", got, err)
	}
	gone := git.ParseDiff("diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-one\n")
	if _, err := gone[0].LinePatch(0, 0, true); err == nil || !strings.Contains(err.Error(), "unstaged whole") {
		t.Errorf("a line of a staged deletion: %v", err)
	}
	if a, d := files[0].Counts(); a != 3 || d != 1 {
		t.Errorf("counts = +%d -%d", a, d)
	}
}

// In a real repository: one line of a staged new file is unstaged alone;
// what is changed and what is untracked is discarded; the last commit is
// amended and undone; the stash is kept by its commit, whatever place it
// has; and a branch is pushed to and pulled from a bare repository, never
// past a commit the other side has.
func TestTheDeedsOfADailyToolInARepository(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("new.txt", "one\ntwo\nthree\n")
	repo.Git("add", "new.txt")
	files, err := git.DiffOf(repo.Root, "new.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := files[0].LinePatch(0, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := git.ApplyToIndex(repo.Root, patch, true); err != nil || repo.Git("show", ":new.txt") != "one\nthree" {
		t.Fatalf("one line unstaged: %v, the index holds %q", err, repo.Git("show", ":new.txt"))
	}
	repo.Commit("A new file")

	repo.Write("new.txt", "one\n")
	repo.Write("scrap.txt", "scrap\n")
	if err := git.Clean(repo.Root, "new.txt"); err != nil || repo.Git("status", "--porcelain") != "M new.txt\n?? scrap.txt" {
		t.Errorf("clean of a tracked file: %v, %q", err, repo.Git("status", "--porcelain"))
	}
	if err := git.Restore(repo.Root, "new.txt"); err != nil {
		t.Fatal(err)
	}
	if err := git.Clean(repo.Root, "scrap.txt"); err != nil || repo.Git("status", "--porcelain") != "" {
		t.Errorf("discarded: %v, %q", err, repo.Git("status", "--porcelain"))
	}

	repo.Write("more.txt", "more\n")
	repo.Git("add", "more.txt")
	if hash, err := git.Amend(repo.Root, ""); err != nil || hash != repo.Git("rev-parse", "--short", "HEAD") || repo.Git("log", "-1", "--format=%s") != "A new file" || repo.Git("show", "--format=", "--name-only") != "more.txt\nnew.txt" {
		t.Errorf("amended: %v, %q holding %q", err, repo.Git("log", "-1", "--format=%s"), repo.Git("show", "--format=", "--name-only"))
	}
	if author, when := git.Told(repo.Root, "HEAD"); author != "test" || when.IsZero() {
		t.Errorf("told = %q at %v", author, when)
	}
	if err := git.UndoCommit(repo.Root); err != nil || repo.Git("log", "-1", "--format=%s") != "first" || repo.Git("diff", "--cached", "--name-only") != "more.txt\nnew.txt" {
		t.Errorf("undone: %v, staged %q", err, repo.Git("diff", "--cached", "--name-only"))
	}
	repo.Commit("A new file")

	repo.Write("new.txt", "first stash\n")
	if err := git.StashAll(repo.Root, "the first"); err != nil {
		t.Fatal(err)
	}
	repo.Write("loose.txt", "second stash\n")
	if err := git.StashAll(repo.Root, "the second"); err != nil {
		t.Fatal(err)
	}
	stashes := git.Stashes(repo.Root)
	if len(stashes) != 2 || stashes[0].Subject != "On main: the second" || stashes[1].Subject != "On main: the first" {
		t.Fatalf("stashes = %+v", stashes)
	}
	if shown, err := git.StashShown(repo.Root, stashes[0].SHA); err != nil || len(shown) != 1 || shown[0].Path != "loose.txt" {
		t.Errorf("the second stash's diff = %+v, %v", shown, err)
	}
	if err := git.StashDrop(repo.Root, stashes[0].SHA); err != nil { // the first is now stash@{0}
		t.Fatal(err)
	}
	if err := git.StashPop(repo.Root, stashes[1].SHA); err != nil || repo.Git("status", "--porcelain") != "M new.txt" || len(git.Stashes(repo.Root)) != 0 {
		t.Errorf("popped by its commit: %v, %q", err, repo.Git("status", "--porcelain"))
	}
	if err := git.StashDrop(repo.Root, stashes[1].SHA); err == nil || err.Error() != "that stash is no longer there" {
		t.Errorf("a stash that is gone: %v", err)
	}
	repo.Git("checkout", "-q", "--", "new.txt")

	bare := filepath.Join(filepath.Dir(repo.Root), "origin.git")
	repo.GitIn(filepath.Dir(repo.Root), "init", "-q", "--bare", "-b", "main", bare)
	repo.Git("remote", "add", "origin", bare)
	if remote, theirs := git.Follows(repo.Root, "main"); remote != "" || theirs != "" || git.Unpushed(repo.Root) != 2 {
		t.Errorf("before the first push: follows %q %q, %d unpushed", remote, theirs, git.Unpushed(repo.Root))
	}
	if said, err := git.PushTo(repo.Root, "origin", "main", "main", true); err != nil || said == "" {
		t.Fatalf("pushed: %q, %v", said, err)
	}
	if remote, theirs := git.Follows(repo.Root, "main"); remote != "origin" || theirs != "main" || git.Unpushed(repo.Root) != 0 {
		t.Errorf("after it: follows %q %q", remote, theirs)
	}
	pushed := repo.Git("rev-parse", "HEAD")
	repo.Git("commit", "-q", "--amend", "-m", "Rewritten")
	if _, err := git.PushTo(repo.Root, "origin", "main", "main", false); err == nil || !strings.Contains(err.Error(), "rejected") || repo.GitIn(bare, "rev-parse", "main") != pushed {
		t.Errorf("a push past what the remote has: %v", err)
	}
	if _, err := git.PullFastForward(repo.Root); err == nil || !strings.Contains(err.Error(), "fast-forward") || repo.Git("log", "-1", "--format=%s") != "Rewritten" {
		t.Errorf("a pull that cannot fast-forward: %v", err)
	}
	repo.Git("reset", "-q", "--hard", "HEAD~1")
	if said, err := git.FetchFrom(repo.Root, "origin"); err != nil || said != "" {
		t.Errorf("a fetch of nothing new: %q, %v", said, err)
	}
	if said, err := git.PullFastForward(repo.Root); err != nil || said == "" || repo.Git("rev-parse", "HEAD") != pushed {
		t.Errorf("pulled: %q, %v", said, err)
	}

	repo.Git("branch", "talk")
	branches, err := git.Branches(repo.Root)
	if want := []git.Branch{{Name: "main", Current: true, Upstream: "origin/main"}, {Name: "talk"}, {Name: "origin/main", Remote: true}}; err != nil || !reflect.DeepEqual(branches, want) {
		t.Errorf("branches = %+v, %v", branches, err)
	}
	if err := git.SwitchNew(repo.Root, "words"); err != nil || git.CurrentBranch(repo.Root) != "words" {
		t.Errorf("a new branch: %v", err)
	}
	if err := git.Switch(repo.Root, "talk"); err != nil || git.CurrentBranch(repo.Root) != "talk" {
		t.Errorf("switched: %v", err)
	}
	if err := git.Switch(repo.Root, "origin"); err == nil {
		t.Error("a switch to no branch is no error")
	}
}
