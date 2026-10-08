package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/emusoi/mia-core/internal/git"
)

type Store struct{ Path string }

type edges map[string]string

type file struct {
	Parents edges             `json:"parents"`
	Names   map[string]string `json:"names,omitempty"`
}

func (s Store) read() (file, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return file{Parents: edges{}, Names: map[string]string{}}, nil
	}
	if err != nil {
		return file{}, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil || f.Parents == nil {
		var flat edges
		if err := json.Unmarshal(data, &flat); err != nil {
			return file{}, fmt.Errorf("read %s: %w", s.Path, err)
		}
		f = file{Parents: flat}
	}
	if f.Names == nil {
		f.Names = map[string]string{}
	}
	return f, nil
}

func (s Store) load() (edges, error) {
	f, err := s.read()
	if err != nil {
		return nil, err
	}
	return f.Parents, nil
}

func (s Store) save(parents edges) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	f.Parents = parents
	return s.write(f)
}

func (s Store) Name(name, bottom string) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	for existing, b := range f.Names {
		if b == bottom {
			delete(f.Names, existing)
		}
	}
	f.Names[name] = bottom
	return s.write(f)
}

func (s Store) NameOf(bottom string) string {
	f, err := s.read()
	if err != nil {
		return ""
	}
	for name, b := range f.Names {
		if b == bottom {
			return name
		}
	}
	return ""
}

func (s Store) BottomOf(branch, base string) string {
	parents, err := s.load()
	if err != nil {
		return branch
	}
	bottom := branch
	for {
		parent, ok := parents[bottom]
		if !ok || parent == base || parent == "" {
			return bottom
		}
		bottom = parent
	}
}

func (s Store) Bottom(name string) (string, bool) {
	f, err := s.read()
	if err != nil {
		return "", false
	}
	b, ok := f.Names[name]
	return b, ok
}

func (s Store) write(f file) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.Path), ".mia-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.Path)
}

func (s Store) Record(branch, parent string) error {
	parents, err := s.load()
	if err != nil {
		return err
	}
	for above, on := range parents {
		if on == parent && above != branch {
			parents[above] = branch
		}
	}
	parents[branch] = parent
	return s.save(parents)
}

func (s Store) Drop(branches []string) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	gone := map[string]bool{}
	for _, b := range branches {
		gone[b] = true
		delete(f.Parents, b)
	}
	for name, bottom := range f.Names {
		if gone[bottom] {
			delete(f.Names, name)
		}
	}
	return s.write(f)
}

func (s Store) Forget(branch string) error {
	parents, err := s.load()
	if err != nil {
		return err
	}
	below := parents[branch]
	delete(parents, branch)
	for above, parent := range parents {
		if parent == branch {
			parents[above] = below
		}
	}
	return s.save(parents)
}

type Layer struct {
	Branch       string `json:"branch"`
	Parent       string `json:"parent"`
	Landed       bool   `json:"landed"`
	Ahead        int    `json:"ahead"`
	NeedsRestack bool   `json:"needs_restack"`
	Current      bool   `json:"current"`
	Worktree     string `json:"worktree,omitempty"`
}

func (s Store) MemberOrParent(branch string) bool {
	parents, err := s.load()
	if err != nil || branch == "" {
		return false
	}
	if _, ok := parents[branch]; ok {
		return true
	}
	for _, parent := range parents {
		if parent == branch {
			return true
		}
	}
	return false
}

type Counts struct {
	mu    sync.Mutex
	known map[string][2]int
	path  string
	dirty bool
}

func CountsAt(path string) *Counts {
	c := &Counts{known: map[string][2]int{}, path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c.known)
	}
	return c
}

func (c *Counts) Save() {
	if c == nil || c.path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return
	}
	if len(c.known) > 5000 {
		c.known = map[string][2]int{}
	}
	if data, err := json.Marshal(c.known); err == nil {
		_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
		_ = os.WriteFile(c.path, data, 0o644)
	}
	c.dirty = false
}

func (c *Counts) between(dir, parent, parentSha, layer, layerSha string) (behind, ahead int) {
	key := parentSha + ".." + layerSha
	if c != nil {
		c.mu.Lock()
		if got, ok := c.known[key]; ok {
			c.mu.Unlock()
			return got[0], got[1]
		}
		c.mu.Unlock()
	}
	behind, ahead = git.BehindAhead(dir, parent, layer)
	if c != nil {
		c.mu.Lock()
		c.known[key] = [2]int{behind, ahead}
		c.dirty = true
		c.mu.Unlock()
	}
	return behind, ahead
}

func (c *Counts) ancestor(dir, name, sha, base, baseSha string) bool {
	if sha == "" || baseSha == "" {
		return git.IsAncestor(dir, name, base)
	}
	key := "landed:" + sha + ".." + baseSha
	if c != nil {
		c.mu.Lock()
		got, ok := c.known[key]
		c.mu.Unlock()
		if ok {
			return got[0] == 1
		}
	}
	yes := git.IsAncestor(dir, name, base)
	if c != nil {
		c.mu.Lock()
		c.known[key] = [2]int{map[bool]int{true: 1}[yes], 0}
		c.dirty = true
		c.mu.Unlock()
	}
	return yes
}

func (s Store) Of(dir, branch, base string) ([]Layer, error) {
	return s.OfWith(dir, branch, base, nil)
}

type Survey struct {
	Base   string
	Heads  map[string]string
	Merged map[string]bool
}

func SurveyOf(dir, base string) *Survey {
	survey := &Survey{Base: base, Heads: git.Heads(dir)}
	if base != "" {
		survey.Merged = git.MergedInto(dir, base)
	}
	return survey
}

func (s Store) OfWith(dir, branch, base string, counts *Counts) ([]Layer, error) {
	return s.OfSurveyed(dir, branch, base, counts, nil)
}

func (s Store) OfSurveyed(dir, branch, base string, counts *Counts, survey *Survey) ([]Layer, error) {
	parents, err := s.load()
	if err != nil {
		return nil, err
	}

	bottom := branch
	for {
		parent, ok := parents[bottom]
		if !ok || parent == base || parent == "" {
			break
		}
		bottom = parent
	}

	ordered := []string{bottom}
	seen := map[string]bool{bottom: true}
	for {
		var children []string
		for above, parent := range parents {
			if parent == ordered[len(ordered)-1] && !seen[above] {
				children = append(children, above)
			}
		}
		if len(children) == 0 {
			break
		}
		sort.Strings(children)
		ordered = append(ordered, children[0])
		seen[children[0]] = true
	}

	if survey == nil {
		survey = SurveyOf(dir, base)
	}
	heads, merged := survey.Heads, survey.Merged
	landed := func(name string) bool {
		if merged != nil {
			return merged[name]
		}
		if base == "" {
			return false
		}
		return counts.ancestor(dir, name, heads[name], base, heads[base])
	}
	layers := make([]Layer, 0, len(ordered))
	for _, name := range ordered {
		if _, ok := heads[name]; !ok {
			continue
		}
		parent := parents[name]
		if parent == "" {
			parent = base
		}
		behind, ahead := 0, 0
		if parentSha, ok := heads[parent]; ok || parent == base {
			if !ok {
				parentSha = parent
			}
			behind, ahead = counts.between(dir, parent, parentSha, name, heads[name])
		}
		isLanded := landed(name)
		layers = append(layers, Layer{
			Branch:       name,
			Parent:       parent,
			Landed:       isLanded,
			Ahead:        ahead,
			NeedsRestack: !isLanded && heads[parent] != "" && behind > 0,
			Current:      name == branch,
		})
	}
	return layers, nil
}
func (s Store) Member(branch string) bool {
	parents, err := s.load()
	if err != nil || branch == "" {
		return false
	}
	_, ok := parents[branch]
	return ok
}
