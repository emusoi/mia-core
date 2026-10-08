package env

import "testing"

func TestATokenInYourShellReachesTheEnvironment(t *testing.T) {
	t.Setenv("MIA_EMPTY", "")
	t.Setenv("MY_OWN_SECRET", "hunter2")

	carry := Settings{Pass: []string{"MY_OWN_SECRET", "NEVER_SET"}}.Passing()
	if carry["MY_OWN_SECRET"] != "hunter2" {
		t.Errorf("a named variable did not travel: %v", carry)
	}
	if _, there := carry["NEVER_SET"]; there {
		t.Error("a variable that is not in the shell must not be passed")
	}
	if _, there := carry["MIA_EMPTY"]; there {
		t.Error("an empty variable must not be passed")
	}
}
