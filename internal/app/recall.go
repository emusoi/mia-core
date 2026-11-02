package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
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
	Yours bool   `json:"yours"` // you said it; otherwise claude did
	Line  string `json:"line"`  // the words around it
}

// Recall is the claude conversations of the repository and its worktrees
// that mention text, newest first, one line each from where it came up —
// your own words over claude's.
func (a *App) Recall(text string, limit int) []Recalled {
	needle := strings.ToLower(text)
	var found []Recalled
	for _, file := range transcriptsHolding(a.Root, text) {
		if r, ok := recalledIn(file, needle); ok {
			found = append(found, r)
		}
	}
	// a subagent's transcript is its parent's session: keep the session once
	best := map[string]Recalled{}
	for _, r := range found {
		if b, ok := best[r.Session]; !ok || (r.Yours && !b.Yours) || (r.Yours == b.Yours && r.At.After(b.At)) {
			best[r.Session] = r
		}
	}
	found = found[:0]
	for _, r := range best {
		found = append(found, r)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].At.After(found[j].At) })
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	return found
}

// transcriptsHolding is the repository's transcripts holding text, any case.
func transcriptsHolding(root, text string) []string {
	var files []string
	for _, d := range transcriptDirs(root) {
		_ = filepath.WalkDir(d, func(path string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(path, ".jsonl") {
				files = append(files, path)
			}
			return nil
		})
	}
	return holding(files, text, true)
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

// recalledIn is the latest place in a transcript the needle comes up in
// what you said, else in what claude said.
func recalledIn(file, needle string) (Recalled, bool) {
	f, err := os.Open(file)
	if err != nil {
		return Recalled{}, false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	var yours, theirs *Recalled
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && strings.Contains(strings.ToLower(string(line)), needle) {
			var e entry
			if json.Unmarshal(line, &e) == nil {
				at := Conversation{Session: e.SessionID, Cwd: e.Cwd, File: file, At: e.Timestamp}
				if s := e.said(); strings.Contains(strings.ToLower(s), needle) {
					yours = &Recalled{at, true, around(s, needle)}
				} else if s := e.claudeSaid(); strings.Contains(strings.ToLower(s), needle) {
					theirs = &Recalled{at, false, around(s, needle)}
				}
			}
		}
		if err == io.EOF || err != nil {
			break
		}
	}
	if yours != nil {
		return *yours, true
	}
	if theirs != nil {
		return *theirs, true
	}
	return Recalled{}, false
}

// claudeSaid is the text claude wrote in an assistant entry.
func (e entry) claudeSaid() string {
	if e.Type != "assistant" {
		return ""
	}
	var blocks []block
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return ""
	}
	var out strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			out.WriteString(b.Text)
		}
	}
	return out.String()
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
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	dirs, _ := filepath.Glob(filepath.Join(dir, "projects", projectPrefix.ReplaceAllString(root, "-")+"*"))
	return dirs
}
