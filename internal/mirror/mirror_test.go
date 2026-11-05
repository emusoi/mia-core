package mirror

import (
	"slices"
	"testing"
)

func TestCreateArgsAddsTheProjectsOwnIgnoresOnce(t *testing.T) {
	args := createArgs("mia-x", "/local", "box:/remote", []string{"packages/mobile/ios/Pods", " node_modules ", "", "packages/mobile/ios/Pods"})
	if !slices.Contains(args, "--ignore=packages/mobile/ios/Pods") {
		t.Fatalf("the project's ignore is missing: %v", args)
	}
	if n := countOf(args, "--ignore=node_modules"); n != 1 {
		t.Fatalf("node_modules ignored %d times, want once", n)
	}
	if n := countOf(args, "--ignore=packages/mobile/ios/Pods"); n != 1 {
		t.Fatalf("the project's ignore repeated %d times", n)
	}
	if args[len(args)-2] != "/local" || args[len(args)-1] != "box:/remote" {
		t.Fatalf("the ends must come last: %v", args[len(args)-2:])
	}
	if !slices.Contains(args, "--ignore=.git") {
		t.Fatalf("the built-in ignores are missing: %v", args)
	}
}

func countOf(all []string, one string) int {
	n := 0
	for _, each := range all {
		if each == one {
			n++
		}
	}
	return n
}
