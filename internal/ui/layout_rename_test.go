package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func openForRenameTest(t *testing.T, root, rel, content string) (*Layout, string) {
	t.Helper()

	abs := writeProjectFile(t, root, rel, content)
	layout := newSessionTestLayout(t, root)
	layout.editor.OpenFile(abs)
	layout.recordInode(abs)
	return layout, abs
}

func TestExternalRenameFollowsFile(t *testing.T) {
	root := t.TempDir()
	layout, oldPath := openForRenameTest(t, root, "old.go", "package x\n")

	newPath := filepath.Join(root, "new.go")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}

	layout.handleFileChanges([]FileChange{
		{Path: oldPath, Type: FileRenamed},
		{Path: newPath, Type: FileCreated},
	})

	paths := layout.editor.GetOpenFilePaths()
	if len(paths) != 1 || paths[0] != newPath {
		t.Fatalf("expected tab to follow rename to %q, got %v", newPath, paths)
	}
	if _, tracked := layout.openFileInodes[oldPath]; tracked {
		t.Error("old path should not stay tracked")
	}
	if _, tracked := layout.openFileInodes[newPath]; !tracked {
		t.Error("new path should be tracked")
	}
}

func TestExternalDirectoryRenameRemapsTabs(t *testing.T) {
	root := t.TempDir()
	layout, oldPath := openForRenameTest(t, root, "src/a.go", "package a\n")

	oldDir := filepath.Join(root, "src")
	newDir := filepath.Join(root, "lib")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}

	layout.handleFileChanges([]FileChange{
		{Path: oldDir, Type: FileRenamed},
		{Path: newDir, Type: FileCreated},
	})

	want := filepath.Join(newDir, "a.go")
	paths := layout.editor.GetOpenFilePaths()
	if len(paths) != 1 || paths[0] != want {
		t.Fatalf("expected tab remapped to %q, got %v", want, paths)
	}
	_ = oldPath
}

func TestExternalRenameWithoutPairClosesCleanTab(t *testing.T) {
	root := t.TempDir()
	layout, oldPath := openForRenameTest(t, root, "gone.go", "package x\n")

	outside := filepath.Join(t.TempDir(), "gone.go")
	if err := os.Rename(oldPath, outside); err != nil {
		t.Fatal(err)
	}

	layout.handleFileChanges([]FileChange{
		{Path: oldPath, Type: FileRenamed},
	})

	if count := layout.editor.GetTabCount(); count != 0 {
		t.Fatalf("expected clean tab closed after unpaired rename, got %d tabs", count)
	}
}

func TestReplaceStyleSaveReloadsInsteadOfRemap(t *testing.T) {
	root := t.TempDir()
	layout, oldPath := openForRenameTest(t, root, "doc.txt", "v1\n")

	backup := oldPath + ".bak"
	if err := os.Rename(oldPath, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := layout.editor.tabs[0].lastModTime.Add(2e9)
	if err := os.Chtimes(oldPath, future, future); err != nil {
		t.Fatal(err)
	}

	layout.handleFileChanges([]FileChange{
		{Path: oldPath, Type: FileRenamed},
		{Path: backup, Type: FileCreated},
		{Path: oldPath, Type: FileCreated},
	})

	paths := layout.editor.GetOpenFilePaths()
	if len(paths) != 1 || paths[0] != oldPath {
		t.Fatalf("tab must stay on %q for replace-style saves, got %v", oldPath, paths)
	}
	got := layout.editor.tabs[0].codeEditor.buffer.GetContent()
	if got != "v2\n" && got != "v2" {
		t.Fatalf("expected reloaded content v2, got %q", got)
	}
}
