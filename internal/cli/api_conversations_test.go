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
	return printed(t, append([]string{"api"}, args...)...)
}

// printed is what mia printed when run with args, and how it exited.
func printed(t *testing.T, args ...string) ([]byte, int) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = output
	code := Run(args)
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
	if data, code := printed(t, "recall", "--json", "nothing", "says", "this"); code != exitOK || string(data) != "[]\n" {
		t.Fatalf("nothing recalled is an empty list: %q, exit %d", data, code)
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
	// when it started is the agent's to say and when it was last written to the disk's: both said one way
	var times []map[string]any
	utc := regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d{1,3})?Z$`)
	if err := json.Unmarshal(data, &times); err != nil || !utc.MatchString(times[0]["started"].(string)) || !utc.MatchString(times[0]["active"].(string)) {
		t.Errorf("started %v and active %v are not both UTC to the millisecond", times[0]["started"], times[0]["active"])
	}
	data, code = answered(t, "conversation", "c1", "--after", "1")
	var turns []app.Turn
	if err := json.Unmarshal(data, &turns); err != nil || code != exitOK || len(turns) != 1 || turns[0].N != 2 || turns[0].Kind != "said" || turns[0].Text != "It is faster." {
		t.Fatalf("conversation = %s, exit %d, %v", data, code, err)
	}
	if data, code = answered(t, "conversation", "c1", "--after", "2"); code != exitOK || string(data) != "[]\n" {
		t.Fatalf("nothing new is an empty list: %q, exit %d", data, code)
	}
	// by the worktree it is in, as the list names it: the main checkout's is its folder's name
	if data, code = answered(t, "conversation", "c1", "--worktree", heads[0].Worktree); code != exitOK || json.Unmarshal(data, &turns) != nil || len(turns) != 2 {
		t.Fatalf("conversation --worktree %s = %s, exit %d", heads[0].Worktree, data, code)
	}
	if _, code = answered(t, "conversation", "c1", "--worktree", "nowhere"); code != exitFailed {
		t.Errorf("a worktree it is not in exited %d", code)
	}
	if _, code = answered(t, "conversation", "nobody"); code == exitOK {
		t.Error("a conversation there is not exits non-zero")
	}
	if _, code = answered(t, "conversations", "--limit", "many"); code != exitUsage {
		t.Errorf("a limit that is no number is a usage error, exit %d", code)
	}
}

// mia conversation continue --dry-run says, as JSON too, the handoff, the
// window and the agent it would go on with, and writes and opens nothing.
func TestConversationContinueDryRunSaysWhatItWouldDo(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	t.Chdir(repo.Root)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
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

	data, code := printed(t, "conversation", "continue", "c1", "--with", "codex", "--dry-run", "--json")
	var would app.Continued
	handoffs := filepath.Join(repo.Root, ".git", "mia", "handoffs")
	main := filepath.Base(repo.Root)
	if err := json.Unmarshal(data, &would); err != nil || code != exitOK || !would.DryRun || would.To.Agent != "codex" || would.To.Window.Name != "codex" ||
		would.From != (app.ContinuedFrom{Agent: "claude", ID: "c1", Worktree: main}) || would.Turns != 2 || filepath.Dir(filepath.Dir(would.Handoff)) != handoffs {
		t.Fatalf("would = %s, exit %d, %v", data, code, err)
	}
	// the source and the target each as one thing, for a view to read: no window had the one open, and the other's own conversation is not known yet
	var shape struct {
		From, To map[string]json.RawMessage
	}
	if err := json.Unmarshal(data, &shape); err != nil || len(shape.From) != 3 || len(shape.To) != 2 || string(shape.From["id"]) != `"c1"` || shape.To["window"] == nil || shape.To["id"] != nil {
		t.Errorf("from %v and to %v, %v", shape.From, shape.To, err)
	}
	said, code := printed(t, "conversation", "continue", "c1", "--worktree", main, "--with", "pi", "--dry-run")
	if code != exitOK || !regexp.MustCompile(`(?s)claude's conversation c1, 2 turns, would be written out to .*claude-c1\.md.*a window pi would open — pi — and be sent:\nRead .*\nnot carried over: what claude thought and what came back from its tools — pi has what was said and each thing done, in a line\n$`).Match(said) {
		t.Fatalf("said = %s, exit %d", said, code)
	}
	if _, err := os.Stat(handoffs); err == nil {
		t.Error("a dry run wrote the handoff")
	}
	for _, args := range [][]string{
		{"conversation", "continue", "c1"},                                        // with no agent
		{"conversation", "continue", "--with", "codex"},                           // of no conversation
		{"conversation", "continue", "c1", "--from", "claude", "--with", "codex"}, // said two ways
		{"conversation", "continue", main, "c1", "--with", "codex"},               // a worktree is not said first
		{"conversation", "resume", "c1", "--with", "codex"},                       // resume goes on with its own
		{"conversation", "resume", "c1", "--dry-run"},                             // and has no dry run
		{"conversation", "continue", "c1", "--with", "codex", "--in", "x"},        // --in is resume's
	} {
		if _, code := printed(t, args...); code != exitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	for _, args := range [][]string{
		{"conversation", "continue", "c1", "--with", "nobody", "--dry-run"},           // an agent mia does not know
		{"conversation", "continue", "c1", "--with", "claude", "--dry-run"},           // the agent it is with: its own resume loses nothing
		{"conversation", "continue", "c9", "--with", "codex", "--dry-run"},            // no such conversation
		{"conversation", "continue", "--from", "claude", "--with", "codex", "--json"}, // a window that has none open
	} {
		if data, code := printed(t, args...); code != exitFailed || len(data) != 0 {
			t.Errorf("%v: exit %d, printed %s", args, code, data)
		}
	}
	if _, err := os.Stat(handoffs); err == nil {
		t.Error("a refusal wrote a handoff")
	}
}
