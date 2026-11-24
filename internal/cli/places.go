package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/resolve"
)

// Getting around by name, from any folder: `mia ls` outside a repository is the projects mia
// knows, `mia cd <words>` (the shell function over `mia path`) goes to a worktree of the
// repository here or else to a project, `<project>/<worktree>` to a worktree of another, and
// `mia project ls|cd` (`prj`, `p`) are a project's worktrees alone. Names are matched loosely
// (`closest`): whoever matches best, and when several do as well, they are listed and none is
// guessed.

// closest is which of names the words mean: one that is the words (whatever its case), else
// those that start with them, else those that hold them, else those that hold their letters in
// order. More than one: as good as each other.
func closest(words string, names []string) []int {
	q := strings.ToLower(strings.TrimSpace(words))
	if q == "" {
		return nil
	}
	ranks := []func(name string) bool{
		func(name string) bool { return name == q },
		func(name string) bool { return strings.HasPrefix(name, q) },
		func(name string) bool { return strings.Contains(name, q) },
		func(name string) bool {
			at := 0
			for _, r := range name {
				if at < len(q) && rune(q[at]) == r {
					at++
				}
			}
			return at == len(q)
		},
	}
	for _, is := range ranks {
		var found []int
		for i, name := range names {
			if is(strings.ToLower(name)) {
				found = append(found, i)
			}
		}
		if len(found) > 0 {
			return found
		}
	}
	return nil
}

// several says which the words could mean, for the person to say more.
type several struct {
	words string
	names []string
}

func (e several) Error() string {
	return fmt.Sprintf("%q could be %s — say more of its name", e.words, strings.Join(e.names, ", "))
}

// one is the name the words mean, by its place in names: not found when none does, several
// when more than one does as well.
func one(words string, names []string) (int, error) {
	found := closest(words, names)
	switch len(found) {
	case 0:
		return 0, resolve.NotFound{Query: words}
	case 1:
		return found[0], nil
	}
	seen, all := map[string]bool{}, []string{}
	for _, i := range found {
		if !seen[names[i]] {
			seen[names[i]] = true
			all = append(all, names[i])
		}
	}
	if len(all) == 1 { // the same name twice (a worktree and its folder): the first
		return found[0], nil
	}
	return 0, several{words: words, names: all}
}

// worktreeIn is the path of the worktree of the repository at root that the words mean: by
// what mia calls it exactly (its name, its branch, its folder, a stack), else loosely by name.
func worktreeIn(root, words string) (string, error) {
	a, err := app.Open(root)
	if err != nil {
		return "", err
	}
	return worktreeOf(a, words)
}

func worktreeOf(a *app.App, words string) (string, error) {
	path, err := a.Locate(words)
	var missing resolve.NotFound
	if err == nil || !errors.As(err, &missing) {
		return path, err
	}
	resolver, rerr := a.Resolver()
	if rerr != nil {
		return "", rerr
	}
	names := make([]string, len(resolver.Records))
	for i, record := range resolver.Records {
		names[i] = record.Name
	}
	i, err := one(words, names)
	if err != nil {
		return "", err
	}
	return resolver.Records[i].Path, nil
}

// projectOf is the project the words mean, of those mia knows.
func projectOf(words string, repos []app.Repository) (app.Repository, error) {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	i, err := one(words, names)
	if err != nil {
		var missing resolve.NotFound
		if errors.As(err, &missing) {
			return app.Repository{}, fmt.Errorf("no worktree here and no project called %q — `mia ls` outside a project shows them", words)
		}
		return app.Repository{}, err
	}
	return repos[i], nil
}

// whereTo is the folder the words mean, asked in `here` (nil: in no repository): a worktree of
// the repository here, else a project's main checkout, and `<project>/<worktree>` a worktree
// of that project.
func whereTo(here *app.App, words string, repos []app.Repository) (string, error) {
	if here != nil {
		path, err := worktreeOf(here, words)
		var missing resolve.NotFound
		if err == nil || !errors.As(err, &missing) {
			return path, err
		}
	}
	project, worktree, both := strings.Cut(words, "/")
	repo, err := projectOf(project, repos)
	if err != nil {
		return "", err
	}
	if !both || worktree == "" {
		return repo.Path, nil
	}
	return worktreeIn(repo.Path, worktree)
}

// cmdPathAnywhere is `mia path` (and so `mia cd`) with words, from any folder.
func cmdPathAnywhere(here *app.App, args []string) int {
	if len(args) != 1 {
		return usageErr("mia path [worktree|stack|project|project/worktree]")
	}
	repos, err := app.Repositories()
	if err != nil {
		return fail(err)
	}
	path, err := whereTo(here, args[0], repos)
	if err != nil {
		return fail(err)
	}
	fmt.Println(path)
	return exitOK
}

// cmdProjects is `mia ls` in no repository: the projects mia knows, each with its folder and
// how many worktrees mia has named in it.
func cmdProjects(asJSON bool) int {
	repos, err := app.Repositories()
	if err != nil {
		return fail(err)
	}
	if asJSON {
		return emit(repos)
	}
	if len(repos) == 0 {
		fmt.Println("no projects yet — `mia new <branch>` in a repository makes it one")
		return exitOK
	}
	width := 0
	for _, repo := range repos {
		width = max(width, len(repo.Name))
	}
	home, _ := os.UserHomeDir()
	for _, repo := range repos {
		path := repo.Path
		if rel, err := filepath.Rel(home, path); home != "" && err == nil && !strings.HasPrefix(rel, "..") {
			path = filepath.Join("~", rel)
		}
		fmt.Printf("%-*s  %s\n", width, repo.Name, path)
	}
	return exitOK
}

// cmdProject is `mia project` (`prj`, `p`): `ls [project]` its worktrees (the project here,
// or the one named), `cd <worktree>` the path of one of the project here, for the shell
// function to go to.
func cmdProject(args []string, asJSON bool) int {
	const form = "mia project ls [project]  ·  mia project cd <worktree>   (also: mia prj, mia p)"
	sub := "ls"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	here, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	switch sub {
	case "ls", "list":
		if len(args) > 1 {
			return usageErr(form)
		}
		if len(args) == 1 {
			repos, err := app.Repositories()
			if err != nil {
				return fail(err)
			}
			repo, err := projectOf(args[0], repos)
			if err != nil {
				return fail(err)
			}
			here = repo.Path
		}
		a, err := app.Open(here)
		if err != nil {
			return fail(err)
		}
		return cmdList(a, asJSON)
	case "cd", "path":
		if len(args) != 1 {
			return usageErr(form)
		}
		path, err := worktreeIn(here, args[0])
		if err != nil {
			return fail(err)
		}
		fmt.Println(path)
		return exitOK
	}
	return usageErr(form)
}
