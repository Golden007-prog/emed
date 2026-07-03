package claude

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type stubHandler struct{}

func (stubHandler) HandleTool(sessionID int, method string, params json.RawMessage) (interface{}, error) {
	return map[string]string{"method": method}, nil
}

func startTestServer(t *testing.T) *Server {
	t.Helper()
	overrideHomeDir(t)

	s := NewServer([]string{"/tmp/work"}, stubHandler{})
	if err := s.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

func dialTestClient(t *testing.T, s *Server) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.token)

	url := fmt.Sprintf("ws://127.0.0.1:%d/", s.Port())
	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("failed to dial server: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func roundTrip(t *testing.T, conn *websocket.Conn, id int) {
	t.Helper()
	req := JSONRPCRequest{JSONRPC: "2.0", ID: id, Method: "test_method"}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp JSONRPCResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("no response received: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}
}

func TestServerSupportsMultipleSessions(t *testing.T) {
	s := startTestServer(t)

	first := dialTestClient(t, s)
	second := dialTestClient(t, s)

	roundTrip(t, first, 1)
	roundTrip(t, second, 2)

	if !s.IsConnected() {
		t.Fatalf("server should report connected sessions")
	}

	s.SendNotification("test_notification", map[string]string{"hello": "world"})

	for i, conn := range []*websocket.Conn{first, second} {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var notif JSONRPCRequest
		if err := conn.ReadJSON(&notif); err != nil {
			t.Fatalf("session %d did not receive broadcast: %v", i+1, err)
		}
		if notif.Method != "test_notification" {
			t.Fatalf("session %d got unexpected method: %q", i+1, notif.Method)
		}
	}
}

func TestServerRejectsMissingToken(t *testing.T) {
	s := startTestServer(t)

	url := fmt.Sprintf("ws://127.0.0.1:%d/", s.Port())
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err == nil {
		conn.Close()
		t.Fatalf("expected dial without token to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 response, got %+v", resp)
	}
}
