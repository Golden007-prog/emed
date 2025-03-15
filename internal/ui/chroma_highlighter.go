package ui

import (
	"strings"
	
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gdamore/tcell/v2"
)

// ChromaHighlighter implements the Highlighter interface using Chroma
type ChromaHighlighter struct {
	lexer       chroma.Lexer
	style       *chroma.Style
	language    string
	cache       map[int][]Token  // Line cache for performance
	cacheValid  bool
}

// NewChromaHighlighter creates a new Chroma-based highlighter
func NewChromaHighlighter() *ChromaHighlighter {
	return &ChromaHighlighter{
		style:      styles.Monokai,  // Default style
		cache:      make(map[int][]Token),
		cacheValid: true,
	}
}

// HighlightLine highlights a single line of code
func (h *ChromaHighlighter) HighlightLine(line string, lineNumber int) []Token {
	// Check cache first
	if h.cacheValid {
		if cached, ok := h.cache[lineNumber]; ok {
			return cached
		}
	}
	
	if h.lexer == nil {
		// No lexer, return plain text
		return []Token{{Text: line, Style: tcell.StyleDefault}}
	}
	
	tokens := h.tokenizeLine(line)
	
	// Cache the result
	h.cache[lineNumber] = tokens
	
	return tokens
}

// HighlightRange highlights a range of lines
func (h *ChromaHighlighter) HighlightRange(lines []string, startLine int) [][]Token {
	if h.lexer == nil {
		// No lexer, return plain text
		result := make([][]Token, len(lines))
		for i, line := range lines {
			result[i] = []Token{{Text: line, Style: tcell.StyleDefault}}
		}
		return result
	}
	
	// Join lines for context-aware highlighting
	text := strings.Join(lines, "\n")
	
	// Tokenize the entire block
	iterator, err := h.lexer.Tokenise(nil, text)
	if err != nil {
		// Fall back to line-by-line
		result := make([][]Token, len(lines))
		for i, line := range lines {
			result[i] = h.HighlightLine(line, startLine+i)
		}
		return result
	}
	
	// Convert Chroma tokens to our Token format
	result := make([][]Token, len(lines))
	currentLine := 0
	currentLineTokens := []Token{}
	
	for token := iterator(); token != chroma.EOF; token = iterator() {
		tokenLines := strings.Split(token.Value, "\n")
		style := h.chromaTokenToStyle(token.Type)
		
		for i, tokenLine := range tokenLines {
			if i > 0 {
				// New line, save current line and start new one
				result[currentLine] = currentLineTokens
				currentLine++
				if currentLine >= len(lines) {
					break
				}
				currentLineTokens = []Token{}
			}
			
			if tokenLine != "" {
				currentLineTokens = append(currentLineTokens, Token{
					Text:  tokenLine,
					Style: style,
				})
			}
		}
		
		if currentLine >= len(lines) {
			break
		}
	}
	
	// Save the last line
	if currentLine < len(lines) {
		result[currentLine] = currentLineTokens
	}
	
	// Cache the results
	for i, tokens := range result {
		h.cache[startLine+i] = tokens
	}
	
	return result
}

// SetLanguage sets the language for highlighting
func (h *ChromaHighlighter) SetLanguage(language string) error {
	lexer := lexers.Get(language)
	if lexer == nil {
		// Try to get by file extension
		lexer = lexers.Match(language)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	
	// Ensure lexer is coalesced
	if lexer != nil {
		lexer = chroma.Coalesce(lexer)
	}
	
	h.lexer = lexer
	h.language = language
	h.InvalidateCache()
	
	return nil
}

// DetectLanguage attempts to detect language from filename or content
func (h *ChromaHighlighter) DetectLanguage(filename string, content string) string {
	lexer := lexers.Match(filename)
	if lexer == nil && content != "" {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		return "text"
	}
	
	// Coalesce the lexer
	lexer = chroma.Coalesce(lexer)
	h.lexer = lexer
	
	config := lexer.Config()
	if config != nil && config.Name != "" {
		h.language = config.Name
		return config.Name
	}
	
	return "text"
}

// InvalidateCache clears any cached highlighting data
func (h *ChromaHighlighter) InvalidateCache() {
	h.cache = make(map[int][]Token)
	h.cacheValid = false
}

// GetTheme returns the current color theme name
func (h *ChromaHighlighter) GetTheme() string {
	if h.style == nil {
		return "default"
	}
	return h.style.Name
}

// SetTheme sets the color theme
func (h *ChromaHighlighter) SetTheme(theme string) error {
	style := styles.Get(theme)
	if style == nil {
		style = styles.Fallback
	}
	
	h.style = style
	h.InvalidateCache()
	
	return nil
}

// tokenizeLine tokenizes a single line
func (h *ChromaHighlighter) tokenizeLine(line string) []Token {
	if h.lexer == nil {
		return []Token{{Text: line, Style: tcell.StyleDefault}}
	}
	
	iterator, err := h.lexer.Tokenise(nil, line)
	if err != nil {
		return []Token{{Text: line, Style: tcell.StyleDefault}}
	}
	
	var tokens []Token
	for token := iterator(); token != chroma.EOF; token = iterator() {
		if token.Value != "" {
			tokens = append(tokens, Token{
				Text:  token.Value,
				Style: h.chromaTokenToStyle(token.Type),
			})
		}
	}
	
	if len(tokens) == 0 {
		return []Token{{Text: line, Style: tcell.StyleDefault}}
	}
	
	return tokens
}

// chromaTokenToStyle converts Chroma token types to tcell styles
func (h *ChromaHighlighter) chromaTokenToStyle(tokenType chroma.TokenType) tcell.Style {
	// Map Chroma token types to colors
	// This is a simplified version - you might want to use the actual theme colors
	switch tokenType {
	case chroma.Keyword, chroma.KeywordNamespace, chroma.KeywordType:
		return tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true)
		
	case chroma.Name, chroma.NameFunction, chroma.NameClass:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
		
	case chroma.NameBuiltin:
		return tcell.StyleDefault.Foreground(tcell.ColorAqua)
		
	case chroma.String, chroma.StringDouble, chroma.StringSingle:
		return tcell.StyleDefault.Foreground(tcell.ColorYellow)
		
	case chroma.Number, chroma.NumberInteger, chroma.NumberFloat:
		return tcell.StyleDefault.Foreground(tcell.ColorFuchsia)
		
	case chroma.Comment, chroma.CommentSingle, chroma.CommentMultiline:
		return tcell.StyleDefault.Foreground(tcell.ColorGray).Italic(true)
		
	case chroma.Operator:
		return tcell.StyleDefault.Foreground(tcell.ColorWhite)
		
	case chroma.Punctuation:
		return tcell.StyleDefault.Foreground(tcell.ColorWhite)
		
	default:
		return tcell.StyleDefault
	}
}