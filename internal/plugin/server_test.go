package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func served(t *testing.T) (Plugin, Context, string) {
	t.Helper()
	h, dir := host(t)
	starts := filepath.Join(t.TempDir(), "starts")
	write(t, dir, "live", `case "$1" in
manifest) echo '{"protocol":1,"serve":true,"rows":{}}' ;;
serve)
	echo started >> `+starts+`
	while IFS= read -r line; do
		id=$(printf %s "$line" | sed -n 's/^{"jsonrpc":"2.0","id":\([0-9]*\).*/\1/p')
		case "$line" in
		*'"method":"rows"'*) echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"rows\":{\"monduli\":{\"status\":\"served\"}}}}" ;;
		*'"method":"panel"'*) echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"result\":{\"title\":\"from serve\"}}" ;;
		*'"method":"event"'*) echo '{"jsonrpc":"2.0","method":"refresh"}' ;;
		*'"method":"fail"'*) echo "{\"jsonrpc\":\"2.0\",\"id\":$id,\"error\":{\"code\":1,\"message\":\"nope\"}}" ;;
		*'"method":"die"'*) exit 3 ;;
		esac
	done ;;
esac
`)
	h.Configured = []string{"live"}
	on, _ := h.Enabled()
	if len(on) != 1 || !on[0].Manifest.Serve {
		t.Fatalf("enabled %+v", on)
	}
	return on[0], Context{MiaDir: t.TempDir()}, starts
}

func TestServeAnswersRowsPanelsAndPushesRefresh(t *testing.T) {
	p, c, _ := served(t)
	refreshed := make(chan struct{}, 1)
	p.Server = p.Serve(c, func() { refreshed <- struct{}{} })
	defer p.Server.Stop()

	rows := p.Rows(c, map[string]any{})
	if rows.Problem != "" || rows.Rows["monduli"].Status != "served" {
		t.Fatalf("rows through serve: %+v", rows)
	}
	panel, err := p.Panel(c, "x", nil)
	if err != nil || !strings.Contains(string(panel), "from serve") {
		t.Fatalf("panel through serve: %s %v", panel, err)
	}
	if _, err := p.Server.Call("fail", nil, time.Second); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("an error answer: %v", err)
	}
	if err := p.Send(c, Event{Event: "env.up"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-refreshed:
	case <-time.After(3 * time.Second):
		t.Fatal("the refresh never came")
	}
}

func TestServeComesBackAfterItDies(t *testing.T) {
	was := FirstBackoff
	FirstBackoff = 50 * time.Millisecond
	t.Cleanup(func() { FirstBackoff = was })
	p, c, starts := served(t)
	p.Server = p.Serve(c, nil)
	defer p.Server.Stop()

	if _, err := p.Server.Call("rows", nil, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Server.Call("die", nil, 2*time.Second); err == nil {
		t.Fatal("a request the process died on should fail")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := p.Server.Call("rows", nil, 500*time.Millisecond); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("serve did not come back")
		}
	}
	data, _ := os.ReadFile(starts)
	if n := strings.Count(string(data), "started"); n != 2 {
		t.Errorf("started %d times, want 2", n)
	}
}

func TestStoppingServeEndsTheProcess(t *testing.T) {
	p, c, starts := served(t)
	server := p.Serve(c, nil)
	if _, err := server.Call("rows", nil, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { server.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop hung")
	}
	time.Sleep(200 * time.Millisecond)
	data, _ := os.ReadFile(starts)
	if strings.Count(string(data), "started") != 1 {
		t.Errorf("serve was started again after Stop: %q", data)
	}
}
