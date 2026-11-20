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

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/gittest"
)

// filesHeard is a message of mia files as the window reads it.
type filesHeard struct {
	V      int               `json:"v"`
	Pane   string            `json:"pane"`
	Fill   bool              `json:"fill"`
	Focus  string            `json:"focus"`
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

const filesDeskSaid = `{"v":1,"pane":"%1","fill":true,"view":{"t":"row","split":true,"children":[{"t":"col","pad":6,"w":300,"scroll":true,"children":[{"t":"list","id":"tree","menu":"what to do","live":true,"selected":"src/mui/patch.rs","items":[{"note":"3 changed against main","head":"mia-desk"},{"id":"docs","title":"docs","depth":0,"open":false,"menu":[{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"}]},{"id":"examples","title":"examples","depth":0,"open":false,"menu":[{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"}]},{"id":"src","title":"src","depth":0,"open":true,"menu":[{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"}]},{"id":"src/mui","title":"mui","depth":1,"open":true,"menu":[{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"}]},{"id":"src/mui/patch.rs","title":"patch.rs","depth":2,"right":"A","tone":"ok","menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"src/chatlook.rs","title":"chatlook.rs","depth":1,"menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"src/mui.rs","title":"mui.rs","depth":1,"right":"M","tone":"warn","menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"src/talk.rs","title":"talk.rs","depth":1,"right":"M","tone":"warn","menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"src/term.rs","title":"term.rs","depth":1,"menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"tests","title":"tests","depth":0,"open":false,"menu":[{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"}]},{"id":"Cargo.toml","title":"Cargo.toml","depth":0,"menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]},{"id":"README.md","title":"README.md","depth":0,"menu":[{"id":"edit","title":"Open in editor","right":"e"},{"id":"new","title":"New file here\u2026","right":"a"},{"id":"move","title":"Rename\u2026","right":"R"},{"id":"delete","title":"Delete\u2026","right":"D"},{"id":"copy","title":"Copy path","right":"y"},{"id":"diff","title":"Diff against the base","right":"d"},{"id":"blame","title":"Blame","right":"B"}]}]}]},{"t":"divider"},{"t":"col","id":"side","motion":"none","children":[{"t":"row","pad":10,"gap":10,"children":[{"t":"text","text":"src/mui/patch.rs","style":"mono"},{"t":"text","text":"new against main \u00b7 3 lines","style":"dim"}]},{"t":"code","id":"file","live":true,"lines":[{"new":1,"text":"//! A patch: changes to the UI shown, made in turn, all or none."},{"new":2,"text":"use crate::mui::{Change, Missing, Piece};"},{"new":3,"text":""}]}]}]},"keys":[{"key":"e","label":"edit in nvim","id":"edit"},{"key":"f","label":"find a file","id":"find"},{"key":"/","label":"find in files","id":"grep"},{"key":"a","label":"new file or folder","id":"new"},{"key":"R","label":"rename or move","id":"move"},{"key":"D","label":"delete","id":"delete"},{"key":"y","label":"copy the path","id":"copy"},{"key":"c","label":"changed only","id":"only"},{"key":"d","label":"diff against the base","id":"diff"},{"key":"B","label":"blame","id":"blame"},{"key":":","label":"go to line","id":"line"},{"key":"r","label":"ask again","id":"again"}]}`

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
	if len(cut) != 4 || !reflect.DeepEqual(cut[3], filesEntry{Head: "3 more rows", Note: "shut a folder to see them"}) {
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
		mark            byte
		total, from, to int
		want            string
	}{{'A', 19, 1, 19, "new against main · 19 lines"}, {'M', 1, 1, 1, "changed against main · 1 line"}, {'D', 0, 1, 0, "deleted against main"},
		{0, 311, 239, 311, "lines 239–311 of 311"}, {'M', 9000, 1, 1500, "changed against main · lines 1–1500 of 9000"}, {0, 0, 1, 0, ""}} {
		if got := filesState(c.mark, "main", c.total, c.from, c.to); got != c.want {
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
	view.hitAt = 1
	view.shows("src/talk.rs", 12)
	view.lines, view.total = filesLines([]byte(lineNumbered(1, 30, nil)), 4, filesHitLinesMost)
	view.status = &muiStatus{Text: "2 in 2 files"}
	got = filesSaid(t, view.messages(filesAllPut)[0])
	left = got.View.Children[0]
	if field := left.Children[0]; left.W != 440 || field.List != "hits" || field.Placeholder != "Find in files: /pattern, =whole words" || !field.Bare || field.Value == nil || *field.Value != "apply(" {
		t.Errorf("the left side = %s", encode(left))
	}
	if list := filesPieceOf(got.View, "hits"); list == nil || !list.Live || list.Selected != "h:1" || len(list.Items) != 4 || !reflect.DeepEqual(list.Items[2], filesEntry{Head: "src/talk.rs", Note: "1"}) {
		t.Errorf("the list = %s", encode(list))
	}
	if want := []string{"src/talk.rs", "changed against main · lines 4–30 of 30"}; !slices.Equal(filesTexts(*got.View), want) {
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
		{tree, `{"e":"move","list":"file","id":"2"}`, filesDeed{do: filesOnLine, line: 3}},
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
	if !reflect.DeepEqual(list.Items[0], filesEntry{Head: "app", Note: "4 changed against main"}) || list.Selected != "src/new/added.txt" {
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
	search, err := filesSearchFor(repo.Root, "needle", git.AsWritten, true, 40)
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
	if got := editorAt(repo.Root, "-n.txt", 4).Args; !slices.Equal(got, []string{editor, "--wait", "+4", "./-n.txt"}) {
		t.Errorf("args = %q", got)
	}
	t.Setenv("EDITOR", "")
	if got := editorAt(repo.Root, "a.txt", 1); filepath.Base(got.Path) != "nvim" || got.Dir != repo.Root {
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
	if list := filesPieceOf(got.View, "tree"); got.Status != nil || !reflect.DeepEqual(list.Items[0], filesEntry{Head: "sub", Note: "in no repository"}) || list.Selected != "deep.txt" {
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

// filesRead is a view of a throwaway repository, read once, that keeps what
// it writes to the terminal.
func filesRead(t *testing.T, repo *gittest.Repo) *filesView {
	t.Helper()
	view := &filesView{pane: "%4", dir: repo.Root, hitAt: -1}
	view.read()
	return view
}

// filesAsks is the modal a view says, by its id: its title, the texts in
// it and its buttons each as its label and its key; nil where it says
// none of that id.
func filesAsks(t *testing.T, view *filesView, id string) []string {
	t.Helper()
	got := filesSaid(t, view.messages(0)[0])
	modal := filesPieceOf(got.View, id)
	if modal == nil {
		return nil
	}
	if modal.T != "modal" {
		t.Fatalf("the modal = %+v", modal)
	}
	said := append([]string{modal.Title}, filesTexts(*modal)...)
	for _, child := range modal.Children {
		for _, button := range child.Children {
			said = append(said, button.ID+"="+button.Label+" "+button.Key)
			if button.T != "button" || button.Primary != (button.ID == "yes") {
				t.Fatalf("the button = %+v", button)
			}
		}
	}
	return said
}

// filesToasted is what a view just did, as its toast says it; none is no words.
func filesToasted(view *filesView) muiStatus {
	if view.passing.toast == nil {
		return muiStatus{}
	}
	return *view.passing.toast
}

// filesPress is a key of the program's pressed on an item, done.
func filesPress(view *filesView, key, item string) (bool, filesPlace) {
	return view.act(view.deed(gitEvent{E: "press", ID: key, Item: item}))
}

// A name typed is a path in the folder asked about, by its steps from
// the top, a folder's where it ends in a slash; nothing, a path from the
// root, one that leaves the worktree and one in git's own folder are
// refused.
func TestANameTypedIsAPathInTheFolderAskedAbout(t *testing.T) {
	for _, c := range []struct {
		folder, typed, want string
		isFolder            bool
	}{
		{"src", "x.rs", "src/x.rs", false}, {"", "x.rs", "x.rs", false}, {"src", " deep/er/x.rs ", "src/deep/er/x.rs", false},
		{"src", "made/", "src/made", true}, {"src/new", "../x.rs", "src/x.rs", false}, {"", "./a//b", "a/b", false},
	} {
		if got, isFolder, err := filesNamedIn(c.folder, c.typed); err != nil || got != c.want || isFolder != c.isFolder {
			t.Errorf("%q in %q is %q (a folder: %v), %v", c.typed, c.folder, got, isFolder, err)
		}
	}
	for _, c := range [][2]string{{"src", ""}, {"src", "  "}, {"src", "/etc/x"}, {"src", "../../x"}, {"", ".."}, {"", "."}, {"", ".git/config"}, {"src", "../.git"}} {
		if got, _, err := filesNamedIn(c[0], c[1]); err == nil {
			t.Errorf("%q in %q is %q, and is to be refused", c[1], c[0], got)
		}
	}
}

// a asks a name in a field in a modal over the view, of a file in the
// folder under the keys or in the folder of the file under them; ⏎ makes
// it, with the folders on the way, and the keys are on it in the tree: a
// file is handed to the editor at once, a name that ends in a slash is a
// folder listed while nothing is in it. What is there already is
// refused, said under the field, and esc asks no more.
func TestANewFileOrFolderIsMadeWhereTheKeysAre(t *testing.T) {
	repo := filesWork(t)
	view := filesRead(t, repo)
	if got, want := view.deed(gitEvent{E: "press", ID: "new", Item: "src/keep.txt"}), (filesDeed{do: filesAsked, ask: filesAskNew, path: "src", id: "tree"}); got != want {
		t.Fatalf("a on a file does %+v", got)
	}
	if got, want := view.deed(gitEvent{E: "press", ID: "new", Item: "0"}), (filesDeed{do: filesAsked, ask: filesAskNew, path: "src/new", id: "file"}); got != want {
		t.Errorf("a on a line of the file does %+v", got)
	}
	if got, want := view.deed(gitEvent{E: "press", ID: "new", Item: "top.txt"}), (filesDeed{do: filesAsked, ask: filesAskNew, id: "tree"}); got != want {
		t.Errorf("a on a file of the top does %+v", got)
	}
	if say, place := filesPress(view, "new", "src/keep.txt"); !say || place != filesWordsPut || view.asking != filesAskNew {
		t.Fatalf("a: said %v at %d", say, place)
	}
	got := filesSaid(t, view.messages(filesWordsPut)[0])
	modal := filesPieceOf(got.View, "ask")
	if modal == nil || modal.T != "modal" || modal.Title != "New file or folder" || len(modal.Children) != 1 || filesPieceOf(got.View, "tree").Items[0].Head != "app" {
		t.Fatalf("the modal = %s", encode(modal))
	}
	if field := modal.Children[0]; got.Focus != "name" || field.T != "field" || field.ID != "name" || field.Right != "in src/" || field.Value == nil || *field.Value != "" {
		t.Errorf("focus %q, the field = %s", got.Focus, encode(field))
	}
	if len(got.Keys) != 0 {
		t.Errorf("keys = %+v", got.Keys)
	}
	// while it asks, no other key does anything
	if deed := view.deed(gitEvent{E: "press", ID: "delete", Item: "src/keep.txt"}); deed != (filesDeed{}) {
		t.Errorf("D while a name is asked does %+v", deed)
	}

	if say, place := view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "deep/made.txt"})); !say || place != filesKeysPut || view.asking != filesAskNothing {
		t.Fatalf("⏎: said %v at %d, status %+v", say, place, view.status)
	}
	if info, err := os.Stat(filepath.Join(repo.Root, "src/deep/made.txt")); err != nil || info.Size() != 0 {
		t.Fatalf("the file made: %v", err)
	}
	if view.at != "src/deep/made.txt" || view.file != "src/deep/made.txt" || view.focus != "tree" || filesToasted(view) != (muiStatus{Text: "made src/deep/made.txt", Tone: "ok"}) || view.status != nil || filesAsks(t, view, "ask") != nil {
		t.Errorf("the keys on %q beside %q, focus %q, toast %+v", view.at, view.file, view.focus, filesToasted(view))
	}
	if view.handing == nil || *view.handing != (filesDeed{do: filesEdit, path: "src/deep/made.txt", line: 1}) {
		t.Errorf("handed to the editor: %+v", view.handing)
	}
	view.handing = nil

	// a folder: listed with nothing in it, the keys on it, no editor
	filesPress(view, "new", "src")
	view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "empty/"}))
	if info, err := os.Stat(filepath.Join(repo.Root, "src/empty")); err != nil || !info.IsDir() {
		t.Fatalf("the folder made: %v", err)
	}
	if row, ok := view.row("src/empty"); !ok || !row.folder || view.at != "src/empty" || view.handing != nil {
		t.Errorf("the folder's row %+v, the keys on %q, handed %+v", row, view.at, view.handing)
	}
	// once a file is in it, it is listed as any folder is
	repo.Write("src/empty/in.txt", "in\n")
	if view.read(); len(view.made) != 0 || !slices.Contains(view.paths, "src/empty/in.txt") {
		t.Errorf("made = %q", view.made)
	}

	// what is there, and what is outside, is refused, and the field stays
	filesPress(view, "new", "src")
	for typed, why := range map[string]string{"keep.txt": "src/keep.txt is there already", "../../out.txt": "that is outside the worktree", " ": "no name was typed"} {
		if view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: typed})); view.asking != filesAskNew || *view.status != (muiStatus{Text: why, Tone: "bad"}) {
			t.Errorf("%q: asking %d, status %+v", typed, view.asking, view.status)
		}
		if said := filesAsks(t, view, "ask"); !slices.Equal(said, []string{"New file or folder", why}) {
			t.Errorf("%q: the modal says %q", typed, said)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(repo.Root), "out.txt")); err == nil {
		t.Error("a file was made outside the worktree")
	}
	if say, _ := view.act(view.deed(gitEvent{E: "dismiss", ID: "ask"})); !say || view.asking != filesAskNothing || view.focus != "tree" || filesAsks(t, view, "ask") != nil {
		t.Errorf("esc: said %v, asking %d, focus %q", say, view.asking, view.focus)
	}
	// one git ignores is made, and said not to be listed
	filesPress(view, "new", "top.txt")
	if view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "new.log"})); view.status == nil || view.status.Tone != "warn" || view.handing == nil {
		t.Errorf("an ignored file: status %+v, handed %+v", view.status, view.handing)
	}
}

// R asks where the path under the keys goes, in a field that holds it; ⏎
// renames it, with git mv where git tracks it and on disk alone where it
// does not, the folders on the way made; a path that is there already is
// refused.
func TestAPathIsRenamedWithGitWhereGitTracksIt(t *testing.T) {
	repo := filesWork(t)
	view := filesRead(t, repo)
	if say, place := filesPress(view, "move", "src/same.txt"); !say || place != filesWordsPut || view.asking != filesAskMove || view.answer != "src/same.txt" {
		t.Fatalf("R: said %v at %d, the field holds %q", say, place, view.answer)
	}
	got := filesSaid(t, view.messages(filesWordsPut)[0])
	if field, modal := filesPieceOf(got.View, "name"), filesPieceOf(got.View, "ask"); modal == nil || modal.Title != "Rename or move src/same.txt" || field == nil || field.Value == nil || *field.Value != "src/same.txt" {
		t.Errorf("the modal = %s", encode(modal))
	}
	if view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "src/keep.txt"})); view.asking != filesAskMove || *view.status != (muiStatus{Text: "src/keep.txt is there already", Tone: "bad"}) {
		t.Errorf("over a file that is there: status %+v", view.status)
	}
	if view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "src/same.txt"})); *view.status != (muiStatus{Text: "that is its name already", Tone: "bad"}) {
		t.Errorf("to its own name: status %+v", view.status)
	}
	view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "lib/kept.txt"}))
	if view.asking != filesAskNothing || view.at != "lib/kept.txt" || view.file != "lib/kept.txt" || filesToasted(view) != (muiStatus{Text: "now lib/kept.txt", Tone: "ok"}) || view.status != nil || view.handing != nil {
		t.Fatalf("renamed: the keys on %q, toast %+v", view.at, filesToasted(view))
	}
	if got := repo.Git("status", "--porcelain", "--", "lib", "src/same.txt"); got != "R  src/same.txt -> lib/kept.txt" {
		t.Errorf("git status says %q", got)
	}

	// one git does not track is renamed on disk alone
	filesPress(view, "move", "top.txt")
	view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "over/top.txt"}))
	if _, err := os.Stat(filepath.Join(repo.Root, "over/top.txt")); err != nil || view.at != "over/top.txt" {
		t.Errorf("an untracked file renamed: %v, the keys on %q", err, view.at)
	}
	if got := repo.Git("status", "--porcelain", "--", "over"); got != "?? over/" {
		t.Errorf("git status says %q", got)
	}
	// a folder goes with what is in it, and stays open
	filesPress(view, "move", "src/new")
	view.act(view.deed(gitEvent{E: "submit", ID: "name", Text: "src/newer"}))
	if row, ok := view.row("src/newer"); !ok || !row.folder || !row.open || !slices.Contains(view.paths, "src/newer/added.txt") {
		t.Errorf("a folder renamed: %+v, paths %q, status %+v", row, view.paths, view.status)
	}
}

// What the modal says before a path is deleted is what goes, as its
// title, and under it whether git can bring it back.
func TestWhatADeleteSaysBeforeItIsDone(t *testing.T) {
	for _, c := range []struct {
		named  string
		gone   filesGone
		inside bool
		ask    string
		told   string
	}{
		{"src/x.rs", filesGone{lines: 120}, true, "Delete src/x.rs, 120 lines?", "It is not in git: nothing can bring it back."},
		{"src/x.rs", filesGone{lines: 1, tracked: 1}, true, "Delete src/x.rs, 1 line?", "Tracked: git can bring it back."},
		{"src/x.rs", filesGone{lines: 9, tracked: 1, unsaved: true}, true, "Delete src/x.rs, 9 lines?", "Tracked: git can bring back only what is committed."},
		{"blob.bin", filesGone{}, true, "Delete blob.bin?", "It is not in git: nothing can bring it back."},
		{"src", filesGone{folder: true, files: 5, tracked: 3}, true, "Delete src and its 5 files?", "2 of them not in git, which nothing can bring back."},
		{"src", filesGone{folder: true, files: 1, tracked: 1}, true, "Delete src and its 1 file?", "Tracked: git can bring it back."},
		{"notes.md", filesGone{lines: 2}, false, "Delete notes.md, 2 lines?", "It is in no repository: nothing can bring it back."},
	} {
		if ask, told := filesGoing(c.named, c.gone, c.inside); ask != c.ask || told != c.told {
			t.Errorf("says %q, %q, want %q, %q", ask, told, c.ask, c.told)
		}
	}
	if ask, _ := filesGoing(strings.Repeat("é", 300), filesGone{lines: 1}, true); len([]rune(ask)) > gitWordsMost || !strings.HasSuffix(ask, ", 1 line?") {
		t.Errorf("of a long path: %q", ask)
	}
}

// D asks in a modal: pressed once it says what goes and deletes nothing;
// the modal's button, which D again presses, deletes the path — a file,
// or a folder with all that is in it — and esc and its other button keep
// it. What goes is counted again as it is confirmed, and where it is
// another the modal says it anew and nothing is deleted.
func TestDeleteAsksInAModal(t *testing.T) {
	repo := filesWork(t)
	view := filesRead(t, repo)
	there := func(path string) bool {
		_, err := os.Stat(filepath.Join(repo.Root, path))
		return err == nil
	}
	if say, _ := filesPress(view, "delete", "top.txt"); !say || !there("top.txt") || view.arming != "top.txt" || view.status != nil {
		t.Fatalf("D once: said %v, status %+v", say, view.status)
	}
	if want := []string{"Delete top.txt, 1 line?", "It is not in git: nothing can bring it back.", "yes=Delete D", "no=Keep it esc"}; !slices.Equal(filesAsks(t, view, "sure"), want) {
		t.Fatalf("the modal says %q", filesAsks(t, view, "sure"))
	}
	if got := filesSaid(t, view.messages(0)[0]); got.Focus != "yes" || len(got.Keys) != 0 {
		t.Errorf("focus %q, keys %+v", got.Focus, got.Keys)
	}
	// under the modal only its buttons and esc do anything, and the keys
	// passed between its buttons leave it up
	for _, event := range []gitEvent{{E: "press", ID: "copy", Item: "top.txt"}, {E: "move", List: "tree", ID: "src"}, {E: "focus", ID: "no"}} {
		if deed := view.deed(event); deed != (filesDeed{}) {
			t.Errorf("%+v under the modal does %+v", event, deed)
		}
	}
	// esc and its other button keep the file, and the keys go back to the tree
	for _, never := range []gitEvent{{E: "dismiss", ID: "sure"}, {E: "press", ID: "no"}} {
		filesPress(view, "delete", "top.txt")
		if say, _ := view.act(view.deed(never)); !say || view.arming != "" || !there("top.txt") || filesAsks(t, view, "sure") != nil || view.focus != "tree" {
			t.Errorf("%+v: said %v, arming %q, focus %q", never, say, view.arming, view.focus)
		}
	}
	// the file grew while the modal was up: said anew, and nothing deleted
	filesPress(view, "delete", "top.txt")
	repo.Write("top.txt", "one\ntwo\n")
	if say, _ := filesPress(view, "yes", ""); !say || !there("top.txt") || filesAsks(t, view, "sure")[0] != "Delete top.txt, 2 lines?" {
		t.Fatalf("its button after the file grew: %q", filesAsks(t, view, "sure"))
	}
	if say, _ := filesPress(view, "yes", ""); !say || there("top.txt") || view.arming != "" || filesToasted(view) != (muiStatus{Text: "deleted top.txt", Tone: "ok"}) || view.status != nil || slices.Contains(view.paths, "top.txt") || view.focus != "tree" {
		t.Errorf("its button: said %v, toast %+v, paths %q", say, filesToasted(view), view.paths)
	}
	// a choice of a row's menu asks as D does
	if view.act(view.deed(gitEvent{E: "pick", List: "tree", ID: "README.md", Sub: "delete"})); view.arming != "README.md" || !there("README.md") {
		t.Errorf("Delete… of a row's menu: arming %q", view.arming)
	}
	if want := []string{"Delete README.md, 1 line?", "Tracked: git can bring it back.", "yes=Delete D", "no=Keep it esc"}; !slices.Equal(filesAsks(t, view, "sure"), want) {
		t.Errorf("of a tracked file: %q", filesAsks(t, view, "sure"))
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}))

	// a folder says how many files go, those git does not track among them
	repo.Write("src/new/loose.txt", "loose\n")
	view.read()
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src"}))
	if filesPress(view, "delete", "src/new"); !slices.Equal(filesAsks(t, view, "sure")[:2], []string{"Delete src/new and its 2 files?", "1 of them not in git, which nothing can bring back."}) || !there("src/new/added.txt") {
		t.Errorf("of a folder: %q", filesAsks(t, view, "sure"))
	}
	if filesPress(view, "yes", ""); there("src/new") || slices.Contains(view.paths, "src/new/added.txt") {
		t.Errorf("a folder deleted: there %v, paths %q", there("src/new"), view.paths)
	}
	if view.at != "src" || !slices.Equal(view.lines, []filesLine{{Kind: "gap", Text: "no longer there"}}) {
		t.Errorf("the keys on %q beside %+v", view.at, view.lines)
	}
	// one changed since its commit says git brings back only what is committed
	if filesPress(view, "delete", "src/keep.txt"); view.told != "Tracked: git can bring it back." {
		t.Errorf("of a committed file: %q", view.told)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}))
	repo.Write("src/keep.txt", "other\n")
	filesPress(view, "again", "")
	if filesPress(view, "delete", "src/keep.txt"); view.told != "Tracked: git can bring back only what is committed." {
		t.Errorf("of a file changed since: %q", view.told)
	}
	view.act(view.deed(gitEvent{E: "dismiss", ID: "sure"}))
	// what is not there is not deleted, and no modal asks
	if filesPress(view, "delete", "src/old.txt"); filesToasted(view) != (muiStatus{Text: "src/old.txt is not there to delete", Tone: "bad"}) || view.arming != "" {
		t.Errorf("of a deleted file: %+v", filesToasted(view))
	}
}

// y puts the path under the keys on the clipboard, by the window's own
// copy, and says so in a toast.
func TestThePathIsCopiedByTheWindow(t *testing.T) {
	repo := filesWork(t)
	view := filesRead(t, repo)
	if say, place := filesPress(view, "copy", "src/keep.txt"); !say || place != 0 || view.passing.copy != "src/keep.txt" || filesToasted(view) != (muiStatus{Text: "copied src/keep.txt"}) || view.status != nil {
		t.Errorf("y: said %v at %d, copied %q, toast %+v", say, place, view.passing.copy, filesToasted(view))
	}
	var told muiTold
	told.says(view.pane, view.messages(0), nil, &muiPassing{})
	said := told.says(view.pane, view.messages(0), nil, &view.passing)
	if want := `{"v":1,"pane":"%4","patch":[{"op":"copy","text":"src/keep.txt"},{"op":"toast","text":"copied src/keep.txt"}]}`; len(said) != 1 || string(said[0]) != want {
		t.Errorf("said %s", said)
	}
	if again := told.says(view.pane, view.messages(0), nil, &view.passing); again != nil {
		t.Errorf("said twice: %s", again)
	}
	// on a line of the code it is the file shown, and on a folder the folder
	for item, want := range map[string]string{"0": "src/new/added.txt", "src": "src"} {
		if filesPress(view, "copy", item); view.passing.copy != want {
			t.Errorf("y on %s copied %q", item, view.passing.copy)
		}
	}
}

// Each row of the tree carries the menu of what can be done to it, each
// choice by the id of the key that does the same with that key at its
// right, and picked it does what that key does on the row: a file is
// handed to the editor, shown as its diff and blamed, and a file and a
// folder have a file made by them, are renamed, deleted and their path
// copied. The keys of the foot are those of the piece the window says
// the keys are on, and ⏎ on a blamed line is said to show its commit.
func TestEachRowOfTheTreeCarriesItsMenu(t *testing.T) {
	t.Setenv("EDITOR", "ed")
	view := filesDeskView()
	menus := map[string]string{}
	entries, _ := view.items()
	for _, entry := range entries {
		var choices []string
		for _, choice := range entry.Menu {
			choices = append(choices, choice.ID+"="+choice.Title+" "+choice.Right)
		}
		menus[entry.ID] = strings.Join(choices, ", ")
	}
	const folder = "new=New file here… a, move=Rename… R, delete=Delete… D, copy=Copy path y"
	if menus[""] != "" || menus["src"] != folder || menus["src/mui.rs"] != "edit=Open in editor e, "+folder+", diff=Diff against the base d, blame=Blame B" {
		t.Errorf("the menus = %q", menus)
	}
	if list := filesPieceOf(filesSaid(t, view.messages(0)[0]).View, "tree"); list.Menu == "" {
		t.Errorf("the list names no menu: %+v", list)
	}
	for sub, want := range map[string]filesDeed{
		"edit":   {do: filesEdit, path: "src/mui.rs", line: 1},
		"new":    {do: filesAsked, ask: filesAskNew, path: "src", id: "tree"},
		"move":   {do: filesAsked, ask: filesAskMove, path: "src/mui.rs", id: "tree"},
		"delete": {do: filesDelete, path: "src/mui.rs", id: "tree"},
		"copy":   {do: filesCopy, path: "src/mui.rs"},
		"diff":   {do: filesDiff},
		"blame":  {do: filesBlamed},
	} {
		if got := view.deed(gitEvent{E: "pick", List: "tree", ID: "src/mui.rs", Sub: sub}); got != want {
			t.Errorf("%s of the menu does %+v", sub, got)
		}
	}
	if got := view.deed(gitEvent{E: "pick", List: "tree", ID: "src"}); got.do != filesToggle {
		t.Errorf("⏎ on a folder does %+v", got)
	}

	label := func() string {
		var all []string
		for _, key := range view.keys() {
			all = append(all, key.Key)
		}
		return strings.Join(all, " ")
	}
	if say, _ := view.act(view.deed(gitEvent{E: "focus", ID: "file"})); !say || !view.inFile || label() != "e : d B y f / R D r" {
		t.Errorf("on the file the keys are %q", label())
	}
	if say, _ := view.act(view.deed(gitEvent{E: "focus", ID: "tree"})); !say || view.inFile || label() != "e f / a R D y c d B : r" {
		t.Errorf("on the tree the keys are %q", label())
	}
	if deed := view.deed(gitEvent{E: "focus", ID: "name"}); deed != (filesDeed{}) {
		t.Errorf("the keys in a modal's field do %+v", deed)
	}
	code := func() string { return filesPieceOf(filesSaid(t, view.messages(0)[0]).View, "file").Does }
	if code() != "" {
		t.Errorf("⏎ on a line does %q", code())
	}
	if view.blaming = true; code() != "show its commit" {
		t.Errorf("⏎ on a blamed line does %q", code())
	}
}

// B puts at the right of each line its commit, by a short name, and how
// long ago that was, as git blame has them, and B again takes them off;
// ⏎ on a line while they are there shows that commit in place of the
// file — its name, its date and its subject over what it changed of the
// file — and esc is back in the file on that line.
func TestBlameSaysEachLinesCommitAndShowsIt(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	if got := filesBlamedWord(git.Blamed{Commit: "1a2b3c4d5e6f", When: now.Add(-3 * 24 * time.Hour)}, now); got != "1a2b3c4 3d" {
		t.Errorf("a blamed line says %q", got)
	}
	if got := filesBlamedWord(git.Blamed{}, now); got != "not committed" {
		t.Errorf("a line not committed says %q", got)
	}

	repo := filesWork(t)
	base, work := repo.Git("rev-parse", "--short=7", "main"), repo.Git("rev-parse", "--short=7", "work")
	repo.Write("src/keep.txt", "one\n\ttwo, Changed\nthree\nfour\n")
	view := filesRead(t, repo)
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/keep.txt"}))
	if deed := view.deed(gitEvent{E: "pick", List: "file", ID: "1"}); deed != (filesDeed{}) {
		t.Errorf("⏎ on a line with no blame does %+v", deed)
	}
	if say, _ := filesPress(view, "blame", "src/keep.txt"); !say || !view.blaming || view.status != nil {
		t.Fatalf("B: said %v, status %+v", say, view.status)
	}
	var right []string
	for _, line := range view.lines {
		right = append(right, line.Right)
	}
	if want := []string{base + " now", work + " now", work + " now", "not committed"}; !slices.Equal(right, want) {
		t.Fatalf("the lines say %q, want %q", right, want)
	}
	if got := string(encode(view.lines[0])); got != `{"new":1,"text":"one","right":"`+base+` now"}` {
		t.Errorf("a blamed line is said as %s", got)
	}
	if label := filesSaid(t, view.messages(0)[0]).Keys[9]; label != (gitKey{"B", "no blame", "blame"}) {
		t.Errorf("the key says %+v", label)
	}
	// git is asked once for the file as it stands
	view.blames["src/keep.txt"] = filesBlame{stamp: view.blames["src/keep.txt"].stamp, lines: []git.Blamed{{Commit: "feedfacefeed", When: time.Now()}}}
	if view.load(); view.lines[0].Right != "feedfac now" || view.lines[1].Right != "" {
		t.Errorf("read again: %+v", view.lines)
	}
	delete(view.blames, "src/keep.txt")
	view.load()

	if say, _ := view.act(view.deed(gitEvent{E: "pick", List: "file", ID: "3"})); !say || view.commit != nil || *view.status != (muiStatus{Text: "line 4 is not committed yet"}) {
		t.Errorf("⏎ on a line not committed: said %v, status %+v", say, view.status)
	}
	if say, place := view.act(view.deed(gitEvent{E: "pick", List: "file", ID: "1"})); !say || place != filesLinePut || view.commit == nil {
		t.Fatalf("⏎ on a blamed line: said %v at %d, status %+v", say, place, view.status)
	}
	got := filesSaid(t, view.messages(filesLinePut)[0])
	if want := []string{"src/keep.txt", work + " · " + time.Now().Format("2006-01-02") + " · work's"}; !slices.Equal(filesTexts(*got.View), want) {
		t.Errorf("texts = %q, want %q", filesTexts(*got.View), want)
	}
	want := []filesLine{{Text: "@@ line 1", Kind: "head", Right: "hunk 1 of 1"}, {Old: 1, New: 1, Text: "one"}, {Old: 2, Text: "two", Kind: "del"}, {New: 2, Text: "    two, Changed", Kind: "add"}, {New: 3, Text: "three", Kind: "add"}}
	if !slices.Equal(view.lines, want) {
		t.Errorf("the commit's lines = %+v", view.lines)
	}
	if want := []gitKey{{"e", "edit in nvim", "edit"}, {"esc", "back to the file", "back"}}; !slices.Equal(got.Keys, want) || got.Focus != "file" || filesPieceOf(got.View, "file").Selected != "3" {
		t.Errorf("keys = %+v, focus %q, the code's keys on %q", got.Keys, got.Focus, filesPieceOf(got.View, "file").Selected)
	}
	if say, place := filesPress(view, "back", ""); !say || place != filesLinePut || view.commit != nil || view.on != 2 || len(view.lines) != 4 || view.lines[1].Right != work+" now" {
		t.Errorf("esc: said %v at %d, the keys on line %d, lines %+v", say, place, view.on, view.lines)
	}
	if filesPress(view, "blame", ""); view.blaming || view.lines[0].Right != "" {
		t.Errorf("B again: %+v", view.lines)
	}
	// a file git does not track has no line to blame, and says so
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "top.txt"}))
	if filesPress(view, "blame", "top.txt"); *view.status != (muiStatus{Text: "git blames no line of top.txt", Tone: "warn"}) {
		t.Errorf("B on an untracked file: %+v", view.status)
	}
}

// c lists in the tree only the files changed against the base, with the
// folders on the way to them, and the head says so; c again lists all.
func TestTheTreeListsOnlyWhatChanged(t *testing.T) {
	repo := filesWork(t)
	view := filesRead(t, repo)
	view.act(view.deed(gitEvent{E: "pick", List: "tree", ID: "src/new"})) // shut, and opened again by c
	if say, _ := filesPress(view, "only", ""); !say || !view.only {
		t.Fatalf("c: said %v, status %+v", say, view.status)
	}
	if want := []string{"src", "src/new", "src/new/added.txt", "src/keep.txt", "src/old.txt", "top.txt"}; !slices.Equal(filesIDs(view.rows), want) {
		t.Errorf("rows = %q", filesIDs(view.rows))
	}
	got := filesSaid(t, view.messages(filesAllPut)[0])
	if head := filesPieceOf(got.View, "tree").Items[0]; !reflect.DeepEqual(head, filesEntry{Head: "app", Note: "only the 4 changed against main"}) || got.Keys[7] != (gitKey{"c", "all files", "only"}) {
		t.Errorf("the head = %+v, the key %+v", head, got.Keys[7])
	}
	// the keys on a file that is not changed go to one that is
	filesPress(view, "only", "")
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "README.md"}))
	if _, place := filesPress(view, "only", ""); place != filesKeysPut || view.marks[view.file] == 0 || view.at != cutTo(view.file, gitWordsMost) {
		t.Errorf("c on an unchanged file: at %d, the keys on %q beside %q", place, view.at, view.file)
	}
	if filesPress(view, "only", ""); view.only || !slices.Contains(filesIDs(view.rows), "README.md") || filesSaid(t, view.messages(0)[0]).Keys[7].Label != "changed only" {
		t.Errorf("c again: rows %q", filesIDs(view.rows))
	}
	// with nothing changed there is nothing to list alone
	other := filesRead(t, gittest.New(t))
	if filesPress(other, "only", ""); other.only || *other.status != (muiStatus{Text: "nothing changed against the last commit"}) {
		t.Errorf("with nothing changed: only %v, status %+v", other.only, other.status)
	}
}

// d shows a changed file as its diff against the base in place of its
// lines — the lines added and deleted under their hunk's head, a file git
// does not track all added — and d again shows the file; a file that is
// as the base has it has no diff.
func TestAChangedFileIsShownAsItsDiffAgainstTheBase(t *testing.T) {
	code := []gitLine{{Kind: "head", Text: "@@ fn hear", Right: "hunk 1 of 1"}, {Kind: "same", Text: "a", Old: 1, New: 1}, {Kind: "del", Text: "b", Old: 2}, {Kind: "add", Text: "c", New: 2}, {Kind: "add", Text: "d", New: 3}}
	lines, added, deleted := filesDiffed(code, 3)
	if want := []filesLine{{Text: "@@ fn hear", Kind: "head", Right: "hunk 1 of 1"}, {Old: 1, New: 1, Text: "a"}, {Old: 2, Text: "b", Kind: "del"}, {Kind: "gap", Text: "2 more lines"}}; !slices.Equal(lines, want) || added != 2 || deleted != 1 {
		t.Errorf("+%d -%d: %+v", added, deleted, lines)
	}
	if got := string(encode(lines[2])); got != `{"old":2,"text":"b","kind":"del"}` {
		t.Errorf("a deleted line is said as %s", got)
	}

	repo := filesWork(t)
	view := filesRead(t, repo)
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/keep.txt"}))
	if say, _ := filesPress(view, "diff", "src/keep.txt"); !say || !view.diffing || !view.diffed {
		t.Fatalf("d: said %v, status %+v", say, view.status)
	}
	want := []filesLine{{Text: "@@ line 1", Kind: "head", Right: "hunk 1 of 1"}, {Old: 1, New: 1, Text: "one"}, {Old: 2, Text: "two", Kind: "del"}, {New: 2, Text: "    two, Changed", Kind: "add"}, {New: 3, Text: "three", Kind: "add"}}
	if !slices.Equal(view.lines, want) {
		t.Errorf("the diff's lines = %+v", view.lines)
	}
	got := filesSaid(t, view.messages(0)[0])
	if want := []string{"src/keep.txt", "diff against main · +2 -1"}; !slices.Equal(filesTexts(*got.View), want) || got.Keys[8] != (gitKey{"d", "the whole file", "diff"}) {
		t.Errorf("texts = %q, the key %+v", filesTexts(*got.View), got.Keys[8])
	}
	// e on a line of the diff edits that line of the file
	if deed := view.deed(gitEvent{E: "press", ID: "edit", Item: "4"}); deed != (filesDeed{do: filesEdit, path: "src/keep.txt", line: 3}) {
		t.Errorf("e on a line added does %+v", deed)
	}
	// it stays as the keys go to other files: one untracked is all added,
	// one deleted all deleted, one unchanged its lines
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "top.txt"}))
	if want := []filesLine{{Text: "@@ new file", Kind: "head", Right: "hunk 1 of 1"}, {New: 1, Text: "untracked, and Changed", Kind: "add"}}; !slices.Equal(view.lines, want) {
		t.Errorf("an untracked file's diff = %+v", view.lines)
	}
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/old.txt"}))
	if want := []filesLine{{Text: "@@ deleted file", Kind: "head", Right: "hunk 1 of 1"}, {Old: 1, Text: "old", Kind: "del"}}; !slices.Equal(view.lines, want) {
		t.Errorf("a deleted file's diff = %+v", view.lines)
	}
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "src/same.txt"}))
	if want := []filesLine{{New: 1, Text: "same"}}; !slices.Equal(view.lines, want) || view.diffed {
		t.Errorf("an unchanged file's lines = %+v", view.lines)
	}
	if filesPress(view, "diff", ""); view.diffing {
		t.Error("d again left the diff on")
	}
	if filesPress(view, "diff", ""); view.diffing || *view.status != (muiStatus{Text: "src/same.txt is as main has it"}) {
		t.Errorf("d on an unchanged file: %+v", view.status)
	}
}

// The lines said of a long file are most of them with the keys' line in
// the middle, and others are said when the keys come within the edge of
// the last of them while the file goes on, or of the first while it
// begins before.
func TestTheLinesSaidOfALongFileFollowTheKeys(t *testing.T) {
	for _, c := range []struct{ line, total, most, want int }{{1, 30, 1500, 1}, {30, 30, 1500, 1}, {1, 9000, 1500, 1}, {700, 9000, 1500, 1}, {751, 9000, 1500, 1}, {2000, 9000, 1500, 1250}, {9000, 9000, 1500, 7501}, {8900, 9000, 1500, 7501}} {
		if got := filesWindow(c.line, c.total, c.most); got != c.want {
			t.Errorf("the keys on %d of %d: from %d, want %d", c.line, c.total, got, c.want)
		}
	}
	for _, c := range []struct {
		line, from int
		want       int
		turned     bool
	}{
		{1, 1, 1, false}, {1400, 1, 1, false}, {1401, 1, 651, true}, {1500, 1, 750, true}, // near the last line said
		{1300, 651, 651, false}, {750, 651, 1, true}, {651, 651, 1, true}, // near the first
		{8990, 7501, 7501, false}, {7600, 7501, 6850, true}, // the file ends where the lines said do
		{60, 1, 1, false}, // and begins where they do
	} {
		if got, turned := filesTurned(c.line, c.from, 9000, 1500, 100); got != c.want || turned != c.turned {
			t.Errorf("the keys on %d of the lines from %d: from %d (turned %v), want %d (%v)", c.line, c.from, got, turned, c.want, c.turned)
		}
	}
	if got, turned := filesTurned(20, 1, 30, 1500, 100); got != 1 || turned {
		t.Errorf("a file said whole: from %d, turned %v", got, turned)
	}
	for content, want := range map[string]int{"": 0, "one": 1, "one\n": 1, "one\ntwo": 2, "one\n\n": 2, "\x00\n\n": 0} {
		if got := filesCount([]byte(content)); got != want {
			t.Errorf("%q has %d lines, want %d", content, got, want)
		}
	}
}

// A file of more lines than a view holds is read to its end: its first
// lines are said, the line over them saying which of how many, and as
// the keys come near the last of them the next are said with the keys on
// the same line, and near the first the ones before; : asks a line in a
// field in place of the file's path and says the file from there.
func TestAnyFileIsReadToItsEnd(t *testing.T) {
	repo := filesWork(t)
	repo.Write("long.txt", lineNumbered(1, 5000, nil))
	view := filesRead(t, repo)
	view.act(view.deed(gitEvent{E: "move", List: "tree", ID: "long.txt"}))
	texts := func() []string { return filesTexts(*filesSaid(t, view.messages(0)[0]).View) }
	if want := []string{"long.txt", "new against main · lines 1–1500 of 5000"}; !slices.Equal(texts(), want) || len(view.lines) != filesLinesMost+1 {
		t.Fatalf("texts = %q, %d lines", texts(), len(view.lines))
	}
	if last := view.lines[len(view.lines)-1]; last != (filesLine{Kind: "gap", Text: "3500 more lines"}) {
		t.Errorf("the last line = %+v", last)
	}
	// the keys go down a line at a time: nothing is said again until they
	// are near the last line said, and then with the keys on their line
	said, seen := 0, 0
	for view.on < 5000 {
		at := slices.IndexFunc(view.lines, func(line filesLine) bool { return line.New == max(view.on, 1) })
		if at < 0 || at+1 >= len(view.lines) {
			t.Fatalf("the keys on line %d, which is not said with a line after it", view.on)
		}
		say, place := view.act(view.deed(gitEvent{E: "move", List: "file", ID: strconv.Itoa(at + 1)}))
		if seen++; say {
			said++
			code := filesPieceOf(filesSaid(t, view.messages(place)[0]).View, "file")
			if selected, _ := strconv.Atoi(code.Selected); place != filesLinePut || view.lines[selected].New != view.on {
				t.Fatalf("said again with the code's keys on %q, and they are on line %d", code.Selected, view.on)
			}
			if n := filesCounted(t, *filesSaid(t, view.messages(place)[0]).View); n > muiPieces {
				t.Fatalf("%d pieces, over the %d a UI may have", n, muiPieces)
			}
		}
	}
	if seen != 4999 || said != 6 || view.lines[len(view.lines)-1] != (filesLine{New: 5000, Text: "line 5000"}) {
		t.Errorf("%d moves said the view %d times, and the last line is %+v", seen, said, view.lines[len(view.lines)-1])
	}
	if want := []string{"long.txt", "new against main · lines 3501–5000 of 5000"}; !slices.Equal(texts(), want) || view.lines[0] != (filesLine{Kind: "gap", Text: "3500 lines above"}) {
		t.Errorf("texts = %q, the first line %+v", texts(), view.lines[0])
	}
	// and up again
	if say, _ := view.act(view.deed(gitEvent{E: "move", List: "file", ID: "50"})); !say || view.on != 3550 || view.window != 2800 {
		t.Errorf("near the first line said: said %v, the keys on %d of the lines from %d", say, view.on, view.window)
	}

	// : asks a line, and the file is said from around it
	if say, place := filesPress(view, "line", "3"); !say || place != filesWordsPut || view.asking != filesAskLine {
		t.Fatalf(": said %v at %d", say, place)
	}
	got := filesSaid(t, view.messages(filesWordsPut)[0])
	if field, modal := filesPieceOf(got.View, "line"), filesPieceOf(got.View, "ask"); got.Focus != "line" || modal == nil || modal.Title != "Go to line" || filesPieceOf(modal, "line") == nil || field.T != "field" || field.Placeholder != "Go to line, 1–5000" {
		t.Errorf("focus %q, the modal = %s", got.Focus, encode(modal))
	}
	if view.act(view.deed(gitEvent{E: "submit", ID: "line", Text: "twelve"})); view.asking != filesAskLine || *view.status != (muiStatus{Text: "a line's number, 1 to 5000", Tone: "bad"}) || !slices.Equal(filesAsks(t, view, "ask"), []string{"Go to line", "a line's number, 1 to 5000"}) {
		t.Errorf("no number: asking %d, status %+v", view.asking, view.status)
	}
	if say, place := view.act(view.deed(gitEvent{E: "submit", ID: "line", Text: " 1234 "})); !say || place != filesLinePut || view.asking != filesAskNothing || view.on != 1234 || view.window != 484 || view.focus != "file" {
		t.Errorf("to line 1234: said %v at %d, the keys on %d of the lines from %d", say, place, view.on, view.window)
	}
	code := filesPieceOf(filesSaid(t, view.messages(filesLinePut)[0]).View, "file")
	if selected, _ := strconv.Atoi(code.Selected); view.lines[selected].New != 1234 {
		t.Errorf("the code's keys on %q", code.Selected)
	}
	// past the end is the last line, and esc asks no more
	filesPress(view, "line", "3")
	if view.act(view.deed(gitEvent{E: "submit", ID: "line", Text: "99999"})); view.on != 5000 || view.window != 3501 {
		t.Errorf("past the end: the keys on %d of the lines from %d", view.on, view.window)
	}
	filesPress(view, "line", "3")
	if filesPress(view, "back", ""); view.asking != filesAskNothing || view.focus != "file" || view.on != 5000 {
		t.Errorf("esc: asking %d, focus %q", view.asking, view.focus)
	}
	// e edits the line the keys are on, wherever the lines said begin
	if deed := view.deed(gitEvent{E: "press", ID: "edit", Item: "1"}); deed != (filesDeed{do: filesEdit, path: "long.txt", line: 3501}) {
		t.Errorf("e does %+v", deed)
	}
}

// What is searched for is the words typed as they are written; after a
// slash a regular expression, after an equals sign whole words alone,
// and the status says which. A pattern git cannot read is the status.
func TestFindingInFilesByPatternAndByWholeWords(t *testing.T) {
	for typed, want := range map[string]struct {
		words string
		how   git.Match
		said  string
	}{"apply(": {"apply(", git.AsWritten, ""}, "/fn \\w+\\(": {"fn \\w+\\(", git.AsPattern, "as a pattern"}, "=apply": {"apply", git.WholeWords, "whole words"}, "": {"", git.AsWritten, ""}, "a/b=c": {"a/b=c", git.AsWritten, ""}} {
		if words, how, said := filesSeeking(typed); words != want.words || how != want.how || said != want.said {
			t.Errorf("%q is %q read as %d, said %q", typed, words, how, said)
		}
	}

	repo := filesWork(t)
	repo.Write("src/words.txt", "a change\nchanged\nt.o\n")
	view := filesRead(t, repo)
	filesPress(view, "grep", "")
	sought := func(typed string) []filesHit {
		t.Helper()
		view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: typed}))
		filesSought(t, view)
		return view.hits
	}
	if got, want := sought("/t.o|^thr"), []filesHit{{"src/keep.txt", 2, "two, Changed"}, {"src/keep.txt", 3, "three"}, {"src/words.txt", 3, "t.o"}}; !slices.Equal(got, want) || *view.status != (muiStatus{Text: "3 in 2 files · as a pattern"}) {
		t.Errorf("a pattern found %+v, status %+v", got, view.status)
	}
	if got, want := sought("=change"), []filesHit{{"src/words.txt", 1, "a change"}}; !slices.Equal(got, want) || *view.status != (muiStatus{Text: "1 in 1 file · whole words"}) {
		t.Errorf("whole words found %+v, status %+v", got, view.status)
	}
	if got := sought("change"); len(got) != 4 || *view.status != (muiStatus{Text: "4 in 3 files"}) {
		t.Errorf("the words as written found %+v, status %+v", got, view.status)
	}
	if say, _ := view.act(view.deed(gitEvent{E: "submit", ID: "q", Text: "/t.o"})); !say || *view.status != (muiStatus{Text: "searching as a pattern…"}) {
		t.Errorf("searching: said %v, status %+v", say, view.status)
	}
	filesSought(t, view)
	if got := sought("/(two"); len(got) != 0 || view.status == nil || view.status.Tone != "bad" {
		t.Errorf("a pattern git cannot read found %+v, status %+v", got, view.status)
	}
	if got := sought("/"); len(got) != 0 || view.search != nil || view.status != nil {
		t.Errorf("a pattern of nothing found %+v, status %+v", got, view.status)
	}
}
