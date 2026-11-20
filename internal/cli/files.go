package cli

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/git"
)

const filesUsage = "mia files"

// What a view of a worktree's files holds at most, all of it under the
// pieces the window takes of a UI (muiPieces): the rows of the tree, the
// files a search by name lists, the lines a search in files finds — each
// under a head of its own at worst — and the lines of a file beside each
// of them, with the pieces around them. Then the folders opened on the
// way to what changed as the view comes up, the bytes of a file that is
// read, those of it looked at to tell a binary one, the files listed of a
// folder in no repository, how near the ends of the lines said of a long
// file the keys come before others are said, the characters kept of a
// line a search found, and how long after one batch of what a search
// found the next is said.
const (
	filesRowsMost     = 400
	filesFoundMost    = 50
	filesHitsMost     = 500
	filesLinesMost    = 1500
	filesHitLinesMost = muiPieces - 2*filesHitsMost - filesAround
	filesAround       = 40
	filesOpenMost     = 30
	filesFileMost     = 2 << 20
	filesBinaryLook   = 8000
	filesWalkMost     = 20000
	filesEdge         = 100
	filesHitMost      = 160
	filesBatch        = 100 * time.Millisecond
)

// filesMessage is the whole view said to the window: a UI that fills its
// pane, and the keys the program takes.
type filesMessage struct {
	V    int    `json:"v"`
	Pane string `json:"pane,omitempty"`
	Fill bool   `json:"fill"`
	// Focus is the piece the keys are put on: the window does it once each
	// time another piece is named, so a view said again leaves them be.
	Focus  string     `json:"focus,omitempty"`
	Status *muiStatus `json:"status,omitempty"`
	View   filesPiece `json:"view"`
	Keys   []gitKey   `json:"keys"`
}

// filesPiece is a piece of the UI, of the kind T says, with what that
// kind takes. List is the list a field walks, Right the word at a field's
// end, and Selected the row of a list, or the line of a code, the keys
// are put on.
type filesPiece struct {
	T           string          `json:"t"`
	ID          string          `json:"id,omitempty"`
	Text        string          `json:"text,omitempty"`
	Style       string          `json:"style,omitempty"`
	Pad         int             `json:"pad,omitempty"`
	Gap         int             `json:"gap,omitempty"`
	W           int             `json:"w,omitempty"`
	Split       bool            `json:"split,omitempty"`
	Scroll      bool            `json:"scroll,omitempty"`
	Live        bool            `json:"live,omitempty"`
	Bare        bool            `json:"bare,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Right       string          `json:"right,omitempty"`
	List        string          `json:"list,omitempty"`
	Value       *string         `json:"value,omitempty"`
	Selected    string          `json:"selected,omitempty"`
	Items       []filesEntry    `json:"items,omitempty"`
	Lines       json.RawMessage `json:"lines,omitempty"`
	Children    []filesPiece    `json:"children,omitempty"`
}

// filesEntry is what a list holds: a row of the tree (a depth, and for a
// folder whether it is open), a file found, a line found, or a head.
type filesEntry struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	Depth *int   `json:"depth,omitempty"`
	Open  *bool  `json:"open,omitempty"`
	Note  string `json:"note,omitempty"`
	Right string `json:"right,omitempty"`
	Tone  string `json:"tone,omitempty"`
	Head  string `json:"head,omitempty"`
}

// filesLine is a line of the code: a line of the file with its number
// and, blamed, its commit and that one's age at the right; a line of a
// diff, added or deleted or the same, with its numbers in the old file
// and the new, under a hunk's head; or a gap that stands for lines left
// out and for a file that has none to show.
type filesLine struct {
	Old   int    `json:"old,omitempty"`
	New   int    `json:"new,omitempty"`
	Text  string `json:"text"`
	Kind  string `json:"kind,omitempty"`
	Right string `json:"right,omitempty"`
}

// filesItemOp is the change of a patch that puts an entry last in a list.
type filesItemOp struct {
	Op   string     `json:"op"`
	ID   string     `json:"id"`
	Item filesEntry `json:"item"`
}

// filesRow is a row of the tree: a folder or a file by its path from the
// top, and the id the window knows it by.
type filesRow struct {
	id     string
	path   string
	depth  int
	folder bool
	open   bool
}

// filesHit is a line a search in files found.
type filesHit struct {
	path string
	line int
	text string
}

// filesUnder is the path of a name in a folder, the top's being "".
func filesUnder(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

// filesByName orders names whatever their case, and those that differ
// only in it by their bytes.
func filesByName(a, b string) int {
	return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
}

// filesTree is the rows of the tree of paths: in each folder its folders
// and then its files, by name, and under a folder what is in it only
// where it is open. The top is always open. A path that ends in a slash
// is a folder with nothing in it.
func filesTree(paths []string, open map[string]bool) []filesRow {
	type folder struct{ folders, files []string }
	listed := map[string]*folder{"": {}}
	for _, path := range paths {
		in, rest := "", path
		for {
			name, under, deeper := strings.Cut(rest, "/")
			if !deeper {
				if name != "" {
					listed[in].files = append(listed[in].files, name)
				}
				break
			}
			sub := filesUnder(in, name)
			if listed[sub] == nil {
				listed[sub] = &folder{}
				listed[in].folders = append(listed[in].folders, name)
			}
			if !open[sub] {
				break
			}
			in, rest = sub, under
		}
	}
	var rows []filesRow
	var list func(in string, depth int)
	list = func(in string, depth int) {
		here := listed[in]
		slices.SortFunc(here.folders, filesByName)
		slices.SortFunc(here.files, filesByName)
		for _, name := range here.folders {
			path := filesUnder(in, name)
			rows = append(rows, filesRow{id: cutTo(path, gitWordsMost), path: path, depth: depth, folder: true, open: open[path]})
			if open[path] {
				list(path, depth+1)
			}
		}
		for _, name := range here.files {
			path := filesUnder(in, name)
			rows = append(rows, filesRow{id: cutTo(path, gitWordsMost), path: path, depth: depth})
		}
	}
	list("", 0)
	return rows
}

// filesOnTheWay is the folders on the way to each of paths, all open; and
// none of them where there are more than most.
func filesOnTheWay(paths []string, most int) map[string]bool {
	open := map[string]bool{}
	for _, path := range paths {
		for at := strings.IndexByte(path, '/'); at >= 0; {
			open[path[:at]] = true
			next := strings.IndexByte(path[at+1:], '/')
			if next < 0 {
				break
			}
			at += next + 1
		}
	}
	if len(open) > most {
		return map[string]bool{}
	}
	return open
}

// filesMark is the word and the tone a changed file carries: A for one
// added or untracked, M for one changed, D for one deleted.
func filesMark(mark byte) (word, tone string) {
	switch mark {
	case 'A':
		return "A", "ok"
	case 'M':
		return "M", "warn"
	case 'D':
		return "D", "bad"
	}
	return "", ""
}

// filesEntries is the tree as the list holds it: under a head with the
// repository's name and how many files changed, the first most rows, each
// by its name at its depth, a folder open or shut, a changed file with
// its mark; and a head that says how many rows more there are.
func filesEntries(rows []filesRow, marks map[string]byte, name, note string, most int) []filesEntry {
	entries := []filesEntry{{Head: cutTo(name, gitWordsMost), Note: note}}
	for _, row := range rows[:min(len(rows), most)] {
		entry := filesEntry{ID: row.id, Title: cutTo(filepath.Base(row.path), gitWordsMost), Depth: &row.depth}
		if row.folder {
			entry.Open = &row.open
		} else {
			entry.Right, entry.Tone = filesMark(marks[row.path])
		}
		entries = append(entries, entry)
	}
	if more := len(rows) - most; more > 0 {
		entries = append(entries, filesEntry{Head: counted(more, "more row"), Note: "shut a folder to see them"})
	}
	return entries
}

// filesHas reports whether typed's letters are all in words, in order.
func filesHas(words, typed string) bool {
	for _, letter := range typed {
		at := strings.IndexRune(words, letter)
		if at < 0 {
			return false
		}
		words = words[at+len(string(letter)):]
	}
	return true
}

// filesFound is the paths that have what is typed, its letters in order
// whatever their case, and how many there are: the best most of them
// first, those with the letters in the file's name before those with them
// only in its folders, then the shorter path, then by name.
func filesFound(paths []string, typed string, most int) ([]string, int) {
	typed = strings.ToLower(typed)
	type match struct {
		path   string
		inName bool
	}
	var found []match
	for _, path := range paths {
		lower := strings.ToLower(path)
		if filesHas(lower, typed) {
			found = append(found, match{path, filesHas(lower[strings.LastIndexByte(lower, '/')+1:], typed)})
		}
	}
	slices.SortFunc(found, func(a, b match) int {
		switch {
		case a.inName && !b.inName:
			return -1
		case b.inName && !a.inName:
			return 1
		}
		return cmp.Or(cmp.Compare(len(a.path), len(b.path)), filesByName(a.path, b.path))
	})
	best := make([]string, 0, min(len(found), most))
	for _, match := range found[:min(len(found), most)] {
		best = append(best, match.path)
	}
	return best, len(found)
}

// filesFoundEntries is the files found as the list holds them: each by
// its name, the folder it is in after it, and its mark.
func filesFoundEntries(found []string, marks map[string]byte) []filesEntry {
	entries := make([]filesEntry, 0, len(found))
	for _, path := range found {
		entry := filesEntry{ID: cutTo(path, gitWordsMost), Title: cutTo(filepath.Base(path), gitWordsMost)}
		if folder := filepath.Dir(path); folder != "." {
			entry.Note = cutTo(folder, gitWordsMost)
		}
		entry.Right, entry.Tone = filesMark(marks[path])
		entries = append(entries, entry)
	}
	return entries
}

// filesHitOf is a line git grep wrote as a hit: its path, its number and
// its text, without the room at its ends and no longer than a row shows.
func filesHitOf(line string) (filesHit, bool) {
	fields := strings.SplitN(line, "\x00", 3)
	if len(fields) < 3 {
		return filesHit{}, false
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil || fields[0] == "" {
		return filesHit{}, false
	}
	return filesHit{path: fields[0], line: n, text: cutTo(gitShown(strings.TrimSpace(fields[2])), filesHitMost)}, true
}

// filesHits is the hits of what git grep wrote, in its order.
func filesHits(out string) []filesHit {
	var hits []filesHit
	for _, line := range strings.Split(out, "\n") {
		if hit, ok := filesHitOf(line); ok {
			hits = append(hits, hit)
		}
	}
	return hits
}

// filesHitID is the id of the hit at a place among those found, and
// filesHitAt the place of an id.
func filesHitID(place int) string {
	return "h:" + strconv.Itoa(place)
}

func filesHitAt(id string) (int, bool) {
	number, isHit := strings.CutPrefix(id, "h:")
	place, err := strconv.Atoi(number)
	return place, isHit && err == nil
}

// filesGrouped is hits as the list holds them, by file: a head for each
// file with its path and how many lines of it were found, then each line
// by its text with its number at the right. first is the place of the
// first of them among all that were found.
func filesGrouped(hits []filesHit, first int) []filesEntry {
	var entries []filesEntry
	for i, hit := range hits {
		if i == 0 || hits[i-1].path != hit.path {
			n := 1
			for n < len(hits)-i && hits[i+n].path == hit.path {
				n++
			}
			head := hit.path
			if runes := []rune(head); len(runes) > gitWordsMost { // a long path is known by its end
				head = "…" + string(runes[len(runes)-gitWordsMost+1:])
			}
			entries = append(entries, filesEntry{Head: head, Note: strconv.Itoa(n)})
		}
		title := hit.text
		if title == "" {
			title = " "
		}
		entries = append(entries, filesEntry{ID: filesHitID(first + i), Title: title, Right: strconv.Itoa(hit.line)})
	}
	return entries
}

// filesFitted is the entries build makes of most things that a message
// has room for beside a file's lines: of fewer and fewer of them until
// they are under half of what a message may be.
func filesFitted(most int, build func(most int) []filesEntry) []filesEntry {
	for ; ; most /= 2 {
		if entries := build(most); most <= 1 || len(encode(entries)) < muiLimit/2 {
			return entries
		}
	}
}

// filesLines is a file as lines of code, each with its number and a tab
// as git's diff is shown: most of them from the line from on, a gap
// before them that says how many are above and one after that says how
// many more there are. A file with nothing in it, and one that is not
// text, is a gap that says so. With them, how many lines the file has.
func filesLines(content []byte, from, most int) ([]filesLine, int) {
	switch {
	case len(content) == 0:
		return []filesLine{{Kind: "gap", Text: "an empty file"}}, 0
	case bytes.IndexByte(content[:min(len(content), filesBinaryLook)], 0) >= 0:
		return []filesLine{{Kind: "gap", Text: "a binary file"}}, 0
	}
	all := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	from = min(max(from, 1), len(all))
	to := min(from-1+most, len(all))
	var lines []filesLine
	if from > 1 {
		lines = append(lines, filesLine{Kind: "gap", Text: counted(from-1, "line") + " above"})
	}
	for n := from; n <= to; n++ {
		lines = append(lines, filesLine{New: n, Text: gitShown(all[n-1])})
	}
	if more := len(all) - to; more > 0 {
		lines = append(lines, filesLine{Kind: "gap", Text: counted(more, "more line")})
	}
	return lines, len(all)
}

// filesCount is how many lines a file has: none where it is empty or is
// not text.
func filesCount(content []byte) int {
	switch {
	case len(content) == 0, bytes.IndexByte(content[:min(len(content), filesBinaryLook)], 0) >= 0:
		return 0
	case content[len(content)-1] == '\n':
		return bytes.Count(content, []byte("\n"))
	}
	return bytes.Count(content, []byte("\n")) + 1
}

// filesWindow is the first line said of a file of total lines, most of
// them at a time, so that a line is in the middle of them: the first
// line of the file where all of it is said.
func filesWindow(line, total, most int) int {
	return max(min(line-most/2, total-most+1), 1)
}

// filesTurned is the first line to say of a file once the keys are on a
// line of it, and whether that is another than from, the first said now:
// it is when the keys are within edge lines of the last line said and
// the file goes on after it, or of the first and the file begins before.
func filesTurned(line, from, total, most, edge int) (int, bool) {
	to := min(from+most-1, total)
	if line > to-edge && to < total || line < from+edge && from > 1 {
		next := filesWindow(line, total, most)
		return next, next != from
	}
	return from, false
}

// filesState is the words after a file's path: how it stands against
// what the marks are against, and how many lines it has, or which of
// them are said where that is not all of them.
func filesState(mark byte, against string, total, from, to int) string {
	var words []string
	switch mark {
	case 'A':
		words = append(words, "new against "+against)
	case 'M':
		words = append(words, "changed against "+against)
	case 'D':
		words = append(words, "deleted against "+against)
	}
	switch {
	case total > 0 && (from > 1 || to < total):
		words = append(words, fmt.Sprintf("lines %d–%d of %d", from, to, total))
	case total > 0:
		words = append(words, counted(total, "line"))
	}
	return strings.Join(words, " · ")
}

// filesNamedIn is the path a name typed is of, in a folder: by its
// steps from the worktree's top, and whether it names a folder, which a
// name that ends in a slash does. A name of nothing, one from the root,
// one that leaves the worktree and one in git's own folder are refused.
func filesNamedIn(folder, typed string) (named string, isFolder bool, err error) {
	typed = strings.TrimSpace(typed)
	named = path.Clean(path.Join(folder, typed))
	switch {
	case typed == "":
		return "", false, errors.New("no name was typed")
	case strings.HasPrefix(typed, "/"):
		return "", false, errors.New("a path from the worktree, not from /")
	case named == "." || named == ".." || strings.HasPrefix(named, "../"):
		return "", false, errors.New("that is outside the worktree")
	case named == ".git" || strings.HasPrefix(named, ".git/"):
		return "", false, errors.New("that is git's own folder")
	}
	return named, strings.HasSuffix(typed, "/"), nil
}

// filesGone is what goes with a delete: a file's lines, or the files
// under a folder, how many of them git tracks, and whether what it
// tracks is other than the last commit has it.
type filesGone struct {
	folder  bool
	lines   int
	files   int
	tracked int
	unsaved bool
}

// filesGoing is what D says before it deletes: what goes, whether git
// can bring it back, and that D pressed again does it.
func filesGoing(named string, gone filesGone, inside bool) string {
	what := "delete " + cutTo(named, gitWordsMost/2)
	switch {
	case gone.folder:
		what += " and its " + counted(gone.files, "file")
	case gone.lines > 0:
		what += ", " + counted(gone.lines, "line")
	}
	switch {
	case !inside:
		what += ", in no repository"
	case gone.tracked == 0:
		what += ", not in git"
	case gone.folder && gone.tracked < gone.files:
		what += fmt.Sprintf(", %d not in git", gone.files-gone.tracked)
	case gone.unsaved:
		what += ", tracked: git can bring back only what is committed"
	default:
		what += ", tracked: git can bring it back"
	}
	return what + " · D again to delete"
}

// filesCopied is what puts a path on the clipboard, written to the
// terminal: an OSC 52 string, and inside tmux in tmux's passthrough, each
// ESC of it doubled, as a message to the window is.
func filesCopied(named string, tmux bool) []byte {
	osc := "\033]52;c;" + base64.StdEncoding.EncodeToString([]byte(named)) + "\a"
	if tmux {
		osc = "\033Ptmux;" + strings.ReplaceAll(osc, "\033", "\033\033") + "\033\\"
	}
	return []byte(osc)
}

// filesSeeking is what is searched for, of the words typed, and how they
// are read: after a slash as a regular expression, after an equals sign
// as whole words, else as they are written. With them, how that is said.
func filesSeeking(typed string) (words string, how git.Match, said string) {
	if pattern, ok := strings.CutPrefix(typed, "/"); ok {
		return pattern, git.AsPattern, "as a pattern"
	}
	if whole, ok := strings.CutPrefix(typed, "="); ok {
		return whole, git.WholeWords, "whole words"
	}
	return typed, git.AsWritten, ""
}

// filesBlamedWord is the word at the right of a blamed line: its commit
// by a short name and how long ago that was written.
func filesBlamedWord(blamed git.Blamed, now time.Time) string {
	if blamed.Commit == "" {
		return "not committed"
	}
	return cutTo(blamed.Commit, 7) + " " + gitAge(now.Sub(blamed.When))
}

// filesDiffed is a diff's lines of code as the file's code holds them,
// no more than most of them, a gap after them saying how many more there
// are; with them how many lines are added and deleted in all.
func filesDiffed(code []gitLine, most int) (lines []filesLine, added, deleted int) {
	for n, line := range code {
		switch line.Kind {
		case "add":
			added++
		case "del":
			deleted++
		case "same":
			line.Kind = ""
		}
		if n < most {
			lines = append(lines, filesLine{Old: line.Old, New: line.New, Text: line.Text, Kind: line.Kind, Right: line.Right})
		}
	}
	if more := len(code) - most; more > 0 {
		lines = append(lines, filesLine{Kind: "gap", Text: counted(more, "more line")})
	}
	return lines, added, deleted
}

// filesWalk is the files under a folder of no repository, by their paths
// from it, the first most of them: no .git folder is looked in, and a
// folder that cannot be read is passed over.
func filesWalk(dir string, most int) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil && path == dir:
			return err
		case err != nil:
			return nil
		case entry.IsDir() && entry.Name() == ".git":
			return filepath.SkipDir
		case entry.IsDir():
			return nil
		case len(paths) == most:
			return filepath.SkipAll
		}
		if under, err := filepath.Rel(dir, path); err == nil {
			paths = append(paths, filepath.ToSlash(under))
		}
		return nil
	})
	return paths, err
}

// filesBatched is some of what a search found: the hits of whole files,
// and with the last of them how the search ended — at the most hits that
// are shown, or refused.
type filesBatched struct {
	hits   []filesHit
	done   bool
	more   bool
	failed string
}

// filesSearch is a search in files at work: what it finds comes on found
// a batch at a time, and stop ends it.
type filesSearch struct {
	found <-chan filesBatched
	stop  func()
}

// filesReadLine is the next line of a reader, without what of it is past
// what the reader holds at once.
func filesReadLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadSlice('\n')
	text := strings.TrimSuffix(string(line), "\n")
	for errors.Is(err, bufio.ErrBufferFull) {
		_, err = reader.ReadSlice('\n')
	}
	return text, err
}

// filesSearchFor begins a search of a folder's files for words. What git
// prints is said as it comes: the hits of the files it is done with, no
// more often than once every filesBatch, so a file's head is said with
// its count; the last batch says how it ended. At most hits it is stopped.
func filesSearchFor(dir, words string, how git.Match, repository bool, most int) (*filesSearch, error) {
	search, err := git.Grep(dir, words, how, repository)
	if err != nil {
		return nil, err
	}
	found, quit := make(chan filesBatched), make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(quit)
			search.Stop()
		})
	}
	go func() {
		defer func() { _ = search.Wait() }()
		send := func(batch filesBatched) bool {
			select {
			case found <- batch:
				return true
			case <-quit:
				return false
			}
		}
		reader := bufio.NewReaderSize(search.Lines, 64<<10)
		var pending []filesHit
		var last time.Time
		n := 0
		for {
			line, err := filesReadLine(reader)
			if hit, ok := filesHitOf(line); ok {
				if len(pending) > 0 && pending[len(pending)-1].path != hit.path && time.Since(last) >= filesBatch {
					if !send(filesBatched{hits: pending}) {
						return
					}
					pending, last = nil, time.Now()
				}
				pending = append(pending, hit)
				if n++; n == most {
					search.Stop()
					send(filesBatched{hits: pending, done: true, more: true})
					return
				}
			}
			if err != nil {
				break
			}
		}
		batch := filesBatched{hits: pending, done: true}
		if err := search.Wait(); err != nil {
			batch.failed = cutTo(err.Error(), gitWordsMost)
		}
		send(batch)
	}()
	return &filesSearch{found: found, stop: stop}, nil
}

// filesMode is what the left side is: the tree, the files a search by
// name finds, or the lines a search in files finds.
type filesMode int

const (
	filesInTree filesMode = iota
	filesInFind
	filesInGrep
)

// filesPlace is what a message says of where the person is: the row of
// the list the keys are on, the line of the code they are on, and what
// the field holds. What it does not say stays as the person has it.
type filesPlace uint8

const (
	filesKeysPut filesPlace = 1 << iota
	filesLinePut
	filesWordsPut
	filesAllPut = filesKeysPut | filesLinePut | filesWordsPut
)

// filesAsk is what the one-line field asks for: the name of a new file
// or folder, where a path is renamed to, or the line to go to.
type filesAsk int

const (
	filesAskNothing filesAsk = iota
	filesAskNew
	filesAskMove
	filesAskLine
)

// filesBlame is a file blamed: each of its lines with its commit, as it
// stood when the file was of this size and time.
type filesBlame struct {
	stamp string
	lines []git.Blamed
}

// filesCommit is a commit shown in place of the file: what is said of
// it, what it changed of the file as lines of code, and the line the
// keys were on, where they go back to.
type filesCommit struct {
	said  string
	lines []filesLine
	back  int
}

// filesView is a worktree's files as the window is shown them. top is dir
// itself where dir is in no repository.
type filesView struct {
	pane, dir string          // the pane it is drawn over, and the folder it is run in
	top       string          // the worktree's top folder
	name      string          // the repository's name
	inside    bool            // whether that is a repository's
	against   string          // what the marks are against, "" in no repository
	paths     []string        // every file, by its path from the top
	marks     map[string]byte // those changed, each with A, M or D
	open      map[string]bool // the folders the person opened; nil until the files are first read
	rows      []filesRow      // the tree's rows
	at        string          // the id of the row of the tree under the keys
	mode      filesMode       // what the left side is
	typed     string          // what the field holds
	found     []string        // the best of the files a search by name found
	matched   int             // how many it found in all
	foundAt   string          // the one of them under the keys
	words     string          // what was searched for in the files
	hits      []filesHit      // what that found so far
	hitAt     int             // the one of them under the keys, -1 for none
	search    *filesSearch    // the search at work, nil for none
	kept      string          // the file the tree showed, shown again back in it
	file      string          // the file at the right
	line      int             // the line of it a search found, 0 for none
	lines     []filesLine     // its lines of code
	total     int             // how many lines it has
	status    *muiStatus      // how a search stands, or how the last thing done went
	failed    string          // why the files could not be read, or the editor run
	fork      string          // the commit the marks are against
	made      []string        // the folders the person made, listed while nothing is in them
	only      bool            // the tree lists only what changed
	asking    filesAsk        // what the one-line field asks for
	asked     string          // what it asks about: the folder a new name is in, the path renamed
	answer    string          // what the field holds as it comes up
	focus     string          // the piece the keys were last put on
	back      string          // the piece they go back to when nothing is answered
	arming    string          // the path D was pressed on once
	window    int             // the first line said of the file, 0 until it is read
	on        int             // the line of the file under the keys, 0 for none
	diffing   bool            // a changed file is shown as its diff against the base
	diffed    bool            // the lines are such a diff
	added     int             // the lines it adds
	deleted   int             // and deletes
	blaming   bool            // each line says its commit
	blames    map[string]filesBlame
	commit    *filesCommit      // the commit shown in place of the file, nil for none
	how       string            // how the words searched for are read, "" as written
	handing   *filesDeed        // the file to hand to the editor next, nil for none
	tmux      bool              // the terminal is tmux's
	write     func(data []byte) // writes to the terminal
}

// shows puts a file at the right, from the line a search found, or from
// its first.
func (v *filesView) shows(file string, line int) {
	v.file, v.line, v.on, v.window, v.commit = file, line, line, 0, nil
}

// tree makes the rows of the tree: of every file, with the folders the
// person made, or of those changed alone.
func (v *filesView) tree() {
	paths := v.paths
	if v.only {
		paths = slices.DeleteFunc(slices.Clone(paths), func(named string) bool { return v.marks[named] == 0 })
	} else if len(v.made) > 0 {
		paths = slices.Clone(paths)
		for _, folder := range v.made {
			paths = append(paths, folder+"/")
		}
	}
	v.rows = filesTree(paths, v.open)
}

// full is a path of the worktree as the disk has it.
func (v *filesView) full(named string) string {
	return filepath.Join(v.top, filepath.FromSlash(named))
}

// row is the row of an id.
func (v *filesView) row(id string) (filesRow, bool) {
	at := slices.IndexFunc(v.rows, func(row filesRow) bool { return row.id == id })
	if at < 0 {
		return filesRow{}, false
	}
	return v.rows[at], true
}

// mark reads which files changed against what: the commit the branch left
// its base at (the base as mia git finds it), else the last commit, else
// nothing at all; and the untracked ones as added. A file deleted is
// listed with the others; one that is gone from disk and was not there
// at that commit either is not.
func (v *filesView) mark() error {
	commit, against := "HEAD", "the last commit"
	base := gitBase(v.top, git.CurrentBranch(v.top))
	if git.Head(v.top) == "" {
		empty, err := git.EmptyTree(v.top)
		if err != nil {
			return err
		}
		commit, against = empty, "nothing committed"
	} else if base != "" {
		if fork := git.MergeBase(v.top, base, "HEAD"); fork != "" {
			commit, against = fork, base
		}
	}
	v.against, v.fork = against, commit
	out, err := git.Against(v.top, commit)
	if err != nil {
		return err
	}
	untracked, err := git.Untracked(v.top)
	if err != nil {
		return err
	}
	v.marks = git.ParseAgainst(out)
	for _, path := range untracked {
		v.marks[path] = 'A'
	}
	missing, err := git.Missing(v.top)
	if err != nil {
		return err
	}
	v.paths = slices.DeleteFunc(v.paths, func(named string) bool { return v.marks[named] == 0 && slices.Contains(missing, named) })
	for path, mark := range v.marks {
		if mark == 'D' {
			v.paths = append(v.paths, path)
		}
	}
	slices.Sort(v.paths)
	v.paths = slices.Compact(v.paths)
	return nil
}

// read reads the files again, and reports whether the row of the tree
// under the keys is another than it was: the same one where it still is,
// else the one now in its place. The first time, the folders on the way
// to each changed file are opened, and the keys are on the first of them,
// else on the first file.
func (v *filesView) read() (moved bool) {
	before := v.at
	was := slices.IndexFunc(v.rows, func(row filesRow) bool { return row.id == v.at })
	v.top, v.inside, v.name, v.against, v.fork, v.paths, v.marks, v.failed = v.dir, false, filepath.Base(v.dir), "", "", nil, map[string]byte{}, ""
	top, err := git.Toplevel(v.dir)
	if err != nil { // in no repository: the folder itself
		v.paths, err = filesWalk(v.dir, filesWalkMost)
	} else {
		v.top, v.inside, v.name = top, true, filepath.Base(top)
		if common, err := git.CommonDir(top); err == nil && filepath.Base(common) == ".git" { // a worktree is of the repository it was made from
			v.name = filepath.Base(filepath.Dir(common))
		}
		if v.paths, err = git.Files(top); err == nil {
			err = v.mark()
		}
	}
	if err != nil {
		v.failed = cutTo(err.Error(), gitWordsMost)
	}
	first := v.open == nil
	if first {
		changed := make([]string, 0, len(v.marks))
		for path := range v.marks {
			changed = append(changed, path)
		}
		v.open = filesOnTheWay(changed, filesOpenMost)
	}
	v.made = slices.DeleteFunc(v.made, func(folder string) bool {
		in, err := os.ReadDir(v.full(folder))
		return err != nil || len(in) > 0
	})
	v.tree()
	if _, still := v.row(v.at); !still && len(v.rows) > 0 {
		v.at = v.rows[min(max(was, 0), len(v.rows)-1)].id
	} else if !still {
		v.at = ""
	}
	if first {
		isFile := func(row filesRow) bool { return !row.folder }
		at := slices.IndexFunc(v.rows, func(row filesRow) bool { return !row.folder && v.marks[row.path] != 0 })
		if at < 0 {
			at = slices.IndexFunc(v.rows, isFile)
		}
		if at >= 0 {
			v.at = v.rows[at].id
		}
	}
	switch v.mode {
	case filesInTree:
		if row, ok := v.row(v.at); ok && !row.folder && row.path != v.file {
			v.shows(row.path, 0)
		}
	case filesInFind:
		v.find(v.typed, v.foundAt)
	}
	v.load()
	return v.at != before
}

// find lists the files that have what is typed, with the keys on the one
// of them at, else on the first, which is then the file at the right.
func (v *filesView) find(typed, at string) {
	v.typed = typed
	v.found, v.matched = filesFound(v.paths, typed, filesFoundMost)
	switch {
	case slices.Contains(v.found, at):
		v.foundAt = at
	case len(v.found) > 0:
		v.foundAt = v.found[0]
		v.shows(v.foundAt, 0)
	default:
		v.foundAt = ""
	}
}

// load reads what is at the right: the commit shown in place of the
// file; a changed file's diff against the base, where that is asked for;
// else the file's lines, as many as the view has room for beside its
// list, with the line under the keys in the middle of them where the
// file has more, each with its commit where the file is blamed. A file
// that is not there, is no file, or is over filesFileMost is a gap that
// says so.
func (v *filesView) load() {
	v.lines, v.total, v.diffed, v.added, v.deleted = nil, 0, false, 0, 0
	switch {
	case v.file == "":
		return
	case v.commit != nil:
		v.lines = v.commit.lines
		return
	}
	most := filesLinesMost
	if v.mode == filesInGrep {
		most = filesHitLinesMost
	}
	gap := func(text string) { v.lines = []filesLine{{Kind: "gap", Text: text}} }
	if v.diffing && v.inside && v.marks[v.file] != 0 {
		code, err := v.differs()
		if err != nil {
			gap(cutTo(err.Error(), gitWordsMost))
			return
		}
		v.lines, v.added, v.deleted = filesDiffed(code, filesLinesMost)
		v.diffed = true
		return
	}
	info, err := os.Stat(v.full(v.file))
	switch {
	case err != nil && v.marks[v.file] == 'D':
		gap("deleted against " + v.against)
	case err != nil:
		gap("no longer there")
	case !info.Mode().IsRegular():
		gap("not a file to read")
	case info.Size() > filesFileMost:
		gap(fmt.Sprintf("a file of %d MB", info.Size()>>20))
	default:
		content, err := os.ReadFile(v.full(v.file))
		if err != nil {
			gap(cutTo(err.Error(), gitWordsMost))
			return
		}
		total := filesCount(content)
		if v.window == 0 {
			v.window = filesWindow(max(v.on, 1), total, most)
		}
		v.window = max(min(v.window, total-most+1), 1)
		v.lines, v.total = filesLines(content, v.window, most)
		if v.blaming && v.inside {
			v.blame(fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano()), time.Now())
		}
	}
}

// differs is the diff of the file at the right against the base, as
// lines of code: of a file git does not track, every line of it added.
func (v *filesView) differs() ([]gitLine, error) {
	files, err := git.DiffAgainst(v.top, v.fork, v.file)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		code, _ := gitCode(files)
		return code, nil
	}
	info, err := os.Stat(v.full(v.file))
	switch {
	case err != nil:
		return nil, err
	case info.Size() > filesFileMost:
		return []gitLine{{Kind: "gap", Text: fmt.Sprintf("a file of %d MB", info.Size()>>20)}}, nil
	}
	content, err := os.ReadFile(v.full(v.file))
	return gitAdded(content), err
}

// blame puts at the right of each line said its commit and that one's
// age, as git blame has them: asked once for a file as it stands, by its
// stamp. A file git blames no line of keeps its lines as they are.
func (v *filesView) blame(stamp string, now time.Time) {
	blamed, known := v.blames[v.file]
	if !known || blamed.stamp != stamp {
		lines, err := git.Blame(v.top, v.file)
		if err != nil {
			lines = nil
		}
		blamed = filesBlame{stamp: stamp, lines: lines}
		if v.blames == nil {
			v.blames = map[string]filesBlame{}
		}
		v.blames[v.file] = blamed
	}
	for at, line := range v.lines {
		if line.New > 0 && line.New <= len(blamed.lines) {
			v.lines[at].Right = filesBlamedWord(blamed.lines[line.New-1], now)
		}
	}
}

// state is the words after the path of what is at the right.
func (v *filesView) state() string {
	switch {
	case v.file == "":
		return "nothing to show"
	case v.commit != nil:
		return v.commit.said
	case v.diffed:
		words := "diff against " + v.against
		if lines := plusMinus(v.added, v.deleted); lines != "" {
			words += " · " + lines
		}
		return words
	}
	to := 0
	for _, line := range v.lines {
		to = max(to, line.New)
	}
	return filesState(v.marks[v.file], v.against, v.total, v.from(), to)
}

// from is the number of the first line of the file that is shown.
func (v *filesView) from() int {
	for _, line := range v.lines {
		if line.New > 0 {
			return line.New
		}
	}
	return 1
}

// lineAt is the number of the line of the file at a place in the code, a
// gap's being that of the line nearest over it.
func (v *filesView) lineAt(place int) int {
	for ; place >= 0; place-- {
		if place < len(v.lines) && v.lines[place].New > 0 {
			return v.lines[place].New
		}
	}
	return v.from()
}

// items is what the list of the left side holds, and the id of the one
// of them the keys are on.
func (v *filesView) items() (entries []filesEntry, at string) {
	switch v.mode {
	case filesInFind:
		return filesFitted(len(v.found), func(most int) []filesEntry { return filesFoundEntries(v.found[:min(len(v.found), most)], v.marks) }), cutTo(v.foundAt, gitWordsMost)
	case filesInGrep:
		if v.hitAt >= 0 {
			at = filesHitID(v.hitAt)
		}
		return filesFitted(len(v.hits), func(most int) []filesEntry { return filesGrouped(v.hits[:min(len(v.hits), most)], 0) }), at
	}
	note := "in no repository"
	switch {
	case v.against != "" && len(v.marks) == 0:
		note = "nothing changed against " + v.against
	case v.only:
		note = fmt.Sprintf("only the %d changed against %s", len(v.marks), v.against)
	case v.against != "":
		note = fmt.Sprintf("%d changed against %s", len(v.marks), v.against)
	}
	return filesFitted(filesRowsMost, func(most int) []filesEntry { return filesEntries(v.rows, v.marks, v.name, note, most) }), v.at
}

// keys is the keys the program takes: in the tree all of them, each
// that turns something on saying what it now turns off; over what a
// search found, and while something is asked or a commit shown, e and esc.
func (v *filesView) keys() []gitKey {
	edit := gitKey{"e", "edit in " + filepath.Base(editorWords()[0]), "edit"}
	switch {
	case v.asking != filesAskNothing:
		return []gitKey{{"esc", "never mind", "back"}}
	case v.commit != nil:
		return []gitKey{edit, {"esc", "back to the file", "back"}}
	case v.mode != filesInTree:
		return []gitKey{edit, {"esc", "back to the tree", "back"}}
	}
	only, diff, blame := "changed only", "diff against the base", "blame"
	if v.only {
		only = "all files"
	}
	if v.diffing {
		diff = "the whole file"
	}
	if v.blaming {
		blame = "no blame"
	}
	return []gitKey{edit, {"f", "find a file", "find"}, {"/", "find in files", "grep"},
		{"a", "new file or folder", "new"}, {"R", "rename or move", "move"}, {"D", "delete", "delete"}, {"y", "copy the path", "copy"},
		{"c", only, "only"}, {"d", diff, "diff"}, {"B", blame, "blame"}, {":", "go to line", "line"}, {"r", "ask again", "again"}}
}

// whole is the message that says the whole view with these lines of the
// file in it: the list of the left side — the tree, or a field over what
// a search found — and beside it the file under the keys, its path and
// how it stands over its lines. A name asked for is a field in place of
// the tree's head, and a line asked for one in place of the file's path.
func (v *filesView) whole(lines json.RawMessage, place filesPlace) []byte {
	entries, at := v.items()
	list := filesPiece{T: "list", ID: "tree", Live: true, Items: entries}
	left := filesPiece{T: "col", W: 300, Scroll: true, Pad: 6}
	field := filesPiece{T: "field", ID: "q", Bare: true, Icon: "search", Live: true}
	switch v.mode {
	case filesInFind:
		list.ID, field.Placeholder, field.Right = "found", "Find a file", counted(v.matched, "file")
	case filesInGrep:
		list.ID, field.Placeholder, left.W = "hits", "Find in files: /pattern, =whole words", 440
	}
	if v.mode != filesInTree {
		field.List = list.ID
		if place&filesWordsPut != 0 {
			field.Value = &v.typed
		}
		left.Children = []filesPiece{field, {T: "divider"}}
	}
	asked := filesPiece{T: "field", ID: "name", Bare: true}
	if place&filesWordsPut != 0 {
		asked.Value = &v.answer
	}
	switch v.asking {
	case filesAskNew:
		asked.Placeholder, asked.Right = "New file, or folder/", "in "+cutTo(filesUnder(v.asked, ""), gitWordsMost-3)
		if v.asked == "" {
			asked.Right = "in " + cutTo(v.name, gitWordsMost-3)
		}
	case filesAskMove:
		asked.Placeholder = "Rename or move to"
	case filesAskLine:
		asked.ID, asked.Placeholder = "line", fmt.Sprintf("Go to line, 1–%d", max(v.total, 1))
	}
	if v.asking == filesAskNew || v.asking == filesAskMove {
		left.Children, list.Items = []filesPiece{asked, {T: "divider"}}, entries[1:]
	}
	code := filesPiece{T: "code", ID: "file", Live: true, Lines: lines}
	if place&filesKeysPut != 0 {
		list.Selected = at
	}
	// (The line the keys are on, by its place: a window that puts the keys of
	// a code where it is told has them on it.)
	if at := slices.IndexFunc(v.lines, func(line filesLine) bool { return line.New == v.on }); place&filesLinePut != 0 && v.on > 0 && at >= 0 {
		code.Selected = strconv.Itoa(at)
	}
	left.Children = append(left.Children, list)

	var about []filesPiece
	if v.file != "" {
		about = append(about, filesPiece{T: "text", Text: cutTo(v.file, gitWordsMost), Style: "mono"})
	}
	if state := v.state(); state != "" {
		about = append(about, filesPiece{T: "text", Text: cutTo(state, gitWordsMost), Style: "dim"})
	}
	if v.asking == filesAskLine {
		about = []filesPiece{asked}
	}
	message := filesMessage{V: 1, Pane: v.pane, Fill: true, Focus: v.focus, Keys: v.keys(), Status: v.status, View: filesPiece{T: "row", Split: true, Children: []filesPiece{
		left,
		{T: "divider"},
		{T: "col", Children: []filesPiece{{T: "row", Pad: 10, Gap: 10, Children: about}, code}},
	}}}
	if v.failed != "" {
		message.Status = &muiStatus{Text: v.failed, Tone: "bad"}
	}
	return encode(message)
}

// messages is the messages that say the view: the whole of it with the
// first lines of the file that fit, then the rest of them as patches that
// put them at the code's end, each no more bytes than the window takes.
func (v *filesView) messages(place filesPlace) [][]byte {
	lines := make([]json.RawMessage, len(v.lines))
	for i, line := range v.lines {
		lines[i] = encode(line)
	}
	n := gitFit(lines, muiLimit-len(v.whole(nil, place))-patchEnvelope)
	var first json.RawMessage
	if n > 0 {
		first, _ = json.Marshal(lines[:n]) // each already in ASCII alone
	}
	all := [][]byte{v.whole(first, place)}
	for lines = lines[n:]; len(lines) > 0; lines = lines[n:] {
		n = max(gitFit(lines, muiLimit-2*patchEnvelope), 1)
		all = append(all, encode(muiMessage{V: 1, Pane: v.pane, Patch: []json.RawMessage{encode(gitOp{Op: "lines", ID: "file", Lines: lines[:n]})}}))
	}
	return all
}

// patches is hits just found as patches that put them last in the list,
// each file's under its head: each patch no more bytes and no more
// changes than the window takes. first is the place of the first of them.
func (v *filesView) patches(hits []filesHit, first int) [][]byte {
	var ops []json.RawMessage
	for _, entry := range filesGrouped(hits, first) {
		ops = append(ops, encode(filesItemOp{Op: "item", ID: "hits", Item: entry}))
	}
	var all [][]byte
	for len(ops) > 0 {
		n := min(max(gitFit(ops, muiLimit-2*patchEnvelope), 1), muiOps)
		all = append(all, encode(muiMessage{V: 1, Pane: v.pane, Patch: ops[:n]}))
		ops = ops[n:]
	}
	return all
}

// filesDo is what an event asks for.
type filesDo int

const (
	filesNothing filesDo = iota
	filesLeave
	filesOn        // the keys on a folder of the tree: nothing is read
	filesShow      // the file of another row at the right
	filesToggle    // a folder opened or shut
	filesEdit      // a file handed to the editor at a line
	filesFind      // the field that finds a file
	filesGrep      // the field that finds in files
	filesTyped     // the field's words as they now are
	filesSeek      // the files searched for words
	filesGo        // back to the tree with the keys on a file
	filesBack      // back to the tree as it was
	filesAgain     // read the files again
	filesOnLine    // another line of the file under the keys
	filesAsked     // the field that asks a name, or a line
	filesNever     // what is asked dropped, or the commit shown left
	filesNamed     // a file or a folder made, or a path renamed, as the field says
	filesToLine    // the file said from the line the field says
	filesDelete    // a path deleted: asked for once, and done the second time
	filesCopy      // a path put on the clipboard
	filesBlamed    // each line with its commit, or without again
	filesCommitted // the commit of a blamed line in place of the file
	filesOnly      // the tree of what changed alone, or of all again
	filesDiff      // a changed file as its diff against the base, or whole again
)

// filesDeed is what an event does: with the file's path and a line of
// it, the id of its row or of the piece the keys go back to, the words
// typed, or what is asked, as it needs.
type filesDeed struct {
	do   filesDo
	id   string
	path string
	line int
	text string
	ask  filesAsk
}

// hit is the hit of an id.
func (v *filesView) hit(id string) (filesHit, int, bool) {
	place, ok := filesHitAt(id)
	if !ok || place < 0 || place >= len(v.hits) {
		return filesHit{}, -1, false
	}
	return v.hits[place], place, true
}

// editing is the file and the line e edits, pressed on item: a line of
// the code (its place, a number) is that line of the file shown, a hit is
// its line, and a file of the tree or one found is its first line — or,
// where it is the file shown on a line a search found, that line. A
// folder is nothing to edit. (A file named by a number alone, at the top,
// is read as a line of the code while the code is that long: the file
// shown is then edited, which with the keys on that row is the same file.)
func (v *filesView) editing(item string) filesDeed {
	edit := func(path string, line int) filesDeed { return filesDeed{do: filesEdit, path: path, line: line} }
	shown := func(path string) filesDeed {
		if path == v.file && v.line > 0 {
			return edit(path, v.line)
		}
		return edit(path, 1)
	}
	if place, err := strconv.Atoi(item); err == nil && place >= 0 && place < len(v.lines) && v.file != "" {
		return edit(v.file, v.lineAt(place))
	}
	if hit, _, ok := v.hit(item); ok && v.mode == filesInGrep {
		return edit(hit.path, hit.line)
	}
	row, ok := v.row(item)
	found := slices.IndexFunc(v.found, func(path string) bool { return cutTo(path, gitWordsMost) == item })
	switch {
	case v.mode == filesInTree && ok && !row.folder:
		return shown(row.path)
	case v.mode == filesInTree && ok:
		return filesDeed{}
	case v.mode == filesInFind && found >= 0:
		return shown(v.found[found])
	case v.file != "":
		return shown(v.file)
	}
	return filesDeed{}
}

// under is the path a key pressed on item is about, whether it is a
// folder, and the piece the keys were on: a row of the tree is its path,
// and a line of the code, as nothing is, the file shown.
func (v *filesView) under(item string) (named string, folder bool, piece string, ok bool) {
	if place, err := strconv.Atoi(item); err == nil && place >= 0 && place < len(v.lines) && v.file != "" {
		return v.file, false, "file", true
	}
	if row, ok := v.row(item); ok {
		return row.path, row.folder, "tree", true
	}
	return v.file, false, "tree", v.file != ""
}

// pressed is what a key of the tree's own does, pressed on item: a asks
// the name of a new file in the folder under the keys, or in the folder
// of the file under them; R asks where the path under them goes; D
// deletes it and y copies it; : asks a line of the file shown; c, d and
// B turn the changed files alone, the diff and the blame on and off.
func (v *filesView) pressed(key, item string) filesDeed {
	named, folder, piece, ok := v.under(item)
	switch key {
	case "new":
		in := ""
		switch {
		case ok && folder:
			in = named
		case ok && path.Dir(named) != ".":
			in = path.Dir(named)
		}
		return filesDeed{do: filesAsked, ask: filesAskNew, path: in, id: piece}
	case "only":
		return filesDeed{do: filesOnly}
	}
	if !ok {
		return filesDeed{}
	}
	switch key {
	case "move":
		return filesDeed{do: filesAsked, ask: filesAskMove, path: named, id: piece}
	case "delete":
		return filesDeed{do: filesDelete, path: named}
	case "copy":
		return filesDeed{do: filesCopy, path: named}
	case "line":
		return filesDeed{do: filesAsked, ask: filesAskLine, id: "file"}
	case "diff":
		return filesDeed{do: filesDiff}
	case "blame":
		return filesDeed{do: filesBlamed}
	}
	return filesDeed{}
}

// deed is what an event does to the view as it stands. The row of a list
// come under the keys shows its file; ⏎ on a folder opens or shuts it, on
// a file found or a line found goes back to the tree with the keys on
// that file, and on a file of the tree does nothing more: its file is at
// the right already, and the keys cross to it with l. ⏎ in the field
// that finds in files searches for its words, and so does ⏎ on a line
// found for other words than the field now holds. A line of the file
// come under the keys is where they are, and ⏎ on one that is blamed
// shows its commit. While a name or a line is asked for, only what the
// field sends and esc do anything.
func (v *filesView) deed(event gitEvent) filesDeed {
	switch {
	case event.E == "close":
		return filesDeed{do: filesLeave}
	case event.E == "press" && event.ID == "back" && (v.asking != filesAskNothing || v.commit != nil):
		return filesDeed{do: filesNever}
	case v.asking != filesAskNothing:
		switch {
		case event.E == "submit" && event.ID == "name" && v.asking != filesAskLine:
			return filesDeed{do: filesNamed, text: event.Text}
		case event.E == "submit" && event.ID == "line" && v.asking == filesAskLine:
			return filesDeed{do: filesToLine, text: event.Text}
		}
		return filesDeed{}
	}
	switch event.E {
	case "move", "pick":
		switch event.List {
		case "file":
			place, err := strconv.Atoi(event.ID)
			if err != nil || place < 0 || place >= len(v.lines) {
				break
			}
			line := v.lineAt(place)
			switch {
			case event.E == "pick" && v.blaming && v.commit == nil && !v.diffed && v.lines[place].New > 0:
				return filesDeed{do: filesCommitted, line: line}
			case event.E == "move" && line != v.on:
				return filesDeed{do: filesOnLine, line: line}
			}
		case "tree":
			row, ok := v.row(event.ID)
			switch {
			case !ok || v.mode != filesInTree:
			case row.folder && event.E == "pick":
				return filesDeed{do: filesToggle, id: row.id, path: row.path}
			case row.folder && row.id != v.at:
				return filesDeed{do: filesOn, id: row.id}
			case !row.folder && (row.id != v.at || row.path != v.file):
				return filesDeed{do: filesShow, id: row.id, path: row.path}
			}
		case "found":
			at := slices.IndexFunc(v.found, func(path string) bool { return cutTo(path, gitWordsMost) == event.ID })
			switch {
			case at < 0 || v.mode != filesInFind:
			case event.E == "pick":
				return filesDeed{do: filesGo, path: v.found[at]}
			case v.found[at] != v.foundAt:
				return filesDeed{do: filesShow, path: v.found[at]}
			}
		case "hits":
			hit, place, ok := v.hit(event.ID)
			switch {
			case v.mode != filesInGrep:
			case event.E == "pick" && v.typed != v.words:
				return filesDeed{do: filesSeek, text: v.typed}
			case !ok:
			case event.E == "pick":
				return filesDeed{do: filesGo, path: hit.path, line: hit.line}
			case place != v.hitAt:
				return filesDeed{do: filesShow, id: event.ID, path: hit.path, line: hit.line}
			}
		}
	case "change":
		if event.ID == "q" && v.mode != filesInTree && event.Text != v.typed {
			return filesDeed{do: filesTyped, text: event.Text}
		}
	case "submit":
		if event.ID == "q" && v.mode == filesInGrep {
			return filesDeed{do: filesSeek, text: event.Text}
		}
	case "press":
		switch {
		case event.ID == "edit":
			return v.editing(event.Item)
		case event.ID == "back" && v.mode != filesInTree:
			return filesDeed{do: filesBack}
		case event.ID == "find" && v.mode == filesInTree:
			return filesDeed{do: filesFind}
		case event.ID == "grep" && v.mode == filesInTree:
			return filesDeed{do: filesGrep}
		case event.ID == "again" && v.mode == filesInTree:
			return filesDeed{do: filesAgain}
		case v.mode == filesInTree && v.commit == nil:
			return v.pressed(event.ID, event.Item)
		}
	}
	return filesDeed{}
}

// quiet stops the search at work.
func (v *filesView) quiet() {
	if v.search != nil {
		v.search.stop()
		v.search = nil
	}
}

// results is where the search at work says what it finds: nothing comes
// on it while none is.
func (v *filesView) results() <-chan filesBatched {
	if v.search == nil {
		return nil
	}
	return v.search.found
}

// forget drops what a search in files found, and the search.
func (v *filesView) forget() {
	v.quiet()
	v.words, v.hits, v.hitAt, v.status = "", nil, -1, nil
}

// act does a deed, and reports whether the view is to be said again, and
// what of where the person is it says: after a move not the row of the
// list, so the keys are not put back where they were a moment ago, only
// the line of the code a search found; the row when the list is another.
// What the tree said of the last thing done goes with the next, and with
// it a delete asked for once, unless the next is D again.
func (v *filesView) act(deed filesDeed) (say bool, place filesPlace) {
	stale := false
	if deed.do != filesNothing && deed.do != filesLeave && deed.do != filesEdit && v.mode == filesInTree && (v.status != nil || v.arming != "") {
		if deed.do != filesDelete {
			v.arming = ""
		}
		v.status, stale = nil, true
	}
	say, place = v.did(deed)
	return say || stale, place
}

// said is how the last thing done went, as the view's status.
func (v *filesView) said(text, tone string) {
	v.status = &muiStatus{Text: cutTo(text, gitWordsMost), Tone: tone}
}

// goes puts the keys of the tree on a path, the folders on the way to it
// open, and reports whether it is listed.
func (v *filesView) goes(named string, folder bool) bool {
	for on := range filesOnTheWay([]string{named}, math.MaxInt) {
		v.open[on] = true
	}
	v.read()
	if _, listed := v.row(cutTo(named, gitWordsMost)); !listed {
		return false
	}
	v.at = cutTo(named, gitWordsMost)
	if !folder {
		v.shows(named, 0)
		v.load()
	}
	return true
}

// make makes what a name typed is of, in the folder asked about: a
// folder, or a file with nothing in it, and the folders on the way to
// it. What is there already is left as it is.
func (v *filesView) make(typed string) (named string, folder bool, err error) {
	if named, folder, err = filesNamedIn(v.asked, typed); err != nil {
		return "", false, err
	}
	if _, err := os.Lstat(v.full(named)); err == nil {
		return "", false, fmt.Errorf("%s is there already", named)
	}
	if folder {
		return named, true, os.MkdirAll(v.full(named), 0o755)
	}
	if err = os.MkdirAll(filepath.Dir(v.full(named)), 0o755); err != nil {
		return "", false, err
	}
	file, err := os.OpenFile(v.full(named), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", false, err
	}
	return named, false, file.Close()
}

// move renames the path asked about to the one typed, by its steps from
// the top: with git mv where git tracks it, else on disk alone. A path
// that is there already is refused, but the same one in other capitals.
func (v *filesView) move(typed string) (to string, err error) {
	from := v.asked
	if to, _, err = filesNamedIn("", typed); err != nil {
		return "", err
	}
	was, err := os.Lstat(v.full(from))
	switch {
	case to == from:
		return "", errors.New("that is its name already")
	case err != nil:
		return "", fmt.Errorf("%s is not there to rename", from)
	}
	if there, err := os.Lstat(v.full(to)); err == nil && !os.SameFile(was, there) {
		return "", fmt.Errorf("%s is there already", to)
	}
	if err = os.MkdirAll(filepath.Dir(v.full(to)), 0o755); err != nil {
		return "", err
	}
	if v.inside {
		if tracked, _ := git.Tracked(v.top, from); len(tracked) > 0 {
			return to, git.Move(v.top, from, to)
		}
	}
	return to, os.Rename(v.full(from), v.full(to))
}

// gone is what would go with a path deleted, as the disk and git have it.
func (v *filesView) gone(named string, folder bool) filesGone {
	gone := filesGone{folder: folder}
	if folder {
		_ = filepath.WalkDir(v.full(named), func(_ string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				gone.files++
			}
			return nil
		})
	} else if content, err := os.ReadFile(v.full(named)); err == nil {
		gone.lines = filesCount(content)
	}
	if v.inside {
		tracked, _ := git.Tracked(v.top, named)
		gone.tracked, gone.unsaved = len(tracked), git.Unsaved(v.top, named)
	}
	return gone
}

// did does a deed to the view, and reports what act does of it.
func (v *filesView) did(deed filesDeed) (say bool, place filesPlace) {
	switch deed.do {
	case filesNothing, filesLeave, filesEdit:
		return false, 0
	case filesOn:
		v.at = deed.id
		return false, 0
	case filesOnLine:
		v.on = deed.line
		if v.commit != nil || v.diffed || v.total == 0 {
			return false, 0
		}
		most := filesLinesMost
		if v.mode == filesInGrep {
			most = filesHitLinesMost
		}
		next, turned := filesTurned(deed.line, v.window, v.total, most, filesEdge)
		if !turned {
			return false, 0
		}
		v.window = next
		v.load()
		return true, filesLinePut
	case filesAsked:
		v.asking, v.asked, v.answer, v.back, v.focus = deed.ask, deed.path, "", deed.id, "name"
		switch deed.ask {
		case filesAskMove:
			v.answer = deed.path
		case filesAskLine:
			v.focus = "line"
		}
		return true, filesWordsPut
	case filesNever:
		if v.asking != filesAskNothing {
			v.asking, v.focus = filesAskNothing, v.back
			return true, 0
		}
		v.on, v.commit, v.focus = v.commit.back, nil, "file"
		v.load()
		return true, filesLinePut
	case filesNamed:
		var named string
		var folder bool
		var err error
		if v.asking == filesAskMove {
			was := v.asked
			if named, err = v.move(deed.text); err == nil {
				info, statErr := os.Stat(v.full(named))
				folder = statErr == nil && info.IsDir()
				if v.open[was] {
					v.open[named] = true
				}
				if at := slices.Index(v.made, was); at >= 0 {
					v.made[at] = named
				}
			}
		} else if named, folder, err = v.make(deed.text); err == nil && folder {
			v.made, v.only = append(v.made, named), false
		}
		if err != nil {
			v.said(err.Error(), "bad")
			return true, 0
		}
		made := v.asking == filesAskNew
		v.asking, v.focus = filesAskNothing, "tree"
		switch listed := v.goes(named, folder); {
		case !listed:
			v.said(named+" is made, and not listed: git ignores it", "warn")
		case made:
			v.said("made "+named, "ok")
		default:
			v.said("now "+named, "ok")
		}
		if made && !folder {
			v.handing = &filesDeed{do: filesEdit, path: named, line: 1}
		}
		return true, filesKeysPut
	case filesToLine:
		line, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(deed.text), ":"))
		if err != nil || line < 1 {
			v.said(fmt.Sprintf("a line's number, 1 to %d", max(v.total, 1)), "bad")
			return true, 0
		}
		v.asking, v.focus, v.diffing = filesAskNothing, "file", false
		v.on, v.window = line, 0
		v.load()
		v.on = min(line, max(v.total, 1))
		return true, filesLinePut
	case filesDelete:
		info, err := os.Lstat(v.full(deed.path))
		switch {
		case err != nil:
			v.arming = ""
			v.said(deed.path+" is not there to delete", "bad")
			return true, 0
		case v.arming != deed.path:
			v.arming = deed.path
			v.said(filesGoing(deed.path, v.gone(deed.path, info.IsDir()), v.inside), "warn")
			return true, 0
		}
		v.arming = ""
		if _, _, err = filesNamedIn("", deed.path); err == nil {
			err = os.RemoveAll(v.full(deed.path))
		}
		if err != nil {
			v.said(err.Error(), "bad")
			return true, 0
		}
		place = v.again()
		v.said("deleted "+deed.path, "ok")
		return true, place
	case filesCopy:
		if v.write != nil {
			v.write(filesCopied(deed.path, v.tmux))
		}
		v.said("copied "+deed.path, "")
		return true, 0
	case filesBlamed:
		if v.blaming = !v.blaming; !v.blaming {
			v.load()
			return true, 0
		}
		v.diffing = false
		v.load()
		switch {
		case !v.inside:
			v.blaming = false
			v.said("in no repository: no line is of a commit", "warn")
		case !slices.ContainsFunc(v.lines, func(line filesLine) bool { return line.Right != "" }):
			v.said("git blames no line of "+v.file, "warn")
		}
		return true, 0
	case filesCommitted:
		blamed := v.blames[v.file].lines
		if deed.line < 1 || deed.line > len(blamed) {
			return false, 0
		}
		if blamed[deed.line-1].Commit == "" {
			v.said(fmt.Sprintf("line %d is not committed yet", deed.line), "")
			return true, 0
		}
		commit, err := git.Described(v.top, blamed[deed.line-1].Commit)
		var files []git.FileDiff
		if err == nil {
			files, err = git.ShownOf(v.top, blamed[deed.line-1].Commit, v.file)
		}
		if err != nil {
			v.said(err.Error(), "bad")
			return true, 0
		}
		code, _ := gitCode(files)
		if len(files) == 0 {
			code = []gitLine{{Kind: "gap", Text: "it changed no line of a file of this name"}}
		}
		lines, _, _ := filesDiffed(code, filesLinesMost)
		v.commit = &filesCommit{said: fmt.Sprintf("%s · %s · %s", commit.SHA, commit.When.Format("2006-01-02"), commit.Subject), lines: lines, back: deed.line}
		v.on, v.focus = deed.line, "file"
		v.load()
		return true, filesLinePut
	case filesOnly:
		switch {
		case !v.only && len(v.marks) == 0 && v.against != "":
			v.said("nothing changed against "+v.against, "")
			return true, 0
		case !v.only && len(v.marks) == 0:
			v.said("in no repository: nothing is changed against anything", "")
			return true, 0
		}
		if v.only = !v.only; v.only {
			for path := range v.marks {
				for on := range filesOnTheWay([]string{path}, math.MaxInt) {
					v.open[on] = true
				}
			}
		}
		return true, v.again()
	case filesDiff:
		switch {
		case v.diffing:
			v.diffing, v.window = false, 0
		case !v.inside:
			v.said("in no repository: nothing to diff against", "")
			return true, 0
		case v.marks[v.file] == 0:
			v.said(v.file+" is as "+v.against+" has it", "")
			return true, 0
		default:
			v.diffing, v.blaming = true, false
		}
		v.load()
		return true, 0
	case filesShow:
		switch v.mode {
		case filesInTree:
			v.at = deed.id
		case filesInFind:
			v.foundAt = deed.path
		case filesInGrep:
			_, v.hitAt, _ = v.hit(deed.id)
		}
		v.shows(deed.path, deed.line)
		v.load()
		return true, filesLinePut
	case filesToggle:
		v.at = deed.id
		v.open[deed.path] = !v.open[deed.path]
		v.tree()
		return true, 0
	case filesFind:
		v.mode, v.kept = filesInFind, v.file
		v.find("", "")
		v.load()
		return true, filesAllPut
	case filesGrep:
		v.mode, v.kept, v.typed = filesInGrep, v.file, ""
		v.forget()
		v.load()
		return true, filesAllPut
	case filesTyped:
		if v.mode == filesInFind {
			v.find(deed.text, "")
			v.load()
			return true, filesKeysPut
		}
		// (Other words than were searched for: what was found goes, so ⏎
		// searches for these and picks no line found for the others.)
		v.typed = deed.text
		if v.words == "" || deed.text == v.words {
			return false, 0
		}
		v.forget()
		return true, 0
	case filesSeek:
		v.typed = deed.text
		v.forget()
		if strings.TrimSpace(deed.text) == "" {
			return true, 0
		}
		v.words = deed.text
		words, how, said := filesSeeking(deed.text)
		if strings.TrimSpace(words) == "" {
			return true, 0
		}
		search, err := filesSearchFor(v.top, words, how, v.inside, filesHitsMost)
		if err != nil {
			v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
			return true, 0
		}
		v.search, v.how, v.status = search, said, &muiStatus{Text: "searching…"}
		if said != "" {
			v.status.Text = "searching " + said + "…"
		}
		return true, 0
	case filesGo:
		v.mode, v.typed = filesInTree, ""
		v.forget()
		for folder := range filesOnTheWay([]string{deed.path}, math.MaxInt) {
			v.open[folder] = true
		}
		if v.only && v.marks[deed.path] == 0 {
			v.only = false
		}
		v.tree()
		v.at = cutTo(deed.path, gitWordsMost)
		v.shows(deed.path, deed.line)
		v.load()
		return true, filesKeysPut | filesLinePut
	case filesBack:
		v.mode, v.typed = filesInTree, ""
		v.forget()
		v.shows(v.kept, 0)
		v.load()
		return true, filesKeysPut
	}
	return true, v.again()
}

// again reads the files again, and says where the keys are only where
// the row under them is another.
func (v *filesView) again() filesPlace {
	if v.read() {
		return filesKeysPut
	}
	return 0
}

// take takes in a batch of what the search found, and reports whether
// the view is to be said whole: with the first hits, which the keys go
// on and whose file is shown, and with the last, which say how it ended —
// "6 in 4 files", and that there are more where it was stopped at the
// most. The others are said as patches.
func (v *filesView) take(batch filesBatched) (whole bool, place filesPlace) {
	first := len(v.hits) == 0 && len(batch.hits) > 0
	v.hits = append(v.hits, batch.hits...)
	if first {
		v.hitAt = 0
		v.shows(v.hits[0].path, v.hits[0].line)
		v.load()
		place = filesKeysPut | filesLinePut
	}
	if !batch.done {
		return first, place
	}
	v.search = nil
	files := 0
	for i, hit := range v.hits {
		if i == 0 || v.hits[i-1].path != hit.path {
			files++
		}
	}
	switch {
	case batch.failed != "":
		v.status = &muiStatus{Text: batch.failed, Tone: "bad"}
	case len(v.hits) == 0:
		v.status = &muiStatus{Text: "nothing found"}
	case batch.more:
		v.status = &muiStatus{Text: fmt.Sprintf("the first %d, in %s: there are more", len(v.hits), counted(files, "file")), Tone: "warn"}
	default:
		v.status = &muiStatus{Text: fmt.Sprintf("%d in %s", len(v.hits), counted(files, "file"))}
	}
	if v.how != "" && batch.failed == "" {
		v.status.Text += " · " + v.how
	}
	return true, place
}

// run hands a file to the editor at a line, waits for it to leave and
// reads the files again: they may be others now. An editor that cannot
// be run is the view's status; one that left with an error of its own
// has said so itself.
func (v *filesView) run(deed filesDeed) {
	err := editorAt(v.top, deed.path, deed.line).Run()
	v.read()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		v.failed = cutTo(err.Error(), gitWordsMost)
	}
}

// changed is what changes when the files do, cheap to ask for: what mia
// git looks at, and the size and time of the file shown.
func (v *filesView) changed() string {
	mark := gitMark(v.dir, v.top)
	if info, err := os.Lstat(v.full(v.file)); err == nil && v.file != "" {
		mark += fmt.Sprintf("%d %d\n", info.Size(), info.ModTime().UnixNano())
	}
	return mark
}

// cmdFiles shows the files of the worktree it is run in, drawn by mia
// desk over its pane: the tree beside the file under the keys, a field
// that finds a file by its name and one that finds lines in the files,
// and e, which hands the file to the editor and takes the pane again when
// it leaves; and the keys that make a file or a folder, rename one,
// delete one, copy its path, show only what changed, a file's diff
// against the base and each line's commit, and go to a line. It reads
// the files again when the repository changes.
func cmdFiles(args []string) int {
	if len(args) > 0 {
		return usageErr(filesUsage)
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fail(errors.New("files reads what you do from a terminal: run it in a pane of mia desk"))
	}
	dir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	view := &filesView{pane: os.Getenv("TMUX_PANE"), dir: dir, hitAt: -1, tmux: os.Getenv("TMUX") != "",
		write: func(data []byte) { _, _ = os.Stdout.Write(data) }}
	view.read()
	defer view.quiet()

	stop, next := make(chan struct{}), make(chan struct{}, 1)
	defer close(stop)
	keys := muiKeysInTurn(stop, next)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	under := "mia files: the plain terminal. In mia desk the files of " + view.name + " are drawn over it; q or ⌃C leaves."
	say, shut, err := muiOpen(view.pane, under)
	if err != nil {
		return fail(err)
	}
	defer func() { shut() }()

	var shown []byte // what the view said last, but for where the person is
	show := func(place filesPlace) {
		shown = bytes.Join(view.messages(0), nil)
		say(view.messages(place)...)
	}
	show(filesAllPut)
	mark := view.changed()
	poll := time.NewTicker(gitPoll)
	defer poll.Stop()
	buffer := ""
	for {
		select {
		case <-signals:
			return exitOK
		case <-poll.C:
			now := view.changed()
			if now == mark {
				continue
			}
			mark = now
			if place := view.again(); !bytes.Equal(bytes.Join(view.messages(0), nil), shown) {
				show(place)
			}
		case batch := <-view.results():
			from := len(view.hits)
			if whole, place := view.take(batch); whole {
				show(place)
			} else {
				say(view.patches(batch.hits, from)...)
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
			again, place := sight, filesPlace(0)
			if sight { // the pane came into sight: said again whole
				place = filesAllPut
			}
			for i, event := range events {
				deed := view.deed(event)
				if deed.do != filesLeave && deed.do != filesEdit && !((deed.do == filesShow || deed.do == filesOnLine) && i+1 < len(events) && events[i+1].E == "move") {
					said, here := view.act(deed)
					again, place = again || said, place|here
					if view.handing == nil { // a file just made is handed to the editor at once
						continue
					}
					deed, view.handing = *view.handing, nil
				}
				switch {
				case deed.do == filesLeave:
					return exitOK
				case deed.do != filesEdit:
					continue // the keys held down: only where they came to rest is read
				default:
					shut()
					shut = func() {}
					view.run(deed)
					if !muiLent(signals) {
						return exitOK
					}
					if say, shut, err = muiOpen(view.pane, under); err != nil {
						shut = func() {}
						return fail(err)
					}
					mark = view.changed()
					again, place = true, filesAllPut
				}
			}
			if again {
				show(place)
			}
			next <- struct{}{}
		}
	}
}
