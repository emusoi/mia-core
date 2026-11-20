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

	"github.com/emusoi/mia-core/internal/gittest"
)

// filesHeard is a message of mia files as the window reads it.
type filesHeard struct {
	V      int               `json:"v"`
	Pane   string            `json:"pane"`
	Fill   bool              `json:"fill"`
	Status *muiStatus        `json:"status"`
	View   *filesPiece       `json:"view"`
	Keys   []gitKey          `json:"keys"`
	Patch  []json.RawMessage `json:"patch"`
}

// filesSaid is a message read back, which no message the window takes
// fails: in ASCII alone, and no longer than a message may be.
func filesSaid(t *testing.T, message []byte) filesHeard {
	t.Helper()
	if len(message) > muiLimit {
		t.Fatalf("a message of %d bytes, over the %d the window takes", len(message), muiLimit)
	}
	if at := slices.IndexFunc(message, func(b byte) bool { return b >= 0x80 || b < 0x20 }); at >= 0 {
		t.Fatalf("byte %#x in a message: %.80q", message[at], message)
	}
	var got filesHeard
	if err := json.Unmarshal(message, &got); err != nil {
		t.Fatalf("%v in %.200s", err, message)
	}
	return got
}

// filesPieceOf is the piece of an id in a view, filesTexts every text in
// it in order, and filesCounted how many pieces the window counts in it:
// a list's items and a code's lines each one.
func filesPieceOf(in *filesPiece, id string) *filesPiece {
	if in.ID == id {
		return in
	}
	for i := range in.Children {
		if found := filesPieceOf(&in.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

func filesTexts(in filesPiece) []string {
	var all []string
	if in.T == "text" {
		all = append(all, in.Text)
	}
	for _, child := range in.Children {
		all = append(all, filesTexts(child)...)
	}
	return all
}

func filesCounted(t *testing.T, in filesPiece) int {
	t.Helper()
	n := 1 + len(in.Items)
	if in.Lines != nil {
		var lines []json.RawMessage
		if err := json.Unmarshal(in.Lines, &lines); err != nil {
			t.Fatal(err)
		}
		n += len(lines)
	}
	for _, child := range in.Children {
		n += filesCounted(t, child)
	}
	return n
}

// filesCode is the lines of the file a view's messages say: those in the
// view, then those each patch puts after them.
func filesCode(t *testing.T, messages [][]byte) []filesLine {
	t.Helper()
	var lines []filesLine
	for i, message := range messages {
		got := filesSaid(t, message)
		raw := json.RawMessage("[]")
		if i == 0 {
			if in := filesPieceOf(got.View, "file").Lines; in != nil {
				raw = in
			}
		} else {
			var op gitOp
			if got.View != nil || len(got.Patch) != 1 || json.Unmarshal(got.Patch[0], &op) != nil || op.Op != "lines" || op.ID != "file" {
				t.Fatalf("message %d is no patch of lines: %.200s", i, message)
			}
			raw, _ = json.Marshal(op.Lines)
		}
		var some []filesLine
		if err := json.Unmarshal(raw, &some); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, some...)
	}
	return lines
}

func filesIDs(rows []filesRow) []string {
	var all []string
	for _, row := range rows {
		all = append(all, row.id)
	}
	return all
}

// filesDesk is the files of the view mia desk draws as its fixture
// (MUI_FILES in its scenes), and the view of them: src and src/mui open,
// three files changed against main, the keys on the new one.
var filesDesk = []string{"Cargo.toml", "README.md", "docs/apps.md", "examples/mui-demo.py", "src/chatlook.rs", "src/mui.rs", "src/mui/patch.rs", "src/talk.rs", "src/term.rs", "tests/mui_patch.rs"}

func filesDeskView() *filesView {
	view := &filesView{pane: "%1", dir: "/src/mia-desk", top: "/src/mia-desk", inside: true, name: "mia-desk", against: "main", hitAt: -1,
		paths: filesDesk, marks: map[string]byte{"src/mui/patch.rs": 'A', "src/mui.rs": 'M', "src/talk.rs": 'M'},
		open: map[string]bool{"src": true, "src/mui": true}, at: "src/mui/patch.rs", file: "src/mui/patch.rs"}
	view.rows = filesTree(view.paths, view.open)
	view.lines, view.total = filesLines([]byte("//! A patch: changes to the UI shown, made in turn, all or none.\nuse crate::mui::{Change, Missing, Piece};\n\n"), 1, filesLinesMost)
	return view
}

const filesDeskSaid = `{"v":1,"pane":"%1","fill":true,"view":{"t":"row","split":true,"children":[{"t":"col","w":300,"scroll":true,"pad":6,"children":[{"t":"list","id":"tree","live":true,"selected":"src/mui/patch.rs","items":[{"head":"mia-desk","note":"3 changed against main"},{"id":"docs","title":"docs","depth":0,"open":false},{"id":"examples","title":"examples","depth":0,"open":false},{"id":"src","title":"src","depth":0,"open":true},{"id":"src/mui","title":"mui","depth":1,"open":true},{"id":"src/mui/patch.rs","title":"patch.rs","depth":2,"right":"A","tone":"ok"},{"id":"src/chatlook.rs","title":"chatlook.rs","depth":1},{"id":"src/mui.rs","title":"mui.rs","depth":1,"right":"M","tone":"warn"},{"id":"src/talk.rs","title":"talk.rs","depth":1,"right":"M","tone":"warn"},{"id":"src/term.rs","title":"term.rs","depth":1},{"id":"tests","title":"tests","depth":0,"open":false},{"id":"Cargo.toml","title":"Cargo.toml","depth":0},{"id":"README.md","title":"README.md","depth":0}]}]},{"t":"divider"},{"t":"col","children":[{"t":"row","pad":10,"gap":10,"children":[{"t":"text","text":"src/mui/patch.rs","style":"mono"},{"t":"text","text":"new against main · 3 lines","style":"dim"}]},{"t":"code","id":"file","live":true,"lines":[{"new":1,"text":"//! A patch: changes to the UI shown, made in turn, all or none."},{"new":2,"text":"use crate::mui::{Change, Missing, Piece};"},{"new":3,"text":""}]}]}]},"keys":[{"key":"e","label":"edit in nvim","id":"edit"},{"key":"f","label":"find a file","id":"find"},{"key":"/","label":"find in files","id":"grep"},{"key":"r","label":"ask again","id":"again"}]}`

// The tree lists in each folder its folders and then its files, by name
// whatever its case, and under a folder what is in it only where the
// person opened it; a row is known by its path and set in by its depth.
func TestTheTreeListsOnlyUnderOpenFolders(t *testing.T) {
	paths := []string{"zed.txt", "Makefile", "src/b.go", "src/A.go", "src/deep/in/x.go", "src/deep/y.go", "docs/one.md", "a.txt"}
	if got, want := filesIDs(filesTree(paths, nil)), []string{"docs", "src", "a.txt", "Makefile", "zed.txt"}; !slices.Equal(got, want) {
		t.Errorf("with nothing open: %q", got)
	}
	rows := filesTree(paths, map[string]bool{"src": true, "src/deep": true, "docs/gone": true})
	want := []filesRow{
		{"docs", "docs", 0, true, false},
		{"src", "src", 0, true, true},
		{"src/deep", "src/deep", 1, true, true},
		{"src/deep/in", "src/deep/in", 2, true, false},
		{"src/deep/y.go", "src/deep/y.go", 2, false, false},
		{"src/A.go", "src/A.go", 1, false, false},
		{"src/b.go", "src/b.go", 1, false, false},
		{"a.txt", "a.txt", 0, false, false},
		{"Makefile", "Makefile", 0, false, false},
		{"zed.txt", "zed.txt", 0, false, false},
	}
	if !slices.Equal(rows, want) {
		t.Errorf("rows = %+v", rows)
	}
	// a folder open under one that is shut is not listed
	if got := filesIDs(filesTree(paths, map[string]bool{"src/deep": true})); slices.Contains(got, "src/deep") {
		t.Errorf("under a shut folder: %q", got)
	}
	if rows := filesTree(nil, nil); len(rows) != 0 {
		t.Errorf("of no file: %+v", rows)
	}
}

// The list is the rows under a head with the repository's name and how
// many files changed: a folder with its chevron, a file changed against
// the base with A, M or D in its tone. Past the most rows a head says how
// many more there are.
func TestChangedFilesCarryTheirMarkInTheList(t *testing.T) {
	rows := filesTree([]string{"src/new.go", "src/old.go", "src/same.go", "src/mod.go"}, map[string]bool{"src": true})
	marks := map[string]byte{"src/new.go": 'A', "src/old.go": 'D', "src/mod.go": 'M'}
	zero, one, shut, open := 0, 1, false, true
	want := []filesEntry{
		{Head: "app", Note: "3 changed against main"},
		{ID: "src", Title: "src", Depth: &zero, Open: &open},
		{ID: "src/mod.go", Title: "mod.go", Depth: &one, Right: "M", Tone: "warn"},
		{ID: "src/new.go", Title: "new.go", Depth: &one, Right: "A", Tone: "ok"},
		{ID: "src/old.go", Title: "old.go", Depth: &one, Right: "D", Tone: "bad"},
		{ID: "src/same.go", Title: "same.go", Depth: &one},
	}
	if got := filesEntries(rows, marks, "app", "3 changed against main", filesRowsMost); !reflect.DeepEqual(got, want) {
		t.Errorf("entries = %s", encode(got))
	}
	cut := filesEntries(rows, marks, "app", "", 2)
	if len(cut) != 4 || cut[3] != (filesEntry{Head: "3 more rows", Note: "shut a folder to see them"}) {
		t.Errorf("cut to 2 rows: %s", encode(cut))
	}
	if got := filesEntries(filesTree([]string{"docs/a.md"}, nil), nil, "app", "", 9); string(encode(got[1])) != `{"id":"docs","title":"docs","depth":0,"open":false}` || *got[1].Open != shut {
		t.Errorf("a shut folder at the top = %s", encode(got[1]))
	}
}

// As the view comes up the folders on the way to each changed file are
// open, every one of them down to the file's own; where that is more than
// the most folders, none is.
func TestTheFoldersOnTheWayToWhatChangedAreOpen(t *testing.T) {
	got := filesOnTheWay([]string{"README.md", "src/mui/patch.rs", "src/talk.rs", "tests/deep/in/x.rs"}, filesOpenMost)
	want := map[string]bool{"src": true, "src/mui": true, "tests": true, "tests/deep": true, "tests/deep/in": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("open = %v", got)
	}
	var many []string
	for n := range filesOpenMost + 1 {
		many = append(many, fmt.Sprintf("folder%d/file.txt", n))
	}
	if got := filesOnTheWay(many, filesOpenMost); len(got) != 0 {
		t.Errorf("%d folders open, with more than the most on the way", len(got))
	}
	if got := filesOnTheWay(many[:filesOpenMost], filesOpenMost); len(got) != filesOpenMost {
		t.Errorf("%d folders open of %d", len(got), filesOpenMost)
	}
}

// A file is found by the letters typed, in order and whatever their case,
// anywhere in its path: those with them in the file's name first, then
// the shorter path, and no more than the most.
func TestAFileIsFoundByItsLettersInOrder(t *testing.T) {
	paths := []string{"internal/cli/git.go", "internal/git/standing.go", "internal/git/git.go", "docs/git-notes.md", "Makefile", "internal/gittest/gittest.go"}
	found, n := filesFound(paths, "GiT", filesFoundMost)
	want := []string{"docs/git-notes.md", "internal/cli/git.go", "internal/git/git.go", "internal/gittest/gittest.go", "internal/git/standing.go"}
	if !slices.Equal(found, want) || n != 5 {
		t.Errorf("found %d: %q", n, found)
	}
	if found, _ := filesFound(paths, "mkf", filesFoundMost); !slices.Equal(found, []string{"Makefile"}) {
		t.Errorf("mkf found %q", found)
	}
	if found, n := filesFound(paths, "fm", filesFoundMost); len(found) != 0 || n != 0 {
		t.Errorf("letters out of order found %q", found)
	}
	if found, n := filesFound(paths, "", 2); !slices.Equal(found, []string{"Makefile", "docs/git-notes.md"}) || n != len(paths) {
		t.Errorf("nothing typed found %d: %q", n, found)
	}
	for _, c := range []struct {
		words, typed string
		has          bool
	}{{"src/mui.rs", "smr", true}, {"src/mui.rs", "", true}, {"src/mui.rs", "rs/", false}, {"née.md", "ée", true}, {"née.md", "eé", false}} {
		if got := filesHas(c.words, c.typed); got != c.has {
			t.Errorf("%q has %q: %v", c.words, c.typed, got)
		}
	}
	marked := filesFoundEntries([]string{"src/mui.rs", "Makefile"}, map[string]byte{"src/mui.rs": 'M'})
	if want := []filesEntry{{ID: "src/mui.rs", Title: "mui.rs", Note: "src", Right: "M", Tone: "warn"}, {ID: "Makefile", Title: "Makefile"}}; !reflect.DeepEqual(marked, want) {
		t.Errorf("entries = %s", encode(marked))
	}
}

// What git grep writes is hits, each a path, a line's number and its text
// without the room at its ends; in the list they are grouped by file,
// each file's under a head with its path and its count, each line by its
// text with its number at the right and an id that says its place.
func TestWhatGrepWritesIsHitsGroupedByFile(t *testing.T) {
	out := "src/mui.rs\x00112\x00    pub fn apply(&mut self) {\n" +
		"tests/a b:c.rs\x0012\x00\tassert!(view.apply(x));\r\n" +
		"tests/a b:c.rs\x0027\x00apply(\x00)\n" +
		"not a hit\n" +
		"src/long.rs\x003\x00" + strings.Repeat("é", 500) + "\n"
	hits := filesHits(out)
	want := []filesHit{
		{"src/mui.rs", 112, "pub fn apply(&mut self) {"},
		{"tests/a b:c.rs", 12, "assert!(view.apply(x));"},
		{"tests/a b:c.rs", 27, "apply(\x00)"},
		{"src/long.rs", 3, strings.Repeat("é", filesHitMost)},
	}
	if !slices.Equal(hits, want) {
		t.Fatalf("hits = %+v", hits)
	}
	entries := filesGrouped(hits[:3], 0)
	wantEntries := []filesEntry{
		{Head: "src/mui.rs", Note: "1"},
		{ID: "h:0", Title: "pub fn apply(&mut self) {", Right: "112"},
		{Head: "tests/a b:c.rs", Note: "2"},
		{ID: "h:1", Title: "assert!(view.apply(x));", Right: "12"},
		{ID: "h:2", Title: "apply(\x00)", Right: "27"},
	}
	if !reflect.DeepEqual(entries, wantEntries) {
		t.Errorf("entries = %s", encode(entries))
	}
	// those found later are known by their place among all
	if later := filesGrouped(hits[3:], 3); len(later) != 2 || later[1].ID != "h:3" {
		t.Errorf("later = %s", encode(later))
	}
	if place, ok := filesHitAt("h:12"); place != 12 || !ok {
		t.Errorf("the place of h:12 = %d, %v", place, ok)
	}
	if _, ok := filesHitAt("12"); ok {
		t.Error("a line of the code read as a hit")
	}
}

// A file is its lines numbered from 1, a tab as git's diff shows it; from
// a later line on, a gap says how many are above; past the most, a gap
// says how many more there are. An empty file and one that is not text
// are a gap that says so.
func TestAFileIsItsLinesNumbered(t *testing.T) {
	lines, total := filesLines([]byte("package cli\n\n\tx := 1\r\n"), 1, filesLinesMost)
	if want := []filesLine{{New: 1, Text: "package cli"}, {New: 2, Text: ""}, {New: 3, Text: "    x := 1"}}; !slices.Equal(lines, want) || total != 3 {
		t.Errorf("%d lines: %+v", total, lines)
	}
	if got := string(encode(lines[0])); got != `{"new":1,"text":"package cli"}` {
		t.Errorf("a line is said as %s", got)
	}
	lines, total = filesLines([]byte(lineNumbered(1, 20, nil)), 9, 5)
	want := []filesLine{{Kind: "gap", Text: "8 lines above"}, {New: 9, Text: "line 9"}, {New: 10, Text: "line 10"}, {New: 11, Text: "line 11"}, {New: 12, Text: "line 12"}, {New: 13, Text: "line 13"}, {Kind: "gap", Text: "7 more lines"}}
	if !slices.Equal(lines, want) || total != 20 {
		t.Errorf("from line 9, %d lines: %+v", total, lines)
	}
	// no line before the first, none after the last
	if lines, _ := filesLines([]byte("one\ntwo"), -7, 9); !slices.Equal(lines, []filesLine{{New: 1, Text: "one"}, {New: 2, Text: "two"}}) {
		t.Errorf("from before the first: %+v", lines)
	}
	for content, gap := range map[string]string{"": "an empty file", "\x7fELF\x00\x01": "a binary file"} {
		if lines, total := filesLines([]byte(content), 1, 9); !slices.Equal(lines, []filesLine{{Kind: "gap", Text: gap}}) || total != 0 {
			t.Errorf("%q: %+v", content, lines)
		}
	}
	for _, c := range []struct {
		mark        byte
		total, from int
		want        string
	}{{'A', 19, 1, "new against main · 19 lines"}, {'M', 1, 1, "changed against main · 1 line"}, {'D', 0, 1, "deleted against main"}, {0, 311, 239, "311 lines · from line 239"}, {0, 0, 1, ""}} {
		if got := filesState(c.mark, "main", c.total, c.from); got != c.want {
			t.Errorf("state = %q, want %q", got, c.want)
		}
	}
}

// The view is the message mia desk draws as its own fixture of this app:
// a split row, the tree at the left under the repository's name and how
// many files changed, and beside it the path of the file under the keys,
// how it stands and its lines numbered; then the keys.
func TestTheViewIsTheTreeAndAFileSideBySide(t *testing.T) {
	t.Setenv("EDITOR", "")
	view := filesDeskView()
	messages := view.messages(filesAllPut)
	if len(messages) != 1 {
		t.Fatalf("%d messages", len(messages))
	}
	filesSaid(t, messages[0])
	var got, want map[string]any // compared as JSON, whatever the order of a piece's fields
	if err := json.Unmarshal(messages[0], &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(filesDeskSaid), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the view is said as\n%s\nand mia desk draws\n%s", messages[0], filesDeskSaid)
	}

	// said after the person moved: the keys stay where they put them
	if list := filesPieceOf(filesSaid(t, view.messages(0)[0]).View, "tree"); list.Selected != "" {
		t.Errorf("selected = %q, with no place said", list.Selected)
	}
	// the editor is named by its program, and a failure is the status
	t.Setenv("EDITOR", "/usr/local/bin/hx --wait")
	view.failed = "fatal: not a git repository"
	said := filesSaid(t, view.messages(0)[0])
	if said.Keys[0] != (gitKey{"e", "edit in hx", "edit"}) || said.Status == nil || *said.Status != (muiStatus{"fatal: not a git repository", "bad"}) {
		t.Errorf("keys = %+v, status = %+v", said.Keys, said.Status)
	}
	// with no file under the keys the right side says so
	view.file, view.lines, view.total = "", nil, 0
	if got := filesTexts(*filesSaid(t, view.messages(0)[0]).View); !slices.Equal(got, []string{"nothing to show"}) {
		t.Errorf("texts = %q", got)
	}
}

// Finding a file, the left side is a bare field that walks the list of
// what it found, said with each change of its words; finding in files, a
// field over the lines found by file, with the line under the keys the
// one the code's keys are put on. esc is the program's in both.
func TestTheLeftSideIsAFieldOverWhatWasFound(t *testing.T) {
	t.Setenv("EDITOR", "")
	view := filesDeskView()
	view.act(filesDeed{do: filesFind})
	view.act(filesDeed{do: filesTyped, text: "mui"})
	got := filesSaid(t, view.messages(filesAllPut)[0])
	left := got.View.Children[0]
	if left.W != 300 || len(left.Children) != 3 || left.Children[1].T != "divider" {
		t.Fatalf("the left side = %+v", left)
	}
	typed := "mui"
	if field := left.Children[0]; !reflect.DeepEqual(field, filesPiece{T: "field", ID: "q", Bare: true, Icon: "search", Live: true, List: "found", Placeholder: "Find a file", Right: "4 files", Value: &typed}) {
		t.Errorf("the field = %s", encode(field))
	}
	list := filesPieceOf(got.View, "found")
	if list == nil || !list.Live || list.Selected != "src/mui.rs" || len(list.Items) != 4 || list.Items[3].ID != "src/mui/patch.rs" { // mui is in its folder's name, not its own
		t.Errorf("the list = %s", encode(list))
	}
	if want := []gitKey{{"e", "edit in nvim", "edit"}, {"esc", "back to the tree", "back"}}; !slices.Equal(got.Keys, want) {
		t.Errorf("keys = %+v", got.Keys)
	}
	// said as the words change: what is typed stays the person's
	if field := filesSaid(t, view.messages(filesKeysPut)[0]).View.Children[0].Children[0]; field.Value != nil {
		t.Errorf("the field's value said as %q", *field.Value)
	}

	view = filesDeskView()
	view.mode, view.typed, view.words = filesInGrep, "apply(", "apply("
	view.hits = []filesHit{{"src/mui.rs", 112, "pub fn apply(&mut self) {"}, {"src/talk.rs", 12, "next.apply(change)?;"}}
	view.hitAt, view.file, view.line = 1, "src/talk.rs", 12
	view.lines, view.total = filesLines([]byte(lineNumbered(1, 30, nil)), view.line-filesBefore, filesHitLinesMost)
	view.status = &muiStatus{Text: "2 in 2 files"}
	got = filesSaid(t, view.messages(filesAllPut)[0])
	left = got.View.Children[0]
	if field := left.Children[0]; left.W != 440 || field.List != "hits" || field.Placeholder != "Find in files" || !field.Bare || field.Value == nil || *field.Value != "apply(" {
		t.Errorf("the left side = %s", encode(left))
	}
	if list := filesPieceOf(got.View, "hits"); list == nil || !list.Live || list.Selected != "h:1" || len(list.Items) != 4 || list.Items[2] != (filesEntry{Head: "src/talk.rs", Note: "1"}) {
		t.Errorf("the list = %s", encode(list))
	}
	if want := []string{"src/talk.rs", "changed against main · 30 lines · from line 4"}; !slices.Equal(filesTexts(*got.View), want) {
		t.Errorf("texts = %q", filesTexts(*got.View))
	}
	// 3 lines above as a gap, then line 4 on: line 12 is the ninth place
	if code := filesPieceOf(got.View, "file"); code.Selected != "9" || *got.Status != (muiStatus{Text: "2 in 2 files"}) {
		t.Errorf("the code's keys on %q, status %+v", code.Selected, got.Status)
	}
	if code := filesPieceOf(filesSaid(t, view.messages(0)[0]).View, "file"); code.Selected != "" {
		t.Errorf("the code's keys said on %q, with no place said", code.Selected)
	}
	// hits found later are patches that put them last in the list
	patches := view.patches([]filesHit{{"tests/a.rs", 3, "apply(1)"}, {"tests/a.rs", 9, "apply(2)"}}, 2)
	if len(patches) != 1 {
		t.Fatalf("%d patches", len(patches))
	}
	var ops []filesItemOp
	for _, raw := range filesSaid(t, patches[0]).Patch {
		var op filesItemOp
		if err := json.Unmarshal(raw, &op); err != nil {
			t.Fatal(err)
		}
		ops = append(ops, op)
	}
	if want := []filesItemOp{{"item", "hits", filesEntry{Head: "tests/a.rs", Note: "2"}}, {"item", "hits", filesEntry{ID: "h:2", Title: "apply(1)", Right: "3"}}, {"item", "hits", filesEntry{ID: "h:3", Title: "apply(2)", Right: "9"}}}; !reflect.DeepEqual(ops, want) {
		t.Errorf("ops = %+v", ops)
	}
}

// What is said fits what the window takes, whatever the files: a file
// longer than a message goes on in patches of lines, all of them in
// order; the most rows of the tree, and the most hits each under a head
// of its own, with the most lines beside them are under the pieces a UI
// may have; and a list of long names past ASCII is cut to fewer of them.
func TestWhatIsSaidFitsWhatTheWindowTakes(t *testing.T) {
	view := filesDeskView()
	wide := strings.Repeat("é", gitLineMost)
	var content strings.Builder
	for range filesLinesMost + 200 {
		content.WriteString(wide + "\n")
	}
	view.lines, view.total = filesLines([]byte(content.String()), 1, filesLinesMost)
	messages := view.messages(filesAllPut)
	if len(messages) < 3 {
		t.Fatalf("%d messages for a file of %d bytes", len(messages), content.Len())
	}
	lines := filesCode(t, messages)
	if !slices.Equal(lines, view.lines) || len(lines) != filesLinesMost+1 || lines[filesLinesMost] != (filesLine{Kind: "gap", Text: "200 more lines"}) {
		t.Errorf("%d lines said of %d, the last %+v", len(lines), len(view.lines), lines[len(lines)-1])
	}

	var paths []string
	for n := range filesRowsMost + 50 {
		paths = append(paths, fmt.Sprintf("file%04d.txt", n))
	}
	view.paths, view.rows = paths, filesTree(paths, view.open)
	view.lines, view.total = filesLines([]byte(lineNumbered(1, 3000, nil)), 1, filesLinesMost)
	said := filesSaid(t, view.messages(filesAllPut)[0])
	if items := filesPieceOf(said.View, "tree").Items; len(items) != filesRowsMost+2 || items[filesRowsMost+1].Head != "50 more rows" {
		t.Errorf("%d entries of the tree", len(items))
	}
	if n := filesCounted(t, *said.View); n > muiPieces {
		t.Errorf("%d pieces with the most rows and lines, over the %d a UI may have", n, muiPieces)
	}

	view.mode, view.hits = filesInGrep, nil
	for n := range filesHitsMost {
		view.hits = append(view.hits, filesHit{fmt.Sprintf("src/file%d.rs", n), n + 1, "apply(x)"})
	}
	view.lines, view.total = filesLines([]byte(lineNumbered(1, 3000, nil)), 500, filesHitLinesMost)
	said = filesSaid(t, view.messages(filesAllPut)[0])
	if items := filesPieceOf(said.View, "hits").Items; len(items) != 2*filesHitsMost {
		t.Errorf("%d entries of the hits", len(items))
	}
	if n := filesCounted(t, *said.View); n > muiPieces {
		t.Errorf("%d pieces with the most hits and lines, over the %d a UI may have", n, muiPieces)
	}
	patches := view.patches(view.hits, 0)
	for _, patch := range patches {
		if ops := filesSaid(t, patch).Patch; len(ops) == 0 || len(ops) > muiOps {
			t.Errorf("a patch of %d changes", len(ops))
		}
	}
	if len(patches) != 2 {
		t.Errorf("%d patches for %d changes", len(patches), 2*filesHitsMost)
	}

	for n := range view.hits {
		view.hits[n].text = strings.Repeat("é", filesHitMost)
		view.hits[n].path = strings.Repeat("é", gitWordsMost) + fmt.Sprint(n)
	}
	said = filesSaid(t, view.messages(filesAllPut)[0])
	if items := filesPieceOf(said.View, "hits").Items; len(items) == 0 || len(items) >= 2*filesHitsMost {
		t.Errorf("%d entries of hits too long to say all of", len(items))
	}
}

// What each event does: a row come under the keys shows its file, ⏎ on a
// folder opens or shuts it and on a file of the tree does nothing more;
// e edits the file of the row it is pressed on, or the line of the code;
// ⏎ on what a search found goes to it in the tree.
func TestWhatAnEventDoesToTheFiles(t *testing.T) {
	tree := filesDeskView()
	find := filesDeskView()
	find.act(filesDeed{do: filesFind})
	find.act(filesDeed{do: filesTyped, text: "mui"})
	grep := filesDeskView()
	grep.mode, grep.typed, grep.words, grep.hitAt = filesInGrep, "apply(", "apply(", 0
	grep.hits = []filesHit{{"src/mui.rs", 112, "pub fn apply(&mut self) {"}, {"src/talk.rs", 12, "next.apply(change)?;"}}
	other := filesDeskView()
	other.mode, other.typed, other.words, other.hits, other.hitAt = filesInGrep, "patch(", "apply(", grep.hits, 0

	for _, c := range []struct {
		view  *filesView
		event string
		want  filesDeed
	}{
		{tree, `{"e":"close"}`, filesDeed{do: filesLeave}},
		{tree, `{"e":"move","list":"tree","id":"src/mui.rs"}`, filesDeed{do: filesShow, id: "src/mui.rs", path: "src/mui.rs"}},
		{tree, `{"e":"move","list":"tree","id":"src/mui/patch.rs"}`, filesDeed{}}, // where the keys are
		{tree, `{"e":"move","list":"tree","id":"docs"}`, filesDeed{do: filesOn, id: "docs"}},
		{tree, `{"e":"move","list":"tree","id":"gone.rs"}`, filesDeed{}},
		{tree, `{"e":"move","list":"file","id":"2"}`, filesDeed{}},
		{tree, `{"e":"pick","list":"tree","id":"docs"}`, filesDeed{do: filesToggle, id: "docs", path: "docs"}},
		{tree, `{"e":"pick","list":"tree","id":"src/mui/patch.rs"}`, filesDeed{}}, // its file is at the right already
		{tree, `{"e":"pick","list":"tree","id":"src/talk.rs"}`, filesDeed{do: filesShow, id: "src/talk.rs", path: "src/talk.rs"}},
		{tree, `{"e":"pick","list":"file","id":"1"}`, filesDeed{}},
		{tree, `{"e":"press","id":"edit","item":"src/talk.rs"}`, filesDeed{do: filesEdit, path: "src/talk.rs", line: 1}},
		{tree, `{"e":"press","id":"edit","item":"src"}`, filesDeed{}},
		{tree, `{"e":"press","id":"edit","item":"2"}`, filesDeed{do: filesEdit, path: "src/mui/patch.rs", line: 3}},
		{tree, `{"e":"press","id":"edit"}`, filesDeed{do: filesEdit, path: "src/mui/patch.rs", line: 1}},
		{tree, `{"e":"press","id":"find"}`, filesDeed{do: filesFind}},
		{tree, `{"e":"press","id":"grep"}`, filesDeed{do: filesGrep}},
		{tree, `{"e":"press","id":"again"}`, filesDeed{do: filesAgain}},
		{tree, `{"e":"press","id":"back"}`, filesDeed{}},
		{tree, `{"e":"change","id":"q","text":"x"}`, filesDeed{}},
		{tree, `{"e":"submit","id":"q","text":"x"}`, filesDeed{}},
		{tree, `{"e":"press","id":"unknown"}`, filesDeed{}},

		{find, `{"e":"change","id":"q","text":"mu"}`, filesDeed{do: filesTyped, text: "mu"}},
		{find, `{"e":"change","id":"q","text":"mui"}`, filesDeed{}}, // as it is
		{find, `{"e":"move","list":"found","id":"src/mui/patch.rs"}`, filesDeed{do: filesShow, path: "src/mui/patch.rs"}},
		{find, `{"e":"move","list":"found","id":"src/mui.rs"}`, filesDeed{}},
		{find, `{"e":"pick","list":"found","id":"src/mui.rs"}`, filesDeed{do: filesGo, path: "src/mui.rs"}},
		{find, `{"e":"pick","list":"found","id":"src/talk.rs"}`, filesDeed{}}, // not among those found
		{find, `{"e":"press","id":"edit","item":"src/mui/patch.rs"}`, filesDeed{do: filesEdit, path: "src/mui/patch.rs", line: 1}},
		{find, `{"e":"press","id":"back"}`, filesDeed{do: filesBack}},
		{find, `{"e":"press","id":"find"}`, filesDeed{}},
		{find, `{"e":"press","id":"again"}`, filesDeed{}},
		{find, `{"e":"submit","id":"q","text":"mui"}`, filesDeed{}},

		{grep, `{"e":"submit","id":"q","text":"patch("}`, filesDeed{do: filesSeek, text: "patch("}},
		{grep, `{"e":"change","id":"q","text":"appl"}`, filesDeed{do: filesTyped, text: "appl"}},
		{grep, `{"e":"move","list":"hits","id":"h:1"}`, filesDeed{do: filesShow, id: "h:1", path: "src/talk.rs", line: 12}},
		{grep, `{"e":"move","list":"hits","id":"h:0"}`, filesDeed{}},
		{grep, `{"e":"move","list":"hits","id":"h:7"}`, filesDeed{}},
		{grep, `{"e":"pick","list":"hits","id":"h:1"}`, filesDeed{do: filesGo, path: "src/talk.rs", line: 12}},
		{grep, `{"e":"press","id":"edit","item":"h:1"}`, filesDeed{do: filesEdit, path: "src/talk.rs", line: 12}},
		{grep, `{"e":"press","id":"edit","item":"0"}`, filesDeed{do: filesEdit, path: "src/mui/patch.rs", line: 1}},
		{grep, `{"e":"press","id":"back"}`, filesDeed{do: filesBack}},
		// ⏎ on a line found for other words than the field holds searches for those
		{other, `{"e":"pick","list":"hits","id":"h:1"}`, filesDeed{do: filesSeek, text: "patch("}},
	} {
		events, rest := muiEvents[gitEvent](chatMark + c.event)
		if len(events) != 1 || rest != "" {
			t.Fatalf("%s read as %+v", c.event, events)
		}
		if got := c.view.deed(events[0]); got != c.want {
			t.Errorf("%s does %+v, want %+v", c.event, got, c.want)
		}
	}
}

// What is done is done to the view: a folder opens and shuts with the
// keys on it, a move says the file without saying where the keys are,
// what is typed narrows the files found with the keys on the best, ⏎ on
// one opens the folders on the way to it in the tree, and esc is back in
// the tree as it was.
func TestWhatIsDoneToTheFiles(t *testing.T) {
	gittest.Isolate(t)
	view := filesDeskView()
	view.top = t.TempDir() // none of the files is there to read
	if say, place := view.act(filesDeed{do: filesToggle, id: "docs", path: "docs"}); !say || place != 0 || view.at != "docs" || !slices.Contains(filesIDs(view.rows), "docs/apps.md") {
		t.Errorf("docs opened: said %v at %d, rows %q", say, place, filesIDs(view.rows))
	}
	if view.act(filesDeed{do: filesToggle, id: "docs", path: "docs"}); slices.Contains(filesIDs(view.rows), "docs/apps.md") {
		t.Errorf("docs shut: rows %q", filesIDs(view.rows))
	}
	if say, place := view.act(filesDeed{do: filesOn, id: "tests"}); say || place != 0 || view.at != "tests" || view.file != "src/mui/patch.rs" {
		t.Errorf("the keys on a folder: said %v, at %q beside %q", say, view.at, view.file)
	}
	if say, place := view.act(filesDeed{do: filesShow, id: "src/talk.rs", path: "src/talk.rs"}); !say || place&filesKeysPut != 0 || view.at != "src/talk.rs" || view.file != "src/talk.rs" || !slices.Equal(view.lines, []filesLine{{Kind: "gap", Text: "no longer there"}}) {
		t.Errorf("a move: said %v at %d, at %q, lines %+v", say, place, view.at, view.lines)
	}

	if say, place := view.act(filesDeed{do: filesFind}); !say || place != filesAllPut || view.mode != filesInFind || view.matched != len(filesDesk) || view.foundAt != "README.md" || view.file != "README.md" {
		t.Errorf("find: said %v at %d, %d found, the keys on %q", say, place, view.matched, view.foundAt)
	}
	if say, place := view.act(filesDeed{do: filesTyped, text: "tests"}); !say || place != filesKeysPut || view.typed != "tests" || !slices.Equal(view.found, []string{"tests/mui_patch.rs"}) || view.file != "tests/mui_patch.rs" {
		t.Errorf("typed: said %v at %d, found %q beside %q", say, place, view.found, view.file)
	}
	if view.act(filesDeed{do: filesTyped, text: "zzz"}); len(view.found) != 0 || view.foundAt != "" || view.file != "tests/mui_patch.rs" {
		t.Errorf("nothing found: %q, the keys on %q beside %q", view.found, view.foundAt, view.file)
	}
	back := *view
	if say, place := back.act(filesDeed{do: filesBack}); !say || place != filesKeysPut || back.mode != filesInTree || back.at != "src/talk.rs" || back.file != "src/talk.rs" || back.typed != "" {
		t.Errorf("back: said %v at %d, at %q beside %q", say, place, back.at, back.file)
	}
	if say, place := view.act(filesDeed{do: filesGo, path: "tests/mui_patch.rs"}); !say || place != filesKeysPut|filesLinePut || view.mode != filesInTree || view.at != "tests/mui_patch.rs" || !view.open["tests"] || !slices.Contains(filesIDs(view.rows), "tests/mui_patch.rs") {
		t.Errorf("go: said %v at %d, at %q, rows %q", say, place, view.at, filesIDs(view.rows))
	}

	// finding in files: other words than were searched for drop what was found
	view.act(filesDeed{do: filesGrep})
	view.words, view.hits, view.hitAt = "apply(", []filesHit{{"src/mui.rs", 112, "apply("}}, 0
	if say, _ := view.act(filesDeed{do: filesTyped, text: "apply("}); say || len(view.hits) != 1 {
		t.Errorf("the words searched for typed: said %v, %d hits", say, len(view.hits))
	}
	if say, place := view.act(filesDeed{do: filesTyped, text: "appl"}); !say || place != 0 || len(view.hits) != 0 || view.hitAt != -1 || view.words != "" || view.typed != "appl" {
		t.Errorf("other words typed: said %v at %d, %d hits", say, place, len(view.hits))
	}
	if say, _ := view.act(filesDeed{do: filesSeek, text: "  "}); !say || view.search != nil || view.status != nil {
		t.Errorf("no words searched for: said %v, status %+v", say, view.status)
	}

	// what a search found comes in batches: the first is said whole with the
	// keys on its first line, the last says how it ended
	view.words = "apply("
	if whole, place := view.take(filesBatched{hits: []filesHit{{"src/mui.rs", 112, "apply("}}}); !whole || place != filesKeysPut|filesLinePut || view.hitAt != 0 || view.file != "src/mui.rs" || view.line != 112 {
		t.Errorf("the first batch: whole %v at %d, the keys on %d in %q", whole, place, view.hitAt, view.file)
	}
	if whole, _ := view.take(filesBatched{hits: []filesHit{{"src/talk.rs", 12, "apply("}}}); whole || len(view.hits) != 2 {
		t.Errorf("a later batch: whole %v, %d hits", whole, len(view.hits))
	}
	if whole, place := view.take(filesBatched{hits: []filesHit{{"src/talk.rs", 40, "apply("}}, done: true}); !whole || place != 0 || *view.status != (muiStatus{Text: "3 in 2 files"}) {
		t.Errorf("the last batch: whole %v at %d, status %+v", whole, place, view.status)
	}
	if view.take(filesBatched{done: true, more: true}); *view.status != (muiStatus{Text: "the first 3, in 2 files: there are more", Tone: "warn"}) {
		t.Errorf("stopped at the most: %+v", view.status)
	}
	if view.take(filesBatched{done: true, failed: "fatal: no such thing"}); *view.status != (muiStatus{Text: "fatal: no such thing", Tone: "bad"}) {
		t.Errorf("refused: %+v", view.status)
	}
	view.hits = nil
	if view.take(filesBatched{done: true}); *view.status != (muiStatus{Text: "nothing found"}) {
		t.Errorf("none found: %+v", view.status)
	}
}

// filesWork is a throwaway repository with a branch, work, off main: on
// it one file changed and one added in a commit, one deleted and one
// untracked on disk, and files git ignores.
func filesWork(t *testing.T) *gittest.Repo {
	t.Helper()
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write(".gitignore", "*.log\nbuild/\n")
	repo.Write("src/keep.txt", "one\ntwo\n")
	repo.Write("src/old.txt", "old\n")
	repo.Write("src/same.txt", "same\n")
	repo.Commit("the base")
	repo.Git("checkout", "-q", "-b", "work")
	repo.Write("src/keep.txt", "one\n\ttwo, Changed\nthree\n")
	repo.Write("src/new/added.txt", "added\n")
	repo.Commit("work's")
	if err := os.Remove(filepath.Join(repo.Root, "src/old.txt")); err != nil {
		t.Fatal(err)
	}
	repo.Write("top.txt", "untracked, and Changed\n")
	repo.Write("debug.log", "ignored, and Changed\n")
	repo.Write("build/out.txt", "ignored, and Changed\n")
	return repo
}

// The tree is the files git tracks and those it does not, ignored ones
// and .git left out, run from any folder of the repository; each file
// changed against the base — the branch main here — carries its mark, a
// deleted one is still listed, and the folders on the way to them are
// open with the keys on the first.
func TestTheTreeIsTheFilesGitDoesNotIgnoreMarkedAgainstTheBase(t *testing.T) {
	repo := filesWork(t)
	status := repo.Git("status", "--porcelain")
	view := &filesView{pane: "%4", dir: filepath.Join(repo.Root, "src", "new"), hitAt: -1}
	view.read()
	if want := []string{".gitignore", "README.md", "src/keep.txt", "src/new/added.txt", "src/old.txt", "src/same.txt", "top.txt"}; !slices.Equal(view.paths, want) || view.failed != "" {
		t.Fatalf("paths = %q, %q", view.paths, view.failed)
	}
	if want := map[string]byte{"src/keep.txt": 'M', "src/new/added.txt": 'A', "src/old.txt": 'D', "top.txt": 'A'}; !reflect.DeepEqual(view.marks, want) || view.against != "main" {
		t.Errorf("marks = %v against %q", view.marks, view.against)
	}
	if want := []string{"src", "src/new", "src/new/added.txt", "src/keep.txt", "src/old.txt", "src/same.txt", ".gitignore", "README.md", "top.txt"}; !slices.Equal(filesIDs(view.rows), want) {
		t.Errorf("rows = %q", filesIDs(view.rows))
	}
	if view.at != "src/new/added.txt" || view.file != "src/new/added.txt" || view.top != repo.Root || view.name != "app" {
		t.Errorf("the keys on %q beside %q, in %q of %q", view.at, view.file, view.top, view.name)
	}
	got := filesSaid(t, view.messages(filesAllPut)[0])
	list := filesPieceOf(got.View, "tree")
	if list.Items[0] != (filesEntry{Head: "app", Note: "4 changed against main"}) || list.Selected != "src/new/added.txt" {
		t.Errorf("the list = %s", encode(list))
	}
	if want := []string{"src/new/added.txt", "new against main · 1 line"}; !slices.Equal(filesTexts(*got.View), want) {
		t.Errorf("texts = %q", filesTexts(*got.View))
	}

	// a file's lines, a tab as spaces; a deleted file says it is
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/keep.txt"}))
	if want := []filesLine{{New: 1, Text: "one"}, {New: 2, Text: "    two, Changed"}, {New: 3, Text: "three"}}; !slices.Equal(view.lines, want) || view.total != 3 {
		t.Errorf("lines = %+v", view.lines)
	}
	if got := filesTexts(*filesSaid(t, view.messages(0)[0]).View); !slices.Equal(got, []string{"src/keep.txt", "changed against main · 3 lines"}) {
		t.Errorf("texts = %q", got)
	}
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/old.txt"}))
	if want := []filesLine{{Kind: "gap", Text: "deleted against main"}}; !slices.Equal(view.lines, want) {
		t.Errorf("a deleted file's lines = %+v", view.lines)
	}
	// one that is not text, and one too long to read, say so
	repo.Write("src/blob.bin", "\x00\x01\x02")
	repo.Write("src/huge.txt", strings.Repeat("0123456789abcde\n", filesFileMost/16+1))
	for path, want := range map[string]string{"src/blob.bin": "a binary file", "src/huge.txt": "a file of 2 MB", "src/new": "not a file to read"} {
		view.file = path
		if view.load(); !slices.Equal(view.lines, []filesLine{{Kind: "gap", Text: want}}) {
			t.Errorf("%s: %+v", path, view.lines)
		}
	}

	// read again: what changed is seen, the keys stay, and what the person
	// opened stays open
	mark := view.changed()
	repo.Write("src/same.txt", "same\nno longer\n")
	if again := view.changed(); again == mark {
		t.Error("a file changed, and what the poll looks at did not")
	}
	view.act(filesDeed{do: filesShow, id: "src/same.txt", path: "src/same.txt"})
	if say, place := view.act(filesDeed{do: filesAgain}); !say || place != 0 || view.marks["src/same.txt"] != 'M' || view.at != "src/same.txt" || view.total != 2 || !view.open["src/new"] {
		t.Errorf("again: said %v at %d, marks %v, at %q", say, place, view.marks, view.at)
	}
	// on the base branch itself the marks are against the last commit
	other := &filesView{dir: gittest.New(t).Root, hitAt: -1}
	other.read()
	if other.against != "the last commit" || len(other.marks) != 0 || other.at != "README.md" {
		t.Errorf("on main: %v against %q, the keys on %q", other.marks, other.against, other.at)
	}
	if note := filesSaid(t, other.messages(0)[0]).View.Children[0].Children[0].Items[0].Note; note != "nothing changed against the last commit" {
		t.Errorf("the head's note = %q", note)
	}
	// nothing of the repository was written
	repo.Write("src/same.txt", "same\n")
	for _, path := range []string{"src/blob.bin", "src/huge.txt"} {
		if err := os.Remove(filepath.Join(repo.Root, path)); err != nil {
			t.Fatal(err)
		}
	}
	if now := repo.Git("status", "--porcelain"); now != status {
		t.Errorf("git status was\n%s\nand is\n%s", status, now)
	}
}

// filesSought waits for the search at work to end, taking in what it finds.
func filesSought(t *testing.T, view *filesView) {
	t.Helper()
	for view.search != nil {
		select {
		case batch := <-view.results():
			view.take(batch)
		case <-time.After(20 * time.Second):
			t.Fatal("the search did not end")
		}
	}
}

// Finding in files finds the words as they are written, whatever their
// case when they are typed in lower case, in the files git tracks and in
// an untracked one, and in none it ignores; ⏎ on a line found is back in
// the tree with the keys on its file, and it stops at the most hits.
func TestFindingInFilesFindsALineOfAnUntrackedFile(t *testing.T) {
	repo := filesWork(t)
	view := &filesView{dir: repo.Root, hitAt: -1}
	view.read()
	view.act(view.deed(gitEvent{E: "press", ID: "grep"}))
	if say, _ := view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "changed"})); !say || view.search == nil || *view.status != (muiStatus{Text: "searching…"}) {
		t.Fatalf("searching: said %v, status %+v, %q", say, view.status, view.failed)
	}
	filesSought(t, view)
	want := []filesHit{{"src/keep.txt", 2, "two, Changed"}, {"top.txt", 1, "untracked, and Changed"}}
	if !slices.Equal(view.hits, want) || *view.status != (muiStatus{Text: "2 in 2 files"}) {
		t.Fatalf("hits = %+v, status %+v", view.hits, view.status)
	}
	if view.hitAt != 0 || view.file != "src/keep.txt" || view.line != 2 {
		t.Errorf("the keys on %d, in %q at %d", view.hitAt, view.file, view.line)
	}
	// the line under the keys of the list is the one shown
	if say, place := view.act(view.deed(gitEvent{E: "move", List: "hits", ID: "h:1"})); !say || place != filesLinePut || view.file != "top.txt" || view.line != 1 {
		t.Errorf("a move: said %v at %d, %q at %d", say, place, view.file, view.line)
	}
	// with a capital the case is as written, and a pattern's signs are letters
	view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "Changed"}))
	filesSought(t, view)
	if !slices.Equal(view.hits, want) {
		t.Errorf("Changed found %+v", view.hits)
	}
	view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "untracked"}))
	filesSought(t, view)
	if !slices.Equal(view.hits, want[1:]) {
		t.Errorf("untracked found %+v", view.hits)
	}
	view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "t.o"}))
	if filesSought(t, view); len(view.hits) != 0 || *view.status != (muiStatus{Text: "nothing found"}) {
		t.Errorf("t.o found %+v, status %+v", view.hits, view.status)
	}

	view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "untracked"}))
	filesSought(t, view)
	if say, place := view.act(view.deed(gitEvent{E: "pick", List: "hits", ID: "h:0"})); !say || place != filesKeysPut|filesLinePut || view.mode != filesInTree || view.at != "top.txt" || view.line != 1 || len(view.hits) != 0 {
		t.Errorf("⏎ on a hit: said %v at %d, in the tree on %q", say, place, view.at)
	}

	// a search is stopped at the most hits, and by the next
	var many strings.Builder
	for range 3 * filesHitsMost {
		many.WriteString("needle\n")
	}
	repo.Write("hay.txt", many.String())
	search, err := filesSearchFor(repo.Root, "needle", true, 40)
	if err != nil {
		t.Fatal(err)
	}
	var all []filesHit
	for batch := range search.found {
		if all = append(all, batch.hits...); batch.done {
			if !batch.more || batch.failed != "" {
				t.Errorf("the last batch = %+v", batch)
			}
			break
		}
	}
	if len(all) != 40 || all[39] != (filesHit{"hay.txt", 40, "needle"}) {
		t.Errorf("%d hits at a most of 40", len(all))
	}
	view.act(filesDeed{do: filesGrep})
	view.act(filesDeed{do: filesSeek, text: "needle"})
	first := view.search
	view.act(filesDeed{do: filesSeek, text: "needles"})
	select {
	case _, open := <-first.found:
		if open {
			t.Error("a search went on once the next began")
		}
	case <-time.After(100 * time.Millisecond): // it says no more
	}
	view.quiet()
}

// e hands the file to $EDITOR as +<line> and the path from the worktree's
// top, run there, and the files are read again when it leaves; an editor
// that cannot be run is the view's status, and one that leaves with an
// error has said so itself.
func TestTheEditorIsHandedTheFileAtTheLine(t *testing.T) {
	repo := filesWork(t)
	said := filepath.Join(t.TempDir(), "said")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\npwd > \"$FILES_SAID\"\nprintf '%s\\n' \"$@\" >> \"$FILES_SAID\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILES_SAID", said)
	t.Setenv("EDITOR", editor+" --wait")
	view := &filesView{dir: filepath.Join(repo.Root, "src"), hitAt: -1}
	view.read()
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/keep.txt"}))

	ran := func(event gitEvent) []string {
		t.Helper()
		deed := view.deed(event)
		if deed.do != filesEdit {
			t.Fatalf("%+v does %+v", event, deed)
		}
		view.run(deed)
		out, err := os.ReadFile(said)
		if err != nil || view.failed != "" {
			t.Fatalf("%v, %q", err, view.failed)
		}
		return strings.Split(strings.TrimSpace(string(out)), "\n")
	}
	// on a line of the file: that line; on its row in the tree: the first
	if got, want := ran(gitEvent{E: "press", ID: "edit", Item: "2"}), []string{repo.Root, "--wait", "+3", "src/keep.txt"}; !slices.Equal(got, want) {
		t.Errorf("the editor was run as %q", got)
	}
	if got, want := ran(gitEvent{E: "press", ID: "edit", Item: "src/new/added.txt"}), []string{repo.Root, "--wait", "+1", "src/new/added.txt"}; !slices.Equal(got, want) {
		t.Errorf("the editor was run as %q", got)
	}
	// a path that begins as an option does is said as a path
	if got := filesEditor(repo.Root, "-n.txt", 4).Args; !slices.Equal(got, []string{editor, "--wait", "+4", "./-n.txt"}) {
		t.Errorf("args = %q", got)
	}
	t.Setenv("EDITOR", "")
	if got := filesEditor(repo.Root, "a.txt", 1); filepath.Base(got.Path) != "nvim" || got.Dir != repo.Root {
		t.Errorf("with no $EDITOR: %q in %q", got.Path, got.Dir)
	}

	t.Setenv("EDITOR", filepath.Join(t.TempDir(), "no-such-editor"))
	if view.run(filesDeed{do: filesEdit, path: "top.txt", line: 1}); !strings.Contains(view.failed, "no-such-editor") {
		t.Errorf("no such editor: %q", view.failed)
	}
	if got := filesSaid(t, view.messages(0)[0]).Status; got == nil || got.Tone != "bad" {
		t.Errorf("no such editor: status %+v", got)
	}
	t.Setenv("EDITOR", "false")
	if view.run(filesDeed{do: filesEdit, path: "top.txt", line: 1}); view.failed != "" {
		t.Errorf("an editor that left with an error: %q", view.failed)
	}
}

// A folder in no repository is its own tree, with no mark and nothing
// failed, no .git looked in; its files are searched all the same.
func TestAFolderInNoRepositoryIsItsOwnTree(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	for path, content := range map[string]string{"notes.md": "a needle\n", "sub/deep.txt": "no hay\nthe Needle\n", ".git/config": "needle\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	view := &filesView{pane: "%3", dir: filepath.Join(dir, "sub"), hitAt: -1}
	view.read()
	if !slices.Equal(view.paths, []string{"deep.txt"}) || view.inside || view.failed != "" || len(view.marks) != 0 {
		t.Fatalf("paths = %q, %q", view.paths, view.failed)
	}
	got := filesSaid(t, view.messages(filesAllPut)[0])
	if list := filesPieceOf(got.View, "tree"); got.Status != nil || list.Items[0] != (filesEntry{Head: "sub", Note: "in no repository"}) || list.Selected != "deep.txt" {
		t.Errorf("status %+v, the list = %s", got.Status, encode(list))
	}
	if want := []string{"deep.txt", "2 lines"}; !slices.Equal(filesTexts(*got.View), want) {
		t.Errorf("texts = %q", filesTexts(*got.View))
	}
	view.act(filesDeed{do: filesGrep})
	view.act(filesDeed{do: filesSeek, text: "needle"})
	if filesSought(t, view); !slices.Equal(view.hits, []filesHit{{"deep.txt", 2, "the Needle"}}) {
		t.Errorf("hits = %+v, status %+v", view.hits, view.status)
	}
	if paths, err := filesWalk(dir, filesWalkMost); err != nil || !slices.Equal(paths, []string{"notes.md", "sub/deep.txt"}) {
		t.Errorf("walked %q, %v", paths, err)
	}
	if paths, _ := filesWalk(dir, 1); len(paths) != 1 {
		t.Errorf("walked %q at a most of 1", paths)
	}
}

// mia files reads what the person does from a terminal, and is refused
// where stdin is none; it takes no argument, and needs no repository.
func TestFilesRefusesWhatIsNotATerminal(t *testing.T) {
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
	if code := Run([]string{"files"}); code != exitFailed {
		t.Errorf("files exited %d, want %d", code, exitFailed)
	}
	if code := Run([]string{"files", "src"}); code != exitUsage {
		t.Errorf("files src exited %d, want %d", code, exitUsage)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Features, "files-app") {
		t.Error("capabilities do not list files-app")
	}
}
