package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditorSingleFileModeKeepsSingleTab(t *testing.T) {
	tmpDir := t.TempDir()
	fileA := filepath.Join(tmpDir, "a.txt")
	fileB := filepath.Join(tmpDir, "b.txt")

	if err := os.WriteFile(fileA, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("beta\n"), 0o644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}

	editor := NewEditor()
	editor.SetSingleFileMode(true)

	editor.OpenFile(fileA)
	if editor.GetTabCount() != 1 {
		t.Fatalf("expected 1 tab after first open, got %d", editor.GetTabCount())
	}
	if editor.GetActiveFilePath() != fileA {
		t.Fatalf("expected active file %q, got %q", fileA, editor.GetActiveFilePath())
	}

	editor.OpenFile(fileB)
	if editor.GetTabCount() != 1 {
		t.Fatalf("expected 1 tab after second open, got %d", editor.GetTabCount())
	}
	if editor.GetActiveFilePath() != fileA {
		t.Fatalf("expected active file to remain %q, got %q", fileA, editor.GetActiveFilePath())
	}
}
