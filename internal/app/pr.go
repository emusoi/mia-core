package app

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/git"
	"github.com/emusoi/mia-core/internal/model"
	"github.com/emusoi/mia-core/internal/run"
)

type PRDraft struct {
	Branch           string `json:"branch"`
	Base             string `json:"base"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	Path             string `json:"path"`
	Saved            bool   `json:"saved"`
	RemoveAfterMerge bool   `json:"removeAfterMerge"`
	PushCommand      string `json:"pushCommand"`
	CreateCommand    string `json:"createCommand"`
	RemoveCommand    string `json:"removeCommand"`
}

type PRDraftOptions struct {
	Branch           *string
	Title            *string
	Body             *string
	RemoveAfterMerge *bool
}

type prMetadata struct {
	Title            string `json:"title"`
	RemoveAfterMerge bool   `json:"removeAfterMerge"`
	BodySHA          string `json:"bodySHA,omitempty"`
}

func (a *App) PRPath(branch string) (string, error) {
	if branch == "" || filepath.IsAbs(branch) || strings.ContainsAny(branch, "\\\x00") {
		return "", fmt.Errorf("a PR draft needs a branch name")
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid PR draft branch %q", branch)
		}
	}
	return filepath.Join(a.MiaDir, "pr", branch+".md"), nil
}

func (a *App) PRDraft(record model.Record) (PRDraft, error) {
	draft, err := a.prPreview(record)
	if err != nil {
		return PRDraft{}, err
	}
	body, err := os.ReadFile(draft.Path)
	if errors.Is(err, os.ErrNotExist) {
		return draft, nil
	}
	if err != nil {
		return PRDraft{}, err
	}
	metadata, err := readPRMetadata(draft.Path)
	if err != nil {
		return PRDraft{}, err
	}
	if metadata.Title == "" {
		return PRDraft{}, fmt.Errorf("%s has no saved PR draft metadata — restore %s or move the incomplete body aside, then run `mia pr %s draft`", draft.Path, strings.TrimSuffix(draft.Path, ".md")+".json", record.Name)
	}
	if metadata.BodySHA != "" && metadata.BodySHA != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return PRDraft{}, fmt.Errorf("%s differs from its saved PR draft metadata", draft.Path)
	}
	draft.Body, draft.Title, draft.Saved = string(body), metadata.Title, true
	draft.RemoveAfterMerge = metadata.RemoveAfterMerge
	draft.commands(a.Root, record)
	return draft, nil
}

func (a *App) DraftPR(record model.Record, options PRDraftOptions) (PRDraft, error) {
	draft, err := a.PRDraft(record)
	if err != nil {
		return PRDraft{}, err
	}
	if options.Branch != nil && draft.Branch != *options.Branch {
		return PRDraft{}, fmt.Errorf("%s now holds %s, not %s — reload the PR draft before saving", record.Name, draft.Branch, *options.Branch)
	}
	if options.Title != nil {
		draft.Title = strings.TrimSpace(*options.Title)
	}
	if options.Body != nil {
		draft.Body = *options.Body
	}
	if options.RemoveAfterMerge != nil {
		draft.RemoveAfterMerge = *options.RemoveAfterMerge
	}
	if draft.Title == "" || strings.ContainsAny(draft.Title, "\r\n\x00") {
		return PRDraft{}, fmt.Errorf("a PR title must be one non-empty line")
	}
	data, err := json.Marshal(prMetadata{Title: draft.Title, RemoveAfterMerge: draft.RemoveAfterMerge, BodySHA: fmt.Sprintf("%x", sha256.Sum256([]byte(draft.Body)))})
	if err != nil {
		return PRDraft{}, err
	}
	if err := os.MkdirAll(filepath.Dir(draft.Path), 0o755); err != nil {
		return PRDraft{}, err
	}
	type draftFile struct {
		path string
		data []byte
		temp string
	}
	files := []draftFile{{path: draft.Path, data: []byte(draft.Body)}, {path: strings.TrimSuffix(draft.Path, ".md") + ".json", data: data}}
	for i := range files {
		file, err := os.OpenFile(files[i].path, os.O_WRONLY, 0)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return PRDraft{}, err
		}
		if file != nil {
			if err := file.Close(); err != nil {
				return PRDraft{}, err
			}
		}
		temp, err := os.CreateTemp(filepath.Dir(files[i].path), ".pr-*")
		if err != nil {
			return PRDraft{}, err
		}
		files[i].temp = temp.Name()
		defer os.Remove(temp.Name())
		if err := temp.Chmod(0o644); err != nil {
			temp.Close()
			return PRDraft{}, err
		}
		if _, err := temp.Write(files[i].data); err != nil {
			temp.Close()
			return PRDraft{}, err
		}
		if err := temp.Close(); err != nil {
			return PRDraft{}, err
		}
	}
	for _, file := range files {
		if err := os.Rename(file.temp, file.path); err != nil {
			return PRDraft{}, err
		}
	}
	draft.Saved = true
	draft.commands(a.Root, record)
	return draft, nil
}

func readPRMetadata(path string) (prMetadata, error) {
	data, err := os.ReadFile(strings.TrimSuffix(path, ".md") + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return prMetadata{}, nil
	}
	if err != nil {
		return prMetadata{}, err
	}
	var metadata prMetadata
	err = json.Unmarshal(data, &metadata)
	return metadata, err
}

func (a *App) prPreview(record model.Record) (PRDraft, error) {
	branch := git.CurrentBranch(record.Path)
	path, err := a.PRPath(branch)
	if err != nil {
		return PRDraft{}, err
	}
	base := a.BaseBranch(record.Path)
	layers, err := a.Stacks().Of(record.Path, branch, base)
	if err != nil {
		return PRDraft{}, err
	}
	for _, layer := range layers {
		if layer.Branch == branch {
			base = layer.Parent
		}
	}
	if base == "" {
		return PRDraft{}, fmt.Errorf("no base branch — set base in `mia config`")
	}
	subjects := git.SubjectsSince(record.Path, base+".."+branch)
	title := branch
	if len(subjects) > 0 {
		title = subjects[0]
	}
	var body strings.Builder
	body.WriteString("## Changes\n\n")
	for _, subject := range subjects {
		fmt.Fprintf(&body, "- %s\n", subject)
	}
	if len(subjects) == 0 {
		fmt.Fprintf(&body, "No commits ahead of %s.\n", base)
	}
	draft := PRDraft{Branch: branch, Base: strings.TrimPrefix(base, "origin/"), Title: title, Body: body.String(), Path: path}
	draft.commands(a.Root, record)
	return draft, nil
}

func (d *PRDraft) commands(root string, record model.Record) {
	d.PushCommand = "git -C " + run.ShellJoin([]string{record.Path}) + " push -u origin " + run.ShellJoin([]string{d.Branch})
	d.CreateCommand = "cd " + run.ShellJoin([]string{record.Path}) + " && gh pr create --base " + run.ShellJoin([]string{d.Base}) + " --head " + run.ShellJoin([]string{d.Branch}) + " --title " + run.ShellJoin([]string{d.Title}) + " --body-file " + run.ShellJoin([]string{d.Path})
	if record.Path != root && d.Branch != d.Base {
		d.RemoveCommand = "cd " + run.ShellJoin([]string{root}) + " && mia rm " + run.ShellJoin([]string{record.Name})
	}
}
