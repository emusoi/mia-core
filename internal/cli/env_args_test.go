package cli

import (
	"reflect"
	"testing"
)

func TestEnvWordsAreRecognisedNotPositional(t *testing.T) {
	runtimes := map[string]bool{"fedora": true}
	services := map[string]bool{"web": true}
	worktrees := map[string]bool{"monduli": true, "iringa": true}
	is := func(m map[string]bool) func(string) bool { return func(w string) bool { return m[w] } }

	for _, c := range []struct {
		what   string
		sub    string
		words  []string
		target string
		left   []string
	}{
		{"nothing, anywhere", "up", nil, "", nil},
		{"a worktree", "up", []string{"monduli"}, "monduli", []string{}},

		{"host: the machine alone", "host", []string{"fedora"}, "", []string{"fedora"}},
		{"host: worktree then machine", "host", []string{"monduli", "fedora"}, "monduli", []string{"fedora"}},
		{"host: machine then worktree", "host", []string{"fedora", "monduli"}, "monduli", []string{"fedora"}},
		{"host: a worktree alone reports placement", "host", []string{"monduli"}, "monduli", []string{}},

		{"service: action and id, here", "service", []string{"start", "web"}, "", []string{"start", "web"}},
		{"service: action, worktree, id", "service", []string{"start", "monduli", "web"}, "monduli", []string{"start", "web"}},
		{"service: action and a word that is not a service", "service", []string{"logs", "monduli"}, "monduli", []string{"logs"}},
		{"service: no action is a status query", "service", []string{"monduli"}, "monduli", []string{}},

		{"exec: a command is not a worktree", "exec", []string{"cat", "file"}, "", []string{"cat", "file"}},
		{"exec: worktree then command", "exec", []string{"iringa", "cat", "file"}, "iringa", []string{"cat", "file"}},
	} {
		target, left := splitEnvWords(c.sub, c.words, is(runtimes), is(services), is(worktrees))
		if target != c.target || !reflect.DeepEqual(left, c.left) {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", c.what, target, left, c.target, c.left)
		}
	}
}
