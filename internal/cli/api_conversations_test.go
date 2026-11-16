package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// answered is what a mia api query printed, and how it exited.
func answered(t *testing.T, args ...string) ([]byte, int) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = output
	code := Run(append([]string{"api"}, args...))
	os.Stdout = previous
	output.Close()
	data, _ := os.ReadFile(output.Name())
	return data, code
}

func TestConversationsAPIListsThemAndReadsOneAsTurns(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", "")

	data, code := answered(t, "conversations")
	if code != exitOK || string(data) != "[]\n" {
		t.Fatalf("none is an empty list: %q, exit %d", data, code)
	}
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root, "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	root, _ := json.Marshal(repo.Root)
	line := `{"type":"user","cwd":` + string(root) + `,"timestamp":"2026-11-02T10:00:00Z","message":{"content":"make the list faster"}}` + "\n" +
		`{"type":"assistant","cwd":` + string(root) + `,"timestamp":"2026-11-02T10:00:05Z","message":{"content":[{"type":"text","text":"It is faster."}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(project, "c1.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	data, code = answered(t, "conversations", "--agent", "claude", "--limit", "5")
	var heads []app.Head
	if err := json.Unmarshal(data, &heads); err != nil || code != exitOK || len(heads) != 1 || heads[0].ID != "c1" || heads[0].Asked != "make the list faster" || heads[0].Turns != 1 {
		t.Fatalf("conversations = %s, exit %d, %v", data, code, err)
	}
	data, code = answered(t, "conversation", "c1", "--after", "1")
	var turns []app.Turn
	if err := json.Unmarshal(data, &turns); err != nil || code != exitOK || len(turns) != 1 || turns[0].N != 2 || turns[0].Kind != "said" || turns[0].Text != "It is faster." {
		t.Fatalf("conversation = %s, exit %d, %v", data, code, err)
	}
	if data, code = answered(t, "conversation", "c1", "--after", "2"); code != exitOK || string(data) != "[]\n" {
		t.Fatalf("nothing new is an empty list: %q, exit %d", data, code)
	}
	if _, code = answered(t, "conversation", "nobody"); code == exitOK {
		t.Error("a conversation there is not exits non-zero")
	}
	if _, code = answered(t, "conversations", "--limit", "many"); code != exitUsage {
		t.Errorf("a limit that is no number is a usage error, exit %d", code)
	}
}
