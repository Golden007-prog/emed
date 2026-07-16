package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laborin/emed/internal/project"
	"github.com/rivo/tview"
)

func newSessionTestLayout(t *testing.T, root string) *Layout {
	t.Helper()

	return &Layout{
		editor:           NewEditor(),
		terminal:         NewTerminal(),
		fileManager:      NewFileManager(),
		mainArea:         tview.NewFlex(),
		rightSide:        tview.NewFlex(),
		projectStore:     project.NewStore(root),
		starterTabs:      make(map[string]int),
		terminalPosition: "bottom",
		proportionIndex:  1,
		fileManagerWidth: 30,
	}
}

func writeProjectFile(t *testing.T, root, rel, content string) string {
	t.Helper()

	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestProjectRelPath(t *testing.T) {
	root := t.TempDir()

	rel, ok := projectRelPath(root, filepath.Join(root, "src", "main.go"))
	if !ok || rel != "src/main.go" {
		t.Errorf("expected src/main.go, got %q ok=%v", rel, ok)
	}

	if _, ok := projectRelPath(root, "/etc/passwd"); ok {
		t.Error("paths outside the root must be rejected")
	}
}

func TestResolveProjectPath(t *testing.T) {
	root := t.TempDir()
	abs := writeProjectFile(t, root, "docs/note.md", "hi\n")

	got, ok := resolveProjectPath(root, "docs/note.md")
	if !ok || got != abs {
		t.Errorf("expected %q, got %q ok=%v", abs, got, ok)
	}

	if _, ok := resolveProjectPath(root, "../escape.txt"); ok {
		t.Error("relative paths escaping the root must be rejected")
	}

	if _, ok := resolveProjectPath(root, "missing.txt"); ok {
		t.Error("missing files must be rejected")
	}
}

func TestSessionSaveAndRestoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	fileA := writeProjectFile(t, root, "a.txt", "alpha\n")
	fileB := writeProjectFile(t, root, "sub/b.txt", "beta\n")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("out\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	saver := newSessionTestLayout(t, root)
	saver.editor.OpenFile(fileA)
	saver.editor.OpenFile(fileB)
	saver.editor.OpenFile(outside)
	saver.saveSession()

	session, err := saver.projectStore.LoadSession()
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if len(session.OpenFiles) != 2 {
		t.Fatalf("expected 2 files in session (outside file excluded), got %v", session.OpenFiles)
	}
	if session.OpenFiles[0] != "a.txt" || session.OpenFiles[1] != "sub/b.txt" {
		t.Fatalf("unexpected session files: %v", session.OpenFiles)
	}

	restorer := newSessionTestLayout(t, root)
	restorer.restoreSession()

	if restorer.editor.GetTabCount() != 2 {
		t.Fatalf("expected 2 restored tabs, got %d", restorer.editor.GetTabCount())
	}
	paths := restorer.editor.GetOpenFilePaths()
	if paths[0] != fileA || paths[1] != fileB {
		t.Fatalf("unexpected restored paths: %v", paths)
	}
}

func TestSessionRestoreActivatesLastActiveFile(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "a.txt", "alpha\n")
	fileB := writeProjectFile(t, root, "b.txt", "beta\n")

	store := project.NewStore(root)
	if err := store.SaveSession(project.Session{
		OpenFiles:  []string{"a.txt", "b.txt"},
		ActiveFile: "b.txt",
	}); err != nil {
		t.Fatal(err)
	}

	layout := newSessionTestLayout(t, root)
	layout.restoreSession()

	if got := layout.editor.GetActiveFilePath(); got != fileB {
		t.Errorf("expected active file %q, got %q", fileB, got)
	}
}

func TestSessionRestoreSkipsDeletedFiles(t *testing.T) {
	root := t.TempDir()
	fileA := writeProjectFile(t, root, "a.txt", "alpha\n")

	store := project.NewStore(root)
	if err := store.SaveSession(project.Session{
		OpenFiles: []string{"a.txt", "gone.txt", "../escape.txt"},
	}); err != nil {
		t.Fatal(err)
	}

	layout := newSessionTestLayout(t, root)
	layout.restoreSession()

	if layout.editor.GetTabCount() != 1 {
		t.Fatalf("expected only 1 restored tab, got %d", layout.editor.GetTabCount())
	}
	if layout.editor.GetActiveFilePath() != fileA {
		t.Errorf("expected %q restored, got %q", fileA, layout.editor.GetActiveFilePath())
	}
}

func TestSaveSessionNoOpWithoutProjectStore(t *testing.T) {
	layout := &Layout{editor: NewEditor()}
	layout.saveSession()
	layout.restoreSession()
}
