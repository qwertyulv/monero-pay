package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewSessionAPIKey(t *testing.T) {
	a, err := NewSessionAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSessionAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("session API keys must differ across starts/generations")
	}
	if !strings.HasPrefix(a, "mp_") || !strings.HasPrefix(b, "mp_") {
		t.Fatal("unexpected key prefix")
	}
}

func TestExtractAPIKey(t *testing.T) {
	// Verify API key extraction from supported HTTP authentication headers.
	for name, setup := range map[string]func(*http.Request){
		"bearer":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer secret") },
		"x-api-key": func(r *http.Request) { r.Header.Set("X-API-Key", "secret") },
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/v1/invoices", nil)
			setup(r)
			if got := extractAPIKey(r); got != "secret" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
