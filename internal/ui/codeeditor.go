package ui

import (
	"io/ioutil"
	"strings"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// CodeEditor is a custom primitive for code editing with syntax highlighting
type CodeEditor struct {
	*tview.Box
	
	// Buffer management
	buffer       [][]rune          // The actual text content (2D array of runes for Unicode support)
	lineEndings  string            // Line ending type (\n, \r\n, etc.)
	hasFocus     bool              // Track focus state
	
	// Cursor position
	cursorRow    int
	cursorCol    int
	
	// Viewport (for scrolling)
	offsetRow    int               // First visible row
	offsetCol    int               // First visible column
	viewportHeight int             // Number of visible rows
	viewportWidth  int             // Number of visible columns
	
	// Selection
	selectionStart struct {
		row, col int
	}
	selectionEnd struct {
		row, col int
	}
	hasSelection bool
	
	// File info
	filePath     string
	modified     bool
	readOnly     bool
	
	// Syntax highlighting
	highlighter  Highlighter       // Interface for modular highlighting
	highlighted  [][]Token         // Cached highlighted lines
	
	// Callbacks
	onModified   func(modified bool)
	onCursorMove func(row, col int)
	
	// Application reference
	app          *tview.Application
}

// NewCodeEditor creates a new code editor
func NewCodeEditor() *CodeEditor {
	return &CodeEditor{
		Box:         tview.NewBox(),
		buffer:      [][]rune{{}},  // Start with one empty line
		lineEndings: "\n",
		highlighter: NewChromaHighlighter(), // Default to Chroma
		highlighted: [][]Token{},
		readOnly:    false,
	}
}

// SetHighlighter sets a custom highlighter (allows swapping to tree-sitter later)
func (e *CodeEditor) SetHighlighter(highlighter Highlighter) *CodeEditor {
	e.highlighter = highlighter
	e.rehighlightAll()
	return e
}

// LoadFile loads a file into the editor
func (e *CodeEditor) LoadFile(path string) error {
	// Read file
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	
	// Detect line endings
	e.detectLineEndings(string(content))
	
	// Split into lines
	lines := strings.Split(string(content), e.lineEndings)
	e.buffer = make([][]rune, len(lines))
	for i, line := range lines {
		e.buffer[i] = []rune(line)
	}
	
	// If empty file, ensure at least one line
	if len(e.buffer) == 0 {
		e.buffer = [][]rune{{}}
	}
	
	// Set file path and detect language
	e.filePath = path
	if e.highlighter != nil {
		language := e.highlighter.DetectLanguage(path, string(content))
		e.highlighter.SetLanguage(language)
	}
	
	// Reset cursor and viewport
	e.cursorRow = 0
	e.cursorCol = 0
	e.offsetRow = 0
	e.offsetCol = 0
	e.modified = false
	
	// Highlight content
	e.rehighlightAll()
	
	return nil
}

// SaveFile saves the editor content to a file
func (e *CodeEditor) SaveFile(path string) error {
	if path == "" {
		path = e.filePath
	}
	
	// Build content
	lines := make([]string, len(e.buffer))
	for i, line := range e.buffer {
		lines[i] = string(line)
	}
	content := strings.Join(lines, e.lineEndings)
	
	// Write file
	err := ioutil.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return err
	}
	
	e.filePath = path
	e.modified = false
	
	if e.onModified != nil {
		e.onModified(false)
	}
	
	return nil
}

// Draw draws the editor
func (e *CodeEditor) Draw(screen tcell.Screen) {
	e.Box.DrawForSubclass(screen, e)
	x, y, width, height := e.GetInnerRect()
	
	e.viewportHeight = height
	e.viewportWidth = width
	
	// Draw visible lines
	for row := 0; row < height && row+e.offsetRow < len(e.buffer); row++ {
		e.drawLine(screen, row+e.offsetRow, x, y+row, width)
	}
	
	// Show cursor only when focused
	if e.hasFocus {
		if e.cursorRow >= e.offsetRow && e.cursorRow < e.offsetRow+height {
			cursorX := x + e.getVisualColumn(e.cursorRow, e.cursorCol) - e.offsetCol
			cursorY := y + e.cursorRow - e.offsetRow
			
			if cursorX >= x && cursorX < x+width {
				screen.ShowCursor(cursorX, cursorY)
			}
		}
	} else {
		// Hide cursor when not focused
		screen.HideCursor()
	}
}

// drawLine draws a single line with syntax highlighting
func (e *CodeEditor) drawLine(screen tcell.Screen, lineNum int, x, y, maxWidth int) {
	if lineNum >= len(e.buffer) {
		return
	}
	
	// Get highlighted tokens for this line
	var tokens []Token
	if lineNum < len(e.highlighted) {
		tokens = e.highlighted[lineNum]
	} else {
		// Fallback to plain text
		tokens = []Token{{Text: string(e.buffer[lineNum]), Style: tcell.StyleDefault}}
	}
	
	// Draw tokens
	currentX := x - e.offsetCol
	for _, token := range tokens {
		for _, ch := range token.Text {
			if currentX >= x && currentX < x+maxWidth {
				screen.SetContent(currentX, y, ch, nil, token.Style)
			}
			currentX++
			if currentX >= x+maxWidth {
				return
			}
		}
	}
}

// InputHandler handles keyboard input
func (e *CodeEditor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if e.readOnly {
			// Only allow navigation in read-only mode
			switch event.Key() {
			case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight,
			     tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
				e.handleNavigation(event.Key())
			}
			return
		}
		
		switch event.Key() {
		case tcell.KeyRune:
			e.insertRune(event.Rune())
			
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			e.deleteCharBackward()
			
		case tcell.KeyDelete:
			e.deleteCharForward()
			
		case tcell.KeyEnter:
			e.insertNewline()
			
		case tcell.KeyTab:
			e.insertTab()
			
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
			e.handleNavigation(event.Key())
			
		case tcell.KeyHome:
			e.moveCursorToLineStart()
			
		case tcell.KeyEnd:
			e.moveCursorToLineEnd()
			
		case tcell.KeyPgUp:
			e.pageUp()
			
		case tcell.KeyPgDn:
			e.pageDown()
			
		case tcell.KeyCtrlC:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				e.copySelection()
			}
			
		case tcell.KeyCtrlV:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				e.paste()
			}
			
		case tcell.KeyCtrlX:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				e.cutSelection()
			}
			
		case tcell.KeyCtrlA:
			if event.Modifiers()&tcell.ModCtrl != 0 {
				e.selectAll()
			}
		}
		
		// Update viewport to follow cursor
		e.ensureCursorVisible()
		
		// Don't call app.Draw() here - tview will handle redraw automatically
	}
}

// insertRune inserts a character at the cursor position
func (e *CodeEditor) insertRune(r rune) {
	if e.readOnly {
		return
	}
	
	// Delete selection first if any
	if e.hasSelection {
		e.deleteSelection()
	}
	
	// Ensure we're within bounds
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	// Insert the rune
	line := e.buffer[e.cursorRow]
	newLine := make([]rune, 0, len(line)+1)
	newLine = append(newLine, line[:e.cursorCol]...)
	newLine = append(newLine, r)
	newLine = append(newLine, line[e.cursorCol:]...)
	e.buffer[e.cursorRow] = newLine
	
	// Move cursor
	e.cursorCol++
	
	// Mark as modified
	e.setModified(true)
	
	// Re-highlight this line
	e.rehighlightLine(e.cursorRow)
}

// insertNewline inserts a new line at the cursor position
func (e *CodeEditor) insertNewline() {
	if e.readOnly {
		return
	}
	
	// Delete selection first if any
	if e.hasSelection {
		e.deleteSelection()
	}
	
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	// Split current line
	currentLine := e.buffer[e.cursorRow]
	beforeCursor := currentLine[:e.cursorCol]
	afterCursor := currentLine[e.cursorCol:]
	
	// Update current line
	e.buffer[e.cursorRow] = beforeCursor
	
	// Insert new line
	newBuffer := make([][]rune, len(e.buffer)+1)
	copy(newBuffer[:e.cursorRow+1], e.buffer[:e.cursorRow+1])
	newBuffer[e.cursorRow+1] = afterCursor
	copy(newBuffer[e.cursorRow+2:], e.buffer[e.cursorRow+1:])
	e.buffer = newBuffer
	
	// Move cursor to start of new line
	e.cursorRow++
	e.cursorCol = 0
	
	// Mark as modified
	e.setModified(true)
	
	// Re-highlight affected lines
	e.rehighlightRange(e.cursorRow-1, e.cursorRow+1)
}

// deleteCharBackward deletes the character before the cursor
func (e *CodeEditor) deleteCharBackward() {
	if e.readOnly {
		return
	}
	
	// Delete selection if any
	if e.hasSelection {
		e.deleteSelection()
		return
	}
	
	if e.cursorCol > 0 {
		// Delete within line
		line := e.buffer[e.cursorRow]
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:e.cursorCol-1]...)
		newLine = append(newLine, line[e.cursorCol:]...)
		e.buffer[e.cursorRow] = newLine
		e.cursorCol--
		e.setModified(true)
		e.rehighlightLine(e.cursorRow)
	} else if e.cursorRow > 0 {
		// Join with previous line
		prevLine := e.buffer[e.cursorRow-1]
		currentLine := e.buffer[e.cursorRow]
		e.buffer[e.cursorRow-1] = append(prevLine, currentLine...)
		
		// Remove current line
		newBuffer := make([][]rune, len(e.buffer)-1)
		copy(newBuffer[:e.cursorRow], e.buffer[:e.cursorRow])
		copy(newBuffer[e.cursorRow:], e.buffer[e.cursorRow+1:])
		e.buffer = newBuffer
		
		// Move cursor to end of previous line
		e.cursorRow--
		e.cursorCol = len(prevLine)
		e.setModified(true)
		e.rehighlightRange(e.cursorRow, e.cursorRow+1)
	}
}

// deleteCharForward deletes the character at the cursor
func (e *CodeEditor) deleteCharForward() {
	if e.readOnly {
		return
	}
	
	// Delete selection if any
	if e.hasSelection {
		e.deleteSelection()
		return
	}
	
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	line := e.buffer[e.cursorRow]
	if e.cursorCol < len(line) {
		// Delete within line
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:e.cursorCol]...)
		newLine = append(newLine, line[e.cursorCol+1:]...)
		e.buffer[e.cursorRow] = newLine
		e.setModified(true)
		e.rehighlightLine(e.cursorRow)
	} else if e.cursorRow < len(e.buffer)-1 {
		// Join with next line
		nextLine := e.buffer[e.cursorRow+1]
		e.buffer[e.cursorRow] = append(line, nextLine...)
		
		// Remove next line
		newBuffer := make([][]rune, len(e.buffer)-1)
		copy(newBuffer[:e.cursorRow+1], e.buffer[:e.cursorRow+1])
		copy(newBuffer[e.cursorRow+1:], e.buffer[e.cursorRow+2:])
		e.buffer = newBuffer
		e.setModified(true)
		e.rehighlightRange(e.cursorRow, e.cursorRow+1)
	}
}

// Navigation methods
func (e *CodeEditor) handleNavigation(key tcell.Key) {
	switch key {
	case tcell.KeyUp:
		if e.cursorRow > 0 {
			e.cursorRow--
			e.adjustCursorColumn()
		}
	case tcell.KeyDown:
		if e.cursorRow < len(e.buffer)-1 {
			e.cursorRow++
			e.adjustCursorColumn()
		}
	case tcell.KeyLeft:
		if e.cursorCol > 0 {
			e.cursorCol--
		} else if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = len(e.buffer[e.cursorRow])
		}
	case tcell.KeyRight:
		if e.cursorRow < len(e.buffer) && e.cursorCol < len(e.buffer[e.cursorRow]) {
			e.cursorCol++
		} else if e.cursorRow < len(e.buffer)-1 {
			e.cursorRow++
			e.cursorCol = 0
		}
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// Helper methods
func (e *CodeEditor) adjustCursorColumn() {
	if e.cursorRow < len(e.buffer) {
		lineLen := len(e.buffer[e.cursorRow])
		if e.cursorCol > lineLen {
			e.cursorCol = lineLen
		}
	}
}

func (e *CodeEditor) ensureCursorVisible() {
	// Vertical scrolling
	if e.cursorRow < e.offsetRow {
		e.offsetRow = e.cursorRow
	} else if e.cursorRow >= e.offsetRow+e.viewportHeight {
		e.offsetRow = e.cursorRow - e.viewportHeight + 1
	}
	
	// Horizontal scrolling
	visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
	if visualCol < e.offsetCol {
		e.offsetCol = visualCol
	} else if visualCol >= e.offsetCol+e.viewportWidth {
		e.offsetCol = visualCol - e.viewportWidth + 1
	}
}

func (e *CodeEditor) getVisualColumn(row, col int) int {
	// For now, assume 1:1 mapping (no tab expansion)
	// TODO: Handle tab width properly
	return col
}

func (e *CodeEditor) setModified(modified bool) {
	if e.modified != modified {
		e.modified = modified
		if e.onModified != nil {
			e.onModified(modified)
		}
	}
}

func (e *CodeEditor) detectLineEndings(content string) {
	if strings.Contains(content, "\r\n") {
		e.lineEndings = "\r\n"
	} else if strings.Contains(content, "\r") {
		e.lineEndings = "\r"
	} else {
		e.lineEndings = "\n"
	}
}

// Highlighting methods
func (e *CodeEditor) rehighlightAll() {
	if e.highlighter == nil {
		return
	}
	
	// Convert buffer to strings
	lines := make([]string, len(e.buffer))
	for i, line := range e.buffer {
		lines[i] = string(line)
	}
	
	// Highlight all lines
	e.highlighted = e.highlighter.HighlightRange(lines, 0)
}

func (e *CodeEditor) rehighlightLine(lineNum int) {
	if e.highlighter == nil || lineNum >= len(e.buffer) {
		return
	}
	
	// Ensure highlighted array is large enough
	for len(e.highlighted) <= lineNum {
		e.highlighted = append(e.highlighted, []Token{})
	}
	
	// Highlight the single line
	line := string(e.buffer[lineNum])
	e.highlighted[lineNum] = e.highlighter.HighlightLine(line, lineNum)
}

func (e *CodeEditor) rehighlightRange(start, end int) {
	if e.highlighter == nil {
		return
	}
	
	// Clamp range
	if start < 0 {
		start = 0
	}
	if end > len(e.buffer) {
		end = len(e.buffer)
	}
	
	// Get lines in range
	lines := make([]string, end-start)
	for i := start; i < end; i++ {
		if i < len(e.buffer) {
			lines[i-start] = string(e.buffer[i])
		}
	}
	
	// Highlight range
	highlighted := e.highlighter.HighlightRange(lines, start)
	
	// Ensure highlighted array is large enough
	for len(e.highlighted) < end {
		e.highlighted = append(e.highlighted, []Token{})
	}
	
	// Update highlighted lines
	for i, tokens := range highlighted {
		if start+i < len(e.highlighted) {
			e.highlighted[start+i] = tokens
		}
	}
}

// Stub methods for missing functionality
func (e *CodeEditor) insertTab() {
	// Insert 4 spaces for now
	for i := 0; i < 4; i++ {
		e.insertRune(' ')
	}
}

func (e *CodeEditor) moveCursorToLineStart() {
	e.cursorCol = 0
}

func (e *CodeEditor) moveCursorToLineEnd() {
	if e.cursorRow < len(e.buffer) {
		e.cursorCol = len(e.buffer[e.cursorRow])
	}
}

func (e *CodeEditor) pageUp() {
	e.cursorRow -= e.viewportHeight
	if e.cursorRow < 0 {
		e.cursorRow = 0
	}
	e.adjustCursorColumn()
}

func (e *CodeEditor) pageDown() {
	e.cursorRow += e.viewportHeight
	if e.cursorRow >= len(e.buffer) {
		e.cursorRow = len(e.buffer) - 1
	}
	e.adjustCursorColumn()
}

func (e *CodeEditor) deleteSelection() {
	// TODO: Implement selection deletion
}

func (e *CodeEditor) copySelection() {
	// TODO: Implement copy
}

func (e *CodeEditor) cutSelection() {
	// TODO: Implement cut
}

func (e *CodeEditor) paste() {
	// TODO: Implement paste
}

func (e *CodeEditor) selectAll() {
	// TODO: Implement select all
}

// Focus implements tview.Primitive
func (e *CodeEditor) Focus(delegate func(p tview.Primitive)) {
	// Don't call delegate(e) as that creates infinite recursion
	// The CodeEditor is the leaf primitive, nothing to delegate to
	e.hasFocus = true
}

// HasFocus implements tview.Primitive
func (e *CodeEditor) HasFocus() bool {
	return e.hasFocus
}

// Blur implements tview.Primitive
func (e *CodeEditor) Blur() {
	e.hasFocus = false
}

// Getters
func (e *CodeEditor) IsModified() bool {
	return e.modified
}

func (e *CodeEditor) GetFilePath() string {
	return e.filePath
}

func (e *CodeEditor) SetReadOnly(readOnly bool) *CodeEditor {
	e.readOnly = readOnly
	return e
}

func (e *CodeEditor) SetApp(app *tview.Application) *CodeEditor {
	e.app = app
	return e
}

func (e *CodeEditor) SetOnModified(handler func(modified bool)) *CodeEditor {
	e.onModified = handler
	return e
}

func (e *CodeEditor) SetOnCursorMove(handler func(row, col int)) *CodeEditor {
	e.onCursorMove = handler
	return e
}