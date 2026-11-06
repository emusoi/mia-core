package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	Protocol = 1
	Prefix   = "mia-"
)

var ManifestTimeout = 2 * time.Second

type Verb struct {
	Name  string `json:"name"`
	Usage string `json:"usage,omitempty"`
	Help  string `json:"help,omitempty"`
}

// Manifest is what a plugin says it does: the commands it adds and the
// events it wants to hear. Plugins draw nothing in mia; what they want
// shown they say with mia signal.
type Manifest struct {
	Protocol int      `json:"protocol"`
	Help     string   `json:"help,omitempty"`
	Verbs    []Verb   `json:"verbs,omitempty"`
	Events   []string `json:"events,omitempty"`
}

type Plugin struct {
	Name     string   `json:"name"`
	Path     string   `json:"path,omitempty"`
	Enabled  bool     `json:"enabled"`
	ByConfig bool     `json:"by_config,omitempty"`
	Manifest Manifest `json:"manifest"`
	Problem  string   `json:"problem,omitempty"`
}

type Host struct {
	Dirs       []string
	StatePath  string
	Configured []string
}

func (h Host) Discover() map[string]string {
	found := map[string]string{}
	for _, dir := range h.Dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name, ok := strings.CutPrefix(entry.Name(), Prefix)
			if !ok || name == "" || found[name] != "" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
				found[name] = path
			}
		}
	}
	return found
}

func (h Host) enabledByState() ([]string, error) {
	data, err := os.ReadFile(h.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state struct {
		Enabled []string `json:"enabled"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("%s: %w", h.StatePath, err)
	}
	return state.Enabled, nil
}

func (h Host) writeState(enabled []string) error {
	sort.Strings(enabled)
	data, err := json.MarshalIndent(struct {
		Enabled []string `json:"enabled"`
	}{enabled}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.StatePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(h.StatePath, append(data, '\n'), 0o644)
}

func (h Host) Enable(name string) error {
	if _, ok := h.Discover()[name]; !ok {
		return fmt.Errorf("no %s%s on PATH or in the plugins directory", Prefix, name)
	}
	enabled, err := h.enabledByState()
	if err != nil {
		return err
	}
	if slices.Contains(enabled, name) {
		return nil
	}
	return h.writeState(append(enabled, name))
}

func (h Host) Disable(name string) error {
	enabled, err := h.enabledByState()
	if err != nil {
		return err
	}
	if !slices.Contains(enabled, name) {
		if slices.Contains(h.Configured, name) {
			return fmt.Errorf("%s is enabled by `plugins` in the config; remove it there", name)
		}
		return nil
	}
	return h.writeState(slices.DeleteFunc(enabled, func(n string) bool { return n == name }))
}

func (h Host) List() ([]Plugin, error) {
	found := h.Discover()
	byState, err := h.enabledByState()
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for name := range found {
		names[name] = true
	}
	for _, name := range append(slices.Clone(byState), h.Configured...) {
		names[name] = true
	}
	var plugins []Plugin
	for name := range names {
		p := Plugin{
			Name:     name,
			Path:     found[name],
			ByConfig: slices.Contains(h.Configured, name),
		}
		p.Enabled = p.ByConfig || slices.Contains(byState, name)
		switch {
		case p.Path == "":
			p.Problem = "enabled, but no " + Prefix + name + " is installed"
		case p.Enabled:
			p.Manifest, err = h.manifest(p.Path)
			if err != nil {
				p.Problem = err.Error()
			}
		}
		plugins = append(plugins, p)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, nil
}

func (h Host) Enabled() ([]Plugin, error) {
	all, err := h.List()
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(p Plugin) bool { return !p.Enabled || p.Problem != "" }), nil
}

// manifestCache is what each plugin said its manifest was, by the file's
// resolved path, while the file stays the same size and age: asking costs a
// process each, every poll, event and radar, and mia-share alone takes 0.1s.
type manifestCache map[string]struct {
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Manifest Manifest  `json:"manifest"`
}

func (h Host) cachePath() string {
	if h.StatePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(h.StatePath), "cache", "manifests.json")
}

// manifest is the plugin's manifest, from the cache while its file is
// unchanged, else asked for and kept. A manifest that fails is never kept.
func (h Host) manifest(path string) (Manifest, error) {
	cachePath := h.cachePath()
	real, err := filepath.EvalSymlinks(path)
	info, statErr := os.Stat(real)
	if cachePath == "" || err != nil || statErr != nil {
		return ReadManifest(path)
	}
	cache := manifestCache{}
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &cache)
	}
	if hit, ok := cache[real]; ok && hit.Size == info.Size() && hit.Modified.Equal(info.ModTime()) {
		return hit.Manifest, nil
	}
	m, err := ReadManifest(path)
	if err != nil {
		return m, err
	}
	entry := cache[real]
	entry.Size, entry.Modified, entry.Manifest = info.Size(), info.ModTime(), m
	cache[real] = entry
	if data, err := json.Marshal(cache); err == nil && os.MkdirAll(filepath.Dir(cachePath), 0o755) == nil {
		// another mia may write at the same time: write aside, then rename
		if temp, err := os.CreateTemp(filepath.Dir(cachePath), ".manifests-*"); err == nil {
			_, werr := temp.Write(data)
			cerr := temp.Close()
			if werr == nil && cerr == nil {
				_ = os.Rename(temp.Name(), cachePath)
			} else {
				_ = os.Remove(temp.Name())
			}
		}
	}
	return m, nil
}

func ReadManifest(path string) (Manifest, error) {
	out, err := call(path, os.Environ(), []string{"manifest"}, nil, ManifestTimeout)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		return Manifest{}, fmt.Errorf("manifest is not JSON: %w", err)
	}
	if m.Protocol != Protocol {
		return Manifest{}, fmt.Errorf("speaks protocol %d; this mia speaks %d", m.Protocol, Protocol)
	}
	for _, event := range m.Events {
		if event != "*" && !slices.Contains(Events, event) {
			return Manifest{}, fmt.Errorf("no event called %q; there are %s", event, strings.Join(Events, ", "))
		}
	}
	return m, nil
}

func call(path string, env []string, args []string, stdin []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &out, &errOut
	// A plugin that runs out of time takes what it started with it: a shell
	// plugin's `mia` and `podman` children would otherwise run on, orphaned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
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

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func (p Plugin) Declares(verb string) bool {
	return slices.ContainsFunc(p.Manifest.Verbs, func(v Verb) bool { return v.Name == verb })
}

func Owner(plugins []Plugin, verb string) (Plugin, bool) {
	for _, p := range plugins {
		if p.Declares(verb) {
			return p, true
		}
	}
	return Plugin{}, false
}

type Context struct {
	Repo     string
	MiaDir   string
	Worktree string
	Settings map[string]any
}

func (p Plugin) DataDir(c Context) string {
	return filepath.Join(c.MiaDir, "plugins", p.Name)
}

func (p Plugin) Env(c Context) ([]string, error) {
	data := p.DataDir(c)
	if err := os.MkdirAll(data, 0o755); err != nil {
		return nil, err
	}
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return nil, err
	}
	if c.Settings == nil {
		settings = []byte("{}")
	}
	env := append(os.Environ(),
		"MIA_PLUGIN="+p.Name,
		"MIA_PLUGIN_PROTOCOL="+fmt.Sprint(Protocol),
		"MIA_PLUGIN_DATA="+data,
		"MIA_PLUGIN_CONFIG="+string(settings),
		"MIA_REPO="+c.Repo,
	)
	if c.Worktree != "" {
		env = append(env, "MIA_WORKTREE="+c.Worktree)
	}
	return env, nil
}

func (p Plugin) Run(c Context, args []string) (int, error) {
	env, err := p.Env(c)
	if err != nil {
		return 1, err
	}
	cmd := exec.Command(p.Path, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
