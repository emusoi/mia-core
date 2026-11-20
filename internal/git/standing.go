package git

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// exact is what git wrote, not a byte of it trimmed, with stdin what is
// given: a status begins with a space and a patch can end in blank lines,
// which run takes off. What it refuses is said by the first line it wrote.
func exact(dir, stdin string, args ...string) (string, error) {
	command := gitIn(dir).Command(args...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = strings.TrimSpace(stdout.String())
		}
		if said == "" {
			said = err.Error()
		}
		return "", errors.New(firstLine(said))
	}
	return stdout.String(), nil
}

// look runs a git command that only reads: not even the index is
// refreshed, and a path is written as it is.
func look(dir string, args ...string) (string, error) {
	return exact(dir, "", append([]string{"--no-optional-locks", "-c", "core.quotePath=false"}, args...)...)
}

// Change is a file as git status lists it in one of its groups.
type Change struct {
	Path string
	// From is where a renamed file was.
	From string
	// State is new, deleted, renamed or conflict; "" for one only changed.
	State   string
	Added   int
	Deleted int
}

// Standing is how a worktree stands: its branch ("" detached), the commit
// it is at ("" before the first), what the branch follows ("" nothing)
// with its commits that has not and those of it the branch has not, and
// its files staged, changed and not staged, and untracked. A file staged
// in part is in two groups. Gone, what it follows is no longer there.
type Standing struct {
	Branch    string
	Head      string
	Upstream  string
	Gone      bool
	ToPush    int
	ToPull    int
	Staged    []Change
	Changed   []Change
	Untracked []Change
}

// Status is git status as it writes it for programs (porcelain v2, each
// entry ended by a NUL, every untracked file by itself).
func Status(dir string) (string, error) {
	return look(dir, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all")
}

// ParseStatus is what a Status says.
func ParseStatus(out string) Standing {
	var s Standing
	state := map[byte]string{'A': "new", 'C': "new", 'D': "deleted", 'R': "renamed"}
	entries := strings.Split(out, "\x00")
	counted := false
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		var fields []string
		switch {
		case strings.HasPrefix(entry, "# branch.upstream "):
			s.Upstream = entry[len("# branch.upstream "):]
			continue
		case strings.HasPrefix(entry, "# branch.ab "):
			_, err := fmt.Sscanf(entry, "# branch.ab +%d -%d", &s.ToPush, &s.ToPull)
			counted = err == nil
			continue
		case strings.HasPrefix(entry, "# branch.oid "):
			if s.Head = entry[len("# branch.oid "):]; s.Head == "(initial)" {
				s.Head = ""
			}
			continue
		case strings.HasPrefix(entry, "# branch.head "):
			if s.Branch = entry[len("# branch.head "):]; s.Branch == "(detached)" {
				s.Branch = ""
			}
			continue
		case strings.HasPrefix(entry, "? "):
			s.Untracked = append(s.Untracked, Change{Path: entry[2:]})
			continue
		case strings.HasPrefix(entry, "u "):
			if fields = strings.SplitN(entry, " ", 11); len(fields) == 11 {
				s.Changed = append(s.Changed, Change{Path: fields[10], State: "conflict"})
			}
			continue
		case strings.HasPrefix(entry, "1 "):
			fields = strings.SplitN(entry, " ", 9)
		case strings.HasPrefix(entry, "2 "):
			fields = strings.SplitN(entry, " ", 10)
			i++ // where it was renamed from is the entry after
		}
		if len(fields) < 9 || len(fields[1]) != 2 {
			continue
		}
		change := Change{Path: fields[len(fields)-1]}
		if fields[0] == "2" && i < len(entries) {
			change.From = entries[i]
		}
		if x := fields[1][0]; x != '.' {
			staged := change
			staged.State = state[x]
			s.Staged = append(s.Staged, staged)
		}
		if y := fields[1][1]; y != '.' {
			change.State, change.From = state[y], ""
			s.Changed = append(s.Changed, change)
		}
	}
	s.Gone = s.Upstream != "" && !counted // git counts only against what is there
	return s
}

// parseNumstat is the lines added and deleted in each file of a
// `diff --numstat -z`, by its path (a renamed file's new one). A binary
// file has none.
func parseNumstat(out string) map[string][2]int {
	counts := map[string][2]int{}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		fields := strings.SplitN(entries[i], "\t", 3)
		if len(fields) != 3 {
			continue
		}
		path := fields[2]
		if path == "" && i+2 < len(entries) { // renamed: from, then to
			path = entries[i+2]
			i += 2
		}
		added, _ := strconv.Atoi(fields[0])
		deleted, _ := strconv.Atoi(fields[1])
		counts[path] = [2]int{added, deleted}
	}
	return counts
}

// StandingOf is how the worktree at dir stands, each staged and changed
// file with its lines added and deleted.
func StandingOf(dir string) (Standing, error) {
	out, err := Status(dir)
	if err != nil {
		return Standing{}, err
	}
	s := ParseStatus(out)
	count := func(changes []Change, args ...string) {
		if len(changes) == 0 {
			return
		}
		out, _ := look(dir, append([]string{"diff", "--numstat", "-z"}, args...)...)
		counts := parseNumstat(out)
		for i, change := range changes {
			changes[i].Added, changes[i].Deleted = counts[change.Path][0], counts[change.Path][1]
		}
	}
	count(s.Staged, "--cached")
	count(s.Changed)
	return s, nil
}

// Hunk is one hunk of a diff: where it is in the old file and the new,
// the words git found over it, its head as git wrote it, and its lines,
// each with its mark first (" ", "+", "-", or "\" for a note of git's).
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Section            string
	Head               string
	Body               []string
}

// FileDiff is one file's part of a diff: the lines over its hunks as git
// wrote them, and its hunks. A binary file has none.
type FileDiff struct {
	Path   string
	Header []string
	Hunks  []Hunk
	Binary bool
}

// parseHunkHead is a hunk's head, `@@ -212,6 +212,11 @@ fn hear`, read.
func parseHunkHead(line string) (Hunk, bool) {
	rest, ok := strings.CutPrefix(line, "@@ -")
	ranges, section, closed := strings.Cut(rest, " @@")
	before, after, two := strings.Cut(ranges, " +")
	if !ok || !closed || !two {
		return Hunk{}, false
	}
	span := func(text string) (start, lines int) {
		first, count, counted := strings.Cut(text, ",")
		start, _ = strconv.Atoi(first)
		lines = 1
		if counted {
			lines, _ = strconv.Atoi(count)
		}
		return start, lines
	}
	hunk := Hunk{Section: strings.TrimSpace(section), Head: line}
	hunk.OldStart, hunk.OldLines = span(before)
	hunk.NewStart, hunk.NewLines = span(after)
	return hunk, true
}

// ParseDiff is a diff as git writes it, read into its files and their
// hunks. What stands before the first file (a commit's message, in
// `git show`) is passed over.
func ParseDiff(out string) []FileDiff {
	var files []FileDiff
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			file := FileDiff{Header: []string{line}}
			if at := strings.LastIndex(line, " b/"); at >= 0 {
				file.Path = line[at+3:]
			}
			files = append(files, file)
			continue
		}
		if len(files) == 0 {
			continue
		}
		file := &files[len(files)-1]
		if hunk, ok := parseHunkHead(line); ok {
			file.Hunks = append(file.Hunks, hunk)
			continue
		}
		if len(file.Hunks) > 0 {
			if line != "" && strings.IndexByte(" +-\\", line[0]) >= 0 {
				last := &file.Hunks[len(file.Hunks)-1]
				last.Body = append(last.Body, line)
			}
			continue
		}
		file.Header = append(file.Header, line)
		if path, ok := strings.CutPrefix(line, "+++ b/"); ok {
			file.Path = path
		} else if path, ok := strings.CutPrefix(line, "--- a/"); ok {
			file.Path = path
		} else if strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch" {
			file.Binary = true
		}
	}
	return files
}

// Patch is the patch of one hunk alone, counted from 0: what
// `git apply --cached` takes to stage it, and with -R to unstage it.
func (f FileDiff) Patch(hunk int) string {
	lines := append(append(append([]string{}, f.Header...), f.Hunks[hunk].Head), f.Hunks[hunk].Body...)
	return strings.Join(lines, "\n") + "\n"
}

var asPatch = []string{"--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/"}

// DiffOf is the diff of one file: what is staged of it, or what is
// changed and not staged.
func DiffOf(dir, path string, staged bool) ([]FileDiff, error) {
	args := append([]string{"diff"}, asPatch...)
	if staged {
		args = append(args, "--cached")
	}
	out, err := look(dir, append(args, "--", path)...)
	return ParseDiff(out), err
}

// Shown is the diff of one commit, a merge's against its first parent.
func Shown(dir, commit string) ([]FileDiff, error) {
	out, err := look(dir, append(append([]string{"show", "--format=", "--diff-merges=first-parent"}, asPatch...), commit, "--")...)
	return ParseDiff(out), err
}

// Stage puts files in the index as they are in the worktree, one deleted
// there too; with no path, every file.
func Stage(dir string, paths ...string) error {
	_, err := exact(dir, "", append([]string{"add", "--all", "--"}, paths...)...)
	return err
}

// Unstage puts files back in the index as they are in the last commit.
func Unstage(dir string, paths ...string) error {
	_, err := exact(dir, "", append([]string{"reset", "--quiet", "--"}, paths...)...)
	return err
}

// ApplyToIndex stages a patch, or with reverse unstages it; the worktree
// is not touched.
func ApplyToIndex(dir, patch string, reverse bool) error {
	args := []string{"apply", "--cached"}
	if reverse {
		args = append(args, "--reverse")
	}
	_, err := exact(dir, patch, append(args, "-")...)
	return err
}

// CommitStaged commits what is staged with the message as it is given,
// and is the new commit's short hash. Hooks run; one that fails is the
// error, by the first line it wrote.
func CommitStaged(dir, message string) (string, error) {
	if _, err := exact(dir, "", "commit", "--quiet", "-m", message); err != nil {
		return "", err
	}
	return run(dir, "rev-parse", "--short", "HEAD")
}

// Ahead is the commits HEAD has that base has not, newest first, n at most.
func Ahead(dir, base string, n int) []Commit {
	return commits(dir, n, base+"..HEAD")
}
