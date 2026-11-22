package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/emusoi/mia-core/internal/plugin"
	"github.com/emusoi/mia-core/internal/run"
)

// agy is the Antigravity CLI. It keeps everything in one folder under the
// home directory, whatever account it is signed in to.
const agyFolder = ".gemini/antigravity-cli"

// agyThere reports whether agy can be asked: it is on this machine and
// has kept something.
func agyThere() bool {
	if _, err := exec.LookPath("agy"); err != nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(home, filepath.FromSlash(agyFolder)))
	return err == nil && info.IsDir()
}

// agyModels is agy's models: those its account may use, which only it
// knows, so its own program is asked — agy models, a model a line — and
// what it said is kept, as opencode's is. It is asked only where it has
// kept something.
type agyModels struct {
	// cache is the folder what it said is kept in; nothing is kept with none.
	cache string
}

func (m agyModels) models() []plugin.Model {
	return keptModels(m.cache, "agy", func() ([]plugin.Model, bool) {
		if !agyThere() {
			return nil, false
		}
		out, _ := run.Tool{Binary: "agy", Within: AgentTimeout}.Capture("models") // one that fails has none
		return agyListed(out), true
	})
}

// agyListed is the models in what agy models prints: each line that is an
// id and its name, a tab between them, once. Anything else it prints —
// that it is fetching them, an error — is no model.
func agyListed(out string) []plugin.Model {
	models := []plugin.Model{}
	for _, line := range strings.Split(out, "\n") {
		id, name, ok := strings.Cut(strings.TrimSpace(line), "\t")
		id, name = strings.TrimSpace(id), strings.TrimSpace(name)
		if !ok || id == "" || name == "" || strings.ContainsAny(id, " \t") {
			continue
		}
		if !slices.ContainsFunc(models, func(m plugin.Model) bool { return m.ID == id }) {
			models = append(models, plugin.Model{ID: id, Label: name})
		}
	}
	return models
}
