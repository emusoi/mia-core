package app

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/run"
)

// agy is the Antigravity CLI. It keeps everything in one folder under the
// home directory, whatever account it is signed in to.
const agyFolder = ".gemini/antigravity-cli"

// agyThere reports whether agy can be asked: it is on this machine and
// has kept something.
func agyThere() bool {
	if _, err := exec.LookPath("agy"); err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(home, filepath.FromSlash(agyFolder)))
	return err == nil && info.IsDir()
}

// agyModels is agy's models: those its account may use, which only it
// knows, so its own program is asked — agy models, a model a line — and
// what it said is kept, as opencode's is. It is asked only where it has
// kept something.
type agyModels struct {
	// cache is the folder what it said is kept in; nothing is kept with none.
	cache string
}

func (m agyModels) models() []plugin.Model {
	return keptModels(m.cache, "agy", func() ([]plugin.Model, bool) {
		if !agyThere() {
			return nil, false
		}
		out, _ := run.Tool{Binary: "agy", Within: AgentTimeout}.Capture("models") // one that fails has none
		return agyListed(out), true
	})
}

// agy keeps each conversation in a folder of its own, brain/<id>, and
// writes it there as it goes, a step a line, in
// .system_generated/logs/transcript_full.jsonl; one of an agy before
// that file has only transcript.jsonl, which cuts long fields short and
// has each argument of a tool as JSON in a string. Neither says the
// folder it was made in: agyFolders does.
func agyFiles(home string, _ folders, _ bool) []string {
	files, _ := filepath.Glob(filepath.Join(home, "brain", "*", ".system_generated", "logs", "transcript_full.jsonl"))
	short, _ := filepath.Glob(filepath.Join(home, "brain", "*", ".system_generated", "logs", "transcript.jsonl"))
	for _, file := range short {
		if !slices.Contains(files, filepath.Join(filepath.Dir(file), "transcript_full.jsonl")) {
			files = append(files, file)
		}
	}
	return files
}

func agyFind(home string, _ folders, id string) []string {
	logs := filepath.Join(home, "brain", id, ".system_generated", "logs")
	for _, name := range []string{"transcript_full.jsonl", "transcript.jsonl"} {
		if _, err := os.Stat(filepath.Join(logs, name)); err == nil {
			return []string{filepath.Join(logs, name)}
		}
	}
	return nil
}

// agyID is the conversation's folder: brain/<id>/.system_generated/logs/<file>.
func agyID(path string) string { return filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path)))) }

// agyCwd is the folder a transcript's conversation was made in, as agy
// keeps it.
func agyCwd(path string) string {
	home := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(path)))))
	return agyFolders(home)[agyID(path)]
}

// agyFolders is the folder each conversation was made in, by its id, as
// agy keeps them: in history.jsonl, a line for each thing typed into it
// with the folder and, since it has, the conversation; in
// cache/last_conversations.json, the one last begun in each folder; and in
// cache/conversation_metadata.json, each conversation it listed when it
// last started, with its folders as file URIs, the first its own.
func agyFolders(home string) map[string]string {
	folders := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(home, "history.jsonl")); err == nil {
		for _, line := range bytes.Split(data, []byte("\n")) {
			var typed struct {
				Workspace      string `json:"workspace"`
				ConversationID string `json:"conversationId"`
			}
			if json.Unmarshal(line, &typed) == nil && typed.ConversationID != "" && typed.Workspace != "" {
				folders[typed.ConversationID] = typed.Workspace
			}
		}
	}
	var last map[string]string // a folder → the conversation last begun in it
	if data, err := os.ReadFile(filepath.Join(home, "cache", "last_conversations.json")); err == nil && json.Unmarshal(data, &last) == nil {
		for folder, id := range last {
			folders[id] = folder
		}
	}
	var listed struct {
		Conversations map[string]struct {
			Summary struct {
				WorkspaceURIs []string `json:"WorkspaceURIs"`
			} `json:"summary"`
		} `json:"conversations"`
	}
	if data, err := os.ReadFile(filepath.Join(home, "cache", "conversation_metadata.json")); err == nil && json.Unmarshal(data, &listed) == nil {
		for id, one := range listed.Conversations {
			if len(one.Summary.WorkspaceURIs) == 0 {
				continue
			}
			if made, err := url.Parse(one.Summary.WorkspaceURIs[0]); err == nil && made.Scheme == "file" && made.Path != "" {
				folders[id] = made.Path
			}
		}
	}
	return folders
}

// agyStep is one line of a transcript: what the person sent, the agent's
// reply with the tools it calls, what a tool gave back, or agy's own.
type agyStep struct {
	Source    string    `json:"source"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Content   string    `json:"content"`
	ToolCalls []agyCall `json:"tool_calls"`
}

// agyCall is a tool the agent called, with what it handed it.
type agyCall struct {
	Name string                     `json:"name"`
	Args map[string]json.RawMessage `json:"args"`
}

func agyLine(line []byte, about *about) []Turn {
	var s agyStep
	_ = json.Unmarshal(line, &s) // a field of another shape leaves the rest read
	if about.Started.IsZero() {
		about.Started = s.CreatedAt
	}
	switch {
	case s.Type == "USER_INPUT" && s.Source == "USER_EXPLICIT":
		if asked := agyRequest(s.Content); asked != "" {
			return []Turn{{Kind: turnAsked, At: s.CreatedAt, Text: asked}}
		}
	case s.Type == "PLANNER_RESPONSE":
		var turns []Turn
		if strings.TrimSpace(s.Content) != "" {
			turns = append(turns, Turn{Kind: turnSaid, At: s.CreatedAt, Text: s.Content})
		}
		for _, call := range s.ToolCalls {
			turns = append(turns, call.turn(s.CreatedAt))
		}
		return readsTogether(turns)
	case s.Source == "MODEL": // what a tool gave back
		return commitTurns(s.Content, s.CreatedAt)
	}
	return nil
}

// agyRequest is what the person sent, of a step of theirs: agy puts it
// between lines of its own, <USER_REQUEST> and </USER_REQUEST>, and what
// it adds — the time, a setting changed — after them. What was pasted is
// in it as it was pasted, with nothing around it.
func agyRequest(content string) string {
	request, _ := strings.CutPrefix(content, "<USER_REQUEST>\n")
	if end := strings.LastIndex(request, "\n</USER_REQUEST>"); end >= 0 {
		request = request[:end]
	}
	return strings.TrimSpace(request)
}

// agyArgs is what agy hands its tools, as far as a line about one needs.
type agyArgs struct {
	CommandLine, Cwd                                                             string
	AbsolutePath, Path, TargetFile                                               string
	Query, Pattern                                                               string
	CodeContent, Content, TargetContent, ReplacementContent, Target, Replacement string
	// ReplacementChunks is a list of edits, or that list as JSON in a string
	ReplacementChunks json.RawMessage
}

// turn says the call in a line, as toolTurn says the other agents', by
// the name mia says the same thing under; one mia does not know is named.
func (c agyCall) turn(at time.Time) Turn {
	held := map[string]json.RawMessage{}
	for key, value := range c.Args {
		held[key] = agyArg(value)
	}
	data, _ := json.Marshal(held)
	var a agyArgs
	_ = json.Unmarshal(data, &a)
	file := cmp.Or(a.AbsolutePath, a.Path, a.TargetFile)
	switch c.Name {
	case "run_command":
		turn := toolTurn("bash", toolInput{Command: a.CommandLine}, at)
		turn.cwd = a.Cwd
		return turn
	case "view_file", "read_file":
		return toolTurn("read", toolInput{Path: file}, at)
	case "write_to_file", "write_file":
		return toolTurn("write", toolInput{Path: file, Content: cmp.Or(a.CodeContent, a.Content)}, at)
	case "replace_file_content", "multi_replace_file_content", "edit_file":
		in := toolInput{Path: file, OldString: cmp.Or(a.TargetContent, a.Target), NewString: cmp.Or(a.ReplacementContent, a.Replacement)}
		var chunks []struct{ TargetContent, ReplacementContent string }
		if json.Unmarshal(a.ReplacementChunks, &chunks) != nil {
			var list string
			_ = json.Unmarshal(a.ReplacementChunks, &list)
			_ = json.Unmarshal([]byte(list), &chunks)
		}
		for _, chunk := range chunks {
			in.Edits = append(in.Edits, struct {
				OldString string `json:"old_string"`
				NewString string `json:"new_string"`
				OldText   string `json:"oldText"`
				NewText   string `json:"newText"`
			}{OldString: chunk.TargetContent, NewString: chunk.ReplacementContent})
		}
		return toolTurn("edit", in, at)
	case "grep_search":
		return toolTurn("grep", toolInput{Pattern: a.Query}, at)
	case "find_by_name":
		return toolTurn("find", toolInput{Pattern: a.Pattern}, at)
	}
	return toolTurn(c.Name, toolInput{}, at)
}

// agyArg is an argument of a tool as it was handed: one transcript.jsonl
// wrote as JSON in a string is read, when it is a quoted string, as the
// string it holds, as agy reads its own.
func agyArg(value json.RawMessage) json.RawMessage {
	var held string
	if json.Unmarshal(value, &held) != nil || !strings.HasPrefix(held, `"`) || !json.Valid([]byte(held)) {
		return value
	}
	return json.RawMessage(held)
}

// agyAtWork reports whether agy in a worktree's window is at work, as agy
// itself tells it of a conversation: until the last step it wrote of the
// one the window has open is a reply that calls no tool. A reply is
// written once the model has done writing it, so one being written is at
// work; one that waits on the person to allow a tool is too, and asks
// besides. With nothing written yet it is at rest.
func (a *App) agyAtWork(record model.Record, window int) bool {
	heads, _ := a.Conversations(Wanted{Worktree: record.Name, Agent: "agy"})
	for _, h := range heads {
		if !h.Running || h.Window == nil || h.Window.Index != window {
			continue
		}
		for _, home := range homesOf("agy") {
			for _, file := range agyFind(home.dir, folders{}, h.ID) {
				var last agyStep
				return json.Unmarshal(lastLine(file), &last) != nil || last.Type != "PLANNER_RESPONSE" || len(last.ToolCalls) > 0
			}
		}
	}
	return false
}

// lastLine is a file's last line, read from its end: a transcript runs to
// megabytes. One still being written is as much of it as there is.
func lastLine(file string) []byte {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	for size := int64(64 << 10); ; size *= 2 {
		from := max(0, info.Size()-size)
		tail := make([]byte, info.Size()-from)
		if _, err := f.ReadAt(tail, from); err != nil && err != io.EOF {
			return nil
		}
		tail = bytes.TrimRight(tail, "\n")
		if at := bytes.LastIndexByte(tail, '\n'); at >= 0 {
			return tail[at+1:]
		}
		if from == 0 {
			return tail
		}
	}
}

// agyListed is the models in what agy models prints: each line that is an
// id and its name, a tab between them, once. Anything else it prints —
// that it is fetching them, an error — is no model.
func agyListed(out string) []plugin.Model {
	models := []plugin.Model{}
	for _, line := range strings.Split(out, "\n") {
		id, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		id, name = strings.TrimSpace(id), strings.TrimSpace(name)
		if !ok || id == "" || name == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		if !slices.ContainsFunc(models, func(m plugin.Model) bool { return m.ID == id }) {
			models = append(models, plugin.Model{ID: id, Label: name})
		}
	}
	return models
}
