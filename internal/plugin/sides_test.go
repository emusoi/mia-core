package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	tabs, corners := AllSides(on, contextOf)
	AllSides(on, contextOf)
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
