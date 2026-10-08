package names

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func store(t *testing.T) *Store {
	t.Helper()
	return &Store{Path: filepath.Join(t.TempDir(), "names.json")}
}

func TestAllocationIsPredictable(t *testing.T) {
	s := store(t)
	first, err := s.Allocate("/w/app.one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Allocate("/w/app.two")
	if err != nil {
		t.Fatal(err)
	}
	if first != pool[0] || second != pool[1] {
		t.Fatalf("got %q then %q, want %q then %q — allocation must be first-free in list order", first, second, pool[0], pool[1])
	}
}

func TestAllocationIsIdempotent(t *testing.T) {
	s := store(t)
	first, _ := s.Allocate("/w/app.one")
	again, err := s.Allocate("/w/app.one")
	if err != nil {
		t.Fatal(err)
	}
	if again != first {
		t.Errorf("a second allocation for the same worktree gave %q, want %q", again, first)
	}
	all, _ := s.All()
	if len(all) != 1 {
		t.Errorf("registry holds %d names for one worktree", len(all))
	}
}

func TestReleaseReturnsTheNameToThePool(t *testing.T) {
	s := store(t)
	name, _ := s.Allocate("/w/app.one")
	if err := s.Release("/w/app.one"); err != nil {
		t.Fatal(err)
	}
	if _, held := s.Owner(name); held {
		t.Errorf("%q still has an owner after release", name)
	}
	reused, _ := s.Allocate("/w/app.two")
	if reused != name {
		t.Errorf("the freed name was not reused: got %q, want %q", reused, name)
	}
	if err := s.Release("/w/app.never"); err != nil {
		t.Errorf("releasing an unheld owner failed: %v", err)
	}
}

func TestTheOwnerIsAWorktreePath(t *testing.T) {
	s := store(t)
	if _, err := s.Allocate("app.monduli"); err == nil {
		t.Error("a relative path was accepted as an owner")
	} else if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("the refusal should say why: %v", err)
	}

	name, _ := s.Allocate("/w/app.monduli")
	owner, ok := s.Owner(name)
	if !ok || owner != "/w/app.monduli" {
		t.Errorf("owner of %q = %q, %v — it must be the worktree itself", name, owner, ok)
	}
}

func TestConcurrentAllocationNeverHandsOutOneNameTwice(t *testing.T) {
	s := store(t)
	const workers = 12

	var wg sync.WaitGroup
	got := make([]string, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, err := s.Allocate(filepath.Join("/w", "app."+string(rune('a'+i))))
			if err != nil {
				t.Error(err)
				return
			}
			got[i] = name
		}()
	}
	wg.Wait()

	seen := map[string]int{}
	for i, name := range got {
		if name == "" {
			t.Fatalf("worker %d got no name", i)
		}
		seen[name]++
	}
	for name, count := range seen {
		if count > 1 {
			t.Errorf("%q was handed out %d times", name, count)
		}
	}
	if len(seen) != workers {
		t.Errorf("%d workers received %d distinct names", workers, len(seen))
	}
}

func TestThePoolDoesNotRunOut(t *testing.T) {
	taken := map[string]bool{}
	for _, name := range pool {
		taken[name] = true
	}
	next := firstFree(taken)
	if next != pool[0]+"-2" {
		t.Errorf("after the pool is exhausted, got %q, want %q", next, pool[0]+"-2")
	}
}

func TestANameDiffersOnlyInCaseIsTaken(t *testing.T) {
	s := store(t)
	if err := s.Claim("kisongo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("KISONGO", t.TempDir()); err == nil {
		t.Error("KISONGO was handed to a second worktree; as a hostname it is kisongo")
	}
}

func TestAllocationSkipsANameTakenInAnotherCase(t *testing.T) {
	s := store(t)
	if err := s.Claim(strings.ToUpper(pool[0]), "/w/app.one"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Allocate("/w/app.two"); err != nil || strings.EqualFold(got, pool[0]) {
		t.Errorf("allocated %q %v, a name already held in capitals", got, err)
	}
}

func TestANameHeldByAFolderThatIsGoneCanBeClaimed(t *testing.T) {
	s := store(t)
	gone := filepath.Join(t.TempDir(), "deleted-repo")
	if err := s.Claim("project", gone); err != nil {
		t.Fatal(err)
	}
	here := t.TempDir()
	if err := s.Claim("project", here); err != nil {
		t.Errorf("project is still held by %s, which no longer exists: %v", gone, err)
	}
	if owner, _ := s.Owner("project"); owner != here {
		t.Errorf("project belongs to %q", owner)
	}
	if err := s.Claim("project", t.TempDir()); err == nil {
		t.Error("a name held by a folder that exists was taken")
	}
}
