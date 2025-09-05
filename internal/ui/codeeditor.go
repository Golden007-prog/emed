// Package ui provides the main user interface components for the emed text editor.
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

// CodeEditor provides text editing functionality with syntax highlighting.
type CodeEditor struct {
	*tview.Box

	buffer      *buffer.TextBuffer
	selection   *selection.Selection
	clipboard   *clipboard.Clipboard
	highlighter *syntax.Highlighter

	hasFocus bool

	cursorRow int
	cursorCol int

	offsetRow      int
	offsetCol      int
	viewportHeight int
	viewportWidth  int

	filePath string
	readOnly bool

	onModified   func(modified bool)
	onCursorMove func(row, col int)

	app *tview.Application
}

// NewCodeEditor creates a new code editor instance with default settings.
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
// Draw renders the code editor to the screen.
func (e *CodeEditor) Draw(screen tcell.Screen) {
	e.Box.DrawForSubclass(screen, e)
	x, y, width, height := e.GetInnerRect()

	style := tcell.StyleDefault.Background(tcell.ColorBlack)
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, style)
		}
	}

	lineNumWidth := e.getLineNumberWidth() + 1

	e.viewportHeight = height
	e.viewportWidth = width - lineNumWidth - 1

	if e.highlighter != nil {
		e.highlighter.UpdateViewport(e.offsetRow, e.offsetRow+height-1)
	}

	lines := e.buffer.GetLines()
	for row := 0; row < height && row+e.offsetRow < len(lines); row++ {
		e.drawLineNumber(screen, row+e.offsetRow+1, x, y+row, lineNumWidth-1)

		e.drawLine(screen, row+e.offsetRow, x+lineNumWidth, y+row, width-lineNumWidth-1)
	}

	e.drawScrollbar(screen, x+width-1, y, height)

	if e.hasFocus {
		if e.cursorRow >= e.offsetRow && e.cursorRow < e.offsetRow+height {
			cursorX := x + lineNumWidth + e.getVisualColumn(e.cursorRow, e.cursorCol) - e.offsetCol
			cursorY := y + e.cursorRow - e.offsetRow

			if cursorX >= x+lineNumWidth && cursorX < x+width-1 {
				screen.ShowCursor(cursorX, cursorY)
			}
		}
	} else {
		screen.HideCursor()
	}
}

func (e *CodeEditor) drawLine(screen tcell.Screen, lineNum int, x, y, maxWidth int) {
	line := e.buffer.GetLine(lineNum)
	if len(line) == 0 {
		return
	}

	defaultStyle := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)

	var tokens []syntax.TokenStyle
	if e.highlighter != nil {
		tokens = e.highlighter.GetLineTokens(lineNum)
	}

	getStyleForColumn := func(col int) tcell.Style {
		for _, token := range tokens {
			if col >= token.Start && col < token.End {
				return token.Style
			}
		}
		return defaultStyle
	}

	screenX := x    // Position on screen
	visualCol := 0  // Visual column position (accounting for tabs)
	realCol := 0    // Real column position in buffer

	for _, ch := range line {
		style := getStyleForColumn(realCol)

		isSelected := e.selection.IsPositionSelected(lineNum, realCol)
		if isSelected {
			fg, _, _ := style.Decompose()
			style = tcell.StyleDefault.Background(tcell.ColorDarkGray).Foreground(fg)
		}

		if ch == '\t' {
			tabSize := e.buffer.GetTabSize()
			nextTabStop := ((visualCol / tabSize) + 1) * tabSize
			spacesToAdd := nextTabStop - visualCol

			for i := 0; i < spacesToAdd; i++ {
				if visualCol >= e.offsetCol && screenX < x+maxWidth {
					screen.SetContent(screenX, y, ' ', nil, style)
					screenX++
				}
				visualCol++
			}
			realCol++ // Tab counts as one character in buffer
		} else {
			if visualCol >= e.offsetCol && screenX < x+maxWidth {
				screen.SetContent(screenX, y, ch, nil, style)
				screenX++
			}
			visualCol++
			realCol++
		}

		if screenX >= x+maxWidth {
			return
		}
	}
}

func (e *CodeEditor) drawScrollbar(screen tcell.Screen, x, y, height int) {
	totalLines := e.buffer.GetLineCount()
	if totalLines == 0 {
		totalLines = 1
	}

	visibleRatio := float64(height) / float64(totalLines)
	if visibleRatio >= 1.0 {
		for i := 0; i < height; i++ {
			screen.SetContent(x, y+i, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorDarkGray))
		}
		return
	}

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

	for i := 0; i < height; i++ {
		if i >= thumbPosition && i < thumbPosition+thumbHeight {
			screen.SetContent(x, y+i, '█', nil, tcell.StyleDefault.Foreground(tcell.ColorWhite))
		} else {
			screen.SetContent(x, y+i, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorDarkGray))
		}
	}
}

// InputHandler returns the input handler for keyboard events.
func (e *CodeEditor) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !e.hasFocus {
			return
		}

		if e.readOnly {
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

		e.ensureCursorVisible()
	}
}

func (e *CodeEditor) handleNavigationWithModifiers(event *tcell.EventKey) {
	modifiers := event.Modifiers()
	shiftHeld := modifiers&tcell.ModShift != 0

	if shiftHeld && !e.selection.IsActive() {
		e.selection.Start(e.cursorRow, e.cursorCol)
	}

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

	if shiftHeld && e.selection.IsActive() {
		e.selection.Extend(e.cursorRow, e.cursorCol)
	} else if !shiftHeld && e.selection.IsActive() {
		e.selection.Clear()
	}
}

func (e *CodeEditor) insertRune(r rune) {
	if e.readOnly {
		return
	}

	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	if e.selection.IsActive() {
		e.deleteSelection()
	}

	e.buffer.InsertRune(e.cursorRow, e.cursorCol, r)
	e.cursorCol++

	e.setModified(true)
}

func (e *CodeEditor) insertNewline() {
	if e.readOnly {
		return
	}

	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	if e.selection.IsActive() {
		e.deleteSelection()
	}

	newRow, newCol := e.buffer.InsertNewline(e.cursorRow, e.cursorCol)
	e.cursorRow = newRow
	e.cursorCol = newCol

	e.setModified(true)
}

func (e *CodeEditor) deleteCharBackward() {
	if e.readOnly {
		return
	}

	e.buffer.SaveState(e.cursorRow, e.cursorCol)

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

func (e *CodeEditor) deleteCharForward() {
	if e.readOnly {
		return
	}

	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	if e.selection.IsActive() {
		e.deleteSelection()
		return
	}

	if e.buffer.DeleteCharForward(e.cursorRow, e.cursorCol) {
		e.setModified(true)
	}
}

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

func (e *CodeEditor) scrollUp(lines int) {
	e.offsetRow -= lines
	if e.offsetRow < 0 {
		e.offsetRow = 0
	}
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

func (e *CodeEditor) getVisualColumn(row, col int) int {
	line := e.buffer.GetLine(row)
	visualCol := 0
	tabSize := e.buffer.GetTabSize()

	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '\t' {
			visualCol = ((visualCol / tabSize) + 1) * tabSize
		} else {
			visualCol++
		}
	}

	return visualCol
}

func (e *CodeEditor) getRealColumn(row, targetVisualCol int) int {
	line := e.buffer.GetLine(row)
	visualCol := 0
	realCol := 0
	tabSize := e.buffer.GetTabSize()

	for realCol < len(line) && visualCol < targetVisualCol {
		if line[realCol] == '\t' {
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
	if modified && e.highlighter != nil {
		content := e.buffer.GetContent()
		lines := strings.Split(content, "\n")
		e.highlighter.UpdateContent(lines)
	}
}

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

func (e *CodeEditor) ctrlLeft() {
	line := e.buffer.GetLine(e.cursorRow)

	firstNonSpace := 0
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			firstNonSpace = i
			break
		}
	}

	if e.cursorCol == 0 {
		e.cursorCol = firstNonSpace
	} else if e.cursorCol == firstNonSpace {
		e.cursorCol = 0
	} else {
		e.cursorCol = firstNonSpace
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

func (e *CodeEditor) ctrlRight() {
	e.cursorCol = e.buffer.GetLineLength(e.cursorRow)

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

func (e *CodeEditor) ctrlUp() {
	e.cursorRow = 0
	e.cursorCol = 0

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

func (e *CodeEditor) ctrlDown() {
	if e.buffer.GetLineCount() > 0 {
		e.cursorRow = e.buffer.GetLineCount() - 1
		e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

func (e *CodeEditor) selectAll() {
	if e.buffer.GetLineCount() == 0 {
		return
	}

	lastRow := e.buffer.GetLineCount() - 1
	lastCol := e.buffer.GetLineLength(lastRow)
	e.selection.SelectAll(e.buffer.GetLineCount(), lastCol)

	e.cursorRow = lastRow
	e.cursorCol = lastCol
}

func (e *CodeEditor) deleteSelection() {
	if !e.selection.IsActive() || e.readOnly {
		return
	}

	startRow, startCol, endRow, endCol := e.selection.GetNormalizedRange()
	e.buffer.DeleteRange(startRow, startCol, endRow, endCol)

	e.cursorRow = startRow
	e.cursorCol = startCol
	e.adjustCursorColumn()

	e.selection.Clear()

	e.setModified(true)
}

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

func (e *CodeEditor) pasteText() {
	if e.readOnly {
		return
	}

	text, err := e.clipboard.Paste()
	if err != nil || text == "" {
		return
	}

	e.buffer.SaveState(e.cursorRow, e.cursorCol)

	if e.selection.IsActive() {
		e.deleteSelection()
	}

	newRow, newCol := e.buffer.InsertText(e.cursorRow, e.cursorCol, text)
	e.cursorRow = newRow
	e.cursorCol = newCol

	e.setModified(true)
}

// isWordChar returns true if the character is considered part of a word.
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '_'
}

func (e *CodeEditor) wordLeft() {
	line := e.buffer.GetLine(e.cursorRow)

	if e.cursorCol > len(line) {
		e.cursorCol = len(line)
	}

	if e.cursorCol == 0 {
		if e.cursorRow > 0 {
			e.cursorRow--
			e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
		}
	} else {
		e.cursorCol--

		for e.cursorCol > 0 && !isWordChar(line[e.cursorCol]) {
			e.cursorCol--
		}

		for e.cursorCol > 0 && isWordChar(line[e.cursorCol-1]) {
			e.cursorCol--
		}
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

func (e *CodeEditor) wordRight() {
	line := e.buffer.GetLine(e.cursorRow)

	if e.cursorCol >= len(line) {
		if e.cursorRow < e.buffer.GetLineCount()-1 {
			e.cursorRow++
			e.cursorCol = 0
		}
	} else {
		for e.cursorCol < len(line) && isWordChar(line[e.cursorCol]) {
			e.cursorCol++
		}

		for e.cursorCol < len(line) && !isWordChar(line[e.cursorCol]) {
			e.cursorCol++
		}
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// Focus sets focus to the code editor.
func (e *CodeEditor) Focus(delegate func(p tview.Primitive)) {
	e.hasFocus = true
}

// HasFocus returns whether the code editor has focus.
func (e *CodeEditor) HasFocus() bool {
	return e.hasFocus
}

// Blur removes focus from the code editor.
func (e *CodeEditor) Blur() {
	e.hasFocus = false
}

// MouseHandler returns the mouse handler for mouse events.
func (e *CodeEditor) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y := event.Position()
		rectX, rectY, width, height := e.GetInnerRect()

		if x >= rectX && x < rectX+width && y >= rectY && y < rectY+height {
			if x == rectX+width-1 {
				buttons := event.Buttons()
				if buttons&tcell.Button1 != 0 {
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
				return true, nil
			}

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

func (e *CodeEditor) getLineNumberWidth() int {
	totalLines := e.buffer.GetLineCount()
	if totalLines == 0 {
		totalLines = 1
	}

	digits := 0
	for n := totalLines; n > 0; n /= 10 {
		digits++
	}

	return digits
}

func (e *CodeEditor) drawLineNumber(screen tcell.Screen, lineNum int, x, y, width int) {
	numStr := fmt.Sprintf("%*d", width, lineNum)

	style := tcell.StyleDefault.Foreground(tcell.ColorGray).Background(tcell.NewRGBColor(40, 40, 40))
	for i, ch := range numStr {
		if i < width {
			screen.SetContent(x+i, y, ch, nil, style)
		}
	}
}

func (e *CodeEditor) setCursorFromClick(relativeX, relativeY int) {
	targetRow := e.offsetRow + relativeY
	if targetRow < 0 {
		targetRow = 0
	}
	if targetRow >= e.buffer.GetLineCount() {
		targetRow = e.buffer.GetLineCount() - 1
	}

	targetVisualCol := e.offsetCol + relativeX
	if targetVisualCol < 0 {
		targetVisualCol = 0
	}

	if targetRow >= 0 && targetRow < e.buffer.GetLineCount() {
		e.cursorRow = targetRow
		e.cursorCol = e.getRealColumn(targetRow, targetVisualCol)

		if e.cursorCol > e.buffer.GetLineLength(e.cursorRow) {
			e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
		}
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
}

// IsModified returns whether the editor content has been modified.
func (e *CodeEditor) IsModified() bool {
	return e.buffer.IsModified()
}

// GetFilePath returns the file path of the currently loaded file.
func (e *CodeEditor) GetFilePath() string {
	return e.filePath
}

// SetReadOnly sets the read-only mode of the editor.
func (e *CodeEditor) SetReadOnly(readOnly bool) *CodeEditor {
	e.readOnly = readOnly
	return e
}

// SetApp sets the application reference for the code editor.
func (e *CodeEditor) SetApp(app *tview.Application) *CodeEditor {
	e.app = app
	return e
}

// SetOnModified sets the callback for when the editor content is modified.
func (e *CodeEditor) SetOnModified(handler func(modified bool)) *CodeEditor {
	e.onModified = handler
	return e
}

// SetOnCursorMove sets the callback for when the cursor position changes.
func (e *CodeEditor) SetOnCursorMove(handler func(row, col int)) *CodeEditor {
	e.onCursorMove = handler
	return e
}

// Undo reverts the last edit operation.
func (e *CodeEditor) Undo() {
	if e.readOnly {
		return
	}

	cursorX, cursorY, success := e.buffer.Undo()
	if success {
		e.cursorCol = cursorX
		e.cursorRow = cursorY
		e.selection.Clear()
		e.ensureCursorVisible()
		
		e.setModified(e.buffer.IsModified())
	}
}

// Redo reapplies a previously undone edit operation  
func (e *CodeEditor) Redo() {
	if e.readOnly {
		return
	}

	cursorX, cursorY, success := e.buffer.Redo()
	if success {
		e.cursorCol = cursorX
		e.cursorRow = cursorY
		e.selection.Clear()
		e.ensureCursorVisible()
		
		e.setModified(e.buffer.IsModified())
	}
}

// Close cleans up resources
func (e *CodeEditor) Close() {
	if e.highlighter != nil {
		e.highlighter.Close()
		e.highlighter = nil
	}
}
