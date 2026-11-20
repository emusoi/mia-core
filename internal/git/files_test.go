package git

import (
	"maps"
	"testing"
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
