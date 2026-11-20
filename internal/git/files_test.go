package git

import (
	"maps"
	"slices"
	"testing"
	"time"
)

// What git diff --name-status -z writes is each file with its mark: A
// where it is added, D where it is deleted, and M where it is changed in
// any other way, a change of its type too.
func TestAgainstIsEachFileWithItsMark(t *testing.T) {
	got := ParseAgainst("M\x00src/mui.rs\x00A\x00src/mui/patch — née.rs\x00D\x00old.md\x00T\x00link\x00")
	want := map[string]byte{"src/mui.rs": 'M', "src/mui/patch — née.rs": 'A', "old.md": 'D', "link": 'M'}
	if !maps.Equal(got, want) {
		t.Errorf("marks = %v", got)
	}
	if got := ParseAgainst(""); len(got) != 0 {
		t.Errorf("of nothing: %v", got)
	}
}

// What git blame --porcelain writes is each line of the file with its
// commit and when that was written: a commit's time is said once, the
// first time it is named, and a line not yet committed has no commit.
func TestBlameIsEachLineWithItsCommit(t *testing.T) {
	first, second, none := "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d", "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432", "0000000000000000000000000000000000000000"
	out := first + " 1 1 2\nauthor testuser1\nauthor-time 1700000000\nsummary the first\nfilename a.txt\n\tone\n" +
		first + " 2 2\n\ttwo\n" +
		second + " 1 3 1\nauthor testuser1\nauthor-time 1700003600\nsummary author-time 5\nfilename a.txt\n\tauthor-time 9\n" +
		none + " 4 4 1\nauthor Not Committed Yet\nauthor-time 1700007200\nfilename a.txt\n\tfour\n"
	want := []Blamed{{first, time.Unix(1700000000, 0)}, {first, time.Unix(1700000000, 0)}, {second, time.Unix(1700003600, 0)}, {}}
	if got := ParseBlame(out); !slices.Equal(got, want) {
		t.Errorf("blamed = %+v", got)
	}
	if got := ParseBlame(""); len(got) != 0 {
		t.Errorf("of nothing: %+v", got)
	}
}
