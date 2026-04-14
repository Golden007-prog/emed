package claude

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/laborin/emed/internal/logging"
)

const (
	minPort = 10000
	maxPort = 65535
)

var upgrader = websocket.Upgrader{
	CheckOrigin:  allowLoopbackOrNoOrigin,
	Subprotocols: []string{"mcp"},
}

func allowLoopbackOrNoOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

type ToolHandler interface {
	HandleTool(method string, params json.RawMessage) (interface{}, error)
}

type Server struct {
	port        int
	token       string
	lockMgr     *LockFileManager
	handler     ToolHandler
	server      *http.Server
	conn        *websocket.Conn
	connMu      sync.Mutex
	workFolders []string
	onConnect   func()
}

func NewServer(workspaceFolders []string, handler ToolHandler) *Server {
	return &Server{
		workFolders: workspaceFolders,
		handler:     handler,
	}
}

func (s *Server) Start() error {
	port, listener, err := s.findAvailablePort()
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}
	s.port = port

	s.lockMgr = NewLockFileManager(port, s.workFolders)
	s.token = s.lockMgr.Token()

	if err := s.lockMgr.Write(); err != nil {
		listener.Close()
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	s.server = &http.Server{Handler: mux}

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("Claude server error: %v", err)
		}
	}()

	logging.Info("Claude Code server started on port %d", s.port)
	return nil
}

func (s *Server) Stop() {
	if s.connMu.TryLock() {
		if s.conn != nil {
			s.conn.Close()
		}
		s.connMu.Unlock()
	}

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}

	if s.lockMgr != nil {
		s.lockMgr.Remove()
	}
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) IsConnected() bool {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn != nil
}

func (s *Server) SetOnConnect(handler func()) {
	s.onConnect = handler
}

func (s *Server) findAvailablePort() (int, net.Listener, error) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 100; i++ {
		port := minPort + rng.Intn(maxPort-minPort)
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return port, listener, nil
		}
	}
	return 0, nil, fmt.Errorf("could not find available port after 100 attempts")
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		logging.Warn("Claude connection rejected: invalid auth token")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.Error("WebSocket upgrade failed: %v", err)
		return
	}

	s.connMu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.conn = conn
	s.connMu.Unlock()

	logging.Info("Claude Code connected")

	if s.onConnect != nil {
		s.onConnect()
	}

	s.readLoop(conn)
}

func (s *Server) readLoop(conn *websocket.Conn) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("Claude readLoop panic: %v", r)
		}
		conn.Close()
		s.connMu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.connMu.Unlock()
		logging.Info("Claude Code disconnected")
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			logging.Debug("WebSocket read error: %v", err)
			return
		}

		logging.Debug("Claude recv (%d bytes)", len(message))

		var req JSONRPCRequest
		if err := json.Unmarshal(message, &req); err != nil {
			logging.Error("Claude parse error: %v", err)
			s.sendError(conn, nil, -32700, "Parse error", nil)
			continue
		}

		logging.Debug("Claude method: %s, id: %v", req.Method, req.ID)
		go s.handleRequest(conn, &req)
	}
}

func (s *Server) handleRequest(conn *websocket.Conn, req *JSONRPCRequest) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("Claude handleRequest panic: %v", r)
			if req.ID != nil {
				s.sendError(conn, req.ID, -32603, fmt.Sprintf("Internal error: %v", r), nil)
			}
		}
	}()

	isNotification := req.ID == nil

	if s.handler == nil {
		logging.Debug("Claude: no handler for method %s", req.Method)
		if !isNotification {
			s.sendError(conn, req.ID, -32601, "Method not found", nil)
		}
		return
	}

	result, err := s.handler.HandleTool(req.Method, req.Params)
	if err != nil {
		logging.Debug("Claude tool error: %s: %v", req.Method, err)
		if !isNotification {
			s.sendError(conn, req.ID, -32603, err.Error(), nil)
		}
		return
	}

	logging.Debug("Claude tool success: %s", req.Method)
	if !isNotification {
		s.sendResult(conn, req.ID, result)
	}
}

func (s *Server) authorizeRequest(r *http.Request) bool {
	if s.token == "" {
		return false
	}

	token := extractRequestToken(r)
	if token == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func extractRequestToken(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}

	for _, name := range []string{
		"X-Claude-Code-Ide-Authorization",
		"X-Auth-Token",
		"X-Claude-Code-Token",
		"X-EMED-Token",
	} {
		if token := strings.TrimSpace(r.Header.Get(name)); token != "" {
			return token
		}
	}

	query := r.URL.Query()
	for _, name := range []string{"authToken", "auth_token", "token"} {
		if token := strings.TrimSpace(query.Get(name)); token != "" {
			return token
		}
	}

	for _, protocol := range websocket.Subprotocols(r) {
		p := strings.TrimSpace(protocol)
		if p == "" || strings.EqualFold(p, "mcp") {
			continue
		}
		if strings.HasPrefix(p, "authToken=") {
			return strings.TrimSpace(strings.TrimPrefix(p, "authToken="))
		}
		if strings.HasPrefix(p, "auth_token=") {
			return strings.TrimSpace(strings.TrimPrefix(p, "auth_token="))
		}
		if strings.HasPrefix(p, "token=") {
			return strings.TrimSpace(strings.TrimPrefix(p, "token="))
		}
		return p
	}

	return ""
}

func bearerToken(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 {
		return ""
	}
	if !strings.EqualFold(fields[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

func (s *Server) sendResult(conn *websocket.Conn, id interface{}, result interface{}) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.sendError(conn, id, -32603, "Failed to marshal result", nil)
		return
	}

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  resultJSON,
	}

	s.writeJSON(conn, resp)
}

func (s *Server) sendError(conn *websocket.Conn, id interface{}, code int, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	s.writeJSON(conn, resp)
}

func (s *Server) writeJSON(conn *websocket.Conn, v interface{}) {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if err := conn.WriteJSON(v); err != nil {
		logging.Error("WebSocket write error: %v", err)
	}
}

func (s *Server) SendNotification(method string, params interface{}) {
	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()

	if conn == nil {
		return
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return
	}

	notification := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}

	s.connMu.Lock()
	defer s.connMu.Unlock()

	if err := conn.WriteJSON(notification); err != nil {
		logging.Debug("WebSocket notification write error: %v", err)
	}
}
