package git

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Restore puts files back in the worktree as they are in the index: what
// was changed in them and not staged is gone.
func Restore(dir string, paths ...string) error {
	_, err := exact(dir, "", append([]string{"restore", "--worktree", "--"}, paths...)...)
	return err
}

// Clean deletes untracked files; one git tracks is left as it is.
func Clean(dir string, paths ...string) error {
	_, err := exact(dir, "", append([]string{"clean", "--force", "--quiet", "--"}, paths...)...)
	return err
}

// ApplyToWorktree makes a patch's change in the worktree, or with reverse
// takes it out; the index is not touched.
func ApplyToWorktree(dir, patch string, reverse bool) error {
	args := []string{"apply"}
	if reverse {
		args = append(args, "--reverse")
	}
	_, err := exact(dir, patch, append(args, "-")...)
	return err
}

// Counts is the lines a file's part of a diff adds and deletes.
func (f FileDiff) Counts() (added, deleted int) {
	for _, hunk := range f.Hunks {
		for _, line := range hunk.Body {
			switch line[0] {
			case '+':
				added++
			case '-':
				deleted++
			}
		}
	}
	return added, deleted
}

// LinePatch is the patch of one line of a hunk alone, both counted from 0,
// the line among the hunk's as git wrote them: what `git apply --cached`
// takes to stage that line, and with reverse the patch `--reverse` takes
// to unstage it. The index holds the old side of a diff not staged and
// the new side of a staged one, so the other lines the hunk changes are
// written as they stand there: one it has as a line the same, one it has
// not left out. The file is named as one only changed, whatever else the
// diff says of it.
func (f FileDiff) LinePatch(hunk, line int, reverse bool) (string, error) {
	h := f.Hunks[hunk]
	if line < 0 || line >= len(h.Body) || h.Body[line][0] != '+' && h.Body[line][0] != '-' {
		return "", errors.New("not a line added or deleted")
	}
	var old, new string
	for _, head := range f.Header {
		if name, ok := strings.CutPrefix(head, "--- a/"); ok {
			old = name
		} else if name, ok := strings.CutPrefix(head, "+++ b/"); ok {
			new = name
		}
	}
	name := old
	if reverse || name == "" {
		name = new
	}
	switch {
	case reverse && new == "" && old != "":
		return "", errors.New("a deleted file is unstaged whole")
	case name == "":
		return "", errors.New("git writes this file's name in quotes: its lines are staged by the hunk")
	}
	var body []string
	olds, news, kept := 0, 0, false
	for i, text := range h.Body {
		switch mark := text[0]; {
		case mark == '\\': // git's note is of the line before it
			if kept {
				body = append(body, text)
			}
			continue
		case i == line || mark == ' ':
		case (mark == '-') != reverse: // a line the index has, and keeps
			text = " " + text[1:]
		default:
			kept = false
			continue
		}
		kept = true
		body = append(body, text)
		if text[0] != '+' {
			olds++
		}
		if text[0] != '-' {
			news++
		}
	}
	// Where the hunk is in the index: by the side of it written whole.
	start, whole := h.OldStart, h.OldLines
	if reverse {
		start, whole = h.NewStart, h.NewLines
	}
	if whole == 0 { // it names the line before
		start++
	}
	span := func(lines int) string {
		if lines == 0 {
			return fmt.Sprintf("%d,0", start-1)
		}
		return fmt.Sprintf("%d,%d", start, lines)
	}
	head := []string{
		"diff --git a/" + strings.TrimSuffix(name, "\t") + " b/" + strings.TrimSuffix(name, "\t"),
		"--- a/" + name,
		"+++ b/" + name,
		"@@ -" + span(olds) + " +" + span(news) + " @@",
	}
	return strings.Join(append(head, body...), "\n") + "\n", nil
}

// Told is who wrote a commit and when.
func Told(dir, commit string) (author string, when time.Time) {
	out, err := look(dir, "show", "--no-patch", "--format=%at%x00%an", commit, "--")
	secs, name, ok := strings.Cut(strings.TrimSpace(out), "\x00")
	if err != nil || !ok {
		return "", time.Time{}
	}
	at, _ := strconv.ParseInt(secs, 10, 64)
	return name, time.Unix(at, 0)
}

// Amend makes the last commit again with what is staged added to it, and
// is its new short hash: under the message given, or with none under the
// one it had. Hooks run.
func Amend(dir, message string) (string, error) {
	args := []string{"commit", "--quiet", "--amend", "--no-edit"}
	if message != "" {
		args = []string{"commit", "--quiet", "--amend", "-m", message}
	}
	if _, err := exact(dir, "", args...); err != nil {
		return "", err
	}
	return run(dir, "rev-parse", "--short", "HEAD")
}

// UndoCommit takes the last commit off the branch and leaves what it
// changed staged.
func UndoCommit(dir string) error {
	_, err := exact(dir, "", "reset", "--soft", "HEAD~1")
	return err
}

// Unpushed is how many commits HEAD has that no branch of any remote has.
func Unpushed(dir string) int {
	out, err := look(dir, "rev-list", "--count", "HEAD", "--not", "--remotes")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

// Branch is a branch of the repository, or with Remote one of a remote's
// (origin/main): what it follows, "" for nothing, with its commits that
// has not and those of it the branch has not, and whether what it follows
// is gone. Current is the one the worktree is on.
type Branch struct {
	Name     string
	Remote   bool
	Current  bool
	Upstream string
	Gone     bool
	Ahead    int
	Behind   int
}

const branchFormat = "%(refname)%00%(HEAD)%00%(upstream:short)%00%(upstream:track,nobracket)"

// ParseBranches is what `git for-each-ref` says of branches in
// branchFormat, a remote's own HEAD left out.
func ParseBranches(out string) []Branch {
	var all []Branch
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			continue
		}
		branch := Branch{Current: fields[1] == "*", Upstream: fields[2]}
		name, local := strings.CutPrefix(fields[0], "refs/heads/")
		if !local {
			var remote bool
			if name, remote = strings.CutPrefix(fields[0], "refs/remotes/"); !remote || strings.HasSuffix(name, "/HEAD") {
				continue
			}
			branch.Remote = true
		}
		branch.Name = name
		for _, word := range strings.Split(fields[3], ", ") {
			switch {
			case word == "gone":
				branch.Gone = true
			case strings.HasPrefix(word, "ahead "):
				branch.Ahead, _ = strconv.Atoi(word[len("ahead "):])
			case strings.HasPrefix(word, "behind "):
				branch.Behind, _ = strconv.Atoi(word[len("behind "):])
			}
		}
		all = append(all, branch)
	}
	return all
}

// Branches is the repository's branches by name, then its remotes'.
func Branches(dir string) ([]Branch, error) {
	out, err := look(dir, "for-each-ref", "--format="+branchFormat, "refs/heads", "refs/remotes")
	return ParseBranches(out), err
}

// Switch puts the worktree on a branch. What git refuses (a file changed
// that the branch has otherwise, a branch another worktree is on) is the
// error, by its first line.
func Switch(dir, branch string) error {
	_, err := exact(dir, "", "switch", "--quiet", "--no-guess", branch)
	return err
}

// SwitchTracking makes the branch that follows a remote's (origin/talk:
// talk) and puts the worktree on it.
func SwitchTracking(dir, remote string) error {
	_, err := exact(dir, "", "switch", "--quiet", "--track", remote)
	return err
}

// SwitchNew makes a branch where HEAD is and puts the worktree on it.
func SwitchNew(dir, branch string) error {
	_, err := exact(dir, "", "switch", "--quiet", "--create", branch)
	return err
}

// Stash is one entry of the stash: its commit, what git says it is of,
// and when it was made.
type Stash struct {
	SHA     string
	Subject string
	When    time.Time
}

// Stashes is the stash, newest first.
func Stashes(dir string) []Stash {
	out, err := look(dir, "stash", "list", "--format=%H%x00%ct%x00%gs")
	if err != nil {
		return nil
	}
	var all []Stash
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		secs, _ := strconv.ParseInt(parts[1], 10, 64)
		all = append(all, Stash{SHA: parts[0], Subject: parts[2], When: time.Unix(secs, 0)})
	}
	return all
}

// StashAll stashes every change, staged or not, and every untracked file,
// under a message.
func StashAll(dir, message string) error {
	_, err := exact(dir, "", "stash", "push", "--quiet", "--include-untracked", "--message", message)
	return err
}

// StashShown is the diff of a stash: what it changed against the commit
// it was made on, then its untracked files, which it keeps in a commit
// of their own (its third parent), each as a new file.
func StashShown(dir, sha string) ([]FileDiff, error) {
	out, err := look(dir, append(append([]string{"diff"}, asPatch...), sha+"^1", sha, "--")...)
	untracked, _ := look(dir, append(append([]string{"show", "--format="}, asPatch...), sha+"^3", "--")...)
	return append(ParseDiff(out), ParseDiff(untracked)...), err
}

// stashNamed is the name the stash has now for the entry of a commit
// (stash@{2}): an entry's place changes as others come and go, and a
// worktree shares the stash with every other of its repository.
func stashNamed(dir, sha string) (string, error) {
	for place, stash := range Stashes(dir) {
		if stash.SHA == sha {
			return fmt.Sprintf("stash@{%d}", place), nil
		}
	}
	return "", errors.New("that stash is no longer there")
}

// StashApply makes a stash's changes in the worktree and keeps the stash.
func StashApply(dir, sha string) error {
	_, err := exact(dir, "", "stash", "apply", "--quiet", sha)
	return err
}

// StashPop makes a stash's changes in the worktree and drops it; where
// they do not apply it is kept.
func StashPop(dir, sha string) error {
	name, err := stashNamed(dir, sha)
	if err == nil {
		_, err = exact(dir, "", "stash", "pop", "--quiet", name)
	}
	return err
}

// StashDrop drops a stash.
func StashDrop(dir, sha string) error {
	name, err := stashNamed(dir, sha)
	if err == nil {
		_, err = exact(dir, "", "stash", "drop", "--quiet", name)
	}
	return err
}

// Follows is the remote a branch follows and the branch of it: origin and
// talk. Neither for one that follows nothing, or a branch of this
// repository.
func Follows(dir, branch string) (remote, theirs string) {
	remote, _ = run(dir, "config", "--get", "branch."+branch+".remote")
	merge, _ := run(dir, "config", "--get", "branch."+branch+".merge")
	theirs, ok := strings.CutPrefix(merge, "refs/heads/")
	if remote == "" || remote == "." || !ok {
		return "", ""
	}
	return remote, theirs
}

// outcome is the line of what git wrote that says how a command went: the
// last, and of one that failed the first that says why. Its hints are
// passed over.
func outcome(out string, failed bool) string {
	last := ""
	for _, line := range strings.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "hint:") {
			continue
		}
		if failed && (strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "! ")) {
			return line
		}
		last = line
	}
	return last
}

// afar runs a git command that talks to a remote, and is the line that
// says how it went. It has no terminal: a remote that wants a password
// typed fails, and nothing is asked over the UI the terminal is under.
func afar(dir string, args ...string) (string, error) {
	command := gitIn(dir).Command(args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out, err := command.CombinedOutput()
	said := outcome(string(out), err != nil)
	if err != nil {
		if said == "" {
			said = err.Error()
		}
		return "", errors.New(said)
	}
	return said, nil
}

// FetchFrom fetches a remote's branches, those gone from it dropped here.
func FetchFrom(dir, remote string) (string, error) {
	return afar(dir, "fetch", "--prune", remote)
}

// PullFastForward moves the branch on to what it follows where that is
// only ahead of it. A branch with commits of its own is refused: nothing
// is merged and nothing rebased.
func PullFastForward(dir string) (string, error) {
	return afar(dir, "pull", "--ff-only", "--no-rebase")
}

// PushTo pushes a branch to a remote's branch, with follow making that
// what it follows from then on. Never by force: a remote that has commits
// the branch has not refuses it.
func PushTo(dir, remote, branch, theirs string, follow bool) (string, error) {
	args := []string{"push"}
	if follow {
		args = append(args, "--set-upstream")
	}
	return afar(dir, append(args, remote, "refs/heads/"+branch+":refs/heads/"+theirs)...)
}
