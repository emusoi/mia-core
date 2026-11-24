package cli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/resolve"
)

// Words mean the name that is them, else the ones that start with them, hold them, or hold
// their letters in order: the best there is, and all that are as good.
func TestClosestIsTheBestMatchAndEveryOneAsGood(t *testing.T) {
	names := []string{"new-idea", "mia-desk", "mia-core", "idea", "notes"}
	for words, want := range map[string][]int{
		"idea":  {3},    // the name itself, though new-idea holds it
		"IDEA":  {3},    // whatever its case
		"new":   {0},    // starts with
		"mia":   {1, 2}, // two start with it
		"desk":  {1},    // holds it
		"nwid":  {0},    // its letters in order
		"mc":    {2},    // mia-core alone has m then c
		"zzz":   nil,
		"":      nil,
		" core": {2},
	} {
		if got := closest(words, names); !reflect.DeepEqual(got, want) {
			t.Errorf("closest(%q) = %v, want %v", words, got, want)
		}
	}
}

// From a repository the words are a worktree of it before they are a project; in none, a
// project, and `<project>/<worktree>` a worktree of that project; several as good are named
// and none is guessed.
func TestWhereToIsAWorktreeHereThenAProject(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	goba, err := a.New("goba")
	if err != nil {
		t.Fatal(err)
	}
	other := gittest.New(t)
	repos := []app.Repository{{Name: "mia-desk", Path: repo.Root}, {Name: "mia-core", Path: other.Root}, {Name: goba.Name, Path: other.Root}}

	if path, err := whereTo(a, goba.Name[:3], repos); err != nil || path != goba.Path {
		t.Fatalf("a worktree here by the start of its name = %q, %v; want %q (before the project of that name)", path, err, goba.Path)
	}
	if path, err := whereTo(a, "core", repos); err != nil || path != other.Root {
		t.Fatalf("a project from inside another = %q, %v", path, err)
	}
	if path, err := whereTo(nil, "desk", repos); err != nil || path != repo.Root {
		t.Fatalf("a project from no repository = %q, %v", path, err)
	}
	if path, err := whereTo(nil, "desk/"+goba.Name[:3], repos); err != nil || path != goba.Path {
		t.Fatalf("project/worktree = %q, %v; want %q", path, err, goba.Path)
	}
	var many several
	if _, err := whereTo(nil, "mia", repos); !errors.As(err, &many) || len(many.names) != 2 {
		t.Fatalf("two projects as good: %v", err)
	}
	if _, err := whereTo(nil, "nosuch", repos); err == nil {
		t.Fatal("a name nothing has was found")
	}
	var missing resolve.NotFound
	if _, err := worktreeOf(a, "nosuch"); !errors.As(err, &missing) {
		t.Fatalf("no such worktree: %v", err)
	}
}

// In no repository `mia ls` and `mia path <words>` answer, where they said there was no
// repository; `mia project ls` there still needs one.
func TestLsAndPathAnswerOutsideARepository(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	if code := Run([]string{"ls"}); code != exitOK {
		t.Fatalf("mia ls outside a repository exited %d", code)
	}
	if code := Run([]string{"path", "nosuch"}); code != exitFailed {
		t.Fatalf("mia path of no project exited %d", code)
	}
	if code := Run([]string{"p", "ls"}); code != exitFailed {
		t.Fatalf("mia p ls outside a repository exited %d", code)
	}
	if code := Run([]string{"__complete", "names"}); code != exitOK {
		t.Fatalf("completing names outside a repository exited %d", code)
	}
}
