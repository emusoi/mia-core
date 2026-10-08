package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/emusoi/mia-core/internal/model"
)

func TestContainerOnlyPathsBecomeVolumes(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"packages/server/node_modules", "packages/web-client/node_modules", "packages/server/src"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	m := Manager{Settings: Settings{ContainerOnly: []string{"node_modules", "packages/*/node_modules"}}}
	got := m.privateVolumes(model.Record{Path: root}, Location{Dir: "/work/shop.engaruka"}, "mia-engaruka")

	want := map[string]string{
		"mia-engaruka-node-modules":                     "/work/shop.engaruka/node_modules",
		"mia-engaruka-packages-server-node-modules":     "/work/shop.engaruka/packages/server/node_modules",
		"mia-engaruka-packages-web-client-node-modules": "/work/shop.engaruka/packages/web-client/node_modules",
	}
	if len(got) != len(want) {
		t.Fatalf("volumes = %v, want %v", got, want)
	}
	for volume, at := range want {
		if got[volume] != at {
			t.Errorf("%s mounted at %q, want %q", volume, got[volume], at)
		}
	}
}

func TestNoContainerOnlyPathsMeansNoVolumes(t *testing.T) {
	m := Manager{}
	if got := m.privateVolumes(model.Record{Path: t.TempDir()}, Location{Dir: "/work"}, "mia-x"); len(got) != 0 {
		t.Fatalf("volumes = %v, want none", got)
	}
}

func TestCacheVolumesAreNamedForTheRepositoryNotTheWorktree(t *testing.T) {
	m := Manager{Repo: "/home/me/src/shop", Settings: Settings{Cache: []string{"/root/.cache/yarn", "~/.npm"}}}

	got, notAbsolute := m.cacheVolumes()

	want := map[string]string{"mia-cache-shop-root--cache-yarn": "/root/.cache/yarn"}
	if len(got) != len(want) {
		t.Fatalf("volumes = %v, want %v", got, want)
	}
	for volume, at := range want {
		if got[volume] != at {
			t.Errorf("%s mounted at %q, want %q", volume, got[volume], at)
		}
	}
	if len(notAbsolute) != 1 || notAbsolute[0] != "~/.npm" {
		t.Fatalf("a path the engine cannot expand must be reported, not mounted: %v", notAbsolute)
	}
}

func TestACacheOutlivesTheWorktreeThatFilledIt(t *testing.T) {
	m := Manager{Repo: "/home/me/src/shop", Settings: Settings{
		ContainerOnly: []string{"node_modules"},
		Cache:         []string{"/root/.cache/yarn"},
	}}
	removed := m.privateVolumes(model.Record{Path: t.TempDir()}, Location{Dir: "/work/shop.engaruka"}, "mia-engaruka")

	caches, _ := m.cacheVolumes()
	for volume := range caches {
		if _, gone := removed[volume]; gone {
			t.Fatalf("`mia env remove` would delete the shared cache %s", volume)
		}
	}
}
