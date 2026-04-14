package claude

import (
	"net/http"
	"testing"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect string
	}{
		{name: "valid bearer", value: "Bearer abc123", expect: "abc123"},
		{name: "valid bearer mixed case", value: "bEaReR xyz", expect: "xyz"},
		{name: "missing token", value: "Bearer", expect: ""},
		{name: "wrong scheme", value: "Basic abc", expect: ""},
		{name: "empty", value: "", expect: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bearerToken(tc.value); got != tc.expect {
				t.Fatalf("unexpected token: got %q want %q", got, tc.expect)
			}
		})
	}
}

func TestExtractRequestToken(t *testing.T) {
	reqAuth, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqAuth.Header.Set("Authorization", "Bearer auth-token")

	reqHeader, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqHeader.Header.Set("X-Auth-Token", "header-token")

	reqClaudeHeader, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqClaudeHeader.Header.Set("X-Claude-Code-Ide-Authorization", "claude-header-token")

	reqQuery, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/?authToken=query-token", nil)

	reqProtocol, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqProtocol.Header.Set("Sec-WebSocket-Protocol", "mcp, token=proto-token")

	reqProtocolRaw, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqProtocolRaw.Header.Set("Sec-WebSocket-Protocol", "mcp, raw-token")

	reqNone, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)

	tests := []struct {
		name   string
		req    *http.Request
		expect string
	}{
		{name: "authorization header", req: reqAuth, expect: "auth-token"},
		{name: "custom header", req: reqHeader, expect: "header-token"},
		{name: "claude header", req: reqClaudeHeader, expect: "claude-header-token"},
		{name: "query token", req: reqQuery, expect: "query-token"},
		{name: "protocol token key", req: reqProtocol, expect: "proto-token"},
		{name: "protocol raw token", req: reqProtocolRaw, expect: "raw-token"},
		{name: "no token", req: reqNone, expect: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractRequestToken(tc.req); got != tc.expect {
				t.Fatalf("unexpected token: got %q want %q", got, tc.expect)
			}
		})
	}
}

func TestAuthorizeRequest(t *testing.T) {
	s := &Server{token: "expected-token"}

	reqOK, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqOK.Header.Set("Authorization", "Bearer expected-token")

	reqBad, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)
	reqBad.Header.Set("Authorization", "Bearer wrong-token")

	reqMissing, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1234/", nil)

	if !s.authorizeRequest(reqOK) {
		t.Fatalf("expected valid token to authorize request")
	}
	if s.authorizeRequest(reqBad) {
		t.Fatalf("expected invalid token to be rejected")
	}
	if s.authorizeRequest(reqMissing) {
		t.Fatalf("expected missing token to be rejected")
	}
}
