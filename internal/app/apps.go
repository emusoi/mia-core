package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"

	"github.com/emusoi/mia-core/internal/paths"
	"github.com/emusoi/mia-core/internal/session"
)

// Manifest is one app as it declares itself in apps/<name>/app.toml of
// mia's folder: what to call it, how to tell a window that runs it, its
// icon and the keys worth showing beside it. The folder's name is its name.
type Manifest struct {
	Name  string `toml:"-" json:"name"`
	Label string `toml:"label" json:"label,omitempty"`
	// Icon is a file beside app.toml as it is written, and that file's
	// whole path as it is said; "" when the file is not there.
	Icon  string        `toml:"icon" json:"icon,omitempty"`
	Match ManifestMatch `toml:"match" json:"match"`
	Keys  []ManifestKey `toml:"keys" json:"keys"`
}

// ManifestMatch is how a pane is told to run the app: its command is one of
// Commands, or its command is one of Via — what a runtime shows as — and its
// title matches Title.
type ManifestMatch struct {
	Commands []string `toml:"commands" json:"commands"`
	Title    string   `toml:"title" json:"title,omitempty"`
	Via      []string `toml:"via" json:"via"`

	title *regexp.Regexp // Title, compiled as the manifest is read
}

// ManifestKey is a key of the app's and what it does; mia only passes it on.
type ManifestKey struct {
	Key   string `toml:"key" json:"key"`
	Label string `toml:"label" json:"label"`
}

// Apps is every app with a manifest that reads, by name, and a line for
// each that does not. They are read as they are now, each time. An agent's
// name is not an app's to take.
func (a *App) Apps() ([]Manifest, []string) {
	root := paths.In("apps")
	if root == "" {
		return []Manifest{}, nil
	}
	entries, _ := os.ReadDir(root) // by name; none when there is no folder
	apps, problems := []Manifest{}, []string(nil)
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		file := filepath.Join(dir, "app.toml")
		if _, err := os.Stat(file); err != nil {
			continue
		}
		if _, taken := agentNamed(a.agents(), entry.Name()); taken {
			problems = append(problems, fmt.Sprintf("%s: %s is an agent mia knows, and no app's name", file, entry.Name()))
			continue
		}
		one, err := readManifest(dir, entry.Name())
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", file, err))
			continue
		}
		apps = append(apps, one)
	}
	return apps, problems
}

func readManifest(dir, name string) (Manifest, error) {
	one := Manifest{Name: name}
	if _, err := toml.DecodeFile(filepath.Join(dir, "app.toml"), &one); err != nil {
		return Manifest{}, err
	}
	match := &one.Match
	switch {
	case len(match.Commands) == 0 && match.Title == "":
		return Manifest{}, fmt.Errorf("[match] names no commands and no title")
	case match.Title != "" && len(match.Via) == 0:
		return Manifest{}, fmt.Errorf("[match] title is tried only under a command of via, and via names none")
	}
	if match.Title != "" {
		title, err := regexp.Compile(match.Title)
		if err != nil {
			return Manifest{}, fmt.Errorf("[match] title: %v", err)
		}
		match.title = title
	}
	if match.Commands == nil {
		match.Commands = []string{}
	}
	if match.Via == nil {
		match.Via = []string{}
	}
	if one.Keys == nil {
		one.Keys = []ManifestKey{}
	}
	icon := one.Icon
	one.Icon = ""
	if filepath.IsLocal(icon) {
		if info, err := os.Stat(filepath.Join(dir, icon)); err == nil && info.Mode().IsRegular() {
			one.Icon = filepath.Join(dir, icon)
		}
	}
	return one, nil
}

// apps is how a window is told to be an app, read once: the manifests, and
// the agents no manifest names over.
func (a *App) apps() session.Apps {
	a.appsOnce.Do(func() {
		manifests, _ := a.Apps() // what does not read is said by `mia api apps`
		for _, one := range manifests {
			a.appTable.Known = append(a.appTable.Known, session.App{Name: one.Name, Commands: one.Match.Commands, Title: one.Match.title, Via: one.Match.Via})
		}
		a.appTable.Agent = func(command, title string) bool { return agentIn(a.agents(), command, title) != "" }
	})
	return a.appTable
}
