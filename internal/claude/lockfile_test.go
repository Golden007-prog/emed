package claude

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func overrideHomeDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	orig := userHomeDir
	userHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDir = orig })
	return home
}

func writeLockFile(t *testing.T, dir, name string, lock LockFile) string {
	t.Helper()
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatalf("failed to marshal lock: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}
	return path
}

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot spawn helper process: %v", err)
	}
	return cmd.Process.Pid
}

func TestCleanStaleLocks(t *testing.T) {
	home := overrideHomeDir(t)
	dir := filepath.Join(home, lockDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create lock dir: %v", err)
	}

	dead := deadPID(t)

	staleEmed := writeLockFile(t, dir, "10001.lock", LockFile{PID: dead, IDEName: ideName, Transport: "ws"})
	aliveEmed := writeLockFile(t, dir, "10002.lock", LockFile{PID: os.Getpid(), IDEName: ideName, Transport: "ws"})
	staleOtherIDE := writeLockFile(t, dir, "10003.lock", LockFile{PID: dead, IDEName: "vscode", Transport: "ws"})

	garbage := filepath.Join(dir, "10004.lock")
	if err := os.WriteFile(garbage, []byte("not json"), 0600); err != nil {
		t.Fatalf("failed to write garbage file: %v", err)
	}

	CleanStaleLocks()

	checks := []struct {
		name     string
		path     string
		wantGone bool
	}{
		{name: "stale emed lock removed", path: staleEmed, wantGone: true},
		{name: "alive emed lock kept", path: aliveEmed, wantGone: false},
		{name: "other IDE lock kept", path: staleOtherIDE, wantGone: false},
		{name: "unparseable file kept", path: garbage, wantGone: false},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			_, err := os.Stat(tc.path)
			gone := os.IsNotExist(err)
			if gone != tc.wantGone {
				t.Fatalf("unexpected state for %s: gone=%t want=%t", tc.path, gone, tc.wantGone)
			}
		})
	}
}

func TestLockFileWriteAndRemove(t *testing.T) {
	home := overrideHomeDir(t)

	mgr := NewLockFileManager(12345, []string{"/tmp/work"})
	if err := mgr.Write(); err != nil {
		t.Fatalf("failed to write lock file: %v", err)
	}

	path := filepath.Join(home, lockDir, "12345.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	var lock LockFile
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatalf("lock file is not valid json: %v", err)
	}

	if lock.PID != os.Getpid() {
		t.Fatalf("unexpected pid: got %d want %d", lock.PID, os.Getpid())
	}
	if lock.IDEName != ideName {
		t.Fatalf("unexpected ide name: %q", lock.IDEName)
	}
	if lock.Token != mgr.Token() || lock.Token == "" {
		t.Fatalf("token mismatch in lock file")
	}

	if err := mgr.Remove(); err != nil {
		t.Fatalf("failed to remove lock file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file still exists after remove")
	}
}
