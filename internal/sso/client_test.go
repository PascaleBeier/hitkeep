package sso

import "testing"

func TestNormalizeIssuerURL(t *testing.T) {
	got, err := NormalizeIssuerURL(" https://ID.Example.com/realms/acme/ ")
	if err != nil {
		t.Fatalf("normalize issuer: %v", err)
	}
	if got != "https://id.example.com/realms/acme" {
		t.Fatalf("unexpected issuer %q", got)
	}
}

func TestNormalizeIssuerURLRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"http://id.example.com",
		"https://user:secret@id.example.com",
		"https://id.example.com?tenant=acme",
		"//id.example.com",
	} {
		if _, err := NormalizeIssuerURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
