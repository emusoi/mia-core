package git

import (
	"errors"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// nulls is the paths of an output that ends each with a NUL. A folder git
// lists as one thing (a repository inside this one) is said without the
// slash at its end.
func nulls(out string) []string {
	var paths []string
	for _, path := range strings.Split(out, "\x00") {
		if path = strings.TrimSuffix(path, "/"); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// Files is every file of a worktree git does not ignore, those it tracks
// and those it does not, each once by its path from dir, in order.
func Files(dir string) ([]string, error) {
	out, err := look(dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	paths := nulls(out)
	slices.Sort(paths)
	return slices.Compact(paths), nil
}

// Untracked is the files of a worktree git neither tracks nor ignores.
func Untracked(dir string) ([]string, error) {
	out, err := look(dir, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return nulls(out), nil
}

// Missing is the files git tracks that are not on disk.
func Missing(dir string) ([]string, error) {
	out, err := look(dir, "ls-files", "-z", "--deleted")
	return nulls(out), err
}

// EmptyTree is the name of the tree with nothing in it: what a worktree
// with no commit stands against.
func EmptyTree(dir string) (string, error) {
	out, err := look(dir, "hash-object", "-t", "tree", "/dev/null")
	return strings.TrimSpace(out), err
}

// Against is how the files of a worktree, as they are on disk, differ
// from a commit, as git diff --name-status -z writes it: a rename is a
// file deleted and one added.
func Against(dir, commit string) (string, error) {
	return look(dir, "diff", "--name-status", "--no-renames", "-z", commit, "--")
}

// ParseAgainst is what an Against says: each path with A where it is
// added, D where it is deleted and M where it is changed in any other way.
func ParseAgainst(out string) map[string]byte {
	marks := map[string]byte{}
	fields := strings.Split(out, "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == "" || fields[i+1] == "" {
			continue
		}
		mark := fields[i][0]
		if mark != 'A' && mark != 'D' {
			mark = 'M'
		}
		marks[fields[i+1]] = mark
	}
	return marks
}

// Search is a git grep at work. Lines is what it finds as it finds it,
// each line a path, the line's number and its text with a NUL between.
type Search struct {
	Lines   io.Reader
	command *exec.Cmd
	refused strings.Builder
}

// Match is how a search reads its words: as they are written, as a
// regular expression, or as they are written and only where they are
// whole words.
type Match int

const (
	AsWritten Match = iota
	AsPattern
	WholeWords
)

// Grep begins a search of dir's files for words, read as how says,
// whatever their case where they are all in lower case. In a repository
// it looks in the files git tracks and those it does not, ignored ones
// left out; in a folder of none, in every file under it. Binary files are
// passed over.
func Grep(dir, words string, how Match, repository bool) (*Search, error) {
	args := []string{"--no-optional-locks", "-c", "core.quotePath=false", "grep", "--no-color", "-n", "-I", "-z"}
	switch how {
	case AsPattern:
		args = append(args, "-E")
	case WholeWords:
		args = append(args, "-F", "-w")
	default:
		args = append(args, "-F")
	}
	if repository {
		args = append(args, "--untracked")
	} else {
		args = append(args, "--no-index")
	}
	if words == strings.ToLower(words) {
		args = append(args, "-i")
	}
	search := &Search{command: gitIn(dir).Command(append(args, "-e", words)...)}
	search.command.Stderr = &search.refused
	lines, err := search.command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	search.Lines = lines
	return search, search.command.Start()
}

// Stop ends the search where it is.
func (s *Search) Stop() {
	_ = s.command.Process.Kill()
}

// Wait is how the search ended, once Lines is read to its end: nil where
// git found lines or found none, else what it refused by its first line.
func (s *Search) Wait() error {
	err := s.command.Wait()
	var exit *exec.ExitError
	if err == nil || errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	if said := firstLine(strings.TrimSpace(s.refused.String())); said != "" {
		return errors.New(said)
	}
	return err
}

// at reads of one path, written as it is: no character of it is a
// pattern's.
func at(dir string, args ...string) (string, error) {
	return look(dir, append([]string{"--literal-pathspecs"}, args...)...)
}

// Tracked is the files git tracks at a path: the file itself, or every
// one under a folder.
func Tracked(dir, path string) ([]string, error) {
	out, err := at(dir, "ls-files", "-z", "--", path)
	return nulls(out), err
}

// Unsaved reports whether what git tracks at a path is other than the
// last commit has it: git cannot bring that back.
func Unsaved(dir, path string) bool {
	out, err := at(dir, "status", "--porcelain", "--untracked-files=no", "--", path)
	return err == nil && strings.TrimSpace(out) != ""
}

// Move renames a path git tracks, a file or a folder, on disk and in the
// index.
func Move(dir, from, to string) error {
	_, err := exact(dir, "", "mv", "--", from, to)
	return err
}

// DiffAgainst is how one file, as it is on disk, differs from a commit.
func DiffAgainst(dir, commit, path string) ([]FileDiff, error) {
	out, err := at(dir, append(append([]string{"diff"}, asPatch...), commit, "--", path)...)
	return ParseDiff(out), err
}

// DiffBetween is how one file differs between two commits.
func DiffBetween(dir, base, head, path string) ([]FileDiff, error) {
	out, err := at(dir, append(append([]string{"diff"}, asPatch...), base, head, "--", path)...)
	return ParseDiff(out), err
}

// ShownOf is what one commit changed of one file, a merge's against its
// first parent.
func ShownOf(dir, commit, path string) ([]FileDiff, error) {
	out, err := at(dir, append(append([]string{"show", "--format=", "--diff-merges=first-parent"}, asPatch...), commit, "--", path)...)
	return ParseDiff(out), err
}

// Described is a commit by its short name, its subject and when it was
// written.
func Described(dir, commit string) (Commit, error) {
	out, err := look(dir, "log", "-1", "--format=%h%x00%at%x00%s", commit, "--")
	parts := strings.SplitN(strings.TrimSpace(out), "\x00", 3)
	if err != nil || len(parts) != 3 {
		return Commit{}, errors.Join(err, errors.New("no commit "+commit))
	}
	secs, _ := strconv.ParseInt(parts[1], 10, 64)
	return Commit{SHA: parts[0], Subject: parts[2], When: time.Unix(secs, 0)}, nil
}

// Blamed is the commit a line of a file is of, and when that was
// written. A line not yet committed has no commit.
type Blamed struct {
	Commit string
	When   time.Time
}

// Blame is each line of a file as it is on disk with the commit it is of.
func Blame(dir, path string) ([]Blamed, error) {
	out, err := look(dir, "blame", "--porcelain", "--", path)
	return ParseBlame(out), err
}

// ParseBlame is what git blame --porcelain writes, read: a line of the
// file is a commit's name with the line's numbers, what is known of the
// commit the first time it is named, then the line itself after a tab.
func ParseBlame(out string) []Blamed {
	var lines []Blamed
	when := map[string]time.Time{}
	commit := ""
	for _, line := range strings.Split(out, "\n") {
		name, rest, _ := strings.Cut(line, " ")
		switch {
		case strings.HasPrefix(line, "\t"):
			blamed := Blamed{Commit: commit, When: when[commit]}
			if strings.Trim(commit, "0") == "" {
				blamed = Blamed{}
			}
			lines = append(lines, blamed)
		case name == "author-time":
			secs, _ := strconv.ParseInt(rest, 10, 64)
			when[commit] = time.Unix(secs, 0)
		case len(name) >= 40 && strings.Trim(name, "0123456789abcdef") == "":
			commit = name
		}
	}
	return lines
}
