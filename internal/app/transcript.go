package app

import (
	"bufio"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// transcripts is the conversations of an agent that writes each to a file
// of JSON lines, which is all three mia has built in. What differs between
// them is where the files are and what a line means.
type transcripts struct {
	agent string
	// files is the transcripts in an account's folder that may be the
	// folders'; deep adds those of subagents, which belong to their
	// parent's conversation.
	files func(home string, in folders, deep bool) []string
	// find is the transcripts in an account's folder of one conversation:
	// one, or one for each folder it was copied to, and none when there is
	// none.
	find func(home string, in folders, id string) []string
	// id is the conversation a transcript is of, from its name.
	id func(path string) string
	// decode reads one line.
	decode decoder
	// cache is the folder what was read of each file is kept in.
	cache string
}

// decoder reads one line of an agent's transcript: what the line says of
// the conversation itself goes into about, and the turns it holds come back
// in order, not yet numbered.
type decoder func(line []byte, about *about) []Turn

// about is what a transcript says of its conversation.
type about struct {
	Cwd     string    `json:"cwd,omitempty"`
	Started time.Time `json:"started,omitzero"`
}

// mark is a place in a transcript reading can go on from: how far in, how
// many turns came before, and the questions still unanswered there.
type mark struct {
	Offset int64          `json:"offset"`
	N      int            `json:"n"`
	Open   map[string]int `json:"open,omitempty"` // a question's ref → its turn
}

// scan is what is kept of a transcript between runs, so that a list reads
// only what was written since, and a view that polls only what is new.
type scan struct {
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified,omitzero"`
	about
	Asked string `json:"asked,omitempty"`
	Said  string `json:"said,omitempty"`
	Asks  int    `json:"asks,omitempty"`
	// Marks are the last few ends of the file that were read to; the last is
	// where the summary above stands.
	Marks []mark `json:"marks,omitempty"`
}

// marksKept is how many places a transcript keeps to go on from: a view
// polls from where it last read to, and a list has usually read past that
// since.
const marksKept = 8

func (s scan) end() mark {
	if len(s.Marks) == 0 {
		return mark{}
	}
	return s.Marks[len(s.Marks)-1]
}

// read reads a transcript from a mark to its last whole line, numbering
// each turn and handing it to each, and returns where it stopped. stop,
// when given, is asked after every line whether to give up.
func (t transcripts) read(path string, from mark, about *about, each func(Turn), stop func() bool) (mark, error) {
	f, err := os.Open(path)
	if err != nil {
		return from, err
	}
	defer f.Close()
	if _, err := f.Seek(from.Offset, io.SeekStart); err != nil {
		return from, err
	}
	at := mark{Offset: from.Offset, N: from.N, Open: maps.Clone(from.Open)}
	// a line at a time: a transcript runs to hundreds of megabytes
	r := bufio.NewReaderSize(f, 1<<20)
	var long []byte // a line longer than the reader holds, put together here
	for {
		line, err := r.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			long = append(long[:0], line...)
			for err == bufio.ErrBufferFull {
				line, err = r.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if err != nil {
			break // the end, or a line still being written: read next time
		}
		at.Offset += int64(len(line))
		for _, turn := range t.decode(line, about) {
			at.N++
			turn.N = at.N
			switch {
			case turn.Kind == turnQuestion && turn.ref != "":
				if at.Open == nil {
					at.Open = map[string]int{}
				}
				at.Open[turn.ref] = turn.N
			case turn.Kind == turnAnswer:
				turn.Question = at.Open[turn.ref]
				delete(at.Open, turn.ref)
			}
			each(turn)
		}
		if stop != nil && stop() {
			break
		}
	}
	return at, nil
}

// advance reads what was written to a transcript since s was kept, and
// reports whether s changed. A file that shrank is read from its start. A
// conversation that turns out not to be the folders' is read no further.
// ponytail: a file rewritten in place to the same size or longer is taken
// as appended to; all three agents only append.
func (t transcripts) advance(path string, s *scan, in *folders, each func(Turn)) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	foreign := func() bool { return in != nil && s.Cwd != "" && !in.holds(s.Cwd) }
	if foreign() || (s.Size == info.Size() && s.Modified.Equal(info.ModTime())) {
		return false
	}
	if info.Size() < s.end().Offset {
		*s = scan{}
	}
	end, err := t.read(path, s.end(), &s.about, func(turn Turn) {
		switch turn.Kind {
		case turnAsked:
			if s.Asks == 0 {
				s.Asked = oneLine(turn.Text)
			}
			s.Asks++
		case turnSaid:
			s.Said = oneLine(turn.Text)
		}
		if each != nil {
			each(turn)
		}
	}, foreign)
	if err != nil {
		return false
	}
	if foreign() {
		s.Marks = nil // only whose it is is kept
		return true
	}
	s.Size, s.Modified = info.Size(), info.ModTime()
	if end.Offset != s.end().Offset || len(s.Marks) == 0 {
		s.Marks = append(s.Marks, end)
		if len(s.Marks) > marksKept {
			s.Marks = slices.Delete(s.Marks, 0, len(s.Marks)-marksKept)
		}
	}
	return true
}

func (t transcripts) cacheFile() string {
	if t.cache == "" {
		return ""
	}
	return filepath.Join(t.cache, t.agent+".json")
}

func (t transcripts) kept() map[string]*scan {
	all := map[string]*scan{}
	if data, err := os.ReadFile(t.cacheFile()); err == nil {
		_ = json.Unmarshal(data, &all)
	}
	return all
}

func (t transcripts) keep(all map[string]*scan) {
	file := t.cacheFile()
	if file == "" || os.MkdirAll(filepath.Dir(file), 0o755) != nil {
		return
	}
	if data, err := json.Marshal(all); err == nil {
		_ = writeAtomic(file, data)
	}
}

// all is the transcripts in every account's folder of the agent that may be
// the folders', each with the account it is in.
func (t transcripts) all(in folders, deep bool) ([]string, map[string]string) {
	var files []string
	accounts := map[string]string{}
	for _, h := range homesOf(t.agent) {
		for _, file := range t.files(h.dir, in, deep) {
			if _, seen := accounts[file]; !seen {
				files, accounts[file] = append(files, file), h.account
			}
		}
	}
	return files, accounts
}

func (t transcripts) heads(in folders) ([]Head, error) {
	files, accounts := t.all(in, false)
	was := t.kept()
	now := make(map[string]*scan, len(files)) // of the files there are: what is kept of one that went is dropped
	changed := make([]bool, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	for _, file := range files {
		s := was[file]
		if s == nil {
			s = &scan{}
		}
		now[file] = s
	}
	for range goruntime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				changed[i] = t.advance(files[i], now[files[i]], &in, nil)
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	if slices.Contains(changed, true) || len(now) != len(was) {
		t.keep(now)
	}
	var heads []Head
	for _, file := range files {
		s := now[file]
		if !in.holds(s.Cwd) || s.end().N == 0 {
			continue // someone else's, or nothing said in it yet
		}
		heads = append(heads, Head{ID: t.id(file), Agent: t.agent, Account: accounts[file], Cwd: s.Cwd, Asked: s.Asked, Said: s.Said,
			Started: s.Started, Active: s.Modified, Turns: s.Asks})
	}
	return heads, nil
}

func (t transcripts) turns(in folders, id, cwd string, after int) ([]Turn, bool, error) {
	return t.locate(in, id, cwd, after, nil)
}

// placed is how far a transcript that is followed has been read: the file,
// where reading stopped, and its size and time when it was.
type placed struct {
	file     string
	end      mark
	size     int64
	modified time.Time
}

// follow finds the transcript as turns does, then reads it on from where
// it stopped each time its size or time changes. A turn of a transcript
// is a whole line, written once: none grows under its n.
func (t transcripts) follow(in folders, id, cwd string, after int) (func() ([]Turn, bool, error), time.Duration) {
	var at placed
	return func() ([]Turn, bool, error) {
		if at.file == "" {
			return t.locate(in, id, cwd, after, &at)
		}
		info, err := os.Stat(at.file)
		if err != nil {
			return nil, true, err
		}
		if info.Size() == at.size && info.ModTime().Equal(at.modified) {
			return nil, true, nil
		}
		from := at.end
		if info.Size() < from.Offset {
			from = mark{} // written again from its start: each turn is given again, under its n
		}
		at.size, at.modified = info.Size(), info.ModTime()
		var turns []Turn
		if at.end, err = t.read(at.file, from, &about{}, collecting(after, &turns), nil); err != nil {
			return nil, true, err
		}
		return turns, true, nil
	}, 150 * time.Millisecond
}

// collecting gathers the turns after after into turns, an answer given to
// the question it answers when that is among them.
func collecting(after int, turns *[]Turn) func(Turn) {
	return func(turn Turn) {
		if turn.Kind == turnAnswer {
			for i := range *turns {
				if (*turns)[i].N == turn.Question {
					(*turns)[i].Answer = turn.Text
				}
			}
		}
		if turn.N > after {
			*turns = append(*turns, turn)
		}
	}
}

// locate is turns, and which transcript was read and how far, into at when
// it is given.
func (t transcripts) locate(in folders, id, cwd string, after int, at *placed) ([]Turn, bool, error) {
	if strings.ContainsAny(id, `/\*?[`) {
		return nil, false, nil
	}
	// one copied to another folder, or another account, is there once for
	// each, under one id: the one made in cwd is read, or with none the
	// last written to, as it is the first in a list
	var files []string
	for _, h := range homesOf(t.agent) {
		for _, file := range t.find(h.dir, in, id) {
			if !slices.Contains(files, file) {
				files = append(files, file)
			}
		}
	}
	written := func(file string) time.Time {
		info, err := os.Stat(file)
		if err != nil {
			return time.Time{}
		}
		return info.ModTime()
	}
	slices.SortStableFunc(files, func(x, y string) int { return written(y).Compare(written(x)) })
	all := t.kept()
	for _, file := range files {
		s := all[file]
		if s == nil {
			s = &scan{}
			all[file] = s
		}
		var turns []Turn
		collect := collecting(after, &turns)
		// what is new is read once, for the list as well; a view that has kept
		// up needs nothing older
		from := s.end()
		if t.advance(file, s, nil, collect) {
			t.keep(all)
		}
		if !in.holds(s.Cwd) || (cwd != "" && s.Cwd != cwd) {
			continue
		}
		if after < from.N {
			// further back than the last read: from the latest place at or before it
			start := mark{}
			for _, m := range s.Marks {
				if m.N <= after {
					start = m
				}
			}
			turns = nil
			if _, err := t.read(file, start, &about{}, collect, nil); err != nil {
				return nil, true, err
			}
		}
		if at != nil {
			*at = placed{file: file, end: s.end(), size: s.Size, modified: s.Modified}
		}
		return turns, true, nil
	}
	return nil, false, nil
}

func (t transcripts) search(in folders, text string, limit int) ([]Recalled, error) {
	needle := strings.ToLower(text)
	files, accounts := t.all(in, true)
	written := map[string]time.Time{}
	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			written[file] = info.ModTime()
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return written[files[i]].After(written[files[j]]) })
	// newest first, stopping once there are enough: a common word is in
	// nearly every transcript, and the old ones would come last anyway
	// One for each transcript, its subagents' with it. A conversation copied
	// to another folder is two, under one id, and each numbers its own turns.
	best := map[string]Recalled{} // by the transcript's path, less .jsonl
	for _, file := range holding(files, text, true) {
		if limit > 0 && len(best) >= limit {
			break
		}
		// a subagent's transcript is its parent's conversation, and the one
		// asking in it is the agent that sent it
		id, subagent, home := t.id(file), false, strings.TrimSuffix(file, ".jsonl")
		if dir := filepath.Dir(file); filepath.Base(dir) == "subagents" {
			id, subagent, home = filepath.Base(filepath.Dir(dir)), true, filepath.Dir(dir)
		}
		var yours, theirs *Recalled
		var is about
		_, _ = t.read(file, mark{}, &is, func(turn Turn) {
			if (turn.Kind != turnAsked && turn.Kind != turnSaid) || !strings.Contains(strings.ToLower(turn.Text), needle) {
				return
			}
			r := &Recalled{Conversation: Conversation{Session: id, File: file, At: turn.At, Account: accounts[file]}, Agent: t.agent, N: turn.N, Line: around(turn.Text, needle)}
			if subagent {
				r.N = 0 // not a turn of the conversation itself
			}
			if turn.Kind == turnAsked && !subagent {
				r.Yours, yours = true, r
			} else {
				theirs = r
			}
		}, nil)
		r := yours // your own words over the agent's, the latest of each
		if r == nil {
			r = theirs
		}
		if r == nil || !in.holds(is.Cwd) {
			continue
		}
		r.Cwd = is.Cwd
		if b, ok := best[home]; !ok || (r.Yours && !b.Yours) || (r.Yours == b.Yours && r.At.After(b.At)) {
			best[home] = *r
		}
	}
	return slices.Collect(maps.Values(best)), nil
}

// jsonl is the JSON-lines files right in each folder, and with deep those
// anywhere under it.
func jsonl(dirs []string, deep bool) []string {
	var files []string
	for _, d := range dirs {
		if !deep {
			found, _ := filepath.Glob(filepath.Join(d, "*.jsonl"))
			files = append(files, found...)
			continue
		}
		_ = filepath.WalkDir(d, func(path string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() && strings.HasSuffix(path, ".jsonl") {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

// dirsOf is an agent's folders for a repository's conversations: those
// whose name starts as the main checkout's does, which a worktree's at
// <root>.<name> does whether it is still there or not, and the one of each
// worktree kept somewhere else.
func dirsOf(in folders, base string, name func(path string) string, glob func(root string) string) []string {
	dirs, _ := filepath.Glob(filepath.Join(base, glob(in.Root)))
	for _, p := range in.Paths {
		if d := filepath.Join(base, name(p)); !slices.Contains(dirs, d) {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}
