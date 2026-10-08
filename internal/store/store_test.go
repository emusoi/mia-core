package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/emusoi/mia-core/internal/model"
)

func store(t *testing.T) Store {
	t.Helper()
	return Store{Dir: filepath.Join(t.TempDir(), "mia")}
}

func record(path, name string) model.Record {
	return model.Record{Path: path, Name: name}
}

func TestAnUnknownRepositoryIsEmptyNotBroken(t *testing.T) {
	records, err := store(t).Load()
	if err != nil {
		t.Fatalf("loading a repository with no records failed: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records from nothing", len(records))
	}
}

func TestPutLoadRemove(t *testing.T) {
	s := store(t)
	if err := s.Put(record("/w/app.monduli", "monduli")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(record("/w/app.longido", "longido")); err != nil {
		t.Fatal(err)
	}
	records, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].Path != "/w/app.longido" {
		t.Errorf("records are not ordered by path: %v", records[0].Path)
	}

	if err := s.Put(model.Record{Path: "/w/app.monduli", Name: "monduli", Env: &model.Environment{Placement: model.Local()}}); err != nil {
		t.Fatal(err)
	}
	records, _ = s.Load()
	if len(records) != 2 {
		t.Fatalf("replacing a record changed the count to %d", len(records))
	}

	if err := s.Remove("/w/app.monduli"); err != nil {
		t.Fatal(err)
	}
	records, _ = s.Load()
	if len(records) != 1 {
		t.Errorf("after removal, %d records remain", len(records))
	}
	if err := s.Remove("/w/app.never"); err != nil {
		t.Errorf("removing an absent record failed: %v", err)
	}
}

func TestTheSetInvariantsAreEnforced(t *testing.T) {
	s := store(t)

	err := s.Update(func([]model.Record) ([]model.Record, error) {
		return []model.Record{record("/w/app.a", "monduli"), record("/w/app.a", "longido")}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "one identity") {
		t.Errorf("two records for one path were accepted: %v", err)
	}

	err = s.Update(func([]model.Record) ([]model.Record, error) {
		return []model.Record{record("/w/app.a", "monduli"), record("/w/app.b", "monduli")}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "hostname") {
		t.Errorf("one name on two worktrees was accepted, and a name becomes a hostname: %v", err)
	}

	if records, _ := s.Load(); len(records) != 0 {
		t.Errorf("a refused update wrote %d records", len(records))
	}
}

func TestAPlacementSurvivesTheFile(t *testing.T) {
	s := store(t)
	placed, err := model.OnRuntime("fedora", "/w/app.lane2")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(model.Record{Path: "/w/app.monduli", Name: "monduli", Env: &model.Environment{Placement: placed}}); err != nil {
		t.Fatal(err)
	}
	records, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	back := records[0].Env.Placement
	if back != placed {
		t.Fatalf("placement changed through the file: %+v → %+v", placed, back)
	}
	staging, ok := back.Staging()
	if !ok || staging != "/w/app.lane2" {
		t.Errorf("staging = %q, %v after a round trip", staging, ok)
	}
}

func TestABrokenFileIsRefusedWhereItIsRead(t *testing.T) {
	s := store(t)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(), []byte(`[{"path":"relative/path","name":"monduli"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(); err == nil {
		t.Error("a record with a relative path was loaded without complaint")
	} else if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("the refusal should name the problem: %v", err)
	}
}

func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	s := store(t)
	const workers = 10

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			letter := string(rune('a' + i))
			if err := s.Put(record("/w/app."+letter, "name"+letter)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	records, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != workers {
		t.Errorf("%d concurrent writers left %d records", workers, len(records))
	}
}
