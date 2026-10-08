package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/run"
)

// prepare readies a new worktree before its setup runs: the paths named in clone come over
// from the main checkout, and with commit_times its files carry their last commit's time.
func (a *App) prepare(path string) error {
	for _, pattern := range a.Config.Clone {
		if err := cloneInto(a.Root, path, pattern); err != nil {
			return err
		}
	}
	if !a.Config.CommitTimes {
		return nil
	}
	times, err := git.CommitTimes(path)
	if err != nil {
		return err
	}
	for name, at := range times {
		// A tracked file the checkout left out (sparse, or deleted) is not an error.
		if err := os.Chtimes(filepath.Join(path, name), at, at); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// cloneInto copies what pattern matches in the main checkout to the same place in a new
// worktree. It never overwrites: what the worktree already has, it keeps.
func cloneInto(root, path, pattern string) error {
	clean := filepath.Clean(pattern)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("clone %q: name a path inside the repository", pattern)
	}
	matches, err := filepath.Glob(filepath.Join(root, clean))
	if err != nil {
		return fmt.Errorf("clone %q: %w", pattern, err)
	}
	for _, from := range matches {
		rel, err := filepath.Rel(root, from)
		if err != nil {
			return err
		}
		to := filepath.Join(path, rel)
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := copyTree(from, to); err != nil {
			return fmt.Errorf("clone %s: %w", rel, err)
		}
	}
	return nil
}

// copyTree copies a file or directory keeping modes and times, as a copy-on-write clone where
// the file system has them (APFS, btrfs, xfs) and a plain copy where it does not.
func copyTree(from, to string) error {
	cp := run.Local("cp")
	if runtime.GOOS == "darwin" {
		if _, err := cp.Capture("-cRp", from, to); err == nil {
			return nil
		}
		// -c fails outright on a volume without clones; a partial copy may be left behind.
		if err := os.RemoveAll(to); err != nil {
			return err
		}
		_, err := cp.Capture("-Rp", from, to)
		return err
	}
	_, err := cp.Capture("-R", "--reflink=auto", "--preserve=mode,timestamps,links", from, to)
	return err
}
