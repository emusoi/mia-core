package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/gittest"
)

// A tool's turn says how it went once what it gave back is written, as
// each agent's transcript says it: given first with no status, then again
// under its n, ok or error, when its result's line comes; one whose
// result says nothing of it stays as it was. Read whole, or back from its
// end as a chat reads it, each says it in place.
func TestEachToolSaysHowItWentOnceItsResultIsWritten(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	cwd := strings.ReplaceAll(`"`+dir+`"`, `\`, `\\`)
	at := `"2026-11-02T10:00:00Z"`
	claude := func(kind, content, more string) string {
		return `{"parentUuid":"p","type":"` + kind + `","cwd":` + cwd + `,"timestamp":` + at + `,"message":{"content":` + content + `}` + more + `}`
	}
	codex := func(payload string) string {
		return `{"timestamp":` + at + `,"type":"response_item","payload":` + payload + `}`
	}
	pi := func(message string) string {
		return `{"type":"message","timestamp":` + at + `,"message":` + message + `}`
	}
	for _, agent := range []struct {
		name   string
		decode decoder
		lines  []string
		want   [][]string // what each line makes heard
		whole  string     // each turn read whole, with its status
	}{
		{"claude", claudeLine, []string{
			claude("user", `"fix the cart"`, ""),
			claude("assistant", `[{"type":"tool_use","id":"u1","name":"Bash","input":{"command":"go test ./cart"}}]`, ""),
			claude("assistant", `[{"type":"tool_use","id":"u2","name":"Read","input":{"file_path":"/w/gone.go"}}]`, ""),
			claude("user", `[{"tool_use_id":"u1","type":"tool_result","content":"ok cart","is_error":false}]`, `,"toolUseResult":{"stdout":"ok cart \"is_error\":true"}`),
			claude("user", `[{"tool_use_id":"u2","type":"tool_result","content":"File does not exist.","is_error":true}]`, `,"toolUseResult":"Error: File does not exist."`),
			// two read together, one failed: an error, whichever came first
			claude("assistant", `[{"type":"tool_use","id":"u3","name":"Read","input":{"file_path":"/w/a.go"}},{"type":"tool_use","id":"u4","name":"Read","input":{"file_path":"/w/b.go"}}]`, ""),
			claude("user", `[{"tool_use_id":"u3","type":"tool_result","content":"package a"}]`, `,"toolUseResult":{"file":"a.go"}`),
			claude("user", `[{"tool_use_id":"u4","type":"tool_result","content":"no","is_error":true}]`, `,"toolUseResult":{"file":"b.go"}`),
		}, [][]string{{"1 asked"}, {"2 tool"}, {"3 tool"}, {"2 tool ok"}, {"3 tool error"}, {"4 tool"}, nil, {"4 tool error"}},
			"1 asked|2 tool ok|3 tool error|4 tool error"},
		{"codex", codexLine, []string{
			`{"timestamp":` + at + `,"type":"session_meta","payload":{"cwd":` + cwd + `}}` + "\n" +
				codex(`{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the cart"}]}`),
			codex(`{"type":"function_call","name":"shell","call_id":"k1","arguments":"{\"command\":[\"bash\",\"-lc\",\"go test ./cart\"]}"}`),
			codex(`{"type":"function_call_output","call_id":"k1","output":"{\"output\":\"FAIL\\n\",\"metadata\":{\"exit_code\":1}}"}`),
			codex(`{"type":"function_call","name":"exec_command","call_id":"k2","arguments":"{\"cmd\":\"go vet ./...\"}"}`),
			codex(`{"type":"function_call_output","call_id":"k2","output":"Chunk ID: a1\nWall time: 0.2 seconds\nProcess exited with code 0\nOriginal token count: 1\nOutput:\nExit code: 3\n"}`),
			codex(`{"type":"custom_tool_call","name":"exec","call_id":"k3","input":"tools.exec_command({cmd:\"ls\"})"}`),
			codex(`{"type":"custom_tool_call_output","call_id":"k3","output":[{"type":"input_text","text":"Script failed\nWall time 0.0 seconds\nOutput:\n"}]}`),
			codex(`{"type":"function_call","name":"wait_agent","call_id":"k4","arguments":"{}"}`),
			codex(`{"type":"function_call_output","call_id":"k4","output":"{\"message\":\"Wait completed.\"}"}`),
		}, [][]string{{"1 asked"}, {"2 tool"}, {"2 tool error"}, {"3 tool"}, {"3 tool ok"}, {"4 tool"}, {"4 tool error"}, {"5 tool"}, nil},
			"1 asked|2 tool error|3 tool ok|4 tool error|5 tool"},
		{"pi", piLine, []string{
			`{"type":"session","cwd":` + cwd + `,"timestamp":` + at + `}` + "\n" + pi(`{"role":"user","content":"fix the cart"}`),
			pi(`{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"bash","arguments":{"command":"go test ./cart"}},{"type":"toolCall","id":"c2","name":"edit","arguments":{"path":"cart.go","edits":[{"oldText":"a","newText":"b"}]}}]}`),
			pi(`{"role":"toolResult","toolCallId":"c1","toolName":"bash","content":[{"type":"text","text":"ok"}],"isError":false}`),
			pi(`{"role":"toolResult","toolCallId":"c2","toolName":"edit","content":[{"type":"text","text":"no match"}],"isError":true}`),
		}, [][]string{{"1 asked"}, {"2 tool", "3 tool"}, {"2 tool ok"}, {"3 tool error"}},
			"1 asked|2 tool ok|3 tool error"},
		{"agy", agyLine, []string{
			`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":` + at + `,"content":"<USER_REQUEST>\nfix the cart\n</USER_REQUEST>"}`,
			`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","created_at":` + at + `,"tool_calls":[{"name":"run_command","args":{"CommandLine":"go test ./cart"}},{"name":"run_command","args":{"CommandLine":"go vet ./..."}},{"name":"view_file","args":{"AbsolutePath":"/w/a.go"}}]}`,
			`{"step_index":2,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":` + at + `,"content":"Created At: x\nCompleted At: y\nCommand exited with code 1.\nOutput:\nFAIL"}`,
			`{"step_index":3,"source":"MODEL","type":"RUN_COMMAND","status":"DONE","created_at":` + at + `,"content":"Created At: x\n\n\t\t\t\tThe command completed successfully.\n\t\t\t\tOutput:\n"}`,
			`{"step_index":4,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":` + at + `,"content":"Showing lines 1-3 of 3\nCommand exited with code 0"}`,
		}, [][]string{{"1 asked"}, {"2 tool", "3 tool", "4 tool"}, {"2 tool error"}, {"3 tool ok"}, nil},
			"1 asked|2 tool error|3 tool ok|4 tool"},
	} {
		t.Run(agent.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "c1.jsonl")
			if err := os.WriteFile(file, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			reader := transcripts{agent: agent.name, decode: agent.decode,
				find: func(string, folders, string) []string { return []string{file} },
				cwd:  func(string) string { return dir }}
			said := func(turn Turn) string {
				return strings.TrimSpace(fmt.Sprintf("%d %s %s", turn.N, turn.Kind, turn.Status))
			}
			next, _ := reader.follow(folders{Root: dir}, "c1", "", 0)
			if _, found, err := next(); !found || err != nil {
				t.Fatalf("found %v, %v", found, err)
			}
			for i, line := range agent.lines {
				f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				f.WriteString(line + "\n")
				f.Close()
				turns, _, err := next()
				var got []string
				for _, turn := range turns {
					got = append(got, said(turn))
				}
				if err != nil || !slices.Equal(got, agent.want[i]) {
					t.Errorf("line %d heard %v, want %v (%v)", i, got, agent.want[i], err)
				}
			}
			last, _ := reader.tail(folders{Root: dir}, "c1", "", func([]Turn) bool { return false })
			for how, read := range map[string]func() ([]Turn, bool, error){
				"read whole":     func() ([]Turn, bool, error) { return reader.turns(folders{Root: dir}, "c1", "", 0) },
				"read as a chat": last,
			} {
				turns, _, err := read()
				var got []string
				for _, turn := range turns {
					got = append(got, said(turn))
				}
				if strings.Join(got, "|") != agent.whole || err != nil {
					t.Errorf("%s: %s, want %s (%v)", how, strings.Join(got, "|"), agent.whole, err)
				}
			}
		})
	}
}

// opencode keeps how each tool went on the part that says it: ok once it
// completed, an error once it failed, and nothing while it runs.
func TestOpencodesToolsSayHowTheyWent(t *testing.T) {
	tool := func(name, state string) opencodePart {
		var part opencodePart
		part.Type, part.Name, part.State.Status = "tool", name, state
		part.State.Input = []byte(`{"command":"go test ./cart"}`)
		return part
	}
	reads := func(states ...string) []opencodePart {
		var parts []opencodePart
		for _, s := range states {
			parts = append(parts, tool("read", s))
		}
		return parts
	}
	for _, c := range []struct {
		parts []opencodePart
		want  string
	}{
		{[]opencodePart{tool("bash", "running")}, "1 tool"},
		{[]opencodePart{tool("bash", "completed")}, "1 tool ok"},
		{[]opencodePart{tool("bash", "error")}, "1 tool error"},
		{reads("completed", "completed"), "1 tool ok"},
		{reads("completed", "running"), "1 tool"},
		{reads("running", "error"), "1 tool error"},
	} {
		var got []string
		for _, turn := range numbered(opencodeTurns([]opencodeMessage{{Type: "assistant", Content: c.parts}})) {
			got = append(got, strings.TrimSpace(fmt.Sprintf("%d %s %s", turn.N, turn.Kind, turn.Status)))
		}
		if strings.Join(got, "|") != c.want {
			t.Errorf("%+v: %s, want %s", c.parts, strings.Join(got, "|"), c.want)
		}
	}
}
