package git

import (
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
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

// Grep begins a search of dir's files for words as they are written, not
// as a pattern, whatever their case where they are all in lower case. In
// a repository it looks in the files git tracks and those it does not,
// ignored ones left out; in a folder of none, in every file under it.
// Binary files are passed over.
func Grep(dir, words string, repository bool) (*Search, error) {
	args := []string{"--no-optional-locks", "-c", "core.quotePath=false", "grep", "--no-color", "-n", "-I", "-z", "-F"}
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
