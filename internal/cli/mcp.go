package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/emusoi/mia-core/internal/app"
)

// mia mcp: an MCP server on stdio, for an agent to look through the conversations had with
// agents before — its own and every other's, in every project — and read one. It reads and
// answers; it changes nothing. An agent is told of it by the agent's own means, which mia does
// not touch: `claude mcp add mia -- mia mcp`, and the like.

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var mcpTools = []mcpTool{
	{"recall", "Search the past conversations the person had with coding agents (claude, codex, pi, opencode, agy), in every project on this machine, for some words. Use it before starting work that may have been tried before, or when the person refers to something done earlier. Each result is a conversation: its project, agent, date, the line that matched, and the root, id and turn to read it with.",
		map[string]any{"type": "object", "required": []string{"text"}, "properties": map[string]any{
			"text":  map[string]any{"type": "string", "description": "The words to look for, as they would have been said: a phrase, an error message, a name. Not a question."},
			"limit": map[string]any{"type": "integer", "description": "How many conversations at most (default 10)."}}}},
	{"conversation", "Read what was asked and said in one past conversation that recall found: the person's words and the agent's, without tool output. Give the root and id from recall, and its turn to read around the place that matched; with no turn, the end of it.",
		map[string]any{"type": "object", "required": []string{"root", "id"}, "properties": map[string]any{
			"root": map[string]any{"type": "string", "description": "The project's folder, as recall gave it."},
			"id":   map[string]any{"type": "string", "description": "The conversation's id, as recall gave it."},
			"turn": map[string]any{"type": "integer", "description": "The turn to read around, as recall gave it."}}}},
}

// mcpCall does a tool and gives what it says.
func mcpCall(name string, args map[string]any) (string, error) {
	text, _ := args["text"].(string)
	number := func(key string) int {
		n, _ := args[key].(float64)
		return int(n)
	}
	switch name {
	case "recall":
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("say the words to look for, in text")
		}
		limit := number("limit")
		if limit <= 0 {
			limit = 10
		}
		found, err := recalled("", text, limit)
		if err != nil {
			return "", err
		}
		if len(found) == 0 {
			return "no conversation mentions it", nil
		}
		var b strings.Builder
		for _, r := range found {
			who := r.Agent
			if r.Yours {
				who = "the person"
			}
			fmt.Fprintf(&b, "%s  %s  with %s\n  %s said: %s\n  root %s  id %s  turn %d\n\n", r.At.Local().Format("2006-01-02 15:04"), r.Project, r.Agent, who, r.Line, r.Root, r.Session, r.N)
		}
		return b.String(), nil
	case "conversation":
		root, _ := args["root"].(string)
		id, _ := args["id"].(string)
		a, err := app.Open(root)
		if err != nil {
			return "", err
		}
		turns, err := a.ConversationTurns(id, 0)
		if err != nil {
			return "", err
		}
		// What was asked and said, forty turns of it: around the one named, or its end.
		at, said := -1, []app.Turn{}
		for _, t := range turns {
			if (t.Kind == "asked" || t.Kind == "said") && t.Text != "" {
				if at < 0 && number("turn") > 0 && t.N >= number("turn") {
					at = len(said)
				}
				said = append(said, t)
			}
		}
		from := max(len(said)-40, 0)
		if at >= 0 {
			from = max(min(at-15, len(said)-40), 0)
		}
		var b strings.Builder
		for _, t := range said[from:min(from+40, len(said))] {
			who, words := "agent", []rune(t.Text)
			if t.Kind == "asked" {
				who = "person"
			}
			if len(words) > 1500 {
				words = append(words[:1500], '…')
			}
			fmt.Fprintf(&b, "[%d] %s: %s\n\n", t.N, who, string(words))
		}
		if b.Len() == 0 {
			return "nothing is said in it yet", nil
		}
		return b.String(), nil
	}
	return "", fmt.Errorf("no tool called %s", name)
}

// serveMCP answers MCP's requests, a line each, until in ends.
func serveMCP(in io.Reader, out io.Writer) error {
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 1<<20), 16<<20)
	for lines.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ProtocolVersion string         `json:"protocolVersion"`
				Name            string         `json:"name"`
				Arguments       map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(lines.Bytes(), &req) != nil || req.ID == nil {
			continue // not a request: a notification, which is answered nothing
		}
		answer := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			speaks := req.Params.ProtocolVersion
			if speaks == "" {
				speaks = "2024-11-05"
			}
			answer["result"] = map[string]any{"protocolVersion": speaks, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "mia", "version": versionOf(version)}}
		case "ping":
			answer["result"] = map[string]any{}
		case "tools/list":
			answer["result"] = map[string]any{"tools": mcpTools}
		case "tools/call":
			text, err := mcpCall(req.Params.Name, req.Params.Arguments)
			if err != nil {
				text = err.Error()
			}
			answer["result"] = map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": err != nil}
		default:
			answer["error"] = map[string]any{"code": -32601, "message": "mia has no " + req.Method}
		}
		line, err := json.Marshal(answer)
		if err != nil {
			return err
		}
		if _, err := out.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return lines.Err()
}

// cmdMCP is mia mcp: the server, on this process's stdin and stdout.
func cmdMCP() int {
	if err := serveMCP(os.Stdin, os.Stdout); err != nil {
		return fail(err)
	}
	return exitOK
}
