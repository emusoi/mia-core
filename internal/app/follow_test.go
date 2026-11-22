package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/gittest"
)

// A conversation asked for whole each time gives what is new, and a turn
// that grew again under its n; nothing when nothing changed.
func TestPollingGivesWhatIsNewAndWhatGrew(t *testing.T) {
	asked := [][]Turn{
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}},
		{{N: 1, Kind: turnAsked, Text: "make it faster"}, {N: 2, Kind: turnSaid, Text: "I will look"}, {N: 3, Kind: turnTool, Text: "Read 3 files"}},
	}
	next := polling(func() ([]Turn, bool, error) {
		turns := asked[0]
		asked = asked[1:]
		return turns, true, nil
	})
	for _, want := range [][]int{{1, 2}, {2}, nil, {3}} {
		turns, found, err := next()
		var got []int
		for _, turn := range turns {
			got = append(got, turn.N)
		}
		if !found || err != nil || !slices.Equal(got, want) {
			t.Errorf("turns %v, found %v, %v; want %v", got, found, err, want)
		}
	}
}

// A reader gated on where its agent writes is asked first, then again
// only once the agent wrote there, or the backstop went by.
func TestWhenWrittenAsksOnlyOnceTheAgentWrote(t *testing.T) {
	db := filepath.Join(t.TempDir(), "opencode.db")
	asked := 0
	next := whenWritten(func() []string { return []string{db, db + "-wal"} }, time.Hour, func() ([]Turn, bool, error) {
		asked++
		return []Turn{{N: asked}}, true, nil
	})
	call := func(want int) {
		t.Helper()
		if _, found, err := next(); !found || err != nil || asked != want {
			t.Fatalf("asked %d times, found %v, %v; want %d", asked, found, err, want)
		}
	}
	call(1) // the first time, whatever is there
	call(1) // nothing written since
	if err := os.WriteFile(db+"-wal", []byte("a page"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(2) // the log written ahead of the database came
	call(2)
	if err := os.WriteFile(db+"-wal", []byte("a longer page"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(3) // and grew

	backstopped := 0
	again := whenWritten(func() []string { return []string{db} }, 0, func() ([]Turn, bool, error) {
		backstopped++
		return nil, false, nil
	})
	for range 3 {
		if _, found, _ := again(); found {
			t.Fatal("found is what it said when asked")
		}
	}
	if backstopped != 3 {
		t.Errorf("past its backstop it is asked whatever is written: %d", backstopped)
	}
}

// Each agent that writes a transcript is heard a line at a time as it
// writes one: a line not yet whole is left for when it is, and each turn
// comes once its line is there, under the number it keeps — what the
// agent thought, the tool it calls before it runs, and what it said.
func TestEachTranscriptIsHeardALineAtATimeAsItIsWritten(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	cwd := strings.ReplaceAll(`"`+dir+`"`, `\`, `\\`)
	at := `"2026-11-02T10:00:00Z"`
	for _, agent := range []struct {
		name   string
		decode decoder
		lines  []string
		want   [][]string // what each line makes heard
	}{
		{"claude", claudeLine, []string{
			`{"type":"user","cwd":` + cwd + `,"timestamp":` + at + `,"message":{"content":"fix the cart"}}`,
			`{"type":"assistant","cwd":` + cwd + `,"timestamp":` + at + `,"message":{"content":[{"type":"thinking","thinking":"where is it"}]}}`,
			`{"type":"assistant","cwd":` + cwd + `,"timestamp":` + at + `,"message":{"content":[{"type":"tool_use","id":"u1","name":"Bash","input":{"command":"go test ./cart"}}]}}`,
			`{"type":"assistant","cwd":` + cwd + `,"timestamp":` + at + `,"message":{"content":[{"type":"text","text":"It is fixed."}]}}`,
		}, [][]string{{"1 asked"}, {"2 thought"}, {"3 tool"}, {"4 said"}}},
		{"codex", codexLine, []string{
			`{"timestamp":` + at + `,"type":"session_meta","payload":{"cwd":` + cwd + `}}` + "\n" +
				`{"timestamp":` + at + `,"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix the cart"}]}}`,
			`{"timestamp":` + at + `,"type":"response_item","payload":{"type":"reasoning","summary":[{"text":"where is it"}]}}`,
			`{"timestamp":` + at + `,"type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"go test ./cart\"]}"}}`,
			`{"timestamp":` + at + `,"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"It is fixed."}]}}`,
		}, [][]string{{"1 asked"}, {"2 thought"}, {"3 tool"}, {"4 said"}}},
		{"pi", piLine, []string{
			`{"type":"session","cwd":` + cwd + `,"timestamp":` + at + `}` + "\n" +
				`{"type":"message","timestamp":` + at + `,"message":{"role":"user","content":"fix the cart"}}`,
			`{"type":"message","timestamp":` + at + `,"message":{"role":"assistant","content":[{"type":"thinking","thinking":"where is it"},{"type":"toolCall","name":"bash","arguments":{"command":"go test ./cart"}}]}}`,
			`{"type":"message","timestamp":` + at + `,"message":{"role":"toolResult","content":"ok"}}`,
			`{"type":"message","timestamp":` + at + `,"message":{"role":"assistant","content":[{"type":"text","text":"It is fixed."}]}}`,
		}, [][]string{{"1 asked"}, {"2 thought", "3 tool"}, nil, {"4 said"}}},
		{"agy", agyLine, []string{
			`{"source":"USER_EXPLICIT","type":"USER_INPUT","created_at":` + at + `,"content":"<USER_REQUEST>\nfix the cart\n</USER_REQUEST>"}`,
			`{"source":"MODEL","type":"PLANNER_RESPONSE","created_at":` + at + `,"thinking":"where is it","tool_calls":[{"name":"run_command","args":{"CommandLine":"go test ./cart"}}]}`,
			`{"source":"MODEL","type":"RUN_COMMAND","created_at":` + at + `,"content":"ok"}`,
			`{"source":"MODEL","type":"PLANNER_RESPONSE","created_at":` + at + `,"content":"It is fixed."}`,
		}, [][]string{{"1 asked"}, {"2 thought", "3 tool"}, nil, {"4 said"}}},
	} {
		t.Run(agent.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "c1.jsonl")
			if err := os.WriteFile(file, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			reader := transcripts{agent: agent.name, decode: agent.decode,
				find: func(string, folders, string) []string { return []string{file} },
				cwd:  func(string) string { return dir }}
			next, every := reader.follow(folders{Root: dir}, "c1", "", 0)
			if every > 200*time.Millisecond {
				t.Errorf("looked at every %s", every)
			}
			heard := func() []string {
				t.Helper()
				turns, found, err := next()
				if !found || err != nil {
					t.Fatalf("found %v, %v", found, err)
				}
				var got []string
				for _, turn := range turns {
					got = append(got, fmt.Sprintf("%d %s", turn.N, turn.Kind))
				}
				return got
			}
			heard()
			for i, line := range agent.lines {
				f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				half := len(line) - 5
				f.WriteString(line[:half])
				if got := heard(); got != nil {
					t.Errorf("line %d heard while half written: %v", i, got)
				}
				f.WriteString(line[half:] + "\n")
				f.Close()
				if got := heard(); !slices.Equal(got, agent.want[i]) {
					t.Errorf("line %d heard %v, want %v", i, got, agent.want[i])
				}
			}
		})
	}
}

// What pi has not yet written is read from mia's live file after what it
// has, a turn given again only when it is not as it was, each under the
// number it has once pi writes it; a live file written for the session as
// it was before is not read.
func TestWhatPiHasNotYetWrittenIsHeardAfterWhatItHas(t *testing.T) {
	gittest.Isolate(t)
	dir := t.TempDir()
	file := filepath.Join(t.TempDir(), "2026-11-22T10-00-00-000Z_p1.jsonl")
	cwd := strings.ReplaceAll(`"`+dir+`"`, `\`, `\\`)
	header := `{"type":"session","id":"p1","cwd":` + cwd + `,"timestamp":"2026-11-22T10:00:00Z"}`
	asked := `{"type":"message","timestamp":"2026-11-22T10:00:01Z","message":{"role":"user","content":"fix the cart"}}`
	reply := func(live bool, call string) string {
		return fmt.Sprintf(`{"type":"message","live":%v,"timestamp":"2026-11-22T10:00:02Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"where"},{"type":"text","text":"Looking."},{"type":"toolCall","name":"bash","arguments":%s}]}}`, live, call)
	}
	live := func(size int, lines ...string) {
		t.Helper()
		if err := os.WriteFile(file+piLive, []byte(fmt.Sprintf(`{"size":%d}`, size)+"\n"+strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reader := transcripts{agent: "pi", decode: piLine, live: piLive,
		find: func(string, folders, string) []string { return []string{file} }}
	next, _ := reader.follow(folders{Root: dir}, "p1", "", 0)
	heard := func(want ...string) {
		t.Helper()
		turns, found, err := next()
		if !found || err != nil {
			t.Fatalf("found %v, %v", found, err)
		}
		var got []string
		for _, turn := range turns {
			got = append(got, fmt.Sprintf("%d %s %s", turn.N, turn.Kind, turn.Text))
		}
		if !slices.Equal(got, want) {
			t.Errorf("heard %q, want %q", got, want)
		}
	}

	live(-1, header, asked, reply(true, `{}`))
	heard("1 asked fix the cart", "2 thought where", "3 said Looking.", "4 tool Using bash")
	live(-1, header, asked, reply(true, `{"command":"go te"}`))
	heard("4 tool Ran go te")

	written := strings.Join([]string{header, asked, reply(false, `{"command":"go test"}`)}, "\n") + "\n"
	if err := os.WriteFile(file, []byte(written), 0o644); err != nil {
		t.Fatal(err)
	}
	heard("1 asked fix the cart", "2 thought where", "3 said Looking.", "4 tool Ran go test")

	live(len(written)-1, `{"type":"message","live":true,"timestamp":"2026-11-22T10:00:03Z","message":{"role":"assistant","content":"stale"}}`)
	heard()
	live(len(written), `{"type":"message","live":true,"timestamp":"2026-11-22T10:00:03Z","message":{"role":"assistant","content":"Fixed."}}`)
	heard("5 said Fixed.")
}
