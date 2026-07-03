package claude

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/laborin/emed/internal/logging"
)

const (
	lockDir = ".claude/ide"
	ideName = "emed"
)

// userHomeDir is a variable so tests can point the lock dir to a temp path.
var userHomeDir = os.UserHomeDir

func lockDirPath() (string, error) {
	homeDir, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, lockDir), nil
}

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
	dir, err := lockDirPath()
	if err != nil {
		return err
	}

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

// CleanStaleLocks removes lock files left by emed instances that died
// without a clean shutdown. Locks from other IDEs are never touched.
func CleanStaleLocks() {
	dir, err := lockDirPath()
	if err != nil {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".lock" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var lock LockFile
		if err := json.Unmarshal(data, &lock); err != nil {
			continue
		}

		if lock.IDEName != ideName || lock.PID <= 0 || isProcessAlive(lock.PID) {
			continue
		}

		if err := os.Remove(path); err == nil {
			logging.Info("Removed stale Claude lock file %s (pid %d is dead)", entry.Name(), lock.PID)
		}
	}
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
