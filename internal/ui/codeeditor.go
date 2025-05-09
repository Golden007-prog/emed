package ui

import (
	"fmt"
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
	tabSize      int               // Tab width in spaces (default 4)
	
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
		tabSize:     4,              // Default tab size
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
	
	// Calculate line number width (digits + 1 space padding)
	lineNumWidth := e.getLineNumberWidth() + 1
	
	// Adjust viewport and content area
	e.viewportHeight = height
	e.viewportWidth = width - lineNumWidth - 1  // Reserve space for line numbers and scrollbar
	
	// Draw line numbers and content
	for row := 0; row < height && row+e.offsetRow < len(e.buffer); row++ {
		// Draw line number
		e.drawLineNumber(screen, row+e.offsetRow+1, x, y+row, lineNumWidth-1)
		
		// Draw line content
		e.drawLine(screen, row+e.offsetRow, x+lineNumWidth, y+row, width-lineNumWidth-1)
	}
	
	// Draw vertical scrollbar
	e.drawScrollbar(screen, x+width-1, y, height)
	
	// Show cursor only when focused
	if e.hasFocus {
		if e.cursorRow >= e.offsetRow && e.cursorRow < e.offsetRow+height {
			cursorX := x + lineNumWidth + e.getVisualColumn(e.cursorRow, e.cursorCol) - e.offsetCol
			cursorY := y + e.cursorRow - e.offsetRow
			
			if cursorX >= x+lineNumWidth && cursorX < x+width-1 {
				screen.ShowCursor(cursorX, cursorY)
			}
		}
	} else {
		// Hide cursor when not focused
		screen.HideCursor()
	}
}

// drawLine draws a single line with syntax highlighting and tab expansion
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
	
	// Draw tokens with tab expansion
	screenX := x     // Position on screen
	visualCol := 0   // Visual column position (accounting for tabs)
	
	for _, token := range tokens {
		for _, ch := range token.Text {
			if ch == '\t' {
				// Calculate spaces to next tab stop
				nextTabStop := ((visualCol / e.tabSize) + 1) * e.tabSize
				spacesToAdd := nextTabStop - visualCol
				
				// Draw spaces for the tab
				for i := 0; i < spacesToAdd; i++ {
					if visualCol >= e.offsetCol && screenX < x+maxWidth {
						screen.SetContent(screenX, y, ' ', nil, token.Style)
						screenX++
					}
					visualCol++
				}
			} else {
				// Regular character
				if visualCol >= e.offsetCol && screenX < x+maxWidth {
					screen.SetContent(screenX, y, ch, nil, token.Style)
					screenX++
				}
				visualCol++
			}
			
			// Stop if we've filled the line
			if screenX >= x+maxWidth {
				return
			}
		}
	}
}

// drawScrollbar draws the vertical scrollbar
func (e *CodeEditor) drawScrollbar(screen tcell.Screen, x, y, height int) {
	totalLines := len(e.buffer)
	if totalLines == 0 {
		totalLines = 1
	}
	
	// Calculate scrollbar metrics
	visibleRatio := float64(height) / float64(totalLines)
	if visibleRatio >= 1.0 {
		// No scrollbar needed if all content fits
		for i := 0; i < height; i++ {
			screen.SetContent(x, y+i, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorDarkGray))
		}
		return
	}
	
	// Calculate thumb size and position
	thumbHeight := int(float64(height) * visibleRatio)
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	
	scrollPosition := float64(e.offsetRow) / float64(totalLines-height)
	if scrollPosition < 0 {
		scrollPosition = 0
	}
	if scrollPosition > 1 {
		scrollPosition = 1
	}
	
	thumbPosition := int(scrollPosition * float64(height-thumbHeight))
	
	// Draw scrollbar track and thumb
	for i := 0; i < height; i++ {
		if i >= thumbPosition && i < thumbPosition+thumbHeight {
			// Draw thumb
			screen.SetContent(x, y+i, '█', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite))
		} else {
			// Draw track
			screen.SetContent(x, y+i, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorDarkGray))
		}
	}
}

// InputHandler handles keyboard input
func (e *CodeEditor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		// Ignore input if not focused
		if !e.hasFocus {
			return
		}
		
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
			// Remember visual column for vertical movement
			visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
			e.cursorRow--
			// Convert visual column back to real column on new line
			e.cursorCol = e.getRealColumn(e.cursorRow, visualCol)
			e.adjustCursorColumn()
		}
	case tcell.KeyDown:
		if e.cursorRow < len(e.buffer)-1 {
			// Remember visual column for vertical movement
			visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
			e.cursorRow++
			// Convert visual column back to real column on new line
			e.cursorCol = e.getRealColumn(e.cursorRow, visualCol)
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
	
	// Horizontal scrolling with tab support
	visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
	if visualCol < e.offsetCol {
		e.offsetCol = visualCol
	} else if visualCol >= e.offsetCol+e.viewportWidth {
		e.offsetCol = visualCol - e.viewportWidth + 1
	}
}

// scrollUp scrolls the viewport up by the specified number of lines
func (e *CodeEditor) scrollUp(lines int) {
	e.offsetRow -= lines
	if e.offsetRow < 0 {
		e.offsetRow = 0
	}
}

// scrollDown scrolls the viewport down by the specified number of lines
func (e *CodeEditor) scrollDown(lines int) {
	e.offsetRow += lines
	maxOffset := len(e.buffer) - e.viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if e.offsetRow > maxOffset {
		e.offsetRow = maxOffset
	}
}

// scrollLeft scrolls the viewport left by the specified number of columns
func (e *CodeEditor) scrollLeft(cols int) {
	e.offsetCol -= cols
	if e.offsetCol < 0 {
		e.offsetCol = 0
	}
}

// scrollRight scrolls the viewport right by the specified number of columns
func (e *CodeEditor) scrollRight(cols int) {
	e.offsetCol += cols
	// Find the longest visual line to determine max offset
	maxVisualLength := 0
	for i, line := range e.buffer {
		visualLength := e.getVisualColumn(i, len(line))
		if visualLength > maxVisualLength {
			maxVisualLength = visualLength
		}
	}
	maxOffset := maxVisualLength - e.viewportWidth
	if maxOffset < 0 {
		maxOffset = 0
	}
	if e.offsetCol > maxOffset {
		e.offsetCol = maxOffset
	}
}

// getVisualColumn calculates the visual column position accounting for tabs
func (e *CodeEditor) getVisualColumn(row, col int) int {
	if row >= len(e.buffer) {
		return col
	}
	
	line := e.buffer[row]
	visualCol := 0
	
	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '\t' {
			// Tab advances to next tab stop
			visualCol = ((visualCol / e.tabSize) + 1) * e.tabSize
		} else {
			visualCol++
		}
	}
	
	return visualCol
}

// getRealColumn converts a visual column to a real buffer column
func (e *CodeEditor) getRealColumn(row, targetVisualCol int) int {
	if row >= len(e.buffer) {
		return targetVisualCol
	}
	
	line := e.buffer[row]
	visualCol := 0
	realCol := 0
	
	for realCol < len(line) && visualCol < targetVisualCol {
		if line[realCol] == '\t' {
			// Tab advances to next tab stop
			nextTabStop := ((visualCol / e.tabSize) + 1) * e.tabSize
			if nextTabStop > targetVisualCol {
				break
			}
			visualCol = nextTabStop
		} else {
			visualCol++
		}
		realCol++
	}
	
	return realCol
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

// insertTab inserts a tab character
func (e *CodeEditor) insertTab() {
	// Insert actual tab character
	e.insertRune('\t')
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

// MouseHandler handles mouse events
func (e *CodeEditor) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		// Get the position of the click relative to the box
		x, y := event.Position()
		rectX, rectY, width, height := e.GetInnerRect()
		
		// Check if event is within the editor bounds
		if x >= rectX && x < rectX+width && y >= rectY && y < rectY+height {
			// Check if mouse is on scrollbar column and button is pressed
			if x == rectX+width-1 {
				buttons := event.Buttons()
				if buttons&tcell.Button1 != 0 {
					// Mouse button is pressed on scrollbar - update scroll position
					relativeY := y - rectY
					if relativeY >= 0 && relativeY < height {
						totalLines := len(e.buffer)
						if totalLines > height {
							targetRow := (relativeY * totalLines) / height
							e.offsetRow = targetRow
							
							maxOffset := totalLines - height
							if e.offsetRow > maxOffset {
								e.offsetRow = maxOffset
							}
							if e.offsetRow < 0 {
								e.offsetRow = 0
							}
						}
					}
				}
				return true, nil // Consume all scrollbar events
			}
			
			// Handle regular editor area events
			switch action {
			case tview.MouseScrollUp:
				// Check if Alt is held for horizontal scrolling (Shift often doesn't work with scroll)
				modifiers := event.Modifiers()
				if modifiers&tcell.ModAlt != 0 {
					// Horizontal scroll left
					e.scrollLeft(5)
				} else {
					// Vertical scroll up
					e.scrollUp(1)
				}
				return true, nil
				
			case tview.MouseScrollDown:
				// Check if Alt is held for horizontal scrolling
				modifiers := event.Modifiers()
				if modifiers&tcell.ModAlt != 0 {
					// Horizontal scroll right
					e.scrollRight(5)
				} else {
					// Vertical scroll down
					e.scrollDown(1)
				}
				return true, nil
				
			case tview.MouseScrollLeft:
				// Native horizontal scroll left (if supported by terminal)
				e.scrollLeft(5)
				return true, nil
				
			case tview.MouseScrollRight:
				// Native horizontal scroll right (if supported by terminal)
				e.scrollRight(5)
				return true, nil
				
			case tview.MouseLeftClick:
				// Request focus when clicked
				setFocus(e)
				
				// Set cursor position from click
				lineNumWidth := e.getLineNumberWidth() + 1
				relativeX := x - rectX - lineNumWidth
				relativeY := y - rectY
				e.setCursorFromClick(relativeX, relativeY)
				
				return true, nil
				
			case tview.MouseLeftDoubleClick:
				// TODO: Implement word selection
				setFocus(e)
				return true, nil
				
			case tview.MouseLeftDown:
				// TODO: Start text selection
				setFocus(e)
				return true, e // Capture for drag selection
				
			case tview.MouseLeftUp:
				// TODO: End text selection
				return true, nil
				
			case tview.MouseMove:
				// TODO: Update selection during drag
				// Process mouse move regardless of focus for future features
				return true, nil
			}
		}
		
		return false, nil
	}
}

// getLineNumberWidth calculates the number of digits needed for line numbers
func (e *CodeEditor) getLineNumberWidth() int {
	totalLines := len(e.buffer)
	if totalLines == 0 {
		totalLines = 1
	}
	
	// Calculate number of digits needed
	digits := 0
	for n := totalLines; n > 0; n /= 10 {
		digits++
	}
	
	return digits
}

// drawLineNumber draws a line number at the specified position
func (e *CodeEditor) drawLineNumber(screen tcell.Screen, lineNum int, x, y, width int) {
	// Format the line number with right alignment
	numStr := fmt.Sprintf("%*d", width, lineNum)
	
	// Draw each character with gray text on very dark gray background
	style := tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.NewRGBColor(40, 40, 40))
	for i, ch := range numStr {
		if i < width {
			screen.SetContent(x+i, y, ch, nil, style)
		}
	}
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

// setCursorFromClick sets the cursor position based on a mouse click
func (e *CodeEditor) setCursorFromClick(relativeX, relativeY int) {
	// Calculate target row
	targetRow := e.offsetRow + relativeY
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= len(e.buffer) {
		targetRow = len(e.buffer) - 1
	}
	
	// Calculate target visual column
	targetVisualCol := e.offsetCol + relativeX
	if targetVisualCol < 0 {
		targetVisualCol = 0
	}
	
	// Convert visual column to real column
	if targetRow >= 0 && targetRow < len(e.buffer) {
		e.cursorRow = targetRow
		e.cursorCol = e.getRealColumn(targetRow, targetVisualCol)
		
		// Ensure cursor doesn't go past end of line
		if e.cursorCol > len(e.buffer[e.cursorRow]) {
			e.cursorCol = len(e.buffer[e.cursorRow])
		}
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}