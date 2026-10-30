package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSidesAndCornersAreAskedCachedAndRanked(t *testing.T) {
	h, dir := host(t)
	counter := filepath.Join(t.TempDir(), "calls")
	write(t, dir, "agent", `case "$1" in
manifest) echo '{"protocol":1,"sides":[{"id":"answered","side":"top","label":"answered","rank":20,"every":"1h"},{"id":"waiting","side":"top","label":"waiting","rank":10,"every":"1h"}],"corners":[{"id":"agents","at":"bl","every":"1h"}]}' ;;
side) echo "$2 $MIA_REPO" >> `+counter+`; echo '{"count":"2","lines":[[{"text":"●","width":3,"style":"yellow"},{"text":"sinza"}],[],{"cells":[{"text":"LB-9"}],"open":{"popup":["wazo","issues","show","LB-9"],"title":"issues / LB-9"}}]}' ;;
corner) echo '{"segments":[{"text":"⠋ 2","style":"blue"},{"text":"  agents","style":"dim"}]}' ;;
esac
`)
	write(t, dir, "issue", `case "$1" in
manifest) echo '{"protocol":1,"sides":[{"id":"issues","side":"left","label":"issues","rank":10}]}' ;;
side) echo boom >&2; exit 1 ;;
esac
`)
	h.Configured = []string{"agent", "issue"}
	on, _ := h.Enabled()
	if len(on) != 2 {
		t.Fatalf("enabled %+v", on)
	}
	c := Context{Repo: "/r", MiaDir: t.TempDir()}
	contextOf := func(Plugin) Context { return c }

	tabs, corners := AllSides(on, contextOf, nil)
	AllSides(on, contextOf, nil)
	calls, _ := os.ReadFile(counter)
	if string(calls) != "answered /r\nwaiting /r\n" && string(calls) != "waiting /r\nanswered /r\n" {
		t.Errorf("each side asked once with its id and the repo, then cached: %q", calls)
	}
	var order []string
	for _, tab := range tabs {
		order = append(order, tab.Where+":"+tab.ID)
	}
	if strings.Join(order, " ") != "left:issues top:waiting top:answered" {
		t.Errorf("tabs grouped by side, lowest rank first: %v", order)
	}
	waiting := tabs[1]
	if waiting.Count != "2" || len(waiting.Lines) != 3 || waiting.Lines[0].Cells[0].Width != 3 || waiting.Lines[0].Cells[0].Style != "yellow" || len(waiting.Lines[1].Cells) != 0 || waiting.Lines[0].Open != nil {
		t.Errorf("a side's answer: %+v", waiting)
	}
	if open := waiting.Lines[2]; open.Cells[0].Text != "LB-9" || open.Open == nil || open.Open.Title != "issues / LB-9" || len(open.Open.Popup) != 4 {
		t.Errorf("a line may be {cells, open}: %+v", open)
	}
	if !strings.Contains(tabs[0].Problem, "boom") || tabs[0].Lines != nil {
		t.Errorf("a failing side says why in one line: %+v", tabs[0])
	}
	if len(corners) != 1 || corners[0].At != "bl" || len(corners[0].Segments) != 2 || corners[0].Segments[0].Style != "blue" {
		t.Errorf("corner: %+v", corners)
	}
}

func TestASideOrCornerInNoPlaceIsAManifestProblem(t *testing.T) {
	h, dir := host(t)
	write(t, dir, "middle", `echo '{"protocol":1,"sides":[{"id":"x","side":"middle","label":"x"}]}'`)
	write(t, dir, "tl", `echo '{"protocol":1,"corners":[{"id":"x","at":"tl"}]}'`)
	write(t, dir, "often", `echo '{"protocol":1,"sides":[{"id":"x","side":"top","label":"x","every":"often"}]}'`)
	write(t, dir, "fine", `echo '{"protocol":1,"sides":[{"id":"x","side":"top","label":"x"}],"corners":[{"id":"y","at":"br"}]}'`)
	h.Configured = []string{"middle", "tl", "often", "fine"}
	all, _ := h.List()
	for _, p := range all {
		if (p.Problem == "") != (p.Name == "fine") {
			t.Errorf("%s: %q", p.Name, p.Problem)
		}
	}
}

func TestAnItemOpensExactlyOneWay(t *testing.T) {
	for _, bad := range []string{`{"cells":[],"open":{}}`, `{"cells":[],"open":{"verb":"issue","lands":"ilala"}}`} {
		var it Item
		if err := json.Unmarshal([]byte(bad), &it); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}

func TestAFailureIsKeptForEverySoTheDashDoesNotWaitOnIt(t *testing.T) {
	h, dir := host(t)
	counter := filepath.Join(t.TempDir(), "calls")
	write(t, dir, "gh", `case "$1" in
manifest) echo '{"protocol":1,"rows":{"every":"1h"},"sides":[{"id":"prs","side":"left","label":"prs","every":"1h"}]}' ;;
*) echo "$1" >> `+counter+`; echo slow >&2; exit 1 ;;
esac
`)
	h.Configured = []string{"gh"}
	on, _ := h.Enabled()
	c := Context{MiaDir: t.TempDir()}
	contextOf := func(Plugin) Context { return c }

	var tabs []SideTab
	var rows Rows
	for range 2 {
		tabs, _ = AllSides(on, contextOf, nil)
		rows = on[0].Rows(c, nil)
	}
	calls, _ := os.ReadFile(counter)
	if string(calls) != "side\nrows\n" {
		t.Errorf("a failing side and rows are each asked once per every: %q", calls)
	}
	if !strings.Contains(tabs[0].Problem, "slow") || !strings.Contains(rows.Problem, "slow") {
		t.Errorf("the kept failure still says why: side %q rows %q", tabs[0].Problem, rows.Problem)
	}
}

func TestSidesGetTheInputOnStdin(t *testing.T) {
	h, dir := host(t)
	got := filepath.Join(t.TempDir(), "stdin")
	write(t, dir, "issue", `case "$1" in
manifest) echo '{"protocol":1,"sides":[{"id":"issues","side":"left","label":"issues"}]}' ;;
side) cat > `+got+`; echo '{"count":"","lines":[]}' ;;
esac
`)
	h.Configured = []string{"issue"}
	on, _ := h.Enabled()
	AllSides(on, func(Plugin) Context { return Context{Repo: "/r", MiaDir: t.TempDir()} }, map[string]any{"worktrees": []string{"sinza"}})
	if data, _ := os.ReadFile(got); string(data) != `{"worktrees":["sinza"]}` {
		t.Fatalf("side read %q", data)
	}
}

func TestATimedOutPluginTakesItsChildrenWithIt(t *testing.T) {
	dir := t.TempDir()
	late := filepath.Join(dir, "late")
	path := write(t, dir, "slow", `(sleep 1; touch `+late+`) & wait`)
	if _, err := call(path, os.Environ(), []string{"side"}, nil, 200*time.Millisecond); err == nil || !strings.Contains(err.Error(), "took longer") {
		t.Fatalf("err %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(late); err == nil {
		t.Fatal("the plugin's child outlived the timeout")
	}
}

func TestAViewsNewKindsOfLineParseAndFlatten(t *testing.T) {
	var answer SideAnswer
	raw := `{"count":"3","head":"3 to do","note":"in WL","lines":[
		{"label":"to do","cells":[{"text":"3"}]},
		{"chart":[{"label":"mon","value":1},{"label":"today","value":4,"on":true}]},
		{"cells":[{"text":"WL-55","width":7},{"text":"A title","grow":true},{"meter":{"value":1,"max":2},"width":4}],"open":{"lands":"lemara"}}]}`
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Head != "3 to do" || answer.Lines[0].Label != "to do" || len(answer.Lines[1].Chart) != 2 || !answer.Lines[2].Cells[1].Grow {
		t.Fatalf("parsed %+v", answer)
	}
	flat := func(it Item) string {
		var s string
		for _, c := range it.Flat() {
			s += c.Text
		}
		return s
	}
	if got := flat(answer.Lines[0]); got != "to do  3" {
		t.Errorf("a label leads its count: %q", got)
	}
	if got := flat(answer.Lines[1]); got != "▃█" {
		t.Errorf("a chart is a row of blocks for the room: %q", got)
	}
	if got := flat(answer.Lines[2]); got != "WL-55A title━━╌╌" {
		t.Errorf("a meter is a bar of its width: %q", got)
	}
}
