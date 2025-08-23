package interfaces

// BufferInterface defines the contract for text buffer implementations
type BufferInterface interface {
	// Content management
	LoadFromFile(path string) error
	SaveToFile(path string) error
	LoadFromString(content string)
	GetContent() string
	Clear()
	
	// Line operations
	GetLine(n int) string
	GetLines() []string
	GetLineCount() int
	InsertLine(n int, content string)
	DeleteLine(n int)
	ReplaceLine(n int, content string)
	
	// Character operations
	InsertChar(row, col int, ch rune)
	DeleteChar(row, col int)
	GetChar(row, col int) rune
	
	// Range operations
	GetRange(start, end Position) string
	DeleteRange(start, end Position)
	ReplaceRange(start, end Position, text string)
	
	// Modification tracking
	IsModified() bool
	SetModified(modified bool)
	GetVersion() int // Increments on each change
	
	// Undo/Redo
	BeginUndoGroup()
	EndUndoGroup()
	Undo() bool
	Redo() bool
	CanUndo() bool
	CanRedo() bool
	ClearUndoHistory()
	
	// Events
	OnModified(handler func())
	OnLinesChanged(handler func(start, end int))
	
	// Utilities
	GetLineLength(n int) int
	IsValidPosition(row, col int) bool
	NormalizePosition(row, col int) Position
}