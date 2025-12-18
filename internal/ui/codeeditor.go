// Package ui provides the main user interface components for the emed text editor.
package ui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/core/buffer"
	"github.com/laborin/emed/internal/core/clipboard"
	"github.com/laborin/emed/internal/core/selection"
	"github.com/laborin/emed/internal/core/syntax"
	"github.com/laborin/emed/internal/git"
	"github.com/rivo/tview"
)

// SearchOptions contains options for search operations.
type SearchOptions struct {
	CaseSensitive bool
	WholeWord     bool
	UseRegex      bool
}

// SearchMatch represents a single search match.
type SearchMatch struct {
	StartRow int
	StartCol int
	EndRow   int
	EndCol   int
}

// GhostText represents overlay text that appears in the editor but isn't part of the buffer.
type GhostText struct {
	Row       int      // Buffer row where ghost text starts
	Col       int      // Column position (-1 means after end of line)
	Lines     []string // Text content (supports multiline)
	StyleFunc func() tcell.Style // Optional custom style function
}

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

	ghostTexts     []GhostText
	ghostTextStyle tcell.Style
	tabStyle       tcell.Style

	blameEnabled   bool
	blameText      string
	blameRow       int
	blameTimer     *time.Timer
	blameMutex     sync.Mutex

	pendingCenterCursor bool
}

// NewCodeEditor creates a new code editor instance with default settings.
func NewCodeEditor() *CodeEditor {
	return &CodeEditor{
		Box:            tview.NewBox(),
		buffer:         buffer.New(),
		selection:      selection.New(),
		clipboard:      clipboard.New(),
		readOnly:       false,
		ghostTextStyle: tcell.StyleDefault.Foreground(tcell.NewRGBColor(100, 100, 100)).Background(tcell.NewRGBColor(0, 0, 0)),
		tabStyle:       tcell.StyleDefault.Foreground(tcell.NewRGBColor(50, 50, 50)).Background(tcell.NewRGBColor(0, 0, 0)),
		blameEnabled:   true,
		blameRow:       -1,
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

	if e.highlighter != nil {
		e.highlighter.Close()
	}
	e.highlighter = syntax.NewHighlighter(path)
	if e.highlighter != nil {
		content := e.buffer.GetContent()
		lines := strings.Split(content, "\n")
		e.highlighter.UpdateContent(lines)
	}

	e.blameRow = -1
	e.ClearGhostTexts()
	e.scheduleBlameUpdate()

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

	style := tcell.StyleDefault.Background(tcell.NewRGBColor(0, 0, 0))
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, style)
		}
	}

	lineNumWidth := e.getLineNumberWidth() + 1

	e.viewportHeight = height
	e.viewportWidth = width - lineNumWidth - 1

	if e.pendingCenterCursor {
		e.pendingCenterCursor = false
		e.CenterCursor()
	}

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

	defaultStyle := tcell.StyleDefault.Background(tcell.NewRGBColor(0, 0, 0)).Foreground(tcell.ColorWhite)

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

	ghostText, ghostCol, ghostStyle := e.getGhostTextForLine(lineNum)

	for _, ch := range line {
		style := getStyleForColumn(realCol)

		isSelected := e.selection.IsPositionSelected(lineNum, realCol)
		if isSelected {
			fg, _, _ := style.Decompose()
			style = tcell.StyleDefault.Background(tcell.NewRGBColor(50, 50, 50)).Foreground(fg)
		}

		isMatch, isCurrent := e.isSearchMatch(lineNum, realCol)
		if isMatch {
			if isCurrent {
				style = tcell.StyleDefault.Background(tcell.ColorOrange).Foreground(tcell.ColorBlack)
			} else {
				style = tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack)
			}
		}

		if ch == '\t' {
			tabSize := e.buffer.GetTabSize()
			nextTabStop := ((visualCol / tabSize) + 1) * tabSize
			spacesToAdd := nextTabStop - visualCol

			for i := 0; i < spacesToAdd; i++ {
				if visualCol >= e.offsetCol && screenX < x+maxWidth {
					displayChar := ' '
					displayStyle := style
					if i == spacesToAdd-1 {
						displayChar = '▸'
						displayStyle = e.tabStyle
					} else if i < spacesToAdd-1 {
						displayChar = '─'
						displayStyle = e.tabStyle
					}
					screen.SetContent(screenX, y, displayChar, nil, displayStyle)
					screenX++
				}
				visualCol++
			}
			realCol++
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

	if ghostText != "" && screenX < x+maxWidth {
		startX := screenX
		if ghostCol >= 0 {
			targetVisualCol := ghostCol
			if targetVisualCol > visualCol {
				for visualCol < targetVisualCol && screenX < x+maxWidth {
					screen.SetContent(screenX, y, ' ', nil, defaultStyle)
					screenX++
					visualCol++
				}
			}
			startX = x + targetVisualCol - e.offsetCol
			if startX < screenX {
				startX = screenX
			}
		}

		for _, ch := range ghostText {
			if startX >= x && startX < x+maxWidth {
				screen.SetContent(startX, y, ch, nil, ghostStyle)
			}
			startX++
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
	key := event.Key()

	// Workaround: some terminals (e.g. Terminal.app) don't send Shift modifier for Up/Down.
	// If selection is active and user presses Up/Down, assume they want to extend selection.
	isVertical := key == tcell.KeyUp || key == tcell.KeyDown
	extendSelection := shiftHeld || (isVertical && e.selection.IsActive())

	if extendSelection && !e.selection.IsActive() {
		e.selection.Start(e.cursorRow, e.cursorCol)
	}

	if modifiers&tcell.ModCtrl != 0 || modifiers&tcell.ModMeta != 0 {
		switch key {
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
		switch key {
		case tcell.KeyLeft:
			e.wordLeft()
		case tcell.KeyRight:
			e.wordRight()
		default:
			e.handleNavigation(key)
		}
	} else {
		e.handleNavigation(key)
	}

	if extendSelection && e.selection.IsActive() {
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
	e.scheduleBlameUpdate()
}

func (e *CodeEditor) adjustCursorColumn() {
	lineLen := e.buffer.GetLineLength(e.cursorRow)
	if e.cursorCol > lineLen {
		e.cursorCol = lineLen
	}
}

func (e *CodeEditor) ensureCursorVisible() {
	if e.cursorRow < e.offsetRow {
		e.offsetRow = e.cursorRow
	} else if e.cursorRow >= e.offsetRow+e.viewportHeight {
		e.offsetRow = e.cursorRow - e.viewportHeight + 1
	}

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
	e.scheduleBlameUpdate()
}

func (e *CodeEditor) pageDown() {
	e.cursorRow += e.viewportHeight
	if e.cursorRow >= e.buffer.GetLineCount() {
		e.cursorRow = e.buffer.GetLineCount() - 1
	}
	e.adjustCursorColumn()
	e.scheduleBlameUpdate()
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
	e.scheduleBlameUpdate()
}

func (e *CodeEditor) ctrlDown() {
	if e.buffer.GetLineCount() > 0 {
		e.cursorRow = e.buffer.GetLineCount() - 1
		e.cursorCol = e.buffer.GetLineLength(e.cursorRow)
	}

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
	e.scheduleBlameUpdate()
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
	e.scheduleBlameUpdate()
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
	e.scheduleBlameUpdate()
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

	style := tcell.StyleDefault.Foreground(tcell.NewRGBColor(102, 102, 102)).Background(tcell.NewRGBColor(40, 40, 40))
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
	e.scheduleBlameUpdate()
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
	e.blameMutex.Lock()
	if e.blameTimer != nil {
		e.blameTimer.Stop()
	}
	e.blameMutex.Unlock()
}

func (e *CodeEditor) scheduleBlameUpdate() {
	if !e.blameEnabled || e.filePath == "" {
		return
	}

	e.blameMutex.Lock()
	defer e.blameMutex.Unlock()

	if e.blameTimer != nil {
		e.blameTimer.Stop()
	}

	if e.cursorRow == e.blameRow {
		return
	}

	e.blameTimer = time.AfterFunc(300*time.Millisecond, func() {
		e.fetchBlameAndDisplay()
	})
}

func (e *CodeEditor) fetchBlameAndDisplay() {
	e.blameMutex.Lock()
	row := e.cursorRow
	filePath := e.filePath
	e.blameMutex.Unlock()

	if filePath == "" {
		return
	}

	lineNum := row + 1
	blameInfo, err := git.GetBlameForLine(filePath, lineNum)

	e.blameMutex.Lock()
	if e.cursorRow != row {
		e.blameMutex.Unlock()
		return
	}

	e.blameRow = row

	if err != nil {
		e.blameText = ""
		e.ClearGhostTexts()
	} else {
		e.blameText = git.FormatBlameInfo(blameInfo)
		e.ClearGhostTexts()
		if e.blameText != "" {
			e.AddGhostText(row, -1, e.blameText)
		}
	}
	e.blameMutex.Unlock()

	if e.app != nil {
		e.app.QueueUpdateDraw(func() {})
	}
}

// SetGhostText sets a single ghost text overlay at the specified position.
func (e *CodeEditor) SetGhostText(row, col int, text string) {
	lines := strings.Split(text, "\n")
	e.ghostTexts = []GhostText{{Row: row, Col: col, Lines: lines}}
}

// SetGhostTextWithStyle sets a ghost text with a custom style function.
func (e *CodeEditor) SetGhostTextWithStyle(row, col int, text string, styleFunc func() tcell.Style) {
	lines := strings.Split(text, "\n")
	e.ghostTexts = []GhostText{{Row: row, Col: col, Lines: lines, StyleFunc: styleFunc}}
}

// AddGhostText adds a ghost text overlay without clearing existing ones.
func (e *CodeEditor) AddGhostText(row, col int, text string) {
	lines := strings.Split(text, "\n")
	e.ghostTexts = append(e.ghostTexts, GhostText{Row: row, Col: col, Lines: lines})
}

// ClearGhostTexts removes all ghost text overlays.
func (e *CodeEditor) ClearGhostTexts() {
	e.ghostTexts = nil
}

// SetGhostTextStyle sets the default style for ghost text rendering.
func (e *CodeEditor) SetGhostTextStyle(style tcell.Style) {
	e.ghostTextStyle = style
}

// getGhostTextForLine returns ghost text content for a specific line if any.
func (e *CodeEditor) getGhostTextForLine(row int) (ghostText string, col int, style tcell.Style) {
	for _, gt := range e.ghostTexts {
		lineOffset := row - gt.Row
		if lineOffset >= 0 && lineOffset < len(gt.Lines) {
			style := e.ghostTextStyle
			if gt.StyleFunc != nil {
				style = gt.StyleFunc()
			}
			if lineOffset == 0 {
				return gt.Lines[lineOffset], gt.Col, style
			}
			return gt.Lines[lineOffset], 0, style
		}
	}
	return "", -1, e.ghostTextStyle
}

// Search performs a search in the editor content.
func (e *CodeEditor) Search(text string, caseSensitive, wholeWord, useRegex bool) []buffer.SearchMatch {
	lines := e.buffer.GetLines()
	runeLines := make([][]rune, len(lines))
	for i, line := range lines {
		runeLines[i] = []rune(line)
	}
	
	options := SearchOptions{
		CaseSensitive: caseSensitive,
		WholeWord:     wholeWord,
		UseRegex:      useRegex,
	}
	
	matches := performTextSearch(runeLines, text, options)
	
	bufferMatches := make([]buffer.SearchMatch, len(matches))
	for i, match := range matches {
		bufferMatches[i] = buffer.SearchMatch{
			StartRow: match.StartRow,
			StartCol: match.StartCol,
			EndRow:   match.EndRow,
			EndCol:   match.EndCol,
		}
	}
	
	e.buffer.SetSearchMatches(bufferMatches)
	
	if useRegex {
		flags := ""
		if !caseSensitive {
			flags = "(?i)"
		}
		if wholeWord {
			text = `\b` + text + `\b`
		}
		re, err := regexp.Compile(flags + text)
		if err == nil {
			e.buffer.SetSearchPattern(text, re)
		}
	} else {
		e.buffer.SetSearchPattern(text, nil)
	}
	
	return bufferMatches
}

// ClearSearchHighlights clears all search highlights.
func (e *CodeEditor) ClearSearchHighlights() {
	e.buffer.ClearSearchMatches()
}

// GetSelectedText returns the currently selected text.
func (e *CodeEditor) GetSelectedText() string {
	if !e.selection.IsActive() {
		return ""
	}
	
	startRow, startCol, endRow, endCol := e.selection.GetNormalizedRange()
	return e.buffer.GetTextRange(startRow, startCol, endRow, endCol)
}

// SetCursor sets the cursor position.
func (e *CodeEditor) SetCursor(row, col int) {
	lines := e.buffer.GetLines()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	if col < 0 {
		col = 0
	}
	if row >= 0 && row < len(lines) {
		lineLen := len([]rune(lines[row]))
		if col > lineLen {
			col = lineLen
		}
	}

	e.cursorRow = row
	e.cursorCol = col
	e.ensureCursorVisible()
	e.scheduleBlameUpdate()
}

// ScrollToCursor scrolls the view to make the cursor visible.
func (e *CodeEditor) ScrollToCursor() {
	e.ensureCursorVisible()
}

// RequestCenterCursor schedules cursor centering for the next draw cycle.
func (e *CodeEditor) RequestCenterCursor() {
	e.pendingCenterCursor = true
}

// CenterCursor centers the cursor in the viewport
func (e *CodeEditor) CenterCursor() {
	if e.viewportHeight <= 0 {
		_, _, _, height := e.GetInnerRect()
		if height > 0 {
			e.viewportHeight = height
		} else {
			e.viewportHeight = 30
		}
	}
	
	targetOffset := e.cursorRow - (e.viewportHeight / 2)
	
	if targetOffset < 0 {
		e.offsetRow = 0
	} else {
		maxOffset := len(e.buffer.GetLines()) - e.viewportHeight
		if maxOffset < 0 {
			maxOffset = 0
		}
		if targetOffset > maxOffset {
			e.offsetRow = maxOffset
		} else {
			e.offsetRow = targetOffset
		}
	}
	
	visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
	
	if visualCol < e.offsetCol {
		e.offsetCol = visualCol - 10
		if e.offsetCol < 0 {
			e.offsetCol = 0
		}
	} else if e.viewportWidth > 0 && visualCol >= e.offsetCol + e.viewportWidth {
		e.offsetCol = visualCol - e.viewportWidth + 10
		if e.offsetCol < 0 {
			e.offsetCol = 0
		}
	} else if e.offsetCol > 0 && e.viewportWidth > 0 && visualCol < e.viewportWidth {
		e.offsetCol = 0
	}
}

// isSearchMatch checks if a position is part of a search match.
func (e *CodeEditor) isSearchMatch(row, col int) (isMatch, isCurrent bool) {
	matches := e.buffer.GetSearchMatches()
	if len(matches) == 0 {
		return false, false
	}
	
	currentIndex, _ := e.buffer.GetCurrentMatch()
	for i, match := range matches {
		if row == match.StartRow && col >= match.StartCol && col < match.EndCol {
			return true, i == currentIndex-1
		}
	}
	return false, false
}

func performTextSearch(lines [][]rune, pattern string, options SearchOptions) []SearchMatch {
	if pattern == "" {
		return nil
	}
	
	var matches []SearchMatch
	
	if options.UseRegex {
		matches = searchWithRegex(lines, pattern, options)
	} else {
		matches = searchPlainText(lines, pattern, options)
	}
	
	return matches
}

func searchPlainText(lines [][]rune, pattern string, options SearchOptions) []SearchMatch {
	var matches []SearchMatch
	searchPattern := pattern
	
	if !options.CaseSensitive {
		searchPattern = strings.ToLower(pattern)
	}
	
	patternRunes := []rune(searchPattern)
	patternLen := len(patternRunes)
	
	for row, line := range lines {
		lineStr := string(line)
		if !options.CaseSensitive {
			lineStr = strings.ToLower(lineStr)
		}
		lineRunes := []rune(lineStr)
		
		for col := 0; col <= len(lineRunes)-patternLen; col++ {
			if matchesAt(lineRunes, patternRunes, col) {
				if options.WholeWord {
					if !isWordBoundary(lineRunes, col, col+patternLen) {
						continue
					}
				}
				matches = append(matches, SearchMatch{
					StartRow: row,
					StartCol: col,
					EndRow:   row,
					EndCol:   col + patternLen,
				})
			}
		}
	}
	
	return matches
}

func searchWithRegex(lines [][]rune, pattern string, options SearchOptions) []SearchMatch {
	var matches []SearchMatch
	
	flags := ""
	if !options.CaseSensitive {
		flags = "(?i)"
	}
	
	if options.WholeWord {
		pattern = `\b` + pattern + `\b`
	}
	
	re, err := regexp.Compile(flags + pattern)
	if err != nil {
		return nil
	}
	
	for row, line := range lines {
		lineStr := string(line)
		allMatches := re.FindAllStringIndex(lineStr, -1)
		
		for _, match := range allMatches {
			startCol := len([]rune(lineStr[:match[0]]))
			endCol := len([]rune(lineStr[:match[1]]))
			
			matches = append(matches, SearchMatch{
				StartRow: row,
				StartCol: startCol,
				EndRow:   row,
				EndCol:   endCol,
			})
		}
	}
	
	return matches
}

func matchesAt(line, pattern []rune, pos int) bool {
	if pos+len(pattern) > len(line) {
		return false
	}
	
	for i, r := range pattern {
		if line[pos+i] != r {
			return false
		}
	}
	
	return true
}

func isWordBoundary(line []rune, start, end int) bool {
	if start > 0 {
		prevChar := line[start-1]
		if unicode.IsLetter(prevChar) || unicode.IsDigit(prevChar) || prevChar == '_' {
			return false
		}
	}
	
	if end < len(line) {
		nextChar := line[end]
		if unicode.IsLetter(nextChar) || unicode.IsDigit(nextChar) || nextChar == '_' {
			return false
		}
	}

	return true
}

// CenterViewOnCursor adjusts the scroll offset to center the cursor vertically.
func (c *CodeEditor) CenterViewOnCursor() {
	_, _, _, height := c.GetInnerRect()
	if height <= 0 {
		height = 24
	}

	lines := c.buffer.GetLines()
	lineCount := len(lines)

	targetOffset := c.cursorRow - height/2
	if targetOffset < 0 {
		targetOffset = 0
	}

	maxOffset := lineCount - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if targetOffset > maxOffset {
		targetOffset = maxOffset
	}

	c.offsetRow = targetOffset
}
