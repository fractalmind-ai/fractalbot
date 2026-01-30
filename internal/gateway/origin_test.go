package gateway

import (
	"net/http/httptest"
	"testing"
)

func TestOriginCheckerAllowsAllWhenUnset(t *testing.T) {
	checker := buildOriginChecker(nil)

	req := httptest.NewRequest("GET", "http://example/ws", nil)
	if !checker(req) {
		t.Fatalf("expected allow when allowlist unset")
	}

	req.Header.Set("Origin", "https://example.com")
	if !checker(req) {
		t.Fatalf("expected allow for any origin when allowlist unset")
	}
}

func TestOriginCheckerAllowsConfiguredOrigins(t *testing.T) {
	checker := buildOriginChecker([]string{"https://example.com", "http://localhost:3000"})

	req := httptest.NewRequest("GET", "http://example/ws", nil)
	req.Header.Set("Origin", "https://example.com")
	if !checker(req) {
		t.Fatalf("expected allow for listed origin")
	}

	req.Header.Set("Origin", "https://example.com/")
	if !checker(req) {
		t.Fatalf("expected allow for normalized origin")
	}

	req.Header.Set("Origin", "https://evil.com")
	if checker(req) {
		t.Fatalf("expected reject for unlisted origin")
	}

	req.Header.Del("Origin")
	if checker(req) {
		t.Fatalf("expected reject when origin missing and allowlist configured")
	}
}

func TestOriginCheckerRejectsInvalidAllowlist(t *testing.T) {
	checker := buildOriginChecker([]string{"not a url"})

	req := httptest.NewRequest("GET", "http://example/ws", nil)
	req.Header.Set("Origin", "https://example.com")
	if checker(req) {
		t.Fatalf("expected reject when allowlist invalid")
	}
}
