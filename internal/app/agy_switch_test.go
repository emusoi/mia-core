package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// An agy conversation goes on with another agent from a handoff, and
// another's with agy; with agy itself it is taken up again by its id. Back
// to agy from a conversation that went on from one of agy's, agy takes its
// own up again, and is handed only what was said since.
func TestAgyGoesOnWithAnotherAgentAndTakesItsOwnUpAgainComingBack(t *testing.T) {
	gittest.Isolate(t)
	repo := gittest.New(t)
	claudeDir := agentHomes(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	agyTalked(t, record.Path, repo.Root, "go on")
	talked(t, claudeDir, record.Path)
	// its models, from an agy of the test's own: never the machine's
	bin := t.TempDir()
	put(t, filepath.Join(bin, "agy"), "#!/bin/sh", `[ "$1" = models ] && printf 'gemini-4-flash\tGemini 4 Flash\n'`)
	if err := os.Chmod(filepath.Join(bin, "agy"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if command := a.ResumeCommand("agy", agyNow); command != "agy --conversation "+agyNow {
		t.Errorf("taken up again by its id: %s", command)
	}
	if _, err := a.ContinueConversation(app.Continue{ID: agyNow, With: "agy", DryRun: true}); err == nil || !strings.Contains(err.Error(), "(agy --conversation "+agyNow+")") {
		t.Errorf("with agy itself it is taken up again, not handed over: %v", err)
	}
	from, err := a.ContinueConversation(app.Continue{ID: agyNow, With: "claude", DryRun: true})
	if err != nil || from.From != (app.ContinuedFrom{Agent: "agy", ID: agyNow, Worktree: record.Name}) || from.Turns != 13 || from.Command != "claude" ||
		from.Handoff != filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "agy-"+agyNow+".md") {
		t.Errorf("agy's goes on with claude: %+v %v", from, err)
	}
	if _, err := a.ContinueConversation(app.Continue{ID: "c1", With: "agy", Model: "gemini-9", DryRun: true}); err == nil || !strings.Contains(err.Error(), "agy has no model gemini-9") {
		t.Errorf("a model agy does not list: %v", err)
	}
	to, err := a.ContinueConversation(app.Continue{ID: "c1", With: "agy", Model: "gemini-4-flash", DryRun: true})
	if err != nil || to.From.Agent != "claude" || to.Command != "agy '--model' 'gemini-4-flash'" || to.Resumes != "" {
		t.Errorf("claude's goes on with agy, a conversation of its own: %+v %v", to, err)
	}

	// claude's went on from agy's
	put(t, filepath.Join(repo.Root, ".git", "mia", "handoffs", record.Name, "agy-"+agyNow+".1.json"),
		`{"from":{"agent":"agy","id":"`+agyNow+`","worktree":`+quoted(record.Name)+`},"to":{"agent":"claude","window":{"index":2,"name":"claude"},"id":"c1"},"handoff":"/h/agy.md","at":"2026-11-02T09:59:00Z"}`)
	back, err := a.ContinueConversation(app.Continue{ID: "c1", With: "agy", Effort: "high", DryRun: true})
	if err != nil || back.Resumes != agyNow || back.To.ID != agyNow || back.Command != "agy '--effort' 'high' --conversation "+agyNow || back.Turns != to.Turns {
		t.Fatalf("back to agy, its own taken up again: %+v %v", back, err)
	}
	if heads, _ := a.Conversations(app.Wanted{Agent: "claude"}); len(heads) != 1 || heads[0].Continues == nil || *heads[0].Continues != (app.Talk{Agent: "agy", ID: agyNow}) {
		t.Errorf("claude's goes on from agy's: %+v", heads)
	}
}

// agy is taken up again in a window of its conversation's worktree, and
// switched to another model by being started again there on it.
func TestAgyIsResumedInAWindowAndSwitchedByStartingItAgain(t *testing.T) {
	// its models from $TMUX_TMPDIR/models; anything else is it started: it says how, and waits
	lab(t, map[string]string{"agy": `[ "$1" = models ] && echo "Fetching available models..." && exec cat "$TMUX_TMPDIR/models"
echo "agy $*"
cat`,
		// a login shell puts the machine's own programs before the lab's,
		// a real agy among them: this one keeps the lab's first
		"sh": `[ "$1" = -lc ] && shift && exec /bin/sh -c "$@"
exec /bin/sh "$@"`})
	bin := filepath.Join(os.Getenv("TMUX_TMPDIR"), "bin")
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("no tmux")
	}
	if err := os.Symlink(tmux, filepath.Join(bin, "tmux")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	t.Setenv("SHELL", filepath.Join(bin, "sh"))
	if out, err := exec.Command(os.Getenv("SHELL"), "-lc", "command -v agy").Output(); err != nil || strings.TrimSpace(string(out)) != filepath.Join(bin, "agy") {
		t.Fatalf("a window's shell finds agy at %q, not the lab's: no window is opened", out)
	}
	put(t, filepath.Join(os.Getenv("TMUX_TMPDIR"), "models"), "gemini-4-neon-high\tGemini 4 Neon (High)", "gemini-4-flash\tGemini 4 Flash")
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.New("feature")
	if err != nil {
		t.Fatal(err)
	}
	agyTalked(t, record.Path, repo.Root, "go on")

	done, err := a.ResumeConversation(agyNow, "", "", "", "", "")
	if err != nil || done.Command != "agy --conversation "+agyNow || done.Worktree != record.Name || done.Account != "" {
		t.Fatalf("resumed = %+v, %v", done, err)
	}
	screenOf(t, record.Name, done.Window.Name, "agy --conversation "+agyNow)
	running := func() {
		t.Helper()
		heads, _ := a.Conversations(app.Wanted{Agent: "agy", Worktree: record.Name})
		if len(heads) != 1 || heads[0].ID != agyNow || !heads[0].Running || heads[0].Window == nil || heads[0].Window.Index != done.Window.Index {
			t.Fatalf("the window has it open, behind its shell: %+v", heads)
		}
	}
	running()
	if runs := a.WindowRuns(record, done.Window.Name); runs.Agent != "agy" || !runs.Reads || runs.Live || !runs.Again {
		t.Errorf("the window runs %+v", runs)
	}
	if again, err := a.ResumeConversation(agyNow, "", "", "", "", ""); err != nil || !again.Already || again.Window != done.Window {
		t.Errorf("one a window has open is left as it is: %+v %v", again, err)
	}

	// at work until the last step agy wrote is a reply that calls no tool
	transcript := filepath.Join(agyLogs(agyNow), "transcript_full.jsonl")
	for _, step := range []struct {
		line string
		work bool
	}{
		{"", false}, // its last, "Done."
		{agyStep(13, "USER_EXPLICIT", "USER_INPUT", "2026-11-02T10:01:00Z", `,"content":`+agyAsked("and the map?")), true},
		{agyStep(14, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:01:01Z", `,"content":"Looking.","tool_calls":[{"name":"run_command","args":{"CommandLine":"go test ./..."}}]`), true},
		{agyStep(15, "MODEL", "GENERIC", "2026-11-02T10:01:02Z", `,"content":"ok"`), true},
		{agyStep(16, "SYSTEM", "SYSTEM_MESSAGE", "2026-11-02T10:01:02Z", `,"content":"a task finished"`), true},
		{agyStep(17, "MODEL", "PLANNER_RESPONSE", "2026-11-02T10:01:03Z", `,"content":"The map too."`), false},
	} {
		if step.line != "" {
			put(t, transcript, step.line)
		}
		if got := a.AtWork(record, done.Window.Index); got != step.work {
			t.Errorf("after %q at work = %v", step.line, got)
		}
	}
	// a step being written is one not done
	f, err := os.OpenFile(transcript, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"step_index":18,"source":"USER_EXPLICIT","type":"USER_IN`)
	f.Close()
	if err != nil || !a.AtWork(record, done.Window.Index) {
		t.Errorf("with a step half written, at rest: %v", err)
	}

	now, err := a.SwitchWindow(record, done.Window.Name, "gemini-4-flash", "high")
	if err != nil || now != (app.Chosen{Model: "gemini-4-flash", Effort: "high"}) {
		t.Fatalf("switched: %+v, %v", now, err)
	}
	screenOf(t, record.Name, done.Window.Name, "agy --model gemini-4-flash --effort high --conversation "+agyNow)
	running()
	if heads, _ := a.Conversations(app.Wanted{Agent: "agy", Worktree: record.Name}); heads[0].Model != "gemini-4-flash" || heads[0].Effort != "high" {
		t.Errorf("as switched: %+v", heads[0])
	}
}
