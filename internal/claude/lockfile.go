package claude

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	lockDir = ".claude/ide"
	ideName = "emed"
)

type LockFileManager struct {
	port     int
	token    string
	lockPath string
	folders  []string
}

func NewLockFileManager(port int, workspaceFolders []string) *LockFileManager {
	return &LockFileManager{
		port:    port,
		token:   generateToken(),
		folders: workspaceFolders,
	}
}

func (m *LockFileManager) Token() string {
	return m.token
}

func (m *LockFileManager) Write() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	dir := filepath.Join(homeDir, lockDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	m.lockPath = filepath.Join(dir, fmt.Sprintf("%d.lock", m.port))

	lockFile := LockFile{
		PID:              os.Getpid(),
		WorkspaceFolders: m.folders,
		IDEName:          ideName,
		Transport:        "ws",
		Token:            m.token,
	}

	data, err := json.MarshalIndent(lockFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock file: %w", err)
	}

	if err := os.WriteFile(m.lockPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	return nil
}

func (m *LockFileManager) Remove() error {
	if m.lockPath == "" {
		return nil
	}
	return os.Remove(m.lockPath)
}

func generateToken() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
