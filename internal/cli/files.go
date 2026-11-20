package cli

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"os/signal"
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
// folder in no repository, the lines sent before the one a search found,
// the characters kept of such a line, and how long after one batch of
// what a search found the next is said.
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
	filesBefore       = 8
	filesHitMost      = 160
	filesBatch        = 100 * time.Millisecond
)

// filesMessage is the whole view said to the window: a UI that fills its
// pane, and the keys the program takes.
type filesMessage struct {
	V      int        `json:"v"`
	Pane   string     `json:"pane,omitempty"`
	Fill   bool       `json:"fill"`
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

// filesLine is a line of the code: a line of the file with its number, or
// a gap that stands for lines left out and for a file that has none to
// show.
type filesLine struct {
	New  int    `json:"new,omitempty"`
	Text string `json:"text"`
	Kind string `json:"kind,omitempty"`
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
// where it is open. The top is always open.
func filesTree(paths []string, open map[string]bool) []filesRow {
	type folder struct{ folders, files []string }
	listed := map[string]*folder{"": {}}
	for _, path := range paths {
		in, rest := "", path
		for {
			name, under, deeper := strings.Cut(rest, "/")
			if !deeper {
				listed[in].files = append(listed[in].files, name)
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

// filesState is the words after a file's path: how it stands against
// what the marks are against, how many lines it has, and the line the
// code begins at where that is not its first.
func filesState(mark byte, against string, total, from int) string {
	var words []string
	switch mark {
	case 'A':
		words = append(words, "new against "+against)
	case 'M':
		words = append(words, "changed against "+against)
	case 'D':
		words = append(words, "deleted against "+against)
	}
	if total > 0 {
		words = append(words, counted(total, "line"))
	}
	if from > 1 {
		words = append(words, fmt.Sprintf("from line %d", from))
	}
	return strings.Join(words, " · ")
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

// filesEditorWords is the editor's command: $EDITOR, else nvim.
func filesEditorWords() []string {
	if words := strings.Fields(os.Getenv("EDITOR")); len(words) > 0 {
		return words
	}
	return []string{"nvim"}
}

// filesEditor is the command that edits a file at a line: the editor
// with +<line> and the file's path from the worktree's top, run there
// with this terminal as its own.
func filesEditor(top, path string, line int) *exec.Cmd {
	words := filesEditorWords()
	if strings.HasPrefix(path, "-") || strings.HasPrefix(path, "+") { // a path, not an option
		path = "./" + path
	}
	command := exec.Command(words[0], append(words[1:], fmt.Sprintf("+%d", line), path)...)
	command.Dir = top
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command
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
func filesSearchFor(dir, words string, repository bool, most int) (*filesSearch, error) {
	search, err := git.Grep(dir, words, repository)
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
	status    *muiStatus      // how a search stands
	failed    string          // why the files could not be read, or the editor run
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
// listed with the others.
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
	v.against = against
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
	v.top, v.inside, v.name, v.against, v.paths, v.marks, v.failed = v.dir, false, filepath.Base(v.dir), "", nil, map[string]byte{}, ""
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
	v.rows = filesTree(v.paths, v.open)
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
			v.file, v.line = row.path, 0
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
		v.file, v.line = v.foundAt, 0
	default:
		v.foundAt = ""
	}
}

// load reads the file at the right: its lines from the first, or from a
// few before the one a search found, as many as the view has room for
// beside its list. One that is not there, is no file, or is over
// filesFileMost is a gap that says so.
func (v *filesView) load() {
	v.lines, v.total = nil, 0
	if v.file == "" {
		return
	}
	most := filesLinesMost
	if v.mode == filesInGrep {
		most = filesHitLinesMost
	}
	gap := func(text string) { v.lines = []filesLine{{Kind: "gap", Text: text}} }
	info, err := os.Stat(filepath.Join(v.top, v.file))
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
		content, err := os.ReadFile(filepath.Join(v.top, v.file))
		if err != nil {
			gap(cutTo(err.Error(), gitWordsMost))
			return
		}
		v.lines, v.total = filesLines(content, v.line-filesBefore, most)
	}
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
	case v.against != "":
		note = fmt.Sprintf("%d changed against %s", len(v.marks), v.against)
	}
	return filesFitted(filesRowsMost, func(most int) []filesEntry { return filesEntries(v.rows, v.marks, v.name, note, most) }), v.at
}

// whole is the message that says the whole view with these lines of the
// file in it: the list of the left side — the tree, or a field over what
// a search found — and beside it the file under the keys, its path and
// how it stands over its lines.
func (v *filesView) whole(lines json.RawMessage, place filesPlace) []byte {
	entries, at := v.items()
	list := filesPiece{T: "list", ID: "tree", Live: true, Items: entries}
	left := filesPiece{T: "col", W: 300, Scroll: true, Pad: 6}
	field := filesPiece{T: "field", ID: "q", Bare: true, Icon: "search", Live: true}
	edit := gitKey{"e", "edit in " + filepath.Base(filesEditorWords()[0]), "edit"}
	keys := []gitKey{edit, {"f", "find a file", "find"}, {"/", "find in files", "grep"}, {"r", "ask again", "again"}}
	switch v.mode {
	case filesInFind:
		list.ID, field.Placeholder, field.Right = "found", "Find a file", counted(v.matched, "file")
	case filesInGrep:
		list.ID, field.Placeholder, left.W = "hits", "Find in files", 440
	}
	if v.mode != filesInTree {
		field.List = list.ID
		if place&filesWordsPut != 0 {
			field.Value = &v.typed
		}
		left.Children = []filesPiece{field, {T: "divider"}}
		keys = []gitKey{edit, {"esc", "back to the tree", "back"}}
	}
	code := filesPiece{T: "code", ID: "file", Live: true, Lines: lines}
	if place&filesKeysPut != 0 {
		list.Selected = at
	}
	// (The line a search found, by its place: a window that puts the keys of
	// a code where it is told has them on it.)
	if at := slices.IndexFunc(v.lines, func(line filesLine) bool { return line.New == v.line }); place&filesLinePut != 0 && v.line > 0 && at >= 0 {
		code.Selected = strconv.Itoa(at)
	}
	left.Children = append(left.Children, list)

	var about []filesPiece
	if v.file != "" {
		about = append(about, filesPiece{T: "text", Text: cutTo(v.file, gitWordsMost), Style: "mono"})
	}
	state := filesState(v.marks[v.file], v.against, v.total, v.from())
	if v.file == "" {
		state = "nothing to show"
	}
	if state != "" {
		about = append(about, filesPiece{T: "text", Text: state, Style: "dim"})
	}
	message := filesMessage{V: 1, Pane: v.pane, Fill: true, Keys: keys, Status: v.status, View: filesPiece{T: "row", Split: true, Children: []filesPiece{
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
	filesOn     // the keys on a folder of the tree: nothing is read
	filesShow   // the file of another row at the right
	filesToggle // a folder opened or shut
	filesEdit   // a file handed to the editor at a line
	filesFind   // the field that finds a file
	filesGrep   // the field that finds in files
	filesTyped  // the field's words as they now are
	filesSeek   // the files searched for words
	filesGo     // back to the tree with the keys on a file
	filesBack   // back to the tree as it was
	filesAgain  // read the files again
)

// filesDeed is what an event does: with the file's path and a line of
// it, the id of its row, or the words typed, as it needs.
type filesDeed struct {
	do   filesDo
	id   string
	path string
	line int
	text string
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

// deed is what an event does to the view as it stands. The row of a list
// come under the keys shows its file; ⏎ on a folder opens or shuts it, on
// a file found or a line found goes back to the tree with the keys on
// that file, and on a file of the tree does nothing more: its file is at
// the right already, and the keys cross to it with l. ⏎ in the field
// that finds in files searches for its words, and so does ⏎ on a line
// found for other words than the field now holds.
func (v *filesView) deed(event gitEvent) filesDeed {
	switch event.E {
	case "close":
		return filesDeed{do: filesLeave}
	case "move", "pick":
		switch event.List {
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
func (v *filesView) act(deed filesDeed) (say bool, place filesPlace) {
	switch deed.do {
	case filesNothing, filesLeave, filesEdit:
		return false, 0
	case filesOn:
		v.at = deed.id
		return false, 0
	case filesShow:
		switch v.mode {
		case filesInTree:
			v.at = deed.id
		case filesInFind:
			v.foundAt = deed.path
		case filesInGrep:
			_, v.hitAt, _ = v.hit(deed.id)
		}
		v.file, v.line = deed.path, deed.line
		v.load()
		return true, filesLinePut
	case filesToggle:
		v.at = deed.id
		v.open[deed.path] = !v.open[deed.path]
		v.rows = filesTree(v.paths, v.open)
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
		search, err := filesSearchFor(v.top, deed.text, v.inside, filesHitsMost)
		if err != nil {
			v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
			return true, 0
		}
		v.search, v.status = search, &muiStatus{Text: "searching…"}
		return true, 0
	case filesGo:
		v.mode, v.typed = filesInTree, ""
		v.forget()
		for folder := range filesOnTheWay([]string{deed.path}, math.MaxInt) {
			v.open[folder] = true
		}
		v.rows = filesTree(v.paths, v.open)
		v.at, v.file, v.line = cutTo(deed.path, gitWordsMost), deed.path, deed.line
		v.load()
		return true, filesKeysPut | filesLinePut
	case filesBack:
		v.mode, v.typed = filesInTree, ""
		v.forget()
		v.file, v.line = v.kept, 0
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
		v.hitAt, v.file, v.line = 0, v.hits[0].path, v.hits[0].line
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
	return true, place
}

// run hands a file to the editor at a line, waits for it to leave and
// reads the files again: they may be others now. An editor that cannot
// be run is the view's status; one that left with an error of its own
// has said so itself.
func (v *filesView) run(deed filesDeed) {
	err := filesEditor(v.top, deed.path, deed.line).Run()
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
	if info, err := os.Lstat(filepath.Join(v.top, v.file)); err == nil && v.file != "" {
		mark += fmt.Sprintf("%d %d\n", info.Size(), info.ModTime().UnixNano())
	}
	return mark
}

// filesKeys is what is typed into the terminal, a read at a time, until
// stop. The next read is begun only once next says the last was dealt
// with: while an editor has the terminal, nothing typed to it is read
// here. It is closed when the terminal ends.
func filesKeys(stop, next <-chan struct{}) <-chan string {
	keys := make(chan string)
	go func() {
		data := make([]byte, 64<<10)
		for {
			n, err := os.Stdin.Read(data)
			if n > 0 {
				select {
				case keys <- string(data[:n]):
				case <-stop:
					return
				}
			}
			if err != nil {
				close(keys)
				return
			}
			if n > 0 {
				select {
				case <-next:
				case <-stop:
					return
				}
			}
		}
	}()
	return keys
}

// cmdFiles shows the files of the worktree it is run in, drawn by mia
// desk over its pane: the tree beside the file under the keys, a field
// that finds a file by its name and one that finds lines in the files,
// and e, which hands the file to the editor and takes the pane again when
// it leaves. It writes nothing; it reads the files again when the
// repository changes.
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
	view := &filesView{pane: os.Getenv("TMUX_PANE"), dir: dir, hitAt: -1}
	view.read()
	defer view.quiet()

	stop, next := make(chan struct{}), make(chan struct{}, 1)
	defer close(stop)
	keys := filesKeys(stop, next)
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
				switch {
				case deed.do == filesLeave:
					return exitOK
				case deed.do == filesShow && i+1 < len(events) && events[i+1].E == "move":
					continue // the keys held down: only where they came to rest is read
				case deed.do == filesEdit:
					shut()
					shut = func() {}
					view.run(deed)
					for waiting := true; waiting; { // ⌃C in an editor that takes it as a signal was the editor's
						select {
						case signal := <-signals:
							if signal != syscall.SIGINT {
								return exitOK
							}
						default:
							waiting = false
						}
					}
					if say, shut, err = muiOpen(view.pane, under); err != nil {
						shut = func() {}
						return fail(err)
					}
					mark = view.changed()
					again, place = true, filesAllPut
					continue
				}
				said, here := view.act(deed)
				again, place = again || said, place|here
			}
			if again {
				show(place)
			}
			next <- struct{}{}
		}
	}
}
