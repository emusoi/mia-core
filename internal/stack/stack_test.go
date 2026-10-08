package stack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/stack"
)

func store(t *testing.T) stack.Store {
	t.Helper()
	return stack.Store{Path: filepath.Join(t.TempDir(), "stack.json")}
}

func TestAddingALayerInTheMiddleInserts(t *testing.T) {
	repo := gittest.New(t)
	s := store(t)

	for _, edge := range [][2]string{{"schema", "main"}, {"api", "schema"}, {"ui", "api"}} {
		repo.Git("branch", edge[0])
		if err := s.Record(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	repo.Git("branch", "backfill")
	if err := s.Record("backfill", "schema"); err != nil {
		t.Fatal(err)
	}

	layers, err := s.Of(repo.Root, "backfill", "main")
	if err != nil {
		t.Fatal(err)
	}
	got := names(layers)
	want := []string{"schema", "backfill", "api", "ui"}
	if len(got) != len(want) {
		t.Fatalf("the stack is %v, want %v — a layer went missing", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the stack is %v, want %v", got, want)
		}
	}
	for _, layer := range layers {
		if layer.Branch == "api" && layer.Parent != "backfill" {
			t.Errorf("api still sits on %s", layer.Parent)
		}
	}
}

func TestRemovingALayerRejoinsTheStack(t *testing.T) {
	repo := gittest.New(t)
	s := store(t)
	for _, edge := range [][2]string{{"schema", "main"}, {"api", "schema"}, {"ui", "api"}} {
		repo.Git("branch", edge[0])
		if err := s.Record(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Forget("api"); err != nil {
		t.Fatal(err)
	}
	layers, err := s.Of(repo.Root, "ui", "main")
	if err != nil {
		t.Fatal(err)
	}
	got := names(layers)
	if len(got) != 2 || got[0] != "schema" || got[1] != "ui" {
		t.Fatalf("after removing the middle layer the stack is %v, want [schema ui]", got)
	}
}

func TestLandingIsDerivedFromGit(t *testing.T) {
	repo := gittest.New(t)
	s := store(t)
	repo.Git("branch", "schema")
	if err := s.Record("schema", "main"); err != nil {
		t.Fatal(err)
	}

	layers, err := s.Of(repo.Root, "schema", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !layers[0].Landed {
		t.Error("a branch with nothing on top of main was not read as landed")
	}

	repo.GitIn(repo.Root, "checkout", "schema")
	repo.Write("added.txt", "x")
	repo.Commit("Add the column")
	repo.GitIn(repo.Root, "checkout", "main")

	layers, err = s.Of(repo.Root, "schema", "main")
	if err != nil {
		t.Fatal(err)
	}
	if layers[0].Landed {
		t.Error("a branch with a commit main does not have was read as landed")
	}
	if layers[0].Ahead != 1 {
		t.Errorf("it is %d commits ahead, want 1", layers[0].Ahead)
	}
}

func names(layers []stack.Layer) []string {
	var out []string
	for _, layer := range layers {
		out = append(out, layer.Branch)
	}
	return out
}

func TestAStackCanBeNamedAndTheOldFlatFileStillReads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.json")
	if err := os.WriteFile(path, []byte(`{"api":"schema","schema":"main"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := stack.Store{Path: path}
	if err := s.Name("payments", "schema"); err != nil {
		t.Fatal(err)
	}
	if b, ok := s.Bottom("payments"); !ok || b != "schema" {
		t.Errorf("bottom of payments = %q %v", b, ok)
	}
	if s.NameOf("schema") != "payments" {
		t.Error("the bottom does not know its name")
	}
	if err := s.Name("billing", "schema"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Bottom("payments"); ok {
		t.Error("renaming kept the old name")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"api": "schema"`) {
		t.Errorf("naming lost the edges:\n%s", data)
	}
}

func TestTheBaseBranchIsNotAStackMember(t *testing.T) {
	s := store(t)
	if err := s.Record("schema", "main"); err != nil {
		t.Fatal(err)
	}
	if err := s.Record("api", "schema"); err != nil {
		t.Fatal(err)
	}
	if s.Member("main") {
		t.Error("the base branch was treated as a layer because another layer sits on it")
	}
	for _, branch := range []string{"schema", "api"} {
		if !s.Member(branch) {
			t.Errorf("recorded layer %q is not a stack member", branch)
		}
	}
}

func TestMemberOrParentFindsAnImplicitBottom(t *testing.T) {
	s := store(t)
	if err := s.Record("top", "bottom"); err != nil {
		t.Fatal(err)
	}
	if s.Member("bottom") {
		t.Fatal("bottom unexpectedly recorded as a member")
	}
	for _, branch := range []string{"top", "bottom"} {
		if !s.MemberOrParent(branch) {
			t.Errorf("%q not found in its stack", branch)
		}
	}
	for _, branch := range []string{"", "unrelated"} {
		if s.MemberOrParent(branch) {
			t.Errorf("%q treated as part of a stack", branch)
		}
	}
}

func TestALandedLayerNeverNeedsARestack(t *testing.T) {
	repo := gittest.New(t)
	s := store(t)
	repo.Git("branch", "schema")
	if err := s.Record("schema", "main"); err != nil {
		t.Fatal(err)
	}
	repo.Write("later.txt", "main moved on")
	repo.Commit("Main moves on")

	layers, err := s.Of(repo.Root, "schema", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !layers[0].Landed || layers[0].NeedsRestack {
		t.Errorf("landed %v, needs restack %v; want landed and no restack", layers[0].Landed, layers[0].NeedsRestack)
	}
}
