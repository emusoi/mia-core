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
// and says how to pick it up again: what it packs is under the folder of
// the account, the agent's own here.
func TestCarriersPackTheNewestConversationUnderTheNewPath(t *testing.T) {
	from, to := "/home/a/src/x.old", "/Users/b/src/x.new"
	cases := []struct {
		program, old, newer, there, command string
	}{
		{"claude", ".claude/projects/-home-a-src-x-old/1.jsonl", ".claude/projects/-home-a-src-x-old/2.jsonl", "projects/-Users-b-src-x-new/2.jsonl", "claude --continue"},
		{"codex", ".codex/sessions/2026/01/01/rollout-2026-01-01T00-00-00-aaaaaaaa-0000-0000-0000-000000000000.jsonl", ".codex/sessions/2026/01/02/rollout-2026-01-02T00-00-00-bbbbbbbb-0000-0000-0000-000000000000.jsonl", "sessions/2026/01/02/rollout-2026-01-02T00-00-00-bbbbbbbb-0000-0000-0000-000000000000.jsonl", "codex resume bbbbbbbb-0000-0000-0000-000000000000"},
		{"pi", ".pi/agent/sessions/--home-a-src-x.old--/1.jsonl", ".pi/agent/sessions/--home-a-src-x.old--/2.jsonl", "sessions/--Users-b-src-x.new--/2.jsonl", "pi --continue"},
	}
	for _, c := range cases {
		t.Run(c.program, func(t *testing.T) {
			home := t.TempDir()
			unaccounted(t, home)
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
			agent, _ := agentNamed(builtin, c.program)
			packed, err := local.bytesOf(carrierOf(agent, ""), from, to, "pack")
			if err != nil || len(packed) == 0 {
				t.Fatalf("pack: %v, %d bytes", err, len(packed))
			}
			files := untar(t, packed)
			for name := range files {
				// macOS tar adds one for each file with attributes of its own; on Linux
				// it is litter, and the newest *.jsonl there the next time
				if strings.HasPrefix(filepath.Base(name), "._") {
					t.Errorf("packed %s: an AppleDouble file must not cross", name)
				}
			}
			got, ok := files[c.there]
			if !ok {
				t.Fatalf("packed %v, wanted %s", keys(files), c.there)
			}
			if !strings.Contains(got, `"cwd":"`+to+`"`) || !strings.Contains(got, c.newer) {
				t.Errorf("packed %q: the folder should read %s, the file be the newest", got, to)
			}
			if cmd, _ := local.sh(carrierOf(agent, ""), from, to, "command"); strings.TrimSpace(cmd) != c.command {
				t.Errorf("command %q, wanted %q", strings.TrimSpace(cmd), c.command)
			}
		})
	}
}

func TestACarrierPacksNothingWhenTheWorktreeHadNoConversation(t *testing.T) {
	unaccounted(t, t.TempDir())
	local := side{}
	for program := range carriers {
		agent, _ := agentNamed(builtin, program)
		if packed, err := local.bytesOf(carrierOf(agent, ""), "/nowhere/x", "/elsewhere/x", "pack"); err != nil || len(packed) != 0 {
			t.Errorf("%s: %v, %d bytes; wanted nothing, quietly", program, err, len(packed))
		}
	}
}

// unaccounted makes home the home directory, with no agent told of a
// folder of its own and mia's config under it.
func unaccounted(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, agent := range builtin {
		if agent.AccountEnv != "" {
			t.Setenv(agent.AccountEnv, "")
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
	unaccounted(t, home)
	from, to := "/home/a/src/x.old", "/Users/b/src/x.new"
	said := filepath.Join(home, ".claude/projects/-home-a-src-x-old/1.jsonl")
	if err := os.MkdirAll(filepath.Dir(said), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(said, []byte(`{"cwd":"`+from+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	windows := []Window{{Name: "work", Splits: []Pane{{Command: "/opt/bin/claude --model opus"}, {Command: "claude"}, {Command: "npm run dev"}}}}
	carried := (*App)(nil).carry(side{}, side{}, from, to, windows, io.Discard)
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

// Given a conversation's id, a carrier packs that one and not the newest.
func TestCarriersPackTheConversationNamed(t *testing.T) {
	from, to := "/home/a/src/x_old", "/Users/b/src/x.new"
	cases := []struct {
		program, id, named, newest, there string
	}{
		{"claude", "c1", ".claude/projects/-home-a-src-x-old/c1.jsonl", ".claude/projects/-home-a-src-x-old/c2.jsonl", "projects/-Users-b-src-x-new/c1.jsonl"},
		{"codex", "aaaaaaaa-0000", ".codex/sessions/2026/01/01/rollout-2026-01-01T00-00-00-aaaaaaaa-0000.jsonl", ".codex/sessions/2026/01/02/rollout-2026-01-02T00-00-00-bbbbbbbb-0000.jsonl", "sessions/2026/01/01/rollout-2026-01-01T00-00-00-aaaaaaaa-0000.jsonl"},
		{"pi", "p1", ".pi/agent/sessions/--home-a-src-x_old--/2026-01-01_p1.jsonl", ".pi/agent/sessions/--home-a-src-x_old--/2026-01-02_p2.jsonl", "sessions/--Users-b-src-x.new--/2026-01-01_p1.jsonl"},
	}
	for _, c := range cases {
		t.Run(c.program, func(t *testing.T) {
			home := t.TempDir()
			unaccounted(t, home)
			for _, f := range []string{c.named, c.newest} {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(home, f)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, f), []byte(`{"cwd":"`+from+`"}`+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(filepath.Join(home, c.newest), time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			agent, _ := agentNamed(builtin, c.program)
			packed, err := side{}.bytesOf(carrierOf(agent, ""), from, to, "pack", c.id)
			if err != nil || len(packed) == 0 {
				t.Fatalf("pack: %v, %d bytes", err, len(packed))
			}
			files := untar(t, packed)
			if body, ok := files[c.there]; !ok || !strings.Contains(body, to) || len(files) != 1 {
				t.Errorf("packed %v, wanted only %s with the folder rewritten", files, c.there)
			}
		})
	}
}

// A window on an account carries its conversation into the account of the
// same name on the other side, and starts there on it; with no account of
// that name there, into the default one, which it says.
func TestCarryLandsInTheAccountOfTheSameName(t *testing.T) {
	home := t.TempDir()
	unaccounted(t, home)
	from, to := "/home/a/src/x.old", "/Users/b/src/x.new"
	accounts := filepath.Join(home, ".config", "mia", "accounts", "claude")
	said := filepath.Join(accounts, "work", "projects", "-home-a-src-x-old", "1.jsonl")
	if err := os.MkdirAll(filepath.Dir(said), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(said, []byte(`{"cwd":"`+from+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var told strings.Builder
	windows := []Window{{Name: "claude", Pane: Pane{Command: "claude", Account: "work"}}}
	if carried := (*App)(nil).carry(side{}, side{}, from, to, windows, &told); len(carried) != 1 {
		t.Fatalf("carried %v", carried)
	}
	if _, err := os.Stat(filepath.Join(accounts, "work", "projects", "-Users-b-src-x-new", "1.jsonl")); err != nil {
		t.Errorf("not in the account of the same name: %v", err)
	}
	if want := "CLAUDE_CONFIG_DIR='" + filepath.Join(accounts, "work") + "' claude --continue"; windows[0].Command != want {
		t.Errorf("restarts as %q, wanted %q", windows[0].Command, want)
	}
	if told.Len() > 0 {
		t.Errorf("said %q of an account that is there", told.String())
	}

	// on a side with no such account: the default one, said so
	if err := os.Rename(filepath.Join(accounts, "work"), filepath.Join(accounts, "away")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(accounts, "away", "projects", "-home-a-src-x-old"), 0o700); err != nil {
		t.Fatal(err)
	}
	windows = []Window{{Name: "claude", Pane: Pane{Command: "claude", Account: "work"}}}
	_ = (*App)(nil).carry(side{}, side{}, from, to, windows, &told)
	if windows[0].Command != "claude" || !strings.Contains(told.String(), "no claude account work here") {
		t.Errorf("restarts as %q, said %q", windows[0].Command, told.String())
	}
}
