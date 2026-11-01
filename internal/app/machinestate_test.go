package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/emusoi/mia-core/internal/runtime"
)

// Clients read `mia api machines`: its keys are lower case, like the rest of the API.
func TestMachineStateJSON(t *testing.T) {
	data, err := json.Marshal(MachineState{Machine: runtime.Machine{Name: "fedora"}, Answers: true, Health: &Health{Cores: 32, MemTotal: 64}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"name":"fedora"`, `"answers":true`, `"cores":32`, `"mem_total":64`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("%s lacks %s", data, key)
		}
	}
}

// `mia api here` lists no machines as [], never null, so clients can range over it.
func TestHereStateJSON(t *testing.T) {
	data, err := json.Marshal(HereState{Machines: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"machines":[]}` {
		t.Errorf("got %s", data)
	}
}
