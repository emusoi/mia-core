package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ── claude ──────────────────────────────────────────────────────────────
// ~/.claude/projects/<the folder's path, every other character a ->/<id>.jsonl

func claudeFiles(in folders, deep bool) []string {
	name := func(path string) string { return projectPrefix.ReplaceAllString(path, "-") }
	return jsonl(dirsOf(in, claudeProjects(), name, func(root string) string { return name(root) + "*" }), deep)
}

func claudeFind(in folders, id string) []string {
	return slices.DeleteFunc(claudeFiles(in, false), func(file string) bool { return claudeID(file) != id })
}

func claudeID(path string) string { return strings.TrimSuffix(filepath.Base(path), ".jsonl") }

var claudeOwn, claudeUser, claudeAssistant = []byte(`{"type":"`), []byte(`{"type":"user"`), []byte(`{"type":"assistant"`)

// A key as claude writes it: inside a string its quotes would be escaped.
var claudeResult, claudeAnswers = []byte(`"toolUseResult":`), []byte(`"answers":`)

// mayShowCommit reports whether text has what git commit prints in it, as
// far as a quick look can tell: seven or more hex digits after a space
// and before "] ".
func mayShowCommit(text []byte) bool {
	for {
		i := bytes.Index(text, []byte("] "))
		if i < 0 {
			return false
		}
		hex := 0
		for j := i - 1; j >= 0 && (('0' <= text[j] && text[j] <= '9') || ('a' <= text[j] && text[j] <= 'f')); j-- {
			hex++
		}
		if hex >= 7 && i-hex-1 >= 0 && text[i-hex-1] == ' ' {
			return true
		}
		text = text[i+2:]
	}
}

// asking is what claude's AskUserQuestion gave back.
type asking struct {
	Questions []struct {
		Question string `json:"question"`
	} `json:"questions"`
	Answers map[string]string `json:"answers"`
}

func claudeLine(line []byte, about *about) []Turn {
	// claude's own bookkeeping — a request, a title, a snapshot — starts with
	// its type and is most of a long transcript: not worth decoding
	if bytes.HasPrefix(line, claudeOwn) && !bytes.HasPrefix(line, claudeUser) && !bytes.HasPrefix(line, claudeAssistant) {
		return nil
	}
	// what a tool gave back is the rest of it, and says something only when
	// it is an answer or shows a commit made
	if bytes.Contains(line, claudeResult) && !bytes.Contains(line, claudeAnswers) && !mayShowCommit(line) {
		return nil
	}
	var e entry
	if json.Unmarshal(line, &e) != nil || e.IsSidechain || (e.Type != "user" && e.Type != "assistant") {
		return nil
	}
	if about.Cwd == "" {
		about.Cwd = e.Cwd
	}
	if about.Started.IsZero() {
		about.Started = e.Timestamp
	}
	var turns []Turn
	if e.Type == "user" {
		if said := e.said(); said != "" {
			return []Turn{{Kind: turnAsked, At: e.Timestamp, Text: said}}
		}
		var blocks []block
		_ = json.Unmarshal(e.Message.Content, &blocks)
		for _, b := range blocks {
			if b.Type != "tool_result" {
				continue
			}
			var asked asking
			if json.Unmarshal(e.ToolUseResult, &asked) == nil {
				for i, q := range asked.Questions {
					if answer, ok := asked.Answers[q.Question]; ok {
						turns = append(turns, Turn{Kind: turnAnswer, At: e.Timestamp, Text: answer, ref: fmt.Sprintf("%s#%d", b.ToolUseID, i)})
					}
				}
			}
		}
		return append(turns, commitTurns(e.toolOutput(), e.Timestamp)...)
	}
	var blocks []block
	_ = json.Unmarshal(e.Message.Content, &blocks)
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				turns = append(turns, Turn{Kind: turnSaid, At: e.Timestamp, Text: b.Text})
			}
		case "tool_use":
			var input toolInput
			_ = json.Unmarshal(b.Input, &input)
			if len(input.Questions) > 0 {
				for i, q := range input.Questions {
					turn := Turn{Kind: turnQuestion, At: e.Timestamp, Text: q.Question, ref: fmt.Sprintf("%s#%d", b.ID, i)}
					for _, o := range q.Options {
						turn.Options = append(turn.Options, o.Label)
					}
					turns = append(turns, turn)
				}
				continue
			}
			turn := toolTurn(b.Name, input, e.Timestamp)
			turn.cwd = e.Cwd
			turns = append(turns, turn)
		}
	}
	return readsTogether(turns)
}

// ── codex ───────────────────────────────────────────────────────────────
// ~/.codex/sessions/<y>/<m>/<d>/rollout-<time>-<id>.jsonl, every folder's
// together: the folder is in the session_meta line the file starts with.

func codexSessions() string {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".codex")
	}
	return filepath.Join(dir, "sessions")
}

func codexFiles(folders, bool) []string {
	files, _ := filepath.Glob(filepath.Join(codexSessions(), "*", "*", "*", "rollout-*.jsonl"))
	return files
}

func codexFind(_ folders, id string) []string {
	files, _ := filepath.Glob(filepath.Join(codexSessions(), "*", "*", "*", "rollout-*-"+id+".jsonl"))
	return files
}

// codexID is what follows rollout-<2026-01-02T03-04-05>- in the name.
func codexID(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if len(name) > 28 {
		return name[28:]
	}
	return name
}

type codexEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Payload   struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Cwd     string `json:"cwd"`
		Name    string `json:"name"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Arguments string          `json:"arguments"` // a function's, as JSON in a string
		Input     string          `json:"input"`     // a custom tool's, as it is
		Output    json.RawMessage `json:"output"`
	} `json:"payload"`
}

// codexCall is what codex hands its shell: an argv or one line.
type codexCall struct {
	Command json.RawMessage `json:"command"`
	Cmd     string          `json:"cmd"`
	Input   string          `json:"input"`
}

func codexLine(line []byte, about *about) []Turn {
	var e codexEntry
	_ = json.Unmarshal(line, &e) // a field of another shape leaves the rest read
	p := e.Payload
	if e.Type == "session_meta" {
		if about.Cwd == "" {
			about.Cwd, about.Started = p.Cwd, e.Timestamp
		}
		return nil
	}
	if e.Type != "response_item" {
		return nil // an event says again what an item already did
	}
	switch p.Type {
	case "message":
		var text strings.Builder
		for _, c := range p.Content {
			text.WriteString(c.Text)
		}
		said := strings.TrimSpace(text.String())
		switch {
		case said == "":
		case p.Role == "assistant":
			return []Turn{{Kind: turnSaid, At: e.Timestamp, Text: said}}
		case p.Role == "user" && !strings.HasPrefix(said, "<") && !strings.HasPrefix(said, "# AGENTS.md instructions"):
			return []Turn{{Kind: turnAsked, At: e.Timestamp, Text: said}}
		}
	case "function_call", "local_shell_call":
		var call codexCall
		_ = json.Unmarshal([]byte(p.Arguments), &call)
		command := call.Cmd
		var argv []string
		if json.Unmarshal(call.Command, &argv) == nil && len(argv) > 0 {
			command = strings.Join(argv, " ")
			if len(argv) == 3 && strings.HasPrefix(argv[1], "-") { // bash -lc '…'
				command = argv[2]
			}
			if argv[0] == "apply_patch" && len(argv) > 1 {
				return []Turn{patchTurn(argv[1], e.Timestamp)}
			}
		} else if command == "" {
			_ = json.Unmarshal(call.Command, &command)
		}
		if p.Name == "apply_patch" {
			return []Turn{patchTurn(call.Input, e.Timestamp)}
		}
		if command != "" {
			return []Turn{toolTurn("shell", toolInput{Command: command}, e.Timestamp)}
		}
		return []Turn{toolTurn(p.Name, toolInput{}, e.Timestamp)}
	case "custom_tool_call":
		if p.Name == "apply_patch" {
			return []Turn{patchTurn(p.Input, e.Timestamp)}
		}
		return []Turn{toolTurn(p.Name, toolInput{}, e.Timestamp)}
	case "function_call_output", "custom_tool_call_output":
		// a string, or JSON in a string with the output inside
		output := string(p.Output)
		var text string
		if json.Unmarshal(p.Output, &text) == nil {
			output = text
			var inner struct {
				Output string `json:"output"`
			}
			if json.Unmarshal([]byte(text), &inner) == nil && inner.Output != "" {
				output = inner.Output
			}
		}
		return commitTurns(output, e.Timestamp)
	}
	return nil
}

// patchTurn says a codex patch in a line: the file it changes, or how many.
func patchTurn(patch string, at time.Time) Turn {
	var files []string
	added, removed := 0, 0
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "*** "):
			for _, verb := range []string{"*** Update File: ", "*** Add File: ", "*** Delete File: "} {
				if file, ok := strings.CutPrefix(l, verb); ok {
					files = append(files, strings.TrimSpace(file))
				}
			}
		case strings.HasPrefix(l, "+"):
			added++
		case strings.HasPrefix(l, "-"):
			removed++
		}
	}
	turn := Turn{Kind: turnTool, At: at, Name: "apply_patch"}
	switch len(files) {
	case 0:
		turn.Text = "Edited files"
	case 1:
		turn.File = files[0]
		turn.Text = fmt.Sprintf("Edited %s +%d −%d", filepath.Base(files[0]), added, removed)
	default:
		turn.Text = fmt.Sprintf("Edited %d files +%d −%d", len(files), added, removed)
	}
	return turn
}

// ── pi ──────────────────────────────────────────────────────────────────
// ~/.pi/agent/sessions/-<the folder's path, / as ->--/<time>_<id>.jsonl

func piSessions() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "agent", "sessions")
}

func piFiles(in folders, _ bool) []string {
	name := func(path string) string { return "-" + strings.ReplaceAll(path, "/", "-") + "--" }
	return jsonl(dirsOf(in, piSessions(), name, func(root string) string { return "-" + strings.ReplaceAll(root, "/", "-") + "*--" }), false)
}

func piFind(in folders, id string) []string {
	return slices.DeleteFunc(piFiles(in, false), func(file string) bool { return piID(file) != id })
}

func piID(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if _, id, ok := strings.Cut(name, "_"); ok {
		return id
	}
	return name
}

type piEntry struct {
	Type      string    `json:"type"`
	Cwd       string    `json:"cwd"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type piBlock struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Name      string    `json:"name"`
	Arguments toolInput `json:"arguments"`
}

func piLine(line []byte, about *about) []Turn {
	var e piEntry
	_ = json.Unmarshal(line, &e) // a field of another shape leaves the rest read
	if e.Type == "session" {
		if about.Cwd == "" {
			about.Cwd, about.Started = e.Cwd, e.Timestamp
		}
		return nil
	}
	if e.Type != "message" {
		return nil
	}
	var blocks []piBlock
	var text string
	if json.Unmarshal(e.Message.Content, &text) == nil {
		blocks = []piBlock{{Type: "text", Text: text}}
	} else {
		_ = json.Unmarshal(e.Message.Content, &blocks)
	}
	var said strings.Builder
	var turns []Turn
	for _, b := range blocks {
		switch b.Type {
		case "text":
			said.WriteString(b.Text)
		case "toolCall":
			turns = append(turns, toolTurn(b.Name, b.Arguments, e.Timestamp))
		}
	}
	text = strings.TrimSpace(said.String())
	switch e.Message.Role {
	case "user":
		if text != "" {
			return []Turn{{Kind: turnAsked, At: e.Timestamp, Text: text}}
		}
	case "assistant":
		if text != "" {
			turns = append([]Turn{{Kind: turnSaid, At: e.Timestamp, Text: text}}, turns...)
		}
		return readsTogether(turns)
	case "toolResult":
		return commitTurns(text, e.Timestamp)
	}
	return nil
}

// ── what the three share ────────────────────────────────────────────────

// toolInput is what an agent hands a tool, as far as a line about it needs:
// claude's and pi's tools name these alike.
type toolInput struct {
	FilePath     string `json:"file_path"`
	Path         string `json:"path"`
	NotebookPath string `json:"notebook_path"`
	Command      string `json:"command"`
	Description  string `json:"description"`
	Pattern      string `json:"pattern"`
	Query        string `json:"query"`
	URL          string `json:"url"`
	Skill        string `json:"skill"`
	OldString    string `json:"old_string"`
	NewString    string `json:"new_string"`
	Content      string `json:"content"`
	Edits        []struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		OldText   string `json:"oldText"`
		NewText   string `json:"newText"`
	} `json:"edits"`
	Questions []struct {
		Question string `json:"question"`
		Options  []struct {
			Label string `json:"label"`
		} `json:"options"`
	} `json:"questions"`
}

// toolTurn says what a tool was used for in a line: "Edited view.lua +12 −3",
// "Ran go test ./...". One mia does not know is named.
func toolTurn(name string, in toolInput, at time.Time) Turn {
	turn := Turn{Kind: turnTool, At: at, Name: name}
	file := in.FilePath
	if file == "" {
		file = in.Path
	}
	if file == "" {
		file = in.NotebookPath
	}
	short := func(text string) string {
		line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
		if r := []rune(line); len(r) > 120 {
			line = string(r[:120]) + "…"
		}
		return line
	}
	switch strings.ToLower(name) {
	case "bash", "shell", "exec_command", "shell_command":
		turn.Text, turn.command = "Ran "+short(in.Command), in.Command
	case "edit", "multiedit", "notebookedit":
		added, removed := changed(in.OldString, in.NewString)
		for _, e := range in.Edits {
			a, r := changed(e.OldString+e.OldText, e.NewString+e.NewText)
			added, removed = added+a, removed+r
		}
		turn.File, turn.Text = file, fmt.Sprintf("Edited %s +%d −%d", filepath.Base(file), added, removed)
	case "write":
		turn.File, turn.Text = file, fmt.Sprintf("Wrote %s +%d", filepath.Base(file), lines(in.Content))
	case "read":
		turn.File, turn.Text = file, "Read "+filepath.Base(file)
	case "grep":
		turn.Text = "Searched for " + short(in.Pattern)
	case "glob", "find":
		turn.Text = "Looked for " + short(in.Pattern)
	case "agent", "task":
		turn.Text = "Asked an agent: " + short(in.Description)
	case "webfetch":
		turn.Text = "Fetched " + in.URL
	case "websearch":
		turn.Text = "Searched the web for " + short(in.Query)
	case "todowrite":
		turn.Text = "Updated its to-do list"
	case "skill":
		turn.Text = "Used the skill " + in.Skill
	default:
		turn.Text = "Used " + name
		if parts := strings.Split(name, "__"); len(parts) == 3 && parts[0] == "mcp" {
			turn.Text = "Used " + parts[2] + " of " + parts[1]
		}
	}
	return turn
}

func lines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
}

// changed is how many lines an edit adds and takes away, the lines it
// leaves alone at either end not counted.
func changed(before, after string) (added, removed int) {
	if before == "" && after == "" {
		return 0, 0
	}
	was, is := strings.Split(before, "\n"), strings.Split(after, "\n")
	if before == "" {
		was = nil
	}
	if after == "" {
		is = nil
	}
	for len(was) > 0 && len(is) > 0 && was[0] == is[0] {
		was, is = was[1:], is[1:]
	}
	for len(was) > 0 && len(is) > 0 && was[len(was)-1] == is[len(is)-1] {
		was, is = was[:len(was)-1], is[:len(is)-1]
	}
	return len(is), len(was)
}

// readsTogether makes one turn of files read one after another in the same
// message: "Read 3 files".
func readsTogether(turns []Turn) []Turn {
	var out []Turn
	run := 0
	for _, turn := range turns {
		if turn.Kind == turnTool && strings.EqualFold(turn.Name, "read") {
			if run++; run > 1 {
				last := &out[len(out)-1]
				last.File, last.Text = "", fmt.Sprintf("Read %d files", run)
				continue
			}
		} else {
			run = 0
		}
		out = append(out, turn)
	}
	return out
}

// commitTurns is a turn for each commit that what a tool gave back shows
// was made.
func commitTurns(output string, at time.Time) []Turn {
	var turns []Turn
	for _, c := range commitsMade(output) {
		turns = append(turns, Turn{Kind: turnCommit, At: at, Hash: c[0][:7], Subject: c[1]})
	}
	return turns
}
