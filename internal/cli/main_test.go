package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
	"github.com/emusoi/mia-core/internal/runtime"
)

// The main agent and where every agent stands are a person's, not one
// repository's: an app asks from wherever its server was started.
func TestTheMainAgentAndStandingAnswerOutsideARepository(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())

	data, code := answered(t, "main")
	var state app.MainState
	if err := json.Unmarshal(data, &state); err != nil || code != exitOK || state.Running || state.Agent != "claude" || state.Session != "mia-main" || state.Home == "" {
		t.Fatalf("api main = %s, exit %d, %v", data, code, err)
	}
	if data, code = answered(t, "main", "turns", "--after", "3"); code != exitOK || string(data) != "[]\n" {
		t.Fatalf("no conversation is no turns: %q, exit %d", data, code)
	}
	if _, code = answered(t, "main", "turns", "--after", "soon"); code != exitUsage {
		t.Errorf("--after soon exited %d", code)
	}
	data, code = answered(t, "standing")
	var standing app.Standing
	if err := json.Unmarshal(data, &standing); err != nil || code != exitOK || standing.Waiting == nil || standing.Working == nil || standing.DoneUnread == nil || len(standing.Machines) != 1 {
		t.Fatalf("api standing = %s, exit %d, %v", data, code, err)
	}
	if _, code = answered(t, "standing", "--seen", "nowhere/kilwa"); code != exitFailed {
		t.Errorf("--seen of a repository there is not exited %d", code)
	}
	if caps := apiCapabilities(); !slices.Contains(caps.Queries, "main") || !slices.Contains(caps.Queries, "standing") || !slices.Contains(caps.Features, "main-agent") {
		t.Errorf("capabilities = %+v", caps)
	}

	// what is said to it is its own: not read for mia's flags, and refused only because nothing runs
	if code := Run([]string{"main", "say", "what does", "--json", "-h", "mean"}); code != exitFailed {
		t.Errorf("say to a main agent that is not running exited %d", code)
	}
	if code := Run([]string{"main", "say"}); code != exitUsage {
		t.Errorf("say with nothing to say exited %d", code)
	}
	if code := Run([]string{"main", "press", "1"}); code != exitFailed {
		t.Errorf("a key pressed in a main agent that is not running exited %d", code)
	}
	for _, none := range [][]string{{"main", "press"}, {"main", "press", "--"}} {
		if code := Run(none); code != exitUsage {
			t.Errorf("%v, no key to press, exited %d", none, code)
		}
	}
	if code := Run([]string{"main", "restart"}); code != exitUsage {
		t.Errorf("a verb main has not exited %d", code)
	}
	if code := Run([]string{"main"}); code != exitOK {
		t.Errorf("mia main exited %d", code)
	}
}

// What the main agent is told to run has to be what this build runs: its
// notes name a verb, a query or a window verb only when there is one.
func TestTheMainAgentsNotesNameOnlyVerbsThereAre(t *testing.T) {
	notes, err := os.ReadFile("../app/main.md")
	if err != nil {
		t.Fatal(err)
	}
	known := func(name string) (Verb, bool) {
		at := slices.IndexFunc(verbs, func(v Verb) bool { return v.Name == name })
		if at < 0 {
			return Verb{}, false
		}
		return verbs[at], true
	}
	window, _ := known("window")
	named := regexp.MustCompile("(?m)(?:^    |`)mia ([a-z-]+)(?: ([a-z-]+))?").FindAllStringSubmatch(string(notes), -1)
	if len(named) < 8 {
		t.Fatalf("the notes name %d verbs: %v", len(named), named)
	}
	for _, m := range named {
		verb, sub := m[1], m[2]
		if _, ok := known(verb); !ok {
			t.Errorf("the notes say `mia %s`, and there is no such verb", verb)
		}
		if verb == "api" && !slices.Contains(apiQueryList, sub) {
			t.Errorf("the notes say `mia api %s`, and there is no such query", sub)
		}
		if verb == "window" && !regexp.MustCompile(`[\[|]`+regexp.QuoteMeta(sub)+`[|\]]`).MatchString(window.Usage) {
			t.Errorf("the notes say `mia window %s`, and window has no such verb: %s", sub, window.Usage)
		}
	}
	if strings.Contains(string(notes), "skill") {
		t.Error("the notes send it to a skill, which may name verbs this build has not")
	}
}

// The main agent lives on one machine, and from another every verb about
// it is that machine's to do: asked over ssh, as standing asks a machine.
func TestTheMainAgentOnAnotherMachineIsAskedThere(t *testing.T) {
	gittest.Isolate(t)
	t.Chdir(t.TempDir())
	t.Setenv("MIA_MAIN_HERE", "")
	bin := t.TempDir()
	log := filepath.Join(bin, "asked")
	// fedora's mia answers; box is off
	ssh := `#!/bin/sh
for arg do command=$arg; done
printf '%s\n' "$command" >> ` + log + `
case " $* " in *" fedora.invalid "*) ;; *) echo "ssh: connect to host box.invalid port 22: Operation timed out" >&2; exit 255 ;; esac
case "$command" in
*"'api' 'main' 'turns'"*) echo '[{"n":3,"kind":"said","text":"goba is on it"}]' ;;
*"'api' 'main'"*|*"'main' 'status' '--json'"*) echo '{"running":true,"agent":"claude","machine":"local","home":"/home/user/.config/mia/main","session":"mia-main","window":{"index":1,"name":"claude"},"conversation":"c1","working":true}' ;;
*"'main' 'status'"*) echo "claude is the main agent, working, in window claude of mia-main since Nov 2 10:00" ;;
*"'main' 'say'"*|*"'main' 'press'"*) ;;
*) echo "mia: the main agent is not running — \` + "`mia main start\\`" + `" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	machines := runtime.Store{Path: filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "runtimes.json")}
	for _, name := range []string{"box", "fedora"} {
		if err := machines.Add(runtime.Machine{Name: name, SSH: name + ".invalid"}); err != nil {
			t.Fatal(err)
		}
	}
	lives := func(on string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "mia", "config.toml"), []byte("[main]\nmachine = \""+on+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	asked := func() string {
		data, _ := os.ReadFile(log)
		return string(data)
	}

	times := func() int { return strings.Count(asked(), "MIA_MAIN_HERE=1") }

	lives("fedora")
	for _, ask := range [][]string{{"api", "main"}, {"main", "status", "--json"}} {
		data, code := printed(t, ask...)
		var state app.MainState
		if err := json.Unmarshal(data, &state); err != nil || code != exitOK || !state.Running || state.Machine != "fedora" || state.Home != "/home/user/.config/mia/main" || state.Conversation != "c1" {
			t.Fatalf("%v = %s, exit %d, %v", ask, data, code, err)
		}
	}
	data, code := answered(t, "main", "turns", "--after", "2")
	var turns []app.Turn
	if err := json.Unmarshal(data, &turns); err != nil || code != exitOK || len(turns) != 1 || turns[0].N != 3 || turns[0].Text != "goba is on it" {
		t.Fatalf("its turns = %s, exit %d, %v", data, code, err)
	}
	if data, code := printed(t, "main"); code != exitOK || !strings.HasPrefix(string(data), "claude is the main agent, working") {
		t.Errorf("mia main = %q, exit %d", data, code)
	}
	if code := Run([]string{"main", "say", "start goba", "--json", "and it's -h"}); code != exitOK {
		t.Errorf("said to it there exited %d", code)
	}
	if code := Run([]string{"main", "press", "2"}); code != exitOK {
		t.Errorf("a key pressed in it there exited %d", code)
	}
	// each asked of its own mia, told to answer for its own machine; what is said goes as it was typed
	for _, want := range []string{"MIA_MAIN_HERE=1 $HOME/.local/bin/mia", "'api' 'main' 'turns' '--after' '2'", `'main' 'say' '--' 'start goba --json and it'\''s -h'`, "'main' 'press' '--' '2'"} {
		if !strings.Contains(asked(), want) {
			t.Errorf("fedora was not asked %s:\n%s", want, asked())
		}
	}
	// what its mia refuses is said as fedora's, and is no word on whether it answers
	if code := Run([]string{"main", "interrupt"}); code != exitFailed {
		t.Errorf("interrupting one that does not run there exited %d", code)
	}
	a, err := app.AtMain()
	if err != nil {
		t.Fatal(err)
	}
	fedora, elsewhere, err := a.MainElsewhere()
	if err != nil || !elsewhere || fedora.Name != "fedora" {
		t.Fatalf("where it lives = %+v, %v, %v", fedora, elsewhere, err)
	}
	if _, err := a.OnMainMachine(fedora, false, "main", "interrupt"); err == nil || err.Error() != "on fedora: the main agent is not running — `mia main start`" {
		t.Errorf("refused there: %v", err)
	}
	if code := Run([]string{"main", "restart"}); code != exitUsage || strings.Contains(asked(), "restart") {
		t.Errorf("a verb main has not exited %d, and fedora was asked it: %v", code, strings.Contains(asked(), "restart"))
	}

	// a machine that does not answer is one sentence that names it
	lives("box")
	if _, code := answered(t, "main"); code != exitFailed {
		t.Errorf("api main of a machine that is off exited %d", code)
	}
	if a, err = app.AtMain(); err != nil {
		t.Fatal(err)
	}
	box, _, _ := a.MainElsewhere()
	_, err = a.OnMainMachine(box, true, "api", "main")
	if err == nil || err.Error() != "box, the machine the main agent lives on, does not answer: ssh: connect to host box.invalid port 22: Operation timed out" {
		t.Errorf("off: %v", err)
	}
	// an app that polls does not wait out ssh each time; a verb a person types asks again
	before := times()
	if _, code := answered(t, "main", "turns"); code != exitFailed || times() != before {
		t.Errorf("asked again at once: exit %d, ssh ran %d more times", code, times()-before)
	}
	if code := Run([]string{"main", "start"}); code != exitFailed || times() != before+1 {
		t.Errorf("mia main start exited %d, ssh ran %d more times", code, times()-before)
	}

	// a machine there is not, and the machine itself
	lives("nowhere")
	if _, code := answered(t, "main"); code != exitFailed {
		t.Errorf("a machine mia does not know exited %d", code)
	}
	before = times()
	for _, here := range []func(){func() { lives("local") }, func() { lives("box"); t.Setenv("MIA_MAIN_HERE", "1") }} {
		here()
		data, code := answered(t, "main")
		var state app.MainState
		if err := json.Unmarshal(data, &state); err != nil || code != exitOK || state.Running || state.Machine != "local" || times() != before {
			t.Errorf("on the machine itself = %s, exit %d, %v", data, code, err)
		}
	}
}
