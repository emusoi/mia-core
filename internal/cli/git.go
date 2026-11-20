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
// ahead, its stashes, the files said of a commit, the branches to pick
// from, and the lines of a diff; the characters the window keeps of a
// line of code and of an id, a title or a status; the bytes read of an
// untracked file to show it; and how often the repository is looked at.
const (
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
// (an id and a title; the branch the worktree is on checked, and what ⏎
// does on one), a group's head, or a rule.
type gitEntry struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Note    string `json:"note,omitempty"`
	Right   string `json:"right,omitempty"`
	Icon    string `json:"icon,omitempty"`
	Checked bool   `json:"checked,omitempty"`
	Does    string `json:"does,omitempty"`
	Head    string `json:"head,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Rule    bool   `json:"rule,omitempty"`
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
	inDiff    bool         // the keys are on a line of the diff, as far as the window said
	writing   bool         // the commit screen is up
	message   string       // the commit message typed so far
	picking   bool         // the branches are up, to pick one
	typed     string       // the words typed to find one
	branches  []git.Branch // and the branches there are
	armed     gitArmed     // what was asked, and is done when it is asked again
	busy      string       // what is being done with a remote: "fetching origin"
	work      func() gitOutcome
	status    *muiStatus // how the last thing done went
	failed    string     // why the standing could not be read
}

// gitArmed is a deed asked once that loses something, or sends it where
// it cannot be taken back: what it is, the row it is on, the hunk, and
// the words that said what it does. Asked again the same, it is done.
type gitArmed struct {
	do    gitDo
	id    string
	patch string
	ask   string
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
	// are listed until the list leaves the diff most of the message.)
	for most := gitRowsMost; ; most /= 2 {
		v.entries, v.rows = gitList(v.standing, commits, v.base, v.ahead, most, now)
		v.entries, v.rows = gitStashed(v.entries, v.rows, v.stashes, now)
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
// stages and discards it, in its diff what does so to a hunk and a line,
// on the last commit what undoes it, on a stash what applies, pops and
// drops it; then those of the worktree whole. The commit screen and the
// branches have their own few.
func (v *gitView) keys() []gitKey {
	row, _ := v.row(v.at)
	stage := "stage"
	if row.group == 's' {
		stage = "unstage"
	}
	switch {
	case v.picking:
		return []gitKey{{"esc", "back to the files", "back"}}
	case v.writing:
		return []gitKey{{"space", stage, "stage"}, {"a", "stage all", "all"}, {"r", "ask again", "again"}, {"esc", "back to the files", "back"}}
	}
	var keys []gitKey
	switch {
	case row.group == 't':
		keys = []gitKey{{"space", "apply", "stage"}, {"p", "pop", "push"}, {"d", "drop", "discard"}}
	case row.group == 'k' && v.tip(row) && v.kept() == "":
		keys = []gitKey{{"u", "undo the commit", "undo"}}
	case row.group == 'c' && v.inDiff:
		keys = []gitKey{{"space", "stage hunk", "stage"}, {"⏎", "stage line", "line"}, {"d", "discard hunk", "discard"}}
	case row.group == 's' && v.inDiff:
		keys = []gitKey{{"space", "unstage hunk", "stage"}, {"⏎", "unstage line", "line"}}
	case row.group == 's':
		keys = []gitKey{{"space", stage, "stage"}}
	case row.group == 'c':
		keys = []gitKey{{"space", stage, "stage"}, {"d", "discard", "discard"}}
	case row.group == 'u':
		keys = []gitKey{{"space", stage, "stage"}, {"d", "delete", "discard"}}
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
	head := gitPiece{T: "row", Pad: 14, Gap: 10, Children: append(append(append(append(
		words(cutTo(name, gitWordsMost), "title", ""),
		words(gitStands(v.ahead, v.behind, v.base), "dim", "")...),
		words(gitFollows(s), "dim", "")...),
		gitPiece{T: "col", Grow: 1}),
		words(gitGroups(s), "dim", "")...)}

	list := gitPiece{T: "list", ID: "files", Live: true, Items: v.entries}
	if placed {
		list.Selected = v.at
	}
	code := gitPiece{T: "code", ID: "diff", Live: true, Lines: lines}
	side := gitPiece{T: "col", Children: append(append([]gitPiece{v.about()}, v.told()...), code)}
	left := gitPiece{T: "col", W: 360, Scroll: true, Pad: 6, Children: []gitPiece{list}}
	if v.picking {
		// The branches in place of the files: a field that walks their list.
		entries, found := gitBranchList(v.branches, s.Branch, v.typed, gitBranchMost)
		count := fmt.Sprintf("%d branches", found)
		if found == 1 {
			count = "1 branch"
		}
		field := gitPiece{T: "field", ID: "q", Bare: true, Icon: "search", Live: true, List: "branches", Placeholder: "Switch to a branch, or name a new one", Right: count}
		picks := gitPiece{T: "list", ID: "branches", Items: entries}
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
		left.Children = []gitPiece{field, {T: "divider"}, picks}
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
	// Writing a commit, the keys are in its message; picking a branch, in
	// the words that find it; back at the files, on them.
	focus := "files"
	switch {
	case v.picking:
		focus = "q"
	case v.writing:
		focus = "message"
	}
	message := gitMessage{V: 1, Pane: v.pane, Fill: true, Focus: focus, Keys: v.keys(), Status: v.status, View: gitPiece{T: "col", Children: []gitPiece{
		head,
		{T: "divider"},
		{T: "row", Split: true, Grow: 1, Children: []gitPiece{
			left,
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
)

// gitDeed is what an event does: with a row, a hunk's or a line's patch,
// the commit message, the words typed or why it is refused, as it needs.
// One that loses something has what it does in words (ask) and is done
// only once it is sure: asked a second time, the same. where is where the
// event says the keys are: f on the files, d in the diff, 0 unsaid.
type gitDeed struct {
	do    gitDo
	row   gitRow
	patch string
	text  string
	ask   string
	sure  bool
	where byte
}

// refused is the deed that only says why not.
func refused(why string) gitDeed { return gitDeed{do: gitRefuse, text: why} }

// twice is a deed that is asked twice: sure when what it does, in these
// words, is what was asked last.
func (v *gitView) twice(deed gitDeed, ask string) gitDeed {
	deed.ask = cutTo(ask, gitWordsMost)
	deed.sure = v.armed == gitArmed{deed.do, deed.row.id, deed.patch, deed.ask}
	return deed
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
// twice, in words that say what is lost. A staged file is unstaged first.
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
		return v.twice(gitDeed{do: gitStashDrop, row: row}, fmt.Sprintf("drop the stash %q and what it holds; d again to drop", cutTo(row.stash.Subject, 100)))
	case 's':
		return refused("what is staged is not discarded: space unstages it first")
	case 'u':
		return v.twice(gitDeed{do: gitDiscard, row: row}, "delete "+name+", untracked and in no commit; d again to discard")
	case 'c':
		if place < 0 {
			return v.twice(gitDeed{do: gitDiscard, row: row}, fmt.Sprintf("discard what is changed in %s and not staged (%s); d again to discard", name, lost))
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
		return v.twice(gitDeed{do: gitDiscardHunk, row: row, patch: file.Patch(in.hunk)}, fmt.Sprintf("discard %s of %s (%s); d again to discard", which, name, plusMinus(added, deleted)))
	}
	return gitDeed{}
}

// undoing is what u does: on the last commit, where no remote has it, it
// is taken off the branch and what it changed left staged, asked twice.
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
	return v.twice(gitDeed{do: gitUndo, row: row}, fmt.Sprintf("undo %s %q: what it changed stays staged; u again to undo", row.commit.SHA, cutTo(row.commit.Subject, 100)))
}

// pushing is what p does off a stash: the branch's commits are pushed to
// what it follows, or to origin under its own name where it follows
// nothing, asked twice in words that say how many and where.
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
	return v.twice(gitDeed{do: gitPush}, fmt.Sprintf("push %s to %s; p again to push", counted(n, "commit"), gitNamed(to)))
}

// deed is what an event does to the view as it stands, and where the
// event says the keys are: on a row of the files, or on a line of the
// diff. It decides and changes nothing.
func (v *gitView) deed(event gitEvent) gitDeed {
	deed := v.asked(event)
	_, onRow := v.row(event.Item)
	switch {
	case event.E == "move" && event.List == "files", event.E == "press" && onRow:
		deed.where = 'f'
	case (event.E == "move" || event.E == "pick") && event.List == "diff", event.E == "press" && event.Item != "":
		deed.where = 'd'
	}
	return deed
}

// asked is what an event asks for. space on a file's row stages it, or
// unstages one that is staged; on a line of the diff it does so to the
// hunk the line is in, and to the whole of an untracked file; on a
// commit, and on a line in no hunk, it does nothing. d, u and p are asked
// twice, and say
// first what they do; ⏎ on a line of the diff stages or unstages that
// line alone; b brings the branches up, where ⏎ switches to one or makes
// the one named; s stashes; on a stash space applies it, p pops it and d
// drops it; f, P and p fetch, pull and push.
func (v *gitView) asked(event gitEvent) gitDeed {
	s := v.standing
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
	case "line":
		return v.lined(event.Item)
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

// working is the status while something is done with a remote, and none
// otherwise.
func (v *gitView) working() *muiStatus {
	if v.busy == "" {
		return nil
	}
	return &muiStatus{Text: cutTo(v.busy+"…", gitWordsMost)}
}

// begin has a deed done with a remote: the status says it is at work, and
// the doing of it is left for the loop to start, so the keys are taken
// while it runs. quiet is what is said of one git wrote nothing about.
// One at a time: while one runs another is refused.
func (v *gitView) begin(what, quiet string, do func() (string, error)) {
	if v.busy != "" {
		v.status = &muiStatus{Text: cutTo(v.busy+": wait for it to end", gitWordsMost), Tone: "bad"}
		return
	}
	v.busy, v.status = what, nil
	v.status = v.working()
	v.work = func() gitOutcome {
		said, err := do()
		if said == "" {
			said = quiet
		}
		return gitOutcome{what, said, err}
	}
}

// finish takes how a deed done with a remote went: the line git wrote
// last is the status, and the standing is read again. It reports whether
// the row under the keys is another.
func (v *gitView) finish(outcome gitOutcome, now time.Time) (moved bool) {
	v.busy, v.armed = "", gitArmed{}
	v.status = &muiStatus{Text: cutTo(outcome.said, gitWordsMost), Tone: "ok"}
	if outcome.err != nil {
		v.status = &muiStatus{Text: cutTo(outcome.err.Error(), gitWordsMost), Tone: "bad"}
	}
	return v.read(now)
}

// act does a deed, and reports whether the view is to be said again, and
// whether with where the keys are and what the commit message holds:
// when the row under them is another, and when the commit screen comes up.
func (v *gitView) act(deed gitDeed, now time.Time) (say, placed bool) {
	keys := v.keys()
	if deed.where != 0 {
		v.inDiff = deed.where == 'd'
	}
	// Anything but what was asked, asked again, drops the asking.
	dropped := v.armed != gitArmed{} && !deed.sure
	if v.armed, deed.sure = (gitArmed{}), deed.sure || deed.ask == ""; dropped {
		v.status = v.working()
	}
	switch deed.do {
	case gitNothing, gitLeave:
		return dropped || !slices.Equal(keys, v.keys()), false
	case gitTyped:
		v.message = deed.text
		return dropped, false
	case gitMove:
		v.at, v.failed = deed.row.id, ""
		if v.top == "" { // nowhere to read its diff
			return true, false
		}
		if err := v.show(); err != nil {
			v.failed = cutTo(err.Error(), gitWordsMost)
		}
		return true, false
	case gitNarrow:
		v.typed = deed.text
		return true, true
	case gitRefuse:
		v.status = &muiStatus{Text: cutTo(deed.text, gitWordsMost), Tone: "bad"}
		return true, false
	}
	if !deed.sure { // asked once: said, and done when asked again
		v.armed = gitArmed{deed.do, deed.row.id, deed.patch, deed.ask}
		v.status = &muiStatus{Text: deed.ask, Tone: "warn"}
		return true, false
	}
	// git is run in the view's own folder and nowhere else: with none, a
	// command would run in the folder mia is in, on another repository.
	if v.top == "" && deed.do != gitWrite && deed.do != gitBack {
		v.status = &muiStatus{Text: "mia git has no folder to run git in: nothing is done", Tone: "bad"}
		return true, false
	}
	v.status = v.working()
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
		v.writing = false
		return true, false
	case gitBranches:
		v.picking, v.typed = true, ""
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
	switch {
	case err != nil:
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
	case done != "":
		v.status = &muiStatus{Text: cutTo(done, gitWordsMost), Tone: "ok"}
	}
	return true, v.read(now) || placed
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
// stash, switch branch, fetch, pull and push. It reads the standing again
// after each thing done and when the repository changes; what is done
// with a remote runs beside it, and the keys are taken meanwhile.
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
	mark := gitMark(dir, view.top) + view.stashMark()
	poll := time.NewTicker(gitPoll)
	defer poll.Stop()
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
		case <-poll.C:
			now := gitMark(dir, view.top) + view.stashMark()
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
