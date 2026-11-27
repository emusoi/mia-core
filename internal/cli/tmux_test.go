package cli

import (
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/signal"
)

// A key that went keeps its place: every key after it sends the number it sent before, and
// the retired ones are bound to nothing and given no terminal key.
func TestRetiredKeysKeepEveryOtherKeysNumber(t *testing.T) {
	lines, err := terminalKeys("kitty")
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{`cmd+k send_text all \x1b[99;1~`, `cmd+o send_text all \x1b[99;2~`, `cmd+j send_text all \x1b[99;6~`,
		`cmd+[ send_text all \x1b[99;8~`, `cmd+] send_text all \x1b[99;9~`, `cmd+t send_text all \x1b[99;10~`, `cmd+1 send_text all \x1b[99;14~`, `cmd+9 send_text all \x1b[99;22~`} {
		if !strings.Contains(lines, "map "+kept+"\n") {
			t.Errorf("no %q in %s", kept, lines)
		}
	}
	for _, gone := range []string{"cmd+shift+j ", "cmd+n ", "cmd+. ", "[99;3~", "[99;4~", "[99;5~", "[99;7~"} {
		if strings.Contains(lines, gone) {
			t.Errorf("a retired key is sent: %q in %s", gone, lines)
		}
	}
	snippet := tmuxSnippet("mia")
	for _, gone := range []string{"User42", "User43", "User44", "User46", "bind Space", "bind n ", "bind m ", "bind e ", " radar", "tmux menu", "tmux act"} {
		if strings.Contains(snippet, gone) {
			t.Errorf("a retired key is bound: %q in %s", gone, snippet)
		}
	}
	for _, kept := range []string{"bind -n User40 ", "bind -n User45 ", "bind -n User47 ", "bind -n User61 ", "bind k ", "bind a ", "bind b ", "bind B "} {
		if !strings.Contains(snippet, kept) {
			t.Errorf("no %q in %s", kept, snippet)
		}
	}
}

func TestPrefixAGoesRoundWhatNeedsYouBestFirst(t *testing.T) {
	sig := func(rank int) []signal.Signal { return []signal.Signal{{Kind: "k", Rank: rank}} }
	local := []app.Listing{
		{Name: "busy", Signals: sig(60)},
		{Name: "done", Signals: sig(30)},
		{Name: "asks", Signals: sig(10)},
		{Name: "quiet"},
	}
	for here, want := range map[string]string{"quiet": "asks", "asks": "done", "done": "asks"} {
		if got := nextNeedingYou(local, here); got != want {
			t.Errorf("from %s: %q, want %q", here, got, want)
		}
	}
	if got := nextNeedingYou(local[:1], "busy"); got != "" {
		t.Errorf("a busy signal does not call for you: %q", got)
	}
}
