package env

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emusoi/mia-core/internal/model"
)

func BuiltTag(repo, base string, setup []string) string {
	sum := sha256.Sum256([]byte(base + "\x00" + strings.Join(setup, "\x00")))
	return fmt.Sprintf("mia-%s:%s", Sanitise(filepath.Base(repo)), hex.EncodeToString(sum[:])[:12])
}

func Containerfile(base string, setup []string) string {
	file := "FROM " + base + "\n"
	if len(setup) > 0 {
		file += "RUN " + shellJoin(setup) + "\n"
	}
	return file
}

func (m Manager) BuildImage(record model.Record, out *os.File) (string, error) {
	if len(m.Settings.ImageSetup) == 0 {
		return "", fmt.Errorf("nothing to build — `[env] image_setup` is what describes the machine")
	}
	where, err := m.Where(record)
	if err != nil {
		return "", err
	}
	if err := where.Engine.Ready(); err != nil {
		return "", err
	}

	base := DiscoverImage(record.Path, m.Settings.Image)
	tag := BuiltTag(m.Repo, base.Ref, m.Settings.ImageSetup)
	if where.Engine.HasImage(tag) {
		fmt.Fprintf(out, "%s is already built on %s\n", tag, machineName(where))
		return tag, nil
	}
	if !where.Engine.HasImage(base.Ref) {
		if err := where.Engine.Pull(base.Ref); err != nil {
			return "", err
		}
	}
	fmt.Fprintf(out, "building %s on %s from %s\n", tag, machineName(where), base.Ref)
	if err := where.Engine.Build(tag, Containerfile(base.Ref, m.Settings.ImageSetup)); err != nil {
		return "", err
	}
	return tag, nil
}

func (m Manager) imageFor(where Location, record model.Record) (Image, bool) {
	base := DiscoverImage(record.Path, m.Settings.Image)
	if len(m.Settings.ImageSetup) == 0 {
		return base, false
	}
	tag := BuiltTag(m.Repo, base.Ref, m.Settings.ImageSetup)
	if where.Engine.HasImage(tag) {
		return Image{Ref: tag, Source: FromBuild}, true
	}
	return base, false
}

func machineName(where Location) string {
	if where.Remote() {
		return where.Machine.Name
	}
	return "this computer"
}
