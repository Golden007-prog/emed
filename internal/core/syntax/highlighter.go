// Package syntax provides syntax highlighting functionality using Chroma.
package syntax

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
)

// TokenStyle represents a highlighted token with position and style
type TokenStyle struct {
	Start int
	End   int
	Style tcell.Style
}

// Highlighter handles syntax highlighting using Chroma
type Highlighter struct {
	lexer    chroma.Lexer
	filename string

	// Cache for all lines' tokens
	lineTokens [][]TokenStyle
	cacheMu    sync.RWMutex

	// Track the last content to detect changes
	lastContent []string

	// Viewport-based highlighting
	viewportStart                  int  // First visible line
	viewportEnd                    int  // Last visible line
	contextBuffer                  int  // Extra lines to tokenize around viewport
	fullTokenized                  bool // Whether the entire document has been tokenized
	maxFileSizeForFullTokenization int  // Threshold to disable full tokenization
}

// NewHighlighter creates a new syntax highlighter for the given file.
func NewHighlighter(filename string) *Highlighter {
	// Get lexer based on filename
	lexer := lexers.Match(filename)
	if lexer == nil {
		// Try by extension
		ext := strings.ToLower(filepath.Ext(filename))
		if ext != "" {
			lexer = lexers.Get(ext[1:]) // Remove the dot
		}
	}

	// If still no lexer, try common aliases
	if lexer == nil && filename != "" {
		base := filepath.Base(filename)
		switch base {
		case "Makefile", "makefile", "GNUmakefile":
			lexer = lexers.Get("make")
		case "Dockerfile":
			lexer = lexers.Get("docker")
		case ".bashrc", ".bash_profile":
			lexer = lexers.Get("bash")
		case ".zshrc", ".zshenv":
			lexer = lexers.Get("zsh")
		}
	}

	if lexer == nil {
		return nil // No highlighting for unsupported files
	}

	// Ensure the lexer is mutable
	lexer = chroma.Coalesce(lexer)

	return &Highlighter{
		lexer:                          lexer,
		filename:                       filename,
		contextBuffer:                  50,   // Tokenize 50 lines around viewport
		maxFileSizeForFullTokenization: 5000, // Don't fully tokenize files > 5000 lines
	}
}

// UpdateContent updates the highlighter with new content.
func (h *Highlighter) UpdateContent(lines []string) {
	if h == nil || h.lexer == nil {
		return
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	// Disable highlighting for extremely large files (> 50000 lines)
	if len(lines) > 50000 {
		h.lastContent = nil
		h.lineTokens = nil
		h.fullTokenized = false
		return
	}

	// Store the new content
	h.lastContent = make([]string, len(lines))
	copy(h.lastContent, lines)

	// Clear existing tokens
	h.lineTokens = make([][]TokenStyle, len(h.lastContent))
	h.fullTokenized = false

	// For small files, tokenize the entire document immediately
	if len(lines) <= h.maxFileSizeForFullTokenization {
		h.tokenizeDocument()
		h.fullTokenized = true
	}
	// For large files, wait for viewport update to tokenize only visible portion
}

// UpdateViewport updates the visible viewport and tokenizes the necessary region.
func (h *Highlighter) UpdateViewport(startLine, endLine int) {
	if h == nil || h.lexer == nil {
		return
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	// Store viewport bounds
	h.viewportStart = startLine
	h.viewportEnd = endLine

	// If already fully tokenized or no content, nothing to do
	if h.fullTokenized || len(h.lastContent) == 0 {
		return
	}

	// Calculate region to tokenize (viewport + context buffer)
	tokenizeStart := startLine - h.contextBuffer
	if tokenizeStart < 0 {
		tokenizeStart = 0
	}
	tokenizeEnd := endLine + h.contextBuffer
	if tokenizeEnd >= len(h.lastContent) {
		tokenizeEnd = len(h.lastContent) - 1
	}

	// Find the actual range that needs tokenization (first and last nil lines)
	firstNil := -1
	lastNil := -1
	for i := tokenizeStart; i <= tokenizeEnd; i++ {
		if h.lineTokens[i] == nil {
			if firstNil == -1 {
				firstNil = i
			}
			lastNil = i
		}
	}

	// Only tokenize if there are nil lines, and only the range that needs it
	if firstNil != -1 {
		h.tokenizeRegion(firstNil, lastNil)
	}
}

// tokenizeRegion tokenizes a specific region of the document
func (h *Highlighter) tokenizeRegion(startLine, endLine int) {
	if startLine < 0 || endLine >= len(h.lastContent) || startLine > endLine {
		return
	}

	// Clear existing tokens for this region to prevent duplicates
	for i := startLine; i <= endLine; i++ {
		h.lineTokens[i] = nil
	}

	// Build text for the region
	regionLines := h.lastContent[startLine : endLine+1]
	regionText := strings.Join(regionLines, "\n")

	// Tokenize the region
	iterator, err := h.lexer.Tokenise(nil, regionText)
	if err != nil {
		return
	}

	// Process tokens and assign to lines
	currentLine := startLine
	currentPos := 0
	lineStartPos := 0

	for _, token := range iterator.Tokens() {
		tokenType := token.Type
		tokenValue := token.Value

		if tokenValue == "" {
			continue
		}

		style := h.tokenTypeToStyle(tokenType)
		tokenStart := currentPos

		// Process each character in the token
		for _, ch := range tokenValue {
			if ch == '\n' {
				// Token spans multiple lines
				// Add the portion on the current line
				if currentLine < len(h.lineTokens) {
					relStart := tokenStart - lineStartPos
					relEnd := currentPos - lineStartPos

					if relEnd > relStart && relStart >= 0 {
						if h.lineTokens[currentLine] == nil {
							h.lineTokens[currentLine] = make([]TokenStyle, 0)
						}
						h.lineTokens[currentLine] = append(h.lineTokens[currentLine], TokenStyle{
							Start: relStart,
							End:   relEnd,
							Style: style,
						})
					}
				}

				// Move to next line
				currentLine++
				lineStartPos = currentPos + 1
				tokenStart = lineStartPos
			}
			currentPos++
		}

		// Add remaining portion of token on current line
		if currentLine < len(h.lineTokens) && tokenStart < currentPos {
			relStart := tokenStart - lineStartPos
			relEnd := currentPos - lineStartPos

			if relEnd > relStart && relStart >= 0 {
				if h.lineTokens[currentLine] == nil {
					h.lineTokens[currentLine] = make([]TokenStyle, 0)
				}
				h.lineTokens[currentLine] = append(h.lineTokens[currentLine], TokenStyle{
					Start: relStart,
					End:   relEnd,
					Style: style,
				})
			}
		}
	}
}

// tokenizeDocument tokenizes the entire document and caches tokens per line
func (h *Highlighter) tokenizeDocument() {
	// Clear existing tokens
	h.lineTokens = make([][]TokenStyle, len(h.lastContent))

	if len(h.lastContent) == 0 {
		return
	}

	// Build the full text
	fullText := strings.Join(h.lastContent, "\n")

	// Tokenize
	iterator, err := h.lexer.Tokenise(nil, fullText)
	if err != nil {
		return
	}

	// Process tokens and assign to lines
	currentLine := 0
	currentPos := 0
	lineStartPos := 0

	for _, token := range iterator.Tokens() {
		tokenType := token.Type
		tokenValue := token.Value

		if tokenValue == "" {
			continue
		}

		style := h.tokenTypeToStyle(tokenType)
		tokenStart := currentPos

		// Process each character in the token
		for _, ch := range tokenValue {
			if ch == '\n' {
				// Token spans multiple lines
				// Add the portion on the current line
				if currentLine < len(h.lineTokens) {
					relStart := tokenStart - lineStartPos
					relEnd := currentPos - lineStartPos

					if relEnd > relStart && relStart >= 0 {
						h.lineTokens[currentLine] = append(h.lineTokens[currentLine], TokenStyle{
							Start: relStart,
							End:   relEnd,
							Style: style,
						})
					}
				}

				// Move to next line
				currentLine++
				lineStartPos = currentPos + 1
				tokenStart = lineStartPos
			}
			currentPos++
		}

		// Add remaining portion of token on current line
		if currentLine < len(h.lineTokens) && tokenStart < currentPos {
			relStart := tokenStart - lineStartPos
			relEnd := currentPos - lineStartPos

			if relEnd > relStart && relStart >= 0 {
				h.lineTokens[currentLine] = append(h.lineTokens[currentLine], TokenStyle{
					Start: relStart,
					End:   relEnd,
					Style: style,
				})
			}
		}
	}
}

// GetLineTokens returns the syntax tokens for a specific line.
func (h *Highlighter) GetLineTokens(lineNumber int) []TokenStyle {
	if h == nil || h.lexer == nil {
		return nil
	}

	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()

	// Return nil if highlighting is disabled or line is out of bounds
	if h.lineTokens == nil || lineNumber < 0 || lineNumber >= len(h.lineTokens) {
		return nil
	}

	return h.lineTokens[lineNumber]
}

// tokenTypeToStyle converts Chroma token types to tcell styles
func (h *Highlighter) tokenTypeToStyle(tokenType chroma.TokenType) tcell.Style {
	style := tcell.StyleDefault.Background(tcell.NewRGBColor(0, 0, 0)).Foreground(tcell.ColorWhite)

	switch tokenType {
	// Comments
	case chroma.Comment, chroma.CommentSingle, chroma.CommentMultiline,
		chroma.CommentSpecial, chroma.CommentPreproc, chroma.CommentHashbang:
		style = style.Foreground(tcell.ColorGray)

	// Strings (String constants are aliases for LiteralString in Chroma)
	case chroma.String, chroma.StringBacktick, chroma.StringChar,
		chroma.StringDoc, chroma.StringDouble, chroma.StringEscape,
		chroma.StringHeredoc, chroma.StringInterpol, chroma.StringOther,
		chroma.StringRegex, chroma.StringSingle, chroma.StringSymbol,
		chroma.LiteralStringAffix, chroma.LiteralStringAtom, chroma.LiteralStringBoolean,
		chroma.LiteralStringDelimiter, chroma.LiteralStringName:
		style = style.Foreground(tcell.ColorGreen)

	// Numbers (Number constants are aliases for LiteralNumber in Chroma)
	case chroma.Number, chroma.NumberBin, chroma.NumberFloat,
		chroma.NumberHex, chroma.NumberInteger, chroma.NumberOct,
		chroma.LiteralNumberIntegerLong:
		style = style.Foreground(tcell.ColorAqua)

	// Keywords
	case chroma.Keyword, chroma.KeywordConstant, chroma.KeywordDeclaration,
		chroma.KeywordNamespace, chroma.KeywordPseudo, chroma.KeywordReserved,
		chroma.KeywordType:
		style = style.Foreground(tcell.ColorPurple).Bold(true)

	// Functions
	case chroma.NameFunction, chroma.NameFunctionMagic:
		style = style.Foreground(tcell.ColorBlue)

	// Built-in names
	case chroma.NameBuiltin, chroma.NameBuiltinPseudo:
		style = style.Foreground(tcell.ColorOrange)

	// Classes and types
	case chroma.NameClass, chroma.NameException:
		style = style.Foreground(tcell.ColorTeal)

	// Constants
	case chroma.NameConstant:
		style = style.Foreground(tcell.ColorAqua).Bold(true)

	// Decorators and attributes
	case chroma.NameDecorator, chroma.NameAttribute, chroma.NameProperty:
		style = style.Foreground(tcell.ColorOrange)

	// Tags (HTML/XML)
	case chroma.NameTag:
		style = style.Foreground(tcell.ColorRed)

	// Variables
	case chroma.NameVariable, chroma.NameVariableClass, chroma.NameVariableGlobal,
		chroma.NameVariableInstance, chroma.NameVariableMagic, chroma.NameVariableAnonymous:
		style = style.Foreground(tcell.ColorLightCyan)

	// Namespaces
	case chroma.NameNamespace:
		style = style.Foreground(tcell.ColorYellow)

	// Labels
	case chroma.NameLabel:
		style = style.Foreground(tcell.ColorPurple)

	// Entities
	case chroma.NameEntity:
		style = style.Foreground(tcell.ColorOrange).Bold(true)

	// Other names
	case chroma.Name, chroma.NameOther, chroma.NamePseudo:
		style = style.Foreground(tcell.ColorLightGray)

	// Operators
	case chroma.Operator, chroma.OperatorWord:
		style = style.Foreground(tcell.ColorYellow)

	// Punctuation
	case chroma.Punctuation:
		style = style.Foreground(tcell.ColorWhite)

	// Literals
	case chroma.Literal, chroma.LiteralDate, chroma.LiteralOther:
		style = style.Foreground(tcell.ColorAqua)

	// Text (regular text, whitespace, etc.)
	case chroma.Text, chroma.TextWhitespace:
		style = style.Foreground(tcell.ColorWhite)

	// Generic types (errors, diffs, etc.)
	case chroma.GenericError:
		style = style.Foreground(tcell.ColorRed).Bold(true)

	case chroma.GenericDeleted:
		style = style.Foreground(tcell.ColorRed)

	case chroma.GenericInserted:
		style = style.Foreground(tcell.ColorGreen)

	case chroma.GenericHeading, chroma.GenericStrong:
		style = style.Bold(true)

	case chroma.GenericEmph:
		style = style.Italic(true)

	case chroma.GenericSubheading:
		style = style.Foreground(tcell.ColorPurple).Bold(true)

	case chroma.GenericPrompt:
		style = style.Foreground(tcell.ColorBlue).Bold(true)

	case chroma.GenericOutput:
		style = style.Foreground(tcell.ColorGray)

	case chroma.GenericTraceback:
		style = style.Foreground(tcell.ColorRed)

	case chroma.GenericUnderline:
		style = style.Underline(true)

	// Error token
	case chroma.Error:
		style = style.Background(tcell.ColorRed).Foreground(tcell.ColorWhite)

	// Background, line numbers, etc. (UI elements, not code)
	case chroma.Background, chroma.LineNumbers, chroma.LineNumbersTable,
		chroma.LineHighlight, chroma.LineTable, chroma.LineTableTD:
		// These are for output formatting, not for our terminal use
		style = style.Foreground(tcell.ColorGray)

	}

	return style
}

// Close frees any resources.
func (h *Highlighter) Close() {
	if h == nil {
		return
	}

	h.cacheMu.Lock()
	h.lineTokens = nil
	h.lastContent = nil
	h.cacheMu.Unlock()
}
