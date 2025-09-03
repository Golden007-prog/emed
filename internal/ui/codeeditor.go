package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/core/buffer"
	"github.com/laborin/emed/internal/core/clipboard"
	"github.com/laborin/emed/internal/core/selection"
	"github.com/laborin/emed/internal/core/syntax"
	"github.com/rivo/tview"
)

// CodeEditor is a custom primitive for code editing with syntax highlighting
type CodeEditor struct {
	*tview.Box

	// Core components
	buffer      *buffer.TextBuffer
	selection   *selection.Selection
	clipboard   *clipboard.Clipboard
	highlighter *syntax.Highlighter

	// Focus state
	hasFocus bool

	// Cursor position
	cursorRow int
	cursorCol int

	// Viewport (for scrolling)
	offsetRow      int // First visible row
	offsetCol      int // First visible column
	viewportHeight int // Number of visible rows
	viewportWidth  int // Number of visible columns

	// File info
	filePath string
	readOnly bool

	// Callbacks
	onModified   func(modified bool)
	onCursorMove func(row, col int)

	// Application reference
	app *tview.Application
}

// NewCodeEditor creates a new code editor
func NewCodeEditor() *CodeEditor {
	return &CodeEditor{
		Box:       tview.NewBox(),
		buffer:    buffer.New(),
		selection: selection.New(),
		clipboard: clipboard.New(),
		readOnly:  false,
	}
}

// LoadFile loads a file into the editor
func (e *CodeEditor) LoadFile(path string) error {
	if err := e.buffer.LoadFile(path); err != nil {
		return err
	}

	e.filePath = path
	e.cursorRow = 0
	e.cursorCol = 0
	e.offsetRow = 0
	e.offsetCol = 0
	e.selection.Clear()

	// Initialize syntax highlighter
	if e.highlighter != nil {
		e.highlighter.Close()
	}
	e.highlighter = syntax.NewHighlighter(path)
	if e.highlighter != nil {
		// Convert buffer content to lines for the highlighter
		content := e.buffer.GetContent()
		lines := strings.Split(content, "\n")
		e.highlighter.UpdateContent(lines)
	}

	return nil
}

// SetContent replaces the editor content with new text
func (e *CodeEditor) SetContent(content string) {
	e.buffer.SetContent(content)
	e.cursorRow = 0
	e.cursorCol = 0
	e.offsetRow = 0
	e.offsetCol = 0
	e.selection.Clear()

	if e.onModified != nil {
		e.onModified(false)
	}
}

// SaveFile saves the editor content to a file
func (e *CodeEditor) SaveFile(path string) error {
	if path == "" {
		path = e.filePath
	}

	if err := e.buffer.SaveFile(path); err != nil {
		return err
	}

	// Update highlighter if file extension changed
	if path != e.filePath {
		if e.highlighter != nil {
			e.highlighter.Close()
		}
		e.highlighter = syntax.NewHighlighter(path)
		if e.highlighter != nil {
			content := e.buffer.GetContent()
			lines := strings.Split(content, "\n")
			e.highlighter.UpdateContent(lines)
		}
	}

	e.filePath = path

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
	e.viewportWidth = width - lineNumWidth - 1 // Reserve space for line numbers and scrollbar

	// Update highlighter viewport for lazy tokenization
	if e.highlighter != nil {
		e.highlighter.UpdateViewport(e.offsetRow, e.offsetRow+height-1)
	}

	// Draw line numbers and content
	lines := e.buffer.GetLines()
	for row := 0; row < height && row+e.offsetRow < len(lines); row++ {
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
	line := e.buffer.GetLine(lineNum)
	if len(line) == 0 {
		return
	}

	// Default style - white text on black background
	defaultStyle := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)

	// Get syntax highlighting for this line if available
	var tokens []syntax.TokenStyle
	if e.highlighter != nil {
		tokens = e.highlighter.GetLineTokens(lineNum)
	}

	// Helper function to get style for a column position
	getStyleForColumn := func(col int) tcell.Style {
		for _, token := range tokens {
			if col >= token.Start && col < token.End {
				return token.Style
			}
		}
		return defaultStyle
	}

	// Draw characters with tab expansion
	screenX := x    // Position on screen
	visualCol := 0  // Visual column position (accounting for tabs)
	realCol := 0    // Real column position in buffer

	for _, ch := range line {
		// Get base style from syntax highlighting
		style := getStyleForColumn(realCol)

		// Check if this position is selected
		isSelected := e.selection.IsPositionSelected(lineNum, realCol)
		if isSelected {
			// Preserve foreground color from syntax highlighting, change background
			fg, _, _ := style.Decompose()
			style = tcell.StyleDefault.Background(tcell.ColorDarkGray).Foreground(fg)
		}

		if ch == '\t' {
			// Calculate spaces to next tab stop
			tabSize := e.buffer.GetTabSize()
			nextTabStop := ((visualCol / tabSize) + 1) * tabSize
			spacesToAdd := nextTabStop - visualCol

			// Draw spaces for the tab
			for i := 0; i < spacesToAdd; i++ {
				if visualCol >= e.offsetCol && screenX < x+maxWidth {
					screen.SetContent(screenX, y, ' ', nil, style)
					screenX++
				}
				visualCol++
			}
			realCol++ // Tab counts as one character in buffer
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
	totalLines := e.buffer.GetLineCount()
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
				e.handleNavigationWithModifiers(event)
			case tcell.KeyHome, tcell.KeyEnd, tcell.KeyPgUp, tcell.KeyPgDn:
				e.handleNavigation(event.Key())
			}
			return
		}

		switch event.Key() {
		case tcell.KeyEscape:
			return

		case tcell.KeyRune:
			modifiers := event.Modifiers()

			// Handle Ctrl/Cmd key combinations
			if modifiers&(tcell.ModCtrl|tcell.ModMeta) != 0 {
				r := event.Rune()
				switch {
				case r == 3 || r == 'c' || r == 'C': // Ctrl+C
					e.copySelection()
					return
				case r == 22 || r == 'v' || r == 'V': // Ctrl+V
					e.pasteText()
					return
				case r == 24 || r == 'x' || r == 'X': // Ctrl+X
					if e.selection.IsActive() {
						e.copySelection()
						e.deleteSelection()
					}
					return
				case r == 1 || r == 'a' || r == 'A': // Ctrl+A
					e.selectAll()
					return
				}
			}

			// Handle Alt/Meta word navigation
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
			e.handleNavigationWithModifiers(event)

		case tcell.KeyHome:
			e.moveCursorToLineStart()

		case tcell.KeyEnd:
			e.moveCursorToLineEnd()

		case tcell.KeyPgUp:
			e.pageUp()

		case tcell.KeyPgDn:
			e.pageDown()

		// Handle Ctrl key combinations that come as special keys
		case tcell.KeyCtrlC:
			e.copySelection()
			return

		case tcell.KeyCtrlV:
			e.pasteText()
			return

		case tcell.KeyCtrlX:
			if e.selection.IsActive() {
				e.copySelection()
				e.deleteSelection()
			}
			return

		case tcell.KeyCtrlA:
			e.selectAll()
			return

		case tcell.KeyCtrlZ:
			e.Undo()
			return

		case tcell.KeyCtrlY:
			e.Redo()
			return
		}

		// Update viewport to follow cursor
		e.ensureCursorVisible()
	}
}

// handleNavigationWithModifiers handles navigation with modifier keys
func (e *CodeEditor) handleNavigationWithModifiers(event *tcell.EventKey) {
	modifiers := event.Modifiers()
	shiftHeld := modifiers&tcell.ModShift != 0

	// Start selection if Shift is held and we don't have one
	if shiftHeld && !e.selection.IsActive() {
		e.selection.Start(e.cursorRow, e.cursorCol)
	}

	// Handle Ctrl/Cmd+arrow navigation
	if modifiers&tcell.ModCtrl != 0 || modifiers&tcell.ModMeta != 0 {
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
	if shiftHeld && e.selection.IsActive() {
		e.selection.Extend(e.cursorRow, e.cursorCol)
	} else if !shiftHeld && e.selection.IsActive() {
		e.selection.Clear()
	}
}

// insertRune inserts a character at the cursor position
func (e *CodeEditor) insertRune(r rune) {
	if e.readOnly {
		return
	}

	// Save state for undo before making changes
	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	// Delete selection first if any
	if e.selection.IsActive() {
		e.deleteSelection()
	}

	// Insert the rune
	e.buffer.InsertRune(e.cursorRow, e.cursorCol, r)
	e.cursorCol++

	// Mark as modified
	e.setModified(true)
}

// insertNewline inserts a new line at the cursor position
func (e *CodeEditor) insertNewline() {
	if e.readOnly {
		return
	}

	// Save state for undo before making changes
	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	// Delete selection first if any
	if e.selection.IsActive() {
		e.deleteSelection()
	}

	// Insert newline and update cursor position
	newRow, newCol := e.buffer.InsertNewline(e.cursorRow, e.cursorCol)
	e.cursorRow = newRow
	e.cursorCol = newCol

	// Mark as modified
	e.setModified(true)
}

// deleteCharBackward deletes the character before the cursor
func (e *CodeEditor) deleteCharBackward() {
	if e.readOnly {
		return
	}

	// Save state for undo before making changes
	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	// Delete selection if any
	if e.selection.IsActive() {
		e.deleteSelection()
		return
	}

	newRow, newCol, deleted := e.buffer.DeleteCharBackward(e.cursorRow, e.cursorCol)
	if deleted {
		e.cursorRow = newRow
		e.cursorCol = newCol
		e.setModified(true)
	}
}

// deleteCharForward deletes the character at the cursor
func (e *CodeEditor) deleteCharForward() {
	if e.readOnly {
		return
	}

	// Save state for undo before making changes
	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	// Delete selection if any
	if e.selection.IsActive() {
		e.deleteSelection()
		return
	}

	if e.buffer.DeleteCharForward(e.cursorRow, e.cursorCol) {
		e.setModified(true)
	}
}

// Navigation methods
func (e *CodeEditor) handleNavigation(key tcell.Key) {
	switch key {
	case tcell.KeyUp:
		if e.cursorRow > 0 {
			visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
			e.cursorRow--
			e.cursorCol = e.getRealColumn(e.cursorRow, visualCol)
			e.adjustCursorColumn()
		}
	case tcell.KeyDown:
		if e.cursorRow < e.buffer.GetLineCount()-1 {
			visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
			e.cursorRow++
			e.cursorCol = e.getRealColumn(e.cursorRow, visualCol)
			e.adjustCursorColumn()
		}
	case tcell.KeyLeft:
		if e.cursorCol > 0 {
			e.cursorCol--
		} else if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
		}
	case tcell.KeyRight:
		lineLength := e.buffer.GetLineLength(e.cursorRow)
		if e.cursorCol < lineLength {
			e.cursorCol++
		} else if e.cursorRow < e.buffer.GetLineCount()-1 {
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
	lineLen := e.buffer.GetLineLength(e.cursorRow)
	if e.cursorCol > lineLen {
		e.cursorCol = lineLen
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

// Scrolling methods
func (e *CodeEditor) scrollUp(lines int) {
	e.offsetRow -= lines
	if e.offsetRow < 0 {
		e.offsetRow = 0
	}
	// Update highlighter viewport when scrolling
	if e.highlighter != nil && e.viewportHeight > 0 {
		e.highlighter.UpdateViewport(e.offsetRow, e.offsetRow+e.viewportHeight-1)
	}
}

func (e *CodeEditor) scrollDown(lines int) {
	e.offsetRow += lines
	maxOffset := e.buffer.GetLineCount() - e.viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if e.offsetRow > maxOffset {
		e.offsetRow = maxOffset
	}
	// Update highlighter viewport when scrolling
	if e.highlighter != nil && e.viewportHeight > 0 {
		e.highlighter.UpdateViewport(e.offsetRow, e.offsetRow+e.viewportHeight-1)
	}
}

func (e *CodeEditor) scrollLeft(cols int) {
	e.offsetCol -= cols
	if e.offsetCol < 0 {
		e.offsetCol = 0
	}
}

func (e *CodeEditor) scrollRight(cols int) {
	e.offsetCol += cols
	// Find the longest visual line to determine max offset
	maxVisualLength := 0
	lines := e.buffer.GetLines()
	for i, line := range lines {
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
	line := e.buffer.GetLine(row)
	visualCol := 0
	tabSize := e.buffer.GetTabSize()

	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '\t' {
			// Tab advances to next tab stop
			visualCol = ((visualCol / tabSize) + 1) * tabSize
		} else {
			visualCol++
		}
	}

	return visualCol
}

// getRealColumn converts a visual column to a real buffer column
func (e *CodeEditor) getRealColumn(row, targetVisualCol int) int {
	line := e.buffer.GetLine(row)
	visualCol := 0
	realCol := 0
	tabSize := e.buffer.GetTabSize()

	for realCol < len(line) && visualCol < targetVisualCol {
		if line[realCol] == '\t' {
			// Tab advances to next tab stop
			nextTabStop := ((visualCol / tabSize) + 1) * tabSize
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
	e.buffer.SetModified(modified)
	if e.onModified != nil {
		e.onModified(modified)
	}
	// Update syntax highlighting after modifications
	if modified && e.highlighter != nil {
		content := e.buffer.GetContent()
		lines := strings.Split(content, "\n")
		e.highlighter.UpdateContent(lines)
	}
}

// insertTab inserts a tab character
func (e *CodeEditor) insertTab() {
	e.insertRune('\t')
}

func (e *CodeEditor) moveCursorToLineStart() {
	e.cursorCol = 0
}

func (e *CodeEditor) moveCursorToLineEnd() {
	e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
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
	if e.cursorRow >= e.buffer.GetLineCount() {
		e.cursorRow = e.buffer.GetLineCount() - 1
	}
	e.adjustCursorColumn()
}

// ctrlLeft moves cursor to first non-whitespace char or toggle to column 0
func (e *CodeEditor) ctrlLeft() {
	line := e.buffer.GetLine(e.cursorRow)

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
	e.cursorCol = e.buffer.GetLineLength(e.cursorRow)

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
	if e.buffer.GetLineCount() > 0 {
		e.cursorRow = e.buffer.GetLineCount() - 1
		e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// selectAll selects all text in the editor
func (e *CodeEditor) selectAll() {
	if e.buffer.GetLineCount() == 0 {
		return
	}

	lastRow := e.buffer.GetLineCount() - 1
	lastCol := e.buffer.GetLineLength(lastRow)
	e.selection.SelectAll(e.buffer.GetLineCount(), lastCol)

	// Move cursor to end
	e.cursorRow = lastRow
	e.cursorCol = lastCol
}

// deleteSelection deletes the currently selected text
func (e *CodeEditor) deleteSelection() {
	if !e.selection.IsActive() || e.readOnly {
		return
	}

	startRow, startCol, endRow, endCol := e.selection.GetNormalizedRange()
	e.buffer.DeleteRange(startRow, startCol, endRow, endCol)

	// Move cursor to start of deletion
	e.cursorRow = startRow
	e.cursorCol = startCol
	e.adjustCursorColumn()

	// Clear selection
	e.selection.Clear()

	// Mark as modified
	e.setModified(true)
}

// copySelection copies the selected text to clipboard
func (e *CodeEditor) copySelection() {
	if !e.selection.IsActive() {
		return
	}

	startRow, startCol, endRow, endCol := e.selection.GetNormalizedRange()
	text := e.buffer.GetTextRange(startRow, startCol, endRow, endCol)
	if text != "" {
		e.clipboard.Copy(text)
	}
}

// pasteText pastes text from clipboard at cursor position
func (e *CodeEditor) pasteText() {
	if e.readOnly {
		return
	}

	text, err := e.clipboard.Paste()
	if err != nil || text == "" {
		return
	}

	// Save state for undo before making changes
	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	// Delete selection if any
	if e.selection.IsActive() {
		e.deleteSelection()
	}

	// Insert pasted text and update cursor position
	newRow, newCol := e.buffer.InsertText(e.cursorRow, e.cursorCol, text)
	e.cursorRow = newRow
	e.cursorCol = newCol

	// Mark as modified
	e.setModified(true)
}

// isWordChar checks if a rune is a word character
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

// wordLeft moves cursor to beginning of current/previous word
func (e *CodeEditor) wordLeft() {
	line := e.buffer.GetLine(e.cursorRow)

	// If we're past the line end, move to line end
	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}

	// If at beginning of line, move to previous line
	if e.cursorCol == 0 {
		if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
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
	line := e.buffer.GetLine(e.cursorRow)

	// If at end of line, move to next line
	if e.cursorCol >= len(line) {
		if e.cursorRow < e.buffer.GetLineCount()-1 {
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
						totalLines := e.buffer.GetLineCount()
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
				modifiers := event.Modifiers()
				if modifiers&tcell.ModAlt != 0 {
					e.scrollLeft(5)
				} else {
					e.scrollUp(1)
				}
				return true, nil

			case tview.MouseScrollDown:
				modifiers := event.Modifiers()
				if modifiers&tcell.ModAlt != 0 {
					e.scrollRight(5)
				} else {
					e.scrollDown(1)
				}
				return true, nil

			case tview.MouseScrollLeft:
				e.scrollLeft(5)
				return true, nil

			case tview.MouseScrollRight:
				e.scrollRight(5)
				return true, nil

			case tview.MouseLeftClick:
				setFocus(e)
				lineNumWidth := e.getLineNumberWidth() + 1
				relativeX := x - rectX - lineNumWidth
				relativeY := y - rectY
				e.setCursorFromClick(relativeX, relativeY)
				return true, nil

			case tview.MouseLeftDown:
				setFocus(e)
				e.selection.Clear()

				lineNumWidth := e.getLineNumberWidth() + 1
				mouseX, mouseY := event.Position()
				clickX := mouseX - rectX - lineNumWidth
				clickY := mouseY - rectY

				if clickX >= 0 && clickY >= 0 {
					row := e.offsetRow + clickY
					if row < e.buffer.GetLineCount() {
						col := e.getRealColumn(row, e.offsetCol+clickX)
						e.cursorRow = row
						e.cursorCol = col
						e.adjustCursorColumn()

						e.selection.StartMouseDrag(e.cursorRow, e.cursorCol)
					}
				}

				return true, e // Capture for drag selection

			case tview.MouseLeftUp:
				e.selection.StopMouseDrag()
				return true, nil

			case tview.MouseMove:
				if e.selection.IsMouseDragging() {
					lineNumWidth := e.getLineNumberWidth() + 1
					mouseX, mouseY := event.Position()
					clickX := mouseX - rectX - lineNumWidth
					clickY := mouseY - rectY

					if clickX >= 0 && clickY >= 0 {
						row := e.offsetRow + clickY
						if row < e.buffer.GetLineCount() {
							col := e.getRealColumn(row, e.offsetCol+clickX)
							e.cursorRow = row
							e.cursorCol = col
							e.adjustCursorColumn()

							e.selection.ExtendMouse(e.cursorRow, e.cursorCol)
						}
					}
					return true, nil // Only consume if we're actually dragging
				}
				return false, nil // Don't consume if not dragging
			}
		}

		return false, nil
	}
}

// getLineNumberWidth calculates the number of digits needed for line numbers
func (e *CodeEditor) getLineNumberWidth() int {
	totalLines := e.buffer.GetLineCount()
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

// setCursorFromClick sets the cursor position based on a mouse click
func (e *CodeEditor) setCursorFromClick(relativeX, relativeY int) {
	// Calculate target row
	targetRow := e.offsetRow + relativeY
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= e.buffer.GetLineCount() {
		targetRow = e.buffer.GetLineCount() - 1
	}

	// Calculate target visual column
	targetVisualCol := e.offsetCol + relativeX
	if targetVisualCol < 0 {
		targetVisualCol = 0
	}

	// Convert visual column to real column
	if targetRow >= 0 && targetRow < e.buffer.GetLineCount() {
		e.cursorRow = targetRow
		e.cursorCol = e.getRealColumn(targetRow, targetVisualCol)

		// Ensure cursor doesn't go past end of line
		if e.cursorCol > e.buffer.GetLineLength(e.cursorRow) {
			e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
		}
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// Getters and setters to maintain compatibility
func (e *CodeEditor) IsModified() bool {
	return e.buffer.IsModified()
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

// Undo reverts the last edit operation
func (e *CodeEditor) Undo() {
	if e.readOnly {
		return
	}

	// Perform undo and get the restored cursor position
	cursorX, cursorY, success := e.buffer.Undo()
	if success {
		e.cursorCol = cursorX
		e.cursorRow = cursorY
		e.selection.Clear()
		e.ensureCursorVisible()
		
		// Mark as modified (undo still changes the buffer)
		e.setModified(true)
		
		// The display will be refreshed automatically by tview
	}
}

// Redo reapplies a previously undone edit operation  
func (e *CodeEditor) Redo() {
	if e.readOnly {
		return
	}

	// Perform redo and get the restored cursor position
	cursorX, cursorY, success := e.buffer.Redo()
	if success {
		e.cursorCol = cursorX
		e.cursorRow = cursorY
		e.selection.Clear()
		e.ensureCursorVisible()
		
		// Mark as modified
		e.setModified(true)
		
		// The display will be refreshed automatically by tview
	}
}

// Close cleans up resources
func (e *CodeEditor) Close() {
	if e.highlighter != nil {
		e.highlighter.Close()
		e.highlighter = nil
	}
}
