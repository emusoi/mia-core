package app

import (
	"bytes"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"sync"
	gutf8 "unicode/utf8"
)

// Recalled is one conversation that mentions what was asked for.
type Recalled struct {
	Conversation
	Agent string `json:"agent"` // whose conversation it is: claude, codex, pi, a plugin's
	// Worktree is the one its folder is or is in, as a list of
	// conversations names it: which copy n counts in, of one that was copied.
	Worktree string `json:"worktree"`
	N        int    `json:"n"`     // the turn it came up in, to open the conversation at; 0 when in a subagent's
	Yours    bool   `json:"yours"` // you said it; otherwise the agent did
	Line     string `json:"line"`  // the words around it
}

// Recall is the agent conversations of the repository and its worktrees
// that mention text, newest first, one line each from where it came up —
// your own words over the agent's. None is an empty list, not nil: it is
// printed as JSON.
func (a *App) Recall(text string, limit int) []Recalled {
	in, records := a.where()
	found := []Recalled{}
	for _, agent := range a.agents() {
		if agent.talks == nil {
			continue
		}
		its, _ := agent.talks.search(in, text, limit) // an agent that cannot say hides no other's
		found = append(found, its...)
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].At.After(found[j].At) })
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	for i := range found {
		found[i].Worktree, _ = a.worktreeOf(found[i].Cwd, records)
	}
	return found
}

// holding is the files that hold needle, in any ASCII case when fold,
// read a few at once. Go reads them, as /usr/bin/grep on macOS takes
// seconds where this takes a fraction of one.
func holding(files []string, needle string, fold bool) []string {
	want := []byte(needle)
	if fold {
		want = asciiLower(want)
	}
	hit := make([]bool, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	for range goruntime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				data, err := os.ReadFile(files[i])
				if err != nil {
					continue
				}
				if fold {
					data = asciiLower(data)
				}
				hit[i] = bytes.Contains(data, want)
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	var found []string
	for i, f := range files {
		if hit[i] {
			found = append(found, f)
		}
	}
	return found
}

func asciiLower(b []byte) []byte {
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return b
}

// around is the words either side of the needle, on one line.
func around(text, needle string) string {
	text = strings.Join(strings.Fields(text), " ")
	i := strings.Index(strings.ToLower(text), needle)
	if i < 0 {
		i = 0
	}
	start, end := max(i-60, 0), min(i+len(needle)+80, len(text))
	for start > 0 && !gutf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !gutf8.RuneStart(text[end]) {
		end++
	}
	out := text[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}

// transcriptDirs is claude's project folders for the repository and its
// worktrees: claude names each after its path, every other character a -,
// and a worktree <root>.<name> starts the same.
func transcriptDirs(root string) []string {
	dirs, _ := filepath.Glob(filepath.Join(claudeProjects(), projectPrefix.ReplaceAllString(root, "-")+"*"))
	return dirs
}

func claudeProjects() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "projects")
}
