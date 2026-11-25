package app

import (
	"bytes"
	"encoding/gob"
	"maps"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	gutf8 "unicode/utf8"
)

// Recalled is one conversation that mentions what was asked for.
type Recalled struct {
	Conversation
	Agent string `json:"agent"` // whose conversation it is: claude, codex, pi, opencode, agy, a plugin's
	// Worktree is the one its folder is or is in, as a list of
	// conversations names it: which copy n counts in, of one that was copied.
	Worktree string `json:"worktree"`
	N        int    `json:"n"`     // the turn it came up in, to open the conversation at; 0 when in a subagent's
	Yours    bool   `json:"yours"` // you said it; otherwise the agent did
	Line     string `json:"line"`  // the words around it
}

// spoken is what recall keeps of one conversation: what was asked and said in it, without the
// tools' output that is most of a transcript, as it stood when the conversation was last
// written to.
type spoken struct {
	Active time.Time
	Last   int // the last turn read
	Turns  []spoke
	// Of a subagent's transcript, kept by its file: the conversation that sent it and its folder.
	Session, Cwd string
}

type spoke struct {
	N     int
	At    time.Time
	Yours bool
	Text  string
}

// Recall is the agent conversations of the repository and its worktrees
// that mention text, newest first, one line each from where it came up —
// your own words over the agent's. None is an empty list, not nil: it is
// printed as JSON. What is asked and said in each is kept beside the
// repository's other caches and read on from only where a conversation was
// written to since, so asking again is a look through that and not through
// every transcript (twelve seconds of them, in a repository much worked in).
func (a *App) Recall(text string, limit int) []Recalled {
	in, records := a.where()
	path := filepath.Join(a.MiaDir, "cache", "recall.gob")
	kept := map[string]*spoken{}
	if f, err := os.Open(path); err == nil {
		_ = gob.NewDecoder(f).Decode(&kept) // one that cannot be read is read again from the transcripts
		_ = f.Close()
	}
	var agents []Agent
	for _, agent := range a.agents() {
		if agent.talks != nil {
			agents = append(agents, agent)
		}
	}
	needle := strings.ToLower(text)
	var mu sync.Mutex
	var wg sync.WaitGroup
	found, seen, changed := []Recalled{}, map[string]bool{}, false
	subs := map[string]Recalled{} // what a conversation's subagents said, by the conversation
	for _, agent := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			heads, _ := agent.talks.heads(in) // an agent that cannot say hides no other's
			for _, h := range heads {
				key := agent.Name + "\x00" + h.ID + "\x00" + h.Cwd
				mu.Lock()
				s := kept[key]
				seen[key] = true
				mu.Unlock()
				if s == nil {
					s = &spoken{}
				}
				if active := instant(h.Active); len(s.Turns) == 0 || !s.Active.Equal(active) {
					// (Its last turn again too: it may have been read while it was being written.)
					after := max(s.Last-1, 0)
					if turns, ok, err := agent.talks.turns(in, h.ID, h.Cwd, after); err == nil && ok {
						s.Turns = slices.DeleteFunc(s.Turns, func(t spoke) bool { return t.N > after })
						for _, t := range turns {
							if (t.Kind == turnAsked || t.Kind == turnSaid) && t.Text != "" {
								s.Turns = append(s.Turns, spoke{N: t.N, At: t.At, Yours: t.Kind == turnAsked, Text: t.Text})
							}
							s.Last = max(s.Last, t.N)
						}
						s.Active = active
						mu.Lock()
						kept[key], changed = s, true
						mu.Unlock()
					}
				}
				if best := s.mentions(needle); best != nil {
					r := Recalled{Conversation: Conversation{Session: h.ID, Cwd: h.Cwd, At: best.At, Account: h.Account}, Agent: agent.Name, N: best.N, Yours: best.Yours, Line: around(best.Text, needle)}
					mu.Lock()
					found = append(found, r)
					mu.Unlock()
				}
			}
			its, ok := agent.talks.(transcripts)
			if !ok {
				return
			}
			for file, written := range its.asides(in) {
				key := agent.Name + "\x00sub\x00" + file
				mu.Lock()
				s := kept[key]
				seen[key] = true
				mu.Unlock()
				if s == nil || !s.Active.Equal(written) {
					s = &spoken{Active: written}
					s.Session, s.Cwd, s.Turns = its.aside(file)
					mu.Lock()
					kept[key], changed = s, true
					mu.Unlock()
				}
				if best := s.mentions(needle); best != nil && in.holds(s.Cwd) {
					mu.Lock()
					subs[agent.Name+"\x00"+s.Session+"\x00"+s.Cwd] = Recalled{Conversation: Conversation{Session: s.Session, Cwd: s.Cwd, At: best.At}, Agent: agent.Name, Line: around(best.Text, needle)}
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	for _, r := range found { // a conversation that mentions it itself is listed once, by that
		delete(subs, r.Agent+"\x00"+r.Session+"\x00"+r.Cwd)
	}
	found = append(found, slices.Collect(maps.Values(subs))...)
	for key := range kept {
		if !seen[key] {
			delete(kept, key)
			changed = true
		}
	}
	if changed && os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		if f, err := os.CreateTemp(filepath.Dir(path), "recall-*"); err == nil {
			if gob.NewEncoder(f).Encode(kept) == nil && f.Close() == nil {
				_ = os.Rename(f.Name(), path)
			}
			_ = os.Remove(f.Name()) // gone already when it was renamed
		}
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

// mentions is the turn of it that holds needle: your own words over the agent's, the latest of
// each; nil when none does.
func (s *spoken) mentions(needle string) *spoke {
	var best *spoke
	for i := range s.Turns {
		t := &s.Turns[i]
		if strings.Contains(strings.ToLower(t.Text), needle) && (best == nil || t.Yours || !best.Yours) {
			best = t
		}
	}
	return best
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
// worktrees, in each of its accounts' folders: claude names each after its
// path, every other character a -, and a worktree <root>.<name> starts the
// same.
func transcriptDirs(root string) []string {
	var dirs []string
	for _, h := range homesOf("claude") {
		found, _ := filepath.Glob(filepath.Join(h.dir, "projects", projectPrefix.ReplaceAllString(root, "-")+"*"))
		dirs = append(dirs, found...)
	}
	return dirs
}
