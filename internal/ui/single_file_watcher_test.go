package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSingleFileWatcherDetectsCreated(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "created.txt")

	watcher := NewSingleFileWatcher(path)
	watcher.exists = false
	watcher.modTime = time.Time{}

	var got []FileChange
	watcher.SetOnChange(func(changes []FileChange) {
		got = append(got, changes...)
	})

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	watcher.poll()

	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].Type != FileCreated {
		t.Fatalf("expected FileCreated, got %v", got[0].Type)
	}
}

func TestSingleFileWatcherDetectsModified(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "modified.txt")

	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	watcher := NewSingleFileWatcher(path)
	watcher.exists = true
	watcher.modTime = info.ModTime().Add(-time.Second)

	var got []FileChange
	watcher.SetOnChange(func(changes []FileChange) {
		got = append(got, changes...)
	})

	if err := os.WriteFile(path, []byte("two"), 0o644); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}

	watcher.poll()

	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].Type != FileModified {
		t.Fatalf("expected FileModified, got %v", got[0].Type)
	}
}

func TestSingleFileWatcherDetectsDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "deleted.txt")

	if err := os.WriteFile(path, []byte("bye"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}

	watcher := NewSingleFileWatcher(path)
	watcher.exists = true
	watcher.modTime = info.ModTime()

	var got []FileChange
	watcher.SetOnChange(func(changes []FileChange) {
		got = append(got, changes...)
	})

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	watcher.poll()

	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d", len(got))
	}
	if got[0].Type != FileDeleted {
		t.Fatalf("expected FileDeleted, got %v", got[0].Type)
	}
}
