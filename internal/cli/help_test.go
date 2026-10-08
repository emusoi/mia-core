package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

func TestHelpFlagCreatesNothing(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	t.Chdir(repo.Root)
	for _, args := range [][]string{{"new", "--help"}, {"new", "-h"}} {
		if code := Run(args); code != exitOK {
			t.Fatalf("%v exited %d", args, code)
		}
	}
	if code := Run([]string{"new", "--bogus"}); code != exitUsage {
		t.Fatalf("new --bogus exited %d, want %d", code, exitUsage)
	}
	if out := repo.Git("branch", "--format=%(refname:short)"); out != "main" {
		t.Fatalf("branches created: %q", out)
	}
}

func TestVerbHelpIsThatVerbsUsage(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "help")
	if err != nil {
		t.Fatal(err)
	}
	verbUsage(out, "star")
	out.Close()
	text, _ := os.ReadFile(out.Name())
	if !strings.HasPrefix(string(text), "usage: mia star") || strings.Contains(string(text), "mia new") {
		t.Fatalf("star --help printed %q", text)
	}
}

func TestAVerbThatTakesNothingRefusesWhatItWasGiven(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	t.Chdir(repo.Root)
	for _, args := range [][]string{{"ls", "kisongo"}, {"gc", "--aply"}, {"bearings", "x"}} {
		if code := Run(args); code != exitUsage {
			t.Errorf("%v exited %d, want %d", args, code, exitUsage)
		}
	}
}

func TestAnUnknownCommandIsAUsageError(t *testing.T) {
	repo := gittest.New(t)
	gittest.Isolate(t)
	t.Chdir(repo.Root)
	if code := Run([]string{"adopt", repo.Root}); code != exitOK {
		t.Fatalf("adopt exited %d", code)
	}
	if code := Run([]string{"nosuchverb", "run"}); code != exitUsage {
		t.Errorf("nosuchverb exited %d, want %d", code, exitUsage)
	}
}

func TestAStampedVersionIsKeptAndDevSaysWhichCommit(t *testing.T) {
	if got := versionOf("1.4.0"); got != "1.4.0" {
		t.Errorf("a stamped version became %q", got)
	}
	if got := versionOf("dev"); !strings.HasPrefix(got, "dev") {
		t.Errorf("an unstamped build says %q", got)
	}
}
