package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// A window says which conversation only when it has one open and is the
// only window of its name: otherwise none is taken for it, and the ids of
// those open are said.
func TestAWindowNamesAConversationOnlyWhenItHasOneOpen(t *testing.T) {
	open := func(index int, name, agent, id string) Head {
		return Head{ID: id, Agent: agent, Worktree: "feature", Running: true, Window: &WindowRef{Index: index, Name: name}}
	}
	heads := []Head{open(1, "claude", "claude", "c1"), open(2, "claude", "claude", "c2"), open(3, "pi", "pi", "p1"),
		{ID: "old", Agent: "claude", Worktree: "feature"}, {ID: "o1", Agent: "codex", Worktree: "other", Running: true, Window: &WindowRef{Index: 4, Name: "codex"}}}
	for window, want := range map[string]string{
		"pi":     "p1",
		"2":      "c2", // a window's number is one window
		"claude": "2 windows of feature are claude, each with a conversation open — say which by its id: c1 (claude, window 1 claude), c2 (claude, window 2 claude), p1 (pi, window 3 pi)",
		"codex":  "window codex of feature has no conversation open — say which by its id: c1 (claude, window 1 claude)", // another worktree's
		"old":    "window old of feature has no conversation open",                                                       // an id is no window
		"":       "window  of feature has no conversation open",
	} {
		got, err := headIn(heads, "feature", window)
		if said := fmt.Sprint(got.ID, err); (err == nil && got.ID != want) || (err != nil && !strings.Contains(said, want)) {
			t.Errorf("headIn(%q) = %s, want %s", window, said, want)
		}
	}
	if _, err := headIn(heads[3:], "feature", "claude"); err == nil || !strings.Contains(err.Error(), "no window of feature has a conversation open") {
		t.Errorf("none open: %v", err)
	}
}

// A handoff over its budget keeps the first thing asked and the last turns
// in full, and says in one line how many of the middle are left out and
// the file that has them all; the whole has every turn.
func TestAHandoffOverItsBudgetLeavesOutItsMiddleAndSaysWhereItIs(t *testing.T) {
	at := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	head := Head{ID: "c1", Agent: "claude", Worktree: "feature", Cwd: "/w"}
	turns := []Turn{{N: 1, Kind: turnAsked, Text: "make the list faster"}}
	for n := 2; n <= 41; n += 2 {
		turns = append(turns, Turn{N: n, Kind: turnTool, Text: fmt.Sprintf("Ran step %d", n)},
			Turn{N: n + 1, Kind: turnSaid, Text: fmt.Sprintf("said %d %s", n+1, strings.Repeat("é", 5000))})
	}
	whole, left := handoff(head, turns, at, 0, "")
	if left != 0 || strings.Count(whole, "\n### ") != 21 || strings.Count(whole, "\n- Ran step") != 20 || strings.Contains(whole, "left out here") {
		t.Fatalf("the whole has every turn: %d left out", left)
	}
	text, left := handoff(head, turns, at, handoffBudget, "/git/mia/handoffs/feature/claude-c1-full.md")
	// 60,000 characters: the first asked and the last eleven said, 5,000 each, with the steps between them
	if size := len([]rune(text)); left != 18 || size > handoffBudget+1500 || size < handoffBudget-6000 {
		t.Fatalf("%d left out, of %d characters", left, size)
	}
	for _, has := range []string{
		"41 turns, in order, 18 of them left out here and all of them in `/git/mia/handoffs/feature/claude-c1-full.md`\n",
		"### 1 · user\n\nmake the list faster\n\n[… 18 turns, 2 to 19, are left out here to keep this short: every turn is in `/git/mia/handoffs/feature/claude-c1-full.md` — look there for what you need of them]\n\nclaude did:\n\n- Ran step 20\n\n### 21 · claude\n\nsaid 21 é",
		"\nclaude did:\n\n- Ran step 40\n\n### 41 · claude\n\nsaid 41 é",
	} {
		if !strings.Contains(text, has) {
			t.Errorf("the handoff has no %q", has)
		}
	}
	if strings.Contains(text, "said 19 ") || strings.Contains(text, "Ran step 18") {
		t.Error("the handoff has turns of its middle")
	}
	// one within its budget is all there, and one whose last turn alone is over it has that turn all the same
	if all, left := handoff(head, turns[:9], at, handoffBudget, "full"); left != 0 || strings.Contains(all, "left out here") || !strings.Contains(all, "said 9 ") {
		t.Errorf("within its budget: %d left out", left)
	}
	if last, left := handoff(head, turns, at, 100, "full"); left != 39 || !strings.Contains(last, "said 41 ") || !strings.Contains(last, "make the list faster") {
		t.Errorf("a budget smaller than a turn: %d left out", left)
	}
}

// The conversation a handoff began is told by its first message, which a
// list has cut at 200 characters: one cut inside the file's name is read
// whole, and one that does not reach it is not read at all.
func TestTheConversationAHandoffBeganIsToldByItsFirstMessage(t *testing.T) {
	at := time.Date(2026, 11, 2, 10, 30, 0, 0, time.UTC)
	file := "/src/" + strings.Repeat("deep/", 40) + ".git/mia/handoffs/feature/pi-p1.md" // longer than a list shows
	one := handedOn{From: ContinuedFrom{Agent: "pi", ID: "p1", Worktree: "feature"}, To: ContinuedTo{Agent: "claude"}, Handoff: file, At: at}
	conversation := func(id, agent, worktree string, started time.Time, asked string) Head {
		return Head{ID: id, Agent: agent, Worktree: worktree, Started: started, Asked: oneLine(asked)}
	}
	whole := map[string]string{"cut": "Read " + file + " and go on", "other": "Read " + strings.TrimSuffix(file, "pi-p1.md") + "claude-c9.md and go on"}
	heads := []Head{
		conversation("before", "claude", "feature", at.Add(-time.Minute), "Read "+file),
		conversation("codex", "codex", "feature", at, "Read "+file),
		conversation("elsewhere", "claude", "other", at, "Read "+file),
		conversation("unlike", "claude", "feature", at, strings.Repeat("something else altogether ", 20)),
		conversation("other", "claude", "feature", at.Add(time.Second), whole["other"]),
		conversation("later", "claude", "feature", at.Add(time.Hour), whole["cut"]),
		conversation("cut", "claude", "feature", at.Add(time.Minute), whole["cut"]),
	}
	var read []string
	first := func(h Head) string {
		read = append(read, h.ID)
		return whole[h.ID]
	}
	if got := begunBy(heads, one, first); got != "cut" {
		t.Errorf("begunBy = %q, want cut", got)
	}
	if strings.Contains(strings.Join(read, " "), "unlike") || strings.Contains(strings.Join(read, " "), "before") {
		t.Errorf("read whole: %v — only one cut inside the file's name is", read)
	}
	if got := begunBy(heads[:5], one, first); got != "" {
		t.Errorf("none names it, and begunBy = %q", got)
	}
	one.Handoff = "/src/app/.git/mia/handoffs/feature/pi-p1.md" // as short as most are: nothing is read
	read = nil
	heads = []Head{heads[3], conversation("short", "claude", "feature", at, "Read "+one.Handoff+" — a conversation")}
	if got := begunBy(heads, one, first); got != "short" || len(read) != 0 {
		t.Errorf("begunBy = %q, having read %v", got, read)
	}
	// Told nothing, and sent the conversation in front of the person's first words: told by how that begins.
	heads = []Head{heads[0], conversation("quiet", "claude", "feature", at, CarriedOpens("pi")+"\n\n# Handoff\n\n"+CarriedRule+"\n\nand the tests"),
		conversation("codex's", "claude", "feature", at, CarriedOpens("codex")+"\n\nx")}
	if got := begunBy(heads, one, first); got != "quiet" {
		t.Errorf("begunBy = %q, want quiet", got)
	}
	if got := OwnWords(CarriedOpens("pi") + "\n\nall of it\n\n" + CarriedRule + "\n\nand the tests"); got != "and the tests" {
		t.Errorf("OwnWords = %q", got)
	}
}
