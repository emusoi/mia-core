package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// pi is given the extension that says what it does only when asked: in the
// folder of the account named, whole, once, and never over a file that is
// the person's own. Nothing else of the folder is touched, a sign-in least.
func TestPiIsGivenWhatSaysWhatItDoesAndNothingOfThePersonsIsWrittenOver(t *testing.T) {
	gittest.Isolate(t)
	home, accounts := ownFolders(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(home, ".pi", "agent")
	file := filepath.Join(own, "extensions", "mia-signal.ts")
	one := func(name string, all, dry bool) app.Signalling {
		t.Helper()
		done, err := a.SignalAccount("pi", name, all, dry)
		if err != nil || len(done) != 1 {
			t.Fatalf("signals: %+v %v", done, err)
		}
		return done[0]
	}

	if _, err := a.SignalAccount("pi", "", false, false); err == nil || !strings.Contains(err.Error(), "keeps nothing") {
		t.Fatalf("a pi that was never started has no folder to write in: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi")); err == nil {
		t.Fatal("a folder was made for a pi that has none")
	}
	put(t, filepath.Join(own, "auth.json"), "a sign-in")

	if did := one("", false, true); did.Did != app.SignalsWritten || !did.DryRun || did.File != file {
		t.Errorf("a dry run says what would be written: %+v", did)
	}
	if _, err := os.Stat(filepath.Dir(file)); err == nil {
		t.Fatal("a dry run wrote")
	}
	if did := one("default", false, false); did.Did != app.SignalsWritten || did.DryRun || did.File != file {
		t.Errorf("written = %+v", did)
	}
	written, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(string(written), "// mia-signal: written by mia") {
		t.Fatalf("the extension begins with the line that says it is mia's: %.80s %v", written, err)
	}
	for _, said := range []string{`spawn("mia", ["signal", ".", ...args]`, `"agent_start"`, `"agent_settled"`, `"ui_prompt_start"`, `"--from", "pi"`} {
		if !strings.Contains(string(written), said) {
			t.Errorf("the extension has no %s", said)
		}
	}
	if entries, _ := os.ReadDir(own); len(entries) != 2 {
		t.Errorf("pi's folder has more than its sign-in and extensions: %v", entries)
	}
	if entries, _ := os.ReadDir(filepath.Dir(file)); len(entries) != 1 {
		t.Errorf("extensions has more than the one file: %v", entries)
	}
	if kept, _ := os.ReadFile(filepath.Join(own, "auth.json")); string(kept) != "a sign-in\n" {
		t.Errorf("the sign-in was touched: %q", kept)
	}

	before, _ := os.Stat(file)
	if did := one("", false, false); did.Did != app.SignalsAlready {
		t.Errorf("a second time = %+v", did)
	}
	if after, _ := os.Stat(file); !after.ModTime().Equal(before.ModTime()) {
		t.Error("a second time wrote it again")
	}

	// mia's own of another version is written again
	if err := os.WriteFile(file, []byte("// mia-signal: written by mia — an older one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if did := one("", false, true); did.Did != app.SignalsUpdated {
		t.Errorf("a dry run of an older one = %+v", did)
	}
	if now, _ := os.ReadFile(file); strings.Contains(string(now), "spawn") {
		t.Error("a dry run wrote over the older one")
	}
	if did := one("", false, false); did.Did != app.SignalsUpdated {
		t.Errorf("an older one = %+v", did)
	}
	if now, _ := os.ReadFile(file); string(now) != string(written) {
		t.Error("the older one was not written over")
	}

	// the person's own under that name is left as it is: a file, and a link
	if err := os.WriteFile(file, []byte("export default function () {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if did := one("", false, false); did.Did != app.SignalsKept {
		t.Errorf("the person's own file = %+v", did)
	}
	if now, _ := os.ReadFile(file); string(now) != "export default function () {}\n" {
		t.Errorf("the person's own file was written over: %q", now)
	}
	theirs := filepath.Join(home, "theirs.ts")
	if err := os.WriteFile(theirs, written, 0o644); err != nil { // even one that reads as mia's
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(theirs, file); err != nil {
		t.Fatal(err)
	}
	if did := one("", false, false); did.Did != app.SignalsKept {
		t.Errorf("the person's own link = %+v", did)
	}
	if to, err := os.Readlink(file); err != nil || to != theirs {
		t.Errorf("the link was replaced: %q %v", to, err)
	}

	// every account, each in its own folder; one with the person's file is said so
	work := filepath.Join(accounts, "pi", "work")
	put(t, filepath.Join(work, "auth.json"), "another sign-in")
	all, err := a.SignalAccount("pi", "", true, false)
	if err != nil || len(all) != 2 || all[0].Name != "default" || all[0].Did != app.SignalsKept ||
		all[1].Name != "work" || all[1].Did != app.SignalsWritten || all[1].File != filepath.Join(work, "extensions", "mia-signal.ts") {
		t.Fatalf("every account: %+v %v", all, err)
	}
	if now, _ := os.ReadFile(all[1].File); string(now) != string(written) {
		t.Error("the account's own is not the extension")
	}
	if _, err := a.SignalAccount("pi", "nobody", false, false); err == nil || !strings.Contains(err.Error(), "default, work") {
		t.Errorf("an account pi has not, with those it has: %v", err)
	}

	// the default account is where pi's variable says its folder is
	elsewhere := filepath.Join(home, "pi-elsewhere")
	if err := os.Mkdir(elsewhere, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", elsewhere)
	if did := one("", false, false); did.Did != app.SignalsWritten || did.File != filepath.Join(elsewhere, "extensions", "mia-signal.ts") {
		t.Errorf("where the variable says = %+v", did)
	}

	if _, err := a.SignalAccount("claude", "", false, false); err == nil || !strings.Contains(err.Error(), "hooks") {
		t.Errorf("claude's are its hooks, the person's to write: %v", err)
	}
	if _, err := a.SignalAccount("opencode", "", false, true); err == nil || !strings.Contains(err.Error(), "nothing that makes opencode") {
		t.Errorf("an agent mia has none for: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); err == nil {
		t.Error("claude's folder was made")
	}
}
