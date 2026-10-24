package gateway

import "testing"

func TestOnlyGatewayNamesGoToTheStatusPages(t *testing.T) {
	for _, host := range []string{"127.0.0.1:7842", "localhost:7842"} {
		if !isSelf(host) {
			t.Errorf("%s bypassed the gateway's own pages", host)
		}
	}
	for _, host := range []string{"monduli.mia:5173", "monduli.localhost:7842"} {
		if isSelf(host) {
			t.Errorf("%s went to the gateway's status page instead of its environment", host)
		}
	}
}
