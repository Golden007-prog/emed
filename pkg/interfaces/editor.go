package interfaces

import "github.com/gdamore/tcell/v2"

// EditorInterface defines the contract for text editor implementations
type EditorInterface interface {
	// File operations
	OpenFile(path string) error
	SaveFile() error
	SaveFileAs(path string) error
	CloseFile() error
	IsModified() bool
	GetFilePath() string
	
	// Text operations
	InsertText(text string)
	DeleteChar()
	DeleteLine()
	GetText() string
	GetLine(n int) string
	GetLineCount() int
	
	// Cursor operations
	SetCursor(row, col int)
	GetCursor() (row, col int)
	MoveCursorUp()
	MoveCursorDown()
	MoveCursorLeft()
	MoveCursorRight()
	MoveCursorToLineStart()
	MoveCursorToLineEnd()
	
	// Selection operations
	StartSelection()
	EndSelection()
	GetSelectedText() string
	DeleteSelection()
	HasSelection() bool
	
	// Clipboard operations
	Copy() error
	Cut() error
	Paste() error
	
	// Undo/Redo
	Undo() error
	Redo() error
	CanUndo() bool
	CanRedo() bool
	
	// Search/Replace
	Find(pattern string, caseSensitive bool) []Position
	Replace(pattern, replacement string, all bool) int
	
	// Events
	OnModified(handler func())
	OnCursorMove(handler func(row, col int))
	
	// UI integration
	HandleKey(event *tcell.EventKey) bool
	Render() // For custom rendering if needed
}

// Position represents a location in the text
type Position struct {
	Row int
	Col int
}

// Range represents a text range
type Range struct {
	Start Position
	End   Position
}