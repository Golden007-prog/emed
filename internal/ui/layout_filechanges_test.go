package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// atomicSave replaces the target file like most linux apps does when saving:
// write a temp file and rename it over the target. The watcher see this as a
// Create event, not a Write.
func atomicSave(t *testing.T, target, content string) {
	t.Helper()

	tmp := target + ".new"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		t.Fatalf("rename over target: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(target, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func openLayoutWithFile(t *testing.T, content string) (*Layout, *EditorTab, string) {
	t.Helper()

	target := filepath.Join(t.TempDir(), "doc.txt")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	editor := NewEditor()
	editor.OpenFile(target)
	if len(editor.tabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(editor.tabs))
	}

	return &Layout{editor: editor}, editor.tabs[0], target
}

func TestHandleFileChangesCreatedEventReloadsCleanTab(t *testing.T) {
	layout, tab, target := openLayoutWithFile(t, "v1\n")

	atomicSave(t, target, "v2\n")

	layout.handleFileChanges([]FileChange{{Path: target, Type: FileCreated}})

	got := tab.codeEditor.buffer.GetContent()
	if !strings.Contains(got, "v2") {
		t.Errorf("clean tab was not reloaded after atomic save, content = %q", got)
	}
	if tab.externallyModified {
		t.Errorf("clean tab should not be marked as externally modified after reload")
	}
}

func TestHandleFileChangesCreatedEventMarksConflictOnDirtyTab(t *testing.T) {
	layout, tab, target := openLayoutWithFile(t, "v1\n")
	tab.SetModified(true)

	atomicSave(t, target, "v2\n")

	layout.handleFileChanges([]FileChange{{Path: target, Type: FileCreated}})

	got := tab.codeEditor.buffer.GetContent()
	if strings.Contains(got, "v2") {
		t.Errorf("dirty tab must not be reloaded, but content was replaced: %q", got)
	}
	if !tab.externallyModified {
		t.Errorf("dirty tab should be marked as externally modified")
	}
}

func TestHandleFileChangesModifiedEventStillReloadsCleanTab(t *testing.T) {
	layout, tab, target := openLayoutWithFile(t, "v1\n")

	if err := os.WriteFile(target, []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("in-place write: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(target, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	layout.handleFileChanges([]FileChange{{Path: target, Type: FileModified}})

	got := tab.codeEditor.buffer.GetContent()
	if !strings.Contains(got, "v2") {
		t.Errorf("clean tab was not reloaded after in-place write, content = %q", got)
	}
}
