package resolve

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
)

type Repo struct {
	Root    string
	Records []model.Record
}

type NotFound struct{ Query string }

func (e NotFound) Error() string {
	return fmt.Sprintf("no worktree called %q — `mia ls` shows what exists", e.Query)
}

func (r Repo) Worktree(query string) (string, error) {
	if query == "" {
		return "", NotFound{Query: query}
	}

	if strings.ContainsAny(query, string(filepath.Separator)) || query == "." {
		if top, err := git.Toplevel(query); err == nil {
			return top, nil
		}
	}

	for _, record := range r.Records {
		if strings.EqualFold(record.Name, query) {
			return record.Path, nil
		}
	}

	trees, err := git.Worktrees(r.Root)
	if err != nil {
		return "", err
	}

	for _, tree := range trees {
		if tree.Branch != "" && tree.Branch == query {
			return tree.Path, nil
		}
	}

	for _, tree := range trees {
		if filepath.Base(tree.Path) == query {
			return tree.Path, nil
		}
	}

	return "", NotFound{Query: query}
}

func (r Repo) Record(query string) (model.Record, error) {
	path, err := r.Worktree(query)
	if err != nil {
		return model.Record{}, err
	}
	for _, record := range r.Records {
		if record.Path == path {
			return record, nil
		}
	}
	return model.Record{}, fmt.Errorf("%s is a worktree of this repository but mia has no record of it — `mia adopt %s` to take it over", path, query)
}

func (r Repo) Here(dir string) (string, error) {
	top, err := git.Toplevel(dir)
	if err != nil {
		return "", fmt.Errorf("not inside a worktree of this repository")
	}
	return top, nil
}
