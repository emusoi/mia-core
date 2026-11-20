package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
)

const gitUsage = "mia git"

// What a view of a worktree's standing holds at most, all of it under the
// pieces the window takes of a UI (muiPieces): its files, its commits
// ahead, and the lines of a diff; the characters the window keeps of a
// line of code and of an id, a title or a status; the bytes read of an
// untracked file to show it; and how often the repository is looked at.
const (
	gitRowsMost    = 400
	gitCommitsMost = 20
	gitLinesMost   = 1500
	gitLineMost    = 400
	gitWordsMost   = 200
	gitFileMost    = 8 << 20
	gitPoll        = 2 * time.Second
)

// gitMessage is the whole view said to the window: a UI that fills its
// pane, and the keys the program takes.
type gitMessage struct {
	V      int        `json:"v"`
	Pane   string     `json:"pane,omitempty"`
	Fill   bool       `json:"fill"`
	Status *muiStatus `json:"status,omitempty"`
	View   gitPiece   `json:"view"`
	Keys   []gitKey   `json:"keys"`
}

type gitKey struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	ID    string `json:"id"`
}

// gitPiece is a piece of the UI, of the kind T says, with what that kind
// takes. Lines is a code's lines, or the number of lines tall a text area
// is: the window has the one name for both.
type gitPiece struct {
	T           string          `json:"t"`
	ID          string          `json:"id,omitempty"`
	Text        string          `json:"text,omitempty"`
	Style       string          `json:"style,omitempty"`
	Tone        string          `json:"tone,omitempty"`
	Pad         int             `json:"pad,omitempty"`
	Gap         int             `json:"gap,omitempty"`
	Grow        int             `json:"grow,omitempty"`
	W           int             `json:"w,omitempty"`
	Split       bool            `json:"split,omitempty"`
	Scroll      bool            `json:"scroll,omitempty"`
	Live        bool            `json:"live,omitempty"`
	Selected    string          `json:"selected,omitempty"`
	Items       []gitEntry      `json:"items,omitempty"`
	Lines       json.RawMessage `json:"lines,omitempty"`
	Label       string          `json:"label,omitempty"`
	Primary     bool            `json:"primary,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Value       *string         `json:"value,omitempty"`
	Children    []gitPiece      `json:"children,omitempty"`
}

// gitEntry is what the list holds: a file or a commit (an id and a
// title), a group's head, or a rule.
type gitEntry struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Note  string `json:"note,omitempty"`
	Right string `json:"right,omitempty"`
	Head  string `json:"head,omitempty"`
	Tag   string `json:"tag,omitempty"`
	Rule  bool   `json:"rule,omitempty"`
}

// gitLine is a line of the code: a hunk's head with its place at the
// right, a line added, deleted or the same with its numbers in the old
// file and the new, or a gap that stands for lines left out.
type gitLine struct {
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Old   int    `json:"old,omitempty"`
	New   int    `json:"new,omitempty"`
	Right string `json:"right,omitempty"`
}

// gitOp is the one change a patch of this view makes: lines put at the
// end of the code's.
type gitOp struct {
	Op    string            `json:"op"`
	ID    string            `json:"id"`
	Lines []json.RawMessage `json:"lines"`
}

// gitRow is a row the keys can be on: a file in its group (s staged, c
// changed, u untracked) or a commit (k), by the id the window knows it by.
type gitRow struct {
	id     string
	group  byte
	change git.Change
	commit git.Commit
}

// gitHunk is the hunk a line of the code is in: of which file of the diff,
// and which of its hunks. A line in none has file -1.
type gitHunk struct{ file, hunk int }

var noHunk = gitHunk{-1, -1}

// cutTo is the first most characters of words.
func cutTo(words string, most int) string {
	if runes := []rune(words); len(runes) > most {
		return string(runes[:most])
	}
	return words
}

// counted is a number of things in words: "3 files", "1 file".
func counted(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

// plusMinus is lines added and deleted as the list and the diff's head
// say them: "+15 -3", "+96", "" for none.
func plusMinus(added, deleted int) string {
	var words []string
	if added > 0 {
		words = append(words, fmt.Sprintf("+%d", added))
	}
	if deleted > 0 {
		words = append(words, fmt.Sprintf("-%d", deleted))
	}
	return strings.Join(words, " ")
}

// gitAge is how long ago, in a short word: "now", "5m", "2h", "3d".
func gitAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// gitStands is where a branch stands against its base: "3 ahead of main,
// none behind". With no base it says nothing.
func gitStands(ahead, behind int, base string) string {
	some := func(n int) string {
		if n == 0 {
			return "none"
		}
		return strconv.Itoa(n)
	}
	switch {
	case base == "":
		return ""
	case ahead == 0 && behind == 0:
		return "level with " + base
	}
	return fmt.Sprintf("%s ahead of %s, %s behind", some(ahead), base, some(behind))
}

// gitGroups is how many files are in each group, those with none left
// out: "2 staged · 3 changed · 2 untracked", and "clean" with none at all.
func gitGroups(s git.Standing) string {
	var words []string
	for _, group := range []struct {
		n    int
		word string
	}{{len(s.Staged), "staged"}, {len(s.Changed), "changed"}, {len(s.Untracked), "untracked"}} {
		if group.n > 0 {
			words = append(words, fmt.Sprintf("%d %s", group.n, group.word))
		}
	}
	if len(words) == 0 {
		return "clean"
	}
	return strings.Join(words, " · ")
}

// gitList is the list of a standing: its files under Staged, Changed and
// Untracked, a group with none left out and no more than most files in
// all, then the commits ahead of the base under a rule; and the rows of
// it the keys can be on, in order.
func gitList(s git.Standing, commits []git.Commit, base string, ahead, most int, now time.Time) ([]gitEntry, []gitRow) {
	var entries []gitEntry
	var rows []gitRow
	for _, group := range []struct {
		head    string
		mark    byte
		changes []git.Change
	}{{"Staged", 's', s.Staged}, {"Changed", 'c', s.Changed}, {"Untracked", 'u', s.Untracked}} {
		if len(group.changes) == 0 {
			continue
		}
		shown := group.changes[:min(len(group.changes), most)]
		most -= len(shown)
		note := strconv.Itoa(len(group.changes))
		if len(shown) < len(group.changes) {
			note = fmt.Sprintf("%d of %d shown", len(shown), len(group.changes))
		}
		entries = append(entries, gitEntry{Head: group.head, Note: note})
		for _, change := range shown {
			id := cutTo(string(group.mark)+":"+change.Path, gitWordsMost)
			title := change.Path
			if runes := []rune(title); len(runes) > gitWordsMost { // a long path is known by its end
				title = "…" + string(runes[len(runes)-gitWordsMost+1:])
			}
			entries = append(entries, gitEntry{ID: id, Title: title, Note: change.State, Right: plusMinus(change.Added, change.Deleted)})
			rows = append(rows, gitRow{id: id, group: group.mark, change: change})
		}
	}
	if len(commits) == 0 {
		return entries, rows
	}
	if len(entries) > 0 {
		entries = append(entries, gitEntry{Rule: true})
	}
	branch := s.Branch
	if branch == "" {
		branch = "no branch"
	}
	entries = append(entries, gitEntry{Head: "Commits", Tag: "on " + branch, Note: fmt.Sprintf("%d ahead of %s", ahead, base)})
	for _, commit := range commits {
		title := cutTo(commit.Subject, gitWordsMost)
		if title == "" {
			title = commit.SHA
		}
		entries = append(entries, gitEntry{ID: "k:" + commit.SHA, Title: title, Right: gitAge(now.Sub(commit.When))})
		rows = append(rows, gitRow{id: "k:" + commit.SHA, group: 'k', commit: commit})
	}
	return entries, rows
}

// gitKept is the row the keys are on once the standing was read again:
// the same row where it still is, else the same file in another group (a
// file staged is no longer changed), else the row now where it was, which
// is the one that came after it.
func gitKept(at string, was []string, rows []gitRow) string {
	has := func(id string) bool {
		return slices.ContainsFunc(rows, func(row gitRow) bool { return row.id == id })
	}
	switch {
	case len(rows) == 0:
		return ""
	case has(at):
		return at
	}
	if len(at) > 2 && at[0] != 'k' {
		for _, mark := range []string{"s", "c", "u"} {
			if id := mark + at[1:]; has(id) {
				return id
			}
		}
	}
	return rows[min(max(slices.Index(was, at), 0), len(rows)-1)].id
}

// gitShown is a line of a file as the window is told it: a tab as the
// spaces to the next fourth column, a carriage return at its end taken
// off, and no more characters than the window keeps.
func gitShown(text string) string {
	text = strings.TrimSuffix(text, "\r")
	if !strings.Contains(text, "\t") {
		return cutTo(text, gitLineMost)
	}
	var out []rune
	for _, r := range text {
		if len(out) >= gitLineMost {
			break
		}
		if r != '\t' {
			out = append(out, r)
			continue
		}
		for range 4 - len(out)%4 {
			out = append(out, ' ')
		}
	}
	return string(out)
}

// gitCode is a diff as lines of code: each hunk under a head that says
// what it is in and which hunk of how many it is, each line with its
// number in the old file and the new, and the lines git left out between
// two hunks as a gap. With them, the hunk each line is in. A diff of more
// than one file (a commit's) has a head over each file's.
func gitCode(files []git.FileDiff) ([]gitLine, []gitHunk) {
	var lines []gitLine
	var hunks []gitHunk
	put := func(line gitLine, in gitHunk) {
		lines, hunks = append(lines, line), append(hunks, in)
	}
	total, n := 0, 0
	for _, file := range files {
		total += len(file.Hunks)
	}
	for f, file := range files {
		if len(files) > 1 {
			put(gitLine{Kind: "head", Text: gitShown(file.Path)}, noHunk)
		}
		switch {
		case file.Binary:
			put(gitLine{Kind: "gap", Text: "a binary file"}, noHunk)
		case len(file.Hunks) == 0:
			put(gitLine{Kind: "gap", Text: "no line of it changed"}, noHunk)
		}
		for h, hunk := range file.Hunks {
			in := gitHunk{f, h}
			if h > 0 {
				before := file.Hunks[h-1]
				if same := hunk.OldStart - before.OldStart - before.OldLines; same > 0 {
					put(gitLine{Kind: "gap", Text: counted(same, "line") + " the same"}, noHunk)
				}
			}
			n++
			head := "@@ " + hunk.Section
			switch {
			case hunk.OldLines == 0 && hunk.OldStart == 0:
				head = "@@ new file"
			case hunk.NewLines == 0 && hunk.NewStart == 0:
				head = "@@ deleted file"
			case hunk.Section == "":
				head = fmt.Sprintf("@@ line %d", hunk.NewStart)
			}
			put(gitLine{Kind: "head", Text: gitShown(head), Right: fmt.Sprintf("hunk %d of %d", n, total)}, in)
			old, new := hunk.OldStart, hunk.NewStart
			for _, line := range hunk.Body {
				switch line[0] {
				case '+':
					put(gitLine{Kind: "add", Text: gitShown(line[1:]), New: new}, in)
					new++
				case '-':
					put(gitLine{Kind: "del", Text: gitShown(line[1:]), Old: old}, in)
					old++
				case ' ':
					put(gitLine{Kind: "same", Text: gitShown(line[1:]), Old: old, New: new}, in)
					old, new = old+1, new+1
				}
			}
		}
	}
	return lines, hunks
}

// gitAdded is an untracked file as lines of code: every line of it added.
func gitAdded(content []byte) []gitLine {
	switch {
	case len(content) == 0:
		return []gitLine{{Kind: "gap", Text: "an empty file"}}
	case bytes.IndexByte(content, 0) >= 0:
		return []gitLine{{Kind: "gap", Text: "a binary file"}}
	}
	lines := []gitLine{{Kind: "head", Text: "@@ new file", Right: "hunk 1 of 1"}}
	for n, line := range strings.Split(strings.TrimSuffix(string(content), "\n"), "\n") {
		lines = append(lines, gitLine{Kind: "add", Text: gitShown(line), New: n + 1})
	}
	return lines
}

// gitCut is the first most lines of a code, and after them a gap that
// says how many more there are.
func gitCut(lines []gitLine, hunks []gitHunk, most int) ([]gitLine, []gitHunk) {
	if len(lines) <= most {
		return lines, hunks
	}
	gap := gitLine{Kind: "gap", Text: counted(len(lines)-most, "more line")}
	return append(lines[:most:most], gap), append(hunks[:most:most], noHunk)
}

// gitFit is how many of a code's lines, from the first, a message with
// room bytes for them can say.
func gitFit(lines []json.RawMessage, room int) int {
	n := 0
	for n < len(lines) && room > len(lines[n]) {
		room -= len(lines[n]) + 1 // and the comma between two
		n++
	}
	return n
}

// gitPatches is a code's lines as patches that put them at its end, in
// order: each no more bytes than the window takes.
func gitPatches(pane string, lines []json.RawMessage) [][]byte {
	var all [][]byte
	for len(lines) > 0 {
		n := max(gitFit(lines, muiLimit-2*patchEnvelope), 1)
		all = append(all, encode(muiMessage{V: 1, Pane: pane, Patch: []json.RawMessage{encode(gitOp{Op: "lines", ID: "diff", Lines: lines[:n]})}}))
		lines = lines[n:]
	}
	return all
}

// gitView is how a worktree stands, as the window is shown it. top is dir
// itself where dir is in no repository: git is run nowhere else.
type gitView struct {
	pane, dir string // the pane it is drawn over, and the folder it is run in
	top       string // the worktree's top folder
	standing  git.Standing
	base      string // what the branch stands against, "" where there is nothing
	ahead     int
	behind    int
	entries   []gitEntry
	rows      []gitRow
	at        string         // the id of the row under the keys
	files     []git.FileDiff // the diff of that row, a file's or a commit's
	lines     []gitLine      // and its lines of code
	hunks     []gitHunk      // the hunk each of them is in
	added     int
	deleted   int
	writing   bool       // the commit screen is up
	message   string     // the commit message typed so far
	status    *muiStatus // how the last thing done went
	failed    string     // why the standing could not be read
}

// row is the row of an id.
func (v *gitView) row(id string) (gitRow, bool) {
	at := slices.IndexFunc(v.rows, func(row gitRow) bool { return row.id == id })
	if at < 0 {
		return gitRow{}, false
	}
	return v.rows[at], true
}

// gitBase is what a branch stands against: the layer under it in a stack
// mia knows, else the repository's base branch, and for the base branch
// itself what it follows.
func gitBase(top, branch string) string {
	base := git.DefaultBranch(top)
	if a, err := app.Open(top); err == nil {
		if under := a.Stacks().ParentOf(branch); under != "" && branch != "" {
			return under
		}
		base = a.BaseBranch(top)
	}
	if base == branch && branch != "" {
		return git.Upstream(top, branch)
	}
	return base
}

// read reads how the worktree stands again, and reports whether the row
// under the keys is another than it was: the same file where it still is,
// else the next.
func (v *gitView) read(now time.Time) (moved bool) {
	var was []string
	for _, row := range v.rows {
		was = append(was, row.id)
	}
	before := v.at
	v.top, v.standing, v.base, v.ahead, v.behind, v.failed = v.dir, git.Standing{}, "", 0, 0, ""
	var commits []git.Commit
	top, err := git.Toplevel(v.dir)
	if err == nil {
		v.top = top
		v.standing, err = git.StandingOf(top)
	}
	if err != nil {
		v.failed = cutTo(err.Error(), gitWordsMost)
	} else if v.base = gitBase(top, v.standing.Branch); v.base != "" && v.standing.Head != "" {
		v.behind, v.ahead = git.BehindAhead(top, v.base, "HEAD")
		commits = git.Ahead(top, v.base, gitCommitsMost)
	}
	// (Paths past ASCII are six bytes a character in a message: fewer files
	// are listed until the list leaves the diff most of the message.)
	for most := gitRowsMost; ; most /= 2 {
		v.entries, v.rows = gitList(v.standing, commits, v.base, v.ahead, most, now)
		if most <= 1 || len(encode(v.entries)) < muiLimit/4 {
			break
		}
	}
	v.at = gitKept(v.at, was, v.rows)
	if err := v.show(); err != nil {
		v.failed = cutTo(err.Error(), gitWordsMost)
	}
	return v.at != before
}

// show reads the diff of the row under the keys: what is staged of a
// staged file, what is not of a changed one, an untracked file's lines as
// added, a commit's own.
func (v *gitView) show() error {
	v.files, v.lines, v.hunks, v.added, v.deleted = nil, nil, nil, 0, 0
	row, ok := v.row(v.at)
	if !ok {
		return nil
	}
	var err error
	switch row.group {
	case 's', 'c':
		v.files, err = git.DiffOf(v.top, row.change.Path, row.group == 's')
	case 'k':
		v.files, err = git.Shown(v.top, row.commit.SHA)
	case 'u':
		path := filepath.Join(v.top, row.change.Path)
		var info os.FileInfo
		var content []byte
		if info, err = os.Stat(path); err == nil && info.Size() > gitFileMost {
			v.lines = []gitLine{{Kind: "gap", Text: fmt.Sprintf("a file of %d MB", info.Size()>>20)}}
		} else if content, err = os.ReadFile(path); err == nil {
			v.lines = gitAdded(content)
		}
		v.hunks = slices.Repeat([]gitHunk{noHunk}, len(v.lines))
	}
	if row.group != 'u' {
		v.lines, v.hunks = gitCode(v.files)
	}
	for _, line := range v.lines {
		switch line.Kind {
		case "add":
			v.added++
		case "del":
			v.deleted++
		}
	}
	v.lines, v.hunks = gitCut(v.lines, v.hunks, gitLinesMost)
	return err
}

// words is a text of words in a style and a tone, and nothing where there
// are no words: the window takes no text without them.
func words(text, style, tone string) []gitPiece {
	if text == "" {
		return nil
	}
	return []gitPiece{{T: "text", Text: text, Style: style, Tone: tone}}
}

// lineOf is a row of pieces with room between them, those after the first
// gap at its right end.
func lineOf(left []gitPiece, right ...gitPiece) gitPiece {
	return gitPiece{T: "row", Pad: 10, Gap: 10, Children: append(append(left, gitPiece{T: "col", Grow: 1}), right...)}
}

// about is the line over the diff: the file or the commit it is of, its
// state, and its lines added and deleted.
func (v *gitView) about() gitPiece {
	row, _ := v.row(v.at)
	name, state := row.change.Path, ""
	also := row.change.State
	switch {
	case also == "new":
		also = ", new file"
	case also == "renamed":
		also = ", renamed from " + row.change.From
	case also != "":
		also = ", " + also
	}
	switch row.group {
	case 's':
		state = "staged" + also
	case 'c':
		state = "changed, not staged" + also
	case 'u':
		state = "untracked"
	case 'k':
		name, state = row.commit.SHA, row.commit.Subject
	default:
		state = "nothing to show"
	}
	if v.writing && row.group == 's' {
		state += " · what goes in"
	} else if v.writing && (row.group == 'c' || row.group == 'u') {
		state += " · stays out"
	}
	left := append(words(cutTo(name, gitWordsMost), "mono", ""), words(cutTo(state, gitWordsMost), "dim", "")...)
	return lineOf(left, words(plusMinus(v.added, v.deleted), "mono", "ok")...)
}

// whole is the message that says the whole view with these lines of the
// diff in it. With placed it says where the keys are and what the commit
// message holds; without, those stay as the person has them.
func (v *gitView) whole(lines json.RawMessage, placed bool) []byte {
	s := v.standing
	name := s.Branch
	switch {
	case name == "" && s.Head != "":
		name = "detached at " + cutTo(s.Head, 7)
	case name == "": // in no repository
		name = filepath.Base(v.dir)
	}
	head := gitPiece{T: "row", Pad: 14, Gap: 10, Children: append(append(append(
		words(cutTo(name, gitWordsMost), "title", ""),
		words(gitStands(v.ahead, v.behind, v.base), "dim", "")...),
		gitPiece{T: "col", Grow: 1}),
		words(gitGroups(s), "dim", "")...)}

	list := gitPiece{T: "list", ID: "files", Live: true, Items: v.entries}
	code := gitPiece{T: "code", ID: "diff", Live: true, Lines: lines}
	side := gitPiece{T: "col", Children: []gitPiece{v.about(), code}}
	stage := "stage"
	if strings.HasPrefix(v.at, "s:") {
		stage = "unstage"
	}
	keys := []gitKey{{"space", stage, "stage"}, {"a", "stage all", "all"}}
	if v.writing {
		added, deleted := 0, 0
		for _, change := range s.Staged {
			added, deleted = added+change.Added, deleted+change.Deleted
		}
		var out []string
		if n := len(s.Changed); n > 0 {
			out = append(out, fmt.Sprintf("%d changed", n))
		}
		if n := len(s.Untracked); n > 0 {
			out = append(out, fmt.Sprintf("%d untracked", n))
		}
		if len(out) > 0 {
			out = []string{strings.Join(out, " and ") + " stay out"}
		}
		area := gitPiece{T: "field", ID: "message", Live: true, Lines: json.RawMessage("6"), Placeholder: "What this commit makes true"}
		if placed {
			area.Value = &v.message
		}
		side.Children = []gitPiece{
			lineOf(append(append(words("Commit to "+cutTo(name, gitWordsMost-10), "title", ""), words(counted(len(s.Staged), "file")+" staged", "dim", "")...), words(plusMinus(added, deleted), "mono", "ok")...)),
			{T: "col", Pad: 10, Children: []gitPiece{area}},
			lineOf([]gitPiece{{T: "button", ID: "send", Label: "Commit", Primary: true}}, words(strings.Join(out, ""), "dim", "")...),
			{T: "divider"},
			v.about(), code,
		}
		keys = append(keys, gitKey{"r", "ask again", "again"}, gitKey{"esc", "back to the files", "back"})
	} else {
		keys = append(keys, gitKey{"c", "commit", "commit"}, gitKey{"r", "ask again", "again"})
	}
	if placed {
		list.Selected = v.at
	}
	message := gitMessage{V: 1, Pane: v.pane, Fill: true, Keys: keys, Status: v.status, View: gitPiece{T: "col", Children: []gitPiece{
		head,
		{T: "divider"},
		{T: "row", Split: true, Grow: 1, Children: []gitPiece{
			{T: "col", W: 360, Scroll: true, Pad: 6, Children: []gitPiece{list}},
			{T: "divider"},
			side,
		}},
	}}}
	if v.failed != "" {
		message.Status = &muiStatus{Text: v.failed, Tone: "bad"}
	}
	return encode(message)
}

// messages is the messages that say the view: the whole of it with the
// first lines of the diff that fit, then the rest of them as patches.
func (v *gitView) messages(placed bool) [][]byte {
	lines := make([]json.RawMessage, len(v.lines))
	for i, line := range v.lines {
		lines[i] = encode(line)
	}
	n := gitFit(lines, muiLimit-len(v.whole(nil, placed))-patchEnvelope)
	var first json.RawMessage
	if n > 0 {
		first, _ = json.Marshal(lines[:n]) // each already in ASCII alone
	}
	return append([][]byte{v.whole(first, placed)}, gitPatches(v.pane, lines[n:])...)
}

// gitEvent is what the person did in the window: a row of the list come
// under the keys (move), a key or the button pressed, with the row or the
// line of the diff it was pressed on as its item, the commit message
// changed or sent, close.
type gitEvent struct {
	E    string `json:"e"`
	ID   string `json:"id"`
	List string `json:"list"`
	Item string `json:"item"`
	Text string `json:"text"`
}

// gitDo is what an event asks for.
type gitDo int

const (
	gitNothing gitDo = iota
	gitLeave
	gitMove        // the diff of another row
	gitTyped       // the commit message as it now is
	gitStage       // a file into the index
	gitUnstage     // a file out of it
	gitStageHunk   // one hunk into the index
	gitUnstageHunk // one hunk out of it
	gitStageAll
	gitWrite  // the commit screen
	gitBack   // back to the files
	gitAgain  // read the standing again
	gitCommit // commit what is staged
)

// gitDeed is what an event does: with a row, a hunk's patch or the commit
// message, as it needs.
type gitDeed struct {
	do    gitDo
	row   gitRow
	patch string
	text  string
}

// deed is what an event does to the view as it stands. space on a file's
// row stages it, or unstages one that is staged; on a line of the diff it
// does so to the hunk the line is in, and to the whole of an untracked
// file. On a commit, and on a line in no hunk, it does nothing.
func (v *gitView) deed(event gitEvent) gitDeed {
	switch {
	case event.E == "close":
		return gitDeed{do: gitLeave}
	case event.E == "move" && event.List == "files":
		if row, ok := v.row(event.ID); ok && row.id != v.at {
			return gitDeed{do: gitMove, row: row}
		}
	case event.E == "change" && event.ID == "message":
		return gitDeed{do: gitTyped, text: event.Text}
	case event.E == "submit" && event.ID == "message":
		return gitDeed{do: gitCommit, text: event.Text}
	}
	if event.E != "press" {
		return gitDeed{}
	}
	switch event.ID {
	case "all":
		return gitDeed{do: gitStageAll}
	case "commit":
		return gitDeed{do: gitWrite}
	case "back":
		return gitDeed{do: gitBack}
	case "again":
		return gitDeed{do: gitAgain}
	case "send":
		return gitDeed{do: gitCommit, text: v.message}
	case "stage":
		row, onRow := v.row(event.Item)
		if !onRow { // on a line of the diff, which is of the row the list is on
			place, err := strconv.Atoi(event.Item)
			if err != nil || place < 0 || place >= len(v.hunks) {
				return gitDeed{}
			}
			row, _ = v.row(v.at)
			switch in := v.hunks[place]; {
			case row.group == 'u':
			case in == noHunk || row.group != 's' && row.group != 'c':
				return gitDeed{}
			case row.group == 's':
				return gitDeed{do: gitUnstageHunk, row: row, patch: v.files[in.file].Patch(in.hunk)}
			default:
				return gitDeed{do: gitStageHunk, row: row, patch: v.files[in.file].Patch(in.hunk)}
			}
		}
		switch row.group {
		case 's':
			return gitDeed{do: gitUnstage, row: row}
		case 'c', 'u':
			return gitDeed{do: gitStage, row: row}
		}
	}
	return gitDeed{}
}

// act does a deed, and reports whether the view is to be said again, and
// whether with where the keys are and what the commit message holds:
// when the row under them is another, and when the commit screen comes up.
func (v *gitView) act(deed gitDeed, now time.Time) (say, placed bool) {
	switch deed.do {
	case gitNothing, gitLeave:
		return false, false
	case gitTyped:
		v.message = deed.text
		return false, false
	case gitMove:
		v.at, v.failed = deed.row.id, ""
		if err := v.show(); err != nil {
			v.failed = cutTo(err.Error(), gitWordsMost)
		}
		return true, false
	}
	v.status = nil
	var err error
	switch deed.do {
	case gitWrite:
		v.writing = true
		return true, true
	case gitBack:
		v.writing = false
		return true, false
	case gitStage:
		err = git.Stage(v.top, deed.row.change.Path)
	case gitUnstage:
		paths := []string{deed.row.change.Path}
		if from := deed.row.change.From; from != "" {
			paths = append(paths, from)
		}
		err = git.Unstage(v.top, paths...)
	case gitStageHunk, gitUnstageHunk:
		err = git.ApplyToIndex(v.top, deed.patch, deed.do == gitUnstageHunk)
	case gitStageAll:
		err = git.Stage(v.top)
	case gitCommit:
		var hash string
		switch {
		case len(v.standing.Staged) == 0:
			err = errors.New("nothing is staged: space stages a file, a stages all")
		case strings.TrimSpace(deed.text) == "":
			err = errors.New("no message: write what this commit makes true")
		default:
			if hash, err = git.CommitStaged(v.top, deed.text); err == nil {
				v.writing, v.message = false, ""
				v.status = &muiStatus{Text: "committed " + hash, Tone: "ok"}
			}
		}
	}
	if err != nil {
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
	}
	return true, v.read(now)
}

// gitMark is what changes when the standing does, cheap to ask for: git
// status as it writes it, with the commit it is at, and the size and time
// of each file changed or untracked, which says a file changed again.
func gitMark(dir, top string) string {
	out, err := git.Status(dir)
	if err != nil {
		return err.Error()
	}
	s := git.ParseStatus(out)
	var mark strings.Builder
	mark.WriteString(out)
	for _, change := range slices.Concat(s.Changed, s.Untracked) {
		if info, err := os.Lstat(filepath.Join(top, change.Path)); err == nil {
			fmt.Fprintf(&mark, "%d %d\n", info.Size(), info.ModTime().UnixNano())
		}
	}
	return mark.String()
}

// cmdGit shows how the worktree it is run in stands in git, drawn by mia
// desk over its pane: its files staged, changed and untracked and its
// commits ahead, the diff of the one under the keys beside them, and the
// keys that stage, unstage and commit. It reads the standing again after
// each thing done and when the repository changes.
func cmdGit(args []string) int {
	if len(args) > 0 {
		return usageErr(gitUsage)
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fail(errors.New("git reads what you do from a terminal: run it in a pane of mia desk"))
	}
	dir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	view := &gitView{pane: os.Getenv("TMUX_PANE"), dir: dir}
	view.read(time.Now())

	stop := make(chan struct{})
	defer close(stop)
	keys := muiKeys(stop)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	say, shut, err := muiOpen(view.pane, "mia git: the plain terminal. In mia desk how "+filepath.Base(dir)+" stands in git is drawn over it; q or ⌃C leaves.")
	if err != nil {
		return fail(err)
	}
	defer shut()

	var shown []byte // what the view said last, but for where the keys are
	show := func(placed bool) {
		shown = bytes.Join(view.messages(false), nil)
		say(view.messages(placed)...)
	}
	show(true)
	mark := gitMark(dir, view.top)
	poll := time.NewTicker(gitPoll)
	defer poll.Stop()
	buffer := ""
	for {
		select {
		case <-signals:
			return exitOK
		case <-poll.C:
			now := gitMark(dir, view.top)
			if now == mark {
				continue
			}
			mark = now
			if placed := view.read(time.Now()); !bytes.Equal(bytes.Join(view.messages(false), nil), shown) {
				show(placed)
			}
		case data, open := <-keys:
			if !open {
				return exitOK
			}
			events, rest, sight, leave := muiTyped[gitEvent](buffer, data)
			if leave {
				return exitOK
			}
			buffer = rest
			again, placed := sight, sight // the pane came into sight: said again whole
			for i, event := range events {
				deed := view.deed(event)
				if deed.do == gitLeave {
					return exitOK
				}
				if deed.do == gitMove && i+1 < len(events) && events[i+1].E == "move" {
					continue // the keys held down: only where they came to rest is read
				}
				said, here := view.act(deed, time.Now())
				again, placed = again || said, placed || here
			}
			if again {
				show(placed)
			}
		}
	}
}
