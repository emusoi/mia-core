package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/resolve"

	"github.com/emusoi/mia-core/internal/run"
)

func (a *App) Locate(query string) (string, error) {
	if query == "" {
		return a.Root, nil
	}
	resolver, err := a.Resolver()
	if err != nil {
		return "", err
	}
	path, err := resolver.Worktree(query)
	var missing resolve.NotFound
	if err == nil || !errors.As(err, &missing) {
		return path, err
	}
	bottom, named := a.Stacks().Bottom(query)
	if !named {
		bottom = query
	}
	for _, record := range resolver.Records {
		branch := git.CurrentBranch(record.Path)
		if branch == "" {
			continue
		}
		layers, err := a.Stacks().Of(record.Path, branch, a.BaseBranch(record.Path))
		if err != nil {
			continue
		}
		for _, layer := range layers {
			if layer.Branch == bottom {
				return record.Path, nil
			}
		}
	}
	if !named {
		return "", err
	}
	return "", fmt.Errorf("stack %s (bottom %s) is not checked out anywhere", query, bottom)
}

func (a *App) StackName(record model.Record, name string) (string, error) {
	branch := git.CurrentBranch(record.Path)
	if branch == "" {
		return "", fmt.Errorf("%s is detached, and a stack is made of branches", record.Name)
	}
	layers, err := a.Stacks().Of(record.Path, branch, a.BaseBranch(record.Path))
	if err != nil {
		return "", err
	}
	if len(layers) == 0 {
		return "", fmt.Errorf("%s is not in a stack — `mia new <name> --stack` starts one", branch)
	}
	return layers[0].Branch, a.Stacks().Name(name, layers[0].Branch)
}

type Abandoned struct {
	Record   model.Record `json:"record"`
	Branch   string       `json:"branch"`
	LastWork time.Time    `json:"last_work"`
	Size     string       `json:"size"`
}

func (a *App) Abandoned(olderThan time.Duration, now time.Time) ([]Abandoned, error) {
	listings, err := a.List()
	if err != nil {
		return nil, err
	}
	records, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	byPath := map[string]model.Record{}
	for _, record := range records {
		byPath[record.Path] = record
	}
	var out []Abandoned
	for _, l := range listings {
		if l.Main || !l.Adopted || l.Dirty || l.Session || l.Starred {
			continue
		}
		last := git.LastCommitTime(l.Path)
		if last.IsZero() || now.Sub(last) < olderThan {
			continue
		}
		out = append(out, Abandoned{Record: byPath[l.Path], Branch: l.Branch, LastWork: last, Size: diskSize(l.Path)})
	}
	return out, nil
}

func diskSize(path string) string {
	out, err := run.Local("du").Capture("-sh", path)
	if err != nil {
		return "?"
	}
	return strings.Fields(string(out))[0]
}

func (a *App) Collect(one Abandoned) error {
	removal, err := a.PlanRemoval(one.Record.Name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(one.Record.Path); err != nil {
		return err
	}
	return a.Remove(removal, false, false)
}
