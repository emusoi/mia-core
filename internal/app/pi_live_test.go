package app_test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emusoi/mia-core/internal/app"
	"github.com/emusoi/mia-core/internal/gittest"
)

// piAsItIs plays pi to mia's extension: it hands it a session's events as
// pi does, writes the session file as pi does, after every handler of a
// message_end, and stops after each step for the test to look.
const piAsItIs = `
import { appendFileSync, writeFileSync } from "node:fs";
import { createInterface } from "node:readline";
import { pathToFileURL } from "node:url";

const [extension, file, cwd] = process.argv.slice(2);
if (!process.features?.typescript) process.exit(3);
const { default: load } = await import(pathToFileURL(extension).href);
const handlers = {};
load({ on: (name, handler) => (handlers[name] ??= []).push(handler) });

const header = { type: "session", version: 3, id: "p9", timestamp: "2026-11-22T10:00:00.000Z", cwd };
const entries = [{ type: "model_change", id: "e0", parentId: null, timestamp: "2026-11-22T10:00:00.000Z" }];
const ctx = { cwd, hasUI: false, sessionManager: { getSessionFile: () => file, getHeader: () => header, getEntries: () => [...entries] } };
const fire = (name, event) => (handlers[name] ?? []).forEach((handler) => handler({ type: name, ...event }, ctx));
let flushed = false;
const end = (message) => {
	fire("message_end", { message });
	const entry = { type: "message", id: "e" + entries.length, parentId: entries.at(-1).id, timestamp: new Date().toISOString(), message };
	entries.push(entry);
	if (flushed) appendFileSync(file, JSON.stringify(entry) + "\n");
	else if (message.role === "assistant") {
		writeFileSync(file, [header, ...entries].map((e) => JSON.stringify(e)).join("\n") + "\n");
		flushed = true;
	}
};
const stream = (content) => fire("message_update", { message: { role: "assistant", content, timestamp: Date.now() } });
const lines = createInterface({ input: process.stdin })[Symbol.asyncIterator]();
const step = async (name) => {
	await new Promise((resolve) => setTimeout(resolve, 400));
	console.log(name);
	await lines.next();
};

const asked = { role: "user", content: [{ type: "text", text: "fix the cart" }], timestamp: Date.now() };
fire("message_start", { message: asked });
end(asked);
await step("asked");

fire("message_start", { message: { role: "assistant", content: [], timestamp: Date.now() } });
stream([{ type: "thinking", thinking: "where is", thinkingSignature: "c2ln" }]);
await step("thinking");

stream([{ type: "thinking", thinking: "where is it" }, { type: "text", text: "Looking at it." }, { type: "toolCall", id: "t1", name: "bash", arguments: {}, partialJson: "" }]);
await step("calling");

stream([{ type: "thinking", thinking: "where is it" }, { type: "text", text: "Looking at it." }, { type: "toolCall", id: "t1", name: "bash", arguments: { command: "go test ./cart" }, partialJson: "{\"command\":\"go test ./cart\"}" }]);
await step("called");

end({ role: "assistant", content: [{ type: "thinking", thinking: "where is it", thinkingSignature: "c2ln" }, { type: "text", text: "Looking at it." }, { type: "toolCall", id: "t1", name: "bash", arguments: { command: "go test ./cart" } }], stopReason: "toolUse", timestamp: Date.now() });
await step("written");

const result = { role: "toolResult", toolCallId: "t1", toolName: "bash", content: [{ type: "text", text: "ok" }], timestamp: Date.now() };
fire("message_start", { message: result });
end(result);
fire("message_start", { message: { role: "assistant", content: [], timestamp: Date.now() } });
stream([{ type: "text", text: "It is fix" }]);
await step("saying");

stream([{ type: "text", text: "It is fixed." }]);
await step("said");

end({ role: "assistant", content: [{ type: "text", text: "It is fixed." }], stopReason: "stop", timestamp: Date.now() });
fire("session_shutdown", { reason: "quit" });
await step("done");
process.exit(0);
`

// pi writes a message to its session only once it is done, and nothing of
// a new session until its first reply is: mia's extension writes what pi
// has not yet, and mia reads it after what pi has written. A new session
// is listed from the first thing asked in it, what pi thinks, says and
// calls is heard as it writes it, each under the number it keeps once pi
// has written it, a live file written for the session as it was before is
// not read, and once pi has written all of it there is nothing of mia's
// beside pi's own file.
func TestPiIsHeardAsItWritesThroughMiasExtension(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node to run pi's extension with")
	}
	gittest.Isolate(t)
	home, _ := ownFolders(t)
	repo := gittest.New(t)
	a, err := app.Open(repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(home, ".pi", "agent", "auth.json"), "a sign-in")
	done, err := a.SignalAccount("pi", "", false, false)
	if err != nil || len(done) != 1 {
		t.Fatalf("signals: %+v %v", done, err)
	}
	shipped, err := os.ReadFile(done[0].File)
	if err != nil {
		t.Fatal(err)
	}
	extension := filepath.Join(t.TempDir(), "mia-signal.mts") // a module node strips the types of
	driver := filepath.Join(filepath.Dir(extension), "pi.mjs")
	if os.WriteFile(extension, shipped, 0o644) != nil || os.WriteFile(driver, []byte(piAsItIs), 0o644) != nil {
		t.Fatal("could not write pi")
	}
	sessions := filepath.Join(home, ".pi", "agent", "sessions", "-"+strings.ReplaceAll(repo.Root, "/", "-")+"--")
	if err := os.MkdirAll(sessions, 0o755); err != nil { // pi makes it as it starts
		t.Fatal(err)
	}
	file := filepath.Join(sessions, "2026-11-22T10-00-00-000Z_p9.jsonl")

	pi := exec.Command(node, driver, extension, file, repo.Root)
	pi.Env = []string{"PATH=" + filepath.Dir(node), "HOME=" + home} // no mia to call
	pi.Stderr = os.Stderr
	in, _ := pi.StdinPipe()
	out, _ := pi.StdoutPipe()
	if err := pi.Start(); err != nil {
		t.Fatal(err)
	}
	defer pi.Process.Kill()
	steps := bufio.NewScanner(out)
	step := func(want string) {
		t.Helper()
		if !steps.Scan() {
			if pi.Wait(); pi.ProcessState.ExitCode() == 3 {
				t.Skip("this node runs no TypeScript")
			}
			t.Fatalf("pi stopped before %s", want)
		}
		if steps.Text() != want {
			t.Fatalf("pi is at %q, not %q", steps.Text(), want)
		}
	}
	next := func() { _, _ = io.WriteString(in, "\n") }
	turns := func() string {
		t.Helper()
		got, err := a.ConversationTurns("p9", 0)
		if err != nil {
			t.Fatal(err)
		}
		var all []string
		for _, turn := range got {
			all = append(all, fmt.Sprintf("%d %s %s", turn.N, turn.Kind, turn.Text))
		}
		return strings.Join(all, " | ")
	}

	// a view follows it from the first thing asked
	var mu sync.Mutex
	var heard []app.Turn
	stop := make(chan struct{})
	defer close(stop)
	followed := func() string {
		mu.Lock()
		defer mu.Unlock()
		last := map[int]string{}
		top := 0
		for _, turn := range heard {
			last[turn.N] = turn.Kind + " " + turn.Text
			top = max(top, turn.N)
		}
		var all []string
		for n := 1; n <= top; n++ {
			all = append(all, fmt.Sprintf("%d %s", n, last[n]))
		}
		return strings.Join(all, " | ")
	}
	hears := func(want string) {
		t.Helper()
		for deadline := time.Now().Add(time.Second); followed() != want; {
			if time.Now().After(deadline) {
				t.Fatalf("the view heard %q within a second, not %q", followed(), want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	both := func(want string) {
		t.Helper()
		if got := turns(); got != want {
			t.Errorf("turns %q, want %q", got, want)
		}
		hears(want)
	}

	step("asked")
	if _, err := os.Stat(file); err == nil {
		t.Fatal("pi wrote its session before its first reply: this plays it wrong")
	}
	heads, _ := a.Conversations(app.Wanted{Agent: "pi"})
	if len(heads) != 1 || heads[0].ID != "p9" || heads[0].Asked != "fix the cart" || heads[0].Cwd != repo.Root || heads[0].Turns != 1 {
		t.Fatalf("a session pi has not yet written is listed from what was asked: %+v", heads)
	}
	go func() {
		_ = a.FollowConversation("p9", "", 0, stop, func(turns []app.Turn) error {
			mu.Lock()
			heard = append(heard, turns...)
			mu.Unlock()
			return nil
		})
	}()
	both("1 asked fix the cart")
	next()

	step("thinking")
	both("1 asked fix the cart | 2 thought where is")
	next()

	step("calling")
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Using bash")
	next()

	step("called")
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Ran go test ./cart")
	next()

	step("written")
	if _, err := os.Stat(file + ".live"); err == nil {
		t.Error("the live file is there after pi wrote all of it")
	}
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Ran go test ./cart")
	next()

	step("saying")
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Ran go test ./cart | 5 said It is fix")
	// pi writes something else first: what is live was of the session before it
	put(t, file, `{"type":"label","id":"l1","parentId":"e3","timestamp":"2026-11-22T10:00:09.000Z","targetId":"e1","label":"cart"}`)
	if got := turns(); strings.Contains(got, "5 said") {
		t.Errorf("a live file of the session as it was is read: %q", got)
	}
	next()

	step("said")
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Ran go test ./cart | 5 said It is fixed.")
	next()

	step("done")
	both("1 asked fix the cart | 2 thought where is it | 3 said Looking at it. | 4 tool Ran go test ./cart | 5 said It is fixed.")
	next()
	if err := pi.Wait(); err != nil {
		t.Fatalf("pi: %v", err)
	}
	if entries, _ := os.ReadDir(sessions); len(entries) != 1 || entries[0].Name() != filepath.Base(file) {
		t.Errorf("beside pi's session there is more than it: %v", entries)
	}
	mu.Lock()
	defer mu.Unlock()
	using := -1
	for i, turn := range heard {
		if turn.N > 5 {
			t.Errorf("a turn heard past the last: %+v", turn)
		}
		if turn.N == 4 && turn.Text == "Using bash" && using < 0 {
			using = i
		}
		if turn.N == 4 && turn.Text == "Ran go test ./cart" && using < 0 {
			t.Error("the tool was heard as run before it was heard begun")
		}
	}
}
