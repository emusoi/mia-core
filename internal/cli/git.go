package cli

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
// ahead, its stashes, the files said of a commit, the branches to pick
// from, and the lines of a diff; the characters the window keeps of a
// line of code and of an id, a title or a status; the bytes read of an
// untracked file to show it; how often the repository is looked at; and
// the files past which the list is behind tabs, a group at a time.
const (
	gitTabsOver    = 30
	gitRowsMost    = 400
	gitCommitsMost = 20
	gitStashesMost = 10
	gitFilesMost   = 6
	gitBranchMost  = 300
	gitLinesMost   = 1500
	gitLineMost    = 400
	gitWordsMost   = 200
	gitFileMost    = 8 << 20
	gitPoll        = 2 * time.Second
)

// gitMessage is the whole view said to the window: a UI that fills its
// pane, and the keys the program takes.
type gitMessage struct {
	V    int    `json:"v"`
	Pane string `json:"pane,omitempty"`
	Fill bool   `json:"fill"`
	// Focus is the piece the keys are put on: the window does it once each
	// time another piece is named, so a view said again leaves them be.
	Focus  string     `json:"focus,omitempty"`
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
// is: the window has the one name for both. Title is a modal's, Menu the
// word for what → opens on a list's items, Does what ⏎ does on a line of
// a code, Max the rows a list shows in a room of its own, and Motion how
// a column comes in.
type gitPiece struct {
	T           string          `json:"t"`
	ID          string          `json:"id,omitempty"`
	Title       string          `json:"title,omitempty"`
	Menu        string          `json:"menu,omitempty"`
	Does        string          `json:"does,omitempty"`
	Max         int             `json:"max,omitempty"`
	Motion      string          `json:"motion,omitempty"`
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
	Key         string          `json:"key,omitempty"`
	Primary     bool            `json:"primary,omitempty"`
	Bare        bool            `json:"bare,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Right       string          `json:"right,omitempty"`
	List        string          `json:"list,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Value       *string         `json:"value,omitempty"`
	Children    []gitPiece      `json:"children,omitempty"`
}

// gitEntry is what a list holds: a file, a commit, a stash or a branch
// (an id and a title; the branch the worktree is on checked, what ⏎
// does on one, and the menu of what can be done to it), a group's head,
// or a rule; and what tabs hold: a tab, with its count as its note.
type gitEntry struct {
	ID      string      `json:"id,omitempty"`
	Title   string      `json:"title,omitempty"`
	Note    string      `json:"note,omitempty"`
	Right   string      `json:"right,omitempty"`
	Icon    string      `json:"icon,omitempty"`
	Checked bool        `json:"checked,omitempty"`
	Does    string      `json:"does,omitempty"`
	Head    string      `json:"head,omitempty"`
	Tag     string      `json:"tag,omitempty"`
	Rule    bool        `json:"rule,omitempty"`
	Menu    []muiChoice `json:"menu,omitempty"`
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
// changed, u untracked), a commit (k) or a stash (t), by the id the window
// knows it by.
type gitRow struct {
	id     string
	group  byte
	change git.Change
	commit git.Commit
	stash  git.Stash
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

// gitFollows is where a branch stands against what it follows: "2 to
// push, 1 to pull". A branch that follows nothing says nothing.
func gitFollows(s git.Standing) string {
	var words []string
	if s.ToPush > 0 {
		words = append(words, fmt.Sprintf("%d to push", s.ToPush))
	}
	if s.ToPull > 0 {
		words = append(words, fmt.Sprintf("%d to pull", s.ToPull))
	}
	switch {
	case s.Upstream == "":
		return ""
	case s.Gone:
		return cutTo(s.Upstream, gitWordsMost-10) + " is gone"
	case len(words) == 0:
		return "nothing to push or pull"
	}
	return strings.Join(words, ", ")
}

// gitNamed is a path in words said of it, a long one known by its end.
func gitNamed(path string) string {
	const most = 80
	if runes := []rune(path); len(runes) > most {
		return "…" + string(runes[len(runes)-most+1:])
	}
	return path
}

// gitStashed is the list with the stashes after it, newest first, under a
// rule and their head, and the rows with theirs. With no stash both are
// as they were.
func gitStashed(entries []gitEntry, rows []gitRow, stashes []git.Stash, now time.Time) ([]gitEntry, []gitRow) {
	if len(stashes) == 0 {
		return entries, rows
	}
	if len(entries) > 0 {
		entries = append(entries, gitEntry{Rule: true})
	}
	note := strconv.Itoa(len(stashes))
	if len(stashes) > gitStashesMost {
		note = fmt.Sprintf("%d of %d shown", gitStashesMost, len(stashes))
	}
	entries = append(entries, gitEntry{Head: "Stashes", Note: note})
	for _, stash := range stashes[:min(len(stashes), gitStashesMost)] {
		id := "t:" + cutTo(stash.SHA, 10)
		entries = append(entries, gitEntry{ID: id, Title: cutTo(stash.Subject, gitWordsMost), Right: gitAge(now.Sub(stash.When))})
		rows = append(rows, gitRow{id: id, group: 't', stash: stash})
	}
	return entries, rows
}

// gitMenus is the entries with, on each file, commit and stash, the menu
// of what can be done to it: each choice by the id of the key that does
// the same, with that key at its right. A file is staged or unstaged,
// discarded where it is not staged, handed to the editor where it is
// there, and its path copied; a commit's hash is copied, and the last
// one undone where undo says it can be; a stash is applied, popped and
// dropped. Three dots end the choices that ask first.
func gitMenus(entries []gitEntry, rows []gitRow, undo bool) []gitEntry {
	edit := muiChoice{ID: "edit", Title: "Open in editor", Right: "e"}
	path := muiChoice{ID: "copy", Title: "Copy path", Right: "y"}
	at, tip := 0, true // the rows are the entries that have an id, in order
	for i := range entries {
		if entries[i].ID == "" || at >= len(rows) {
			continue
		}
		row := rows[at]
		at++
		var menu []muiChoice
		switch row.group {
		case 's':
			menu = []muiChoice{{ID: "stage", Title: "Unstage", Right: "space"}}
		case 'c':
			menu = []muiChoice{{ID: "stage", Title: "Stage", Right: "space"}, {ID: "discard", Title: "Discard…", Right: "d"}}
		case 'u':
			menu = []muiChoice{{ID: "stage", Title: "Stage", Right: "space"}, {ID: "discard", Title: "Delete…", Right: "d"}}
		case 'k':
			menu = []muiChoice{{ID: "copy", Title: "Copy its hash", Right: "y"}}
			if tip && undo {
				menu = append(menu, muiChoice{ID: "undo", Title: "Undo…", Right: "u"})
			}
			tip = false
		case 't':
			menu = []muiChoice{{ID: "stage", Title: "Apply", Right: "space"}, {ID: "push", Title: "Pop", Right: "p"}, {ID: "discard", Title: "Drop…", Right: "d"}}
		}
		if row.group == 's' || row.group == 'c' || row.group == 'u' {
			if row.change.State != "deleted" {
				menu = append(menu, edit)
			}
			menu = append(menu, path)
		}
		entries[i].Menu = menu
	}
	return entries
}

// gitTabOf is the tab a row of a group is behind: the files under
// Changes, the commits and the stashes under their own.
func gitTabOf(group byte) string {
	switch group {
	case 'k':
		return "commits"
	case 't':
		return "stashes"
	}
	return "changes"
}

// gitBehind is the entries of a list that are behind a tab: its groups
// under their heads, without the rules between one tab's and the next's.
func gitBehind(entries []gitEntry, tab string) []gitEntry {
	var behind []gitEntry
	in := "changes"
	for _, entry := range entries {
		switch {
		case entry.Rule:
			continue
		case entry.Head == "Commits":
			in = "commits"
		case entry.Head == "Stashes":
			in = "stashes"
		}
		if in == tab {
			behind = append(behind, entry)
		}
	}
	return behind
}

// gitBranchStands is how a branch stands against what it follows, in a
// short word: "↑2 ↓1", "level", "gone", and "no upstream" for one that
// follows nothing.
func gitBranchStands(branch git.Branch) string {
	var words []string
	if branch.Ahead > 0 {
		words = append(words, fmt.Sprintf("↑%d", branch.Ahead))
	}
	if branch.Behind > 0 {
		words = append(words, fmt.Sprintf("↓%d", branch.Behind))
	}
	switch {
	case branch.Upstream == "":
		return "no upstream"
	case branch.Gone:
		return "gone"
	case len(words) == 0:
		return "level"
	}
	return strings.Join(words, " ")
}

// gitBranchList is the branches to pick from, those with the words typed
// in their name whatever their case: the repository's own (l:), the one
// the worktree is on checked and each with how it stands against what it
// follows, then under a head the remotes' no branch here is named for
// (r:), most at most in all. Words that are no branch's name offer a new
// branch of that name from the one the worktree is on (n:): first where
// they are in no name at all, else last under a rule. With them, how many
// branches there are to pick from.
func gitBranchList(branches []git.Branch, current, typed string, most int) ([]gitEntry, int) {
	typed = strings.TrimSpace(typed)
	has := func(name string) bool {
		return strings.Contains(strings.ToLower(name), strings.ToLower(typed))
	}
	local := map[string]bool{}
	for _, branch := range branches {
		if !branch.Remote {
			local[branch.Name] = true
		}
	}
	var own, far []gitEntry
	exact := false
	for _, branch := range branches {
		_, short, _ := strings.Cut(branch.Name, "/")
		switch {
		case branch.Remote && local[short] || !has(branch.Name):
		case branch.Remote:
			exact = exact || short == typed
			far = append(far, gitEntry{ID: cutTo("r:"+branch.Name, gitWordsMost), Title: cutTo(branch.Name, gitWordsMost), Does: "follow it here"})
		default:
			exact = exact || branch.Name == typed
			own = append(own, gitEntry{ID: cutTo("l:"+branch.Name, gitWordsMost), Title: cutTo(branch.Name, gitWordsMost), Right: gitBranchStands(branch), Icon: "git-branch", Checked: branch.Current, Does: "switch"})
		}
	}
	own = own[:min(len(own), most)]
	far = far[:min(len(far), most-len(own))]
	if current == "" {
		current = "here"
	}
	create := gitEntry{ID: cutTo("n:"+typed, gitWordsMost), Title: cutTo("Create "+typed+" from "+current, gitWordsMost), Does: "create"}
	found := len(own) + len(far)
	entries := own
	if len(far) > 0 {
		entries = append(append(entries, gitEntry{Head: "Remote", Note: strconv.Itoa(len(far))}), far...)
	}
	switch {
	case typed == "" || exact:
	case found == 0:
		entries = []gitEntry{create}
	default:
		entries = append(entries, gitEntry{Rule: true}, create)
	}
	return entries, found
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
	author    string       // who wrote the commit under the keys
	when      time.Time    // and when
	common    string       // the repository's own folder, where its stash is kept
	stashes   []git.Stash  // the stash, newest first
	unpushed  int          // the commits no remote has, of a branch that follows nothing
	inDiff    bool         // the keys are on a line of the diff, as the window said
	writing   bool         // the commit screen is up
	message   string       // the commit message typed so far
	picking   bool         // the branches are up in a modal, to pick one
	typed     string       // the words typed to find one
	branches  []git.Branch // and the branches there are
	armed     gitArmed     // what a modal asks, and is done when it is asked again
	back      string       // the piece the keys go back to when a modal goes, "" for the files
	tab       string       // the tab the list shows, where it is behind tabs
	busy      string       // what is being done with a remote: "fetching origin"
	work      func() gitOutcome
	status    *muiStatus // what was refused, or failed, and is the person's to act on
	passing   muiPassing // what was just done, said once
	failed    string     // why the standing could not be read
}

// gitArmed is a deed asked once that loses something, or sends it where
// it cannot be taken back: what it is, the row it is on, the hunk, and
// the words that say what it does, which a modal asks in; with them the
// line under those words, what its button is called, and the key and the
// item it was asked by, by which it is asked again. Asked again the same
// in its first four, it is done.
type gitArmed struct {
	do    gitDo
	id    string
	patch string
	ask   string
	told  string
	yes   string
	key   string
	item  string
}

// gitOutcome is how a deed done with a remote went: what it was, the line
// git wrote last, or why it failed.
type gitOutcome struct {
	what string
	said string
	err  error
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
	v.stashes, v.unpushed = nil, 0
	var commits []git.Commit
	top, err := "", errors.New("mia git has no folder to run git in")
	if v.dir != "" { // with none git would read the folder mia is in
		top, err = git.Toplevel(v.dir)
	}
	if err == nil {
		v.top = top
		v.standing, err = git.StandingOf(top)
	}
	if err == nil {
		v.stashes = git.Stashes(top)
		if v.common == "" {
			v.common, _ = git.CommonDir(top)
		}
		if (v.standing.Upstream == "" || v.standing.Gone) && v.standing.Head != "" {
			v.unpushed = git.Unpushed(top)
		}
		if v.picking {
			v.branches, _ = git.Branches(top)
		}
	}
	if err != nil {
		v.failed = cutTo(err.Error(), gitWordsMost)
	} else if v.base = gitBase(top, v.standing.Branch); v.base != "" && v.standing.Head != "" {
		v.behind, v.ahead = git.BehindAhead(top, v.base, "HEAD")
		commits = git.Ahead(top, v.base, gitCommitsMost)
	}
	// (Paths past ASCII are six bytes a character in a message: fewer files
	// are listed until the list, with its menus, leaves the diff half of it.)
	for most := gitRowsMost; ; most /= 2 {
		v.entries, v.rows = gitList(v.standing, commits, v.base, v.ahead, most, now)
		v.entries, v.rows = gitStashed(v.entries, v.rows, v.stashes, now)
		v.entries = gitMenus(v.entries, v.rows, v.kept() == "")
		if most <= 1 || len(encode(v.entries)) < muiLimit/2 {
			break
		}
	}
	v.at = gitKept(v.at, was, v.rows)
	if row, ok := v.row(v.at); ok {
		v.tab = gitTabOf(row.group)
	}
	if err := v.show(); err != nil {
		v.failed = cutTo(err.Error(), gitWordsMost)
	}
	return v.at != before
}

// show reads the diff of the row under the keys: what is staged of a
// staged file, what is not of a changed one, an untracked file's lines as
// added, a commit's own with who wrote it and when, a stash's.
func (v *gitView) show() error {
	v.files, v.lines, v.hunks, v.added, v.deleted = nil, nil, nil, 0, 0
	v.author, v.when = "", time.Time{}
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
		v.author, v.when = git.Told(v.top, row.commit.SHA)
	case 't':
		v.files, err = git.StashShown(v.top, row.stash.SHA)
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

// about is the line over the diff: the file, the commit or the stash it
// is of, its state — of a commit who wrote it and when — and its lines
// added and deleted.
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
		if !v.writing { // its subject is said under this line, over its files
			state = strings.TrimPrefix(v.author+" · "+v.when.Format("2006-01-02 15:04"), " · ")
		}
	case 't':
		name, state = cutTo(row.stash.SHA, 7), row.stash.Subject
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

// told is what is said of a commit over its diff: its subject, and each
// file it changed with its lines added and deleted, the first of them
// where they are many.
func (v *gitView) told() []gitPiece {
	row, _ := v.row(v.at)
	if row.group != 'k' || v.writing {
		return nil
	}
	lines := words(cutTo(row.commit.Subject, gitWordsMost), "", "")
	for _, file := range v.files[:min(len(v.files), gitFilesMost)] {
		added, deleted := file.Counts()
		name := words(cutTo(strings.TrimSuffix(file.Path, "\t"), gitWordsMost), "mono", "")
		lines = append(lines, gitPiece{T: "row", Gap: 10, Children: append(append(name, gitPiece{T: "col", Grow: 1}), words(plusMinus(added, deleted), "mono", "ok")...)})
	}
	if more := len(v.files) - gitFilesMost; more > 0 {
		lines = append(lines, words("and "+counted(more, "more file"), "dim", "")...)
	}
	return []gitPiece{{T: "col", Pad: 10, Gap: 4, Children: lines}, {T: "divider"}}
}

// kept is why the last commit is not to be made again or taken off: a
// remote has it, or it is the base's own. "" where it is this branch's
// alone.
func (v *gitView) kept() string {
	s := v.standing
	switch {
	case s.Upstream != "" && !s.Gone && s.ToPush == 0:
		return "the last commit is on " + cutTo(s.Upstream, gitWordsMost/2) + " already"
	case v.base != "" && v.ahead == 0:
		return "the last commit is " + cutTo(v.base, gitWordsMost/2) + "'s, not this branch's"
	}
	return ""
}

// tip is whether a row is the last commit, the one u can undo.
func (v *gitView) tip(row gitRow) bool {
	first := slices.IndexFunc(v.rows, func(row gitRow) bool { return row.group == 'k' })
	return row.group == 'k' && first >= 0 && v.rows[first].id == row.id
}

// keys is the keys the program takes where the keys are: on a file what
// stages and discards it, hands it to the editor and copies its path, in
// its diff what stages and discards a hunk (what ⏎ does to a line the
// code says itself), on a commit what copies its hash and on the last
// one what undoes it, on a stash what applies, pops and drops it; then
// those of the worktree whole. The commit screen has its own few, and
// under a modal the window takes none of them.
func (v *gitView) keys() []gitKey {
	row, _ := v.row(v.at)
	stage := "stage"
	if row.group == 's' {
		stage = "unstage"
	}
	switch {
	case v.picking: // the modal's own: the window says them itself
		return []gitKey{}
	case v.writing:
		return []gitKey{{"space", stage, "stage"}, {"a", "stage all", "all"}, {"r", "ask again", "again"}, {"esc", "back to the files", "back"}}
	}
	var keys []gitKey
	switch {
	case row.group == 't':
		keys = []gitKey{{"space", "apply", "stage"}, {"p", "pop", "push"}, {"d", "drop", "discard"}}
	case row.group == 'k' && v.tip(row) && v.kept() == "":
		keys = []gitKey{{"u", "undo the commit", "undo"}, {"y", "copy the hash", "copy"}}
	case row.group == 'k':
		keys = []gitKey{{"y", "copy the hash", "copy"}}
	case row.group == 'c' && v.inDiff:
		keys = []gitKey{{"space", "stage hunk", "stage"}, {"d", "discard hunk", "discard"}}
	case row.group == 's' && v.inDiff:
		keys = []gitKey{{"space", "unstage hunk", "stage"}}
	case row.group == 's':
		keys = []gitKey{{"space", stage, "stage"}}
	case row.group == 'c':
		keys = []gitKey{{"space", stage, "stage"}, {"d", "discard", "discard"}}
	case row.group == 'u':
		keys = []gitKey{{"space", stage, "stage"}, {"d", "delete", "discard"}}
	}
	if file := row.group == 's' || row.group == 'c' || row.group == 'u'; file && row.change.State != "deleted" {
		keys = append(keys, gitKey{"e", "edit in " + filepath.Base(editorWords()[0]), "edit"})
	}
	if (row.group == 's' || row.group == 'c' || row.group == 'u') && !v.inDiff {
		keys = append(keys, gitKey{"y", "copy the path", "copy"})
	}
	s := v.standing
	if len(s.Changed)+len(s.Untracked) > 0 {
		keys = append(keys, gitKey{"a", "stage all", "all"})
	}
	keys = append(keys, gitKey{"c", "commit", "commit"}, gitKey{"b", "branches", "branches"})
	if len(s.Staged)+len(s.Changed)+len(s.Untracked) > 0 {
		keys = append(keys, gitKey{"s", "stash", "stash"})
	}
	keys = append(keys, gitKey{"f", "fetch", "fetch"}, gitKey{"P", "pull", "pull"})
	if row.group != 't' { // on a stash p pops it
		keys = append(keys, gitKey{"p", "push", "push"})
	}
	return append(keys, gitKey{"r", "ask again", "again"})
}

// tabs is the tabs the list is behind where its files are many: Changes,
// and Commits and Stashes where there are some, each with its count. With
// fewer files there are none, and the list is whole.
func (v *gitView) tabs() []gitEntry {
	s := v.standing
	files := len(s.Staged) + len(s.Changed) + len(s.Untracked)
	if files <= gitTabsOver {
		return nil
	}
	tabs := []gitEntry{{ID: "changes", Title: "Changes", Note: strconv.Itoa(files)}}
	if slices.ContainsFunc(v.rows, func(row gitRow) bool { return row.group == 'k' }) {
		tabs = append(tabs, gitEntry{ID: "commits", Title: "Commits", Note: strconv.Itoa(v.ahead)})
	}
	if len(v.stashes) > 0 {
		tabs = append(tabs, gitEntry{ID: "stashes", Title: "Stashes", Note: strconv.Itoa(len(v.stashes))})
	}
	return tabs
}

// gitLetters is the key that asks for each deed a modal asks about: its
// button has the same, so the key pressed twice does it.
var gitLetters = map[string]string{"discard": "d", "undo": "u", "push": "p"}

// modals is the modal over the view, where there is one: what is asked
// before a deed that loses something, in the words that say what it
// does, with the button that does it — its key the one that asked — and
// the one that leaves things as they are; or the branches to pick from, a
// field that walks their list, with why git refused the last one picked.
func (v *gitView) modals(placed bool) []gitPiece {
	switch {
	case v.armed != gitArmed{}:
		return []gitPiece{{T: "modal", ID: "sure", Title: v.armed.ask, W: 520, Children: append(words(v.armed.told, "dim", ""), gitPiece{T: "row", Children: []gitPiece{
			{T: "button", ID: "yes", Label: v.armed.yes, Key: gitLetters[v.armed.key], Primary: true},
			{T: "button", ID: "no", Label: "Keep things as they are", Key: "esc"},
		}})}}
	case v.picking:
		entries, found := gitBranchList(v.branches, v.standing.Branch, v.typed, gitBranchMost)
		count := fmt.Sprintf("%d branches", found)
		if found == 1 {
			count = "1 branch"
		}
		field := gitPiece{T: "field", ID: "q", Bare: true, Icon: "search", Live: true, List: "branches", Placeholder: "Switch to a branch, or name a new one", Right: count}
		picks := gitPiece{T: "list", ID: "branches", Max: 12, Items: entries}
		if placed {
			// The keys on the branch the worktree is on, where ⏎ changes
			// nothing; once words are typed, on the first they find.
			at := slices.IndexFunc(entries, func(entry gitEntry) bool { return entry.ID != "" && (entry.Checked || v.typed != "") })
			if at < 0 {
				at = slices.IndexFunc(entries, func(entry gitEntry) bool { return entry.ID != "" })
			}
			if at >= 0 {
				picks.Selected = entries[at].ID
			}
			if v.typed == "" {
				field.Value = &v.typed
			}
		}
		pieces := []gitPiece{field, {T: "divider"}, picks}
		if v.status != nil && v.status.Tone == "bad" {
			pieces = append(pieces, words(v.status.Text, "", "bad")...)
		}
		return []gitPiece{{T: "modal", ID: "switch", Title: "Switch branch", W: 520, Children: pieces}}
	}
	return nil
}

// whole is the message that says the whole view with these lines of the
// diff in it. With placed it says where the keys are and what the commit
// message holds; without, those stay as the person has them. What is
// done with a remote is a bar at the head's end while it runs, and a
// modal is said last, over all of it.
func (v *gitView) whole(lines json.RawMessage, placed bool) []byte {
	s := v.standing
	name := s.Branch
	switch {
	case name == "" && s.Head != "":
		name = "detached at " + cutTo(s.Head, 7)
	case name == "": // in no repository
		name = filepath.Base(v.dir)
	}
	head := gitPiece{T: "row", Pad: 14, Gap: 10, Children: append(append(append(append(
		words(cutTo(name, gitWordsMost), "title", ""),
		words(gitStands(v.ahead, v.behind, v.base), "dim", "")...),
		words(gitFollows(s), "dim", "")...),
		gitPiece{T: "col", Grow: 1}),
		words(gitGroups(s), "dim", "")...)}
	if v.busy != "" {
		head.Children = append(head.Children, gitPiece{T: "col", W: 240, Children: []gitPiece{{T: "progress", ID: "remote", Text: cutTo(v.busy+"…", gitWordsMost)}}})
	}

	row, _ := v.row(v.at)
	list := gitPiece{T: "list", ID: "files", Live: true, Menu: "what to do", Items: v.entries}
	if placed {
		list.Selected = v.at
	}
	code := gitPiece{T: "code", ID: "diff", Live: true, Lines: lines}
	switch row.group {
	case 'c':
		code.Does = "stage line"
	case 's':
		code.Does = "unstage line"
	}
	// (Said again whole on every move: it comes in at once, and no diff fades.)
	side := gitPiece{T: "col", ID: "side", Motion: "none", Children: append(append([]gitPiece{v.about()}, v.told()...), code)}
	left := gitPiece{T: "col", W: 360, Scroll: true, Pad: 6, Children: []gitPiece{list}}
	if tabs := v.tabs(); tabs != nil {
		list.Items = gitBehind(v.entries, v.tab)
		left = gitPiece{T: "col", W: 360, Children: []gitPiece{
			{T: "tabs", ID: "tabs", Selected: v.tab, Items: tabs},
			{T: "col", Scroll: true, Grow: 1, Pad: 6, Children: []gitPiece{list}},
		}}
	}
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
			// (⌘A is said on the button and is no key the window takes: tab and ⏎ press it.)
			lineOf([]gitPiece{{T: "button", ID: "send", Label: "Commit", Primary: true}, {T: "button", ID: "amend", Label: "Amend the last one", Key: "⌘A"}}, words(strings.Join(out, ""), "dim", "")...),
			{T: "divider"},
			v.about(), code,
		}
	}
	// Asked before a deed, the keys are on the button that does it;
	// picking a branch, in the words that find it; writing a commit, in
	// its message; and back from each, on the piece they were on.
	focus := cmp.Or(v.back, "files")
	switch {
	case v.armed != gitArmed{}:
		focus = "yes"
	case v.picking:
		focus = "q"
	case v.writing:
		focus = "message"
	}
	message := gitMessage{V: 1, Pane: v.pane, Fill: true, Focus: focus, Keys: v.keys(), Status: v.status, View: gitPiece{T: "col", Children: append([]gitPiece{
		head,
		{T: "divider"},
		{T: "row", Split: true, Grow: 1, Children: []gitPiece{
			left,
			{T: "divider"},
			side,
		}},
	}, v.modals(placed)...)}}
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

// gitEvent is what the person did in the window: a row of the list or a
// line of the diff come under the keys (move), a key or a button pressed,
// with the row or the line of the diff it was pressed on as its item, a
// line of the diff or a branch picked with ⏎, the commit message or the
// words that find a branch changed or sent, close.
type gitEvent struct {
	E    string `json:"e"`
	ID   string `json:"id"`
	List string `json:"list"`
	Item string `json:"item"`
	Text string `json:"text"`
	Sub  string `json:"sub"`
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
	gitRefuse // say why not, and do nothing
	gitStageLine
	gitUnstageLine
	gitDiscard     // what is changed in a file, or an untracked file
	gitDiscardHunk // one hunk of what is changed
	gitAmend       // the last commit made again
	gitUndo        // the last commit taken off, its changes left staged
	gitBranches    // the branches, to pick one
	gitNarrow      // those with the words typed
	gitSwitch      // to the branch picked, or a new one
	gitStash       // every change put in the stash
	gitStashApply
	gitStashPop
	gitStashDrop
	gitFetch
	gitPull
	gitPush
	gitKeep  // what a modal asks left undone, and the modal taken away
	gitFocus // the keys on another piece: the files, their tabs or the diff
	gitTab   // another tab of the list
	gitEdit  // a file handed to the editor at a line
	gitCopy  // a path, or a commit's hash, put on the clipboard
)

// gitDeed is what an event does: with a row, a hunk's or a line's patch,
// a line of the file, the commit message, the words typed or copied or
// why it is refused, as it needs. One that loses something has what it
// does in words (ask), a line under them (told) and the name of the
// button that does it (yes), with the key and the item it was asked by,
// and is done only once it is sure: asked a second time, the same.
type gitDeed struct {
	do    gitDo
	row   gitRow
	patch string
	line  int
	text  string
	ask   string
	told  string
	yes   string
	key   string
	item  string
	sure  bool
}

// refused is the deed that only says why not.
func refused(why string) gitDeed { return gitDeed{do: gitRefuse, text: why} }

// twice is a deed that is asked twice, first in a modal: sure when what
// it does, in these words, is what the modal asks.
func (v *gitView) twice(deed gitDeed, ask, told, yes string) gitDeed {
	deed.ask, deed.told, deed.yes = cutTo(ask, gitWordsMost), told, yes
	deed.sure = v.armed.ask != "" && v.armed.do == deed.do && v.armed.id == deed.row.id && v.armed.patch == deed.patch && v.armed.ask == deed.ask
	return deed
}

// editing is what e does: the file of the row the keys are on is handed
// to the editor, on a line of its diff at that line of the file as it is
// now, a deleted line's being that of the line after it. A file that is
// deleted has nothing to edit.
func (v *gitView) editing(item string) gitDeed {
	row, place, ok := v.on(item)
	switch {
	case !ok || row.group != 's' && row.group != 'c' && row.group != 'u':
		return gitDeed{}
	case row.change.State == "deleted":
		return refused(gitNamed(row.change.Path) + " is deleted: there is nothing to edit")
	}
	line := 1
	for at := max(place, 0); place >= 0 && at < len(v.lines); at++ {
		if v.lines[at].New > 0 {
			line = v.lines[at].New
			break
		}
	}
	return gitDeed{do: gitEdit, row: row, line: line}
}

// copying is what y does: on a file its path is put on the clipboard, on
// a commit its hash.
func (v *gitView) copying(item string) gitDeed {
	row, place, ok := v.on(item)
	switch {
	case !ok || place >= 0:
	case row.group == 'k':
		return gitDeed{do: gitCopy, text: row.commit.SHA}
	case row.group != 't':
		return gitDeed{do: gitCopy, text: row.change.Path}
	}
	return gitDeed{}
}

// on is the row a key was pressed on and, pressed on a line of the diff,
// that line's place: the diff is of the row the list is on. On a row
// itself the place is -1.
func (v *gitView) on(item string) (row gitRow, place int, ok bool) {
	if row, ok := v.row(item); ok {
		return row, -1, true
	}
	place, err := strconv.Atoi(item)
	if err != nil || place < 0 || place >= len(v.hunks) {
		return gitRow{}, -1, false
	}
	row, ok = v.row(v.at)
	return row, place, ok
}

// lined is what ⏎ does on a line of the diff: a line added or deleted of
// a changed file is staged alone, of a staged file unstaged alone.
func (v *gitView) lined(item string) gitDeed {
	row, place, ok := v.on(item)
	if !ok || place < 0 || row.group != 's' && row.group != 'c' {
		return gitDeed{}
	}
	in := v.hunks[place]
	if kind := v.lines[place].Kind; in == noHunk || kind != "add" && kind != "del" {
		return refused("⏎ stages a line added or deleted: the keys are on neither")
	}
	// Its place among the hunk's lines as git wrote them: git's own notes
	// are not said, and the hunk's head is the line over its first.
	nth := 0
	for at := place - 1; at >= 0 && v.lines[at].Kind != "head"; at-- {
		nth++
	}
	written := -1
	for at, text := range v.files[in.file].Hunks[in.hunk].Body {
		if text[0] == '\\' {
			continue
		}
		if nth == 0 {
			written = at
			break
		}
		nth--
	}
	patch, err := v.files[in.file].LinePatch(in.hunk, written, row.group == 's')
	switch {
	case err != nil:
		return refused(err.Error())
	case row.group == 's':
		return gitDeed{do: gitUnstageLine, row: row, patch: patch}
	}
	return gitDeed{do: gitStageLine, row: row, patch: patch}
}

// discarding is what d does: on a changed file what is changed in it and
// not staged is discarded, on a line of its diff the hunk that line is
// in; an untracked file is deleted; a stash is dropped. Each is asked
// first, in a modal whose words say what is lost. A staged file is
// unstaged first.
func (v *gitView) discarding(item string) gitDeed {
	row, place, ok := v.on(item)
	if !ok {
		return gitDeed{}
	}
	name := gitNamed(row.change.Path)
	lost := plusMinus(row.change.Added, row.change.Deleted)
	if row.change.State != "" {
		lost = strings.TrimSpace(row.change.State + " " + lost)
	}
	switch row.group {
	case 't':
		return v.twice(gitDeed{do: gitStashDrop, row: row}, fmt.Sprintf("Drop the stash %q and what it holds?", cutTo(row.stash.Subject, 100)), "Git keeps no other copy of it.", "Drop")
	case 's':
		return refused("what is staged is not discarded: space unstages it first")
	case 'u':
		return v.twice(gitDeed{do: gitDiscard, row: row}, "Delete "+name+", untracked and in no commit?", "Git cannot bring it back.", "Delete")
	case 'c':
		if place < 0 {
			return v.twice(gitDeed{do: gitDiscard, row: row}, fmt.Sprintf("Discard what is changed in %s and not staged (%s)?", name, lost), "Git cannot bring it back. What is staged of it stays.", "Discard")
		}
		in := v.hunks[place]
		if in == noHunk {
			return gitDeed{}
		}
		file := v.files[in.file]
		added, deleted := git.FileDiff{Hunks: file.Hunks[in.hunk : in.hunk+1]}.Counts()
		which := "this hunk"
		for at := place; at >= 0; at-- {
			if v.lines[at].Kind == "head" {
				which = v.lines[at].Right
				break
			}
		}
		return v.twice(gitDeed{do: gitDiscardHunk, row: row, patch: file.Patch(in.hunk)}, fmt.Sprintf("Discard %s of %s (%s)?", which, name, plusMinus(added, deleted)), "Git cannot bring it back.", "Discard")
	}
	return gitDeed{}
}

// undoing is what u does: on the last commit, where no remote has it, it
// is taken off the branch and what it changed left staged, asked first.
func (v *gitView) undoing(item string) gitDeed {
	row, _, ok := v.on(item)
	switch why := v.kept(); {
	case !ok || row.group != 'k':
		return gitDeed{}
	case !v.tip(row):
		return refused("only the last commit is undone: the keys are on an older one")
	case why != "":
		return refused(why + ": to undo it would take a push by force")
	}
	return v.twice(gitDeed{do: gitUndo, row: row}, fmt.Sprintf("Undo %s %q?", row.commit.SHA, cutTo(row.commit.Subject, 100)), "It is taken off the branch, and what it changed stays staged.", "Undo")
}

// pushing is what p does off a stash: the branch's commits are pushed to
// what it follows, or to origin under its own name where it follows
// nothing, asked first in words that say how many and where.
func (v *gitView) pushing() gitDeed {
	s := v.standing
	to, n := s.Upstream, s.ToPush
	if to == "" || s.Gone {
		n = v.unpushed
	}
	if to == "" {
		to = "origin/" + s.Branch
	}
	switch {
	case s.Branch == "":
		return refused("on no branch: there is nothing to push")
	case n == 0 && s.Upstream != "" && !s.Gone:
		return refused("nothing to push: " + cutTo(to, gitWordsMost/2) + " has every commit")
	}
	return v.twice(gitDeed{do: gitPush}, fmt.Sprintf("Push %s to %s?", counted(n, "commit"), gitNamed(to)), "Nothing is forced: a remote that has moved on refuses it.", "Push")
}

// deed is what an event does to the view as it stands. It decides and
// changes nothing. The button of a modal that does what it asks, and the
// key that asked pressed again, ask it again as things now stand; a
// choice of a row's menu does what the key of its id does on that row.
func (v *gitView) deed(event gitEvent) gitDeed {
	switch {
	case event.E == "press" && event.ID == "yes" && v.armed != gitArmed{}:
		event = gitEvent{E: "press", ID: v.armed.key, Item: v.armed.item}
	case event.E == "pick" && event.List == "files" && event.Sub != "":
		event = gitEvent{E: "press", ID: event.Sub, Item: event.ID}
	}
	deed := v.asked(event)
	if deed.ask != "" {
		deed.key, deed.item = event.ID, event.Item
	}
	return deed
}

// asked is what an event asks for. space on a file's row stages it, or
// unstages one that is staged; on a line of the diff it does so to the
// hunk the line is in, and to the whole of an untracked file; on a
// commit, and on a line in no hunk, it does nothing. d, u and p are asked
// in a modal first, which says what they do, and esc and its other
// button leave it undone; ⏎ on a line of the diff stages or unstages that
// line alone; b brings the branches up in a modal, where ⏎ switches to
// one or makes the one named; s stashes; on a stash space applies it, p
// pops it and d drops it; f, P and p fetch, pull and push; e hands a file
// to the editor and y copies its path, or a commit's hash.
func (v *gitView) asked(event gitEvent) gitDeed {
	s := v.standing
	switch {
	case event.E == "close":
		return gitDeed{do: gitLeave}
	case event.E == "dismiss" && v.picking:
		return gitDeed{do: gitBack}
	case event.E == "dismiss":
		return gitDeed{do: gitKeep}
	case event.E == "focus":
		if event.ID == "files" || event.ID == "tabs" || event.ID == "diff" {
			return gitDeed{do: gitFocus, text: event.ID}
		}
	case event.E == "pick" && event.List == "tabs":
		// The keys of the list go on the first row behind the tab.
		first := slices.IndexFunc(v.rows, func(row gitRow) bool { return gitTabOf(row.group) == event.ID })
		if first >= 0 && event.ID != v.tab && v.tabs() != nil {
			return gitDeed{do: gitTab, row: v.rows[first], text: event.ID}
		}
	case event.E == "move" && event.List == "files":
		if row, ok := v.row(event.ID); ok && row.id != v.at {
			return gitDeed{do: gitMove, row: row}
		}
	case event.E == "change" && event.ID == "message":
		return gitDeed{do: gitTyped, text: event.Text}
	case event.E == "submit" && event.ID == "message":
		return gitDeed{do: gitCommit, text: event.Text}
	case event.E == "change" && event.ID == "q" && v.picking && event.Text != v.typed:
		return gitDeed{do: gitNarrow, text: event.Text}
	case event.E == "pick" && event.List == "branches" && v.picking:
		if event.ID == "l:"+s.Branch { // where the worktree is already
			return gitDeed{do: gitBack}
		}
		return gitDeed{do: gitSwitch, text: event.ID}
	case event.E == "pick" && event.List == "diff":
		return v.lined(event.ID)
	}
	if event.E != "press" {
		return gitDeed{}
	}
	switch event.ID {
	case "no":
		return gitDeed{do: gitKeep}
	case "edit":
		return v.editing(event.Item)
	case "copy":
		return v.copying(event.Item)
	case "discard":
		return v.discarding(event.Item)
	case "undo":
		return v.undoing(event.Item)
	case "branches":
		return gitDeed{do: gitBranches}
	case "stash":
		if len(s.Staged)+len(s.Changed)+len(s.Untracked) == 0 {
			return refused("nothing to stash: no file is changed")
		}
		return gitDeed{do: gitStash}
	case "fetch":
		return gitDeed{do: gitFetch}
	case "pull":
		switch {
		case s.Branch == "":
			return refused("on no branch: there is nothing to pull")
		case s.Upstream == "" || s.Gone:
			return refused(cutTo(s.Branch, gitWordsMost/2) + " follows no branch that is there: there is nothing to pull")
		}
		return gitDeed{do: gitPull}
	case "push":
		if row, _, ok := v.on(event.Item); ok && row.group == 't' {
			return gitDeed{do: gitStashPop, row: row}
		}
		return v.pushing()
	case "amend":
		switch why := v.kept(); {
		case s.Head == "":
			return refused("there is no commit to amend")
		case why != "":
			return refused(why + ": to amend it would take a push by force")
		case len(s.Staged) == 0 && strings.TrimSpace(v.message) == "":
			return refused("nothing is staged and no message is written: there is nothing to amend it with")
		}
		return gitDeed{do: gitAmend, text: v.message}
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
			case row.group == 'u', row.group == 't':
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
		case 't':
			return gitDeed{do: gitStashApply, row: row}
		}
	}
	return gitDeed{}
}

// begin has a deed done with a remote: a bar at the head's end says it
// is at work, and the doing of it is left for the loop to start, so the
// keys are taken while it runs. quiet is what is said of one git wrote
// nothing about. One at a time: while one runs another is refused.
func (v *gitView) begin(what, quiet string, do func() (string, error)) {
	if v.busy != "" {
		v.passing.done(v.busy+": wait for it to end", "bad")
		return
	}
	v.busy, v.status = what, nil
	v.work = func() gitOutcome {
		said, err := do()
		if said == "" {
			said = quiet
		}
		return gitOutcome{what, said, err}
	}
}

// finish takes how a deed done with a remote went: the line git wrote
// last is said in a toast, or why it failed is the status, and the
// standing is read again. It reports whether the row under the keys is
// another.
func (v *gitView) finish(outcome gitOutcome, now time.Time) (moved bool) {
	v.busy, v.status = "", nil
	if outcome.err != nil {
		v.status = &muiStatus{Text: cutTo(outcome.err.Error(), gitWordsMost), Tone: "bad"}
	} else {
		v.passing.done(outcome.said, "ok")
	}
	return v.read(now)
}

// here is the piece the keys are on, of the files and the diff: where
// they go back to when a modal goes.
func (v *gitView) here() string {
	if v.inDiff {
		return "diff"
	}
	return "files"
}

// act does a deed, and reports whether the view is to be said again —
// whole, or only its status and its keys where no more is another — and
// whether with where the keys are and what the commit message holds:
// when the row under them is another, and when the commit screen comes
// up. What went well is said once, in a toast; what was refused or
// failed is the status until the next thing is done.
func (v *gitView) act(deed gitDeed, now time.Time) (say, placed bool) {
	switch deed.do {
	case gitNothing, gitLeave, gitEdit:
		return false, false
	case gitFocus:
		v.inDiff = deed.text == "diff"
		return true, false
	case gitTyped:
		v.message = deed.text
		return false, false
	}
	// What a modal asked is answered by whatever is done next: done where
	// that is the same asked again, else left undone.
	v.armed, deed.sure = gitArmed{}, deed.sure || deed.ask == ""
	switch deed.do {
	case gitKeep:
		return true, false
	case gitMove, gitTab:
		v.at, v.failed = deed.row.id, ""
		if deed.do == gitTab {
			v.tab, v.back = deed.text, "tabs"
		}
		if v.top == "" { // nowhere to read its diff
			return true, deed.do == gitTab
		}
		if err := v.show(); err != nil {
			v.failed = cutTo(err.Error(), gitWordsMost)
		}
		return true, deed.do == gitTab
	case gitNarrow:
		v.typed = deed.text
		return true, true
	case gitRefuse:
		v.status = &muiStatus{Text: cutTo(deed.text, gitWordsMost), Tone: "bad"}
		return true, false
	case gitCopy:
		v.passing.copy = deed.text
		v.passing.done("copied "+gitNamed(deed.text), "")
		return true, false
	}
	if !deed.sure { // asked once, in a modal: done when asked again
		v.armed = gitArmed{deed.do, deed.row.id, deed.patch, deed.ask, deed.told, deed.yes, deed.key, deed.item}
		v.back = v.here()
		return true, false
	}
	// git is run in the view's own folder and nowhere else: with none, a
	// command would run in the folder mia is in, on another repository.
	if v.top == "" && deed.do != gitWrite && deed.do != gitBack {
		v.status = &muiStatus{Text: "mia git has no folder to run git in: nothing is done", Tone: "bad"}
		return true, false
	}
	v.status = nil
	var err error
	done := "" // what is said of a deed that went well
	s := v.standing
	switch deed.do {
	case gitWrite:
		v.writing = true
		return true, true
	case gitBack:
		if v.picking {
			v.picking = false
			return true, true
		}
		v.writing, v.back = false, "files"
		return true, false
	case gitBranches:
		v.picking, v.typed, v.back = true, "", v.here()
		if v.branches, err = git.Branches(v.top); err != nil {
			v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
		}
		return true, true
	case gitSwitch:
		kind, name, _ := strings.Cut(deed.text, ":")
		switch kind {
		case "l":
			err = git.Switch(v.top, name)
		case "r":
			err = git.SwitchTracking(v.top, name)
			_, name, _ = strings.Cut(name, "/")
		case "n":
			err = git.SwitchNew(v.top, name)
		}
		if err == nil {
			v.picking, done = false, "on "+name
		}
		placed = true
	case gitStageLine, gitUnstageLine:
		err = git.ApplyToIndex(v.top, deed.patch, deed.do == gitUnstageLine)
	case gitDiscard:
		if deed.row.group == 'u' {
			err, done = git.Clean(v.top, deed.row.change.Path), "deleted "+gitNamed(deed.row.change.Path)
		} else {
			err, done = git.Restore(v.top, deed.row.change.Path), "discarded what was changed in "+gitNamed(deed.row.change.Path)
		}
	case gitDiscardHunk:
		err, done = git.ApplyToWorktree(v.top, deed.patch, true), "discarded a hunk of "+gitNamed(deed.row.change.Path)
	case gitUndo:
		err, done = git.UndoCommit(v.top), "undone: what "+deed.row.commit.SHA+" changed is staged"
	case gitAmend:
		var hash string
		message := deed.text
		if strings.TrimSpace(message) == "" { // the one it had
			message = ""
		}
		if hash, err = git.Amend(v.top, message); err == nil {
			v.writing, v.message, done = false, "", "amended: the last commit is "+hash
		}
	case gitStash:
		branch := s.Branch
		if branch == "" {
			branch = "detached at " + cutTo(s.Head, 7)
		}
		message := "mia git: " + branch + " " + now.Format("2006-01-02 15:04")
		err, done = git.StashAll(v.top, message), "stashed as "+message
	case gitStashApply:
		err, done = git.StashApply(v.top, deed.row.stash.SHA), "applied, and kept: "+deed.row.stash.Subject
	case gitStashPop:
		err, done = git.StashPop(v.top, deed.row.stash.SHA), "popped: "+deed.row.stash.Subject
	case gitStashDrop:
		err, done = git.StashDrop(v.top, deed.row.stash.SHA), "dropped: "+deed.row.stash.Subject
	case gitFetch, gitPull, gitPush:
		top, branch := v.top, s.Branch
		remote, theirs := "", ""
		if branch != "" {
			remote, theirs = git.Follows(top, branch)
		}
		follows := remote != ""
		if !follows {
			remote, theirs = "origin", branch
		}
		switch {
		case deed.do == gitPull:
			v.begin("pulling from "+s.Upstream, "pulled from "+s.Upstream, func() (string, error) { return git.PullFastForward(top) })
		case !follows && !git.HasRemote(top, remote):
			v.status = &muiStatus{Text: "there is no remote named origin, and " + cutTo(filepath.Base(top), gitWordsMost/2) + " follows none", Tone: "bad"}
		case deed.do == gitFetch:
			v.begin("fetching "+remote, "fetched "+remote+": nothing new", func() (string, error) { return git.FetchFrom(top, remote) })
		case s.Upstream != "" && !s.Gone && !follows:
			v.status = &muiStatus{Text: cutTo(branch+" follows "+s.Upstream+", a branch here: there is no remote to push to", gitWordsMost), Tone: "bad"}
		default:
			v.begin("pushing to "+remote+"/"+theirs, "pushed to "+remote+"/"+theirs, func() (string, error) { return git.PushTo(top, remote, branch, theirs, !follows) })
		}
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
		err, done = git.Stage(v.top), "staged "+counted(len(s.Changed)+len(s.Untracked), "file")
	case gitCommit:
		var hash string
		switch {
		case len(v.standing.Staged) == 0:
			err = errors.New("nothing is staged: space stages a file, a stages all")
		case strings.TrimSpace(deed.text) == "":
			err = errors.New("no message: write what this commit makes true")
		default:
			if hash, err = git.CommitStaged(v.top, deed.text); err == nil {
				v.writing, v.message, v.back, done = false, "", "files", "committed "+hash
			}
		}
	}
	switch {
	case err != nil:
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
	case done != "":
		v.passing.done(done, "ok")
	}
	return true, v.read(now) || placed
}

// edit hands a file to the editor at a line, as mia files does, waits
// for it to leave and reads the standing again. An editor that cannot be
// run is the view's status; one that left with an error of its own has
// said so itself.
func (v *gitView) edit(deed gitDeed) {
	err := editorAt(v.top, deed.row.change.Path, deed.line).Run()
	v.read(time.Now())
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
	}
}

// stashMark is what changes when the stash does: the size and time of
// the file git keeps its entries in.
func (v *gitView) stashMark() string {
	info, err := os.Stat(filepath.Join(v.common, "logs", "refs", "stash"))
	if v.common == "" || err != nil {
		return ""
	}
	return fmt.Sprintf("stash %d %d\n", info.Size(), info.ModTime().UnixNano())
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
// desk over its pane: its files staged, changed and untracked, its
// commits ahead and its stashes, the diff of the one under the keys
// beside them, and the keys that stage, discard, commit, amend, undo,
// stash, switch branch, fetch, pull and push, hand a file to the editor
// and copy its path. It reads the standing again after each thing done
// and when the repository changes, which mia serve tells it of when one
// of this build answers, and it looks at every two seconds when none
// does; what is done with a remote runs beside it, and the keys are taken
// meanwhile.
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
	heard, hangUp := listenServer(asked("status", dir))
	defer hangUp()
	view := &gitView{pane: os.Getenv("TMUX_PANE"), dir: dir}
	view.read(time.Now())

	stop, next := make(chan struct{}), make(chan struct{}, 1)
	defer close(stop)
	keys := muiKeysInTurn(stop, next)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	under := "mia git: the plain terminal. In mia desk how " + filepath.Base(dir) + " stands in git is drawn over it; q or ⌃C leaves."
	say, shut, err := muiOpen(view.pane, under)
	if err != nil {
		return fail(err)
	}
	defer func() { shut() }()

	// What was told last: the view is said whole when more than its status
	// and its keys is another, and when it says where the keys are.
	var told muiTold
	show := func(placed bool) {
		var where [][]byte
		if placed {
			where = view.messages(true)
		}
		say(told.says(view.pane, view.messages(false), where, &view.passing)...)
	}
	show(true)
	mark := gitMark(dir, view.top) + view.stashMark()
	poll := time.NewTicker(gitPoll)
	defer poll.Stop()
	if heard != nil {
		poll.Stop()
	}
	done := make(chan gitOutcome, 1) // one deed with a remote at a time
	buffer := ""
	for {
		if work := view.work; work != nil {
			view.work = nil
			go func() { done <- work() }()
		}
		select {
		case <-signals:
			return exitOK
		case outcome := <-done:
			show(view.finish(outcome, time.Now()))
		case _, open := <-heard:
			if !open { // the server went: looked at as with none
				heard = nil
				poll.Reset(gitPoll)
				continue
			}
			show(view.read(time.Now()))
		case <-poll.C:
			now := gitMark(dir, view.top) + view.stashMark()
			if now == mark {
				continue
			}
			mark = now
			show(view.read(time.Now()))
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
				if deed.do == gitEdit { // the editor has the terminal until it leaves
					shut()
					shut = func() {}
					view.edit(deed)
					if !muiLent(signals) {
						return exitOK
					}
					if say, shut, err = muiOpen(view.pane, under); err != nil {
						shut = func() {}
						return fail(err)
					}
					mark = gitMark(dir, view.top) + view.stashMark()
					again, placed = true, true
					continue
				}
				said, here := view.act(deed, time.Now())
				again, placed = again || said, placed || here
			}
			if again {
				show(placed)
			}
			next <- struct{}{}
		}
	}
}
