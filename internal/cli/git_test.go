package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

// gitHeard is a message of mia git as the window reads it.
type gitHeard struct {
	V      int        `json:"v"`
	Pane   string     `json:"pane"`
	Fill   bool       `json:"fill"`
	Status *muiStatus `json:"status"`
	View   *gitPiece  `json:"view"`
	Keys   []gitKey   `json:"keys"`
	Patch  []gitOp    `json:"patch"`
}

// heard is a message read back, which no message the window takes fails:
// in ASCII alone, and no longer than a message may be.
func heard(t *testing.T, message []byte) gitHeard {
	t.Helper()
	if len(message) > muiLimit {
		t.Fatalf("a message of %d bytes, over the %d the window takes", len(message), muiLimit)
	}
	if at := slices.IndexFunc(message, func(b byte) bool { return b >= 0x80 || b < 0x20 }); at >= 0 {
		t.Fatalf("byte %#x in a message: %.80q", message[at], message)
	}
	var got gitHeard
	if err := json.Unmarshal(message, &got); err != nil {
		t.Fatalf("%v in %.200s", err, message)
	}
	return got
}

// piece is the piece of an id in a view, and texts every text in it in order.
func piece(in *gitPiece, id string) *gitPiece {
	if in.ID == id {
		return in
	}
	for i := range in.Children {
		if found := piece(&in.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

func texts(in gitPiece) []string {
	var all []string
	if in.T == "text" {
		all = append(all, in.Text)
	}
	for _, child := range in.Children {
		all = append(all, texts(child)...)
	}
	return all
}

// code is the lines of the diff a view's messages say: those in the view,
// then those each patch puts after them.
func code(t *testing.T, messages [][]byte) []gitLine {
	t.Helper()
	var lines []gitLine
	read := func(raw json.RawMessage) {
		var line gitLine
		if err := json.Unmarshal(raw, &line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	for i, message := range messages {
		got := heard(t, message)
		if i == 0 {
			var first []json.RawMessage
			if raw := piece(got.View, "diff").Lines; raw != nil {
				if err := json.Unmarshal(raw, &first); err != nil {
					t.Fatal(err)
				}
			}
			for _, raw := range first {
				read(raw)
			}
			continue
		}
		if got.View != nil || len(got.Patch) != 1 || got.Patch[0].Op != "lines" || got.Patch[0].ID != "diff" {
			t.Fatalf("message %d is no patch of lines: %.200s", i, message)
		}
		for _, raw := range got.Patch[0].Lines {
			read(raw)
		}
	}
	return lines
}

func lineNumbered(from, to int, change map[int]string) string {
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

// The list has the files under Staged, Changed and Untracked, each head
// with its count, each file with its lines and its state and an id that
// says its group; then a rule and the commits ahead, newest first. A group
// with nothing is left out, and the rows are the items in order.
func TestTheListSaysEachGroupAndTheCommitsAhead(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	standing := git.Standing{
		Branch:    "mui-patch",
		Staged:    []git.Change{{Path: "src/mui/patch.rs", State: "new", Added: 96}, {Path: "src/mui.rs", Added: 31, Deleted: 8}},
		Changed:   []git.Change{{Path: "src/mui.rs", Added: 17, Deleted: 4}, {Path: "old.md", State: "deleted", Deleted: 9}},
		Untracked: []git.Change{{Path: "notes.md"}},
	}
	commits := []git.Commit{
		{SHA: "a41c9e2", Subject: "A list's item can be put in place by an op", When: now.Add(-2 * time.Hour)},
		{SHA: "19f2c6a", Subject: "", When: now.Add(-50 * time.Hour)},
	}
	entries, rows := gitList(standing, commits, "main", 2, gitRowsMost, now)
	want := []gitEntry{
		{Head: "Staged", Note: "2"},
		{ID: "s:src/mui/patch.rs", Title: "src/mui/patch.rs", Note: "new", Right: "+96"},
		{ID: "s:src/mui.rs", Title: "src/mui.rs", Right: "+31 -8"},
		{Head: "Changed", Note: "2"},
		{ID: "c:src/mui.rs", Title: "src/mui.rs", Right: "+17 -4"},
		{ID: "c:old.md", Title: "old.md", Note: "deleted", Right: "-9"},
		{Head: "Untracked", Note: "1"},
		{ID: "u:notes.md", Title: "notes.md"},
		{Rule: true},
		{Head: "Commits", Tag: "on mui-patch", Note: "2 ahead of main"},
		{ID: "k:a41c9e2", Title: "A list's item can be put in place by an op", Right: "2h"},
		{ID: "k:19f2c6a", Title: "19f2c6a", Right: "2d"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v\nwant      %+v", entries, want)
	}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.id)
	}
	if want := []string{"s:src/mui/patch.rs", "s:src/mui.rs", "c:src/mui.rs", "c:old.md", "u:notes.md", "k:a41c9e2", "k:19f2c6a"}; !slices.Equal(ids, want) {
		t.Errorf("rows = %v", ids)
	}

	entries, rows = gitList(git.Standing{Branch: "main", Changed: standing.Changed, Untracked: standing.Untracked}, nil, "", 0, 1, now)
	want = []gitEntry{{Head: "Changed", Note: "1 of 2 shown"}, {ID: "c:src/mui.rs", Title: "src/mui.rs", Right: "+17 -4"}, {Head: "Untracked", Note: "0 of 1 shown"}}
	if !reflect.DeepEqual(entries, want) || len(rows) != 1 {
		t.Errorf("with room for one file: %+v, %d rows", entries, len(rows))
	}
	if got := gitGroups(standing); got != "2 staged · 2 changed · 1 untracked" {
		t.Errorf("the groups = %q", got)
	}
	if got := gitGroups(git.Standing{Untracked: standing.Untracked}); got != "1 untracked" || gitGroups(git.Standing{}) != "clean" {
		t.Errorf("one group = %q, none = %q", got, gitGroups(git.Standing{}))
	}
	for _, c := range []struct {
		ahead, behind int
		base, want    string
	}{{3, 0, "main", "3 ahead of main, none behind"}, {0, 2, "main", "none ahead of main, 2 behind"}, {0, 0, "lower", "level with lower"}, {4, 1, "", ""}} {
		if got := gitStands(c.ahead, c.behind, c.base); got != c.want {
			t.Errorf("%d ahead and %d behind %q = %q, want %q", c.ahead, c.behind, c.base, got, c.want)
		}
	}
}

const gitTwoHunks = `diff --git a/src/talk.rs b/src/talk.rs
index 1111111..2222222 100644
--- a/src/talk.rs
+++ b/src/talk.rs
@@ -212,4 +212,5 @@ fn hear
 	let ui = self.uis.entry(pane);
-	ui.view = msg.view;
+	ui.take(msg);
+	ui.seen = true;
 	Heard::Drawn
 }
@@ -240,2 +241,3 @@
 fn patch() {
+	let next = self.view.clone();
 }
\ No newline at end of file
`

// A diff is lines of code: a head over each hunk that says which of how
// many it is, each line with its number in the old file and the new, a
// tab as spaces, and the lines git left out between two hunks as a gap.
// Each line knows the hunk it is in; a gap, and git's own note, are in none.
func TestADiffIsLinesOfCodeWithNumbersAndHunks(t *testing.T) {
	lines, hunks := gitCode(git.ParseDiff(gitTwoHunks))
	want := []gitLine{
		{Kind: "head", Text: "@@ fn hear", Right: "hunk 1 of 2"},
		{Kind: "same", Text: "    let ui = self.uis.entry(pane);", Old: 212, New: 212},
		{Kind: "del", Text: "    ui.view = msg.view;", Old: 213},
		{Kind: "add", Text: "    ui.take(msg);", New: 213},
		{Kind: "add", Text: "    ui.seen = true;", New: 214},
		{Kind: "same", Text: "    Heard::Drawn", Old: 214, New: 215},
		{Kind: "same", Text: "}", Old: 215, New: 216},
		{Kind: "gap", Text: "24 lines the same"},
		{Kind: "head", Text: "@@ line 241", Right: "hunk 2 of 2"},
		{Kind: "same", Text: "fn patch() {", Old: 240, New: 241},
		{Kind: "add", Text: "    let next = self.view.clone();", New: 242},
		{Kind: "same", Text: "}", Old: 241, New: 243},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("lines = %+v\nwant    %+v", lines, want)
	}
	first, second := gitHunk{0, 0}, gitHunk{0, 1}
	if want := []gitHunk{first, first, first, first, first, first, first, noHunk, second, second, second, second}; !slices.Equal(hunks, want) {
		t.Errorf("hunks = %v", hunks)
	}

	// a commit's diff: a head over each file, the hunks counted through
	// them all; a new file, a deleted one and a binary one say what they are
	lines, hunks = gitCode(git.ParseDiff(gitTwoHunks +
		"diff --git a/new.txt b/new.txt\nnew file mode 100644\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+one\n" +
		"diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-one\n" +
		"diff --git a/logo.png b/logo.png\nBinary files a/logo.png and b/logo.png differ\n"))
	var heads []gitLine
	for _, line := range lines {
		if line.Kind == "head" || line.Kind == "gap" {
			heads = append(heads, line)
		}
	}
	want = []gitLine{
		{Kind: "head", Text: "src/talk.rs"}, {Kind: "head", Text: "@@ fn hear", Right: "hunk 1 of 4"}, {Kind: "gap", Text: "24 lines the same"}, {Kind: "head", Text: "@@ line 241", Right: "hunk 2 of 4"},
		{Kind: "head", Text: "new.txt"}, {Kind: "head", Text: "@@ new file", Right: "hunk 3 of 4"},
		{Kind: "head", Text: "gone.txt"}, {Kind: "head", Text: "@@ deleted file", Right: "hunk 4 of 4"},
		{Kind: "head", Text: "logo.png"}, {Kind: "gap", Text: "a binary file"},
	}
	if !reflect.DeepEqual(heads, want) || hunks[len(hunks)-3] != (gitHunk{2, 0}) || len(hunks) != len(lines) {
		t.Errorf("heads = %+v", heads)
	}

	if got := gitShown("a\tbc\tdefg\tx\r"); got != "a   bc  defg    x" {
		t.Errorf("tabs as spaces = %q", got)
	}
	if got := gitShown(strings.Repeat("\t", 200) + "é"); len([]rune(got)) != gitLineMost {
		t.Errorf("a long line is kept to %d characters", len([]rune(got)))
	}
}

// An untracked file is its lines, each added; one with no byte and one
// that is no text say so. A code past its most lines ends in a gap that
// says how many more there are.
func TestAnUntrackedFileIsItsLinesAddedAndALongCodeIsCut(t *testing.T) {
	want := []gitLine{{Kind: "head", Text: "@@ new file", Right: "hunk 1 of 1"}, {Kind: "add", Text: "one", New: 1}, {Kind: "add", Text: "", New: 2}, {Kind: "add", Text: "three", New: 3}}
	for _, content := range []string{"one\n\nthree\n", "one\n\nthree"} {
		if got := gitAdded([]byte(content)); !reflect.DeepEqual(got, want) {
			t.Errorf("%q = %+v", content, got)
		}
	}
	if got := gitAdded(nil); len(got) != 1 || got[0] != (gitLine{Kind: "gap", Text: "an empty file"}) {
		t.Errorf("an empty file = %+v", got)
	}
	if got := gitAdded([]byte("PNG\x00\x01")); len(got) != 1 || got[0] != (gitLine{Kind: "gap", Text: "a binary file"}) {
		t.Errorf("a binary file = %+v", got)
	}

	lines := gitAdded([]byte(strings.Repeat("a line\n", 2000)))
	lines, hunks := gitCut(lines, slices.Repeat([]gitHunk{noHunk}, len(lines)), gitLinesMost)
	if len(lines) != gitLinesMost+1 || len(hunks) != len(lines) || lines[gitLinesMost] != (gitLine{Kind: "gap", Text: "501 more lines"}) {
		t.Errorf("%d lines, the last %+v", len(lines), lines[len(lines)-1])
	}
	if got, _ := gitCut(want, slices.Repeat([]gitHunk{noHunk}, len(want)), 3); got[3].Text != "1 more line" {
		t.Errorf("one line over = %+v", got[3])
	}
}

// The whole view is a UI that fills its pane: the branch and where it
// stands over a row split in two, the live list of files 360 points wide
// and the live code of the diff beside it, with the keys that stage,
// commit and ask again. Where the keys are is said only when asked.
func TestTheViewIsAListAndTheDiffSideBySide(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{pane: "%12", dir: "/src/app", top: "/src/app", base: "main", ahead: 3, at: "c:src/talk.rs",
		standing: git.Standing{Branch: "mui-patch", Head: "0123456789", Changed: []git.Change{{Path: "src/talk.rs", Added: 3, Deleted: 1}}, Untracked: []git.Change{{Path: "notes — née.md"}}}}
	view.entries, view.rows = gitList(view.standing, nil, "main", 3, gitRowsMost, now)
	view.lines, view.hunks = gitCode(git.ParseDiff(gitTwoHunks))
	view.added, view.deleted = 3, 1

	messages := view.messages(true)
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	got := heard(t, messages[0])
	if got.V != 1 || got.Pane != "%12" || !got.Fill || got.Status != nil || got.View == nil {
		t.Fatalf("message = %s", messages[0])
	}
	if want := []string{"mui-patch", "3 ahead of main, none behind", "1 changed · 1 untracked", "src/talk.rs", "changed, not staged", "+3 -1"}; !slices.Equal(texts(*got.View), want) {
		t.Errorf("texts = %q", texts(*got.View))
	}
	sides := got.View.Children[len(got.View.Children)-1]
	if sides.T != "row" || !sides.Split || sides.Grow != 1 || len(sides.Children) != 3 || sides.Children[1].T != "divider" {
		t.Fatalf("the sides = %+v", sides)
	}
	if left := sides.Children[0]; left.T != "col" || left.W != 360 || !left.Scroll {
		t.Errorf("the left side = %+v", left)
	}
	list := piece(got.View, "files")
	if list == nil || list.T != "list" || !list.Live || list.Selected != "c:src/talk.rs" || len(list.Items) != 4 || list.Items[3].ID != "u:notes — née.md" {
		t.Errorf("the list = %+v", list)
	}
	if diff := piece(got.View, "diff"); diff == nil || diff.T != "code" || !diff.Live {
		t.Errorf("the code = %+v", diff)
	}
	if lines := code(t, messages); !reflect.DeepEqual(lines, view.lines) {
		t.Errorf("the code's lines = %+v", lines)
	}
	if want := []gitKey{{"space", "stage", "stage"}, {"a", "stage all", "all"}, {"c", "commit", "commit"}, {"r", "ask again", "again"}}; !slices.Equal(got.Keys, want) {
		t.Errorf("keys = %+v", got.Keys)
	}

	// said after the person moved: the keys stay where they put them
	if list := piece(heard(t, view.messages(false)[0]).View, "files"); list.Selected != "" {
		t.Errorf("selected = %q, with no place said", list.Selected)
	}
	// on a staged file space unstages, and a failure is the status
	view.at, view.failed = "s:src/talk.rs", "fatal: not a git repository"
	got = heard(t, view.messages(false)[0])
	if got.Keys[0] != (gitKey{"space", "unstage", "stage"}) || got.Status == nil || *got.Status != (muiStatus{"fatal: not a git repository", "bad"}) {
		t.Errorf("keys = %+v, status = %+v", got.Keys, got.Status)
	}
}

// A diff longer than a message is said by the first of its lines that fit
// in the view and the rest in patches of lines, each a message the window
// takes, and all of them in order.
func TestADiffLongerThanAMessageIsSaidOnInPatches(t *testing.T) {
	view := &gitView{pane: "%12", dir: "/src/app", top: "/src/app", standing: git.Standing{Branch: "main"}}
	long := strings.Repeat("é", gitLineMost) // six bytes a character, said in ASCII
	view.lines = gitAdded([]byte(strings.Repeat(long+"\n", 3000)))
	view.lines, view.hunks = gitCut(view.lines, slices.Repeat([]gitHunk{noHunk}, len(view.lines)), gitLinesMost)

	messages := view.messages(true)
	if len(messages) < 3 {
		t.Fatalf("%d messages for %d lines of %d bytes", len(messages), len(view.lines), 6*gitLineMost)
	}
	first := heard(t, messages[0])
	if first.View == nil || len(messages[0]) < muiLimit/2 {
		t.Errorf("the view is %d bytes, with room for more lines", len(messages[0]))
	}
	lines := code(t, messages)
	if !reflect.DeepEqual(lines, view.lines) || lines[len(lines)-1] != (gitLine{Kind: "gap", Text: "1501 more lines"}) {
		t.Errorf("%d lines said of %d, the last %+v", len(lines), len(view.lines), lines[len(lines)-1])
	}
	if n := gitFit([]json.RawMessage{[]byte(`{"a":1}`), []byte(`{"b":2}`), []byte(`{"c":3}`)}, 16); n != 2 {
		t.Errorf("%d lines fit 16 bytes, want 2", n)
	}
}

// What an event does: a row come under the keys shows its diff; space on
// a file stages it or unstages a staged one, on a line of the diff the
// hunk that line is in, and nothing on a commit or a line in no hunk; the
// other keys and the commit screen's field and button do their own.
func TestWhatAnEventDoes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{at: "c:src/talk.rs", message: "typed so far", standing: git.Standing{
		Staged:    []git.Change{{Path: "src/talk.rs"}},
		Changed:   []git.Change{{Path: "src/talk.rs"}},
		Untracked: []git.Change{{Path: "notes.md"}},
	}}
	view.entries, view.rows = gitList(view.standing, []git.Commit{{SHA: "a41c9e2", Subject: "A subject", When: now}}, "main", 1, gitRowsMost, now)
	view.files = git.ParseDiff(gitTwoHunks)
	view.lines, view.hunks = gitCode(view.files)
	second := view.files[0].Patch(1)

	for _, c := range []struct {
		event string
		do    gitDo
		row   string
		patch string
		text  string
	}{
		{`{"e":"close"}`, gitLeave, "", "", ""},
		{`{"e":"move","list":"files","id":"u:notes.md"}`, gitMove, "u:notes.md", "", ""},
		{`{"e":"move","list":"files","id":"c:src/talk.rs"}`, gitNothing, "", "", ""}, // where the keys are
		{`{"e":"move","list":"files","id":"c:gone.rs"}`, gitNothing, "", "", ""},
		{`{"e":"move","list":"diff","id":"3"}`, gitNothing, "", "", ""},
		{`{"e":"press","id":"stage","item":"c:src/talk.rs"}`, gitStage, "c:src/talk.rs", "", ""},
		{`{"e":"press","id":"stage","item":"u:notes.md"}`, gitStage, "u:notes.md", "", ""},
		{`{"e":"press","id":"stage","item":"s:src/talk.rs"}`, gitUnstage, "s:src/talk.rs", "", ""},
		{`{"e":"press","id":"stage","item":"k:a41c9e2"}`, gitNothing, "", "", ""},
		{`{"e":"press","id":"stage","item":"10"}`, gitStageHunk, "c:src/talk.rs", second, ""},
		{`{"e":"press","id":"stage","item":"8"}`, gitStageHunk, "c:src/talk.rs", second, ""}, // the hunk's head
		{`{"e":"press","id":"stage","item":"7"}`, gitNothing, "", "", ""},                    // the gap between two
		{`{"e":"press","id":"stage","item":"12"}`, gitNothing, "", "", ""},                   // past the last line
		{`{"e":"press","id":"stage"}`, gitNothing, "", "", ""},
		{`{"e":"press","id":"all","item":"c:src/talk.rs"}`, gitStageAll, "", "", ""},
		{`{"e":"press","id":"commit"}`, gitWrite, "", "", ""},
		{`{"e":"press","id":"back"}`, gitBack, "", "", ""},
		{`{"e":"press","id":"again","item":"3"}`, gitAgain, "", "", ""},
		{`{"e":"change","id":"message","text":"typed"}`, gitTyped, "", "", "typed"},
		{`{"e":"submit","id":"message","text":"Sent\nas typed"}`, gitCommit, "", "", "Sent\nas typed"},
		{`{"e":"press","id":"send"}`, gitCommit, "", "", "typed so far"},
		{`{"e":"pick","list":"files","id":"u:notes.md"}`, gitNothing, "", "", ""},
	} {
		events, rest, _, leave := muiTyped[gitEvent]("", "\033[200~\x1fmia:"+c.event+"\033[201~")
		if len(events) != 1 || rest != "" || leave {
			t.Fatalf("%s is read as %+v, kept %q", c.event, events, rest)
		}
		if got := view.deed(events[0]); got.do != c.do || got.row.id != c.row || got.patch != c.patch || got.text != c.text {
			t.Errorf("%s does %d on %q with %q %q", c.event, got.do, got.row.id, got.patch, got.text)
		}
	}

	// in the diff of a staged file space unstages the hunk, in an
	// untracked file's it stages the file, in a commit's it does nothing
	for at, want := range map[string]gitDo{"s:src/talk.rs": gitUnstageHunk, "u:notes.md": gitStage, "k:a41c9e2": gitNothing} {
		view.at = at
		if got := view.deed(gitEvent{E: "press", ID: "stage", Item: "2"}); got.do != want || want != gitNothing && got.row.id != at {
			t.Errorf("space in the diff of %s does %d on %q", at, got.do, got.row.id)
		}
	}
}

// Read again, the keys stay on their row; a file that left its group is
// followed into the one it is in now; and where it is nowhere the keys are
// on the row that came after it, or the last.
func TestTheKeysStayOnTheSameFileElseTheNext(t *testing.T) {
	rows := func(ids ...string) []gitRow {
		var all []gitRow
		for _, id := range ids {
			all = append(all, gitRow{id: id})
		}
		return all
	}
	was := []string{"s:a", "c:b", "c:c", "u:d", "k:0123abc"}
	for _, c := range []struct {
		at   string
		now  []gitRow
		want string
	}{
		{"c:b", rows("s:a", "c:b", "c:c"), "c:b"},
		{"c:b", rows("s:a", "s:b", "c:c", "u:d"), "s:b"},
		{"s:a", rows("c:a", "c:b"), "c:a"},
		{"s:a", rows("c:b", "u:a"), "u:a"},
		{"c:b", rows("s:a", "c:c", "u:d"), "c:c"},
		{"u:d", rows("s:a", "c:b"), "c:b"},
		{"k:0123abc", rows("s:a", "c:b", "k:4567def"), "k:4567def"},
		{"", rows("s:a", "c:b"), "s:a"},
		{"c:b", nil, ""},
	} {
		if got := gitKept(c.at, was, c.now); got != c.want {
			t.Errorf("from %q among %v: %q, want %q", c.at, c.now, got, c.want)
		}
	}
}

// press is what the view does when the window says a key was pressed on a
// row or a line; it fails the test when the view is not to be said again.
func press(t *testing.T, view *gitView, id, item string) {
	t.Helper()
	if say, _ := view.act(view.deed(gitEvent{E: "press", ID: id, Item: item}), time.Now()); !say {
		t.Fatalf("%s on %q changes nothing said", id, item)
	}
}

func ids(view *gitView) []string {
	var all []string
	for _, row := range view.rows {
		all = append(all, row.id)
	}
	return all
}

// In a real repository: space stages a changed file and unstages it
// again, the keys following the file; a stages every file; an untracked
// file's diff is its lines added.
func TestSpaceStagesAndUnstagesAFile(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", lineNumbered(1, 30, nil))
	repo.Commit("a file")
	repo.Write("talk.txt", lineNumbered(1, 30, map[int]string{3: "three"}))
	repo.Write("notes/new.txt", "one\n\ttwo\n")

	view := &gitView{pane: "%3", dir: filepath.Join(repo.Root, "notes")}
	if moved := view.read(time.Now()); !moved || view.failed != "" || !slices.Equal(ids(view), []string{"c:talk.txt", "u:notes/new.txt"}) || view.at != "c:talk.txt" {
		t.Fatalf("rows = %v, on %q, %q", ids(view), view.at, view.failed)
	}
	if view.added != 1 || view.deleted != 1 || len(view.lines) != 8 || view.lines[0] != (gitLine{Kind: "head", Text: "@@ line 1", Right: "hunk 1 of 1"}) {
		t.Errorf("the changed file's diff = +%d -%d, %+v", view.added, view.deleted, view.lines)
	}

	press(t, view, "stage", "c:talk.txt")
	if !slices.Equal(ids(view), []string{"s:talk.txt", "u:notes/new.txt"}) || view.at != "s:talk.txt" || view.status != nil {
		t.Fatalf("staged: rows = %v, on %q, %+v", ids(view), view.at, view.status)
	}
	if got := repo.Git("diff", "--cached", "--name-only"); got != "talk.txt" {
		t.Errorf("git has staged %q", got)
	}
	press(t, view, "stage", "s:talk.txt")
	if !slices.Equal(ids(view), []string{"c:talk.txt", "u:notes/new.txt"}) || view.at != "c:talk.txt" || repo.Git("diff", "--cached", "--name-only") != "" {
		t.Fatalf("unstaged: rows = %v, on %q", ids(view), view.at)
	}

	if say, placed := view.act(view.deed(gitEvent{E: "move", List: "files", ID: "u:notes/new.txt"}), time.Now()); !say || placed {
		t.Fatal("the row under the keys moved, and its diff is not said")
	}
	want := []gitLine{{Kind: "head", Text: "@@ new file", Right: "hunk 1 of 1"}, {Kind: "add", Text: "one", New: 1}, {Kind: "add", Text: "    two", New: 2}}
	if !reflect.DeepEqual(view.lines, want) || view.added != 2 {
		t.Errorf("the untracked file's diff = %+v", view.lines)
	}
	if got := texts(*heard(t, view.messages(true)[0]).View); !slices.Equal(got, []string{"main", "1 changed · 1 untracked", "notes/new.txt", "untracked", "+2"}) {
		t.Errorf("texts = %q", got)
	}

	press(t, view, "all", "")
	if !slices.Equal(ids(view), []string{"s:notes/new.txt", "s:talk.txt"}) || view.at != "s:notes/new.txt" {
		t.Errorf("all staged: rows = %v, on %q", ids(view), view.at)
	}
}

// In a real repository: space on a line of a changed file's diff stages
// the hunk that line is in and leaves the other changed; on a line of the
// staged diff it unstages it again.
func TestSpaceInTheDiffStagesAndUnstagesOneHunkOfTwo(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", lineNumbered(1, 30, nil))
	repo.Commit("a file")
	repo.Write("talk.txt", lineNumbered(1, 30, map[int]string{3: "three", 28: "twenty-eight"}))

	view := &gitView{dir: repo.Root}
	view.read(time.Now())
	in := slices.IndexFunc(view.lines, func(line gitLine) bool { return line.Text == "twenty-eight" })
	if in < 0 || view.lines[in].New != 28 || view.hunks[in] != (gitHunk{0, 1}) {
		t.Fatalf("the diff = %+v", view.lines)
	}
	press(t, view, "stage", fmt.Sprint(in))
	if !slices.Equal(ids(view), []string{"s:talk.txt", "c:talk.txt"}) || view.at != "c:talk.txt" || view.status != nil {
		t.Fatalf("rows = %v, on %q, %+v", ids(view), view.at, view.status)
	}
	if got := repo.Git("diff", "--cached", "--unified=0"); !strings.Contains(got, "+twenty-eight") || strings.Contains(got, "+three") {
		t.Errorf("staged:\n%s", got)
	}
	if slices.ContainsFunc(view.lines, func(line gitLine) bool { return line.Text == "twenty-eight" }) || view.lines[0].Right != "hunk 1 of 1" {
		t.Errorf("still changed: %+v", view.lines)
	}

	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "s:talk.txt"}), time.Now())
	in = slices.IndexFunc(view.lines, func(line gitLine) bool { return line.Text == "twenty-eight" })
	if in < 0 {
		t.Fatalf("the staged diff = %+v", view.lines)
	}
	press(t, view, "stage", fmt.Sprint(in))
	if !slices.Equal(ids(view), []string{"c:talk.txt"}) || view.at != "c:talk.txt" || repo.Git("diff", "--cached") != "" {
		t.Errorf("unstaged: rows = %v, on %q, staged %q", ids(view), view.at, repo.Git("diff", "--cached", "--name-only"))
	}
	if !strings.Contains(repo.Git("diff", "--unified=0"), "+twenty-eight") {
		t.Error("the hunk unstaged is no longer in the worktree")
	}

	// a hunk that is no longer what the file holds is refused in git's words
	stale := view.deed(gitEvent{E: "press", ID: "stage", Item: "2"})
	repo.Write("talk.txt", lineNumbered(1, 30, map[int]string{2: "two", 3: "3", 4: "four"}))
	repo.Git("add", "talk.txt")
	view.act(stale, time.Now())
	if view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "patch") {
		t.Errorf("status = %+v", view.status)
	}
}

// In a real repository: c brings the commit screen up, with the message's
// field and the Commit button over the staged diff, and esc goes back and
// keeps the words. A commit with nothing staged, or with no message, does
// nothing and says why; one made is said by its hash, back on the files;
// and a hook that fails is said by its first line.
func TestACommitIsMadeOfWhatIsStagedWithTheMessageAsTyped(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Git("checkout", "-q", "-b", "words")
	repo.Write("talk.txt", "one\n")
	repo.Write("notes.txt", "kept out\n")
	head := repo.Git("rev-parse", "HEAD")

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	if say, placed := view.act(view.deed(gitEvent{E: "press", ID: "commit"}), time.Now()); !say || !placed || !view.writing {
		t.Fatal("c does not bring the commit screen up")
	}
	view.act(view.deed(gitEvent{E: "change", ID: "message", Text: "A file says one"}), time.Now())
	press(t, view, "send", "")
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "nothing is staged") || !view.writing || repo.Git("rev-parse", "HEAD") != head {
		t.Fatalf("a commit of nothing staged: %+v", view.status)
	}

	press(t, view, "stage", "u:talk.txt")
	if view.status != nil || view.at != "u:notes.txt" {
		t.Fatalf("status = %+v, on %q", view.status, view.at)
	}
	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "s:talk.txt"}), time.Now())
	got := heard(t, view.messages(true)[0])
	if want := []string{"words", "level with main", "1 staged · 1 untracked", "Commit to words", "1 file staged", "+1", "1 untracked stay out", "talk.txt", "staged, new file · what goes in", "+1"}; !slices.Equal(texts(*got.View), want) {
		t.Errorf("texts = %q", texts(*got.View))
	}
	area, button := piece(got.View, "message"), piece(got.View, "send")
	if area == nil || area.T != "field" || string(area.Lines) != "6" || !area.Live || area.Value == nil || *area.Value != "A file says one" {
		t.Errorf("the message's field = %+v", area)
	}
	if button == nil || button.T != "button" || button.Label != "Commit" || !button.Primary || piece(got.View, "files") == nil || piece(got.View, "diff") == nil {
		t.Errorf("the button = %+v", button)
	}
	if want := []gitKey{{"space", "unstage", "stage"}, {"a", "stage all", "all"}, {"r", "ask again", "again"}, {"esc", "back to the files", "back"}}; !slices.Equal(got.Keys, want) {
		t.Errorf("keys = %+v", got.Keys)
	}
	if area := piece(heard(t, view.messages(false)[0]).View, "message"); area.Value != nil {
		t.Errorf("the field is said %q over what is being typed", *area.Value)
	}

	press(t, view, "back", "")
	if view.writing || view.message != "A file says one" {
		t.Fatalf("esc: writing %v, the words %q", view.writing, view.message)
	}
	press(t, view, "commit", "")
	view.act(view.deed(gitEvent{E: "submit", ID: "message", Text: " \n"}), time.Now())
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "no message") || repo.Git("rev-parse", "HEAD") != head {
		t.Fatalf("a commit with no message: %+v", view.status)
	}

	hook := filepath.Join(repo.Root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'the linter found 2 problems' >&2\necho 'and says more' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	const message = "A file says one\n\nAnd nothing else — as typed."
	view.act(view.deed(gitEvent{E: "submit", ID: "message", Text: message}), time.Now())
	if view.status == nil || *view.status != (muiStatus{"the linter found 2 problems", "bad"}) || !view.writing || repo.Git("rev-parse", "HEAD") != head {
		t.Fatalf("a hook that fails: %+v", view.status)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}

	view.act(view.deed(gitEvent{E: "submit", ID: "message", Text: message}), time.Now())
	hash := repo.Git("rev-parse", "--short", "HEAD")
	if view.status == nil || *view.status != (muiStatus{"committed " + hash, "ok"}) || view.writing || view.message != "" {
		t.Fatalf("committed: %+v, writing %v", view.status, view.writing)
	}
	if got := repo.Git("log", "-1", "--format=%B"); got != message {
		t.Errorf("the message = %q", got)
	}
	if !slices.Equal(ids(view), []string{"u:notes.txt", "k:" + hash}) || view.at != "u:notes.txt" || view.ahead != 1 {
		t.Errorf("rows = %v, on %q, %d ahead", ids(view), view.at, view.ahead)
	}
	got = heard(t, view.messages(true)[0])
	if want := []string{"words", "1 ahead of main, none behind", "1 untracked", "notes.txt", "untracked", "+1"}; !slices.Equal(texts(*got.View), want) || got.Status == nil || got.Status.Tone != "ok" {
		t.Errorf("texts = %q, status %+v", texts(*got.View), got.Status)
	}
	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "k:" + hash}), time.Now())
	if got := texts(*heard(t, view.messages(true)[0]).View); !slices.Equal(got[3:], []string{hash, "A file says one", "+1"}) {
		t.Errorf("the commit's line = %q", got[3:])
	}
}

// A branch stands against the layer under it in a stack mia knows, else
// against the repository's main branch.
func TestABranchStandsAgainstTheLayerUnderIt(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Git("checkout", "-q", "-b", "lower")
	repo.Write("a.txt", "a\n")
	repo.Commit("lower's")
	repo.Git("checkout", "-q", "-b", "upper")
	repo.Write("b.txt", "b\n")
	repo.Commit("upper's")

	view := &gitView{dir: repo.Root}
	view.read(time.Now())
	if view.base != "main" || view.ahead != 2 || view.behind != 0 || !slices.Equal(ids(view), []string{"k:" + repo.Git("rev-parse", "--short", "HEAD"), "k:" + repo.Git("rev-parse", "--short", "HEAD~1")}) {
		t.Fatalf("in no stack: against %q, %d ahead, rows %v", view.base, view.ahead, ids(view))
	}
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Stacks().Record("upper", "lower"); err != nil {
		t.Fatal(err)
	}
	view.read(time.Now())
	if view.base != "lower" || view.ahead != 1 || len(view.rows) != 1 {
		t.Errorf("in a stack: against %q, %d ahead, rows %v", view.base, view.ahead, ids(view))
	}
	repo.Git("checkout", "-q", "main")
	view.read(time.Now())
	if view.base != "" || len(view.rows) != 0 || view.failed != "" {
		t.Errorf("on main: against %q, rows %v, %q", view.base, ids(view), view.failed)
	}
}

// A folder in no repository is shown as that: the view stays up with why
// as its status, and what would change the standing says git's refusal.
// What the poll looks at changes when a file does, and not otherwise.
func TestAFolderInNoRepositoryIsShownAsThat(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	view := &gitView{pane: "%3", dir: dir}
	view.read(time.Now())
	got := heard(t, view.messages(true)[0])
	if got.Status == nil || got.Status.Tone != "bad" || !strings.Contains(got.Status.Text, "not a git repository") || len([]rune(got.Status.Text)) > gitWordsMost {
		t.Fatalf("status = %+v", got.Status)
	}
	if want := []string{filepath.Base(dir), "clean", "nothing to show"}; !slices.Equal(texts(*got.View), want) || piece(got.View, "files") == nil {
		t.Errorf("texts = %q", texts(*got.View))
	}
	t.Chdir(gittest.New(t).Root) // git is run in the folder the view is of, never in the one mia is in
	press(t, view, "all", "")
	if view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "not a git repository") {
		t.Errorf("stage all in no repository: %+v", view.status)
	}

	repo := gittest.New(t)
	repo.Write("talk.txt", "one\n")
	view = &gitView{dir: repo.Root}
	view.read(time.Now())
	mark := gitMark(repo.Root, view.top)
	if again := gitMark(repo.Root, view.top); again != mark {
		t.Error("the mark changed, and nothing did")
	}
	repo.Write("talk.txt", "one\ntwo\n")
	if again := gitMark(repo.Root, view.top); again == mark {
		t.Error("an untracked file changed, and the mark did not")
	}
}

// mia git reads what the person does from a terminal, and is refused
// where stdin is none; it takes no argument, and needs no repository.
func TestGitRefusesWhatIsNotATerminal(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	stdin := os.Stdin
	os.Stdin = null
	defer func() { os.Stdin = stdin }()
	if code := Run([]string{"git"}); code != exitFailed {
		t.Errorf("git exited %d, want %d", code, exitFailed)
	}
	if code := Run([]string{"git", "status"}); code != exitUsage {
		t.Errorf("git status exited %d, want %d", code, exitUsage)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Features, "git-app") {
		t.Error("capabilities do not list git-app")
	}
}
