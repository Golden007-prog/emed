package ui

import (
	"github.com/gdamore/tcell/v2"
)

// Token represents a syntax-highlighted piece of text
type Token struct {
	Text  string
	Style tcell.Style
}

// Highlighter is an interface for syntax highlighting implementations
// This allows us to swap between Chroma, Tree-sitter, or other highlighters
type Highlighter interface {
	// HighlightLine highlights a single line of code
	// Returns tokens with style information
	HighlightLine(line string, lineNumber int) []Token
	
	// HighlightRange highlights a range of lines
	// Useful for multi-line constructs like strings or comments
	HighlightRange(lines []string, startLine int) [][]Token
	
	// SetLanguage sets the language for highlighting
	SetLanguage(language string) error
	
	// DetectLanguage attempts to detect language from filename or content
	DetectLanguage(filename string, content string) string
	
	// InvalidateCache clears any cached highlighting data
	// Called when file content changes significantly
	InvalidateCache()
	
	// GetTheme returns the current color theme name
	GetTheme() string
	
	// SetTheme sets the color theme
	SetTheme(theme string) error
}

// HighlightResult contains the result of highlighting operation
type HighlightResult struct {
	Tokens [][]Token
	Language string
	Success bool
	Error error
}