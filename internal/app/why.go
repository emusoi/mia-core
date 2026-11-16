package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emusoi/mia-core/internal/run"
)

// Why is a commit and the agent conversations that made or first saw it.
type Why struct {
	Commit, Subject string
	Conversations   []Conversation
}

// Conversation is one agent session that a commit appears in.
type Conversation struct {
	Session string    `json:"session"`
	Cwd     string    `json:"cwd"`
	File    string    `json:"file"`
	At      time.Time `json:"at"`
	Asked   string    `json:"asked"` // the last thing you said before it
	Made    bool      `json:"made"`  // git commit printed it there; otherwise first seen there
}

// Why finds the conversation behind a commit, or behind the commit that
// last touched file:line. It reads claude's own transcripts, which hold what
// `git commit` printed, so nothing has to be recorded when committing.
func (a *App) Why(target string) (Why, error) {
	commit, err := a.commitOf(target)
	if err != nil {
		return Why{}, err
	}
	out, _ := run.Tool{Binary: "git", Dir: a.Root}.Capture("log", "-1", "--format=%ct%x09%s", commit)
	stamp, subject, _ := strings.Cut(out, "\t")
	why := Why{Commit: commit, Subject: subject}
	seconds, _ := strconv.ParseInt(stamp, 10, 64)
	files := transcriptsMentioning(a.Root, commit[:7], time.Unix(seconds, 0))
	for _, file := range files {
		if c, ok := conversationIn(file, commit[:7]); ok {
			why.Conversations = append(why.Conversations, c)
		}
	}
	// the one that made it first, then the earliest that saw it
	sort.SliceStable(why.Conversations, func(i, j int) bool {
		ci, cj := why.Conversations[i], why.Conversations[j]
		if ci.Made != cj.Made {
			return ci.Made
		}
		return ci.At.Before(cj.At)
	})
	// the ones that made it; failing those, the first to see it — later
	// ones only read it in a log
	keep := 1
	for keep < len(why.Conversations) && why.Conversations[keep].Made {
		keep++
	}
	if len(why.Conversations) > keep {
		why.Conversations = why.Conversations[:keep]
	}
	return why, nil
}

// commitOf is the commit a target names: file:line through blame, or a rev.
func (a *App) commitOf(target string) (string, error) {
	git := run.Tool{Binary: "git", Dir: a.Root}
	if file, line, ok := strings.Cut(target, ":"); ok {
		if n, err := strconv.Atoi(line); err == nil {
			if !filepath.IsAbs(file) {
				if wd, err := os.Getwd(); err == nil {
					file = filepath.Join(wd, file)
				}
			}
			out, err := run.Tool{Binary: "git", Dir: filepath.Dir(file)}.Capture("blame", "--porcelain", "-L", fmt.Sprintf("%d,%d", n, n), "--", file)
			if err != nil {
				return "", fmt.Errorf("blame %s: %s", target, strings.TrimSpace(out))
			}
			sha, _, _ := strings.Cut(out, " ")
			if strings.Trim(sha, "0") == "" {
				return "", fmt.Errorf("%s is not committed yet", target)
			}
			return sha, nil
		}
	}
	out, err := git.Capture("rev-parse", "--verify", target+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s names no commit", target)
	}
	return strings.TrimSpace(out), nil
}

var projectPrefix = regexp.MustCompile(`[^a-zA-Z0-9]`)

// made is the line git commit prints when it makes one: the branch, the
// commit's short hash and its subject.
var made = regexp.MustCompile(`\[\S+ (?:\(root-commit\) )?([0-9a-f]{7,40})\] ([^\n]*)`)

// commitsMade is each commit that what a tool gave back shows was made:
// its hash as printed, and its subject.
func commitsMade(output string) [][2]string {
	var commits [][2]string
	for _, m := range made.FindAllStringSubmatch(output, -1) {
		commits = append(commits, [2]string{m[1], strings.TrimSpace(m[2])})
	}
	return commits
}

// transcriptsMentioning is claude's transcripts, for the repository and its
// worktrees, that hold the short hash; only those written to since the
// commit was made can.
func transcriptsMentioning(root, short string, since time.Time) []string {
	dirs := transcriptDirs(root)
	var recent []string
	for _, d := range dirs {
		_ = filepath.WalkDir(d, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".jsonl") {
				return nil
			}
			if info, err := entry.Info(); err == nil && !info.ModTime().Before(since) {
				recent = append(recent, path)
			}
			return nil
		})
	}
	return holding(recent, short, false)
}

type entry struct {
	Type        string    `json:"type"`
	IsMeta      bool      `json:"isMeta"`
	IsSidechain bool      `json:"isSidechain"`
	SessionID   string    `json:"sessionId"`
	Cwd         string    `json:"cwd"`
	Timestamp   time.Time `json:"timestamp"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Content   json.RawMessage `json:"content"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
}

// said is what you typed in a user entry; "" for tool results and mia's,
// claude's or a hook's own messages.
func (e entry) said() string {
	if e.Type != "user" || e.IsMeta {
		return ""
	}
	var text string
	if json.Unmarshal(e.Message.Content, &text) != nil {
		var blocks []block
		_ = json.Unmarshal(e.Message.Content, &blocks)
		for _, b := range blocks {
			if b.Type == "tool_result" {
				return ""
			}
			if b.Type == "text" {
				text += b.Text
			}
		}
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "<") || strings.HasPrefix(text, "This session is being continued") {
		return ""
	}
	return text
}

// toolOutput is what tools gave back in a user entry.
func (e entry) toolOutput() string {
	if e.Type != "user" {
		return ""
	}
	var blocks []block
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return ""
	}
	var out strings.Builder
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		var s string
		if json.Unmarshal(b.Content, &s) == nil {
			out.WriteString(s)
			continue
		}
		var parts []block
		_ = json.Unmarshal(b.Content, &parts)
		for _, p := range parts {
			out.WriteString(p.Text)
		}
	}
	return out.String()
}

// conversationIn is where a transcript first shows the commit in what a
// tool gave back — git commit's own line when there is one — and what you
// had last asked before it.
func conversationIn(file, short string) (Conversation, bool) {
	f, err := os.Open(file)
	if err != nil {
		return Conversation{}, false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	var asked string
	var seen *Conversation
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var e entry
			if json.Unmarshal(line, &e) == nil {
				if s := e.said(); s != "" {
					asked = s
				}
				if strings.Contains(string(line), short) {
					if out := e.toolOutput(); strings.Contains(out, short) {
						c := Conversation{Session: e.SessionID, Cwd: e.Cwd, File: file, At: e.Timestamp, Asked: asked}
						if slices.ContainsFunc(commitsMade(out), func(c [2]string) bool { return strings.HasPrefix(c[0], short) }) {
							c.Made = true
							return c, true
						}
						if seen == nil {
							seen = &c
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	if seen != nil {
		return *seen, true
	}
	return Conversation{}, false
}
