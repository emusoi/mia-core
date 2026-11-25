package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/resolve"
)

// Getting around by name, from any folder: `mia ls` is the projects mia knows, `mia cd <words>` (the shell function over `mia path`) goes to a worktree of the
// repository here or else to a project, `<project>/<worktree>` to a worktree of another, and
// `mia p ls|cd` (`project`, `pj`, `prj`) are a project's worktrees alone. Names are matched loosely
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
	path, err := nameOf(a, words)
	var missing resolve.NotFound
	if err == nil || !errors.As(err, &missing) {
		return path, err
	}
	return a.Locate(words) // a stack, or a layer no worktree has out: every worktree's stack is read
}

// nameOf is worktreeOf without the stacks, which cost a git for each worktree: what a worktree
// is called exactly (its name, its branch, its folder), else loosely by name.
func nameOf(a *app.App, words string) (string, error) {
	resolver, err := a.Resolver()
	if err != nil {
		return "", err
	}
	path, err := resolver.Worktree(words)
	var missing resolve.NotFound
	if err == nil || !errors.As(err, &missing) {
		return path, err
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
			return app.Repository{}, fmt.Errorf("no worktree or project called %q", words)
		}
		return app.Repository{}, err
	}
	return repos[i], nil
}

// whereTo is the folder the words mean, asked in `here` (nil: in no repository): a worktree of
// the repository here, else a project's main checkout, and `<project>/<worktree>` a worktree
// of that project.
func whereTo(here *app.App, words string, repos []app.Repository) (string, error) {
	var missing resolve.NotFound
	if here != nil {
		path, err := nameOf(here, words)
		if err == nil || !errors.As(err, &missing) {
			return path, err
		}
	}
	project, worktree, both := strings.Cut(words, "/")
	repo, err := projectOf(project, repos)
	if err != nil {
		// (No worktree here by that name and no project: a worktree of any project, by its
		// name or its branch; then a stack of the repository here, last.)
		if path, aerr := anywhere(words, repos); aerr == nil || !errors.As(aerr, &missing) {
			return path, aerr
		}
		if here != nil {
			if path, serr := here.Locate(words); serr == nil {
				return path, nil
			}
		}
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

// cmdProjects is `mia ls`: the projects mia knows, each with its folder, the one you are in
// marked.
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
	here := ""
	if dir, err := os.Getwd(); err == nil {
		if top, err := git.Toplevel(dir); err == nil {
			here = top
		}
	}
	for _, repo := range repos {
		path := repo.Path
		if rel, err := filepath.Rel(home, path); home != "" && err == nil && !strings.HasPrefix(rel, "..") {
			path = filepath.Join("~", rel)
		}
		mark := "  "
		if here != "" && (here == repo.Path || strings.HasPrefix(here, repo.Path+".") || strings.HasPrefix(here, repo.Path+string(filepath.Separator))) {
			mark = "▸ "
		}
		fmt.Printf("%s%-*s  %s\n", mark, width, repo.Name, path)
	}
	return exitOK
}

// cmdProject is `mia project` (`prj`, `p`): `ls [project]` its worktrees (the project here,
// or the one named), `cd <worktree>` the path of one of the project here, for the shell
// function to go to.
func cmdProject(args []string, asJSON bool) int {
	const form = "mia p ls [project]  ·  mia p cd <worktree>   (also: mia project, mia pj, mia prj)"
	asJSON = take(&args, "--json") || asJSON
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

// anywhere is the worktree the words mean of every project mia knows: by its name, else by
// the branch it has out. Several as good are named `project/worktree`, for the person to say
// which.
func anywhere(words string, repos []app.Repository) (string, error) {
	var names, labels, paths []string
	for _, repo := range repos {
		a, err := app.Open(repo.Path)
		if err != nil {
			continue
		}
		resolver, err := a.Resolver()
		if err != nil {
			continue
		}
		for _, record := range resolver.Records {
			names = append(names, record.Name)
			labels = append(labels, repo.Name+"/"+record.Name)
			paths = append(paths, record.Path)
		}
	}
	found := closest(words, names)
	if len(found) == 0 { // (Its branch, read from its HEAD: a git for each is most of a second.)
		branches := make([]string, len(paths))
		for i, path := range paths {
			branches[i] = headBranch(path)
		}
		found = closest(words, branches)
	}
	switch len(found) {
	case 0:
		return "", resolve.NotFound{Query: words}
	case 1:
		return paths[found[0]], nil
	}
	all := make([]string, len(found))
	for i, at := range found {
		all[i] = labels[at]
	}
	return "", several{words: words, names: all}
}

// headBranch is the branch a worktree has out, read from its files (its `.git`, a folder or a
// line naming one, then HEAD): none when it is detached or cannot be read.
func headBranch(worktree string) string {
	dir := filepath.Join(worktree, ".git")
	if line, err := os.ReadFile(dir); err == nil {
		named, is := strings.CutPrefix(strings.TrimSpace(string(line)), "gitdir: ")
		if !is {
			return ""
		}
		if dir = named; !filepath.IsAbs(dir) {
			dir = filepath.Join(worktree, dir)
		}
	}
	head, err := os.ReadFile(filepath.Join(dir, "HEAD"))
	if err != nil {
		return ""
	}
	branch, _ := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/")
	if strings.HasPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/") {
		return branch
	}
	return ""
}
