package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

// What claude says a pane's claude has open is read by mia itself, nothing
// run: the sessions/<pid>.json of a process that leads a group in the
// pane's session, in the account's folder or one of mia's; one of a
// process of another session is not the pane's.
func TestWhatClaudeHoldsIsReadOfTheProcessInThePane(t *testing.T) {
	gittest.Isolate(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	// a pane's program, which leads its session as tmux starts it
	pane := exec.Command("sleep", "30")
	pane.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := pane.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pane.Process.Kill(); pane.Wait() })
	pid := pane.Process.Pid
	if said := claudeHolds(pid); said.ID != "" {
		t.Fatalf("nothing written, and it holds %+v", said)
	}
	// this process is of another session: what it would say is no pane's
	written(t, filepath.Join(claude, "sessions", strconv.Itoa(os.Getpid())+".json"), `{"sessionId":"not-the-pane"}`)
	if said := claudeHolds(pid); said.ID != "" {
		t.Fatalf("another session's: %+v", said)
	}
	account := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "accounts", "claude", "work")
	written(t, filepath.Join(account, "sessions", strconv.Itoa(pid)+".json"), `{"sessionId":"s1","cwd":"/w","startedAt":1793613600000}`)
	if said := claudeHolds(pid); said.ID != "s1" || said.Cwd != "/w" || said.Started.UnixMilli() != 1793613600000 {
		t.Fatalf("on one of mia's accounts: %+v", said)
	}
	if said := (Agent{}).holding(pid); said.ID != "" {
		t.Errorf("an agent that does not say: %+v", said)
	}
}

// agy's record of the folder each conversation was made in is read once
// for a list of them all, and again once agy has written it.
func TestAgysFoldersAreReadAgainOnlyOnceWritten(t *testing.T) {
	home := t.TempDir()
	written(t, filepath.Join(home, "history.jsonl"), `{"workspace":"/w1","conversationId":"c1"}`)
	first := agyFoldersNow(home)
	if first["c1"] != "/w1" {
		t.Fatalf("folders = %v", first)
	}
	first["kept"] = "as read" // what is kept is what is given again
	if again := agyFoldersNow(home); again["kept"] != "as read" {
		t.Errorf("read again with nothing written: %v", again)
	}
	written(t, filepath.Join(home, "history.jsonl"), `{"workspace":"/w2","conversationId":"c2"}`)
	if again := agyFoldersNow(home); again["c2"] != "/w2" || again["kept"] != "" {
		t.Errorf("written since, and not read again: %v", again)
	}
}

func written(t *testing.T, file, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
