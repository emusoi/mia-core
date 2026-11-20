package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

var reviewNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// sawaFake is sw as a test hands it to the view: it answers from the
// review it holds, does to it what the verbs that write do, refuses the
// verbs it is told to, and keeps the arguments of each run.
type sawaFake struct {
	state  sawaState
	rows   map[string][]sawaRow
	layers []sawaLayer
	refuse map[string]string
	ran    [][]string
}

func (f *sawaFake) run(args ...string) ([]byte, error) {
	f.ran = append(f.ran, args)
	if why, refused := f.refuse[args[0]]; refused {
		out, _ := json.Marshal(sawaAnswer{Message: why, Fix: "do the other thing"})
		return out, errors.New("exit status 1")
	}
	flag := func(name string) string {
		if at := slices.Index(args, name); at >= 0 && at+1 < len(args) {
			return args[at+1]
		}
		return ""
	}
	finding := func(id string) *sawaFinding {
		at := slices.IndexFunc(f.state.Threads, func(t sawaFinding) bool { return t.ID == id })
		return &f.state.Threads[at]
	}
	var data any = f.state
	switch args[0] {
	case "list":
		data = []sawaListed{{Slug: f.state.Slug, Kind: f.state.Target.Kind, Target: f.state.Target}}
	case "rows":
		data = []sawaRows{{File: flag("-f"), Rows: f.rows[flag("-f")]}}
	case "read":
		for i := range f.state.Files {
			if f.state.Files[i].Path == flag("-f") {
				f.state.Files[i].Read = true
			}
		}
	case "reply":
		told := finding(args[1])
		told.Posts = append(told.Posts, sawaPost{Author: "user", At: reviewNow, Body: flag("-m")})
	case "resolve":
		finding(args[1]).State = "resolved"
	case "reopen":
		told := finding(args[1])
		if told.State = "open"; flag("-m") != "" {
			told.Posts = append(told.Posts, sawaPost{Author: "user", At: reviewNow, Body: flag("-m")})
		}
	case "apply":
		finding(args[1]).AppliedAt = &reviewNow
	case "gate":
		for i := range f.state.Gates {
			if f.state.Gates[i].Name == args[1] {
				f.state.Gates[i].Gate = &sawaJudged{State: args[2], Note: flag("-m"), By: "user", At: reviewNow}
			}
		}
	case "stack":
		data = sawaStack{Layers: f.layers}
	case "open":
		if flag("-r") != "" && f.layers != nil { // a layer's review
			data = sawaListed{Slug: "range_" + flag("-r")}
		}
	case "verdict":
		f.state.Verdict = sawaVerdict{Verdict: args[1], Label: args[1] + " given"}
	case "add":
		line, _ := strconv.Atoi(flag("-l"))
		filed := sawaFinding{ID: "t9", File: flag("-f"), Line: line, Side: flag("--side"), Type: "note", State: "open",
			Posts: []sawaPost{{Author: "user", At: reviewNow, Body: flag("-m")}}}
		f.state.Threads = append(f.state.Threads, filed)
		for i, row := range f.rows[filed.File] {
			if filed.Side == "new" && row.New == line || filed.Side == "old" && row.Old == line && row.Kind == "del" {
				f.rows[filed.File][i].Threads = append(row.Threads, filed.ID)
			}
		}
		data = filed
	}
	raw, _ := json.Marshal(data)
	out, _ := json.Marshal(sawaAnswer{OK: true, Data: raw})
	return out, nil
}

// asked is the runs of sw since the last time it was asked for them, but
// those that only read.
func (f *sawaFake) asked() [][]string {
	var wrote [][]string
	for _, args := range f.ran {
		if args[0] != "state" && args[0] != "rows" && args[0] != "list" && args[0] != "stack" {
			wrote = append(wrote, args)
		}
	}
	f.ran = nil
	return wrote
}

// reviewFixed is a review as sawa says it, the one mia desk's fixture
// draws: seven files, two of them read; two findings that stand and one
// resolved on src/talk.rs, one that stands on src/mui/patch.rs; and a
// reading path through three of the files.
func reviewFixed() *sawaFake {
	ago := func(minutes int) time.Time { return reviewNow.Add(-time.Duration(minutes) * time.Minute) }
	state := sawaState{
		Slug: "range_main..HEAD", Label: "main..HEAD", Target: sawaTarget{Kind: "range", Revset: "main..HEAD"},
		Summary: "Adds patch messages to Mui: the UI is said whole once,\nthen its pieces are changed by id.",
		Files: []sawaFile{
			{Path: "docs/apps.md", Added: 31, Removed: 4},
			{Path: "examples/mui-chat.py", Added: 22, Removed: 9},
			{Path: "src/chatlook.rs", Added: 6, Removed: 2},
			{Path: "src/mui.rs", Added: 48, Removed: 12, Read: true},
			{Path: "src/mui/patch.rs", Added: 96, Read: true},
			{Path: "src/talk.rs", Added: 15, Removed: 3},
			{Path: "tests/mui_patch.rs", Added: 64},
		},
		Threads: []sawaFinding{
			{ID: "t1", File: "src/talk.rs", Line: 245, Side: "new", Type: "issue", Severity: "major", State: "open", Posts: []sawaPost{
				{Author: "sawa", At: ago(4), Body: "self.view.clone() copies the whole view for every patch.\nCheck every id first and then change in place."},
				{Author: "user", At: ago(2), Body: "It is what makes it all or none."},
			}},
			{ID: "t2", File: "src/talk.rs", Line: 247, Side: "new", Type: "suggestion", Severity: "minor", State: "addressed", Posts: []sawaPost{
				{Author: "sawa", At: ago(9), Body: "Dropped does not say which change named no piece"},
			}},
			{ID: "t3", File: "src/mui/patch.rs", Line: 2, Side: "new", Type: "issue", State: "open", Posts: []sawaPost{
				{Author: "sawa", At: ago(30), Body: "A patch of no changes is taken as one"},
			}},
			{ID: "t4", File: "src/talk.rs", Line: 246, Side: "new", Type: "note", State: "resolved", Posts: []sawaPost{
				{Author: "sawa", At: ago(50), Body: "The loop is the old one"},
			}},
		},
		Path: []sawaStop{
			{File: "src/mui/patch.rs", Why: "what a change is, and how one is made"},
			{File: "src/mui.rs", Why: "a message is a view or a patch"},
			{File: "src/mui/patch.rs", Why: "the same file again"},
			{File: "src/talk.rs", Why: "where a patch is heard: all or none"},
		},
		Verdict: sawaVerdict{Label: "no verdict"},
	}
	state.Counts.Added, state.Counts.Removed = 282, 30
	return &sawaFake{state: state, rows: map[string][]sawaRow{
		"src/talk.rs": {
			{Kind: "add", New: 219, Text: "\t\t}"},
			{Kind: "add", New: 220, Text: "\t\tBody::Patch(changes) => ui.patch(changes),"},
			{Kind: "context", Old: 217, New: 221, Text: "    }"},
			{Kind: "context", Old: 239, New: 243, Text: "    /// Changes are made in turn, and all or none."},
			{Kind: "del", Old: 240, Text: "    fn patch(&mut self) {"},
			{Kind: "add", New: 244, Text: "    fn patch(&mut self, changes: Vec<Change>) -> Heard {"},
			{Kind: "add", New: 245, Text: "        let mut next = self.view.clone();", Threads: []string{"t1"}},
			{Kind: "add", New: 246, Text: "        for change in &changes {", Threads: []string{"t4"}},
			{Kind: "add", New: 247, Text: "                return Heard::Dropped;", Threads: []string{"t2"}},
			{Kind: "context", Old: 241, New: 248, Text: "    }"},
		},
		"src/mui/patch.rs": {
			{Kind: "add", New: 1, Text: "pub struct Change;"},
			{Kind: "add", New: 2, Text: "pub fn apply() {}", Threads: []string{"t3"}},
		},
	}}
}

// reviewOf is the view of a fake's review, read once.
func reviewOf(fake *sawaFake) *reviewView {
	view := &reviewView{pane: "%1", run: fake.run, slug: fake.state.Slug, branch: "mui-patch", typed: map[string]string{}}
	view.read(reviewNow)
	fake.ran = nil
	return view
}

// reviewHeard is a message of mia review as the window reads it.
type reviewHeard struct {
	V      int          `json:"v"`
	Pane   string       `json:"pane"`
	Fill   bool         `json:"fill"`
	Focus  string       `json:"focus"`
	Status *muiStatus   `json:"status"`
	View   *reviewPiece `json:"view"`
	Keys   []gitKey     `json:"keys"`
	Patch  []gitOp      `json:"patch"`
}

// reviewSaid is the messages of a view read back, none of which the
// window fails: the first, and the lines of the code all of them say,
// those in the view and then those each patch puts after them.
func reviewSaid(t *testing.T, messages [][]byte) (reviewHeard, []reviewLine) {
	t.Helper()
	var first reviewHeard
	var lines []reviewLine
	for i, message := range messages {
		if len(message) > muiLimit {
			t.Fatalf("a message of %d bytes, over the %d the window takes", len(message), muiLimit)
		}
		if at := slices.IndexFunc(message, func(b byte) bool { return b >= 0x80 || b < 0x20 }); at >= 0 {
			t.Fatalf("byte %#x in a message: %.80q", message[at], message)
		}
		var got reviewHeard
		if err := json.Unmarshal(message, &got); err != nil {
			t.Fatalf("%v in %.200s", err, message)
		}
		var raw []json.RawMessage
		if i == 0 {
			first = got
			if code := reviewPieceOf(got.View, "diff"); code != nil && code.Lines != nil {
				if err := json.Unmarshal(code.Lines, &raw); err != nil {
					t.Fatal(err)
				}
			}
		} else if len(got.Patch) != 1 || got.Patch[0].Op != "lines" || got.Patch[0].ID != "diff" {
			t.Fatalf("message %d is not lines put after the diff's: %.200s", i, message)
		} else {
			raw = got.Patch[0].Lines
		}
		for _, one := range raw {
			var line reviewLine
			if err := json.Unmarshal(one, &line); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
	}
	return first, lines
}

// reviewPieceOf is the piece of an id in a view, and reviewTexts every
// text in a piece in order.
func reviewPieceOf(in *reviewPiece, id string) *reviewPiece {
	if in == nil || in.ID == id {
		return in
	}
	for i := range in.Children {
		if found := reviewPieceOf(&in.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

func reviewTexts(in reviewPiece) []string {
	var all []string
	if in.T == "text" {
		all = append(all, in.Text)
	}
	for _, child := range in.Children {
		all = append(all, reviewTexts(child)...)
	}
	return all
}

// brief is a line of the code in a few words: its kind, its numbers and
// its words, a note by its label.
func brief(lines []reviewLine) []string {
	var all []string
	for _, line := range lines {
		switch line.Kind {
		case "note":
			all = append(all, "note "+line.Label)
		case "head", "gap":
			all = append(all, line.Kind+" "+line.Text)
		default:
			all = append(all, strings.TrimSpace(fmt.Sprintf("%s %d %d", line.Kind, line.Old, line.New)))
		}
	}
	return all
}

// By folder each folder is open over its files, a file says its lines,
// its findings that stand and whether it is read; in reading order the
// files sawa's path passes come first, in its order, each with why.
func TestTheFilesAreListedByFolderAndInReadingOrder(t *testing.T) {
	state := reviewFixed().state
	state.Files = append(state.Files, sawaFile{Path: "README.md", Added: 1}, sawaFile{Path: "7", Added: 2, Read: true, Stale: true})

	entries, rows := reviewList(state, false, reviewStanding, reviewRowsMost)
	want := []reviewEntry{
		{Head: "Files", Tag: "by folder", Note: "2 of 9 read"},
		{ID: "f:7", Title: "7", Right: "+2"},
		{ID: "README.md", Title: "README.md", Right: "+1"},
		{ID: "d:docs/", Title: "docs/", Open: true},
		{ID: "docs/apps.md", Title: "apps.md", Depth: 1, Right: "+31 -4"},
		{ID: "d:examples/", Title: "examples/", Open: true},
		{ID: "examples/mui-chat.py", Title: "mui-chat.py", Depth: 1, Right: "+22 -9"},
		{ID: "d:src/", Title: "src/", Open: true},
		{ID: "src/chatlook.rs", Title: "chatlook.rs", Depth: 1, Right: "+6 -2"},
		{ID: "src/mui.rs", Title: "mui.rs", Depth: 1, Right: "+48 -12", Checked: true},
		{ID: "src/talk.rs", Title: "talk.rs", Depth: 1, Right: "+15 -3", Note: "2 findings"},
		{ID: "d:src/mui/", Title: "src/mui/", Open: true},
		{ID: "src/mui/patch.rs", Title: "patch.rs", Depth: 1, Right: "+96", Note: "1 finding", Checked: true},
		{ID: "d:tests/", Title: "tests/", Open: true},
		{ID: "tests/mui_patch.rs", Title: "mui_patch.rs", Depth: 1, Right: "+64"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("by folder the list is\n%+v\nwant\n%+v", entries, want)
	}
	if len(rows) != 9 || rows[0] != (reviewRow{"f:7", "7"}) || rows[8] != (reviewRow{"tests/mui_patch.rs", "tests/mui_patch.rs"}) {
		t.Errorf("its files are %v", rows)
	}

	entries, rows = reviewList(state, true, reviewStanding, reviewRowsMost)
	want = []reviewEntry{
		{Head: "Files", Tag: "reading order", Note: "2 of 9 read"},
		{ID: "src/mui/patch.rs", Title: "src/mui/patch.rs", Note: "what a change is, and how one is made", Right: "1 finding  +96", Checked: true},
		{ID: "src/mui.rs", Title: "src/mui.rs", Note: "a message is a view or a patch", Right: "+48 -12", Checked: true},
		{ID: "src/talk.rs", Title: "src/talk.rs", Note: "where a patch is heard: all or none", Right: "2 findings  +15 -3"},
		{ID: "f:7", Title: "7", Right: "+2"},
		{ID: "README.md", Title: "README.md", Right: "+1"},
	}
	if !reflect.DeepEqual(entries[:6], want) {
		t.Errorf("in reading order the list begins\n%+v\nwant\n%+v", entries[:6], want)
	}
	if len(rows) != 9 || rows[0].path != "src/mui/patch.rs" || rows[3].path != "7" {
		t.Errorf("its files are %v", rows)
	}

	// No more files are listed than asked for, and the head says so.
	entries, rows = reviewList(state, false, reviewStanding, 2)
	if len(rows) != 2 || entries[0].Note != "2 of 9 read · 2 shown" {
		t.Errorf("cut to 2 files the list has %d under %q", len(rows), entries[0].Note)
	}
}

// A file's rows are runs of lines under heads, with what sawa left out
// between two as a gap; a finding is a note directly under the line it
// is about, and one that stands no more is left out unless asked for.
func TestAFilesRowsAreCodeWithEachFindingUnderItsLine(t *testing.T) {
	fake := reviewFixed()
	of := func(file string, resolved bool) reviewNotes {
		notes := reviewNotes{now: reviewNow}
		for _, finding := range fake.state.Threads {
			if finding.File == file && (finding.stands() || resolved) {
				notes.findings = append(notes.findings, finding)
			}
		}
		return notes
	}
	lines, marks := reviewCode(fake.rows["src/talk.rs"], of("src/talk.rs", false))
	want := []string{
		"head @@ line 219", "add 0 219", "add 0 220", "217 221",
		"gap 21 lines the same", "head @@ line 243", "239 243", "del 240 0", "add 0 244",
		"add 0 245", "note issue · major", "add 0 246", "add 0 247", "note suggestion · minor", "241 248",
	}
	if got := brief(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("the code is\n%q\nwant\n%q", got, want)
	}
	if len(marks) != len(lines) || marks[10] != (reviewMark{kind: "note", note: "t1"}) || marks[7] != (reviewMark{kind: "del", old: 240}) || marks[3] != (reviewMark{kind: "same", old: 217, new: 221}) {
		t.Errorf("what the lines are: %+v", marks)
	}
	if lines[0].Right != "hunk 1 of 2" || lines[5].Right != "hunk 2 of 2" || lines[1].Text != "        }" {
		t.Errorf("the heads say %q and %q over %q", lines[0].Right, lines[5].Right, lines[1].Text)
	}
	major, minor := lines[10], lines[13]
	if major.Tone != "bad" || major.Text != "self.view.clone() copies the whole view for every patch." || major.Right != "sawa · open" || major.Under != nil {
		t.Errorf("the major issue is %+v", major)
	}
	if minor.Tone != "warn" || minor.Right != "sawa · addressed" {
		t.Errorf("the minor suggestion is %+v", minor)
	}

	lines, _ = reviewCode(fake.rows["src/talk.rs"], of("src/talk.rs", true))
	if got := brief(lines); got[11] != "add 0 246" || got[12] != "note note" || lines[12].Tone != "ok" || lines[12].Right != "sawa · resolved" {
		t.Errorf("with the resolved shown the code is %q, its note %+v", got, lines[12])
	}

	// A file all of whose lines are added is a new file; a finding on no
	// line shown is over them all, and an issue that does not say how bad
	// it is is a warning.
	notes := of("src/mui/patch.rs", false)
	notes.findings = append(notes.findings, sawaFinding{ID: "t8", File: "src/mui/patch.rs", Type: "question", State: "open", Posts: []sawaPost{{Author: "user", Body: "Why a file of its own?"}}})
	lines, _ = reviewCode(fake.rows["src/mui/patch.rs"], notes)
	if got, want := brief(lines), []string{"note question", "head @@ new file", "add 0 1", "add 0 2", "note issue"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the new file's code is %q, want %q", got, want)
	}
	if lines[0].Tone != "" || lines[4].Tone != "warn" {
		t.Errorf("a question's tone is %q and an issue's %q", lines[0].Tone, lines[4].Tone)
	}
	if lines, _ = reviewCode(nil, reviewNotes{}); len(lines) != 1 || lines[0].Kind != "gap" {
		t.Errorf("a file of no rows is %+v", lines)
	}
}

// An open finding is all its words, with the replies, each with who and
// when, the area to answer in and the buttons under them; a comment
// being written is an area under its line with Comment and Cancel.
func TestAnOpenFindingHasItsThreadUnderIt(t *testing.T) {
	fake := reviewFixed()
	typed := "The copy is"
	notes := reviewNotes{findings: fake.state.Threads[:2], open: "t1", typed: typed, now: reviewNow}
	lines, _ := reviewCode(fake.rows["src/talk.rs"], notes)
	open := lines[10]
	if open.Text != "self.view.clone() copies the whole view for every patch.\nCheck every id first and then change in place." || open.Right != "sawa · 4 min ago · open" || open.Label != "issue · major" {
		t.Errorf("the open finding is %+v", open)
	}
	want := []reviewPiece{
		{T: "markdown", Text: "**user** · 2 min ago\n\nIt is what makes it all or none."},
		{T: "field", ID: "reply", Lines: json.RawMessage("3"), Live: true, Placeholder: "Reply", Value: &typed},
		{T: "row", Children: []reviewPiece{
			{T: "button", ID: "send", Label: "Reply", Key: "⌘⏎", Primary: true},
			{T: "button", ID: "resolve", Label: "Reply and resolve", Key: "⌘R"},
		}},
	}
	if !reflect.DeepEqual(open.Under, want) {
		t.Errorf("under it are\n%+v\nwant\n%+v", open.Under, want)
	}
	if lines[13].Under != nil {
		t.Error("the finding that is not open has pieces under it")
	}

	// One with no reply has no markdown, and one resolved Reply and Reopen.
	under := reviewThread(fake.state.Threads[3], "", reviewNow)
	if len(under) != 2 || under[0].T != "field" || len(under[1].Children) != 2 || under[1].Children[0].Label != "Reply" || under[1].Children[1].Label != "Reopen" {
		t.Errorf("under a resolved finding are %+v", under)
	}

	mark := reviewMark{kind: "del", old: 240}
	lines, marks := reviewCode(fake.rows["src/talk.rs"], reviewNotes{writing: &mark, typed: "why", now: reviewNow})
	note := lines[8]
	if marks[8] != (reviewMark{kind: "note", note: reviewNew}) || note.Label != "comment" || note.Right != "line 240, as it was" || len(note.Under) != 2 {
		t.Fatalf("the comment being written is %+v at %+v", note, marks[8])
	}
	if area, buttons := note.Under[0], note.Under[1].Children; area.ID != "comment" || *area.Value != "why" || string(area.Lines) != "3" || !area.Live || buttons[0].Label != "Comment" || buttons[1].ID != "cancel" {
		t.Errorf("under it are %+v", note.Under)
	}
}

// n goes to the finding after where the keys are and N to the one
// before, file by file as listed and down each, and round again.
func TestNAndShiftNGoThroughTheFindingsAcrossFiles(t *testing.T) {
	spots := []reviewSpot{{file: 1, line: 4, n: 2, id: "a"}, {file: 3, line: 2, n: 0, id: "b"}, {file: 3, line: 2, n: 1, id: "c"}, {file: 3, line: 9, n: 3, id: "d"}}
	for _, c := range []struct {
		here    reviewSpot
		forward bool
		want    string
	}{
		{reviewSpot{file: 0, line: 0, n: -1}, true, "a"},
		{reviewSpot{file: 1, line: 4, n: -1}, true, "a"},
		{spots[0], true, "b"},
		{spots[1], true, "c"},
		{reviewSpot{file: 3, line: 5, n: -1}, true, "d"},
		{spots[3], true, "a"},
		{spots[3], false, "c"},
		{spots[2], false, "b"},
		{reviewSpot{file: 3, line: 2, n: -1}, false, "a"},
		{spots[0], false, "d"},
	} {
		if got, _ := reviewStep(spots, c.here, c.forward); got.id != c.want {
			t.Errorf("from %+v, forward %v, the finding is %q, want %q", c.here, c.forward, got.id, c.want)
		}
	}
	if _, found := reviewStep(nil, reviewSpot{}, true); found {
		t.Error("a finding was found among none")
	}

	// In the view: from the first file's first line n is the two findings
	// that stand of src/talk.rs, then the one of src/mui/patch.rs, listed
	// after it, then the first again; the resolved one is passed over.
	fake := reviewFixed()
	view := reviewOf(fake)
	var went []string
	for range 4 {
		say, aim := view.act(view.deed(reviewEvent{E: "press", ID: "next"}), reviewNow)
		if !say || !aim.list || !aim.code || aim.focus != "diff" {
			t.Fatalf("n says %v with %+v", say, aim)
		}
		went = append(went, fmt.Sprintf("%s %s", view.file, view.marks[view.at].note))
	}
	if want := []string{"src/talk.rs t1", "src/talk.rs t2", "src/mui/patch.rs t3", "src/talk.rs t1"}; !reflect.DeepEqual(went, want) {
		t.Errorf("n went to %q, want %q", went, want)
	}
	view.act(view.deed(reviewEvent{E: "press", ID: "prev"}), reviewNow)
	if view.file != "src/mui/patch.rs" || view.marks[view.at].note != "t3" {
		t.Errorf("N went to %s %+v", view.file, view.marks[view.at])
	}
	first, _ := reviewSaid(t, view.messages(reviewAim{list: true, code: true}, reviewNow))
	if list, code := reviewPieceOf(first.View, "files"), reviewPieceOf(first.View, "diff"); list.Selected != "src/mui/patch.rs" || code.Selected != strconv.Itoa(view.at) {
		t.Errorf("the keys are said on %q and line %q", list.Selected, code.Selected)
	}
	if len(fake.asked()) != 0 {
		t.Error("going through the findings wrote to sawa")
	}
}

// The next unread file is the first after the one read that is not, and
// round to the first again; a file read and changed since is not read.
func TestTheNextUnreadFileIsAfterTheOneReadAndRoundAgain(t *testing.T) {
	files := []sawaFile{{Path: "a", Read: true}, {Path: "b"}, {Path: "c", Read: true}, {Path: "d", Read: true, Stale: true}, {Path: "e", Read: true}}
	for after, want := range map[string]string{"a": "b", "b": "d", "c": "d", "d": "b", "e": "b"} {
		if got := reviewUnread(files, after); got != want {
			t.Errorf("after %s the next unread is %q, want %q", after, got, want)
		}
	}
	if got := reviewUnread([]sawaFile{{Path: "a"}, {Path: "b", Read: true}}, "a"); got != "" {
		t.Errorf("with every other file read the next is %q", got)
	}
}

// The view is the shape mia desk draws: a UI that fills its pane, the
// title with the findings that stand and the verdict at its right, what
// the review is of and sawa's summary under it, and a split row of the
// list of files and the code of one.
func TestTheViewIsTheShapeMiaDeskDraws(t *testing.T) {
	t.Setenv("EDITOR", "")
	view := reviewOf(reviewFixed())
	view.act(view.deed(reviewEvent{E: "move", List: "files", ID: "src/talk.rs"}), reviewNow)
	messages := view.messages(reviewAim{list: true, code: true}, reviewNow)
	first, lines := reviewSaid(t, messages)
	if len(messages) != 1 || first.V != 1 || first.Pane != "%1" || !first.Fill || first.Status != nil || first.Focus != "" {
		t.Fatalf("the message is %+v", first)
	}
	wantKeys := []gitKey{{"n", "next finding", "next"}, {"N", "previous finding", "prev"}, {"c", "comment", "comment"}, {"space", "read, next file", "read"},
		{"e", "edit in nvim", "edit"}, {"f", "mine to answer", "filter"}, {"g", "gates", "gates"},
		{"o", "reading order", "order"}, {"u", "show resolved", "resolved"}, {"v", "verdict", "verdict"}, {"r", "ask again", "again"}}
	if !reflect.DeepEqual(first.Keys, wantKeys) {
		t.Errorf("the keys are %+v", first.Keys)
	}
	root := first.View
	if root.T != "col" || len(root.Children) != 3 || root.Children[1].T != "divider" {
		t.Fatalf("the view is %+v", root)
	}
	head, split := root.Children[0], root.Children[2]
	wantHead := []string{"mui-patch", "3 open", "verdict  no verdict", "mui-patch → main · 7 files · +282 -30 · sawa read it 4 min ago",
		"Adds patch messages to Mui: the UI is said whole once, then its pieces are changed by id."}
	if got := reviewTexts(head); !reflect.DeepEqual(got, wantHead) || head.Pad != 14 || head.Children[0].Children[2].Tone != "warn" {
		t.Errorf("the head says %q, want %q", got, wantHead)
	}
	if split.T != "row" || !split.Split || split.Grow != 1 || len(split.Children) != 3 {
		t.Fatalf("the split row is %+v", split)
	}
	left, right := split.Children[0], split.Children[2]
	list := left.Children[0]
	if left.W != 360 || !left.Scroll || list.T != "list" || list.ID != "files" || !list.Live || list.Selected != "src/talk.rs" || len(list.Items) != 13 {
		t.Errorf("the left side is %+v", left)
	}
	if got, want := reviewTexts(right.Children[0]), []string{"src/talk.rs", "file 5 of 7", "2 findings"}; !reflect.DeepEqual(got, want) {
		t.Errorf("over the code it says %q, want %q", got, want)
	}
	code := right.Children[1]
	if code.T != "code" || code.ID != "diff" || !code.Live || code.Selected != "0" || len(lines) != 15 {
		t.Errorf("the code is %+v with %d lines", code, len(lines))
	}
	if !strings.Contains(string(messages[0]), `{"kind":"note","text":"self.view.clone() copies the whole view for every patch.","right":"sawa \u00b7 open","tone":"bad","label":"issue \u00b7 major"}`) {
		t.Errorf("the finding is not said as a note: %s", messages[0])
	}

	// Said again with nothing of where the person is, the keys stay.
	first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if reviewPieceOf(first.View, "files").Selected != "" || reviewPieceOf(first.View, "diff").Selected != "" {
		t.Error("a view said again says where the keys are")
	}

	// v brings the verdicts up in place of the summary; esc takes them away.
	_, aim := view.act(view.deed(reviewEvent{E: "press", ID: "verdict"}), reviewNow)
	first, _ = reviewSaid(t, view.messages(aim, reviewNow))
	verdicts := reviewPieceOf(first.View, "verdict")
	if aim.focus != "verdict" || first.Focus != "verdict" || verdicts == nil || verdicts.Selected != "approve" || len(verdicts.Items) != 4 || first.Keys[0] != (gitKey{"esc", "no verdict now", "shut"}) {
		t.Fatalf("the verdicts are %+v, the keys on %q", verdicts, first.Focus)
	}
	if slices.Contains(reviewTexts(*first.View), wantHead[4]) {
		t.Error("the summary is said with the verdicts up")
	}
	view.act(view.deed(reviewEvent{E: "press", ID: "shut"}), reviewNow)
	if first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow)); reviewPieceOf(first.View, "verdict") != nil {
		t.Error("esc left the verdicts up")
	}

	// o puts the files in reading order, and says so where sawa has none.
	view.act(view.deed(reviewEvent{E: "press", ID: "order"}), reviewNow)
	first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if items := reviewPieceOf(first.View, "files").Items; items[0].Tag != "reading order" || items[1].ID != "src/mui/patch.rs" || first.Keys[7].Label != "by folder" {
		t.Errorf("in reading order the list is %+v", items[:2])
	}
	if got := reviewTexts(first.View.Children[2].Children[2].Children[0]); got[1] != "file 3 of 7" {
		t.Errorf("in reading order the file is %q", got)
	}
	bare := reviewFixed()
	bare.state.Path = nil
	view = reviewOf(bare)
	view.act(view.deed(reviewEvent{E: "press", ID: "order"}), reviewNow)
	if view.reading || view.status == nil || !strings.Contains(view.status.Text, "no reading order") {
		t.Errorf("with no path o gave %v and %+v", view.reading, view.status)
	}
}

// A long diff is said on in patches and ends at the most lines a view
// holds; a long summary, a long finding and a list of long paths are cut
// to what the window takes.
func TestAViewFitsWhatTheWindowTakes(t *testing.T) {
	fake := reviewFixed()
	var rows []sawaRow
	for n := 1; n <= gitLinesMost+200; n++ {
		rows = append(rows, sawaRow{Kind: "add", New: n, Text: strings.Repeat("é", 500) + strconv.Itoa(n)})
	}
	rows[1].Threads = []string{"t3"}
	fake.rows["src/mui/patch.rs"] = rows
	fake.state.Summary = strings.Repeat("word ", 200)
	fake.state.Threads[2].Posts[0].Body = strings.Repeat("ö", 9000) + "\nmore"
	for n := range 600 {
		fake.state.Files = append(fake.state.Files, sawaFile{Path: fmt.Sprintf("zz/%s/%04d.rs", strings.Repeat("ü", 150), n), Added: 1})
	}
	view := reviewOf(fake)
	view.act(view.deed(reviewEvent{E: "move", List: "files", ID: "src/mui/patch.rs"}), reviewNow)
	view.act(view.deed(reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.place("t3"))}), reviewNow)
	messages := view.messages(reviewAim{list: true, code: true, words: true}, reviewNow)
	first, lines := reviewSaid(t, messages)
	if len(messages) < 2 || len(lines) != gitLinesMost+1 || lines[gitLinesMost].Kind != "gap" || lines[gitLinesMost].Text != "202 more lines" {
		t.Errorf("%d messages say %d lines, the last %+v", len(messages), len(lines), lines[len(lines)-1])
	}
	if note := lines[view.place("t3")]; len([]rune(note.Text)) != reviewNoteMost || len(note.Under) != 2 {
		t.Errorf("the open finding has %d characters and %d pieces under it", len([]rune(note.Text)), len(note.Under))
	}
	if len([]rune(lines[1].Text)) != gitLineMost {
		t.Errorf("a line of code has %d characters", len([]rune(lines[1].Text)))
	}
	items := reviewPieceOf(first.View, "files").Items
	if len(items) >= reviewRowsMost || len(items)+len(lines) > muiPieces-40 || !strings.HasSuffix(items[0].Note, "shown") {
		t.Errorf("the list has %d rows under %q", len(items), items[0].Note)
	}
	for _, item := range items {
		if len([]rune(item.ID)) > gitWordsMost || len([]rune(item.Title)) > gitWordsMost {
			t.Fatalf("a row of the list is longer than the window keeps: %+v", item)
		}
	}
	if summary := reviewTexts(first.View.Children[0])[4]; len([]rune(summary)) != reviewSummaryMost+1 || !strings.HasSuffix(summary, "…") {
		t.Errorf("the summary has %d characters", len([]rune(summary)))
	}
}

// Each thing done is asked of sawa in sw's own words, as the person:
// every run names the review, and what was typed goes after -m.
func TestWhatAnEventAsksOfSawa(t *testing.T) {
	fake := reviewFixed()
	view := reviewOf(fake)
	do := func(event reviewEvent) reviewAim {
		t.Helper()
		_, aim := view.act(view.deed(event), reviewNow)
		return aim
	}
	slug := "range_main..HEAD"

	// space on a row of the list marks that file read and shows the next
	// unread one; on a line of the code it is the file shown.
	do(reviewEvent{E: "press", ID: "read", Item: "docs/apps.md"})
	if got, want := fake.asked(), [][]string{{"read", "--json", "-t", slug, "-f", "docs/apps.md"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("space on a file ran %q, want %q", got, want)
	}
	if view.file != "examples/mui-chat.py" {
		t.Errorf("after it the file shown is %s", view.file)
	}
	do(reviewEvent{E: "press", ID: "read", Item: "0"})
	if got, want := fake.asked(), [][]string{{"read", "--json", "-t", slug, "-f", "examples/mui-chat.py"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("space on a line ran %q, want %q", got, want)
	}
	if do(reviewEvent{E: "press", ID: "read", Item: "d:src/"}); len(fake.asked()) != 0 {
		t.Error("space on a folder ran sw")
	}

	// ⏎ on a note opens it with the keys in its area; what is typed is
	// kept when esc shuts it, and sent by ⌘⏎ or Reply.
	do(reviewEvent{E: "move", List: "files", ID: "src/talk.rs"})
	at := strconv.Itoa(view.place("t1"))
	if aim := do(reviewEvent{E: "pick", List: "diff", ID: at}); view.open != "t1" || aim.focus != "reply" || !aim.words || !aim.code {
		t.Fatalf("⏎ on the note opened %q with %+v", view.open, aim)
	}
	do(reviewEvent{E: "change", ID: "reply", Text: "-m is not a flag here"})
	if aim := do(reviewEvent{E: "press", ID: "shut"}); view.open != "" || aim.focus != "diff" || view.marks[view.at].note != "t1" {
		t.Errorf("esc left %q open, the keys at %+v", view.open, view.marks[view.at])
	}
	do(reviewEvent{E: "pick", List: "diff", ID: at})
	if first, lines := reviewSaid(t, view.messages(reviewAim{words: true}, reviewNow)); *lines[view.place("t1")].Under[1].Value != "-m is not a flag here" || first.Keys[0].Key != "esc" {
		t.Error("the words typed were not kept")
	}
	if _, lines := reviewSaid(t, view.messages(reviewAim{}, reviewNow)); lines[view.place("t1")].Under[1].Value != nil {
		t.Error("a view said again says what is typed")
	}
	do(reviewEvent{E: "press", ID: "send"})
	if got, want := fake.asked(), [][]string{{"reply", "t1", "--json", "-t", slug, "-m", "-m is not a flag here"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reply ran %q, want %q", got, want)
	}
	if view.open != "t1" || view.typed["t1"] != "" || view.status.Tone != "ok" {
		t.Errorf("after a reply %q is open with %q typed, the status %+v", view.open, view.typed["t1"], view.status)
	}
	do(reviewEvent{E: "submit", ID: "reply", Text: "one more"})
	if got, want := fake.asked(), [][]string{{"reply", "t1", "--json", "-t", slug, "-m", "one more"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("⌘⏎ ran %q, want %q", got, want)
	}
	if do(reviewEvent{E: "press", ID: "send"}); len(fake.asked()) != 0 || view.status.Tone != "bad" {
		t.Errorf("a reply of no words ran sw, or was not refused: %+v", view.status)
	}

	// Reply and resolve sends the words and then resolves; with none it
	// only resolves. The finding is shut, and left out of the code.
	do(reviewEvent{E: "change", ID: "reply", Text: "measured: fine"})
	do(reviewEvent{E: "press", ID: "resolve"})
	if got, want := fake.asked(), [][]string{{"reply", "t1", "--json", "-t", slug, "-m", "measured: fine"}, {"resolve", "t1", "--json", "-t", slug}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reply and resolve ran %q, want %q", got, want)
	}
	if view.open != "" || view.place("t1") >= 0 {
		t.Errorf("the resolved finding is open (%q) or shown (%d)", view.open, view.place("t1"))
	}
	do(reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.place("t2"))})
	do(reviewEvent{E: "press", ID: "resolve"})
	if got, want := fake.asked(), [][]string{{"resolve", "t2", "--json", "-t", slug}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reply and resolve with no words ran %q, want %q", got, want)
	}

	// c on a line of the code writes a comment under it, filed on that
	// line as it is now, and on a deleted one as it was.
	added := slices.Index(view.marks, reviewMark{kind: "add", new: 246})
	if aim := do(reviewEvent{E: "press", ID: "comment", Item: strconv.Itoa(added)}); view.open != reviewNew || aim.focus != "comment" || view.marks[view.at].note != reviewNew || view.at != added+1 {
		t.Fatalf("c opened %q at %d with %+v", view.open, view.at, aim)
	}
	do(reviewEvent{E: "submit", ID: "comment", Text: "issue: the loop stops early"})
	if got, want := fake.asked(), [][]string{{"add", "--json", "-t", slug, "-f", "src/talk.rs", "-l", "246", "--side", "new", "-m", "issue: the loop stops early"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Comment ran %q, want %q", got, want)
	}
	if view.open != "" || view.marks[view.at].note != "t9" || view.at != added+1 {
		t.Errorf("after it %q is open and the keys are at %+v", view.open, view.marks[view.at])
	}
	deleted := slices.Index(view.marks, reviewMark{kind: "del", old: 240})
	do(reviewEvent{E: "press", ID: "comment", Item: strconv.Itoa(deleted)})
	do(reviewEvent{E: "change", ID: "comment", Text: "why did this go"})
	do(reviewEvent{E: "press", ID: "send"})
	if got, want := fake.asked(), [][]string{{"add", "--json", "-t", slug, "-f", "src/talk.rs", "-l", "240", "--side", "old", "-m", "why did this go"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Comment on a deleted line ran %q, want %q", got, want)
	}
	for _, item := range []string{"0", "src/talk.rs", ""} { // a head, a row of the list, nothing
		if do(reviewEvent{E: "press", ID: "comment", Item: item}); view.open != "" || view.status == nil {
			t.Errorf("c on %q opened %q", item, view.open)
		}
	}
	do(reviewEvent{E: "press", ID: "comment", Item: strconv.Itoa(deleted)})
	if do(reviewEvent{E: "press", ID: "cancel"}); view.open != "" || view.writing != nil || len(fake.asked()) != 0 {
		t.Error("Cancel left the comment up, or ran sw")
	}

	// A verdict picked is given; one sawa does not offer is not asked for.
	do(reviewEvent{E: "press", ID: "verdict"})
	do(reviewEvent{E: "pick", List: "verdict", ID: "changes"})
	if got, want := fake.asked(), [][]string{{"verdict", "changes", "--json", "-t", slug}}; !reflect.DeepEqual(got, want) {
		t.Errorf("a verdict picked ran %q, want %q", got, want)
	}
	if view.judging || view.state.Verdict.Verdict != "changes" {
		t.Errorf("after it the verdicts are up (%v) and the verdict is %+v", view.judging, view.state.Verdict)
	}
	do(reviewEvent{E: "press", ID: "verdict"})
	if do(reviewEvent{E: "pick", List: "verdict", ID: "delete"}); len(fake.asked()) != 0 {
		t.Error("a verdict sawa does not offer ran sw")
	}

	// r has sawa look at the same change again.
	do(reviewEvent{E: "press", ID: "again"})
	if got, want := fake.asked(), [][]string{{"open", "-r", "main..HEAD", "--json"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("r ran %q, want %q", got, want)
	}

	// What only moves the keys, or shows another file, writes nothing.
	for _, event := range []reviewEvent{{E: "move", List: "diff", ID: "3"}, {E: "move", List: "files", ID: "src/mui.rs"}, {E: "pick", List: "files", ID: "src/talk.rs"},
		{E: "press", ID: "next"}, {E: "press", ID: "resolved"}, {E: "press", ID: "order"}, {E: "press", ID: "nothing"}} {
		if do(event); len(fake.asked()) != 0 {
			t.Errorf("%+v wrote to sawa", event)
		}
	}
	if deed := view.deed(reviewEvent{E: "close"}); deed.do != reviewLeave {
		t.Errorf("close asks for %v", deed.do)
	}
}

// What sawa refuses is the view's status, cut to what the window keeps,
// and the view stays as it was; with no sw, or nothing to review, the
// view says so and r asks again.
func TestARefusalIsTheStatusAndTheViewStays(t *testing.T) {
	fake := reviewFixed()
	view := reviewOf(fake)
	fake.refuse = map[string]string{"read": strings.Repeat("no ", 100), "resolve": "an agent claims a fix"}
	say, _ := view.act(view.deed(reviewEvent{E: "press", ID: "read", Item: "docs/apps.md"}), reviewNow)
	if !say || view.status == nil || view.status.Tone != "bad" || len([]rune(view.status.Text)) != gitWordsMost || view.file != "docs/apps.md" {
		t.Fatalf("a refusal left the status %+v on %s", view.status, view.file)
	}
	first, _ := reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if first.Status == nil || first.Status.Tone != "bad" || reviewPieceOf(first.View, "files") == nil {
		t.Errorf("the view says %+v", first.Status)
	}
	// Of a reply and a resolve the reply was sent: it is not typed again.
	view.act(view.deed(reviewEvent{E: "move", List: "files", ID: "src/talk.rs"}), reviewNow)
	view.act(view.deed(reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.place("t1"))}), reviewNow)
	view.act(view.deed(reviewEvent{E: "change", ID: "reply", Text: "done"}), reviewNow)
	view.act(view.deed(reviewEvent{E: "press", ID: "resolve"}), reviewNow)
	if view.status.Text != "an agent claims a fix: do the other thing" || view.open != "t1" || len(fake.state.Threads[0].Posts) != 3 {
		t.Errorf("a resolve refused left %+v, %q open", view.status, view.open)
	}

	missing := &reviewView{pane: "%1", dir: t.TempDir(), typed: map[string]string{}, run: func(...string) ([]byte, error) {
		return nil, &exec.Error{Name: "sw", Err: exec.ErrNotFound}
	}}
	missing.find(reviewNow)
	if !strings.HasPrefix(missing.failed, "nothing to review") {
		t.Errorf("in no repository the view says %q", missing.failed)
	}
	if _, err := sawaAsk[sawaState](missing.run, "state", "--json"); err == nil || !strings.HasPrefix(err.Error(), "sw is not on PATH") {
		t.Errorf("with no sw the error is %v", err)
	}
	first, _ = reviewSaid(t, missing.messages(reviewAim{list: true, code: true}, reviewNow))
	if got := reviewTexts(*first.View); len(got) != 2 || got[1] != missing.failed || !reflect.DeepEqual(first.Keys, []gitKey{{"r", "ask again", "again"}}) {
		t.Errorf("the view with nothing to show says %q with %+v", got, first.Keys)
	}
	if deed := missing.deed(reviewEvent{E: "press", ID: "again"}); deed.do != reviewAgain {
		t.Errorf("r asks for %v", deed.do)
	}

	empty := reviewFixed()
	empty.state.Files, empty.state.Threads = nil, nil
	first, _ = reviewSaid(t, reviewOf(empty).messages(reviewAim{list: true, code: true}, reviewNow))
	if got := reviewTexts(*first.View); !slices.Contains(got, "nothing to review: main..HEAD changes no file") || reviewPieceOf(first.View, "diff") != nil {
		t.Errorf("a review of no files says %q", got)
	}
}

// When sawa's record changes under the view, a finding come in over the
// line under the keys leaves them on that line.
func TestTheKeysStayOnTheirLineWhenAFindingComesIn(t *testing.T) {
	fake := reviewFixed()
	view := reviewOf(fake)
	view.act(view.deed(reviewEvent{E: "move", List: "files", ID: "src/talk.rs"}), reviewNow)
	on := slices.Index(view.marks, reviewMark{kind: "add", new: 247})
	view.act(view.deed(reviewEvent{E: "move", List: "diff", ID: strconv.Itoa(on)}), reviewNow)
	if _, err := fake.run("add", "-f", "src/talk.rs", "-l", "220", "--side", "new", "-m", "said from outside"); err != nil {
		t.Fatal(err)
	}
	if moved := view.read(reviewNow); !moved || view.at != on+1 || view.marks[view.at] != (reviewMark{kind: "add", new: 247}) {
		t.Errorf("after a finding came in the keys are at %d (%+v), moved %v", view.at, view.marks[view.at], view.at != on)
	}
	if moved := view.read(reviewNow); moved {
		t.Error("read again with nothing new, the keys moved")
	}
}

// The review shown is the one sawa has of the branch against its base,
// else the one of this worktree last written to, never a pull request's;
// a target is opened as sw nvim takes it, and sw is run as the person.
func TestTheReviewOfABranchIsFoundElseOpened(t *testing.T) {
	at := func(minutes int) time.Time { return reviewNow.Add(time.Duration(minutes) * time.Minute) }
	listed := []sawaListed{
		{Slug: "pr_o_r_3", Kind: "pr", UpdatedAt: at(9)},
		{Slug: "worktree", Kind: "worktree", UpdatedAt: at(1)},
		{Slug: "range_dev..HEAD", Kind: "range", Target: sawaTarget{Revset: "dev..HEAD"}, UpdatedAt: at(5)},
		{Slug: "range_main..HEAD", Kind: "range", Target: sawaTarget{Revset: "main..HEAD"}, UpdatedAt: at(2)},
	}
	for base, want := range map[string]string{"main": "range_main..HEAD", "trunk": "range_dev..HEAD", "": "range_dev..HEAD"} {
		if got := reviewHas(listed, base); got != want {
			t.Errorf("against %q the review is %q, want %q", base, got, want)
		}
	}
	if got := reviewHas(listed[:1], "main"); got != "" {
		t.Errorf("with only a pull request's the review is %q", got)
	}
	for target, want := range map[string][]string{
		"main..HEAD": {"open", "-r", "main..HEAD", "--json"},
		"pr 14":      {"open", "--pr", "14", "--json"},
		"w":          {"open", "--json"},
		"":           {"open", "--json"},
	} {
		if got := reviewOpenArgs(target); !reflect.DeepEqual(got, want) {
			t.Errorf("%q is opened by %q, want %q", target, got, want)
		}
	}
	for want, target := range map[string]sawaTarget{"open -r a...b --json": {Kind: "range", Revset: "a...b"}, "open --pr 7 --json": {Kind: "pr", Number: 7}, "open -w --json": {Kind: "worktree"}} {
		if got := strings.Join(reviewAgainArgs(target), " "); got != want {
			t.Errorf("%+v is looked at again by %q, want %q", target, got, want)
		}
	}
	env := sawaEnv([]string{"HOME=/h", "SAWA_AGENT=1", "LOCO_AGENT=1", "SAWA_AUTHOR=claude", "SAWA_SNAPSHOT=abc", "SAWA_TARGET=x"})
	if want := []string{"HOME=/h", "SAWA_TARGET=x"}; !reflect.DeepEqual(env, want) {
		t.Errorf("sw is run with %q, want %q", env, want)
	}
}

// Against real sw, in a throwaway repository: the branch's review is
// opened against main, a finding filed through sw is shown under its
// line, and a reply, a resolve and a file read through the view are in
// sawa's record as the person's, though the pane says it is an agent's.
func TestAReviewIsReadAndWrittenThroughSw(t *testing.T) {
	if _, err := exec.LookPath("sw"); err != nil {
		t.Skip("sw is not on PATH")
	}
	gittest.Isolate(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SAWA_TARGET", "")
	os.Unsetenv("SAWA_TARGET")
	t.Setenv("SAWA_AGENT", "1")
	t.Setenv("SAWA_AUTHOR", "sawa")
	repo := gittest.New(t)
	var talk []string
	for n := 1; n <= 40; n++ {
		talk = append(talk, fmt.Sprintf("line %d", n))
	}
	repo.Write("src/talk.txt", strings.Join(talk, "\n")+"\n")
	repo.Commit("talk")
	repo.Git("checkout", "-q", "-b", "words")
	talk[4], talk[29] = "line five", "line thirty"
	repo.Write("src/talk.txt", strings.Join(talk, "\n")+"\n")
	repo.Write("notes.md", "new\n")
	repo.Commit("Talk says five and thirty in words")

	view := &reviewView{pane: "%1", dir: repo.Root, run: sawaIn(repo.Root), typed: map[string]string{}}
	view.find(reviewNow)
	if view.failed != "" || view.status != nil {
		t.Fatalf("the review was not opened: %q %+v", view.failed, view.status)
	}
	if !strings.HasPrefix(view.state.Store, repo.Root+"/.git/sawa/") {
		t.Fatalf("sawa keeps the review at %s, outside the throwaway repository %s", view.state.Store, repo.Root)
	}
	if view.state.Target.Revset != "main..HEAD" || view.title != "Talk says five and thirty in words" || len(view.rows) != 2 || view.file != "notes.md" {
		t.Fatalf("the review is of %+v, titled %q, its files %v", view.state.Target, view.title, view.rows)
	}

	// sawa, an agent, files a finding; the view is told by sawa's record.
	stamp := reviewStamp(view.state.Store)
	file := exec.Command("sw", "add", "--json", "-f", "src/talk.txt", "-l", "30", "--type", "issue", "--severity", "major", "-m", "thirty is not a number\nSay 30.")
	file.Dir = repo.Root
	if out, err := file.CombinedOutput(); err != nil {
		t.Fatalf("sw add: %v\n%s", err, out)
	}
	if stamp == "" || reviewStamp(view.state.Store) == stamp {
		t.Error("sawa's record changed and its stamp did not")
	}
	view.read(time.Now())
	say, aim := view.act(view.deed(reviewEvent{E: "press", ID: "next"}), time.Now())
	if !say || aim.focus != "diff" || view.file != "src/talk.txt" || view.marks[view.at].note != "t1" {
		t.Fatalf("n went to %s %+v", view.file, view.marks[view.at])
	}
	// (Each run is under the words git has over its hunk: of a file of
	// plain words, the line before it that begins with a letter.)
	_, lines := reviewSaid(t, view.messages(aim, time.Now()))
	want := []string{"head @@ line 1", "2 2", "3 3", "4 4", "del 5 0", "add 0 5", "6 6", "7 7", "8 8",
		"gap 18 lines the same", "head @@ line 26", "27 27", "28 28", "29 29", "del 30 0", "add 0 30", "note issue · major", "31 31", "32 32", "33 33"}
	if got := brief(lines); !reflect.DeepEqual(got, want) {
		t.Fatalf("the code is\n%q\nwant\n%q", got, want)
	}
	if note := lines[16]; note.Text != "thirty is not a number" || note.Tone != "bad" || note.Right != "sawa · open" {
		t.Errorf("the finding is %+v", note)
	}

	// The person replies and resolves, comments on a line, and marks the file read.
	do := func(event reviewEvent) {
		t.Helper()
		if view.act(view.deed(event), time.Now()); view.status != nil && view.status.Tone == "bad" {
			t.Fatalf("%+v: %s", event, view.status.Text)
		}
	}
	do(reviewEvent{E: "pick", List: "diff", ID: "16"})
	do(reviewEvent{E: "submit", ID: "reply", Text: "-it is a word\non purpose"})
	do(reviewEvent{E: "change", ID: "reply", Text: "kept as a word"})
	do(reviewEvent{E: "press", ID: "resolve"})
	do(reviewEvent{E: "press", ID: "comment", Item: "4"})
	do(reviewEvent{E: "submit", ID: "comment", Text: "why not 5"})
	do(reviewEvent{E: "press", ID: "read", Item: "src/talk.txt"})
	do(reviewEvent{E: "press", ID: "verdict"})
	do(reviewEvent{E: "pick", List: "verdict", ID: "comment"})

	state, err := sawaAsk[sawaState](sawaIn(repo.Root), "state", "--json", "-t", view.slug)
	if err != nil {
		t.Fatal(err)
	}
	first, second := state.Threads[0], state.Threads[1]
	if first.State != "resolved" || len(first.Posts) != 3 || first.Posts[1].Body != "-it is a word\non purpose" || first.Posts[1].Author != "test" || first.Posts[2].Body != "kept as a word" {
		t.Errorf("sawa has the finding as %+v", first)
	}
	if second.File != "src/talk.txt" || second.Line != 5 || second.Side != "old" || second.Type != "note" || second.first().Author != "test" || second.first().Body != "why not 5" {
		t.Errorf("sawa has the comment as %+v", second)
	}
	if read := slices.IndexFunc(state.Files, func(f sawaFile) bool { return f.Path == "src/talk.txt" && f.Read }); read < 0 || state.Verdict.Verdict != "comment" {
		t.Errorf("sawa has the files as %+v and the verdict %+v", state.Files, state.Verdict)
	}
	if view.file != "notes.md" || view.state.Verdict.Label != "commented" {
		t.Errorf("the view is on %s with the verdict %q", view.file, view.state.Verdict.Label)
	}

	// Run again, the view takes up the review sawa has, and opens none.
	again := &reviewView{pane: "%1", dir: repo.Root, run: sawaIn(repo.Root), typed: map[string]string{}}
	again.find(time.Now())
	if again.slug != view.slug || len(again.state.Threads) != 2 {
		t.Errorf("run again the review is %q with %d findings", again.slug, len(again.state.Threads))
	}
}

// mia review reads what the person does from a terminal, and is refused
// where stdin is none; it takes no flag.
func TestReviewRefusesWhatIsNotATerminal(t *testing.T) {
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
	if code := Run([]string{"review"}); code != exitFailed {
		t.Errorf("review exited %d, want %d", code, exitFailed)
	}
	if code := Run([]string{"review", "--pr", "3"}); code != exitUsage {
		t.Errorf("review --pr 3 exited %d, want %d", code, exitUsage)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Features, "review-app") {
		t.Error("capabilities do not list review-app")
	}
}

// reviewDone does what an event asks of a view, and gives what it then says.
func reviewDone(view *reviewView, event reviewEvent) (bool, reviewAim) {
	return view.act(view.deed(event), reviewNow)
}

// A finding on no file is on the review as a whole: it is listed in the
// header under the summary, by its first line with its label and how it
// stands, counted with the others; n and N reach it before the first
// file's, ⏎ opens it there to reply or reply and resolve as a file's,
// and esc shuts it.
func TestAFindingOnTheReviewAsAWholeIsInTheHeader(t *testing.T) {
	fake := reviewFixed()
	fake.state.Threads = append(fake.state.Threads, sawaFinding{ID: "t5", Type: "question", State: "open", Posts: []sawaPost{
		{Author: "sawa", At: reviewNow.Add(-time.Minute), Body: "Why a patch and not a second view?\nThe description does not say."},
		{Author: "user", At: reviewNow, Body: "A view is too many bytes."},
	}})
	view := reviewOf(fake)
	first, lines := reviewSaid(t, view.messages(reviewAim{list: true, code: true}, reviewNow))
	list := reviewPieceOf(first.View, "general")
	want := []reviewEntry{{Head: "On the review as a whole", Note: "1"}, {ID: "t5", Title: "Why a patch and not a second view?", Right: "sawa · open", Lead: "question"}}
	if list == nil || !reflect.DeepEqual(list.Items, want) || !list.Live || list.Max != 4 || list.Selected != "" {
		t.Fatalf("the list is %+v", list)
	}
	if head := reviewTexts(first.View.Children[0]); head[1] != "4 open" || len(first.View.Children[0].Children) != 4 {
		t.Errorf("the head says %q", head)
	}
	if slices.ContainsFunc(lines, func(line reviewLine) bool { return line.Kind == "note" }) {
		t.Errorf("it is under a line of %s: %q", view.file, brief(lines))
	}

	// N from the first file is on it, n from it on the first file's, and N back
	say, aim := reviewDone(view, reviewEvent{E: "press", ID: "prev"})
	if !say || view.above != "t5" || !aim.above || aim.focus != "general" || view.file != "docs/apps.md" {
		t.Fatalf("N went to %q with %+v", view.above, aim)
	}
	first, _ = reviewSaid(t, view.messages(aim, reviewNow))
	if got := reviewPieceOf(first.View, "general"); got.Selected != "t5" || first.Focus != "general" {
		t.Errorf("the keys are said on %q of %q", got.Selected, first.Focus)
	}
	if _, aim = reviewDone(view, reviewEvent{E: "press", ID: "next"}); view.above != "" || view.file != "src/talk.rs" || view.marks[view.at].note != "t1" || aim.focus != "diff" {
		t.Errorf("n went to %s %+v, the keys over the files on %q", view.file, view.marks[view.at], view.above)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "prev"}); view.above != "t5" || view.file != "src/talk.rs" {
		t.Errorf("N went to %q beside %s", view.above, view.file)
	}
	// the keys come on it by themselves, and leave it for a line of the diff
	view.above = ""
	if say, _ := reviewDone(view, reviewEvent{E: "move", List: "general", ID: "t5"}); say || view.above != "t5" {
		t.Errorf("the keys on it: said %v, %q", say, view.above)
	}
	if reviewDone(view, reviewEvent{E: "move", List: "diff", ID: "2"}); view.above != "" {
		t.Error("the keys on a line of the diff are still on it")
	}
	if deed := view.deed(reviewEvent{E: "pick", List: "general", ID: "t1"}); deed != (reviewDeed{}) {
		t.Errorf("⏎ on a file's finding in that list does %+v", deed)
	}

	// ⏎ opens it under the list: its words, the replies, the area and the buttons
	if _, aim = reviewDone(view, reviewEvent{E: "pick", List: "general", ID: "t5"}); view.open != "t5" || aim.focus != "reply" || !aim.words || !aim.above {
		t.Fatalf("⏎ opened %q with %+v", view.open, aim)
	}
	reviewDone(view, reviewEvent{E: "change", ID: "reply", Text: "said in the description now"})
	first, _ = reviewSaid(t, view.messages(aim, reviewNow))
	head := first.View.Children[0].Children
	if len(head) != 8 || head[3].ID != "general" || head[4].T != "markdown" || head[4].Text != "Why a patch and not a second view?\nThe description does not say." ||
		head[5].Text != "**user** · just now\n\nA view is too many bytes." || head[6].ID != "reply" || *head[6].Value != "said in the description now" || head[7].Children[1].ID != "resolve" || first.Keys[0].Key != "esc" {
		t.Errorf("under the list are %+v", head[3:])
	}
	if first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow)); first.View.Children[0].Children[6].Value != nil {
		t.Error("a view said again says what is typed")
	}
	if _, aim = reviewDone(view, reviewEvent{E: "press", ID: "shut"}); view.open != "" || aim.focus != "general" || view.typed["t5"] == "" {
		t.Errorf("esc left %q open, the keys on %q", view.open, aim.focus)
	}
	reviewDone(view, reviewEvent{E: "pick", List: "general", ID: "t5"})
	reviewDone(view, reviewEvent{E: "press", ID: "send"})
	_, aim = reviewDone(view, reviewEvent{E: "press", ID: "resolve"})
	wantRan := [][]string{{"reply", "t5", "--json", "-t", "range_main..HEAD", "-m", "said in the description now"}, {"resolve", "t5", "--json", "-t", "range_main..HEAD"}}
	if got := fake.asked(); !reflect.DeepEqual(got, wantRan) || aim.focus != "general" {
		t.Errorf("Reply, then Reply and resolve, ran %q with %+v", got, aim)
	}
	if first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow)); reviewPieceOf(first.View, "general") != nil {
		t.Error("resolved, it is still listed")
	}
}

// f shows every open finding, then only those an agent addressed and
// that wait on the person, then the resolved too, and then every open
// one again: the header's count, each file's, the notes in the code and
// where n goes all follow; u shows the resolved, or not.
func TestTheFindingsAreFilteredByWhoTheyWaitOn(t *testing.T) {
	state := reviewFixed().state
	for filter, want := range map[reviewFilter]string{reviewStanding: "3 open", reviewMine: "1 to answer", reviewAll: "3 open, 1 resolved"} {
		if got, asks := reviewStands(state, filter); got != want || !asks {
			t.Errorf("the findings stand as %q (asking: %v), want %q", got, asks, want)
		}
	}
	for filter, want := range map[reviewFilter]string{reviewStanding: "none open", reviewMine: "none to answer", reviewAll: "none open, none resolved"} {
		if got, asks := reviewStands(sawaState{}, filter); got != want || asks {
			t.Errorf("no finding stands as %q (asking: %v), want %q", got, asks, want)
		}
	}
	for _, c := range []struct {
		filter reviewFilter
		want   []bool // of open, addressed, resolved, wontfix
	}{{reviewStanding, []bool{true, true, false, false}}, {reviewMine, []bool{false, true, false, false}}, {reviewAll, []bool{true, true, true, true}}} {
		for n, state := range []string{"open", "addressed", "resolved", "wontfix"} {
			if got := c.filter.takes(sawaFinding{State: state}); got != c.want[n] {
				t.Errorf("filter %d takes one %s: %v", c.filter, state, got)
			}
		}
	}

	view := reviewOf(reviewFixed())
	reviewDone(view, reviewEvent{E: "move", List: "files", ID: "src/talk.rs"})
	shown := func() (head string, file reviewEntry, notes []string, key string) {
		t.Helper()
		first, lines := reviewSaid(t, view.messages(reviewAim{}, reviewNow))
		for at, line := range lines {
			if line.Kind == "note" {
				notes = append(notes, view.marks[at].note)
			}
		}
		items := reviewPieceOf(first.View, "files").Items
		return reviewTexts(first.View.Children[0])[1], items[slices.IndexFunc(items, func(e reviewEntry) bool { return e.ID == "src/talk.rs" })], notes, first.Keys[5].Label
	}
	if head, file, notes, key := shown(); head != "3 open" || file.Note != "2 findings" || !slices.Equal(notes, []string{"t1", "t2"}) || key != "mine to answer" {
		t.Errorf("every open finding: %q, %q, %q, the key %q", head, file.Note, notes, key)
	}
	if say, _ := reviewDone(view, reviewEvent{E: "press", ID: "filter"}); !say || view.filter != reviewMine || view.status == nil {
		t.Fatalf("f: said %v, filter %d", say, view.filter)
	}
	if head, file, notes, key := shown(); head != "1 to answer" || file.Note != "1 finding" || !slices.Equal(notes, []string{"t2"}) || key != "resolved too" {
		t.Errorf("mine to answer: %q, %q, %q, the key %q", head, file.Note, notes, key)
	}
	// n goes only to what is shown, and says so where that is nothing
	for range 2 {
		if reviewDone(view, reviewEvent{E: "press", ID: "next"}); view.marks[view.at].note != "t2" {
			t.Errorf("n went to %+v", view.marks[view.at])
		}
	}
	reviewDone(view, reviewEvent{E: "press", ID: "filter"})
	if head, file, notes, key := shown(); head != "3 open, 1 resolved" || file.Note != "3 findings" || !slices.Equal(notes, []string{"t1", "t4", "t2"}) || key != "all open" {
		t.Errorf("resolved too: %q, %q, %q, the key %q", head, file.Note, notes, key)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "filter"}); view.filter != reviewStanding {
		t.Errorf("f a third time: filter %d", view.filter)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "resolved"}); view.filter != reviewAll {
		t.Errorf("u: filter %d", view.filter)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "resolved"}); view.filter != reviewStanding {
		t.Errorf("u again: filter %d", view.filter)
	}
	none := reviewFixed()
	none.state.Threads[1].State = "open"
	view = reviewOf(none)
	reviewDone(view, reviewEvent{E: "press", ID: "filter"})
	if reviewDone(view, reviewEvent{E: "press", ID: "next"}); *view.status != (muiStatus{Text: "no finding waits on you"}) {
		t.Errorf("n with none to answer: %+v", view.status)
	}
}

// A finding that suggests an edit shows it under its words as a small
// diff, the lines as they were deleted and the lines suggested added,
// with Apply, which has sawa make the edit in the working tree; once it
// is applied the thread says when, and Apply is gone. A finding that
// stands no more has Reopen, which sends it back with the words typed.
func TestASuggestedEditIsAppliedAndAResolvedFindingReopened(t *testing.T) {
	finding := sawaFinding{Line: 247, Suggest: "\t\treturn Heard::Dropped(n);\n\t\t// which change\n", SuggestFrom: "\t\treturn Heard::Dropped;"}
	want := []reviewLine{{Kind: "del", Old: 247, Text: "        return Heard::Dropped;"}, {Kind: "add", New: 247, Text: "        return Heard::Dropped(n);"}, {Kind: "add", New: 248, Text: "        // which change"}}
	if got := reviewSuggested(finding, reviewEditMost); !reflect.DeepEqual(got, want) {
		t.Errorf("the edit is %+v", got)
	}
	if got := reviewSuggested(finding, 2); len(got) != 3 || got[2].Kind != "gap" || got[2].Text != "1 more line" {
		t.Errorf("cut to two lines the edit is %+v", got)
	}
	if got := reviewSuggested(sawaFinding{Line: 3}, 9); got != nil {
		t.Errorf("a finding that suggests nothing has %+v", got)
	}

	fake := reviewFixed()
	fake.state.Threads[1].Suggest, fake.state.Threads[1].SuggestFrom = finding.Suggest, finding.SuggestFrom
	view := reviewOf(fake)
	reviewDone(view, reviewEvent{E: "move", List: "files", ID: "src/talk.rs"})
	if deed := view.deed(reviewEvent{E: "press", ID: "apply"}); deed != (reviewDeed{}) {
		t.Errorf("Apply with nothing open does %+v", deed)
	}
	reviewDone(view, reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.place("t2"))})
	_, lines := reviewSaid(t, view.messages(reviewAim{words: true}, reviewNow))
	under := lines[view.place("t2")].Under
	var edit []reviewLine
	if len(under) != 3 || under[0].T != "code" || under[0].ID != "suggested" || json.Unmarshal(under[0].Lines, &edit) != nil || !reflect.DeepEqual(edit, want) {
		t.Fatalf("under the finding are %+v", under)
	}
	if buttons := under[2].Children; len(buttons) != 3 || !reflect.DeepEqual(buttons[2], reviewPiece{T: "button", ID: "apply", Label: "Apply"}) {
		t.Errorf("the buttons are %+v", buttons)
	}
	reviewDone(view, reviewEvent{E: "press", ID: "apply"})
	if got, want := fake.asked(), [][]string{{"apply", "t2", "--json", "-t", "range_main..HEAD"}}; !reflect.DeepEqual(got, want) || *view.status != (muiStatus{Text: "t2 applied to the working tree", Tone: "ok"}) {
		t.Errorf("Apply ran %q, status %+v", got, view.status)
	}
	_, lines = reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	under = lines[view.place("t2")].Under
	if len(under) != 4 || !reflect.DeepEqual(under[1], reviewPiece{T: "text", Text: "applied just now", Style: "dim", Tone: "ok"}) || len(under[3].Children) != 2 {
		t.Errorf("once applied, under the finding are %+v", under)
	}
	// what sawa refuses — the file changed since — is the status
	fake.state.Threads[1].AppliedAt = nil
	fake.refuse = map[string]string{"apply": "`src/talk.rs` has changed since this was suggested; read it again before applying"}
	view.read(reviewNow)
	if reviewDone(view, reviewEvent{E: "press", ID: "apply"}); view.status.Tone != "bad" || !strings.Contains(view.status.Text, "has changed since") {
		t.Errorf("a refusal: %+v", view.status)
	}
	fake.refuse = nil
	fake.asked()

	// a resolved finding, shown by u, is reopened with what is typed
	reviewDone(view, reviewEvent{E: "press", ID: "shut"})
	if deed := view.deed(reviewEvent{E: "press", ID: "reopen"}); deed != (reviewDeed{}) {
		t.Errorf("Reopen with nothing open does %+v", deed)
	}
	reviewDone(view, reviewEvent{E: "press", ID: "resolved"})
	reviewDone(view, reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.place("t4"))})
	_, lines = reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if buttons := lines[view.place("t4")].Under[1].Children; len(buttons) != 2 || !reflect.DeepEqual(buttons[1], reviewPiece{T: "button", ID: "reopen", Label: "Reopen"}) {
		t.Errorf("under a resolved finding the buttons are %+v", buttons)
	}
	reviewDone(view, reviewEvent{E: "change", ID: "reply", Text: "it is not the old loop"})
	reviewDone(view, reviewEvent{E: "press", ID: "reopen"})
	if got, want := fake.asked(), [][]string{{"reopen", "t4", "--json", "-t", "range_main..HEAD", "-m", "it is not the old loop"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reopen ran %q, want %q", got, want)
	}
	if finding, _ := view.finding("t4"); finding.State != "open" || view.typed["t4"] != "" || *view.status != (muiStatus{Text: "t4 reopened", Tone: "ok"}) {
		t.Errorf("reopened, it is %s, status %+v", finding.State, view.status)
	}
	// one that stands has no Reopen, and with no words only the verb is run
	if deed := view.deed(reviewEvent{E: "press", ID: "reopen"}); deed != (reviewDeed{}) {
		t.Errorf("Reopen on a finding that stands does %+v", deed)
	}
	fake.state.Threads[3].State = "wontfix"
	view.read(reviewNow)
	reviewDone(view, reviewEvent{E: "press", ID: "reopen"})
	if got, want := fake.asked(), [][]string{{"reopen", "t4", "--json", "-t", "range_main..HEAD"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reopen with no words ran %q, want %q", got, want)
	}
}

// g shows sawa's gates in place of the summary: each by its name with
// what must hold, how it was judged, by whom and when, and why; ⏎ on one
// shows what was judged, with an area for why and Pass, Warn and Fail,
// which record its result through sw as the person; esc goes back to the
// gates, and again to the summary.
func TestTheGatesAreShownAndJudged(t *testing.T) {
	gates := []sawaGate{
		{Name: "tests", Text: "go test ./... passes", Gate: &sawaJudged{State: "pass", Note: "ran it: all ok", By: "sawa", At: reviewNow.Add(-5 * time.Minute)}},
		{Name: "docs", Text: "what changed is in docs/changes.md", Gate: &sawaJudged{State: "fail", By: "sawa", At: reviewNow.Add(-2 * time.Hour)}},
		{Name: "description", Text: "the description says what and why"},
	}
	want := []reviewEntry{
		{Head: "Gates", Note: "2 of 3 judged"},
		{ID: "g:tests", Title: "tests", Note: "go test ./... passes", Right: "pass · sawa · 5 min ago", Tone: "ok", Under: "ran it: all ok"},
		{ID: "g:docs", Title: "docs", Note: "what changed is in docs/changes.md", Right: "fail · sawa · 2 h ago", Tone: "bad"},
		{ID: "g:description", Title: "description", Note: "the description says what and why", Right: "not judged"},
	}
	if got := reviewGates(gates, reviewGatesMost, reviewNow); !reflect.DeepEqual(got, want) {
		t.Errorf("the gates are %+v", got)
	}
	if got := reviewGates(gates, 1, reviewNow); len(got) != 2 {
		t.Errorf("at a most of one the gates are %+v", got)
	}

	fake := reviewFixed()
	view := reviewOf(fake)
	if say, _ := reviewDone(view, reviewEvent{E: "press", ID: "gates"}); !say || view.gating || view.status == nil || !strings.Contains(view.status.Text, "no gates") {
		t.Errorf("g with no gates: gating %v, status %+v", view.gating, view.status)
	}
	fake.state.Gates = gates
	view.read(reviewNow)
	say, aim := reviewDone(view, reviewEvent{E: "press", ID: "gates"})
	first, _ := reviewSaid(t, view.messages(aim, reviewNow))
	list := reviewPieceOf(first.View, "gates")
	if !say || aim.focus != "gates" || list == nil || !reflect.DeepEqual(list.Items, want) || list.Selected != "g:tests" || first.Keys[0] != (gitKey{"esc", "back to the summary", "shut"}) {
		t.Fatalf("g: said %v with %+v, the list %+v", say, aim, list)
	}
	if texts := reviewTexts(first.View.Children[0]); slices.ContainsFunc(texts, func(text string) bool { return strings.HasPrefix(text, "Adds patch") }) {
		t.Errorf("the summary is said with the gates up: %q", texts)
	}

	// ⏎ on one shows what was judged, and a person may record its result
	if _, aim = reviewDone(view, reviewEvent{E: "pick", List: "gates", ID: "g:tests"}); view.gate != "tests" || aim.focus != "why" {
		t.Fatalf("⏎ shows %q with %+v", view.gate, aim)
	}
	first, _ = reviewSaid(t, view.messages(aim, reviewNow))
	head := first.View.Children[0]
	if texts := reviewTexts(head); !slices.Equal(texts[4:], []string{"gate  tests", "go test ./... passes", "pass · sawa · 5 min ago: ran it: all ok"}) || first.Keys[0] != (gitKey{"esc", "back to the gates", "shut"}) {
		t.Errorf("the gate says %q", texts)
	}
	if why, buttons := reviewPieceOf(&head, "why"), head.Children[len(head.Children)-1].Children; why == nil || why.T != "field" || len(buttons) != 3 || !reflect.DeepEqual(buttons[1], reviewPiece{T: "button", ID: "gate-warn", Label: "Warn"}) {
		t.Errorf("under it are %+v and %+v", why, buttons)
	}
	reviewDone(view, reviewEvent{E: "change", ID: "why", Text: "-race is not run"})
	_, aim = reviewDone(view, reviewEvent{E: "press", ID: "gate-warn"})
	if got, want := fake.asked(), [][]string{{"gate", "tests", "warn", "--json", "-t", "range_main..HEAD", "-m", "-race is not run"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Warn ran %q, want %q", got, want)
	}
	if view.gate != "" || !view.gating || aim.focus != "gates" || *view.status != (muiStatus{Text: "tests: warn", Tone: "warn"}) || view.state.Gates[0].Gate.State != "warn" || view.typed["g:tests"] != "" {
		t.Errorf("judged: the gate shown %q, status %+v, sawa has %+v", view.gate, view.status, view.state.Gates[0].Gate)
	}
	// with no words only the result is recorded; a button with no gate shown does nothing
	if deed := view.deed(reviewEvent{E: "press", ID: "gate-pass"}); deed != (reviewDeed{}) {
		t.Errorf("Pass with no gate shown does %+v", deed)
	}
	reviewDone(view, reviewEvent{E: "pick", List: "gates", ID: "g:description"})
	reviewDone(view, reviewEvent{E: "press", ID: "gate-pass"})
	if got, want := fake.asked(), [][]string{{"gate", "description", "pass", "--json", "-t", "range_main..HEAD"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Pass ran %q, want %q", got, want)
	}
	// esc: from a gate to the gates, from the gates to the summary
	reviewDone(view, reviewEvent{E: "pick", List: "gates", ID: "g:docs"})
	if _, aim = reviewDone(view, reviewEvent{E: "press", ID: "shut"}); view.gate != "" || !view.gating || aim.focus != "gates" {
		t.Errorf("esc on a gate: %q shown, gating %v", view.gate, view.gating)
	}
	if _, aim = reviewDone(view, reviewEvent{E: "press", ID: "shut"}); view.gating || aim.focus != "files" {
		t.Errorf("esc on the gates: gating %v with %+v", view.gating, aim)
	}
	if deed := view.deed(reviewEvent{E: "pick", List: "gates", ID: "g:docs"}); deed != (reviewDeed{}) {
		t.Errorf("⏎ on a gate with the gates down does %+v", deed)
	}
}

// The words over a run of lines are those git has over the hunk the run
// is in — the function it is in, where git knows one — found by the
// run's first line in the new file, or in the old where it has none; a
// run in no hunk says where it begins.
func TestAHunkIsNamedAsGitNamesIt(t *testing.T) {
	hunks := []git.Hunk{
		{OldStart: 214, OldLines: 6, NewStart: 216, NewLines: 9, Section: "impl Talk {"},
		{OldStart: 236, OldLines: 6, NewStart: 240, NewLines: 12, Section: "fn hear(&mut self, said: Said) -> Heard {"},
		{OldStart: 300, OldLines: 4, NewStart: 310, NewLines: 0, Section: "fn gone()"},
	}
	for _, c := range []struct {
		old, new int
		want     string
	}{{0, 219, "impl Talk {"}, {217, 221, "impl Talk {"}, {239, 243, "fn hear(&mut self, said: Said) -> Heard {"}, {240, 0, "fn hear(&mut self, said: Said) -> Heard {"},
		{301, 0, "fn gone()"}, {0, 120, ""}, {0, 280, ""}, {120, 0, ""}} {
		if got := reviewSection(hunks, c.old, c.new); got != c.want {
			t.Errorf("the run from %d, %d is in %q, want %q", c.old, c.new, got, c.want)
		}
	}
	fake := reviewFixed()
	lines, _ := reviewCode(fake.rows["src/talk.rs"], reviewNotes{hunks: hunks[:1], now: reviewNow})
	var heads []string
	for _, line := range lines {
		if line.Kind == "head" {
			heads = append(heads, line.Text+" · "+line.Right)
		}
	}
	if want := []string{"@@ impl Talk { · hunk 1 of 2", "@@ line 243 · hunk 2 of 2"}; !slices.Equal(heads, want) {
		t.Errorf("the heads are %q, want %q", heads, want)
	}
	// a new file's head says so, whatever git has
	lines, _ = reviewCode(fake.rows["src/mui/patch.rs"], reviewNotes{hunks: []git.Hunk{{NewStart: 1, NewLines: 2, Section: "x"}}, now: reviewNow})
	if lines[0].Text != "@@ new file" {
		t.Errorf("a new file's head is %q", lines[0].Text)
	}
}

// When the last unread file is marked read there is no next one: the
// status says every file is read, and the keys are on the verdicts.
func TestWhenEveryFileIsReadTheKeysAreOnTheVerdict(t *testing.T) {
	fake := reviewFixed()
	for at := range fake.state.Files {
		fake.state.Files[at].Read = fake.state.Files[at].Path != "src/talk.rs"
	}
	view := reviewOf(fake)
	say, aim := reviewDone(view, reviewEvent{E: "press", ID: "read", Item: "src/talk.rs"})
	if !say || !view.judging || aim.focus != "verdict" || !aim.list || *view.status != (muiStatus{Text: "every file is read: the verdict is yours", Tone: "ok"}) {
		t.Fatalf("space on the last unread file: said %v with %+v, status %+v", say, aim, view.status)
	}
	first, _ := reviewSaid(t, view.messages(aim, reviewNow))
	if verdicts := reviewPieceOf(first.View, "verdict"); verdicts == nil || verdicts.Selected != "approve" || first.Focus != "verdict" {
		t.Errorf("the verdicts are %+v, the keys on %q", verdicts, first.Focus)
	}
	// while another is unread, the next one is shown and no verdict asked
	fake.state.Files[0].Read, fake.state.Files[5].Read = false, false
	view = reviewOf(fake)
	if reviewDone(view, reviewEvent{E: "press", ID: "read", Item: "src/talk.rs"}); view.judging || view.file != "docs/apps.md" {
		t.Errorf("with a file unread: judging %v, on %s", view.judging, view.file)
	}
}

// e hands the file to the editor as mia files does, at the line of the
// file the keys are on in the diff — a deleted line's and a note's being
// the next line the file has — and at its first line on its row in the
// list; nothing is asked of sawa.
func TestEHandsTheFileToTheEditorAtTheLine(t *testing.T) {
	fake := reviewFixed()
	view := reviewOf(fake)
	reviewDone(view, reviewEvent{E: "move", List: "files", ID: "src/talk.rs"})
	place := func(mark reviewMark) string { return strconv.Itoa(slices.Index(view.marks, mark)) }
	for item, want := range map[string]reviewDeed{
		place(reviewMark{kind: "add", new: 246}):            {do: reviewEdit, file: "src/talk.rs", place: 246},
		place(reviewMark{kind: "same", old: 239, new: 243}): {do: reviewEdit, file: "src/talk.rs", place: 243},
		place(reviewMark{kind: "del", old: 240}):            {do: reviewEdit, file: "src/talk.rs", place: 244},
		place(reviewMark{kind: "note", note: "t1"}):         {do: reviewEdit, file: "src/talk.rs", place: 246},
		"0":          {do: reviewEdit, file: "src/talk.rs", place: 219}, // a hunk's head
		"":           {do: reviewEdit, file: "src/talk.rs", place: 219},
		"src/mui.rs": {do: reviewEdit, file: "src/mui.rs", place: 1},
		"d:src/":     {},
	} {
		if got := view.deed(reviewEvent{E: "press", ID: "edit", Item: item}); got != want {
			t.Errorf("e on %q does %+v, want %+v", item, got, want)
		}
	}
	if say, _ := view.act(reviewDeed{do: reviewEdit, file: "src/talk.rs", place: 246}, reviewNow); say || len(fake.asked()) != 0 {
		t.Error("e said the view again, or asked sawa")
	}

	gittest.Isolate(t)
	view.top = git.Resolve(t.TempDir())
	said := filepath.Join(t.TempDir(), "said")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\npwd > \"$REVIEW_SAID\"\nprintf '%s\\n' \"$@\" >> \"$REVIEW_SAID\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVIEW_SAID", said)
	t.Setenv("EDITOR", editor+" --wait")
	view.edit(reviewDeed{do: reviewEdit, file: "src/talk.rs", place: 246})
	out, err := os.ReadFile(said)
	if want := []string{view.top, "--wait", "+246", "src/talk.rs"}; err != nil || !slices.Equal(strings.Split(strings.TrimSpace(string(out)), "\n"), want) || view.status != nil {
		t.Errorf("the editor was run as %q, %v, status %+v", out, err, view.status)
	}
	if first, _ := reviewSaid(t, view.messages(reviewAim{}, reviewNow)); first.Keys[4] != (gitKey{"e", "edit in editor", "edit"}) {
		t.Errorf("the key says %+v", first.Keys[4])
	}
	t.Setenv("EDITOR", filepath.Join(t.TempDir(), "no-such-editor"))
	if view.edit(reviewDeed{do: reviewEdit, file: "src/talk.rs", place: 1}); view.status == nil || view.status.Tone != "bad" || !strings.Contains(view.status.Text, "no-such-editor") {
		t.Errorf("no such editor: %+v", view.status)
	}
}

// Where sawa reviews a stack the header says which layer of how many
// this is, and [ and ] show the layer under and the one over: sawa opens
// that layer's review, and this worktree's own is the one that was
// found. At the bottom and at the top the status says so, and where
// sawa knows no stack there are no layers.
func TestTheLayersOfAStackAreGoneThrough(t *testing.T) {
	fake := reviewFixed()
	view := reviewOf(fake)
	if view.stacked(); len(view.layers) != 0 {
		t.Fatalf("with no stack the layers are %+v", view.layers)
	}
	first, _ := reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if slices.ContainsFunc(first.Keys, func(key gitKey) bool { return key.Key == "[" }) || strings.Contains(reviewTexts(first.View.Children[0])[3], "layer") {
		t.Errorf("with no stack the keys are %+v", first.Keys)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "under"}); *view.status != (muiStatus{Text: "sawa reviews no stack here"}) {
		t.Errorf("[ with no stack: %+v", view.status)
	}

	fake.layers = []sawaLayer{{Branch: "mui", Revset: "main..mui", Slug: "range_main..mui"}, {Branch: "mui-patch", Revset: "mui..mui-patch", Slug: "range_mui..mui-patch", Current: true}, {Branch: "mui-chat", Revset: "mui-patch..mui-chat", Slug: "range_mui-patch..mui-chat"}}
	view.status = nil
	view.stacked()
	fake.asked()
	if len(view.layers) != 3 || view.layer != 1 || view.home != 1 || view.homeSlug != "range_main..HEAD" {
		t.Fatalf("the layers are %+v, this one %d", view.layers, view.layer)
	}
	first, _ = reviewSaid(t, view.messages(reviewAim{}, reviewNow))
	if about := reviewTexts(first.View.Children[0])[3]; !strings.HasSuffix(about, " · layer 2 of 3") || first.Keys[10] != (gitKey{"[", "layer under", "under"}) || first.Keys[11] != (gitKey{"]", "layer over", "over"}) {
		t.Errorf("the header says %q, the keys %+v", about, first.Keys)
	}
	reviewDone(view, reviewEvent{E: "move", List: "files", ID: "src/talk.rs"})
	say, aim := reviewDone(view, reviewEvent{E: "press", ID: "under"})
	if got, want := fake.asked(), [][]string{{"open", "-r", "main..mui", "--json"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("[ ran %q, want %q", got, want)
	}
	if !say || view.layer != 0 || view.slug != "range_main..mui" || view.file != "docs/apps.md" || !aim.list || !aim.code || view.title != "mui" {
		t.Errorf("[: said %v with %+v, layer %d, the review %q titled %q on %s", say, aim, view.layer, view.slug, view.title, view.file)
	}
	if first, _ = reviewSaid(t, view.messages(aim, reviewNow)); !strings.HasSuffix(reviewTexts(first.View.Children[0])[3], " · layer 1 of 3") {
		t.Errorf("the header says %q", reviewTexts(first.View.Children[0])[3])
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "under"}); view.layer != 0 || *view.status != (muiStatus{Text: "this is the bottom layer"}) || len(fake.asked()) != 0 {
		t.Errorf("[ at the bottom: layer %d, status %+v", view.layer, view.status)
	}
	// what is done is done to the review of the layer shown
	reviewDone(view, reviewEvent{E: "press", ID: "read", Item: "docs/apps.md"})
	if got, want := fake.asked(), [][]string{{"read", "--json", "-t", "range_main..mui", "-f", "docs/apps.md"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("space ran %q, want %q", got, want)
	}
	// ] back to this worktree's own, which is not opened again, and on
	if reviewDone(view, reviewEvent{E: "press", ID: "over"}); view.layer != 1 || view.slug != "range_main..HEAD" || len(fake.asked()) != 0 || view.title != "mui-patch" {
		t.Errorf("]: layer %d, the review %q titled %q", view.layer, view.slug, view.title)
	}
	reviewDone(view, reviewEvent{E: "press", ID: "over"})
	if got, want := fake.asked(), [][]string{{"open", "-r", "mui-patch..mui-chat", "--json"}}; !reflect.DeepEqual(got, want) || view.layer != 2 {
		t.Errorf("] ran %q, layer %d", got, view.layer)
	}
	if reviewDone(view, reviewEvent{E: "press", ID: "over"}); view.layer != 2 || *view.status != (muiStatus{Text: "this is the top layer"}) {
		t.Errorf("] at the top: layer %d, status %+v", view.layer, view.status)
	}
	// a layer sawa cannot open is the status, and the layer shown stays
	reviewDone(view, reviewEvent{E: "press", ID: "under"})
	fake.refuse = map[string]string{"open": "no such revision"}
	if reviewDone(view, reviewEvent{E: "press", ID: "over"}); view.layer != 1 || view.slug != "range_main..HEAD" || view.status.Tone != "bad" {
		t.Errorf("a refusal: layer %d, status %+v", view.layer, view.status)
	}
}

// Against real sw, in a throwaway repository: a finding on the review as
// a whole is in the header and is answered there; the edit a finding
// suggests is made in the working tree by Apply; a gate of the
// repository's is recorded by the person, whom sawa lets; a resolved
// finding is reopened; and the hunks are named as git names them.
func TestGatesEditsAndReopeningGoThroughSw(t *testing.T) {
	if _, err := exec.LookPath("sw"); err != nil {
		t.Skip("sw is not on PATH")
	}
	gittest.Isolate(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SAWA_TARGET", "")
	os.Unsetenv("SAWA_TARGET")
	t.Setenv("SAWA_AGENT", "1")
	t.Setenv("SAWA_AUTHOR", "sawa")
	repo := gittest.New(t)
	talk := []string{"package talk", "", "func Five() string {", "\ta := 1", "\tb := 2", "\tc := 3", "\tn := a + b + c - 1", "\treturn \"five\"", "}", "", "// the end"}
	repo.Write("talk.go", strings.Join(talk, "\n")+"\n")
	repo.Write(".sawa/gates.md", "- tests: go test ./... passes\n- docs: what changed is written down\n")
	repo.Commit("talk")
	repo.Git("checkout", "-q", "-b", "words")
	talk[7] = "\treturn \"fives\""
	repo.Write("talk.go", strings.Join(talk, "\n")+"\n")
	repo.Commit("Five says fives")

	view := &reviewView{pane: "%1", dir: repo.Root, run: sawaIn(repo.Root), typed: map[string]string{}}
	view.find(reviewNow)
	if view.failed != "" || view.status != nil {
		t.Fatalf("the review was not opened: %q %+v", view.failed, view.status)
	}
	if !strings.HasPrefix(view.state.Store, repo.Root+"/.git/sawa/") {
		t.Fatalf("sawa keeps the review at %s, outside the throwaway repository %s", view.state.Store, repo.Root)
	}
	if len(view.layers) != 0 || len(view.state.Gates) != 2 || view.state.Gates[0].Gate != nil {
		t.Fatalf("layers %+v, gates %+v", view.layers, view.state.Gates)
	}
	// git names the hunk by the function it is in
	if _, lines := reviewSaid(t, view.messages(reviewAim{}, time.Now())); view.file != "talk.go" || lines[0].Text != "@@ func Five() string {" {
		t.Errorf("the head of %s is %q", view.file, lines[0].Text)
	}

	// sawa, an agent, files one on the review as a whole and one that suggests an edit.
	edit := filepath.Join(t.TempDir(), "edit")
	if err := os.WriteFile(edit, []byte("\treturn \"five\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "--json", "-m", "question: why fives\nThe commit does not say."},
		{"add", "--json", "-f", "talk.go", "-l", "8", "--suggest-file", edit, "-m", "suggestion: five is one word"},
	} {
		file := exec.Command("sw", args...)
		file.Dir = repo.Root
		if out, err := file.CombinedOutput(); err != nil {
			t.Fatalf("sw %s: %v\n%s", args[0], err, out)
		}
	}
	view.read(time.Now())
	do := func(event reviewEvent) reviewAim {
		t.Helper()
		_, aim := view.act(view.deed(event), time.Now())
		if view.status != nil && view.status.Tone == "bad" {
			t.Fatalf("%+v: %s", event, view.status.Text)
		}
		return aim
	}
	sawa := func() sawaState {
		t.Helper()
		state, err := sawaAsk[sawaState](sawaIn(repo.Root), "state", "--json", "-t", view.slug)
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	first, _ := reviewSaid(t, view.messages(reviewAim{}, time.Now()))
	if list := reviewPieceOf(first.View, "general"); list == nil || len(list.Items) != 2 || list.Items[1].ID != "t1" || list.Items[1].Title != "why fives" || list.Items[1].Lead != "question" {
		t.Fatalf("the findings on the review as a whole are %+v", list)
	}

	// N from the top is on it; ⏎ and a reply answer it; Reply and resolve, then Reopen.
	if aim := do(reviewEvent{E: "press", ID: "prev"}); view.above != "t1" || aim.focus != "general" {
		t.Errorf("N went to %q with %+v", view.above, aim)
	}
	do(reviewEvent{E: "pick", List: "general", ID: "t1"})
	do(reviewEvent{E: "submit", ID: "reply", Text: "it is a typo"})
	do(reviewEvent{E: "press", ID: "resolve"})
	if got := sawa().Threads[0]; got.State != "resolved" || len(got.Posts) != 2 || got.Posts[1].Author != "test" || got.Posts[1].Body != "it is a typo" {
		t.Errorf("sawa has the finding as %+v", got)
	}
	do(reviewEvent{E: "press", ID: "resolved"})
	do(reviewEvent{E: "pick", List: "general", ID: "t1"})
	do(reviewEvent{E: "change", ID: "reply", Text: "not fixed yet"})
	do(reviewEvent{E: "press", ID: "reopen"})
	if got := sawa().Threads[0]; got.State != "open" || len(got.Posts) != 3 || got.Posts[2].Body != "not fixed yet" || got.Posts[2].Author != "test" {
		t.Errorf("reopened, sawa has the finding as %+v", got)
	}
	do(reviewEvent{E: "press", ID: "shut"})

	// The edit is under the finding's words, and Apply makes it in the working tree.
	do(reviewEvent{E: "press", ID: "next"})
	if view.marks[view.at].note != "t2" {
		t.Fatalf("n went to %+v", view.marks[view.at])
	}
	do(reviewEvent{E: "pick", List: "diff", ID: strconv.Itoa(view.at)})
	_, lines := reviewSaid(t, view.messages(reviewAim{}, time.Now()))
	var suggested []reviewLine
	want := []reviewLine{{Kind: "del", Old: 8, Text: "    return \"fives\""}, {Kind: "add", New: 8, Text: "    return \"five\""}}
	if under := lines[view.place("t2")].Under; len(under) < 2 || under[0].ID != "suggested" || json.Unmarshal(under[0].Lines, &suggested) != nil || !reflect.DeepEqual(suggested, want) {
		t.Fatalf("under the finding are %+v", under)
	}
	do(reviewEvent{E: "press", ID: "apply"})
	talk[7] = "\treturn \"five\""
	if got, err := os.ReadFile(filepath.Join(repo.Root, "talk.go")); err != nil || string(got) != strings.Join(talk, "\n")+"\n" {
		t.Errorf("the file is now %q, %v", got, err)
	}
	if got := sawa().Threads[1]; got.AppliedAt == nil || *view.status != (muiStatus{Text: "t2 applied to the working tree", Tone: "ok"}) {
		t.Errorf("sawa has the finding as %+v, status %+v", got, view.status)
	}
	if view.act(view.deed(reviewEvent{E: "press", ID: "apply"}), time.Now()); view.status == nil || view.status.Tone != "bad" {
		t.Errorf("applied again: %+v", view.status)
	}
	do(reviewEvent{E: "press", ID: "shut"})

	// The gates are sawa's, and a person records one's result.
	do(reviewEvent{E: "press", ID: "gates"})
	first, _ = reviewSaid(t, view.messages(reviewAim{list: true}, time.Now()))
	if list := reviewPieceOf(first.View, "gates"); list == nil || len(list.Items) != 3 || list.Items[1].Title != "tests" || list.Items[1].Note != "go test ./... passes" || list.Items[1].Right != "not judged" {
		t.Fatalf("the gates are %+v", list)
	}
	do(reviewEvent{E: "pick", List: "gates", ID: "g:tests"})
	do(reviewEvent{E: "change", ID: "why", Text: "-ran it here"})
	do(reviewEvent{E: "press", ID: "gate-warn"})
	if got := sawa().Gates[0].Gate; got == nil || got.State != "warn" || got.Note != "-ran it here" || got.By != "test" {
		t.Errorf("sawa has the gate as %+v", got)
	}
	if got := view.state.Gates[0].Gate; got == nil || got.State != "warn" || view.gate != "" || !view.gating {
		t.Errorf("the view has the gate as %+v", got)
	}
}
