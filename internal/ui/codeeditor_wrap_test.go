package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func newWrappedTestEditor(content string, width, height int) *CodeEditor {
	editor := NewCodeEditor()
	editor.SetContent(content)
	editor.SetLineWrapEnabled(true)
	editor.viewportWidth = width
	editor.viewportHeight = height
	editor.invalidateWrapLayout()
	editor.ensureWrapLayout()
	return editor
}

func TestCodeEditorBuildsWrappedSegments(t *testing.T) {
	editor := newWrappedTestEditor("abcdefghij", 4, 3)

	if len(editor.wrapSegments) != 3 {
		t.Fatalf("expected 3 wrapped segments, got %d", len(editor.wrapSegments))
	}

	expectedStarts := []int{0, 4, 8}
	expectedEnds := []int{4, 8, 10}
	for i, segment := range editor.wrapSegments {
		if segment.row != 0 {
			t.Fatalf("segment %d row: got %d want 0", i, segment.row)
		}
		if segment.startVisualCol != expectedStarts[i] || segment.endVisualCol != expectedEnds[i] {
			t.Fatalf("segment %d bounds: got %d..%d want %d..%d", i, segment.startVisualCol, segment.endVisualCol, expectedStarts[i], expectedEnds[i])
		}
		if segment.continuation != (i > 0) {
			t.Fatalf("segment %d continuation: got %v want %v", i, segment.continuation, i > 0)
		}
	}
}

func TestCodeEditorRebuildsWrapSegmentsAfterResize(t *testing.T) {
	editor := newWrappedTestEditor("abcdefghijkl", 8, 3)

	if len(editor.wrapSegments) != 2 {
		t.Fatalf("expected 2 wrapped segments before resize, got %d", len(editor.wrapSegments))
	}

	editor.viewportWidth = 4
	editor.ensureWrapLayout()

	if len(editor.wrapSegments) != 3 {
		t.Fatalf("expected 3 wrapped segments after resize, got %d", len(editor.wrapSegments))
	}
}

func TestCodeEditorResizeKeepsVisibleWrappedCursorVisible(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()

	screen.SetSize(11, 2)

	editor := NewCodeEditor()
	editor.SetContent("abcdefghijkl")
	editor.SetLineWrapEnabled(true)
	editor.SetRect(0, 0, 11, 2)
	editor.Draw(screen)

	editor.cursorRow = 0
	editor.cursorCol = 10
	editor.ensureCursorVisible()

	screen.SetSize(7, 2)
	editor.SetRect(0, 0, 7, 2)
	editor.Draw(screen)

	if editor.offsetRow != 1 {
		t.Fatalf("expected resized visual offset row 1, got %d", editor.offsetRow)
	}
}

func TestCodeEditorWrappedCursorVisibilityUsesVisualRows(t *testing.T) {
	editor := newWrappedTestEditor("abcdefghij", 4, 2)
	editor.cursorRow = 0
	editor.cursorCol = 9

	editor.ensureCursorVisible()

	if editor.offsetRow != 1 {
		t.Fatalf("expected visual offset row 1, got %d", editor.offsetRow)
	}
	if editor.offsetCol != 0 {
		t.Fatalf("expected horizontal offset to stay 0, got %d", editor.offsetCol)
	}
}

func TestCodeEditorWrappedVerticalNavigationMovesWithinVisualSegments(t *testing.T) {
	editor := newWrappedTestEditor("abcdefghij\n1234", 4, 5)
	editor.cursorRow = 0
	editor.cursorCol = 5

	editor.handleNavigation(tcell.KeyUp)
	if editor.cursorRow != 0 || editor.cursorCol != 1 {
		t.Fatalf("up from wrapped segment: got %d:%d want 0:1", editor.cursorRow, editor.cursorCol)
	}

	editor.handleNavigation(tcell.KeyDown)
	if editor.cursorRow != 0 || editor.cursorCol != 5 {
		t.Fatalf("down to wrapped segment: got %d:%d want 0:5", editor.cursorRow, editor.cursorCol)
	}
}

func TestCodeEditorWrappedMousePositionMapsThroughVisualOffset(t *testing.T) {
	editor := newWrappedTestEditor("abcdefghij", 4, 3)
	editor.offsetRow = 1

	editor.setCursorFromClick(2, 0)

	if editor.cursorRow != 0 || editor.cursorCol != 6 {
		t.Fatalf("click position: got %d:%d want 0:6", editor.cursorRow, editor.cursorCol)
	}
}

func TestCodeEditorDrawsWrappedContinuationWithoutLineNumber(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init screen: %v", err)
	}
	defer screen.Fini()

	screen.SetSize(6, 3)

	editor := NewCodeEditor()
	editor.SetContent("abcdef")
	editor.SetLineWrapEnabled(true)
	editor.SetRect(0, 0, 6, 3)
	editor.Draw(screen)

	lineNum, _, _, _ := screen.GetContent(0, 0)
	if lineNum != '1' {
		t.Fatalf("first visual row line number: got %q want '1'", lineNum)
	}

	continuationLineNum, _, _, _ := screen.GetContent(0, 1)
	if continuationLineNum != ' ' {
		t.Fatalf("continuation visual row line number: got %q want space", continuationLineNum)
	}

	firstWrappedRune, _, _, _ := screen.GetContent(2, 1)
	if firstWrappedRune != 'd' {
		t.Fatalf("wrapped content: got %q want 'd'", firstWrappedRune)
	}
}

func TestEditorTogglesLineWrapForExistingAndFutureTabs(t *testing.T) {
	editor := NewEditor()
	first := NewCodeEditor()
	second := NewCodeEditor()

	tabA := &EditorTab{id: 1, codeEditor: first}
	editor.tabs = append(editor.tabs, tabA)

	if !editor.LineWrapEnabled() {
		t.Fatal("expected line wrap to be enabled by default")
	}

	if editor.ToggleLineWrap() != false {
		t.Fatal("expected line wrap to be disabled")
	}
	if first.LineWrapEnabled() {
		t.Fatal("expected existing tab to stop using line wrap")
	}

	second.SetLineWrapEnabled(editor.LineWrapEnabled())
	if second.LineWrapEnabled() {
		t.Fatal("expected future tab to use editor line wrap setting")
	}
}
