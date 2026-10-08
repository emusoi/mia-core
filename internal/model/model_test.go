package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestALocalPlacementCannotCarryAStaging(t *testing.T) {
	local := Local()
	if !local.IsLocal() {
		t.Fatal("Local() is not local")
	}
	if staging, ok := local.Staging(); ok {
		t.Errorf("a local placement produced a staging %q — this is the bug that detached a worktree", staging)
	}

	remote, err := OnRuntime("fedora", "/Users/j/src/app.lane2")
	if err != nil {
		t.Fatal(err)
	}
	staging, ok := remote.Staging()
	if !ok || staging != "/Users/j/src/app.lane2" {
		t.Errorf("Staging() = %q, %v — a remote placement must have one", staging, ok)
	}

	if _, ok := Local().Staging(); ok {
		t.Error("moving to local left a staging behind")
	}
}

func TestARemotePlacementRequiresBoth(t *testing.T) {
	if _, err := OnRuntime("", "/some/staging"); err == nil {
		t.Error("a placement with a staging and no runtime was accepted")
	}
	if _, err := OnRuntime("fedora", ""); err == nil {
		t.Error("a placement on a box with no staging was accepted")
	} else if !strings.Contains(err.Error(), "staging") {
		t.Errorf("the refusal should say what is missing: %v", err)
	}
}

func TestReadingRefusesAPairThatCannotExist(t *testing.T) {
	var p Placement
	if err := json.Unmarshal([]byte(`{"staging":"/w/app.lane2"}`), &p); err == nil {
		t.Error("a local placement with a staging was read back without complaint")
	}

	original, err := OnRuntime("fedora", "/w/app.lane2")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var back Placement
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != original {
		t.Errorf("round trip changed the placement: %+v → %+v", original, back)
	}

	var empty Placement
	if err := json.Unmarshal([]byte(`{}`), &empty); err != nil || !empty.IsLocal() {
		t.Errorf("an empty placement should read as local: %v, %v", empty, err)
	}
}

func TestARecordWithoutAnIdentityIsRefused(t *testing.T) {
	for _, c := range []struct {
		why    string
		record Record
		expect string
	}{
		{"no path", Record{Name: "monduli"}, "identity"},
		{"relative path", Record{Path: "app.monduli", Name: "monduli"}, "absolute"},
		{"no name", Record{Path: "/w/app.monduli"}, "name"},
		{"a name that is not one word", Record{Path: "/w/app.monduli", Name: "two words"}, "one word"},
	} {
		err := c.record.Validate()
		if err == nil {
			t.Errorf("%s was accepted", c.why)
			continue
		}
		if !strings.Contains(err.Error(), c.expect) {
			t.Errorf("%s: the refusal does not say why (%q): %v", c.why, c.expect, err)
		}
	}

	if err := (Record{Path: "/w/app.monduli", Name: "monduli"}).Validate(); err != nil {
		t.Errorf("a worktree without an environment must be valid: %v", err)
	}
}

func TestARecordWrittenWithABerthStillReadsAndIsWrittenAsStaging(t *testing.T) {
	var p Placement
	if err := json.Unmarshal([]byte(`{"runtime":"box","berth":"/home/x/.mia/app/monduli"}`), &p); err != nil {
		t.Fatal(err)
	}
	if got, ok := p.Staging(); !ok || got != "/home/x/.mia/app/monduli" {
		t.Fatalf("staging = %q, %v", got, ok)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"runtime":"box","staging":"/home/x/.mia/app/monduli"}` {
		t.Errorf("written back as %s", out)
	}
}
