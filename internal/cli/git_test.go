package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
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
	Focus  string     `json:"focus"`
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
	// (The right side is said again whole on every move: it comes in at once.)
	if right := sides.Children[2]; right.ID != "side" || right.Motion != "none" {
		t.Errorf("the right side = %+v", right)
	}
	list := piece(got.View, "files")
	if list == nil || list.T != "list" || !list.Live || list.Selected != "c:src/talk.rs" || len(list.Items) != 4 || list.Items[3].ID != "u:notes — née.md" || list.Menu == "" {
		t.Errorf("the list = %+v", list)
	}
	if diff := piece(got.View, "diff"); diff == nil || diff.T != "code" || !diff.Live || diff.Does != "stage line" {
		t.Errorf("the code = %+v", diff)
	}
	if lines := code(t, messages); !reflect.DeepEqual(lines, view.lines) {
		t.Errorf("the code's lines = %+v", lines)
	}
	t.Setenv("EDITOR", "ed -s")
	if want := []gitKey{{"space", "stage", "stage"}, {"d", "discard", "discard"}, {"e", "edit in ed", "edit"}, {"y", "copy the path", "copy"}, {"a", "stage all", "all"}, {"c", "commit", "commit"}, {"b", "branches", "branches"}, {"s", "stash", "stash"}, {"f", "fetch", "fetch"}, {"P", "pull", "pull"}, {"p", "push", "push"}, {"r", "ask again", "again"}}; !slices.Equal(heard(t, view.messages(true)[0]).Keys, want) {
		t.Errorf("keys = %+v", heard(t, view.messages(true)[0]).Keys)
	}

	// said after the person moved: the keys stay where they put them
	if list := piece(heard(t, view.messages(false)[0]).View, "files"); list.Selected != "" {
		t.Errorf("selected = %q, with no place said", list.Selected)
	}
	// on a staged file space unstages, and a failure is the status
	view.rows = append(view.rows, gitRow{id: "s:src/talk.rs", group: 's'})
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
		// a choice of a row's menu does what the key of its id does on that row
		{`{"e":"pick","list":"files","id":"u:notes.md","sub":"stage"}`, gitStage, "u:notes.md", "", ""},
		{`{"e":"pick","list":"files","id":"s:src/talk.rs","sub":"stage"}`, gitUnstage, "s:src/talk.rs", "", ""},
		{`{"e":"pick","list":"files","id":"c:src/talk.rs","sub":"copy"}`, gitCopy, "", "", "src/talk.rs"},
		{`{"e":"pick","list":"files","id":"k:a41c9e2","sub":"copy"}`, gitCopy, "", "", "a41c9e2"},
		{`{"e":"press","id":"copy","item":"3"}`, gitNothing, "", "", ""}, // on a line of the diff
		{`{"e":"pick","list":"files","id":"c:src/talk.rs","sub":"edit"}`, gitEdit, "c:src/talk.rs", "", ""},
		{`{"e":"press","id":"edit","item":"k:a41c9e2"}`, gitNothing, "", "", ""},
		// where the window says the keys are, and what a modal is answered with
		{`{"e":"focus","id":"diff"}`, gitFocus, "", "", "diff"},
		{`{"e":"focus","id":"files"}`, gitFocus, "", "", "files"},
		{`{"e":"focus","id":"yes"}`, gitNothing, "", "", ""},
		{`{"e":"dismiss","id":"sure"}`, gitKeep, "", "", ""},
		{`{"e":"press","id":"no"}`, gitKeep, "", "", ""},
		{`{"e":"press","id":"yes"}`, gitNothing, "", "", ""},                  // nothing is asked
		{`{"e":"pick","list":"tabs","id":"commits"}`, gitNothing, "", "", ""}, // so few files have no tabs
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
	// e on a line of the diff edits the file at that line as it is now, a
	// deleted line's being the line after it; a deleted file has nothing to edit
	view.at = "c:src/talk.rs"
	for item, want := range map[string]int{"c:src/talk.rs": 1, "1": 212, "2": 213, "3": 213, "10": 242} {
		if got := view.deed(gitEvent{E: "press", ID: "edit", Item: item}); got.do != gitEdit || got.line != want || got.row.change.Path != "src/talk.rs" {
			t.Errorf("e on %s edits line %d of %q", item, got.line, got.row.change.Path)
		}
	}
	view.rows[1].change.State = "deleted"
	if got := view.deed(gitEvent{E: "press", ID: "edit", Item: "c:src/talk.rs"}); got.do != gitRefuse || got.text != "src/talk.rs is deleted: there is nothing to edit" {
		t.Errorf("e on a deleted file: %+v", got)
	}
}

// Each file, commit and stash carries the menu of what can be done to it,
// each choice by the id of the key that does the same with that key at
// its right: a staged file is unstaged, one that is not is staged and
// discarded, an untracked one deleted, each handed to the editor unless
// it is deleted and its path copied; a commit's hash is copied and the
// last one undone where it can be; a stash is applied, popped and dropped.
func TestEachRowCarriesTheMenuOfWhatCanBeDoneToIt(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	standing := git.Standing{Branch: "talk",
		Staged:    []git.Change{{Path: "a.rs"}},
		Changed:   []git.Change{{Path: "b.rs"}, {Path: "old.md", State: "deleted"}},
		Untracked: []git.Change{{Path: "c.rs"}},
	}
	menus := func(undo bool) map[string]string {
		entries, rows := gitList(standing, []git.Commit{{SHA: "a41c9e2", When: now}, {SHA: "19f2c6a", When: now}}, "main", 2, gitRowsMost, now)
		entries, rows = gitStashed(entries, rows, []git.Stash{{SHA: "5e6f7a8b9c0d1e2f", Subject: "On talk: earlier"}}, now)
		all := map[string]string{}
		for _, entry := range gitMenus(entries, rows, undo) {
			var choices []string
			for _, choice := range entry.Menu {
				choices = append(choices, choice.ID+"="+choice.Title+" "+choice.Right)
			}
			if entry.ID == "" && len(choices) > 0 {
				t.Errorf("a head or a rule has a menu: %+v", entry)
			}
			all[entry.ID] = strings.Join(choices, ", ")
		}
		return all
	}
	for id, want := range map[string]string{
		"s:a.rs":       "stage=Unstage space, edit=Open in editor e, copy=Copy path y",
		"c:b.rs":       "stage=Stage space, discard=Discard… d, edit=Open in editor e, copy=Copy path y",
		"c:old.md":     "stage=Stage space, discard=Discard… d, copy=Copy path y",
		"u:c.rs":       "stage=Stage space, discard=Delete… d, edit=Open in editor e, copy=Copy path y",
		"k:a41c9e2":    "copy=Copy its hash y, undo=Undo… u",
		"k:19f2c6a":    "copy=Copy its hash y",
		"t:5e6f7a8b9c": "stage=Apply space, push=Pop p, discard=Drop… d",
	} {
		if got := menus(true)[id]; got != want {
			t.Errorf("the menu of %s = %q", id, got)
		}
	}
	if got := menus(false)["k:a41c9e2"]; got != "copy=Copy its hash y" {
		t.Errorf("the menu of a last commit a remote has = %q", got)
	}
}

// With more files than a screen of them the list is behind tabs, Changes,
// Commits and Stashes, each with its count, and shows the chosen one's
// groups alone; a tab picked puts the keys of the list on its first row,
// and the tab follows the row under the keys. With fewer there are none.
func TestALongListIsBehindTabs(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{pane: "%3", base: "main", ahead: 2, standing: git.Standing{Branch: "talk", Head: "0123456789"}}
	for n := range gitTabsOver {
		view.standing.Changed = append(view.standing.Changed, git.Change{Path: fmt.Sprintf("src/%02d.rs", n)})
	}
	commits := []git.Commit{{SHA: "a41c9e2", Subject: "A subject", When: now}, {SHA: "19f2c6a", Subject: "Another", When: now}}
	list := func() {
		view.entries, view.rows = gitList(view.standing, commits, "main", 2, gitRowsMost, now)
		view.entries, view.rows = gitStashed(view.entries, view.rows, view.stashes, now)
	}
	list()
	view.at, view.tab = "c:src/00.rs", "changes"
	if got := heard(t, view.messages(true)[0]); view.tabs() != nil || piece(got.View, "tabs") != nil || len(piece(got.View, "files").Items) != len(view.entries) {
		t.Fatalf("%d files are behind tabs: %+v", gitTabsOver, view.tabs())
	}

	view.standing.Untracked = []git.Change{{Path: "notes.md"}}
	list()
	if want := []gitEntry{{ID: "changes", Title: "Changes", Note: "31"}, {ID: "commits", Title: "Commits", Note: "2"}}; !reflect.DeepEqual(view.tabs(), want) {
		t.Fatalf("tabs = %+v", view.tabs())
	}
	view.stashes = []git.Stash{{SHA: "5e6f7a8b9c0d1e2f", Subject: "On talk: earlier"}}
	list()
	got := heard(t, view.messages(true)[0])
	tabs, files := piece(got.View, "tabs"), piece(got.View, "files")
	if tabs == nil || tabs.T != "tabs" || tabs.Selected != "changes" || len(tabs.Items) != 3 || !reflect.DeepEqual(tabs.Items[2], gitEntry{ID: "stashes", Title: "Stashes", Note: "1"}) {
		t.Fatalf("the tabs = %+v", tabs)
	}
	if len(files.Items) != 33 || files.Items[0].Head != "Changed" || files.Items[31].Head != "Untracked" || files.Items[32].ID != "u:notes.md" {
		t.Errorf("behind Changes: %d entries, %+v", len(files.Items), files.Items[len(files.Items)-1])
	}
	for tab, want := range map[string][]string{"commits": {"Commits", "k:a41c9e2", "k:19f2c6a"}, "stashes": {"Stashes", "t:5e6f7a8b9c"}} {
		var behind []string
		for _, entry := range gitBehind(view.entries, tab) {
			behind = append(behind, entry.Head+entry.ID)
		}
		if !slices.Equal(behind, want) {
			t.Errorf("behind %s: %q", tab, behind)
		}
	}

	deed := view.deed(gitEvent{E: "pick", List: "tabs", ID: "commits"})
	if deed.do != gitTab || deed.row.id != "k:a41c9e2" || deed.text != "commits" {
		t.Fatalf("a tab picked: %+v", deed)
	}
	if say, placed := view.act(deed, now); !say || !placed || view.tab != "commits" || view.at != "k:a41c9e2" {
		t.Fatalf("on %q behind %q", view.at, view.tab)
	}
	got = heard(t, view.messages(true)[0])
	if files := piece(got.View, "files"); piece(got.View, "tabs").Selected != "commits" || len(files.Items) != 3 || files.Selected != "k:a41c9e2" || got.Focus != "tabs" {
		t.Errorf("behind Commits: %+v, focus %q", files, got.Focus)
	}
	if again := view.deed(gitEvent{E: "pick", List: "tabs", ID: "commits"}); again.do != gitNothing {
		t.Errorf("the tab that is chosen, picked: %+v", again)
	}
	if none := view.deed(gitEvent{E: "pick", List: "tabs", ID: "tags"}); none.do != gitNothing {
		t.Errorf("a tab there is none of, picked: %+v", none)
	}
}

// What the window is told of a view after it was told once: the view
// whole where more than its status and its keys is another, and where it
// says where the keys are; the status and the keys alone, as the changes
// of a patch, where only they are; nothing where nothing is; and after
// any of them what was just done, in a toast, and what is copied, once.
func TestOnlyWhatIsAnotherIsSaidAgain(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{pane: "%3", top: "/src/app", at: "c:src/talk.rs", standing: git.Standing{Branch: "talk", Head: "0123456789", Changed: []git.Change{{Path: "src/talk.rs", Added: 3, Deleted: 1}}}}
	view.entries, view.rows = gitList(view.standing, nil, "", 0, gitRowsMost, now)
	view.files = git.ParseDiff(gitTwoHunks)
	view.lines, view.hunks = gitCode(view.files)
	var told muiTold
	says := func(placed bool) []string {
		var where [][]byte
		if placed {
			where = view.messages(true)
		}
		var said []string
		for _, message := range told.says(view.pane, view.messages(false), where, &view.passing) {
			got := heard(t, message)
			if got.View != nil {
				said = append(said, "whole")
				continue
			}
			var patch struct {
				Patch []struct {
					Op, Text, Tone string
					Keys           []gitKey
				} `json:"patch"`
			}
			if err := json.Unmarshal(message, &patch); err != nil || got.Pane != "%3" {
				t.Fatalf("%v in %s", err, message)
			}
			for _, change := range patch.Patch {
				text := strings.TrimSpace(change.Op + " " + change.Text + " " + change.Tone)
				if len(change.Keys) > 0 {
					text += fmt.Sprintf(" %s %d", change.Keys[0].Label, len(change.Keys))
				}
				said = append(said, text)
			}
		}
		return said
	}
	t.Setenv("EDITOR", "ed")
	if got := says(true); !slices.Equal(got, []string{"whole"}) {
		t.Fatalf("first: %q", got)
	}
	if got := says(false); got != nil {
		t.Errorf("as it was told: %q", got)
	}
	// the keys cross to the diff: only the keys are said
	if say, placed := view.act(view.deed(gitEvent{E: "focus", ID: "diff"}), now); !say || placed || !view.inDiff {
		t.Fatal("the keys in the diff are not known to be")
	}
	if got := says(false); !slices.Equal(got, []string{"keys stage hunk 11"}) {
		t.Errorf("the keys crossed: %q", got)
	}
	// a refusal is the status alone, and taken away alone
	view.act(view.deed(gitEvent{E: "pick", List: "diff", ID: "1"}), now)
	if got := says(false); !slices.Equal(got, []string{"status ⏎ stages a line added or deleted: the keys are on neither bad"}) {
		t.Errorf("a refusal: %q", got)
	}
	view.status = nil
	if got := says(false); !slices.Equal(got, []string{"status"}) {
		t.Errorf("the status gone: %q", got)
	}
	// what is copied and the toast that says so are said once
	view.act(view.deed(gitEvent{E: "focus", ID: "files"}), now)
	view.act(view.deed(gitEvent{E: "press", ID: "copy", Item: "c:src/talk.rs"}), now)
	if got := says(false); !slices.Equal(got, []string{"keys stage 12", "copy src/talk.rs", "toast copied src/talk.rs"}) {
		t.Errorf("a path copied: %q", got)
	}
	if got := says(false); got != nil {
		t.Errorf("said twice: %q", got)
	}
	// a modal is of the view: said whole, and whole again when it goes
	view.act(view.deed(gitEvent{E: "press", ID: "discard", Item: "c:src/talk.rs"}), now)
	if got := says(false); !slices.Equal(got, []string{"whole"}) {
		t.Errorf("a modal up: %q", got)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}), now)
	if got := says(false); !slices.Equal(got, []string{"whole"}) {
		t.Errorf("a modal gone: %q", got)
	}
	// placed, it is said whole though nothing is another
	if got := says(true); !slices.Equal(got, []string{"whole"}) {
		t.Errorf("placed: %q", got)
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

// asks is the modal a view says, with its title, the words under it and
// its buttons each as its label and its key; nil where it says none.
func asks(t *testing.T, view *gitView) []string {
	t.Helper()
	got := heard(t, view.messages(false)[0])
	modal := piece(got.View, "sure")
	if modal == nil {
		return nil
	}
	if modal.T != "modal" || got.Focus != "yes" || got.View.Children[len(got.View.Children)-1].ID != "sure" {
		t.Fatalf("the modal = %+v, focus %q", modal, got.Focus)
	}
	said := append([]string{modal.Title}, texts(*modal)...)
	for _, button := range modal.Children[len(modal.Children)-1].Children {
		said = append(said, button.ID+"="+button.Label+" "+button.Key)
		if button.T != "button" || button.Primary != (button.ID == "yes") {
			t.Fatalf("the button = %+v", button)
		}
	}
	return said
}

// toasted is what a view just did, as its toast says it; none is no words.
func toasted(view *gitView) muiStatus {
	if view.passing.toast == nil {
		return muiStatus{}
	}
	return *view.passing.toast
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
	if got.Focus != "message" { // the keys are in what is being written
		t.Errorf("focus = %q", got.Focus)
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
	if toasted(view) != (muiStatus{"committed " + hash, "ok"}) || view.status != nil || view.writing || view.message != "" {
		t.Fatalf("committed: %+v, writing %v", toasted(view), view.writing)
	}
	if got := repo.Git("log", "-1", "--format=%B"); got != message {
		t.Errorf("the message = %q", got)
	}
	if !slices.Equal(ids(view), []string{"u:notes.txt", "k:" + hash}) || view.at != "u:notes.txt" || view.ahead != 1 {
		t.Errorf("rows = %v, on %q, %d ahead", ids(view), view.at, view.ahead)
	}
	got = heard(t, view.messages(true)[0])
	if want := []string{"words", "1 ahead of main, none behind", "1 untracked", "notes.txt", "untracked", "+1"}; !slices.Equal(texts(*got.View), want) || got.Status != nil || got.Focus != "files" {
		t.Errorf("texts = %q, status %+v", texts(*got.View), got.Status)
	}
	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "k:" + hash}), time.Now())
	// a commit says who wrote it and when, then its subject over its files
	secs, _ := strconv.ParseInt(repo.Git("log", "-1", "--format=%at"), 10, 64)
	when := time.Unix(secs, 0).Format("2006-01-02 15:04")
	if got := texts(*heard(t, view.messages(true)[0]).View); !slices.Equal(got[3:], []string{hash, "test · " + when, "+1", "A file says one", "talk.txt", "+1"}) {
		t.Errorf("the commit's lines = %q", got[3:])
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

// The keys said are those of where the keys are: on a file what stages
// and discards it, in its diff what does so to a hunk and a line, on the
// last commit what undoes it, on a stash what applies, pops and drops it,
// and then those of the worktree whole. What ⏎ does on a line of the
// diff the code says itself.
func TestTheKeysAreThoseOfWhereTheKeysAre(t *testing.T) {
	t.Setenv("EDITOR", "ed")
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{base: "main", ahead: 2, standing: git.Standing{Branch: "talk", Head: "0123456789",
		Staged: []git.Change{{Path: "a.rs"}}, Changed: []git.Change{{Path: "b.rs"}}, Untracked: []git.Change{{Path: "c.rs"}}}}
	view.entries, view.rows = gitList(view.standing, []git.Commit{{SHA: "a41c9e2", When: now}, {SHA: "19f2c6a", When: now}}, "main", 2, gitRowsMost, now)
	view.entries, view.rows = gitStashed(view.entries, view.rows, []git.Stash{{SHA: "5e6f7a8b9c0d1e2f", Subject: "On talk: mia git: talk 2026-11-20 10:04", When: now.Add(-3 * time.Hour)}}, now)
	if last := view.entries[len(view.entries)-3:]; !reflect.DeepEqual(last, []gitEntry{{Rule: true}, {Head: "Stashes", Note: "1"}, {ID: "t:5e6f7a8b9c", Title: "On talk: mia git: talk 2026-11-20 10:04", Right: "3h"}}) {
		t.Errorf("the stashes = %+v", last)
	}
	whole := "a:stage all c:commit b:branches s:stash f:fetch P:pull p:push r:ask again"
	for _, c := range []struct {
		at     string
		inDiff bool
		want   string
	}{
		{"s:a.rs", false, "space:unstage e:edit in ed y:copy the path " + whole},
		{"c:b.rs", false, "space:stage d:discard e:edit in ed y:copy the path " + whole},
		{"u:c.rs", false, "space:stage d:delete e:edit in ed y:copy the path " + whole},
		{"c:b.rs", true, "space:stage hunk d:discard hunk e:edit in ed " + whole},
		{"s:a.rs", true, "space:unstage hunk e:edit in ed " + whole},
		{"k:a41c9e2", false, "u:undo the commit y:copy the hash " + whole},
		{"k:19f2c6a", false, "y:copy the hash " + whole},
		{"t:5e6f7a8b9c", false, "space:apply p:pop d:drop a:stage all c:commit b:branches s:stash f:fetch P:pull r:ask again"},
	} {
		view.at, view.inDiff = c.at, c.inDiff
		var got []string
		for _, key := range view.keys() {
			got = append(got, key.Key+":"+key.Label)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("on %s (in the diff: %v) the keys are %q", c.at, c.inDiff, got)
		}
	}
	// a commit a remote has is not undone, and a clean worktree has nothing to stage or stash
	view.at, view.inDiff = "k:a41c9e2", false
	view.standing = git.Standing{Branch: "talk", Head: "0123456789", Upstream: "origin/talk"}
	if got := view.keys(); got[0] != (gitKey{"y", "copy the hash", "copy"}) || got[1] != (gitKey{"c", "commit", "commit"}) || len(got) != 7 {
		t.Errorf("on a commit origin has, in a clean worktree: %+v", got)
	}
	// ⏎ on a line stages it in a changed file's diff and unstages it in a staged one's
	view.standing = git.Standing{Branch: "talk", Head: "0123456789"}
	for at, want := range map[string]string{"c:b.rs": "stage line", "s:a.rs": "unstage line", "u:c.rs": "", "k:a41c9e2": ""} {
		view.at = at
		if got := piece(heard(t, view.messages(false)[0]).View, "diff").Does; got != want {
			t.Errorf("⏎ in the diff of %s does %q", at, got)
		}
	}

	for _, c := range []struct {
		s    git.Standing
		want string
	}{
		{git.Standing{}, ""},
		{git.Standing{Upstream: "origin/talk", ToPush: 2, ToPull: 1}, "2 to push, 1 to pull"},
		{git.Standing{Upstream: "origin/talk", ToPull: 4}, "4 to pull"},
		{git.Standing{Upstream: "origin/talk"}, "nothing to push or pull"},
		{git.Standing{Upstream: "origin/talk", Gone: true}, "origin/talk is gone"},
	} {
		if got := gitFollows(c.s); got != c.want {
			t.Errorf("%+v follows: %q, want %q", c.s, got, c.want)
		}
	}
}

// What the keys of a daily git tool ask for: d, u and p say first what
// they lose or send, in a modal, and are sure only when the same is asked
// again, by the modal's button or by the key; esc and its other button
// leave it undone; ⏎ on a line added or deleted stages that line alone;
// on a stash space, p and d apply, pop and drop it; and what cannot be
// done is refused with why.
func TestWhatTheKeysOfADailyToolAsk(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	view := &gitView{at: "c:src/talk.rs", base: "main", ahead: 2, standing: git.Standing{Branch: "talk", Head: "0123456789",
		Staged:    []git.Change{{Path: "src/hear.rs"}},
		Changed:   []git.Change{{Path: "src/talk.rs", Added: 3, Deleted: 1}},
		Untracked: []git.Change{{Path: "notes.md"}},
	}}
	view.entries, view.rows = gitList(view.standing, []git.Commit{{SHA: "a41c9e2", Subject: "A subject", When: now}, {SHA: "19f2c6a", When: now}}, "main", 2, gitRowsMost, now)
	view.entries, view.rows = gitStashed(view.entries, view.rows, []git.Stash{{SHA: "5e6f7a8b9c0d1e2f", Subject: "On talk: earlier"}}, now)
	view.files = git.ParseDiff(gitTwoHunks)
	view.lines, view.hunks = gitCode(view.files)
	view.unpushed = 2
	line, _ := view.files[0].LinePatch(0, 2, false)

	for _, c := range []struct {
		event string
		do    gitDo
		row   string
		patch string
		ask   string
		yes   string
	}{
		{`{"e":"press","id":"discard","item":"c:src/talk.rs"}`, gitDiscard, "c:src/talk.rs", "", "Discard what is changed in src/talk.rs and not staged (+3 -1)?", "Discard"},
		{`{"e":"pick","list":"files","id":"c:src/talk.rs","sub":"discard"}`, gitDiscard, "c:src/talk.rs", "", "Discard what is changed in src/talk.rs and not staged (+3 -1)?", "Discard"},
		{`{"e":"press","id":"discard","item":"u:notes.md"}`, gitDiscard, "u:notes.md", "", "Delete notes.md, untracked and in no commit?", "Delete"},
		{`{"e":"press","id":"discard","item":"10"}`, gitDiscardHunk, "c:src/talk.rs", view.files[0].Patch(1), "Discard hunk 2 of 2 of src/talk.rs (+1)?", "Discard"},
		{`{"e":"press","id":"discard","item":"7"}`, gitNothing, "", "", "", ""}, // the gap between two hunks
		{`{"e":"press","id":"discard","item":"s:src/hear.rs"}`, gitRefuse, "", "", "", ""},
		{`{"e":"press","id":"discard","item":"k:a41c9e2"}`, gitNothing, "", "", "", ""},
		{`{"e":"press","id":"discard","item":"t:5e6f7a8b9c"}`, gitStashDrop, "t:5e6f7a8b9c", "", `Drop the stash "On talk: earlier" and what it holds?`, "Drop"},
		{`{"e":"press","id":"stage","item":"t:5e6f7a8b9c"}`, gitStashApply, "t:5e6f7a8b9c", "", "", ""},
		{`{"e":"press","id":"push","item":"t:5e6f7a8b9c"}`, gitStashPop, "t:5e6f7a8b9c", "", "", ""},
		{`{"e":"pick","list":"files","id":"t:5e6f7a8b9c","sub":"push"}`, gitStashPop, "t:5e6f7a8b9c", "", "", ""},
		{`{"e":"press","id":"push","item":"c:src/talk.rs"}`, gitPush, "", "", "Push 2 commits to origin/talk?", "Push"},
		{`{"e":"press","id":"undo","item":"k:a41c9e2"}`, gitUndo, "k:a41c9e2", "", `Undo a41c9e2 "A subject"?`, "Undo"},
		{`{"e":"pick","list":"files","id":"k:a41c9e2","sub":"undo"}`, gitUndo, "k:a41c9e2", "", `Undo a41c9e2 "A subject"?`, "Undo"},
		{`{"e":"press","id":"undo","item":"k:19f2c6a"}`, gitRefuse, "", "", "", ""},
		{`{"e":"press","id":"undo","item":"c:src/talk.rs"}`, gitNothing, "", "", "", ""},
		{`{"e":"pick","list":"diff","id":"3"}`, gitStageLine, "c:src/talk.rs", line, "", ""},
		{`{"e":"pick","list":"diff","id":"1"}`, gitRefuse, "", "", "", ""}, // a line the same
		{`{"e":"pick","list":"diff","id":"0"}`, gitRefuse, "", "", "", ""}, // the hunk's head
		{`{"e":"move","list":"diff","id":"4"}`, gitNothing, "", "", "", ""},
		{`{"e":"move","list":"files","id":"u:notes.md"}`, gitMove, "u:notes.md", "", "", ""},
		{`{"e":"press","id":"branches","item":"c:src/talk.rs"}`, gitBranches, "", "", "", ""},
		{`{"e":"press","id":"stash"}`, gitStash, "", "", "", ""},
		{`{"e":"press","id":"fetch"}`, gitFetch, "", "", "", ""},
		{`{"e":"press","id":"pull"}`, gitRefuse, "", "", "", ""}, // it follows nothing
		{`{"e":"press","id":"amend"}`, gitAmend, "", "", "", ""},
		{`{"e":"pick","list":"branches","id":"l:main"}`, gitNothing, "", "", "", ""}, // the branches are not up
		{`{"e":"change","id":"q","text":"ma"}`, gitNothing, "", "", "", ""},
	} {
		var event gitEvent
		if err := json.Unmarshal([]byte(c.event), &event); err != nil {
			t.Fatal(err)
		}
		if got := view.deed(event); got.do != c.do || got.row.id != c.row || got.patch != c.patch || got.ask != c.ask || got.yes != c.yes || got.sure || (got.ask == "") != (got.told == "") {
			t.Errorf("%s does %d on %q with %q, asks %q under %q, sure %v", c.event, got.do, got.row.id, got.patch, got.ask, got.yes, got.sure)
		}
	}
	if line != "diff --git a/src/talk.rs b/src/talk.rs\n--- a/src/talk.rs\n+++ b/src/talk.rs\n@@ -212,4 +212,5 @@\n \tlet ui = self.uis.entry(pane);\n \tui.view = msg.view;\n+\tui.take(msg);\n \tHeard::Drawn\n }\n" {
		t.Errorf("the patch of one line = %q", line)
	}

	// asked once it is a modal over the view, the keys on the button that
	// does it, and nothing is the status
	asked := view.deed(gitEvent{E: "press", ID: "discard", Item: "c:src/talk.rs"})
	if say, placed := view.act(asked, now); !say || placed || view.status != nil || view.armed.ask != asked.ask {
		t.Fatalf("asked once: %+v, %+v", view.status, view.armed)
	}
	if want := []string{"Discard what is changed in src/talk.rs and not staged (+3 -1)?", "Git cannot bring it back. What is staged of it stays.", "yes=Discard d", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) {
		t.Errorf("the modal says %q", asks(t, view))
	}
	// asked again the same, by its button or by the key, it is sure
	for _, again := range []gitEvent{{E: "press", ID: "yes"}, {E: "press", ID: "discard", Item: "c:src/talk.rs"}} {
		if got := view.deed(again); !got.sure || got.do != gitDiscard || got.row.id != "c:src/talk.rs" {
			t.Errorf("%+v asked again: %+v", again, got)
		}
	}
	if other := view.deed(gitEvent{E: "press", ID: "discard", Item: "u:notes.md"}); other.sure {
		t.Error("d on another row is sure of what was asked of the first")
	}
	// the keys passed between its buttons leave it up; esc and its other
	// button take it away, and the keys go back to where they were
	if say, _ := view.act(view.deed(gitEvent{E: "focus", ID: "no"}), now); say || view.armed.ask == "" {
		t.Errorf("the keys on its other button: said %v, %+v", say, view.armed)
	}
	for _, never := range []gitEvent{{E: "dismiss", ID: "sure"}, {E: "press", ID: "no"}} {
		view.act(asked, now)
		if say, _ := view.act(view.deed(never), now); !say || view.armed != (gitArmed{}) || view.status != nil || asks(t, view) != nil {
			t.Errorf("%+v leaves the modal up: %+v", never, view.armed)
		}
		if got := heard(t, view.messages(false)[0]); got.Focus != "files" {
			t.Errorf("after %+v the keys are put on %q", never, got.Focus)
		}
		if again := view.deed(gitEvent{E: "press", ID: "discard", Item: "c:src/talk.rs"}); again.sure {
			t.Errorf("d is sure after %+v", never)
		}
	}
	// asked from the diff, the keys go back to the diff
	view.act(view.deed(gitEvent{E: "focus", ID: "diff"}), now)
	view.act(view.deed(gitEvent{E: "press", ID: "discard", Item: "10"}), now)
	if want := []string{"Discard hunk 2 of 2 of src/talk.rs (+1)?", "Git cannot bring it back.", "yes=Discard d", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) {
		t.Errorf("the modal says %q", asks(t, view))
	}
	if again := view.deed(gitEvent{E: "press", ID: "yes"}); !again.sure || again.do != gitDiscardHunk || again.patch != view.files[0].Patch(1) {
		t.Errorf("its button: %+v", again)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}), now)
	if got := heard(t, view.messages(false)[0]); got.Focus != "diff" || !view.inDiff {
		t.Errorf("after esc the keys are put on %q", got.Focus)
	}
	view.act(view.deed(gitEvent{E: "focus", ID: "files"}), now)
	// the keys of the other modals are the keys that ask for them
	for id, want := range map[string]string{"undo": "yes=Undo u", "push": "yes=Push p"} {
		view.act(view.deed(gitEvent{E: "press", ID: id, Item: "k:a41c9e2"}), now)
		if got := asks(t, view); len(got) != 4 || got[2] != want {
			t.Errorf("%s asks %q", id, got)
		}
		view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}), now)
	}

	// with a remote that has every commit: nothing to push, amend or undo
	view.standing.Upstream, view.message = "origin/talk", "Other words"
	for id, want := range map[string]string{
		"push":  "nothing to push: origin/talk has every commit",
		"amend": "the last commit is on origin/talk already: to amend it would take a push by force",
		"undo":  "the last commit is on origin/talk already: to undo it would take a push by force",
	} {
		if got := view.deed(gitEvent{E: "press", ID: id, Item: "k:a41c9e2"}); got.do != gitRefuse || got.text != want {
			t.Errorf("%s: %d %q", id, got.do, got.text)
		}
	}
	view.standing.ToPush, view.standing.ToPull = 1, 2
	if got := view.deed(gitEvent{E: "press", ID: "push"}); got.do != gitPush || got.ask != "Push 1 commit to origin/talk?" || got.told != "Nothing is forced: a remote that has moved on refuses it." {
		t.Errorf("push: %+v", got)
	}
	if got := view.deed(gitEvent{E: "press", ID: "amend"}); got.do != gitAmend || got.text != "Other words" {
		t.Errorf("amend: %+v", got)
	}
	if got := view.deed(gitEvent{E: "press", ID: "pull"}); got.do != gitPull {
		t.Errorf("pull: %+v", got)
	}
	view.standing.Staged = nil
	view.message = ""
	if got := view.deed(gitEvent{E: "press", ID: "amend"}); got.do != gitRefuse || !strings.HasPrefix(got.text, "nothing is staged and no message is written") {
		t.Errorf("amend with nothing: %+v", got)
	}

	// the branches up: words narrow them, ⏎ switches, and on the branch
	// the worktree is on goes back, as esc does
	view.picking = true
	for event, want := range map[gitEvent]gitDeed{
		{E: "change", ID: "q", Text: "ma"}:           {do: gitNarrow, text: "ma"},
		{E: "pick", List: "branches", ID: "l:main"}:  {do: gitSwitch, text: "l:main"},
		{E: "pick", List: "branches", ID: "n:other"}: {do: gitSwitch, text: "n:other"},
		{E: "pick", List: "branches", ID: "l:talk"}:  {do: gitBack},
		{E: "dismiss", ID: "switch"}:                 {do: gitBack},
	} {
		if got := view.deed(event); got != want {
			t.Errorf("%+v does %+v", event, got)
		}
	}
}

// The branches to pick from are the repository's, the one the worktree
// is on checked and each with how it stands against what it follows,
// then the remotes' that no branch here is named for; words narrow them
// whatever their case, and words that name no branch offer to make it.
func TestTheBranchesToPickFrom(t *testing.T) {
	branches := git.ParseBranches(strings.Join([]string{
		"refs/heads/main\x00*\x00origin/main\x00",
		"refs/heads/mui-patch\x00 \x00origin/mui-patch\x00ahead 2, behind 1",
		"refs/heads/old\x00 \x00origin/old\x00gone",
		"refs/heads/scratch\x00 \x00\x00",
		"refs/remotes/origin/HEAD\x00 \x00\x00",
		"refs/remotes/origin/main\x00 \x00\x00",
		"refs/remotes/origin/mui-list\x00 \x00\x00",
	}, "\n"))
	entries, found := gitBranchList(branches, "main", "", gitBranchMost)
	want := []gitEntry{
		{ID: "l:main", Title: "main", Right: "level", Icon: "git-branch", Checked: true, Does: "switch"},
		{ID: "l:mui-patch", Title: "mui-patch", Right: "↑2 ↓1", Icon: "git-branch", Does: "switch"},
		{ID: "l:old", Title: "old", Right: "gone", Icon: "git-branch", Does: "switch"},
		{ID: "l:scratch", Title: "scratch", Right: "no upstream", Icon: "git-branch", Does: "switch"},
		{Head: "Remote", Note: "1"},
		{ID: "r:origin/mui-list", Title: "origin/mui-list", Does: "follow it here"},
	}
	if !reflect.DeepEqual(entries, want) || found != 5 {
		t.Errorf("%d branches: %+v", found, entries)
	}
	create := func(name string) gitEntry {
		return gitEntry{ID: "n:" + name, Title: "Create " + name + " from main", Does: "create"}
	}
	for typed, want := range map[string][]gitEntry{
		"MUI":      {want[1], {Head: "Remote", Note: "1"}, want[5], {Rule: true}, create("MUI")},
		" scratch": {want[3]},
		"mui-list": {{Head: "Remote", Note: "1"}, want[5]},
		"flags":    {create("flags")},
	} {
		if got, _ := gitBranchList(branches, "main", typed, gitBranchMost); !reflect.DeepEqual(got, want) {
			t.Errorf("%q finds %+v", typed, got)
		}
	}
	if got, found := gitBranchList(branches, "", "", 2); len(got) != 2 || found != 2 {
		t.Errorf("with room for two: %+v", got)
	}
}

// remote makes a bare repository beside a repository as its origin, with
// main pushed to it and followed, and other is a clone of it that commits
// a file to a branch and pushes it: what someone else did.
func remote(t *testing.T, repo *gittest.Repo) (bare string, other func(branch, name string)) {
	t.Helper()
	beside := filepath.Dir(repo.Root)
	bare = filepath.Join(beside, "origin.git")
	repo.GitIn(beside, "init", "-q", "--bare", "-b", "main", bare)
	repo.Git("remote", "add", "origin", bare)
	repo.Git("push", "-q", "-u", "origin", "main")
	clone := filepath.Join(beside, "other")
	repo.GitIn(beside, "clone", "-q", bare, clone)
	repo.GitIn(clone, "config", "user.email", "test@example.com")
	repo.GitIn(clone, "config", "user.name", "test")
	return bare, func(branch, name string) {
		t.Helper()
		repo.GitIn(clone, "fetch", "-q")
		repo.GitIn(clone, "checkout", "-q", "-B", branch, "origin/"+branch)
		if err := os.WriteFile(filepath.Join(clone, name), []byte("theirs\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		repo.GitIn(clone, "add", "-A")
		repo.GitIn(clone, "commit", "-qm", "theirs: "+name)
		repo.GitIn(clone, "push", "-q", "origin", branch)
	}
}

// ran does what the view left to be done with a remote, as the loop does
// beside itself, and takes how it went.
func ran(t *testing.T, view *gitView) {
	t.Helper()
	work := view.work
	if work == nil {
		t.Fatalf("nothing is left to be done with a remote: %+v", view.status)
	}
	view.work = nil
	view.finish(work(), time.Now())
}

// In a real repository: d on a changed file puts up a modal that says
// what is lost and does nothing; d again, or its button, discards it, and
// esc leaves the file as it is. Where the file changed again between the
// two the modal is said again in the new words. On a line of the diff it
// is the hunk that line is in, on an untracked file the file, and a
// staged file is refused.
func TestDAsksInAModalAndThenDiscards(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", lineNumbered(1, 30, nil))
	repo.Commit("a file")
	changed := lineNumbered(1, 30, map[int]string{3: "three", 28: "twenty-eight"})
	repo.Write("talk.txt", changed)
	repo.Write("notes.txt", "one\n")

	view := &gitView{dir: repo.Root}
	view.read(time.Now())
	const ask = "Discard what is changed in talk.txt and not staged (+2 -2)?"
	press(t, view, "discard", "c:talk.txt")
	if got := asks(t, view); len(got) != 4 || got[0] != ask || got[2] != "yes=Discard d" || view.status != nil || repo.Git("diff", "--name-only") != "talk.txt" {
		t.Fatalf("d once: %q, %+v", got, view.status)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}), time.Now())
	if asks(t, view) != nil || repo.Git("diff", "--name-only") != "talk.txt" {
		t.Fatalf("esc: %q", asks(t, view))
	}
	// the file changed again while the modal was up: asked again, in the new words
	press(t, view, "discard", "c:talk.txt")
	repo.Write("talk.txt", lineNumbered(1, 30, map[int]string{3: "three", 15: "fifteen", 28: "twenty-eight"}))
	view.read(time.Now())
	press(t, view, "yes", "")
	if got := asks(t, view); len(got) != 4 || got[0] != "Discard what is changed in talk.txt and not staged (+3 -3)?" || repo.Git("diff", "--name-only") != "talk.txt" {
		t.Fatalf("its button after the file changed: %q", got)
	}
	repo.Write("talk.txt", changed)
	view.read(time.Now())
	press(t, view, "discard", "c:talk.txt")
	if got := asks(t, view); len(got) != 4 || got[0] != ask {
		t.Fatalf("d after it changed back: %q", got)
	}
	press(t, view, "yes", "")
	if toasted(view) != (muiStatus{"discarded what was changed in talk.txt", "ok"}) || view.status != nil || asks(t, view) != nil || repo.Git("status", "--porcelain") != "?? notes.txt" {
		t.Fatalf("its button: %+v, git says %q", toasted(view), repo.Git("status", "--porcelain"))
	}

	// one hunk of two
	repo.Write("talk.txt", changed)
	view.read(time.Now())
	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "c:talk.txt"}), time.Now())
	in := slices.IndexFunc(view.lines, func(line gitLine) bool { return line.Text == "twenty-eight" })
	press(t, view, "discard", fmt.Sprint(in))
	if got := asks(t, view); len(got) != 4 || got[0] != "Discard hunk 2 of 2 of talk.txt (+1 -1)?" {
		t.Fatalf("d on a line: %q", got)
	}
	press(t, view, "discard", fmt.Sprint(in-1)) // d again, on another line of the same hunk
	if got := repo.Git("diff", "--unified=0"); !strings.Contains(got, "+three") || strings.Contains(got, "twenty-eight") || toasted(view) != (muiStatus{"discarded a hunk of talk.txt", "ok"}) {
		t.Fatalf("the hunk discarded: %+v\n%s", toasted(view), got)
	}

	// a staged file is unstaged first; an untracked one is deleted
	repo.Git("add", "talk.txt")
	view.read(time.Now())
	press(t, view, "discard", "s:talk.txt")
	if view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "space unstages it first") || view.armed != (gitArmed{}) {
		t.Errorf("d on a staged file: %+v", view.status)
	}
	press(t, view, "discard", "u:notes.txt")
	if want := []string{"Delete notes.txt, untracked and in no commit?", "Git cannot bring it back.", "yes=Delete d", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) {
		t.Fatalf("d on an untracked file: %q", asks(t, view))
	}
	press(t, view, "yes", "")
	if _, err := os.Stat(filepath.Join(repo.Root, "notes.txt")); !os.IsNotExist(err) || !slices.Equal(ids(view), []string{"s:talk.txt"}) {
		t.Errorf("the untracked file is still there: %v, rows %v", err, ids(view))
	}
}

// In a real repository: ⏎ on a line added in a changed file's diff stages
// that line alone, on a line deleted that deletion alone; on a line of
// the staged diff it unstages it, and the worktree keeps every change.
func TestEnterInTheDiffStagesAndUnstagesOneLine(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", lineNumbered(1, 12, nil))
	repo.Commit("a file")
	repo.Write("talk.txt", lineNumbered(1, 12, map[int]string{3: "three\nand a half", 9: "nine"}))

	view := &gitView{dir: repo.Root}
	view.read(time.Now())
	at := func(text, kind string) string {
		t.Helper()
		in := slices.IndexFunc(view.lines, func(line gitLine) bool { return line.Text == text && line.Kind == kind })
		if in < 0 {
			t.Fatalf("no line %q in %+v", text, view.lines)
		}
		return fmt.Sprint(in)
	}
	pick := func(text, kind string) {
		t.Helper()
		if say, _ := view.act(view.deed(gitEvent{E: "pick", List: "diff", ID: at(text, kind)}), time.Now()); !say || view.status != nil {
			t.Fatalf("⏎ on %q: %+v", text, view.status)
		}
	}
	pick("and a half", "add")
	if got := repo.Git("diff", "--cached", "--unified=0"); !strings.Contains(got, "+and a half") || strings.Count(got, "\n+")+strings.Count(got, "\n-") != 3 {
		t.Fatalf("one line staged:\n%s", got)
	}
	if got := repo.Git("show", ":talk.txt"); got != strings.TrimSpace(lineNumbered(1, 12, map[int]string{3: "line 3\nand a half"})) {
		t.Errorf("the index holds:\n%s", got)
	}
	if !slices.Equal(ids(view), []string{"s:talk.txt", "c:talk.txt"}) || view.at != "c:talk.txt" {
		t.Fatalf("rows = %v, on %q", ids(view), view.at)
	}
	pick("line 9", "del")
	if got := repo.Git("show", ":talk.txt"); got != strings.TrimSpace(strings.Replace(lineNumbered(1, 12, map[int]string{3: "line 3\nand a half"}), "line 9\n", "", 1)) {
		t.Errorf("a deletion staged, the index holds:\n%s", got)
	}
	if got := repo.Git("diff", "--unified=0"); !strings.Contains(got, "+three") || !strings.Contains(got, "+nine") || strings.Contains(got, "and a half") || strings.Contains(got, "-line 9") {
		t.Errorf("still changed:\n%s", got)
	}

	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "s:talk.txt"}), time.Now())
	pick("and a half", "add")
	pick("line 9", "del")
	if repo.Git("diff", "--cached") != "" || !slices.Equal(ids(view), []string{"c:talk.txt"}) {
		t.Errorf("both unstaged: rows %v, staged:\n%s", ids(view), repo.Git("diff", "--cached"))
	}
	if got, err := os.ReadFile(filepath.Join(repo.Root, "talk.txt")); err != nil || string(got) != lineNumbered(1, 12, map[int]string{3: "three\nand a half", 9: "nine"}) {
		t.Errorf("the worktree's file changed: %q", got)
	}

	// a line the same is no line to stage
	view.act(view.deed(gitEvent{E: "pick", List: "diff", ID: at("line 2", "same")}), time.Now())
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "⏎ stages a line added or deleted") {
		t.Errorf("⏎ on a line the same: %+v", view.status)
	}
}

// In a real repository with a remote: the commit screen has a second
// button that amends the last commit with what is staged, under the
// message typed or with none under the one it had; a commit the remote
// has is not amended, and the reason is the status.
func TestTheLastCommitIsAmendedUnlessARemoteHasIt(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	remote(t, repo)
	repo.Write("talk.txt", "one\n")

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	press(t, view, "all", "")
	press(t, view, "commit", "")
	button := piece(heard(t, view.messages(true)[0]).View, "amend")
	if button == nil || button.T != "button" || button.Label != "Amend the last one" || button.Key != "⌘A" || button.Primary {
		t.Fatalf("the button = %+v", button)
	}
	head := repo.Git("rev-parse", "HEAD")
	press(t, view, "amend", "")
	if view.status == nil || *view.status != (muiStatus{"the last commit is on origin/main already: to amend it would take a push by force", "bad"}) || repo.Git("rev-parse", "HEAD") != head || !view.writing {
		t.Fatalf("amending what origin has: %+v", view.status)
	}

	view.act(view.deed(gitEvent{E: "submit", ID: "message", Text: "A file says one"}), time.Now())
	repo.Write("talk.txt", "one\ntwo\n")
	press(t, view, "all", "")
	press(t, view, "commit", "")
	press(t, view, "amend", "") // with no message: under the one it had
	hash := repo.Git("rev-parse", "--short", "HEAD")
	if toasted(view) != (muiStatus{"amended: the last commit is " + hash, "ok"}) || view.status != nil || view.writing {
		t.Fatalf("amended: %+v", toasted(view))
	}
	if repo.Git("log", "--format=%s", "origin/main..HEAD") != "A file says one" || repo.Git("show", "HEAD:talk.txt") != "one\ntwo" || repo.Git("status", "--porcelain") != "" {
		t.Errorf("the last commit = %q holding %q", repo.Git("log", "--format=%s", "origin/main..HEAD"), repo.Git("show", "HEAD:talk.txt"))
	}

	press(t, view, "commit", "")
	press(t, view, "amend", "")
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "nothing is staged and no message is written") {
		t.Errorf("amending with nothing: %+v", view.status)
	}
	view.act(view.deed(gitEvent{E: "change", ID: "message", Text: "A file says one and two"}), time.Now())
	press(t, view, "amend", "")
	if repo.Git("log", "--format=%s", "origin/main..HEAD") != "A file says one and two" || view.message != "" {
		t.Errorf("amended under other words: %q, %+v", repo.Git("log", "--format=%s", "origin/main..HEAD"), view.status)
	}
}

// In a real repository: u on the last commit says what it undoes, and a
// second u takes it off the branch with what it changed left staged; an
// older commit, and one a remote has, are refused.
func TestUUndoesTheLastCommitAndKeepsItsChangesStaged(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Git("checkout", "-q", "-b", "words")
	repo.Write("a.txt", "a\n")
	repo.Commit("The first")
	repo.Write("b.txt", "b\n")
	repo.Commit("The second")
	first, second := repo.Git("rev-parse", "--short", "HEAD~1"), repo.Git("rev-parse", "--short", "HEAD")

	view := &gitView{dir: repo.Root}
	view.read(time.Now())
	press(t, view, "undo", "k:"+first)
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "only the last commit is undone") {
		t.Fatalf("u on an older commit: %+v", view.status)
	}
	press(t, view, "undo", "k:"+second)
	if want := []string{fmt.Sprintf("Undo %s %q?", second, "The second"), "It is taken off the branch, and what it changed stays staged.", "yes=Undo u", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) || repo.Git("rev-parse", "--short", "HEAD") != second {
		t.Fatalf("u once: %q", asks(t, view))
	}
	press(t, view, "undo", "k:"+second)
	if repo.Git("rev-parse", "--short", "HEAD") != first || repo.Git("diff", "--cached", "--name-only") != "b.txt" || !slices.Equal(ids(view), []string{"s:b.txt", "k:" + first}) {
		t.Fatalf("u twice: at %s, staged %q, rows %v", repo.Git("rev-parse", "--short", "HEAD"), repo.Git("diff", "--cached", "--name-only"), ids(view))
	}
	if toasted(view) != (muiStatus{"undone: what " + second + " changed is staged", "ok"}) || view.status != nil {
		t.Errorf("toast = %+v, status = %+v", toasted(view), view.status)
	}

	remote(t, repo)
	repo.Git("push", "-q", "-u", "origin", "words")
	view.read(time.Now())
	press(t, view, "undo", "k:"+first)
	if view.status == nil || *view.status != (muiStatus{"the last commit is on origin/words already: to undo it would take a push by force", "bad"}) || repo.Git("rev-parse", "--short", "HEAD") != first {
		t.Errorf("u on a commit origin has: %+v", view.status)
	}
}

// In a real repository: a commit's side says who wrote it and when, its
// subject, and each file it changed with its lines, over its diff; past
// six files it says how many more there are.
func TestACommitSaysItsSubjectAndItsFilesOverItsDiff(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Git("checkout", "-q", "-b", "words")
	repo.Write("a.txt", "a\nb\n")
	repo.Write("notes/b.txt", "b\n")
	repo.Commit("Two files\n\nAnd a body.")
	for n := range 8 {
		repo.Write(fmt.Sprintf("many/%d.txt", n), "one\n")
	}
	repo.Git("rm", "-q", "notes/b.txt")
	repo.Commit("Eight files, and one gone")
	many, two := repo.Git("rev-parse", "--short", "HEAD"), repo.Git("rev-parse", "--short", "HEAD~1")

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	if view.at != "k:"+many || view.author != "test" {
		t.Fatalf("on %q, written by %q", view.at, view.author)
	}
	when := view.when.Format("2006-01-02 15:04")
	got := texts(*heard(t, view.messages(true)[0]).View)
	want := []string{many, "test · " + when, "+8 -1", "Eight files, and one gone", "many/0.txt", "+1", "many/1.txt", "+1", "many/2.txt", "+1", "many/3.txt", "+1", "many/4.txt", "+1", "many/5.txt", "+1", "and 3 more files"}
	if !slices.Equal(got[3:], want) {
		t.Errorf("texts = %q", got[3:])
	}
	view.act(view.deed(gitEvent{E: "move", List: "files", ID: "k:" + two}), time.Now())
	messages := view.messages(true)
	if got := texts(*heard(t, messages[0]).View); !slices.Equal(got[3:], []string{two, "test · " + when, "+3", "Two files", "a.txt", "+2", "notes/b.txt", "+1"}) {
		t.Errorf("texts = %q", got[3:])
	}
	if lines := code(t, messages); len(lines) != 7 || lines[0] != (gitLine{Kind: "head", Text: "a.txt"}) {
		t.Errorf("the diff under them = %+v", lines)
	}
}

// In a real repository: s stashes every change and every untracked file
// under a message that says the branch and the time, and the stash is
// listed under the commits with its diff beside it; on it space applies
// it and keeps it, p pops it, and d, asked twice, drops it.
func TestSStashesAndAStashIsAppliedPoppedAndDropped(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("talk.txt", "one\n")
	repo.Commit("a file")
	dirty := func() {
		repo.Write("talk.txt", "one\ntwo\n")
		repo.Write("notes.txt", "kept\n")
	}
	dirty()

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	now := time.Date(2026, 11, 20, 10, 4, 0, 0, time.Local)
	if say, _ := view.act(view.deed(gitEvent{E: "press", ID: "stash"}), now); !say {
		t.Fatal("s says nothing")
	}
	const message = "mia git: main 2026-11-20 10:04"
	if toasted(view) != (muiStatus{"stashed as " + message, "ok"}) || view.status != nil || repo.Git("status", "--porcelain") != "" {
		t.Fatalf("stashed: %+v, git says %q", toasted(view), repo.Git("status", "--porcelain"))
	}
	stash := "t:" + repo.Git("rev-parse", "stash@{0}")[:10]
	if !slices.Equal(ids(view), []string{stash}) || view.at != stash {
		t.Fatalf("rows = %v, on %q", ids(view), view.at)
	}
	if want := []gitEntry{{Head: "Stashes", Note: "1"}, {ID: stash, Title: "On main: " + message, Right: view.entries[1].Right, Menu: view.entries[1].Menu}}; !reflect.DeepEqual(view.entries, want) || len(view.entries[1].Menu) != 3 {
		t.Errorf("entries = %+v", view.entries)
	}
	got := heard(t, view.messages(true)[0])
	if all := texts(*got.View); !slices.Equal(all, []string{"main", "clean", stash[2:9], "On main: " + message, "+2"}) {
		t.Errorf("texts = %q", all)
	}
	if lines := code(t, view.messages(true)); !slices.ContainsFunc(lines, func(line gitLine) bool { return line.Kind == "add" && line.Text == "two" }) || !slices.ContainsFunc(lines, func(line gitLine) bool { return line.Kind == "add" && line.Text == "kept" }) {
		t.Errorf("the stash's diff = %+v", lines)
	}
	if got.Keys[0] != (gitKey{"space", "apply", "stage"}) || got.Keys[1] != (gitKey{"p", "pop", "push"}) || got.Keys[2] != (gitKey{"d", "drop", "discard"}) {
		t.Errorf("keys = %+v", got.Keys)
	}
	press(t, view, "stash", "")
	if view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "nothing to stash") {
		t.Errorf("s in a clean worktree: %+v", view.status)
	}

	press(t, view, "stage", stash) // space: applied, and kept
	if repo.Git("status", "--porcelain") != "M talk.txt\n?? notes.txt" || repo.Git("stash", "list", "--format=%gs") != "On main: "+message || toasted(view) != (muiStatus{"applied, and kept: On main: " + message, "ok"}) {
		t.Fatalf("applied: %+v, git says %q", toasted(view), repo.Git("status", "--porcelain"))
	}
	// what is in the way is said in git's words, and the stash is kept
	press(t, view, "push", stash)
	if view.status == nil || view.status.Tone != "bad" || repo.Git("stash", "list") == "" {
		t.Fatalf("popped onto the same changes: %+v", view.status)
	}
	repo.Git("checkout", "-q", "--", "talk.txt")
	if err := os.Remove(filepath.Join(repo.Root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	view.read(time.Now())
	press(t, view, "push", stash) // p: popped
	if repo.Git("status", "--porcelain") != "M talk.txt\n?? notes.txt" || repo.Git("stash", "list") != "" || slices.Contains(ids(view), stash) {
		t.Fatalf("popped: %+v, git says %q, rows %v", view.status, repo.Git("status", "--porcelain"), ids(view))
	}

	press(t, view, "stash", "")
	stash = "t:" + repo.Git("rev-parse", "stash@{0}")[:10]
	press(t, view, "discard", stash)
	held := repo.Git("stash", "list", "--format=%gs")
	if want := []string{`Drop the stash "` + held + `" and what it holds?`, "Git keeps no other copy of it.", "yes=Drop d", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) || held == "" {
		t.Fatalf("d once on a stash: %q", asks(t, view))
	}
	mark := view.stashMark()
	press(t, view, "discard", stash)
	if repo.Git("stash", "list") != "" || len(view.rows) != 0 || toasted(view) != (muiStatus{"dropped: " + held, "ok"}) || view.stashMark() == mark {
		t.Errorf("dropped: %+v, rows %v", toasted(view), ids(view))
	}
}

// In a real repository with a remote: b brings the branches up in a
// modal over the files, a field over their list; ⏎ switches to one, makes
// the branch that follows a remote's, or makes the one named from where
// the worktree is; what git refuses is the status, in its first line,
// said in the modal too, and the branches stay up; esc goes back to the
// files.
func TestBBringsTheBranchesUpAndEnterSwitches(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	remote(t, repo)
	repo.Git("push", "-q", "origin", "main:refs/heads/far")
	repo.Git("fetch", "-q", "origin")
	repo.Git("checkout", "-q", "-b", "talk")
	repo.Write("README.md", "talk's\n")
	repo.Commit("talk's own")
	repo.Git("checkout", "-q", "main")
	repo.Write("notes.txt", "kept\n")

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	if say, placed := view.act(view.deed(gitEvent{E: "press", ID: "branches"}), time.Now()); !say || !placed || !view.picking {
		t.Fatal("b does not bring the branches up")
	}
	got := heard(t, view.messages(true)[0])
	field, list := piece(got.View, "q"), piece(got.View, "branches")
	if got.Focus != "q" || field == nil || field.T != "field" || !field.Bare || !field.Live || field.Icon != "search" || field.List != "branches" || field.Right != "3 branches" || field.Value == nil || *field.Value != "" {
		t.Fatalf("the field = %+v, focus %q", field, got.Focus)
	}
	want := []gitEntry{
		{ID: "l:main", Title: "main", Right: "level", Icon: "git-branch", Checked: true, Does: "switch"},
		{ID: "l:talk", Title: "talk", Right: "no upstream", Icon: "git-branch", Does: "switch"},
		{Head: "Remote", Note: "1"},
		{ID: "r:origin/far", Title: "origin/far", Does: "follow it here"},
	}
	if list == nil || list.T != "list" || list.Selected != "l:main" || list.Max != 12 || !reflect.DeepEqual(list.Items, want) || piece(got.View, "files") == nil || piece(got.View, "diff") == nil {
		t.Fatalf("the list = %+v", list)
	}
	if modal := piece(got.View, "switch"); modal == nil || modal.T != "modal" || modal.Title != "Switch branch" || piece(modal, "q") == nil || piece(modal, "branches") == nil || piece(modal, "files") != nil {
		t.Errorf("the modal = %+v", modal)
	}

	if say, placed := view.act(view.deed(gitEvent{E: "change", ID: "q", Text: "flags"}), time.Now()); !say || !placed {
		t.Fatal("words typed narrow nothing")
	}
	got = heard(t, view.messages(true)[0])
	if list := piece(got.View, "branches"); len(list.Items) != 1 || !reflect.DeepEqual(list.Items[0], gitEntry{ID: "n:flags", Title: "Create flags from main", Does: "create"}) || list.Selected != "n:flags" || piece(got.View, "q").Value != nil {
		t.Fatalf("a name no branch has: %+v", list)
	}
	pick := func(id string) {
		t.Helper()
		if say, _ := view.act(view.deed(gitEvent{E: "pick", List: "branches", ID: id}), time.Now()); !say {
			t.Fatalf("⏎ on %s says nothing", id)
		}
	}
	pick("n:flags")
	if repo.Git("symbolic-ref", "--short", "HEAD") != "flags" || view.picking || toasted(view) != (muiStatus{"on flags", "ok"}) || view.status != nil || view.standing.Branch != "flags" {
		t.Fatalf("a branch made: on %q, %+v", repo.Git("symbolic-ref", "--short", "HEAD"), toasted(view))
	}
	if got := heard(t, view.messages(true)[0]); got.Focus != "files" || piece(got.View, "files") == nil || piece(got.View, "branches") != nil {
		t.Errorf("back at the files: focus %q", got.Focus)
	}

	press(t, view, "branches", "")
	pick("r:origin/far")
	if repo.Git("symbolic-ref", "--short", "HEAD") != "far" || repo.Git("rev-parse", "--abbrev-ref", "far@{upstream}") != "origin/far" || view.standing.Upstream != "origin/far" {
		t.Fatalf("a remote's branch followed: on %q, %+v", repo.Git("symbolic-ref", "--short", "HEAD"), view.status)
	}

	// a file changed that the branch has otherwise: refused, and said
	repo.Write("README.md", "mine\n")
	press(t, view, "branches", "")
	pick("l:talk")
	if repo.Git("symbolic-ref", "--short", "HEAD") != "far" || !view.picking || view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "error: Your local changes to the following files would be overwritten") {
		t.Fatalf("a switch git refuses: %+v", view.status)
	}
	if said := texts(*piece(heard(t, view.messages(false)[0]).View, "switch")); len(said) != 1 || said[0] != view.status.Text {
		t.Errorf("the modal says %q of it", said)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "switch"}), time.Now())
	if view.picking {
		t.Error("esc leaves the branches up")
	}
	repo.Git("checkout", "-q", "--", "README.md")
	press(t, view, "branches", "")
	pick("l:talk")
	if repo.Git("symbolic-ref", "--short", "HEAD") != "talk" || view.picking {
		t.Errorf("switched: on %q, %+v", repo.Git("symbolic-ref", "--short", "HEAD"), view.status)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, "notes.txt")); err != nil {
		t.Errorf("the untracked file did not come along: %v", err)
	}
}

// In a real repository with a bare one as its remote: the head says what
// is to push and to pull; f fetches, P pulls only where the branch can be
// moved on and says why where it cannot, and p, asked in a modal first,
// pushes to what the branch follows, or to origin under its own name
// where it follows nothing, and never by force. While one runs a bar at
// the head's end says so in place of the status, and another is refused
// in a toast; how it went is a toast after, and the status where it failed.
func TestFetchPullAndPushAgainstABareRemote(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	bare, other := remote(t, repo)
	repo.Write("a.txt", "a\n")
	repo.Commit("mine: a")
	repo.Write("b.txt", "b\n")
	repo.Commit("mine: b")
	other("main", "theirs.txt")

	view := &gitView{pane: "%3", dir: repo.Root}
	view.read(time.Now())
	if got := texts(*heard(t, view.messages(true)[0]).View); !slices.Equal(got[:3], []string{"main", "2 ahead of origin/main, none behind", "2 to push"}) {
		t.Errorf("texts = %q", got)
	}
	if say, placed := view.act(view.deed(gitEvent{E: "press", ID: "fetch"}), time.Now()); !say || placed || view.work == nil {
		t.Fatal("f leaves nothing to be done")
	}
	bar := func() string {
		got := heard(t, view.messages(false)[0])
		if at := piece(got.View, "remote"); at != nil && at.T == "progress" && got.Status == nil {
			return at.Text
		}
		return ""
	}
	if bar() != "fetching origin…" || view.status != nil {
		t.Fatalf("while it fetches: %q, %+v", bar(), view.status)
	}
	// the keys are taken meanwhile; another deed with the remote waits
	press(t, view, "again", "")
	if bar() != "fetching origin…" {
		t.Errorf("after another key: %q", bar())
	}
	work := view.work
	press(t, view, "pull", "")
	if toasted(view) != (muiStatus{"fetching origin: wait for it to end", "bad"}) || bar() != "fetching origin…" {
		t.Errorf("a pull while it fetches: %+v", toasted(view))
	}
	view.work = work
	ran(t, view)
	if got := toasted(view); got.Tone != "ok" || !strings.Contains(got.Text, "main") || !strings.Contains(got.Text, "origin/main") || view.busy != "" || bar() != "" || view.status != nil {
		t.Fatalf("fetched: %+v", got)
	}
	if got := texts(*heard(t, view.messages(true)[0]).View); got[2] != "2 to push, 1 to pull" {
		t.Errorf("texts = %q", got)
	}

	// neither side is only ahead: nothing is merged, rebased or forced
	head, theirs := repo.Git("rev-parse", "HEAD"), repo.GitIn(bare, "rev-parse", "main")
	press(t, view, "pull", "")
	if bar() != "pulling from origin/main…" {
		t.Errorf("while it pulls: %q", bar())
	}
	ran(t, view)
	if view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "fast-forward") || repo.Git("rev-parse", "HEAD") != head {
		t.Fatalf("a pull that cannot fast-forward: %+v", view.status)
	}
	press(t, view, "push", "")
	if want := []string{"Push 2 commits to origin/main?", "Nothing is forced: a remote that has moved on refuses it.", "yes=Push p", "no=Keep things as they are esc"}; !slices.Equal(asks(t, view), want) || view.work != nil {
		t.Fatalf("p once: %q", asks(t, view))
	}
	press(t, view, "push", "")
	if bar() != "pushing to origin/main…" || asks(t, view) != nil {
		t.Errorf("while it pushes: %q", bar())
	}
	ran(t, view)
	if view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "rejected") || repo.GitIn(bare, "rev-parse", "main") != theirs {
		t.Fatalf("a push the remote refuses: %+v", view.status)
	}

	// a branch that follows nothing is pushed to origin under its own name, and follows it
	repo.Git("checkout", "-q", "-b", "talk")
	view.read(time.Now())
	press(t, view, "pull", "")
	if view.status == nil || *view.status != (muiStatus{"talk follows no branch that is there: there is nothing to pull", "bad"}) {
		t.Errorf("a pull of a branch that follows nothing: %+v", view.status)
	}
	press(t, view, "push", "")
	if got := asks(t, view); len(got) != 4 || got[0] != "Push 2 commits to origin/talk?" {
		t.Fatalf("p once: %q", got)
	}
	// a commit made while the modal was up: asked again, with the count as it now is
	repo.Write("c.txt", "c\n")
	repo.Commit("mine: c")
	head = repo.Git("rev-parse", "HEAD")
	view.read(time.Now())
	press(t, view, "yes", "")
	if got := asks(t, view); len(got) != 4 || got[0] != "Push 3 commits to origin/talk?" || view.work != nil {
		t.Fatalf("its button after another commit: %q", got)
	}
	press(t, view, "yes", "")
	ran(t, view)
	if toasted(view).Tone != "ok" || view.status != nil || repo.GitIn(bare, "rev-parse", "talk") != head || view.standing.Upstream != "origin/talk" || view.standing.ToPush != 0 {
		t.Fatalf("pushed: %+v, follows %q", toasted(view), view.standing.Upstream)
	}
	press(t, view, "push", "")
	if view.status == nil || *view.status != (muiStatus{"nothing to push: origin/talk has every commit", "bad"}) {
		t.Errorf("p with nothing to push: %+v", view.status)
	}

	// what the remote has more is pulled where the branch is only behind
	other("talk", "more.txt")
	press(t, view, "fetch", "")
	ran(t, view)
	if view.standing.ToPull != 1 {
		t.Fatalf("%d to pull, %+v", view.standing.ToPull, view.status)
	}
	press(t, view, "pull", "")
	ran(t, view)
	if toasted(view).Tone != "ok" || repo.Git("rev-parse", "HEAD") != repo.GitIn(bare, "rev-parse", "talk") || view.standing.ToPull != 0 {
		t.Errorf("pulled: %+v", toasted(view))
	}
	press(t, view, "fetch", "")
	ran(t, view)
	if toasted(view) != (muiStatus{"fetched origin: nothing new", "ok"}) {
		t.Errorf("a fetch of nothing new: %+v", toasted(view))
	}

	// with no remote at all there is nothing to fetch from
	lone := gittest.New(t)
	view = &gitView{dir: lone.Root}
	view.read(time.Now())
	press(t, view, "fetch", "")
	if view.work != nil || view.status == nil || view.status.Tone != "bad" || !strings.HasPrefix(view.status.Text, "there is no remote named origin") {
		t.Errorf("a fetch with no remote: %+v", view.status)
	}
}

// A view with no folder of its own does nothing: git run with none runs
// in the folder mia is in, on whatever repository that is. Every deed
// that writes, done on such a view while mia is in a repository with a
// commit, changes, an untracked file and a stash, leaves that repository
// as it was and says so; and reading how it stands reads nothing of it.
func TestAViewWithNoFolderRunsGitNowhere(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	remote(t, repo)
	repo.Write("held.txt", "held\n")
	repo.Git("stash", "push", "-q", "--include-untracked", "-m", "held")
	repo.Write("talk.txt", "one\n")
	repo.Commit("A file says one")
	repo.Write("talk.txt", "one\ntwo\n")
	repo.Git("add", "talk.txt")
	repo.Write("talk.txt", "one\ntwo\nthree\n")
	repo.Write("notes.txt", "kept\n")
	stands := func() string {
		return strings.Join([]string{repo.Git("rev-parse", "HEAD"), repo.Git("log", "-1", "--format=%B"), repo.Git("symbolic-ref", "HEAD"), repo.Git("status", "--porcelain"),
			repo.Git("diff"), repo.Git("diff", "--cached"), repo.Git("stash", "list"), repo.Git("for-each-ref"), repo.Git("config", "--get-regexp", "^branch\\.")}, "\n--\n")
	}
	before := stands()
	t.Chdir(repo.Root)

	now := time.Now()
	view := &gitView{}
	if view.read(now); view.failed == "" || view.top != "" || len(view.rows) != 0 || view.standing.Branch != "" {
		t.Fatalf("read with no folder: %q, top %q, rows %v", view.failed, view.top, ids(view))
	}
	files := git.ParseDiff(repo.Git("diff") + "\n")
	change := git.Change{Path: "talk.txt"}
	stash := git.Stash{SHA: repo.Git("rev-parse", "stash@{0}")}
	line, err := files[0].LinePatch(0, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, deed := range []gitDeed{
		{do: gitStage, row: gitRow{group: 'u', change: git.Change{Path: "notes.txt"}}},
		{do: gitUnstage, row: gitRow{group: 's', change: change}},
		{do: gitStageHunk, patch: files[0].Patch(0)},
		{do: gitUnstageHunk, patch: files[0].Patch(0)},
		{do: gitStageLine, patch: line},
		{do: gitUnstageLine, patch: line},
		{do: gitStageAll},
		{do: gitCommit, text: "Never made"},
		{do: gitAmend, text: "Other words"},
		{do: gitAmend},
		{do: gitUndo},
		{do: gitDiscard, row: gitRow{group: 'c', change: change}},
		{do: gitDiscard, row: gitRow{group: 'u', change: git.Change{Path: "notes.txt"}}},
		{do: gitDiscardHunk, row: gitRow{group: 'c', change: change}, patch: files[0].Patch(0)},
		{do: gitStash},
		{do: gitStashApply, row: gitRow{group: 't', stash: stash}},
		{do: gitStashPop, row: gitRow{group: 't', stash: stash}},
		{do: gitStashDrop, row: gitRow{group: 't', stash: stash}},
		{do: gitSwitch, text: "n:never-made"},
		{do: gitSwitch, text: "l:main"},
		{do: gitSwitch, text: "r:origin/main"},
		{do: gitBranches},
		{do: gitAgain},
		{do: gitFetch},
		{do: gitPull},
		{do: gitPush},
	} {
		view.standing = git.Standing{Branch: "main", Head: "0123456789", Upstream: "origin/main", ToPush: 1, Staged: []git.Change{change}}
		view.status, view.picking = nil, false
		if say, _ := view.act(deed, now); !say || view.status == nil || *view.status != (muiStatus{"mia git has no folder to run git in: nothing is done", "bad"}) || view.work != nil || view.busy != "" {
			t.Errorf("deed %d with no folder: %+v", deed.do, view.status)
		}
		if after := stands(); after != before {
			t.Fatalf("deed %d with no folder changed the repository mia is in:\n%s\nwas\n%s", deed.do, after, before)
		}
	}
	// a row come under the keys reads no diff of it
	if view.act(gitDeed{do: gitMove, row: gitRow{id: "c:talk.txt", group: 'c', change: change}}, now); len(view.lines) != 0 || view.failed != "" {
		t.Errorf("a move with no folder read %+v, %q", view.lines, view.failed)
	}
	// what a modal asks, confirmed by its button and by its key, is not
	// done either; nor is it by esc
	view.standing = git.Standing{Branch: "main", Head: "0123456789", Upstream: "origin/main", ToPush: 1, Changed: []git.Change{change}}
	view.rows = []gitRow{
		{id: "c:talk.txt", group: 'c', change: change},
		{id: "u:notes.txt", group: 'u', change: git.Change{Path: "notes.txt"}},
		{id: "k:0123456", group: 'k', commit: git.Commit{SHA: "0123456", Subject: "A file says one"}},
		{id: "t:held", group: 't', stash: stash},
	}
	view.at, view.files = "c:talk.txt", files
	view.lines, view.hunks = gitCode(files)
	for _, asked := range []gitEvent{
		{E: "press", ID: "discard", Item: "c:talk.txt"},
		{E: "press", ID: "discard", Item: "u:notes.txt"},
		{E: "press", ID: "discard", Item: "1"},
		{E: "press", ID: "discard", Item: "t:held"},
		{E: "press", ID: "undo", Item: "k:0123456"},
		{E: "press", ID: "push", Item: "c:talk.txt"},
	} {
		for _, answer := range []gitEvent{{E: "press", ID: "yes"}, asked, {E: "dismiss", ID: "sure"}, {E: "press", ID: "no"}} {
			view.status = nil
			if view.act(view.deed(asked), now); asks(t, view) == nil || view.status != nil {
				t.Fatalf("%+v with no folder puts up no modal: %+v", asked, view.status)
			}
			view.act(view.deed(answer), now)
			if done := answer.ID == "yes" || answer == asked; asks(t, view) != nil || view.work != nil || view.busy != "" || done != (view.status != nil && *view.status == muiStatus{"mia git has no folder to run git in: nothing is done", "bad"}) {
				t.Errorf("%+v answered by %+v with no folder: %+v", asked, answer, view.status)
			}
			if after := stands(); after != before {
				t.Fatalf("%+v answered by %+v with no folder changed the repository mia is in:\n%s\nwas\n%s", asked, answer, after, before)
			}
		}
	}
}
