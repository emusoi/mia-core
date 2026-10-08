package env

import (
	"slices"
	"strings"
	"testing"
)

func TestHostPortsBecomeForwarders(t *testing.T) {
	m := Manager{Settings: Settings{HostPorts: []int{6379, 5432}}}
	forwarders := m.services()
	if len(forwarders) != 2 {
		t.Fatalf("services() = %d, want 2", len(forwarders))
	}
	first := forwarders[0]
	if first.ID != "host-6379" || !first.Autostart {
		t.Fatalf("forwarder = %+v", first)
	}
	line := strings.Join(first.Run, " ")
	if !strings.Contains(line, "bind=127.0.0.1") || !strings.Contains(line, hostGateway+":6379") {
		t.Fatalf("forwarder command must bridge container localhost to the host: %q", line)
	}
	if _, ok := m.serviceByID("host-5432"); !ok {
		t.Fatal("a forwarder must be addressable by id like any other service")
	}
}

func TestForwardedPortsAreNotAdvertised(t *testing.T) {
	s := Settings{HostPorts: []int{6379}}
	got := s.withoutForwarded([]int{3000, 6379})
	if slices.Contains(got, 6379) {
		t.Fatalf("a forwarded host port is not the environment's own: %v", got)
	}
	if !slices.Contains(got, 3000) {
		t.Fatalf("the environment's own ports must survive: %v", got)
	}
}
