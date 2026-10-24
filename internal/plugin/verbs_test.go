package plugin

import (
	"strings"
	"testing"
)

func TestManifestRejectsAnEmptyOrRepeatedVerb(t *testing.T) {
	h, dir := host(t)
	write(t, dir, "repeat", `echo '{"protocol":1,"verbs":[{"name":"hi"},{"name":"hi"}]}'`)
	write(t, dir, "empty", `echo '{"protocol":1,"verbs":[{"name":""}]}'`)
	h.Configured = []string{"repeat", "empty"}
	all, _ := h.List()
	for _, p := range all {
		if p.Problem == "" || (!strings.Contains(p.Problem, "more than once") && !strings.Contains(p.Problem, "non-empty")) {
			t.Errorf("%s: %q", p.Name, p.Problem)
		}
	}
}

func TestVerbCollisionsNameEveryOwner(t *testing.T) {
	plugins := []Plugin{
		{Name: "zeta", Manifest: Manifest{Verbs: []Verb{{Name: "hello"}}}},
		{Name: "alpha", Manifest: Manifest{Verbs: []Verb{{Name: "hello"}}}},
		{Name: "other", Manifest: Manifest{Verbs: []Verb{{Name: "other"}}}},
	}
	collisions := VerbCollisions(plugins)
	if len(collisions) != 1 || collisions[0].Verb != "hello" || len(collisions[0].Plugins) != 2 || collisions[0].Plugins[0] != "alpha" || collisions[0].Plugins[1] != "zeta" {
		t.Fatalf("collisions = %+v", collisions)
	}
}
