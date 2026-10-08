package gateway

import (
	"net/http"
	"testing"
)

func TestOriginBecomesLocalhostLikeTheHostDoes(t *testing.T) {
	for _, one := range []struct {
		name, was, want string
	}{
		{"Origin", "https://engaruka.mia:3000", "http://localhost:3000"},
		{"Origin", "https://engaruka.mia", "http://localhost"},
		{"Referer", "https://engaruka.mia:3000/sheet?id=7", "http://localhost:3000/sheet?id=7"},
		{"Origin", "https://studio.apollographql.com", "https://studio.apollographql.com"},
		{"Origin", "http://localhost:3000", "http://localhost:3000"},
		{"Origin", "null", "null"},
	} {
		header := http.Header{}
		header.Set(one.name, one.was)
		asLocalhost(header, one.name)
		if got := header.Get(one.name); got != one.want {
			t.Errorf("%s %q became %q, want %q", one.name, one.was, got, one.want)
		}
	}
}

func TestAMissingOriginStaysMissing(t *testing.T) {
	header := http.Header{}
	asLocalhost(header, "Origin")
	if _, found := header["Origin"]; found {
		t.Error("a request with no Origin must not gain one")
	}
}

func TestTheAllowedOriginComesBackAsTheBrowserAskedIt(t *testing.T) {
	for _, one := range []struct {
		answered, asked, want string
	}{
		{"http://localhost:3000", "https://engaruka.mia:3000", "https://engaruka.mia:3000"},
		{"*", "https://engaruka.mia:3000", "*"},
		{"https://studio.apollographql.com", "https://engaruka.mia:3000", "https://studio.apollographql.com"},
		{"http://localhost:3000", "", "http://localhost:3000"},
	} {
		header := http.Header{}
		header.Set("Access-Control-Allow-Origin", one.answered)
		asAsked(header, "Access-Control-Allow-Origin", one.asked)
		if got := header.Get("Access-Control-Allow-Origin"); got != one.want {
			t.Errorf("answered %q to a page at %q, sent back %q, want %q", one.answered, one.asked, got, one.want)
		}
	}
}
