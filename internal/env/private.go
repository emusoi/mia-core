package env

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
)

func (m Manager) privateVolumes(record model.Record, where Location, container string) map[string]string {
	volumes := map[string]string{}
	for _, pattern := range m.Settings.ContainerOnly {
		for _, rel := range relativeMatches(record.Path, pattern) {
			volumes[volumeFor(container, rel)] = path.Join(where.Dir, rel)
		}
	}
	return volumes
}

func relativeMatches(root, pattern string) []string {
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{filepath.ToSlash(pattern)}
	}
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		return nil
	}
	found := make([]string, 0, len(matches))
	for _, match := range matches {
		rel, err := filepath.Rel(root, match)
		if err != nil {
			continue
		}
		found = append(found, filepath.ToSlash(rel))
	}
	return found
}

var notInAVolumeName = strings.NewReplacer("/", "-", ".", "-", "_", "-")

func volumeFor(container, rel string) string {
	return container + "-" + notInAVolumeName.Replace(rel)
}

func (m Manager) cacheVolumes() (map[string]string, []string) {
	volumes := map[string]string{}
	var notAbsolute []string
	repo := Sanitise(filepath.Base(m.Repo))
	for _, dir := range m.Settings.Cache {
		at := path.Clean(dir)
		if !path.IsAbs(at) {
			notAbsolute = append(notAbsolute, dir)
			continue
		}
		volumes["mia-cache-"+repo+"-"+Sanitise(strings.TrimPrefix(at, "/"))] = at
	}
	return volumes, notAbsolute
}
