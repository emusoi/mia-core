package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/emusoi/mia-core/internal/git"
)

const reviewUsage = "mia review [<range> | pr <number> | w]"

// What a view of a review holds at most, all of it under the pieces the
// window takes of a UI (muiPieces): the rows of its list of files, with
// the lines of a file's diff (gitLinesMost); and the characters said of
// sawa's summary, of a finding's words and of what is typed under one.
const (
	reviewRowsMost    = 400
	reviewSummaryMost = 400
	reviewNoteMost    = 8000
	reviewAreaMost    = 4000
)

// sawaRun runs sw with these arguments and gives what it printed. It is
// the one place mia meets sawa: a test hands the view another.
type sawaRun func(args ...string) ([]byte, error)

// sawaAnswer is what every verb of sw prints with --json: its data, or
// why it refused and what to do about it.
type sawaAnswer struct {
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Fix     string          `json:"fix"`
}

// sawaState is what sw state says of a review, of which mia reads this.
type sawaState struct {
	Slug    string        `json:"slug"`
	Label   string        `json:"label"`
	Store   string        `json:"store"`
	Target  sawaTarget    `json:"target"`
	Summary string        `json:"summary"`
	Files   []sawaFile    `json:"files"`
	Threads []sawaFinding `json:"threads"`
	Path    []sawaStop    `json:"path"`
	Verdict sawaVerdict   `json:"verdict"`
	Counts  struct {
		Added   int `json:"added"`
		Removed int `json:"removed"`
	} `json:"counts"`
}

// sawaTarget is what a review is of: the working tree, a range of
// commits, or a pull request.
type sawaTarget struct {
	Kind   string `json:"kind"`
	Revset string `json:"revset"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

type sawaFile struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Read    bool   `json:"read"`
	Stale   bool   `json:"stale"` // read, and changed since
}

// sawaFinding is a finding: where it is, what kind and how bad, how it
// stands, and what was said, the finding itself first.
type sawaFinding struct {
	ID       string     `json:"id"`
	File     string     `json:"file"`
	Line     int        `json:"line"`
	Side     string     `json:"side"`
	Type     string     `json:"type"`
	Severity string     `json:"severity"`
	State    string     `json:"state"`
	Posts    []sawaPost `json:"posts"`
}

type sawaPost struct {
	Author string    `json:"author"`
	At     time.Time `json:"at"`
	Body   string    `json:"body"`
}

// sawaStop is a stop of sawa's reading path: a place, and why to look.
type sawaStop struct {
	File string `json:"file"`
	Why  string `json:"why"`
}

type sawaVerdict struct {
	Verdict string `json:"verdict"`
	Label   string `json:"label"`
}

// sawaListed is a review as sw list says it.
type sawaListed struct {
	Slug      string     `json:"slug"`
	Kind      string     `json:"kind"`
	Target    sawaTarget `json:"target"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// sawaRows is a file as sw rows says it: its lines, each with the
// findings filed on it.
type sawaRows struct {
	File string    `json:"file"`
	Rows []sawaRow `json:"rows"`
}

type sawaRow struct {
	Kind    string   `json:"kind"` // add, del or context
	Old     int      `json:"old"`
	New     int      `json:"new"`
	Text    string   `json:"text"`
	Threads []string `json:"threads"`
}

// sawaSaid is what sw printed, read: its data, and as an error what it
// refused with, said as the person is to read it.
func sawaSaid(out []byte, err error) (json.RawMessage, error) {
	var answer sawaAnswer
	read := json.Unmarshal(out, &answer)
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return nil, errors.New("sw is not on PATH: sawa keeps the review, mia review draws it")
	case read != nil && err != nil:
		return nil, err
	case read != nil:
		return nil, errors.New("sw said what mia cannot read")
	case !answer.OK && answer.Fix != "":
		return nil, errors.New(answer.Message + ": " + answer.Fix)
	case !answer.OK:
		return nil, errors.New(answer.Message)
	}
	return answer.Data, nil
}

// sawaAsk runs one verb of sw and reads its data as a T.
func sawaAsk[T any](run sawaRun, args ...string) (T, error) {
	var data T
	raw, err := sawaSaid(run(args...))
	if err == nil && json.Unmarshal(raw, &data) != nil {
		err = errors.New("sw " + args[0] + " said what mia cannot read")
	}
	return data, err
}

// sawaEnv is the environment sw is run in: this one, without what would
// make sw take the person for an agent, sign for them with an agent's
// name, or hold the review to a moment an agent's task began at.
func sawaEnv(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(pair string) bool {
		name, _, _ := strings.Cut(pair, "=")
		return slices.Contains([]string{"SAWA_AGENT", "LOCO_AGENT", "SAWA_AUTHOR", "SAWA_SNAPSHOT"}, name)
	})
}

// sawaIn is sw run in a folder, as the person.
func sawaIn(dir string) sawaRun {
	return func(args ...string) ([]byte, error) {
		command := exec.Command("sw", args...)
		command.Dir, command.Env = dir, sawaEnv(os.Environ())
		return command.Output()
	}
}

// reviewOpenArgs is sw open for a target as sw nvim takes one: pr and a
// number, w for the working tree, anything else a range of commits.
func reviewOpenArgs(target string) []string {
	words := strings.Fields(target)
	switch {
	case len(words) == 0, words[0] == "w", words[0] == "-w":
		return []string{"open", "--json"}
	case words[0] == "pr":
		return []string{"open", "--pr", strings.Join(words[1:], ""), "--json"}
	}
	return []string{"open", "-r", strings.Join(words, " "), "--json"}
}

// reviewAgainArgs is sw open for what a review is of already: sawa then
// looks at the change again, and keeps what was said.
func reviewAgainArgs(target sawaTarget) []string {
	switch target.Kind {
	case "pr":
		return []string{"open", "--pr", strconv.Itoa(target.Number), "--json"}
	case "range":
		return []string{"open", "-r", target.Revset, "--json"}
	}
	return []string{"open", "-w", "--json"}
}

// reviewHas is the review sawa already has for this worktree's branch,
// of those sw list says are this worktree's: the one of the branch
// against its base, else the one last written to. A pull request's is
// every worktree's, so none is taken for this one's.
func reviewHas(listed []sawaListed, base string) string {
	slug, at := "", time.Time{}
	for _, review := range listed {
		switch {
		case review.Kind == "pr":
		case review.Kind == "range" && base != "" && review.Target.Revset == base+"..HEAD":
			return review.Slug
		case slug == "" || review.UpdatedAt.After(at):
			slug, at = review.Slug, review.UpdatedAt
		}
	}
	return slug
}

// stands is whether a finding still asks something of someone: one
// resolved, or closed without a fix, does not.
func (f sawaFinding) stands() bool {
	return f.State != "resolved" && f.State != "wontfix"
}

// first is the finding itself, and by is who filed it.
func (f sawaFinding) first() sawaPost {
	if len(f.Posts) == 0 {
		return sawaPost{}
	}
	return f.Posts[0]
}

// reviewMessage is the whole view said to the window: a UI that fills
// its pane, the piece the keys are put on, and the keys the program takes.
type reviewMessage struct {
	V      int         `json:"v"`
	Pane   string      `json:"pane,omitempty"`
	Fill   bool        `json:"fill"`
	Focus  string      `json:"focus,omitempty"`
	Status *muiStatus  `json:"status,omitempty"`
	View   reviewPiece `json:"view"`
	Keys   []gitKey    `json:"keys"`
}

// reviewPiece is a piece of the UI, of the kind T says, with what that
// kind takes. Lines is a code's lines, or the number of lines tall a text
// area is: the window has the one name for both.
type reviewPiece struct {
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
	Items       []reviewEntry   `json:"items,omitempty"`
	Lines       json.RawMessage `json:"lines,omitempty"`
	Label       string          `json:"label,omitempty"`
	Key         string          `json:"key,omitempty"`
	Primary     bool            `json:"primary,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Value       *string         `json:"value,omitempty"`
	Children    []reviewPiece   `json:"children,omitempty"`
}

// reviewEntry is what a list holds: the head over the files, a folder
// (open, at depth 0), a file, or a verdict to pick.
type reviewEntry struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Note    string `json:"note,omitempty"`
	Right   string `json:"right,omitempty"`
	Depth   int    `json:"depth,omitempty"`
	Open    bool   `json:"open,omitempty"`
	Checked bool   `json:"checked,omitempty"`
	Head    string `json:"head,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

// reviewLine is a line of the code: a hunk's head, a line added, deleted
// or the same (no kind) with its numbers, a gap, or a note: a finding
// under the line it is about, with its label in its tone, who filed it
// at the right and, when it is open, the pieces under its words.
type reviewLine struct {
	Kind  string        `json:"kind,omitempty"`
	Text  string        `json:"text"`
	Old   int           `json:"old,omitempty"`
	New   int           `json:"new,omitempty"`
	Right string        `json:"right,omitempty"`
	Tone  string        `json:"tone,omitempty"`
	Label string        `json:"label,omitempty"`
	Under []reviewPiece `json:"under,omitempty"`
}

// reviewMark is what a line of the code is, apart from its place, so the
// keys stay on it when lines come in over it: a line of the file by its
// kind and numbers, a head or a gap by the line after it, a note by its
// finding. The note of a comment being written is reviewNew.
type reviewMark struct {
	kind     string
	old, new int
	note     string
}

const reviewNew = "+"

// reviewRow is a file in the list: the id the window knows it by, and
// its path.
type reviewRow struct{ id, path string }

// reviewID is the id of a file in the list: its path, which a path of
// digits alone is not, since a line of the code is known by its place.
func reviewID(file string) string {
	if _, err := strconv.Atoi(file); err == nil {
		return "f:" + file
	}
	return cutTo(file, gitWordsMost)
}

// reviewAge is how long ago, as a review says it: "just now", "4 min
// ago", "2 h ago", "3 d ago".
func reviewAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d d ago", int(d.Hours()/24))
}

// reviewOrder is the files of a review in the order they are listed: by
// folder, the files of the top folder first; or in reading order, by
// their first stop on sawa's path, the files it does not pass after
// them. With them in reading order, why each is read.
func reviewOrder(s sawaState, reading bool) ([]sawaFile, map[string]string) {
	files := slices.Clone(s.Files)
	slices.SortStableFunc(files, func(a, b sawaFile) int {
		if in := strings.Compare(path.Dir(a.Path), path.Dir(b.Path)); in != 0 {
			return in
		}
		return strings.Compare(a.Path, b.Path)
	})
	if !reading {
		return files, nil
	}
	why, stop := map[string]string{}, map[string]int{}
	for n, at := range s.Path {
		if _, passed := stop[at.File]; !passed {
			why[at.File], stop[at.File] = at.Why, n
		}
	}
	slices.SortStableFunc(files, func(a, b sawaFile) int {
		at := func(file string) int {
			if n, passed := stop[file]; passed {
				return n
			}
			return len(s.Path)
		}
		return at(a.Path) - at(b.Path)
	})
	return files, why
}

// reviewOpen is how many findings still stand, on each file and in all.
func reviewOpen(s sawaState) (map[string]int, int) {
	on, all := map[string]int{}, 0
	for _, finding := range s.Threads {
		if finding.stands() {
			on[finding.File]++
			all++
		}
	}
	return on, all
}

// reviewList is the list of a review's files, no more than most of them:
// by folder, each folder open over its files, a file with its lines
// added and deleted at the right, its findings that stand as its note
// and checked once read; or in reading order, each file by its whole
// path with why it is read as its note. With it, its files in order.
func reviewList(s sawaState, reading bool, most int) ([]reviewEntry, []reviewRow) {
	files, why := reviewOrder(s, reading)
	open, _ := reviewOpen(s)
	read := 0
	for _, file := range files {
		if file.Read && !file.Stale {
			read++
		}
	}
	head := reviewEntry{Head: "Files", Tag: "by folder", Note: fmt.Sprintf("%d of %d read", read, len(files))}
	if reading {
		head.Tag = "reading order"
	}
	if len(files) > most {
		head.Note += fmt.Sprintf(" · %d shown", most)
		files = files[:most]
	}
	entries := []reviewEntry{head}
	var rows []reviewRow
	folder := ""
	for _, file := range files {
		entry := reviewEntry{ID: reviewID(file.Path), Right: plusMinus(file.Added, file.Removed), Checked: file.Read && !file.Stale}
		findings := ""
		if n := open[file.Path]; n > 0 {
			findings = counted(n, "finding")
		}
		if reading {
			entry.Title, entry.Note = file.Path, cutTo(why[file.Path], gitWordsMost)
			entry.Right = strings.TrimSpace(findings + "  " + entry.Right)
		} else {
			entry.Title, entry.Note = path.Base(file.Path), findings
			if in := path.Dir(file.Path); in != "." {
				if in != folder {
					entries = append(entries, reviewEntry{ID: cutTo("d:"+in+"/", gitWordsMost), Title: cutTo(in+"/", gitWordsMost), Open: true})
				}
				entry.Depth = 1
			}
			folder = path.Dir(file.Path)
		}
		if runes := []rune(entry.Title); len(runes) > gitWordsMost { // a long path is known by its end
			entry.Title = "…" + string(runes[len(runes)-gitWordsMost+1:])
		}
		entries = append(entries, entry)
		rows = append(rows, reviewRow{id: entry.ID, path: file.Path})
	}
	return entries, rows
}

// reviewUnread is the next file after one, in the order listed and round
// to the first again, that is not read; "" when every other is.
func reviewUnread(files []sawaFile, after string) string {
	at := slices.IndexFunc(files, func(file sawaFile) bool { return file.Path == after })
	for n := 1; n < len(files); n++ {
		if file := files[(at+n)%len(files)]; !file.Read || file.Stale {
			return file.Path
		}
	}
	return ""
}

// reviewSpot is where a finding is among all that stand: in which file
// of those listed, how far down it, and which of sawa's it is, so two on
// one line keep their order. A line of the code is a spot with no finding.
type reviewSpot struct {
	file, line, n int
	id            string
}

func (a reviewSpot) before(b reviewSpot) bool {
	return a.file < b.file || a.file == b.file && (a.line < b.line || a.line == b.line && a.n < b.n)
}

// reviewStep is the finding after a spot, or the one before it, of
// findings in order; past the last it is the first again, and before the
// first the last. With no finding there is none.
func reviewStep(spots []reviewSpot, here reviewSpot, forward bool) (reviewSpot, bool) {
	if len(spots) == 0 {
		return reviewSpot{}, false
	}
	if forward {
		for _, spot := range spots {
			if here.before(spot) {
				return spot, true
			}
		}
		return spots[0], true
	}
	for _, spot := range slices.Backward(spots) {
		if spot.before(here) {
			return spot, true
		}
	}
	return spots[len(spots)-1], true
}

// reviewLabel is a finding's label and its tone: its type and how bad it
// is, bad for a blocker and a major one, a warning for less and for an
// issue that does not say, plain for the rest, ok once it stands no more.
func reviewLabel(f sawaFinding) (label, tone string) {
	label = f.Type
	if f.Severity != "" {
		label += " · " + f.Severity
	}
	switch {
	case !f.stands():
		tone = "ok"
	case f.Severity == "blocker" || f.Severity == "major":
		tone = "bad"
	case f.Severity != "" || f.Type == "issue":
		tone = "warn"
	}
	return label, tone
}

// reviewThread is the pieces under a finding that is open: what was said
// in reply, each with who and when; the area to answer in, holding the
// words typed and not sent; and Reply, with Reply and resolve while the
// finding stands.
func reviewThread(f sawaFinding, typed string, now time.Time) []reviewPiece {
	var under []reviewPiece
	var replies []string
	for _, post := range f.Posts[min(1, len(f.Posts)):] {
		replies = append(replies, fmt.Sprintf("**%s** · %s\n\n%s", post.Author, reviewAge(now.Sub(post.At)), post.Body))
	}
	if len(replies) > 0 {
		under = append(under, reviewPiece{T: "markdown", Text: cutTo(strings.Join(replies, "\n\n"), muiProse)})
	}
	buttons := []reviewPiece{{T: "button", ID: "send", Label: "Reply", Key: "⌘⏎", Primary: true}}
	if f.stands() {
		buttons = append(buttons, reviewPiece{T: "button", ID: "resolve", Label: "Reply and resolve", Key: "⌘R"})
	}
	return append(under,
		reviewPiece{T: "field", ID: "reply", Lines: json.RawMessage("3"), Live: true, Placeholder: "Reply", Value: &typed},
		reviewPiece{T: "row", Children: buttons})
}

// reviewWriting is the pieces under a comment being written: the area,
// and Comment and Cancel.
func reviewWriting(typed string) []reviewPiece {
	return []reviewPiece{
		{T: "field", ID: "comment", Lines: json.RawMessage("3"), Live: true, Placeholder: "What is it about this line?", Value: &typed},
		{T: "row", Children: []reviewPiece{
			{T: "button", ID: "send", Label: "Comment", Key: "⌘⏎", Primary: true},
			{T: "button", ID: "cancel", Label: "Cancel"},
		}},
	}
}

// reviewNotes is what is said under a file's lines: its findings that
// are shown, the one that is open and the words typed under it, and the
// line a comment is being written under.
type reviewNotes struct {
	findings []sawaFinding
	open     string
	typed    string
	writing  *reviewMark
	now      time.Time
}

// note is a finding as a line of the code: one quiet line, and open all
// its words with its thread under them.
func (n reviewNotes) note(f sawaFinding) reviewLine {
	label, tone := reviewLabel(f)
	said := f.first()
	line := reviewLine{Kind: "note", Label: label, Tone: tone, Right: cutTo(said.Author+" · "+f.State, gitWordsMost)}
	if f.ID != n.open {
		line.Text, _, _ = strings.Cut(said.Body, "\n")
		line.Text = cutTo(line.Text, gitLineMost)
		return line
	}
	line.Text = cutTo(said.Body, reviewNoteMost)
	line.Right = cutTo(fmt.Sprintf("%s · %s · %s", said.Author, reviewAge(n.now.Sub(said.At)), f.State), gitWordsMost)
	line.Under = reviewThread(f, n.typed, n.now)
	return line
}

// reviewCode is a file's rows, as sw rows says them with the lines the
// same left out, as lines of code: each run of lines under a head that
// says where it begins and which hunk of how many it is, the lines left
// out between two as a gap, and each finding as a note directly under
// the line it is about. A finding on no line shown is over them all.
// With them, what each line is.
func reviewCode(rows []sawaRow, notes reviewNotes) ([]reviewLine, []reviewMark) {
	var lines []reviewLine
	var marks []reviewMark
	put := func(line reviewLine, mark reviewMark) {
		lines, marks = append(lines, line), append(marks, mark)
	}
	placed := map[string]bool{}
	for _, row := range rows {
		for _, id := range row.Threads {
			placed[id] = true
		}
	}
	for _, finding := range notes.findings {
		if !placed[finding.ID] {
			put(notes.note(finding), reviewMark{kind: "note", note: finding.ID})
		}
	}
	if len(rows) == 0 {
		put(reviewLine{Kind: "gap", Text: "no line of it to show"}, reviewMark{kind: "gap"})
		return lines, marks
	}

	// A run begins where a number does not follow the one before it on
	// its side: the first line of each is what its head is put over.
	var begins []int
	var same []int // the lines left out before each run
	old, new, oldKnown, newKnown := 0, 0, false, false
	for i, row := range rows {
		if i == 0 || row.Old != 0 && oldKnown && row.Old != old+1 || row.New != 0 && newKnown && row.New != new+1 {
			out := 0
			switch {
			case i == 0:
			case row.New != 0 && newKnown:
				out = row.New - new - 1
			case row.Old != 0 && oldKnown:
				out = row.Old - old - 1
			}
			begins, same = append(begins, i), append(same, out)
			oldKnown, newKnown = false, false
		}
		if row.Old != 0 {
			old, oldKnown = row.Old, true
		}
		if row.New != 0 {
			new, newKnown = row.New, true
		}
	}
	added := !slices.ContainsFunc(rows, func(row sawaRow) bool { return row.Kind != "add" }) && rows[0].New == 1
	deleted := !slices.ContainsFunc(rows, func(row sawaRow) bool { return row.Kind != "del" }) && rows[0].Old == 1

	run := 0
	for i, row := range rows {
		number := row.New
		if number == 0 {
			number = row.Old
		}
		if run < len(begins) && begins[run] == i {
			if same[run] > 0 {
				put(reviewLine{Kind: "gap", Text: counted(same[run], "line") + " the same"}, reviewMark{kind: "gap", new: number})
			}
			head := fmt.Sprintf("@@ line %d", number)
			switch {
			case added:
				head = "@@ new file"
			case deleted:
				head = "@@ deleted file"
			}
			run++
			put(reviewLine{Kind: "head", Text: head, Right: fmt.Sprintf("hunk %d of %d", run, len(begins))}, reviewMark{kind: "head", new: number})
		}
		mark := reviewMark{kind: row.Kind, old: row.Old, new: row.New}
		switch row.Kind {
		case "add", "del":
			put(reviewLine{Kind: row.Kind, Text: gitShown(row.Text), Old: row.Old, New: row.New}, mark)
		default:
			mark.kind = "same"
			put(reviewLine{Text: gitShown(row.Text), Old: row.Old, New: row.New}, mark)
		}
		for _, id := range row.Threads {
			if at := slices.IndexFunc(notes.findings, func(f sawaFinding) bool { return f.ID == id }); at >= 0 {
				put(notes.note(notes.findings[at]), reviewMark{kind: "note", note: id})
			}
		}
		if notes.writing != nil && *notes.writing == mark {
			where := fmt.Sprintf("line %d", number)
			if row.Kind == "del" {
				where += ", as it was"
			}
			put(reviewLine{Kind: "note", Label: "comment", Right: where, Under: reviewWriting(notes.typed)}, reviewMark{kind: "note", note: reviewNew})
		}
	}
	return lines, marks
}

// reviewCut is the first most lines of a code, and after them a gap that
// says how many more there are.
func reviewCut(lines []reviewLine, marks []reviewMark, most int) ([]reviewLine, []reviewMark) {
	if len(lines) <= most {
		return lines, marks
	}
	gap := reviewLine{Kind: "gap", Text: counted(len(lines)-most, "more line")}
	return append(lines[:most:most], gap), append(marks[:most:most], reviewMark{kind: "gap"})
}

// reviewAim is what a message says of where the person is, which stays
// as they have it when a message does not say: the file the list is on,
// the line the code is on, the words in the area under a note, and the
// piece the keys are put on.
type reviewAim struct {
	list, code, words bool
	focus             string
}

func (a reviewAim) with(b reviewAim) reviewAim {
	if b.focus == "" {
		b.focus = a.focus
	}
	return reviewAim{a.list || b.list, a.code || b.code, a.words || b.words, b.focus}
}

// reviewView is a review as the window is shown it.
type reviewView struct {
	pane, dir string  // the pane it is drawn over, and the folder it is run in
	run       sawaRun // sw, as the person
	target    string  // what the person asked to review, "" for the branch
	top       string  // the worktree's top folder
	branch    string
	slug      string // the review, as sawa names it
	state     sawaState
	raw       []byte // the state as sw printed it
	title     string // the change's title
	failed    string // why there is no review to show
	reading   bool   // the files are in reading order
	resolved  bool   // findings that stand no more are shown too
	judging   bool   // the verdicts are up
	entries   []reviewEntry
	rows      []reviewRow
	file      string    // the file whose diff is shown
	fileRows  []sawaRow // its rows, as sw rows said them
	lines     []reviewLine
	marks     []reviewMark
	at        int               // the line of the code under the keys
	open      string            // the finding that is open, reviewNew while a comment is written
	writing   *reviewMark       // the line that comment is under
	typed     map[string]string // what is typed and not sent, by finding
	status    *muiStatus        // how the last thing done went
}

// find finds the review to show: the one asked for, opened as sw nvim
// would; else the one sawa has for this worktree, else one opened for
// the branch against its base. Then it reads it.
func (v *reviewView) find(now time.Time) {
	v.failed, v.slug = "", ""
	top, err := git.Toplevel(v.dir)
	if err != nil {
		v.failed = "nothing to review: " + filepath.Base(v.dir) + " is in no repository"
		return
	}
	v.top, v.branch = top, git.CurrentBranch(top)
	open := reviewOpenArgs(v.target)
	if v.target == "" {
		listed, err := sawaAsk[[]sawaListed](v.run, "list", "--json")
		if err != nil {
			v.failed = cutTo(err.Error(), gitWordsMost)
			return
		}
		base := gitBase(top, v.branch)
		if v.slug = reviewHas(listed, base); v.slug == "" && base == "" {
			v.failed = "nothing to review: this branch has no base to stand against. mia review <range> names one"
			return
		}
		open = []string{"open", "-r", base + "..HEAD", "--json"}
	}
	if v.slug == "" {
		opened, err := sawaAsk[sawaListed](v.run, open...) // of what it says, its slug
		if err != nil {
			v.failed = cutTo(err.Error(), gitWordsMost)
			return
		}
		v.slug = opened.Slug
	}
	v.read(now)
}

// read reads the review again, and reports whether the line under the
// keys is at another place than it was. The file shown is the same
// where the review still has it, else its first.
func (v *reviewView) read(now time.Time) (moved bool) {
	raw, err := sawaSaid(v.run("state", "--json", "-t", v.slug))
	var state sawaState
	if err == nil && json.Unmarshal(raw, &state) != nil {
		err = errors.New("sw state said what mia cannot read")
	}
	if err != nil {
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
		return false
	}
	v.state, v.raw = state, raw
	v.title = v.titled()
	files, _ := reviewOrder(state, v.reading)
	if !slices.ContainsFunc(files, func(file sawaFile) bool { return file.Path == v.file }) {
		v.file, v.at, v.marks = "", 0, nil
		if len(files) > 0 {
			v.file = files[0].Path
		}
	}
	v.show()
	return v.build(now)
}

// titled is the change's title: a pull request's own, the subject of a
// branch's one commit, else the branch.
func (v *reviewView) titled() string {
	target := v.state.Target
	base, head, ranged := strings.Cut(target.Revset, "..")
	switch {
	case target.Title != "":
		return target.Title
	case v.top != "" && target.Kind == "range" && ranged && head == "HEAD" && !strings.HasPrefix(base, "-"):
		if ahead := git.Ahead(v.top, base, 2); len(ahead) == 1 {
			return ahead[0].Subject
		}
	}
	if v.branch != "" {
		return v.branch
	}
	return v.state.Label
}

// show reads the rows of the file shown.
func (v *reviewView) show() {
	v.fileRows = nil
	if v.file == "" {
		return
	}
	files, err := sawaAsk[[]sawaRows](v.run, "rows", "--changed", "--json", "-t", v.slug, "-f", v.file)
	if err != nil {
		v.status = &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
		return
	}
	for _, file := range files {
		if file.File == v.file {
			v.fileRows = file.Rows
		}
	}
}

// turn shows another file, from its first line, with nothing open.
func (v *reviewView) turn(file string, now time.Time) {
	v.file, v.at, v.marks, v.open, v.writing = file, 0, nil, "", nil
	v.show()
	v.build(now)
}

// build makes the list and the code of what was read, and reports
// whether the line under the keys is at another place than it was: it is
// the same line where that is still there.
func (v *reviewView) build(now time.Time) (moved bool) {
	var on reviewMark
	if v.at < len(v.marks) {
		on = v.marks[v.at]
	}
	// (Paths past ASCII are six bytes a character in a message: fewer files
	// are listed until the list leaves the diff most of the message.)
	for most := reviewRowsMost; ; most /= 2 {
		v.entries, v.rows = reviewList(v.state, v.reading, most)
		if most <= 1 || len(encode(v.entries)) < muiLimit/4 {
			break
		}
	}
	notes := reviewNotes{open: v.open, typed: v.typed[v.open], writing: v.writing, now: now}
	for _, finding := range v.state.Threads {
		if finding.File == v.file && (finding.stands() || v.resolved || finding.ID == v.open) {
			notes.findings = append(notes.findings, finding)
		}
	}
	lines, marks := reviewCode(v.fileRows, notes)
	v.lines, v.marks = reviewCut(lines, marks, gitLinesMost)
	before := v.at
	if to := slices.Index(v.marks, on); to >= 0 {
		v.at = to
	}
	v.at = max(min(v.at, len(v.marks)-1), 0)
	return v.at != before
}

// place is the place of a finding's note in the code, -1 where it has none.
func (v *reviewView) place(id string) int {
	return slices.Index(v.marks, reviewMark{kind: "note", note: id})
}

// spots is the findings that stand on the files listed, in the order n
// and N go through them: file by file as listed, and down each. Those of
// the file shown are known by their place in its code.
func (v *reviewView) spots() ([]reviewSpot, reviewSpot) {
	files, _ := reviewOrder(v.state, v.reading)
	index := func(file string) int {
		return slices.IndexFunc(files, func(f sawaFile) bool { return f.Path == file })
	}
	var spots []reviewSpot
	for n, finding := range v.state.Threads {
		file := index(finding.File)
		if !finding.stands() || file < 0 {
			continue
		}
		spot := reviewSpot{file: file, line: finding.Line, n: n, id: finding.ID}
		if finding.File == v.file {
			if spot.line = v.place(finding.ID); spot.line < 0 {
				spot.line = len(v.marks) + finding.Line
			}
		}
		spots = append(spots, spot)
	}
	slices.SortFunc(spots, func(a, b reviewSpot) int {
		if a.before(b) {
			return -1
		}
		return 1
	})
	here := reviewSpot{file: index(v.file), line: v.at, n: -1}
	if at := slices.IndexFunc(spots, func(s reviewSpot) bool { return s.file == here.file && s.line == here.line }); at >= 0 {
		here = spots[at]
	}
	return spots, here
}

// reviewWords is a text of words in a style and a tone, and nothing where there
// are no words: the window takes no text without them.
func reviewWords(text, style, tone string) []reviewPiece {
	if text == "" {
		return nil
	}
	return []reviewPiece{{T: "text", Text: text, Style: style, Tone: tone}}
}

// about is the line under the title: the branch and what it stands
// against, as the review's range says them, how many files and lines,
// and how long ago its last finding was filed.
func (v *reviewView) about(now time.Time) string {
	s := v.state
	name := s.Label
	if base, head, ranged := strings.Cut(s.Target.Revset, ".."); s.Target.Kind == "range" && ranged {
		if head = strings.TrimPrefix(head, "."); head == "HEAD" && v.branch != "" {
			head = v.branch
		}
		name = head + " → " + base
	}
	words := []string{name, counted(len(s.Files), "file")}
	if lines := plusMinus(s.Counts.Added, s.Counts.Removed); lines != "" {
		words = append(words, lines)
	}
	var last time.Time
	for _, finding := range s.Threads {
		if at := finding.first().At; at.After(last) {
			last = at
		}
	}
	if !last.IsZero() {
		words = append(words, "sawa read it "+reviewAge(now.Sub(last)))
	}
	return cutTo(strings.Join(words, " · "), gitWordsMost)
}

// keys is the keys the program takes: esc first while it shuts something.
func (v *reviewView) keys() []gitKey {
	if v.failed != "" {
		return []gitKey{{"r", "ask again", "again"}}
	}
	order, resolved := "reading order", "show resolved"
	if v.reading {
		order = "by folder"
	}
	if v.resolved {
		resolved = "hide resolved"
	}
	keys := []gitKey{
		{"n", "next finding", "next"}, {"N", "previous finding", "prev"}, {"c", "comment", "comment"},
		{"space", "read, next file", "read"}, {"o", order, "order"}, {"u", resolved, "resolved"},
		{"v", "verdict", "verdict"}, {"r", "ask again", "again"},
	}
	switch {
	case v.judging:
		return append([]gitKey{{"esc", "no verdict now", "shut"}}, keys...)
	case v.open != "":
		return append([]gitKey{{"esc", "shut it, keep the words", "shut"}}, keys...)
	}
	return keys
}

// verdicts is the verdicts sawa offers a person, as a list in place of
// the summary: the one given is checked, and can be taken back.
func (v *reviewView) verdicts(aim reviewAim) reviewPiece {
	given := v.state.Verdict.Verdict
	list := reviewPiece{T: "list", ID: "verdict", Items: []reviewEntry{
		{Head: "Verdict", Note: v.state.Verdict.Label},
		{ID: "approve", Title: "Approve", Checked: given == "approve"},
		{ID: "changes", Title: "Request changes", Checked: given == "changes"},
		{ID: "comment", Title: "Comment only", Checked: given == "comment"},
	}}
	if given != "" {
		list.Items = append(list.Items, reviewEntry{ID: "clear", Title: "No verdict"})
	}
	if aim.list {
		list.Selected = "approve"
		if given != "" {
			list.Selected = given
		}
	}
	return list
}

// whole is the message that says the whole view with these lines of the
// diff in it, and of where the person is what aim says.
func (v *reviewView) whole(lines json.RawMessage, aim reviewAim, now time.Time) []byte {
	message := reviewMessage{V: 1, Pane: v.pane, Fill: true, Focus: aim.focus, Status: v.status, Keys: v.keys()}
	if v.failed != "" {
		message.View = reviewPiece{T: "col", Pad: 14, Gap: 6, Children: append(reviewWords("Review", "title", ""), reviewWords(v.failed, "", "bad")...)}
		return encode(message)
	}
	s := v.state
	on, open := reviewOpen(s)
	standing, quiet, tone := "none open", "dim", ""
	if open > 0 {
		standing, quiet, tone = fmt.Sprintf("%d open", open), "", "warn"
	}
	top := reviewPiece{T: "row", Gap: 10, Children: append(append(append(
		reviewWords(cutTo(v.title, gitWordsMost), "title", ""),
		reviewPiece{T: "col", Grow: 1}),
		reviewWords(standing, quiet, tone)...),
		reviewWords("verdict  "+s.Verdict.Label, "dim", "")...)}
	head := reviewPiece{T: "col", Pad: 14, Gap: 6, Children: append([]reviewPiece{top}, reviewWords(v.about(now), "dim", "")...)}
	summary := strings.Join(strings.Fields(s.Summary), " ")
	if runes := []rune(summary); len(runes) > reviewSummaryMost {
		summary = string(runes[:reviewSummaryMost]) + "…"
	}
	if v.judging {
		head.Children = append(head.Children, v.verdicts(aim))
	} else {
		head.Children = append(head.Children, reviewWords(summary, "", "")...)
	}
	if len(s.Files) == 0 {
		head.Children = append(head.Children, reviewWords("nothing to review: "+s.Label+" changes no file", "dim", "")...)
		message.View = reviewPiece{T: "col", Children: []reviewPiece{head}}
		return encode(message)
	}

	list := reviewPiece{T: "list", ID: "files", Live: true, Items: v.entries}
	code := reviewPiece{T: "code", ID: "diff", Live: true, Lines: lines}
	if aim.list {
		list.Selected = reviewID(v.file)
	}
	if aim.code {
		code.Selected = strconv.Itoa(v.at)
	}
	files, _ := reviewOrder(s, v.reading)
	nth := slices.IndexFunc(files, func(file sawaFile) bool { return file.Path == v.file }) + 1
	findings := ""
	if n := on[v.file]; n > 0 {
		findings = counted(n, "finding")
	}
	over := reviewPiece{T: "row", Pad: 10, Gap: 10, Children: append(append(append(
		reviewWords(cutTo(v.file, gitWordsMost), "mono", ""),
		reviewWords(fmt.Sprintf("file %d of %d", nth, len(files)), "dim", "")...),
		reviewPiece{T: "col", Grow: 1}),
		reviewWords(findings, "", "warn")...)}
	message.View = reviewPiece{T: "col", Children: []reviewPiece{
		head,
		{T: "divider"},
		{T: "row", Split: true, Grow: 1, Children: []reviewPiece{
			{T: "col", W: 360, Scroll: true, Pad: 6, Children: []reviewPiece{list}},
			{T: "divider"},
			{T: "col", Children: []reviewPiece{over, code}},
		}},
	}}
	return encode(message)
}

// messages is the messages that say the view: the whole of it with the
// first lines of the diff that fit, then the rest of them as patches.
// What is typed under a note is said only where aim says the words.
func (v *reviewView) messages(aim reviewAim, now time.Time) [][]byte {
	lines := make([]json.RawMessage, len(v.lines))
	for i, line := range v.lines {
		if !aim.words && len(line.Under) > 0 {
			line.Under = slices.Clone(line.Under)
			for at := range line.Under {
				line.Under[at].Value = nil
			}
		}
		lines[i] = encode(line)
	}
	n := gitFit(lines, muiLimit-len(v.whole(nil, aim, now))-patchEnvelope)
	var first json.RawMessage
	if n > 0 {
		first, _ = json.Marshal(lines[:n]) // each already in ASCII alone
	}
	return append([][]byte{v.whole(first, aim, now)}, gitPatches(v.pane, lines[n:])...)
}

// reviewEvent is what the person did in the window: a file of the list
// or a line of the code come under the keys (move) or picked, a key or a
// button pressed, with the row it was pressed on as its item, the words
// under a note changed or sent, close.
type reviewEvent struct {
	E    string `json:"e"`
	ID   string `json:"id"`
	List string `json:"list"`
	Item string `json:"item"`
	Text string `json:"text"`
}

// reviewDo is what an event asks for.
type reviewDo int

const (
	reviewNothing reviewDo = iota
	reviewLeave
	reviewFile     // the diff of another file
	reviewInto     // that file, and the keys on its diff
	reviewOnLine   // another line of the code under the keys
	reviewTyped    // the words under a note as they now are
	reviewNext     // the keys on the next finding that stands
	reviewPrevious // and on the one before
	reviewUnfold   // a finding opened
	reviewShut     // what is open shut, its words kept
	reviewWrite    // a comment begun under a line
	reviewOrdered  // the files in the other order
	reviewResolved // findings that stand no more shown, or not
	reviewJudge    // the verdicts brought up
	reviewReply    // a reply sent
	reviewSettle   // a reply sent, and the finding resolved
	reviewComment  // the comment filed as a finding
	reviewRead     // a file marked read, and the next unread one shown
	reviewVerdict  // a verdict given
	reviewAgain    // the change looked at again
)

// reviewDeed is what an event does: with a file, a line's place, a
// finding, a verdict or words, as it needs.
type reviewDeed struct {
	do    reviewDo
	file  string
	place int
	id    string
	text  string
}

// fileOf is the file of an id of the list.
func (v *reviewView) fileOf(id string) (string, bool) {
	at := slices.IndexFunc(v.rows, func(row reviewRow) bool { return row.id == id })
	if at < 0 {
		return "", false
	}
	return v.rows[at].path, true
}

// deed is what an event does to the view as it stands. A key pressed on
// a row of the list is about that file, and on a line of the code about
// that line of the file shown.
func (v *reviewView) deed(event reviewEvent) reviewDeed {
	place, err := strconv.Atoi(event.ID)
	onLine := err == nil && place >= 0 && place < len(v.marks)
	switch {
	case event.E == "close":
		return reviewDeed{do: reviewLeave}
	case event.E == "move" && event.List == "files":
		if file, ok := v.fileOf(event.ID); ok && file != v.file {
			return reviewDeed{do: reviewFile, file: file}
		}
	case event.E == "move" && event.List == "diff" && onLine:
		return reviewDeed{do: reviewOnLine, place: place}
	case event.E == "pick" && event.List == "files":
		if file, ok := v.fileOf(event.ID); ok {
			return reviewDeed{do: reviewInto, file: file}
		}
	case event.E == "pick" && event.List == "verdict" && v.judging:
		return reviewDeed{do: reviewVerdict, id: event.ID}
	case event.E == "pick" && event.List == "diff" && onLine:
		switch note := v.marks[place].note; note {
		case "", reviewNew:
		case v.open:
			return reviewDeed{do: reviewShut}
		default:
			return reviewDeed{do: reviewUnfold, id: note}
		}
	case event.E == "change" && (event.ID == "reply" || event.ID == "comment"):
		return reviewDeed{do: reviewTyped, text: event.Text}
	case event.E == "submit" && event.ID == "reply" && v.open != "" && v.open != reviewNew:
		return reviewDeed{do: reviewReply, id: v.open, text: event.Text}
	case event.E == "submit" && event.ID == "comment" && v.open == reviewNew:
		return reviewDeed{do: reviewComment, text: event.Text}
	}
	if event.E != "press" {
		return reviewDeed{}
	}
	switch event.ID {
	case "next":
		return reviewDeed{do: reviewNext}
	case "prev":
		return reviewDeed{do: reviewPrevious}
	case "order":
		return reviewDeed{do: reviewOrdered}
	case "resolved":
		return reviewDeed{do: reviewResolved}
	case "verdict":
		return reviewDeed{do: reviewJudge}
	case "again":
		return reviewDeed{do: reviewAgain}
	case "shut", "cancel":
		return reviewDeed{do: reviewShut}
	case "send":
		switch v.open {
		case "":
		case reviewNew:
			return reviewDeed{do: reviewComment, text: v.typed[reviewNew]}
		default:
			return reviewDeed{do: reviewReply, id: v.open, text: v.typed[v.open]}
		}
	case "resolve":
		if v.open != "" && v.open != reviewNew {
			return reviewDeed{do: reviewSettle, id: v.open, text: v.typed[v.open]}
		}
	case "read":
		if file, ok := v.fileOf(event.Item); ok {
			return reviewDeed{do: reviewRead, file: file}
		}
		if _, err := strconv.Atoi(event.Item); (err == nil || event.Item == "") && v.file != "" {
			return reviewDeed{do: reviewRead, file: v.file} // on a line of the diff, which is of the file shown
		}
	case "comment":
		at, err := strconv.Atoi(event.Item)
		if _, onFile := v.fileOf(event.Item); onFile || err != nil || at < 0 || at >= len(v.marks) {
			return reviewDeed{do: reviewWrite, place: -1}
		}
		return reviewDeed{do: reviewWrite, place: at}
	}
	return reviewDeed{}
}

// asks is what a deed asks of sawa: the arguments of each sw it runs, in
// turn. Every one names the review, and words go after -m, so none is
// read as a flag.
func (v *reviewView) asks(deed reviewDeed) [][]string {
	resolve := []string{"resolve", deed.id, "--json", "-t", v.slug}
	reply := []string{"reply", deed.id, "--json", "-t", v.slug, "-m", deed.text}
	switch deed.do {
	case reviewReply:
		return [][]string{reply}
	case reviewSettle:
		if strings.TrimSpace(deed.text) == "" {
			return [][]string{resolve}
		}
		return [][]string{reply, resolve}
	case reviewComment:
		if v.writing == nil {
			return nil
		}
		side, line := "new", v.writing.new
		if v.writing.kind == "del" {
			side, line = "old", v.writing.old
		}
		return [][]string{{"add", "--json", "-t", v.slug, "-f", v.file, "-l", strconv.Itoa(line), "--side", side, "-m", deed.text}}
	case reviewRead:
		return [][]string{{"read", "--json", "-t", v.slug, "-f", deed.file}}
	case reviewVerdict:
		return [][]string{{"verdict", deed.id, "--json", "-t", v.slug}}
	case reviewAgain:
		return [][]string{reviewAgainArgs(v.state.Target)}
	}
	return nil
}

// act does a deed, and reports whether the view is to be said again, and
// what of where the person is it then says.
func (v *reviewView) act(deed reviewDeed, now time.Time) (say bool, aim reviewAim) {
	onto := func(id string) reviewAim { // the keys on a finding's note
		if at := v.place(id); at >= 0 {
			v.at = at
		}
		return reviewAim{code: true, focus: "diff"}
	}
	switch deed.do {
	case reviewNothing, reviewLeave:
		return false, aim
	case reviewTyped:
		if v.open != "" {
			v.typed[v.open] = cutTo(deed.text, reviewAreaMost)
		}
		return false, aim
	case reviewOnLine:
		v.at = deed.place
		return false, aim
	}
	v.status = nil
	if v.failed != "" {
		if deed.do == reviewAgain {
			v.find(now)
			return true, reviewAim{list: true, code: true}
		}
		return false, aim
	}
	switch deed.do {
	case reviewFile, reviewInto:
		if deed.file != v.file {
			v.turn(deed.file, now)
			aim.code = true
		}
		if deed.do == reviewInto {
			aim.focus = "diff"
		}
		return true, aim
	case reviewNext, reviewPrevious:
		spots, here := v.spots()
		to, found := reviewStep(spots, here, deed.do == reviewNext)
		if !found {
			v.status = &muiStatus{Text: "no finding stands"}
			return true, aim
		}
		files, _ := reviewOrder(v.state, v.reading)
		if file := files[to.file].Path; file != v.file {
			v.turn(file, now)
		} else {
			v.open, v.writing = "", nil
			v.build(now)
		}
		return true, onto(to.id).with(reviewAim{list: true})
	case reviewUnfold:
		v.open, v.writing = deed.id, nil
		v.build(now)
		aim = onto(deed.id).with(reviewAim{words: true, focus: "reply"})
		return true, aim
	case reviewShut:
		was, judged := v.open, v.judging
		v.judging, v.open, v.writing = false, "", nil
		v.build(now)
		switch {
		case judged && was == "":
			return true, reviewAim{focus: "files"}
		case was == "" || was == reviewNew:
			return true, reviewAim{code: true, focus: "diff"}
		}
		return true, onto(was)
	case reviewWrite:
		if deed.place < 0 || v.marks[deed.place].kind == "head" || v.marks[deed.place].kind == "gap" || v.marks[deed.place].kind == "note" {
			v.status = &muiStatus{Text: "c comments on a line of the diff: l passes the keys to it"}
			return true, aim
		}
		mark := v.marks[deed.place]
		v.open, v.writing = reviewNew, &mark
		v.build(now)
		return true, onto(reviewNew).with(reviewAim{words: true, focus: "comment"})
	case reviewOrdered:
		if !v.reading && len(v.state.Path) == 0 {
			v.status = &muiStatus{Text: "sawa has no reading order of this review: sw path writes one"}
			return true, aim
		}
		v.reading = !v.reading
		v.build(now)
		return true, reviewAim{list: true}
	case reviewResolved:
		v.resolved = !v.resolved
		return true, reviewAim{code: v.build(now)}
	case reviewJudge:
		v.judging = true
		return true, reviewAim{list: true, focus: "verdict"}
	}

	// What is left is asked of sawa, and what it refuses is the status.
	switch {
	case (deed.do == reviewReply || deed.do == reviewComment) && strings.TrimSpace(deed.text) == "":
		v.status = &muiStatus{Text: "nothing to send: write it first", Tone: "bad"}
		return true, aim
	case deed.do == reviewVerdict && !slices.Contains([]string{"approve", "changes", "comment", "clear"}, deed.id):
		return false, aim
	}
	var answer json.RawMessage
	for _, args := range v.asks(deed) {
		var err error
		if answer, err = sawaSaid(v.run(args...)); err != nil {
			status := &muiStatus{Text: cutTo(err.Error(), gitWordsMost), Tone: "bad"}
			aim.code = v.read(now) // one of two may have been done
			v.status = status
			return true, aim
		}
	}
	switch deed.do {
	case reviewReply:
		delete(v.typed, deed.id)
		v.status = &muiStatus{Text: "replied", Tone: "ok"}
		aim = reviewAim{words: true}
	case reviewSettle:
		delete(v.typed, deed.id)
		v.open = ""
		v.status = &muiStatus{Text: deed.id + " resolved", Tone: "ok"}
		aim = reviewAim{focus: "diff"}
	case reviewComment:
		var filed sawaFinding
		_ = json.Unmarshal(answer, &filed) // without its id the keys stay where they are
		delete(v.typed, reviewNew)
		v.open, v.writing = "", nil
		v.status = &muiStatus{Text: "filed " + filed.ID, Tone: "ok"}
		v.read(now)
		return true, onto(filed.ID)
	case reviewVerdict:
		v.judging = false
		aim = reviewAim{focus: "files"}
	case reviewAgain:
		v.status = &muiStatus{Text: "looked at again", Tone: "ok"}
	}
	moved := v.read(now)
	if deed.do == reviewRead {
		files, _ := reviewOrder(v.state, v.reading)
		if next := reviewUnread(files, deed.file); next == "" {
			v.status = &muiStatus{Text: "every file is read", Tone: "ok"}
		} else if next != v.file {
			v.turn(next, now)
			return true, reviewAim{list: true, code: true}
		}
	}
	return true, aim.with(reviewAim{code: moved})
}

// reviewStamp is what changes when a review does, cheap to ask for: the
// size and time of the file sawa says it keeps it in.
func reviewStamp(store string) string {
	info, err := os.Stat(store)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano())
}

// cmdReview shows the review sawa keeps of the worktree it is run in,
// drawn by mia desk over its pane: the change's title, findings and
// verdict over its files and, beside them, the diff of the one under the
// keys with each finding under its line; and the keys that go through
// the findings, answer and resolve them, file one, mark a file read and
// give the verdict, each through sw as the person. It reads the review
// again after each and when sawa's record of it changes.
func cmdReview(args []string) int {
	if slices.ContainsFunc(args, func(arg string) bool { return strings.HasPrefix(arg, "-") }) {
		return usageErr(reviewUsage)
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		return fail(errors.New("review reads what you do from a terminal: run it in a pane of mia desk"))
	}
	dir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	view := &reviewView{pane: os.Getenv("TMUX_PANE"), dir: dir, run: sawaIn(dir), target: strings.Join(args, " "), typed: map[string]string{}}
	view.find(time.Now())

	stop := make(chan struct{})
	defer close(stop)
	keys := muiKeys(stop)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	say, shut, err := muiOpen(view.pane, "mia review: the plain terminal. In mia desk sawa's review of "+filepath.Base(dir)+" is drawn over it; q or ⌃C leaves.")
	if err != nil {
		return fail(err)
	}
	defer shut()

	var shown []byte // what the view said last, but for where the person is
	show := func(aim reviewAim) {
		now := time.Now()
		shown = bytes.Join(view.messages(reviewAim{}, now), nil)
		say(view.messages(aim, now)...)
	}
	show(reviewAim{list: true, code: true, words: true})
	stamp := reviewStamp(view.state.Store)
	poll := time.NewTicker(gitPoll)
	defer poll.Stop()
	buffer := ""
	for {
		select {
		case <-signals:
			return exitOK
		case <-poll.C:
			now, aim := reviewStamp(view.state.Store), reviewAim{}
			if now != stamp && view.failed == "" {
				aim.code = view.read(time.Now())
			}
			stamp = now
			// (Said again when it reads another way: a finding come in, an age gone on.)
			if !bytes.Equal(bytes.Join(view.messages(reviewAim{}, time.Now()), nil), shown) {
				show(aim)
			}
		case data, open := <-keys:
			if !open {
				return exitOK
			}
			events, rest, sight, leave := muiTyped[reviewEvent](buffer, data)
			if leave {
				return exitOK
			}
			buffer = rest
			again, aim := sight, reviewAim{}
			if sight { // the pane came into sight: said again whole
				aim = reviewAim{list: true, code: true, words: true}
			}
			for i, event := range events {
				deed := view.deed(event)
				if deed.do == reviewLeave {
					return exitOK
				}
				if deed.do == reviewFile && i+1 < len(events) && events[i+1].E == "move" && events[i+1].List == "files" {
					continue // the keys held down: only where they came to rest is read
				}
				said, here := view.act(deed, time.Now())
				again, aim = again || said, aim.with(here)
			}
			if again {
				show(aim)
			}
			stamp = reviewStamp(view.state.Store)
		}
	}
}
