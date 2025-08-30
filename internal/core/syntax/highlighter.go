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
}

// NewHighlighter creates a new syntax highlighter for the given file
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
		lexer:    lexer,
		filename: filename,
	}
}

// UpdateContent updates the highlighter with new content
func (h *Highlighter) UpdateContent(lines []string) {
	if h == nil || h.lexer == nil {
		return
	}
	
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	
	// Store the new content
	h.lastContent = make([]string, len(lines))
	copy(h.lastContent, lines)
	
	// Re-tokenize the entire document
	h.tokenizeDocument()
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

// GetLineTokens returns the syntax tokens for a specific line
func (h *Highlighter) GetLineTokens(lineNumber int) []TokenStyle {
	if h == nil || h.lexer == nil {
		return nil
	}
	
	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()
	
	if lineNumber < 0 || lineNumber >= len(h.lineTokens) {
		return nil
	}
	
	return h.lineTokens[lineNumber]
}

// tokenTypeToStyle converts Chroma token types to tcell styles
func (h *Highlighter) tokenTypeToStyle(tokenType chroma.TokenType) tcell.Style {
	style := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	
	switch tokenType {
	// Comments
	case chroma.Comment, chroma.CommentSingle, chroma.CommentMultiline,
	     chroma.CommentSpecial, chroma.CommentPreproc, chroma.CommentHashbang:
		style = style.Foreground(tcell.ColorGray)
		
	// Strings
	case chroma.String, chroma.StringBacktick, chroma.StringChar,
	     chroma.StringDoc, chroma.StringDouble, chroma.StringEscape,
	     chroma.StringHeredoc, chroma.StringInterpol, chroma.StringOther,
	     chroma.StringRegex, chroma.StringSingle, chroma.StringSymbol:
		style = style.Foreground(tcell.ColorGreen)
		
	// Numbers
	case chroma.Number, chroma.NumberBin, chroma.NumberFloat,
	     chroma.NumberHex, chroma.NumberInteger, chroma.NumberOct:
		style = style.Foreground(tcell.ColorAqua)
		
	// Keywords
	case chroma.Keyword, chroma.KeywordConstant, chroma.KeywordDeclaration,
	     chroma.KeywordNamespace, chroma.KeywordPseudo, chroma.KeywordReserved,
	     chroma.KeywordType:
		style = style.Foreground(tcell.ColorPurple).Bold(true)
		
	// Names (functions, variables, etc.)
	case chroma.NameFunction, chroma.NameFunctionMagic:
		style = style.Foreground(tcell.ColorBlue)
		
	case chroma.NameBuiltin, chroma.NameBuiltinPseudo:
		style = style.Foreground(tcell.ColorOrange)
		
	case chroma.NameClass, chroma.NameException:
		style = style.Foreground(tcell.ColorTeal)
		
	case chroma.NameConstant:
		style = style.Foreground(tcell.ColorAqua)
		
	case chroma.NameDecorator, chroma.NameAttribute:
		style = style.Foreground(tcell.ColorOrange)
		
	case chroma.NameTag:
		style = style.Foreground(tcell.ColorRed)
		
	// Operators
	case chroma.Operator, chroma.OperatorWord:
		style = style.Foreground(tcell.ColorYellow)
		
	// Punctuation
	case chroma.Punctuation:
		style = style.Foreground(tcell.ColorWhite)
		
	// Literals
	case chroma.Literal, chroma.LiteralDate:
		style = style.Foreground(tcell.ColorAqua)
		
	// Generic types (errors, etc.)
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
	}
	
	return style
}

// Close frees any resources
func (h *Highlighter) Close() {
	if h == nil {
		return
	}
	
	h.cacheMu.Lock()
	h.lineTokens = nil
	h.lastContent = nil
	h.cacheMu.Unlock()
}