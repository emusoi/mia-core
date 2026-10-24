package mirror

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

type Preview struct {
	IgnoreRules []string `json:"ignoreRules"`
	Excluded    []string `json:"excluded,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
}

func BuildPreview(root string, patterns []string) (Preview, error) {
	preview := Preview{IgnoreRules: patterns}
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		secret := !entry.IsDir() && sensitive(path.Base(rel))
		if ignored(rel, patterns) {
			if secret {
				preview.Excluded = append(preview.Excluded, rel)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if secret {
			preview.Secrets = append(preview.Secrets, rel)
		}
		return nil
	})
	return preview, err
}

func sensitive(base string) bool {
	for _, pattern := range SecretIgnores {
		if matches(pattern, base) {
			return true
		}
	}
	return false
}

func ignored(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if matches(pattern, rel) {
			return true
		}
	}
	return false
}

func matches(pattern, rel string) bool {
	pattern = path.Clean(strings.TrimPrefix(pattern, "/"))
	if pattern == "." {
		return false
	}
	if !strings.ContainsAny(pattern, "*?[") {
		if !strings.Contains(pattern, "/") {
			for _, part := range strings.Split(rel, "/") {
				if part == pattern {
					return true
				}
			}
			return false
		}
		return rel == pattern || strings.HasPrefix(rel, pattern+"/")
	}
	if ok, _ := path.Match(pattern, rel); ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, path.Base(rel))
		return ok
	}
	return false
}
