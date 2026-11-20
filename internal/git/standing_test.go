package git_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

// A status is read into its groups: a file staged in part is in two, a
// renamed one says where it was, a conflict is changed, and a path keeps
// its spaces.
func TestAStatusIsReadIntoStagedChangedAndUntracked(t *testing.T) {
	const hashes = "N... 100644 100644 100644 0123456789abcdef0123456789abcdef01234567 0123456789abcdef0123456789abcdef01234567 "
	out := strings.Join([]string{
		"# branch.oid 0123456789abcdef0123456789abcdef01234567",
		"# branch.head mui-patch",
		"1 A. " + hashes + "src/mui/patch.rs",
		"1 MM " + hashes + "src/mui.rs",
		"1 .D " + hashes + "old notes.md",
		"2 R. " + hashes + "R100 src/talk.rs", "src/hear.rs",
		"u UU N... 100644 100644 100644 100644 0123 4567 89ab docs/apps.md",
		"? tests/mui patch.rs",
		"",
	}, "\x00")
	want := git.Standing{
		Branch: "mui-patch", Head: "0123456789abcdef0123456789abcdef01234567",
		Staged: []git.Change{
			{Path: "src/mui/patch.rs", State: "new"},
			{Path: "src/mui.rs"},
			{Path: "src/talk.rs", From: "src/hear.rs", State: "renamed"},
		},
		Changed: []git.Change{
			{Path: "src/mui.rs"},
			{Path: "old notes.md", State: "deleted"},
			{Path: "docs/apps.md", State: "conflict"},
		},
		Untracked: []git.Change{{Path: "tests/mui patch.rs"}},
	}
	if got := git.ParseStatus(out); !reflect.DeepEqual(got, want) {
		t.Errorf("standing = %+v\nwant       %+v", got, want)
	}
	// before the first commit, and on no branch
	got := git.ParseStatus("# branch.oid (initial)\x00# branch.head (detached)\x00")
	if !reflect.DeepEqual(got, git.Standing{}) {
		t.Errorf("standing = %+v, want none", got)
	}
}

const twoHunks = `diff --git a/src/talk.rs b/src/talk.rs
index 1111111..2222222 100644
--- a/src/talk.rs
+++ b/src/talk.rs
@@ -2,3 +2,4 @@ impl Talk
 two
-three
+THREE
+and a half
 four
@@ -40 +41,2 @@
 forty
+
\ No newline at end of file
`

// A diff is read into its files and hunks: where each hunk is, the words
// over it, and its lines as git wrote them, a blank one and git's own note
// too. The patch of one hunk is the file's head and that hunk alone.
func TestADiffIsReadIntoHunksAndOneIsItsOwnPatch(t *testing.T) {
	files := git.ParseDiff("commit 0123abc\n\n    A subject\n\n" + twoHunks)
	if len(files) != 1 || files[0].Path != "src/talk.rs" || files[0].Binary || len(files[0].Hunks) != 2 {
		t.Fatalf("files = %+v", files)
	}
	first, second := files[0].Hunks[0], files[0].Hunks[1]
	if first.OldStart != 2 || first.OldLines != 3 || first.NewStart != 2 || first.NewLines != 4 || first.Section != "impl Talk" || len(first.Body) != 5 {
		t.Errorf("the first hunk = %+v", first)
	}
	if second.OldStart != 40 || second.OldLines != 1 || second.NewStart != 41 || second.NewLines != 2 || second.Section != "" {
		t.Errorf("the second hunk = %+v", second)
	}
	if want := []string{" forty", "+", `\ No newline at end of file`}; !reflect.DeepEqual(second.Body, want) {
		t.Errorf("the second hunk's lines = %q", second.Body)
	}
	head := "diff --git a/src/talk.rs b/src/talk.rs\nindex 1111111..2222222 100644\n--- a/src/talk.rs\n+++ b/src/talk.rs\n"
	if got, want := files[0].Patch(1), head+"@@ -40 +41,2 @@\n forty\n+\n\\ No newline at end of file\n"; got != want {
		t.Errorf("the second hunk's patch = %q\nwant %q", got, want)
	}
	if got := files[0].Patch(0); !strings.HasPrefix(got, head+"@@ -2,3 +2,4 @@ impl Talk\n two\n-three\n") || strings.Contains(got, "forty") {
		t.Errorf("the first hunk's patch = %q", got)
	}

	// a commit's diff has a file after a file; one deleted is known by where
	// it was, a binary one has no hunk, and one only moved has neither
	files = git.ParseDiff(twoHunks +
		"diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\nindex 1111111..0000000\n--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-diff --git a/x b/x\n" +
		"diff --git a/logo.png b/logo.png\nindex 1111111..2222222 100644\nBinary files a/logo.png and b/logo.png differ\n" +
		"diff --git a/was.txt b/is.txt\nsimilarity index 100%\nrename from was.txt\nrename to is.txt\n")
	if len(files) != 4 {
		t.Fatalf("%d files", len(files))
	}
	if gone := files[1]; gone.Path != "gone.txt" || len(gone.Hunks) != 1 || gone.Hunks[0].NewLines != 0 || !reflect.DeepEqual(gone.Hunks[0].Body, []string{"-diff --git a/x b/x"}) {
		t.Errorf("the deleted file = %+v", gone)
	}
	if !files[2].Binary || len(files[2].Hunks) != 0 || files[2].Path != "logo.png" || files[3].Path != "is.txt" {
		t.Errorf("the binary file = %+v, the moved one = %+v", files[2], files[3])
	}
}

func numbered(from, to int, change map[int]string) string {
	var lines strings.Builder
	for n := from; n <= to; n++ {
		line := fmt.Sprintf("line %d", n)
		if other, ok := change[n]; ok {
			line = other
		}
		lines.WriteString(line + "\n")
	}
	return lines.String()
}

// How a worktree stands is read from a real repository: each file in its
// group with its lines added and deleted, a renamed one under its new
// name, and one before the first commit of a branch that has none.
func TestAStandingIsReadFromARepository(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", numbered(1, 30, nil))
	repo.Write("was.txt", numbered(1, 12, nil))
	repo.Commit("two files")
	repo.Write("talk.txt", numbered(1, 30, map[int]string{3: "three", 28: "twenty-eight"}))
	repo.Git("mv", "was.txt", "is.txt")
	repo.Write("new file.txt", "one\ntwo\n")
	repo.Git("add", "new file.txt")
	repo.Write("loose.txt", "loose\n")

	got, err := git.StandingOf(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	want := git.Standing{
		Branch: "main", Head: repo.Git("rev-parse", "HEAD"),
		Staged:    []git.Change{{Path: "is.txt", From: "was.txt", State: "renamed"}, {Path: "new file.txt", State: "new", Added: 2}},
		Changed:   []git.Change{{Path: "talk.txt", Added: 2, Deleted: 2}},
		Untracked: []git.Change{{Path: "loose.txt"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("standing = %+v\nwant       %+v", got, want)
	}

	fresh := filepath.Join(t.TempDir(), "fresh")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	repo.GitIn(fresh, "init", "-q", "-b", "main")
	if got, err := git.StandingOf(fresh); err != nil || got.Branch != "main" || got.Head != "" {
		t.Errorf("a repository with no commit = %+v, %v", got, err)
	}
	if _, err := git.StandingOf(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("a folder in no repository: %v", err)
	}
}

// One hunk of a file's two is staged by its patch and the other stays
// changed; unstaged by the same patch reversed, the file is as it was. A
// commit takes what is staged with the message as given, and with nothing
// staged git refuses in its own words.
func TestOneHunkOfTwoIsStagedUnstagedAndCommitted(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", numbered(1, 30, nil))
	repo.Commit("a file")
	repo.Write("talk.txt", numbered(1, 30, map[int]string{3: "three", 28: "twenty-eight"}))

	changed, err := git.DiffOf(repo.Root, "talk.txt", false)
	if err != nil || len(changed) != 1 || len(changed[0].Hunks) != 2 {
		t.Fatalf("the changed diff = %+v, %v", changed, err)
	}
	if err := git.ApplyToIndex(repo.Root, changed[0].Patch(1), false); err != nil {
		t.Fatal(err)
	}
	if got := repo.Git("diff", "--cached", "--unified=0"); !strings.Contains(got, "+twenty-eight") || strings.Contains(got, "+three") {
		t.Errorf("staged after the second hunk alone:\n%s", got)
	}
	if got := repo.Git("diff", "--unified=0"); !strings.Contains(got, "+three") || strings.Contains(got, "+twenty-eight") {
		t.Errorf("still changed after the second hunk alone:\n%s", got)
	}

	staged, err := git.DiffOf(repo.Root, "talk.txt", true)
	if err != nil || len(staged) != 1 || len(staged[0].Hunks) != 1 {
		t.Fatalf("the staged diff = %+v, %v", staged, err)
	}
	if err := git.ApplyToIndex(repo.Root, staged[0].Patch(0), true); err != nil {
		t.Fatal(err)
	}
	if got := repo.Git("diff", "--cached"); got != "" {
		t.Errorf("staged after the hunk was unstaged:\n%s", got)
	}
	if _, err := git.CommitStaged(repo.Root, "nothing"); err == nil {
		t.Error("a commit of nothing staged was made")
	}

	if err := git.Stage(repo.Root, "talk.txt"); err != nil {
		t.Fatal(err)
	}
	if err := git.Unstage(repo.Root, "talk.txt"); err != nil || repo.Git("diff", "--cached") != "" {
		t.Errorf("a file staged and unstaged is still staged: %v", err)
	}
	if err := git.Stage(repo.Root); err != nil {
		t.Fatal(err)
	}
	const message = "Two lines say their numbers in words\n\n  kept as typed"
	hash, err := git.CommitStaged(repo.Root, message)
	if err != nil || hash != repo.Git("rev-parse", "--short", "HEAD") {
		t.Fatalf("committed %q, %v", hash, err)
	}
	if got := repo.Git("log", "-1", "--format=%B"); got != message {
		t.Errorf("the message = %q", got)
	}
	if ahead := git.Ahead(repo.Root, "HEAD~1", 20); len(ahead) != 1 || ahead[0].SHA != hash || ahead[0].Subject != "Two lines say their numbers in words" {
		t.Errorf("ahead = %+v", ahead)
	}
	shown, err := git.Shown(repo.Root, hash)
	if err != nil || len(shown) != 1 || len(shown[0].Hunks) != 2 {
		t.Errorf("the commit's diff = %+v, %v", shown, err)
	}
}
