package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emusoi/mia-core/internal/config"
	"github.com/emusoi/mia-core/internal/env"
	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/names"
	"github.com/emusoi/mia-core/internal/resolve"
	"github.com/emusoi/mia-core/internal/runtime"
	"github.com/emusoi/mia-core/internal/session"
	"github.com/emusoi/mia-core/internal/stack"
	"github.com/emusoi/mia-core/internal/store"

	"github.com/emusoi/mia-core/internal/run"
)

type App struct {
	Root       string
	MiaDir     string
	Config     config.Config
	Store      store.Store
	Names      *names.Store
	Runtimes   runtime.Store
	commits    sync.Map
	factsOnce  sync.Once
	factsDirty atomic.Bool
	counts     *stack.Counts
	survey     *stack.Survey
	trees      []git.Worktree
	dirtiness  sync.Map
	shots      sync.Map
	reach      sync.Map
	Events     func(Event)
}

type Event struct {
	Name   string
	Record model.Record
	Detail any
}

func (a *App) Emit(name string, record model.Record, detail any) {
	if a.Events != nil {
		a.Events(Event{Name: name, Record: record, Detail: detail})
	}
}

type reachability struct {
	ok     bool
	detail string
	at     time.Time
}

func (a *App) Reachable(machine runtime.Machine) (bool, string) {
	if got, ok := a.reach.Load(machine.Name); ok {
		if r := got.(reachability); time.Since(r.at) < 30*time.Second {
			return r.ok, r.detail
		}
	}
	ok, detail := runtime.Reachable(machine)
	a.reach.Store(machine.Name, reachability{ok, detail, time.Now()})
	return ok, detail
}

type dirtiness struct {
	dirty          bool
	at             time.Time
	untracked      time.Time
	untrackedDirty bool
}

func (a *App) ForgetDirtiness() { a.dirtiness = sync.Map{} }

func (a *App) dirtyOf(path string, worked bool) bool {
	ttl := 30 * time.Second
	if worked {
		ttl = 8 * time.Second
	}
	var d dirtiness
	if got, ok := a.dirtiness.Load(path); ok {
		d = got.(dirtiness)
		if time.Since(d.at) < ttl {
			return d.dirty
		}
	}
	dirty, _ := git.DirtyTracked(path)
	if !dirty {
		if time.Since(d.untracked) > 30*time.Second {
			d.untrackedDirty, _ = git.Dirty(path)
			d.untracked = time.Now()
		}
		dirty = d.untrackedDirty
	}
	d.dirty, d.at = dirty, time.Now()
	a.dirtiness.Store(path, d)
	return dirty
}

type commitFacts struct {
	Ahead   int       `json:"a"`
	Behind  int       `json:"b"`
	Last    time.Time `json:"t"`
	Subject string    `json:"s"`
	Files   int       `json:"f,omitempty"`
	Added   int       `json:"ad,omitempty"`
	Deleted int       `json:"de,omitempty"`
}

const factsVersion = "2\x00"

func (a *App) factsPath() string { return filepath.Join(a.MiaDir, "cache", "facts.json") }

type factsFile struct {
	Commits map[string]commitFacts `json:"commits"`
}

func (a *App) loadFacts() {
	a.factsOnce.Do(func() {
		data, err := os.ReadFile(a.factsPath())
		if err != nil {
			return
		}
		var known factsFile
		if json.Unmarshal(data, &known) == nil {
			for key, facts := range known.Commits {
				if strings.HasPrefix(key, factsVersion) {
					a.commits.Store(key, facts)
				}
			}
		}
	})
}

func (a *App) saveFacts() {
	if !a.factsDirty.Load() {
		return
	}
	known := factsFile{Commits: map[string]commitFacts{}}
	a.commits.Range(func(key, value any) bool {
		known.Commits[key.(string)] = value.(commitFacts)
		return len(known.Commits) < 5000
	})
	if data, err := json.Marshal(known); err == nil {
		_ = os.MkdirAll(filepath.Dir(a.factsPath()), 0o755)
		_ = os.WriteFile(a.factsPath(), data, 0o644)
	}
	a.factsDirty.Store(false)
}

func (a *App) commitFactsFor(tree git.Worktree, base, baseHead string) commitFacts {
	key := factsVersion + tree.Path + "\x00" + tree.Head + "\x00" + baseHead
	if cached, ok := a.commits.Load(key); ok {
		return cached.(commitFacts)
	}
	var facts commitFacts
	if tree.Branch != "" && !tree.Main {
		facts.Behind, facts.Ahead = git.BehindAhead(tree.Path, base, tree.Branch)
	}
	if facts.Ahead > 0 {
		facts.Files, facts.Added, facts.Deleted = git.DiffStat(tree.Path, base, tree.Branch)
	}
	facts.Last, facts.Subject = git.LastCommit(tree.Path)
	a.commits.Store(key, facts)
	a.factsDirty.Store(true)
	return facts
}

func Open(dir string) (*App, error) {
	top, common, err := git.Repository(dir)
	if err != nil {
		return nil, err
	}
	miaDir := filepath.Join(common, "mia")
	root := filepath.Dir(common)
	if filepath.Base(common) != ".git" {
		root = top
	}

	cfg, err := config.Load(miaDir)
	if err != nil {
		return nil, err
	}
	nameStore, err := globalNames()
	if err != nil {
		return nil, err
	}
	nameStore.Own = cfg.Names
	return &App{
		Root:     git.Resolve(root),
		MiaDir:   miaDir,
		Config:   cfg,
		Store:    store.Store{Dir: miaDir},
		Names:    nameStore,
		Runtimes: runtime.Store{Path: config.RuntimesPath()},
	}, nil
}

func globalNames() (*names.Store, error) {
	path := config.NamesPath()
	if path == "" {
		return nil, fmt.Errorf("cannot find a home directory to keep the name registry in")
	}
	return &names.Store{Path: path}, nil
}

func (a *App) Resolver() (resolve.Repo, error) {
	records, err := a.Store.Load()
	if err != nil {
		return resolve.Repo{}, err
	}
	return resolve.Repo{Root: a.Root, Records: records}, nil
}

func (a *App) NewFromPR(number int) (model.Record, error) {
	branch := fmt.Sprintf("pr-%d", number)
	if a.Config.Prefix != "" {
		branch = a.Config.Prefix + branch
	}
	trees, err := git.Worktrees(a.Root)
	if err != nil {
		return model.Record{}, err
	}
	for _, tree := range trees {
		if tree.Branch == branch {
			return a.New(branch)
		}
	}
	if err := git.FetchPR(a.Root, number, branch); err != nil {
		return model.Record{}, err
	}
	return a.New(branch)
}

func (a *App) New(branch string) (model.Record, error) {
	before, _ := a.Store.Load()
	record, err := a.newWorktree(branch)
	if record.Path != "" && !slices.ContainsFunc(before, func(one model.Record) bool { return one.Path == record.Path }) {
		a.Emit("worktree.created", record, nil)
	}
	return record, err
}

func (a *App) newWorktree(branch string) (model.Record, error) {
	if branch == "" {
		return model.Record{}, fmt.Errorf("`mia new <branch>` needs a branch name")
	}
	if a.Config.Prefix != "" && !strings.HasPrefix(branch, a.Config.Prefix) {
		branch = a.Config.Prefix + branch
	}
	if !git.ValidBranch(a.Root, branch) {
		return model.Record{}, fmt.Errorf("%q is not a valid branch name — no spaces, no .., no ~ ^ : ? * [ or \\", branch)
	}

	records, err := a.Store.Load()
	if err != nil {
		return model.Record{}, err
	}
	trees, err := git.Worktrees(a.Root)
	if err != nil {
		return model.Record{}, err
	}
	for _, tree := range trees {
		if tree.Branch == branch {
			for _, record := range records {
				if record.Path == tree.Path {
					return record, nil
				}
			}
			return a.adopt(tree.Path)
		}
	}

	path, name, err := a.reserve()
	if err != nil {
		return model.Record{}, err
	}
	create := !git.BranchExists(a.Root, branch)
	from := ""
	if create && a.Config.Base != "" && git.RevParse(a.Root, a.Config.Base) != "" {
		from = a.Config.Base
	}
	if err := git.AddFrom(a.Root, path, branch, create, from); err != nil {
		_ = a.Names.Release(path)
		return model.Record{}, err
	}

	record := model.Record{Path: git.Resolve(path), Name: name}
	if err := a.Store.Put(record); err != nil {
		return model.Record{}, err
	}
	if err := a.prepare(record.Path); err != nil {
		return record, fmt.Errorf("%s created, but preparing it failed: %w", record.Name, err)
	}
	if err := a.runSetup(record); err != nil {
		return record, fmt.Errorf("%s created, but setup failed: %w", record.Name, err)
	}
	return record, nil
}

func (a *App) reserve() (path, name string, err error) {
	name, err = a.Names.AllocateFor(func(candidate string) (string, bool) {
		final := a.Root + "." + candidate
		if _, statErr := os.Stat(final); statErr == nil {
			return "", false
		}
		return final, true
	})
	if err != nil {
		return "", "", err
	}
	return a.Root + "." + name, name, nil
}

func (a *App) adopt(path string) (model.Record, error) {
	name := ""
	if git.Resolve(path) == git.Resolve(a.Root) {
		if own := filepath.Base(a.Root); model.CheckName(own) == nil && a.Names.Claim(own, path) == nil {
			name = own
		}
	}
	if name == "" {
		allocated, err := a.Names.Allocate(path)
		if err != nil {
			return model.Record{}, err
		}
		name = allocated
	}
	record := model.Record{Path: git.Resolve(path), Name: name}
	return record, a.Store.Put(record)
}

func (a *App) Rename(query, name string) (model.Record, error) {
	record, err := a.RecordOrHere(query)
	if err != nil {
		return record, err
	}
	if err := model.CheckName(name); err != nil {
		return record, err
	}
	if err := a.Names.Release(record.Path); err != nil {
		return record, err
	}
	if err := a.Names.Claim(name, record.Path); err != nil {
		_ = a.Names.Claim(record.Name, record.Path)
		return record, err
	}
	old := record
	record.Name = name
	if err := a.Store.Put(record); err != nil {
		return record, err
	}
	return record, a.sessionOf(old).Rename(old.Name, name)
}

func (a *App) Adopt(query string) (model.Record, error) {
	resolver, err := a.Resolver()
	if err != nil {
		return model.Record{}, err
	}
	path, err := resolver.Worktree(query)
	if err != nil {
		return model.Record{}, err
	}
	for _, record := range resolver.Records {
		if record.Path == path {
			return record, nil
		}
	}
	record, err := a.adopt(path)
	if err == nil {
		a.Emit("worktree.created", record, nil)
	}
	return record, err
}

func (a *App) Setup(query string) error {
	resolver, err := a.Resolver()
	if err != nil {
		return err
	}
	record, err := resolver.Record(query)
	if err != nil {
		return err
	}
	if len(a.Config.Setup) == 0 {
		return fmt.Errorf("no setup is configured — `setup = [...]` in `mia config` names it")
	}
	return a.runSetup(record)
}

func (a *App) runSetup(record model.Record) error {
	if len(a.Config.Setup) == 0 {
		return nil
	}
	tool := run.Tool{Binary: a.Config.Setup[0], Dir: record.Path}
	return tool.Stream(os.Stderr, a.Config.Setup[1:]...)
}

type Listing struct {
	Name        string           `json:"name"`
	Path        string           `json:"path"`
	Branch      string           `json:"branch,omitempty"`
	Dirty       bool             `json:"dirty"`
	Session     bool             `json:"session"`
	Main        bool             `json:"main,omitempty"`
	Adopted     bool             `json:"adopted"`
	Where       string           `json:"where,omitempty"`
	Starred     bool             `json:"starred,omitempty"`
	Seen        time.Time        `json:"seen,omitzero"`
	Ahead       int              `json:"ahead,omitempty"`
	Behind      int              `json:"behind,omitempty"`
	Files       int              `json:"files,omitempty"`
	Added       int              `json:"added,omitempty"`
	Deleted     int              `json:"deleted,omitempty"`
	LastWork    time.Time        `json:"last_work,omitempty"`
	LastSubject string           `json:"last_subject,omitempty"`
	Windows     []session.Window `json:"windows,omitempty"`
	Layers      []stack.Layer    `json:"layers,omitempty"`
	Stack       string           `json:"stack,omitempty"`
}

type listing struct {
	byPath   map[string]model.Record
	trees    []git.Worktree
	panes    map[string][]session.Pane
	baseHead string
	stacks   sync.Map
}

func (a *App) List(names ...string) ([]Listing, error) {
	ctx, err := a.surveyRepository()
	if err != nil {
		return nil, err
	}
	defer a.finishSurvey()
	trees := ctx.trees
	if len(names) != 0 {
		trees = slices.DeleteFunc(slices.Clone(trees), func(tree git.Worktree) bool {
			name := filepath.Base(tree.Path)
			if record, adopted := ctx.byPath[tree.Path]; adopted {
				name = record.Name
			}
			return !slices.Contains(names, name)
		})
	}
	listings := make([]Listing, len(trees))
	var wg sync.WaitGroup
	lanes := make(chan struct{}, 4)
	for i, tree := range trees {
		wg.Add(1)
		go func(i int, tree git.Worktree) {
			defer wg.Done()
			lanes <- struct{}{}
			defer func() { <-lanes }()
			listings[i] = a.listOne(ctx, tree)
		}(i, tree)
	}
	wg.Wait()
	return listings, nil
}

func (a *App) surveyRepository() (*listing, error) {
	records, err := a.Store.Load()
	if err != nil {
		return nil, err
	}
	common := filepath.Dir(a.MiaDir)
	heads := git.HeadsAt(common)
	trees, err := git.WorktreesAt(a.Root, common, heads)
	if err != nil {
		trees, err = git.Worktrees(a.Root)
	}
	if err != nil {
		return nil, err
	}
	ctx := &listing{byPath: map[string]model.Record{}, trees: trees, panes: session.Here().AllPanes()}
	for _, record := range records {
		if _, err := os.Stat(record.Path); errors.Is(err, fs.ErrNotExist) && !slices.ContainsFunc(trees, func(t git.Worktree) bool { return t.Path == record.Path }) {
			_ = a.Store.Remove(record.Path)
			_ = a.Names.Release(record.Path)
			continue
		}
		ctx.byPath[record.Path] = record
	}
	if a.counts == nil {
		a.counts = stack.CountsAt(filepath.Join(a.MiaDir, "cache", "counts.json"))
	}
	a.loadFacts()
	base := a.BaseBranch(a.Root)
	a.survey, a.trees = &stack.Survey{Base: base, Heads: heads}, trees
	if ctx.baseHead = heads[base]; ctx.baseHead == "" {
		ctx.baseHead = git.RevParse(a.Root, base)
	}
	return ctx, nil
}

func (a *App) finishSurvey() {
	a.counts.Save()
	a.saveFacts()
	a.survey, a.trees = nil, nil
}

func (a *App) listOne(ctx *listing, tree git.Worktree) Listing {
	record, adopted := ctx.byPath[tree.Path]
	name := record.Name
	if !adopted {
		name = filepath.Base(tree.Path)
	}
	worked := adopted && a.sessionKnown(record, ctx.panes)
	active := false
	if worked {
		for _, pane := range ctx.panes[session.Name(record.Name)] {
			active = active || time.Since(pane.Activity) < 10*time.Minute
		}
	}
	dirty := a.dirtyOf(tree.Path, active)
	l := Listing{
		Name:    name,
		Path:    tree.Path,
		Branch:  tree.Branch,
		Dirty:   dirty,
		Session: worked,
		Main:    tree.Main,
		Adopted: adopted,
	}
	if adopted {
		l.Where = env.Placement(record).String()
		l.Starred = record.Starred
		l.Seen = record.Seen
	}
	facts := a.commitFactsFor(tree, a.BaseBranch(tree.Path), ctx.baseHead)
	l.Ahead, l.Behind = facts.Ahead, facts.Behind
	l.Files, l.Added, l.Deleted = facts.Files, facts.Added, facts.Deleted
	l.LastWork, l.LastSubject = facts.Last, facts.Subject
	if l.Session {
		if panes, ok := ctx.panes[session.Name(record.Name)]; ok {
			l.Windows = session.WindowsFrom(panes)
		} else {
			l.Windows = a.Windows(record)
		}
	}
	if stacks := a.Stacks(); tree.Branch != a.BaseBranch(tree.Path) && stacks.MemberOrParent(tree.Branch) {
		if layers := a.layersOnce(&ctx.stacks, ctx.trees, tree.Path, tree.Branch); len(layers) >= 2 {
			l.Layers = layers
			if l.Stack = stacks.NameOf(layers[0].Branch); l.Stack == "" {
				l.Stack = layers[0].Branch
			}
		}
	}
	return l
}

type Removal struct {
	Record  model.Record
	Dirty   bool
	Session bool
}

func (r Removal) Blocked(force, stopRunning bool) error {
	if r.Dirty && !force {
		return fmt.Errorf("%s has uncommitted changes — commit them, or `mia rm --force %s` to discard them", r.Record.Name, r.Record.Name)
	}
	if r.Session && !stopRunning {
		return fmt.Errorf("%s has a session running — `mia shell %s` to see what, or `mia rm --stop-running %s` to stop it", r.Record.Name, r.Record.Name, r.Record.Name)
	}
	return nil
}

func (a *App) PlanRemoval(query string) (Removal, error) {
	resolver, err := a.Resolver()
	if err != nil {
		return Removal{}, err
	}
	record, err := resolver.Record(query)
	if err != nil {
		return Removal{}, err
	}
	dirty, err := git.Dirty(record.Path)
	if err != nil {
		return Removal{}, err
	}
	return Removal{Record: record, Dirty: dirty, Session: a.sessionOf(record).Exists(record.Name)}, nil
}

func (a *App) Remove(removal Removal, force, stopRunning bool) error {
	if err := removal.Blocked(force, stopRunning); err != nil {
		return err
	}
	if err := a.sessionOf(removal.Record).Kill(removal.Record.Name); err != nil {
		return err
	}
	if err := git.Remove(a.Root, removal.Record.Path, force); err != nil {
		return err
	}
	if err := a.Store.Remove(removal.Record.Path); err != nil {
		return err
	}
	if err := a.Names.Release(removal.Record.Path); err != nil {
		return err
	}
	a.Emit("worktree.removed", removal.Record, nil)
	return nil
}

func (a *App) ensureSession(host session.Host, record model.Record, dir string) error {
	started := !host.Exists(record.Name)
	if err := host.Ensure(record.Name, dir); err != nil {
		return err
	}
	if started {
		a.Emit("session.started", record, nil)
	}
	return nil
}

func (a *App) Shell(query string, popup bool) error {
	resolver, err := a.Resolver()
	if err != nil {
		return err
	}
	record, err := resolver.Record(query)
	if err != nil {
		return err
	}
	host, err := a.SessionHostOf(record)
	if err != nil {
		return err
	}
	dir, err := a.SessionDir(record)
	if err != nil {
		return err
	}
	if !host.Available() {
		if host.SSH != "" {
			return fmt.Errorf("tmux is not installed on %s, and %s's session lives there", host.SSH, record.Name)
		}
		return fmt.Errorf("tmux not found — sessions need it: https://github.com/tmux/tmux/wiki/Installing")
	}
	if err := a.ensureSession(host, record, dir); err != nil {
		return err
	}
	if popup {
		return host.AttachPopup(record.Name, dir)
	}
	return host.Attach(record.Name)
}

func (a *App) Run(query string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("`mia run <worktree> <command...>` needs a command")
	}
	resolver, err := a.Resolver()
	if err != nil {
		return err
	}
	path, err := resolver.Worktree(query)
	if err != nil {
		return err
	}
	return run.Tool{Binary: argv[0], Dir: path}.Interactive(argv[1:]...)
}

func (a *App) RecordOrHere(query string) (model.Record, error) {
	resolver, err := a.Resolver()
	if err != nil {
		return model.Record{}, err
	}
	if query == "" {
		here, err := os.Getwd()
		if err != nil {
			return model.Record{}, err
		}
		if query, err = resolver.Here(here); err != nil {
			return model.Record{}, err
		}
	}
	record, err := resolver.Record(query)
	if err != nil {
		if path, found := resolver.Worktree(query); found == nil && git.Resolve(path) == git.Resolve(a.Root) {
			return a.adopt(path)
		}
	}
	return record, err
}

func (a *App) MarkSeen(query string) error {
	resolver, err := a.Resolver()
	if err != nil {
		return err
	}
	record, err := resolver.Record(query)
	if err != nil {
		return err
	}
	record.Seen = time.Now()
	return a.Store.Put(record)
}

func (a *App) Star(query string) (bool, error) {
	resolver, err := a.Resolver()
	if err != nil {
		return false, err
	}
	record, err := resolver.Record(query)
	if err != nil {
		return false, err
	}
	record.Starred = !record.Starred
	return record.Starred, a.Store.Put(record)
}

type stackOnce struct {
	once   sync.Once
	layers []stack.Layer
}

func (a *App) layersOnce(seen *sync.Map, trees []git.Worktree, dir, branch string) []stack.Layer {
	bottom := a.Stacks().BottomOf(branch, a.BaseBranch(dir))
	entry, _ := seen.LoadOrStore(bottom, &stackOnce{})
	one := entry.(*stackOnce)
	one.once.Do(func() { one.layers, _ = a.layersAmong(trees, dir, branch) })
	return withCurrent(one.layers, branch)
}

func withCurrent(layers []stack.Layer, branch string) []stack.Layer {
	out := make([]stack.Layer, len(layers))
	for i, layer := range layers {
		layer.Current = layer.Branch == branch
		out[i] = layer
	}
	return out
}

func (a *App) sessionKnown(record model.Record, local map[string][]session.Pane) bool {
	host := a.sessionOf(record)
	if host.SSH == "" {
		_, ok := local[session.Name(record.Name)]
		return ok
	}
	return host.Exists(record.Name)
}

func (a *App) BaseBranch(dir string) string {
	if a.Config.Base != "" {
		return a.Config.Base
	}
	if a.survey != nil {
		return a.survey.Base
	}
	return git.DefaultBranch(dir)
}
