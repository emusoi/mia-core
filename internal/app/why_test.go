package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

func TestWhyFindsTheConversationThatMadeALinesCommit(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	repo.Write("a.go", "one\ntwo\n")
	repo.Commit("two lines")
	sha := strings.TrimSpace(repo.Git("rev-parse", "HEAD"))

	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root+".kijenge", "-"))
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(kind, content string) string {
		return fmt.Sprintf(`{"type":%q,"sessionId":"s1","cwd":"/w","timestamp":"2026-11-02T10:00:00Z","message":{"content":%s}}`, kind, content)
	}
	transcript := strings.Join([]string{
		line("user", `"add the second line"`),
		line("assistant", `[{"type":"tool_use","name":"Bash","input":{"command":"git commit -m x"}}]`),
		line("user", fmt.Sprintf(`[{"type":"tool_result","content":"[main %s] two lines"}]`, sha[:7])),
		line("user", `"thanks"`),
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(project, "s1.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	why, err := a.Why(filepath.Join(repo.Root, "a.go") + ":2")
	if err != nil {
		t.Fatal(err)
	}
	if why.Commit != sha || len(why.Conversations) != 1 {
		t.Fatalf("one conversation for %s: %+v", sha, why)
	}
	if c := why.Conversations[0]; !c.Made || c.Session != "s1" || c.Asked != "add the second line" {
		t.Errorf("made in s1, after you asked for it: %+v", c)
	}
	if _, err := a.Why("nothing-here"); err == nil {
		t.Error("a target naming no commit is refused")
	}
}

func TestRecallFindsTheConversationYouSaidItIn(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	write := func(worktree, session, kind, content string) {
		project := filepath.Join(claude, "projects", regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(repo.Root+worktree, "-"))
		if err := os.MkdirAll(project, 0o755); err != nil {
			t.Fatal(err)
		}
		line := fmt.Sprintf(`{"type":%q,"sessionId":%q,"cwd":%q,"timestamp":"2026-11-02T10:00:00Z","message":{"content":%s}}`+"\n", kind, session, repo.Root+worktree, content)
		file := filepath.Join(project, session+".jsonl")
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".kijenge", "mine", "user", `"fix the Retry loop in the poller"`)
	write("", "theirs", "assistant", `[{"type":"text","text":"I changed the retry loop."}]`)
	write(".temeke", "other", "user", `"nothing to see"`)
	write(".temeke", "sub/subagents/agent-1", "user", `"look into the retry loop for me"`)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	got := a.Recall("retry loop", 10)
	if len(got) != 3 {
		t.Fatalf("two conversations mention it: %+v", got)
	}
	for _, r := range got {
		if r.Session == "sub" && (r.Yours || r.N != 0) {
			t.Errorf("a subagent's prompt is claude's words, not yours, in its parent's conversation and no turn of it: %+v", r)
		}
		if (r.Session == "mine") != r.Yours || !strings.Contains(strings.ToLower(r.Line), "retry loop") || r.Agent != "claude" {
			t.Errorf("who said it, and the words around it: %+v", r)
		}
		if r.Session != "sub" && r.N != 1 {
			t.Errorf("the turn it came up in: %+v", r)
		}
	}
}
