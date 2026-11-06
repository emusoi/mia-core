package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, Prefix+name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func host(t *testing.T) (Host, string) {
	dir := t.TempDir()
	return Host{Dirs: []string{dir}, StatePath: filepath.Join(t.TempDir(), "plugins.json")}, dir
}

const good = `case "$1" in
manifest) echo '{"protocol":1,"help":"says hi","verbs":[{"name":"hi"}]}' ;;
hi) shift; echo "hi $* from $MIA_PLUGIN in $MIA_WORKTREE with $MIA_PLUGIN_CONFIG"; echo data > "$MIA_PLUGIN_DATA/seen"; exit 7 ;;
esac
`

func TestAPluginRunsOnlyOnceEnabled(t *testing.T) {
	h, dir := host(t)
	write(t, dir, "hello", good)
	if err := os.WriteFile(filepath.Join(dir, Prefix+"notexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	all, err := h.List()
	if err != nil || len(all) != 1 || all[0].Enabled || len(all[0].Manifest.Verbs) != 0 {
		t.Fatalf("a found plugin is listed, off, and not run: %+v %v", all, err)
	}
	if err := h.Enable("hello"); err != nil {
		t.Fatal(err)
	}
	on, _ := h.Enabled()
	if len(on) != 1 || !on[0].Declares("hi") || on[0].Manifest.Help != "says hi" {
		t.Fatalf("enabled: %+v", on)
	}
	if err := h.Disable("hello"); err != nil {
		t.Fatal(err)
	}
	if on, _ := h.Enabled(); len(on) != 0 {
		t.Fatalf("still enabled: %+v", on)
	}
	if err := h.Enable("absent"); err == nil {
		t.Fatal("enabling a plugin nobody installed should say so")
	}
}

func TestABadPluginIsShownNeverFatal(t *testing.T) {
	h, dir := host(t)
	write(t, dir, "garbled", `echo not json`)
	write(t, dir, "future", `echo '{"protocol":2}'`)
	write(t, dir, "broken", `echo boom >&2; exit 1`)
	write(t, dir, "slow", `sleep 5`)
	h.Configured = []string{"garbled", "future", "broken", "slow", "gone"}
	was := ManifestTimeout
	ManifestTimeout = 300 * time.Millisecond
	t.Cleanup(func() { ManifestTimeout = was })

	all, err := h.List()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"garbled": "not JSON",
		"future":  "protocol 2",
		"broken":  "boom",
		"slow":    "longer than",
		"gone":    "no mia-gone is installed",
	}
	for _, p := range all {
		if !strings.Contains(p.Problem, want[p.Name]) || want[p.Name] == "" {
			t.Errorf("%s: problem %q, want it to mention %q", p.Name, p.Problem, want[p.Name])
		}
	}
	if on, _ := h.Enabled(); len(on) != 0 {
		t.Errorf("a plugin with a problem must not be offered: %+v", on)
	}
	if err := h.Disable("gone"); err == nil || !strings.Contains(err.Error(), "config") {
		t.Errorf("disabling a configured plugin should point at the config: %v", err)
	}
}

func TestTheFirstDirectoryWins(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	mine := write(t, first, "dup", good)
	write(t, second, "dup", good)
	if got := (Host{Dirs: []string{first, second}}).Discover()["dup"]; got != mine {
		t.Errorf("found %s, want %s", got, mine)
	}
}

func TestAVerbGetsItsArgumentsContextAndData(t *testing.T) {
	h, dir := host(t)
	write(t, dir, "hello", good)
	h.Configured = []string{"hello"}
	on, _ := h.Enabled()
	p, ok := Owner(on, "hi")
	if !ok {
		t.Fatal("hi has no owner")
	}

	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = f
	c := Context{MiaDir: t.TempDir(), Worktree: "monduli", Settings: map[string]any{"token": "x"}}
	code, err := p.Run(c, []string{"hi", "--json", "there"})
	os.Stdout = stdout
	f.Close()
	if err != nil || code != 7 {
		t.Fatalf("exit %d, %v", code, err)
	}
	got, _ := os.ReadFile(out)
	if want := `hi --json there from hello in monduli with {"token":"x"}`; strings.TrimSpace(string(got)) != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(p.DataDir(c), "seen")); err != nil {
		t.Errorf("the plugin could not keep data: %v", err)
	}
}

func TestAnEventReachesOnlyThePluginsThatWantIt(t *testing.T) {
	h, dir := host(t)
	got := filepath.Join(t.TempDir(), "got")
	write(t, dir, "listener", `case "$1" in
manifest) echo '{"protocol":1,"events":["worktree.created"]}' ;;
event) { echo "$2"; cat; } > `+got+` ;;
esac
`)
	write(t, dir, "everything", `echo '{"protocol":1,"events":["*"]}'`)
	write(t, dir, "deaf", `echo '{"protocol":1}'`)
	write(t, dir, "typo", `echo '{"protocol":1,"events":["worktree.made"]}'`)
	h.Configured = []string{"listener", "everything", "deaf", "typo"}
	on, _ := h.Enabled()
	wants := map[string]bool{}
	for _, p := range on {
		wants[p.Name] = p.Wants("worktree.created")
	}
	if !wants["listener"] || !wants["everything"] || wants["deaf"] || len(on) != 3 {
		t.Fatalf("wants %v (a typo'd event is a manifest problem)", wants)
	}

	var listener Plugin
	for _, p := range on {
		if p.Name == "listener" {
			listener = p
		}
	}
	c := Context{MiaDir: t.TempDir()}
	if err := listener.Send(c, Event{Protocol: 1, Event: "worktree.created", Worktree: "monduli"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, _ := os.ReadFile(got)
		if strings.HasPrefix(string(data), "worktree.created\n") && strings.Contains(string(data), `"worktree":"monduli"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the event did not arrive: %q", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(listener.DataDir(c), "events.log")); err != nil {
		t.Errorf("events log: %v", err)
	}
}

func TestAManifestIsAskedOnceWhileThePluginIsTheSame(t *testing.T) {
	h, dir := host(t)
	asked := filepath.Join(t.TempDir(), "asked")
	plugin := func(help string) string {
		return `case "$1" in
manifest) echo x >> ` + asked + `; echo '{"protocol":1,"help":"` + help + `"}' ;;
esac
`
	}
	write(t, dir, "counted", plugin("first"))
	h.Configured = []string{"counted"}
	for range 3 {
		if on, err := h.Enabled(); err != nil || len(on) != 1 || on[0].Manifest.Help != "first" {
			t.Fatalf("enabled %+v %v", on, err)
		}
	}
	if data, _ := os.ReadFile(asked); strings.Count(string(data), "x") != 1 {
		t.Errorf("asked %d times, want once", strings.Count(string(data), "x"))
	}
	write(t, dir, "counted", plugin("rebuilt, and longer"))
	if on, _ := h.Enabled(); len(on) != 1 || on[0].Manifest.Help != "rebuilt, and longer" {
		t.Errorf("a changed plugin is asked again: %+v", on)
	}
}
