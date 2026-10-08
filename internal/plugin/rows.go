package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const DefaultEvery = 10 * time.Second

var CallTimeout = 2 * time.Second

type Schedule struct {
	Every string `json:"every,omitempty"`
}

func (s Schedule) Interval() (time.Duration, error) {
	if s.Every == "" {
		return DefaultEvery, nil
	}
	d, err := time.ParseDuration(s.Every)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("every %q is not a duration like 30s or 5m", s.Every)
	}
	return d, nil
}

type Section struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Rank      int    `json:"rank"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type RowInfo struct {
	Facts   []string            `json:"facts,omitempty"`
	Status  string              `json:"status,omitempty"`
	Note    string              `json:"note,omitempty"`
	Section string              `json:"section,omitempty"`
	Tabs    map[string][]string `json:"tabs,omitempty"`
}

type Rows struct {
	Plugin   string             `json:"plugin"`
	Sections []Section          `json:"sections,omitempty"`
	Tabs     []Tab              `json:"tabs,omitempty"`
	Keys     []Key              `json:"keys,omitempty"`
	Rows     map[string]RowInfo `json:"rows"`
	Problem  string             `json:"problem,omitempty"`
}

type cached struct {
	At   time.Time          `json:"at"`
	Rows map[string]RowInfo `json:"rows"`
}

func call(path string, env []string, args []string, stdin []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.WaitDelay = 100 * time.Millisecond
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%s took longer than %s", args[0], timeout)
		}
		if line := firstLine(errOut.String()); line != "" {
			return nil, fmt.Errorf("%s failed: %s", args[0], line)
		}
		return nil, fmt.Errorf("%s failed: %w", args[0], err)
	}
	return out.Bytes(), nil
}

func (p Plugin) Rows(c Context, input any) Rows {
	result := Rows{Plugin: p.Name, Sections: p.Manifest.Sections, Tabs: p.Manifest.Tabs, Keys: p.Manifest.Keys}
	if p.Manifest.Rows == nil {
		return result
	}
	every, _ := p.Manifest.Rows.Interval()
	cachePath := filepath.Join(p.DataDir(c), "rows.json")
	var last cached
	if data, err := os.ReadFile(cachePath); err == nil && json.Unmarshal(data, &last) == nil {
		result.Rows = last.Rows
		if p.Server == nil && time.Since(last.At) < every {
			return result
		}
	}
	fresh, err := p.fetchRows(c, input)
	if err != nil {
		result.Problem = err.Error()
		return result
	}
	result.Rows = fresh
	if data, err := json.Marshal(cached{At: time.Now(), Rows: fresh}); err == nil {
		_ = os.WriteFile(cachePath, data, 0o644)
	}
	return result
}

func (p Plugin) fetchRows(c Context, input any) (map[string]RowInfo, error) {
	out, err := p.ask(c, []string{"rows"}, "rows", input, input)
	if err != nil {
		return nil, err
	}
	var answer struct {
		Rows map[string]RowInfo `json:"rows"`
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		return nil, fmt.Errorf("rows is not JSON: %w", err)
	}
	return answer.Rows, nil
}

func AllRows(plugins []Plugin, contextOf func(Plugin) Context, input any) []Rows {
	all := make([]Rows, len(plugins))
	var wg sync.WaitGroup
	for i, p := range plugins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			all[i] = p.Rows(contextOf(p), input)
		}()
	}
	wg.Wait()
	return all
}

func (p Plugin) Panel(c Context, id string, input any) ([]byte, error) {
	return p.ask(c, []string{"panel", id}, "panel", panelParams{ID: id, Input: input}, input)
}

type panelParams struct {
	ID    string `json:"id"`
	Input any    `json:"input"`
}

func (p Plugin) ask(c Context, args []string, method string, params, stdin any) ([]byte, error) {
	if p.Server != nil {
		return p.Server.Call(method, params, CallTimeout)
	}
	env, err := p.Env(c)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(stdin)
	if err != nil {
		return nil, err
	}
	return call(p.Path, env, args, data, CallTimeout)
}
