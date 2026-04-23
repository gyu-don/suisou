package policy

import (
	"net/http"
	"testing"
)

func TestEndpointAllowedMatchesQueryString(t *testing.T) {
	endpoint, err := NewEndpoint(EndpointSpec{
		Domain:  "github.com",
		Methods: []string{"GET"},
		Paths:   []string{"/info/refs?service=git-upload-pack"},
	})
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}

	p := &Policy{Endpoints: []Endpoint{endpoint}}
	if !p.EndpointAllowed("github.com", "GET", "/info/refs?service=git-upload-pack", 443) {
		t.Fatal("expected query string path to match")
	}
	if p.EndpointAllowed("github.com", "GET", "/info/refs", 443) {
		t.Fatal("expected plain path without query not to match")
	}
}

func TestInjectCredentialsReplacesAllMarkers(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "real-key")
	matcher, err := NewMatcher("api.openai.com")
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("authorization", "Bearer SUISOU__OPENAI_API_KEY:SUISOU__OPENAI_API_KEY")

	p := &Policy{
		Credentials: []CredentialRule{{
			Domain:  "api.openai.com",
			Matcher: matcher,
			Header:  "authorization",
			Env:     "OPENAI_API_KEY",
		}},
	}
	p.InjectCredentials(req, "api.openai.com")

	want := "Bearer real-key:real-key"
	if got := req.Header.Get("authorization"); got != want {
		t.Fatalf("authorization header = %q, want %q", got, want)
	}
}

func TestReplaceMarkersOnlyAllowedEnvs(t *testing.T) {
	t.Setenv("FIRST", "one")
	t.Setenv("SECOND", "two")

	p := &Policy{}
	got := p.ReplaceMarkers(`{"a":"SUISOU__FIRST","b":"SUISOU__SECOND"}`, map[string]struct{}{
		"FIRST": {},
	})

	want := `{"a":"one","b":"SUISOU__SECOND"}`
	if got != want {
		t.Fatalf("ReplaceMarkers() = %q, want %q", got, want)
	}
}

func TestMatcherSupportsFnmatchStyleWildcards(t *testing.T) {
	matcher, err := NewMatcher("*.githubusercontent.com")
	if err != nil {
		t.Fatalf("NewMatcher: %v", err)
	}
	if !matcher.Match("raw.githubusercontent.com") {
		t.Fatal("expected wildcard domain to match")
	}
}
