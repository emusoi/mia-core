package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
)

// Repository is one repository mia has worked in: its main checkout.
type Repository struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Repositories is every repository mia has named a worktree in, found from
// the name registry: each worktree's repository, by its main checkout. Ones
// gone from disk, bare ones and ones in temporary folders are left out.
func Repositories() ([]Repository, error) {
	store, err := globalNames()
	if err != nil {
		return nil, err
	}
	owners, err := store.All()
	if err != nil {
		return nil, err
	}
	return repositoriesOf(owners), nil
}

func repositoriesOf(owners map[string]string) []Repository {
	temp := []string{git.Resolve(os.TempDir()), "/tmp", "/private/tmp", "/private/var/folders"}
	seen := map[string]bool{}
	out := []Repository{}
	for _, path := range owners {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		_, common, err := git.Repository(path) // by its files: a git for each worktree is most of a second
		if err != nil || filepath.Base(common) != ".git" {
			continue
		}
		root := filepath.Dir(common)
		if seen[root] || inAny(root, temp) {
			continue
		}
		seen[root] = true
		out = append(out, Repository{Name: filepath.Base(root), Path: root})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func inAny(path string, dirs []string) bool {
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
