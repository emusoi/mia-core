package app

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Each carrier finds the worktree's newest conversation where its program
// keeps it, packs it under the new path with the folder inside rewritten,
// and says how to pick it up again.
func TestCarriersPackTheNewestConversationUnderTheNewPath(t *testing.T) {
	from, to := "/home/a/src/x.old", "/Users/b/src/x.new"
	cases := []struct {
		program, old, newer, there, command string
	}{
		{"claude", ".claude/projects/-home-a-src-x-old/1.jsonl", ".claude/projects/-home-a-src-x-old/2.jsonl", ".claude/projects/-Users-b-src-x-new/2.jsonl", "claude --continue"},
		{"codex", ".codex/sessions/2026/01/01/rollout-2026-01-01T00-00-00-aaaaaaaa-0000-0000-0000-000000000000.jsonl", ".codex/sessions/2026/01/02/rollout-2026-01-02T00-00-00-bbbbbbbb-0000-0000-0000-000000000000.jsonl", ".codex/sessions/2026/01/02/rollout-2026-01-02T00-00-00-bbbbbbbb-0000-0000-0000-000000000000.jsonl", "codex resume bbbbbbbb-0000-0000-0000-000000000000"},
		{"pi", ".pi/agent/sessions/--home-a-src-x.old--/1.jsonl", ".pi/agent/sessions/--home-a-src-x.old--/2.jsonl", ".pi/agent/sessions/--Users-b-src-x.new--/2.jsonl", "pi --continue"},
	}
	for _, c := range cases {
		t.Run(c.program, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			for _, f := range []string{c.old, c.newer} {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(home, f)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, f), []byte(`{"cwd":"`+from+`","file":"`+f+`"}`+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(filepath.Join(home, c.newer), time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			local := side{}
			packed, err := local.bytesOf(carriers[c.program], from, to, "pack")
			if err != nil || len(packed) == 0 {
				t.Fatalf("pack: %v, %d bytes", err, len(packed))
			}
			files := untar(t, packed)
			got, ok := files[c.there]
			if !ok {
				t.Fatalf("packed %v, wanted %s", keys(files), c.there)
			}
			if !strings.Contains(got, `"cwd":"`+to+`"`) || !strings.Contains(got, c.newer) {
				t.Errorf("packed %q: the folder should read %s, the file be the newest", got, to)
			}
			if cmd, _ := local.sh(carriers[c.program], from, to, "command"); strings.TrimSpace(cmd) != c.command {
				t.Errorf("command %q, wanted %q", strings.TrimSpace(cmd), c.command)
			}
		})
	}
}

func TestACarrierPacksNothingWhenTheWorktreeHadNoConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	local := side{}
	for program, script := range carriers {
		if packed, err := local.bytesOf(script, "/nowhere/x", "/elsewhere/x", "pack"); err != nil || len(packed) != 0 {
			t.Errorf("%s: %v, %d bytes; wanted nothing, quietly", program, err, len(packed))
		}
	}
}

func untar(t *testing.T, packed []byte) map[string]string {
	t.Helper()
	files := map[string]string{}
	r := tar.NewReader(bytes.NewReader(packed))
	for {
		h, err := r.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		files[strings.TrimPrefix(filepath.Clean(h.Name), "./")] = string(b)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A conversation is carried for an agent in any pane, however ps shows it,
// and once for a folder though two panes run the same agent.
func TestCarryReachesEveryPaneOfAWindow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	from, to := "/home/a/src/x.old", "/Users/b/src/x.new"
	said := filepath.Join(home, ".claude/projects/-home-a-src-x-old/1.jsonl")
	if err := os.MkdirAll(filepath.Dir(said), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(said, []byte(`{"cwd":"`+from+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	windows := []Window{{Name: "work", Splits: []Pane{{Command: "/opt/bin/claude --model opus"}, {Command: "claude"}, {Command: "npm run dev"}}}}
	carried := (*App)(nil).carry(side{}, side{}, from, to, windows)
	if len(carried) != 1 || carried[0] != "claude" {
		t.Errorf("carried %v, wanted claude once", carried)
	}
	if s := windows[0].Splits; s[0].Command != "claude --continue" || s[1].Command != "claude --continue" || s[2].Command != "npm run dev" || windows[0].Command != "" {
		t.Errorf("each claude pane continues, the rest as they were: %+v", windows[0])
	}
	if _, err := os.Stat(filepath.Join(home, ".claude/projects/-Users-b-src-x-new/1.jsonl")); err != nil {
		t.Errorf("the conversation is under the new path: %v", err)
	}
}
