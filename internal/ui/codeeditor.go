package ui

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"runtime"
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
	
	// Reserved for future syntax highlighting
	// Will be replaced with tree-sitter in the future
	
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
		readOnly:    false,
	}
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
	// Language detection will be added with tree-sitter
	
	// Reset cursor and viewport
	e.cursorRow = 0
	e.cursorCol = 0
	e.offsetRow = 0
	e.offsetCol = 0
	e.modified = false
	
	// Syntax highlighting will be added with tree-sitter
	
	return nil
}

// SetContent replaces the editor content with new text
func (e *CodeEditor) SetContent(content string) {
	// Detect line endings
	e.detectLineEndings(content)
	
	// Split into lines
	lines := strings.Split(content, e.lineEndings)
	
	// Convert to buffer
	e.buffer = make([][]rune, len(lines))
	for i, line := range lines {
		e.buffer[i] = []rune(line)
	}
	
	// Reset cursor position
	e.cursorRow = 0
	e.cursorCol = 0
	e.offsetRow = 0
	e.offsetCol = 0
	
	// Syntax highlighting will be added with tree-sitter
	
	// Mark as not modified since this is a fresh load
	e.modified = false
	if e.onModified != nil {
		e.onModified(false)
	}
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
	
	// Clear the background with explicit black color
	style := tcell.StyleDefault.Background(tcell.ColorBlack)
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, style)
		}
	}
	
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
	
	// Default style - white text on black background
	defaultStyle := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	
	// Get the line text
	line := e.buffer[lineNum]
	
	// Draw characters with tab expansion
	screenX := x     // Position on screen
	visualCol := 0   // Visual column position (accounting for tabs)
	realCol := 0     // Real column position in buffer
	
	for _, ch := range line {
		// Check if this position is selected
		isSelected := e.isPositionSelected(lineNum, realCol)
		
		// Apply selection background if needed
		style := defaultStyle
		if isSelected {
			style = style.Background(tcell.ColorDarkGray)
		}
		
		if ch == '\t' {
			// Calculate spaces to next tab stop
			nextTabStop := ((visualCol / e.tabSize) + 1) * e.tabSize
			spacesToAdd := nextTabStop - visualCol
			
			// Draw spaces for the tab
			for i := 0; i < spacesToAdd; i++ {
				if visualCol >= e.offsetCol && screenX < x+maxWidth {
					screen.SetContent(screenX, y, ' ', nil, style)
					screenX++
				}
				visualCol++
			}
			realCol++  // Tab counts as one character in buffer
		} else {
			// Regular character
			if visualCol >= e.offsetCol && screenX < x+maxWidth {
				screen.SetContent(screenX, y, ch, nil, style)
				screenX++
			}
			visualCol++
			realCol++
		}
		
		// Stop if we've filled the line
		if screenX >= x+maxWidth {
			return
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
			case tcell.KeyUp, tcell.KeyDown, tcell.KeyLeft, tcell.KeyRight:
				modifiers := event.Modifiers()
				// Support both Ctrl and Cmd (Meta) for macOS compatibility
				if modifiers&tcell.ModCtrl != 0 || modifiers&tcell.ModMeta != 0 {
					// Ctrl/Cmd+arrow navigation
					switch event.Key() {
					case tcell.KeyLeft:
						e.ctrlLeft()
					case tcell.KeyRight:
						e.ctrlRight()
					case tcell.KeyUp:
						e.ctrlUp()
					case tcell.KeyDown:
						e.ctrlDown()
					}
				} else if modifiers&tcell.ModAlt != 0 {
					// Alt+arrow word navigation
					switch event.Key() {
					case tcell.KeyLeft:
						e.wordLeft()
					case tcell.KeyRight:
						e.wordRight()
					default:
						e.handleNavigation(event.Key())
					}
				} else {
					e.handleNavigation(event.Key())
				}
			case tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
				e.handleNavigation(event.Key())
			}
			return
		}
		
		switch event.Key() {
		case tcell.KeyEscape:
			// ESC key pressed - could be start of Terminal.app Option sequence
			// We don't handle this directly, but tcell should convert ESC+b and ESC+f
			return
			
		case tcell.KeyRune:
			// Debug Terminal.app Option key sequences
			// Uncomment to see what's being received:
			// if event.Rune() == 'b' || event.Rune() == 'f' {
			//     fmt.Fprintf(os.Stderr, "Rune: '%c', Modifiers: %v (Alt=%v)\n", 
			//         event.Rune(), event.Modifiers(), 
			//         event.Modifiers()&tcell.ModAlt != 0)
			// }
			
			modifiers := event.Modifiers()
			
			// Debug: Log Ctrl key combinations
			// Uncomment to debug:
			// if modifiers&tcell.ModCtrl != 0 {
			//     fmt.Fprintf(os.Stderr, "DEBUG: Ctrl+Rune: '%c' (0x%x), Modifiers: %v\n", 
			//         event.Rune(), event.Rune(), modifiers)
			// }
			
			// Check for Ctrl/Cmd+C (copy) and Ctrl/Cmd+V (paste)
			// Note: Ctrl+C might come as 0x03, Ctrl+V as 0x16, Ctrl+X as 0x18, Ctrl+A as 0x01
			if modifiers&(tcell.ModCtrl|tcell.ModMeta) != 0 {
				r := event.Rune()
				// Handle both ASCII control codes and regular letters
				switch {
				case r == 3 || r == 'c' || r == 'C': // Ctrl+C
					e.copySelection()
					return
				case r == 22 || r == 'v' || r == 'V': // Ctrl+V
					e.pasteText()
					return
				case r == 24 || r == 'x' || r == 'X': // Ctrl+X
					// Cut: copy then delete selection
					if e.hasSelection {
						e.copySelection()
						e.deleteSelection()
					}
					return
				case r == 1 || r == 'a' || r == 'A': // Ctrl+A
					// Select all
					e.selectAll()
					return
				}
			}
			
			// Workaround for Terminal.app: Check if Alt/Meta modifier is set with b or f
			// This catches cases where tcell properly detects the Meta modifier
			if modifiers&(tcell.ModAlt|tcell.ModMeta) != 0 {
				switch event.Rune() {
				case 'b', 'B':
					e.wordLeft()
					return
				case 'f', 'F':
					e.wordRight()
					return
				}
			}
			
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
			modifiers := event.Modifiers()
			// Debug: Log what modifiers we're receiving
			// Uncomment to debug Terminal.app issues:
			// fmt.Fprintf(os.Stderr, "Key: %v, Modifiers: %v (Alt=%v, Ctrl=%v, Meta=%v)\n", 
			//     event.Key(), modifiers, 
			//     modifiers&tcell.ModAlt != 0,
			//     modifiers&tcell.ModCtrl != 0, 
			//     modifiers&tcell.ModMeta != 0)
			
			// Check if Shift is held for selection
			shiftHeld := modifiers&tcell.ModShift != 0
			
			// Start selection if Shift is held and we don't have one
			if shiftHeld && !e.hasSelection {
				e.startSelection()
			}
			
			// Support both Ctrl and Cmd (Meta) for macOS compatibility
			if modifiers&tcell.ModCtrl != 0 || modifiers&tcell.ModMeta != 0 {
				// Ctrl/Cmd+arrow navigation
				switch event.Key() {
				case tcell.KeyLeft:
					e.ctrlLeft()
				case tcell.KeyRight:
					e.ctrlRight()
				case tcell.KeyUp:
					e.ctrlUp()
				case tcell.KeyDown:
					e.ctrlDown()
				}
			} else if modifiers&tcell.ModAlt != 0 {
				// Alt+arrow word navigation
				switch event.Key() {
				case tcell.KeyLeft:
					e.wordLeft()
				case tcell.KeyRight:
					e.wordRight()
				default:
					e.handleNavigation(event.Key())
				}
			} else {
				// Regular navigation
				e.handleNavigation(event.Key())
			}
			
			// Update or clear selection based on Shift key
			if shiftHeld && e.hasSelection {
				e.extendSelection()
			} else if !shiftHeld && e.hasSelection {
				e.clearSelection()
			}
			
		case tcell.KeyHome:
			e.moveCursorToLineStart()
			
		case tcell.KeyEnd:
			e.moveCursorToLineEnd()
			
		case tcell.KeyPgUp:
			e.pageUp()
			
		case tcell.KeyPgDn:
			e.pageDown()
			
		// Handle Ctrl key combinations that come as special keys
		// Note: KeyCtrlC should not reach here because layout intercepts it
		case tcell.KeyCtrlC:
			e.copySelection()
			return
			
		case tcell.KeyCtrlV:
			e.pasteText()
			return
			
		case tcell.KeyCtrlX:
			if e.hasSelection {
				e.copySelection()
				e.deleteSelection()
			}
			return
			
		case tcell.KeyCtrlA:
			e.selectAll()
			return
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
	
	// Syntax highlighting will be added with tree-sitter
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
		// Add a new line at the end
		e.buffer = append(e.buffer, []rune{})
		e.cursorRow = len(e.buffer) - 1
		e.cursorCol = 0
		e.setModified(true)
		return
	}
	
	// Get current line
	currentLine := e.buffer[e.cursorRow]
	
	// Split at cursor position
	beforeCursor := make([]rune, 0)
	afterCursor := make([]rune, 0)
	
	if e.cursorCol <= len(currentLine) {
		if e.cursorCol > 0 {
			beforeCursor = make([]rune, e.cursorCol)
			copy(beforeCursor, currentLine[:e.cursorCol])
		}
		if e.cursorCol < len(currentLine) {
			afterCursor = make([]rune, len(currentLine)-e.cursorCol)
			copy(afterCursor, currentLine[e.cursorCol:])
		}
	} else {
		// Cursor is beyond line end, keep entire line
		beforeCursor = make([]rune, len(currentLine))
		copy(beforeCursor, currentLine)
		e.cursorCol = len(currentLine)
	}
	
	// Debug output
	// fmt.Fprintf(os.Stderr, "ENTER: row=%d col=%d buflen=%d\n", e.cursorRow, e.cursorCol, len(e.buffer))
	// fmt.Fprintf(os.Stderr, "  before='%s' after='%s'\n", string(beforeCursor), string(afterCursor))
	
	// Create new buffer with one more line
	newBuffer := make([][]rune, len(e.buffer)+1)
	
	// Copy lines before current row
	for i := 0; i < e.cursorRow; i++ {
		newBuffer[i] = e.buffer[i]
	}
	
	// Set the split lines
	newBuffer[e.cursorRow] = beforeCursor
	newBuffer[e.cursorRow+1] = afterCursor
	
	// Copy lines after current row
	for i := e.cursorRow + 1; i < len(e.buffer); i++ {
		newBuffer[i+1] = e.buffer[i]
	}
	
	e.buffer = newBuffer
	
	// Buffer synchronization maintained for future tree-sitter integration
	
	// Move cursor to start of new line
	e.cursorRow++
	e.cursorCol = 0
	
	// Mark as modified
	e.setModified(true)
	
	// Syntax highlighting will be added with tree-sitter
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
		// Syntax highlighting will be added with tree-sitter
	} else if e.cursorRow > 0 {
		// Join with previous line
		prevLine := e.buffer[e.cursorRow-1]
		currentLine := e.buffer[e.cursorRow]
		e.buffer[e.cursorRow-1] = append(prevLine, currentLine...)
		
		// Remove current line from buffer
		newBuffer := make([][]rune, len(e.buffer)-1)
		copy(newBuffer[:e.cursorRow], e.buffer[:e.cursorRow])
		copy(newBuffer[e.cursorRow:], e.buffer[e.cursorRow+1:])
		e.buffer = newBuffer
		
		// Buffer synchronization maintained for future tree-sitter integration
		
		// Move cursor to end of previous line
		e.cursorRow--
		e.cursorCol = len(prevLine)
		e.setModified(true)
		// Syntax highlighting will be added with tree-sitter
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
		// Syntax highlighting will be added with tree-sitter
	} else if e.cursorRow < len(e.buffer)-1 {
		// Join with next line
		nextLine := e.buffer[e.cursorRow+1]
		e.buffer[e.cursorRow] = append(line, nextLine...)
		
		// Remove next line
		newBuffer := make([][]rune, len(e.buffer)-1)
		copy(newBuffer[:e.cursorRow+1], e.buffer[:e.cursorRow+1])
		copy(newBuffer[e.cursorRow+1:], e.buffer[e.cursorRow+2:])
		e.buffer = newBuffer
		
		// Buffer synchronization maintained for future tree-sitter integration
		
		e.setModified(true)
		// Syntax highlighting will be added with tree-sitter
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

// Tree-sitter integration will be added here

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


// ctrlLeft moves cursor to first non-whitespace char or toggle to column 0
func (e *CodeEditor) ctrlLeft() {
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	line := e.buffer[e.cursorRow]
	
	// Find first non-whitespace character
	firstNonSpace := 0
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			firstNonSpace = i
			break
		}
	}
	
	if e.cursorCol == 0 {
		// At column 0, move to first non-space
		e.cursorCol = firstNonSpace
	} else if e.cursorCol == firstNonSpace {
		// At first non-space, move to column 0
		e.cursorCol = 0
	} else {
		// Anywhere else, move to first non-space
		e.cursorCol = firstNonSpace
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// ctrlRight moves cursor to end of line
func (e *CodeEditor) ctrlRight() {
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	e.cursorCol = len(e.buffer[e.cursorRow])
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// ctrlUp moves cursor to beginning of document
func (e *CodeEditor) ctrlUp() {
	e.cursorRow = 0
	e.cursorCol = 0
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// ctrlDown moves cursor to end of document
func (e *CodeEditor) ctrlDown() {
	if len(e.buffer) > 0 {
		e.cursorRow = len(e.buffer) - 1
		e.cursorCol = len(e.buffer[e.cursorRow])
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// Selection methods
func (e *CodeEditor) startSelection() {
	e.hasSelection = true
	e.selectionStart.row = e.cursorRow
	e.selectionStart.col = e.cursorCol
	e.selectionEnd.row = e.cursorRow
	e.selectionEnd.col = e.cursorCol
}

func (e *CodeEditor) extendSelection() {
	if !e.hasSelection {
		return
	}
	e.selectionEnd.row = e.cursorRow
	e.selectionEnd.col = e.cursorCol
}

func (e *CodeEditor) clearSelection() {
	e.hasSelection = false
	e.selectionStart.row = -1
	e.selectionStart.col = -1
	e.selectionEnd.row = -1
	e.selectionEnd.col = -1
}

// selectAll selects all text in the editor
func (e *CodeEditor) selectAll() {
	if len(e.buffer) == 0 {
		return
	}
	
	e.hasSelection = true
	e.selectionStart.row = 0
	e.selectionStart.col = 0
	e.selectionEnd.row = len(e.buffer) - 1
	e.selectionEnd.col = len(e.buffer[len(e.buffer)-1])
	
	// Move cursor to end
	e.cursorRow = e.selectionEnd.row
	e.cursorCol = e.selectionEnd.col
}

// deleteSelection deletes the currently selected text
func (e *CodeEditor) deleteSelection() {
	if !e.hasSelection || e.readOnly {
		return
	}
	
	// Normalize selection
	startRow, startCol := e.selectionStart.row, e.selectionStart.col
	endRow, endCol := e.selectionEnd.row, e.selectionEnd.col
	
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, endRow = endRow, startRow
		startCol, endCol = endCol, startCol
	}
	
	if startRow == endRow {
		// Single line deletion
		if startRow < len(e.buffer) {
			line := e.buffer[startRow]
			newLine := make([]rune, 0, len(line))
			if startCol > 0 {
				newLine = append(newLine, line[:startCol]...)
			}
			if endCol < len(line) {
				newLine = append(newLine, line[endCol:]...)
			}
			e.buffer[startRow] = newLine
		}
	} else {
		// Multi-line deletion
		// Combine first and last lines
		var newFirstLine []rune
		if startRow < len(e.buffer) && startCol < len(e.buffer[startRow]) {
			newFirstLine = append(newFirstLine, e.buffer[startRow][:startCol]...)
		}
		if endRow < len(e.buffer) && endCol < len(e.buffer[endRow]) {
			newFirstLine = append(newFirstLine, e.buffer[endRow][endCol:]...)
		}
		
		// Create new buffer without deleted lines
		linesToRemove := endRow - startRow
		newBuffer := make([][]rune, len(e.buffer)-linesToRemove)
		copy(newBuffer[:startRow], e.buffer[:startRow])
		newBuffer[startRow] = newFirstLine
		if endRow+1 < len(e.buffer) {
			copy(newBuffer[startRow+1:], e.buffer[endRow+1:])
		}
		e.buffer = newBuffer
		
		// Buffer synchronization maintained for future tree-sitter integration
	}
	
	// Move cursor to start of deletion
	e.cursorRow = startRow
	e.cursorCol = startCol
	e.adjustCursorColumn()
	
	// Clear selection
	e.clearSelection()
	
	// Mark as modified
	e.setModified(true)
	
	// Smart re-highlight after deletion
	// Syntax highlighting will be added with tree-sitter
}

// isPositionSelected checks if a position is within the selection
func (e *CodeEditor) isPositionSelected(row, col int) bool {
	if !e.hasSelection {
		return false
	}
	
	// Normalize selection (start should be before end)
	startRow, startCol := e.selectionStart.row, e.selectionStart.col
	endRow, endCol := e.selectionEnd.row, e.selectionEnd.col
	
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, endRow = endRow, startRow
		startCol, endCol = endCol, startCol
	}
	
	// Check if position is within selection
	if row < startRow || row > endRow {
		return false
	}
	if row == startRow && row == endRow {
		return col >= startCol && col < endCol
	}
	if row == startRow {
		return col >= startCol
	}
	if row == endRow {
		return col < endCol
	}
	return true
}

// copyToClipboard copies text to system clipboard
func (e *CodeEditor) copyToClipboard(text string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found")
		}
	default:
		return fmt.Errorf("unsupported platform")
	}
	
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// pasteFromClipboard pastes text from system clipboard
func (e *CodeEditor) pasteFromClipboard() (string, error) {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-out")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard utility found")
		}
	default:
		return "", fmt.Errorf("unsupported platform")
	}
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// copySelection copies the selected text to clipboard
func (e *CodeEditor) copySelection() {
	if !e.hasSelection {
		return
	}
	
	text := e.getSelectedText()
	if text != "" {
		e.copyToClipboard(text)
	}
}

// pasteText pastes text from clipboard at cursor position
func (e *CodeEditor) pasteText() {
	if e.readOnly {
		return
	}
	
	text, err := e.pasteFromClipboard()
	if err != nil || text == "" {
		return
	}
	
	// Delete selection if any
	if e.hasSelection {
		e.deleteSelection()
	}
	
	// Split pasted text into lines for efficient insertion
	pastedLines := strings.Split(text, "\n")
	
	if len(pastedLines) == 1 {
		// Single line paste - insert directly into current line
		line := e.buffer[e.cursorRow]
		newLine := make([]rune, 0, len(line)+len(text))
		newLine = append(newLine, line[:e.cursorCol]...)
		newLine = append(newLine, []rune(pastedLines[0])...)
		newLine = append(newLine, line[e.cursorCol:]...)
		e.buffer[e.cursorRow] = newLine
		e.cursorCol += len([]rune(pastedLines[0]))
	} else {
		// Multi-line paste - build new buffer in one go
		// First line: current line start + first pasted line
		firstLine := make([]rune, 0)
		if e.cursorRow < len(e.buffer) {
			firstLine = append(firstLine, e.buffer[e.cursorRow][:e.cursorCol]...)
		}
		firstLine = append(firstLine, []rune(pastedLines[0])...)
		
		// Last line: last pasted line + current line end
		lastLine := []rune(pastedLines[len(pastedLines)-1])
		if e.cursorRow < len(e.buffer) && e.cursorCol < len(e.buffer[e.cursorRow]) {
			lastLine = append(lastLine, e.buffer[e.cursorRow][e.cursorCol:]...)
		}
		
		// Build new buffer
		newBuffer := make([][]rune, len(e.buffer)+len(pastedLines)-1)
		
		// Copy lines before paste position
		copy(newBuffer[:e.cursorRow], e.buffer[:e.cursorRow])
		
		// Add first line
		newBuffer[e.cursorRow] = firstLine
		
		// Add middle lines
		for i := 1; i < len(pastedLines)-1; i++ {
			newBuffer[e.cursorRow+i] = []rune(pastedLines[i])
		}
		
		// Add last line
		newBuffer[e.cursorRow+len(pastedLines)-1] = lastLine
		
		// Copy lines after paste position
		if e.cursorRow+1 < len(e.buffer) {
			copy(newBuffer[e.cursorRow+len(pastedLines):], e.buffer[e.cursorRow+1:])
		}
		
		e.buffer = newBuffer
		
		// Update cursor position
		e.cursorRow = e.cursorRow + len(pastedLines) - 1
		e.cursorCol = len([]rune(pastedLines[len(pastedLines)-1]))
		
		// Buffer synchronization maintained for future tree-sitter integration
	}
	
	// Mark as modified
	e.setModified(true)
	
	// Smart re-highlight: find the context needed
	// Syntax highlighting will be added with tree-sitter
}

// getSelectedText returns the currently selected text
func (e *CodeEditor) getSelectedText() string {
	if !e.hasSelection {
		return ""
	}
	
	// Normalize selection
	startRow, startCol := e.selectionStart.row, e.selectionStart.col
	endRow, endCol := e.selectionEnd.row, e.selectionEnd.col
	
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, endRow = endRow, startRow
		startCol, endCol = endCol, startCol
	}
	
	// Extract text
	var result []rune
	
	if startRow == endRow {
		// Single line selection
		if startRow < len(e.buffer) && startCol < len(e.buffer[startRow]) {
			line := e.buffer[startRow]
			if endCol > len(line) {
				endCol = len(line)
			}
			result = append(result, line[startCol:endCol]...)
		}
	} else {
		// Multi-line selection
		for row := startRow; row <= endRow && row < len(e.buffer); row++ {
			line := e.buffer[row]
			if row == startRow {
				// First line: from startCol to end
				if startCol < len(line) {
					result = append(result, line[startCol:]...)
				}
				result = append(result, '\n')
			} else if row == endRow {
				// Last line: from start to endCol
				if endCol > len(line) {
					endCol = len(line)
				}
				result = append(result, line[:endCol]...)
			} else {
				// Middle lines: entire line
				result = append(result, line...)
				result = append(result, '\n')
			}
		}
	}
	
	return string(result)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isWordChar checks if a rune is a word character
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

// wordLeft moves cursor to beginning of current/previous word
func (e *CodeEditor) wordLeft() {
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	line := e.buffer[e.cursorRow]
	
	// If we're past the line end, move to line end
	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}
	
	// If at beginning of line, move to previous line
	if e.cursorCol == 0 {
		if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = len(e.buffer[e.cursorRow])
		}
	} else {
		// Move left one position first
		e.cursorCol--
		
		// Skip any non-word characters
		for e.cursorCol > 0 && !isWordChar(line[e.cursorCol]) {
			e.cursorCol--
		}
		
		// Skip word characters to find beginning of word
		for e.cursorCol > 0 && isWordChar(line[e.cursorCol-1]) {
			e.cursorCol--
		}
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// wordRight moves cursor to end of current/next word
func (e *CodeEditor) wordRight() {
	if e.cursorRow >= len(e.buffer) {
		return
	}
	
	line := e.buffer[e.cursorRow]
	
	// If at end of line, move to next line
	if e.cursorCol >= len(line) {
		if e.cursorRow < len(e.buffer)-1 {
			e.cursorRow++
			e.cursorCol = 0
		}
	} else {
		// Skip current word if we're in one
		for e.cursorCol < len(line) && isWordChar(line[e.cursorCol]) {
			e.cursorCol++
		}
		
		// Skip any non-word characters
		for e.cursorCol < len(line) && !isWordChar(line[e.cursorCol]) {
			e.cursorCol++
		}
	}
	
	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
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
				// Start text selection
				setFocus(e)
				
				// Clear any existing selection
				e.clearSelection()
				
				// Set cursor position from mouse
				lineNumWidth := e.getLineNumberWidth() + 1
				mouseX, mouseY := event.Position()
				clickX := mouseX - x - lineNumWidth
				clickY := mouseY - y
				
				if clickX >= 0 && clickY >= 0 {
					row := e.offsetRow + clickY
					if row < len(e.buffer) {
						// Convert visual column to real column
						col := e.getRealColumn(row, e.offsetCol + clickX)
						e.cursorRow = row
						e.cursorCol = col
						e.adjustCursorColumn()
						
						// Start selection at this position
						e.startSelection()
					}
				}
				
				return true, e // Capture for drag selection
				
			case tview.MouseLeftUp:
				// End text selection
				return true, nil
				
			case tview.MouseMove:
				// Update selection during drag
				if e.hasSelection {
					lineNumWidth := e.getLineNumberWidth() + 1
					mouseX, mouseY := event.Position()
					clickX := mouseX - x - lineNumWidth
					clickY := mouseY - y
					
					if clickX >= 0 && clickY >= 0 {
						row := e.offsetRow + clickY
						if row < len(e.buffer) {
							// Convert visual column to real column
							col := e.getRealColumn(row, e.offsetCol + clickX)
							e.cursorRow = row
							e.cursorCol = col
							e.adjustCursorColumn()
							
							// Extend selection to this position
							e.extendSelection()
						}
					}
				}
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