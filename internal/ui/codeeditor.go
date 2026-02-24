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
	"github.com/mattn/go-runewidth"

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
	Row       int                // Buffer row where ghost text starts
	Col       int                // Column position (-1 means after end of line)
	Lines     []string           // Text content (supports multiline)
	StyleFunc func() tcell.Style // Optional custom style function
}

// wordOccurrence represents a position where the current word appears.
type wordOccurrence struct {
	row      int
	startCol int
	endCol   int
}

// bracketPairs maps opening brackets to closing brackets.
var bracketPairs = map[rune]rune{
	'(': ')',
	'[': ']',
	'{': '}',
	'<': '>',
}

// closingBrackets maps closing brackets to opening brackets.
var closingBrackets = map[rune]rune{
	')': '(',
	']': '[',
	'}': '{',
	'>': '<',
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

	onModified        func(modified bool)
	onCursorMove      func(row, col int)
	onSelectionChange func()

	app *tview.Application

	ghostTexts     []GhostText
	ghostTextStyle tcell.Style
	tabStyle       tcell.Style

	blameEnabled bool
	blameText    string
	blameRow     int
	blameTimer   *time.Timer
	blameMutex   sync.Mutex

	highlightTimer *time.Timer
	highlightMutex sync.Mutex

	pendingCenterCursor bool

	// Bracket matching (stores both opening and closing positions)
	bracketOpenRow  int
	bracketOpenCol  int
	bracketCloseRow int
	bracketCloseCol int
	hasBracketMatch bool

	// Word highlighting
	currentWord     string
	wordOccurrences []wordOccurrence

	// Indentation guide
	indentGuideCol      int
	indentGuideStartRow int
	indentGuideEndRow   int

	// Dirty tracking for highlights
	lastHighlightRow int
	lastHighlightCol int
	highlightsDirty  bool

	// Multi-click detection (double, triple, etc.)
	lastClickTime time.Time
	lastClickRow  int
	lastClickCol  int
	clickCount    int
}

// NewCodeEditor creates a new code editor instance with default settings.
func NewCodeEditor() *CodeEditor {
	return &CodeEditor{
		Box:              tview.NewBox(),
		buffer:           buffer.New(),
		selection:        selection.New(),
		clipboard:        clipboard.New(),
		readOnly:         false,
		ghostTextStyle:   tcell.StyleDefault.Foreground(tcell.NewRGBColor(100, 100, 100)).Background(tcell.NewRGBColor(0, 0, 0)),
		tabStyle:         tcell.StyleDefault.Foreground(tcell.NewRGBColor(50, 50, 50)).Background(tcell.NewRGBColor(0, 0, 0)),
		blameEnabled:     true,
		blameRow:         -1,
		bracketOpenRow:   -1,
		bracketOpenCol:   -1,
		bracketCloseRow:  -1,
		bracketCloseCol:  -1,
		lastHighlightRow: -1,
		lastHighlightCol: -1,
		highlightsDirty:  true,
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
	e.highlightsDirty = true
	e.updateHighlighter()

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

	if e.highlightsDirty || e.cursorRow != e.lastHighlightRow || e.cursorCol != e.lastHighlightCol {
		e.updateBracketMatch()
		e.updateWordOccurrences()
		e.updateIndentGuide()
		e.lastHighlightRow = e.cursorRow
		e.lastHighlightCol = e.cursorCol
		e.highlightsDirty = false
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

	screenX := x   // Position on screen
	visualCol := 0 // Visual column position (accounting for tabs)
	realCol := 0   // Real column position in buffer

	ghostText, ghostCol, ghostStyle := e.getGhostTextForLine(lineNum)

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
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

		if e.isBracketHighlight(lineNum, realCol) {
			style = tcell.StyleDefault.Background(tcell.NewRGBColor(80, 80, 40)).Foreground(tcell.ColorYellow).Bold(true)
		}

		if e.isWordOccurrence(lineNum, realCol) {
			style = style.Underline(true)
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
					if e.shouldShowIndentGuide(lineNum) && visualCol == e.indentGuideCol {
						displayChar = '│'
						displayStyle = tcell.StyleDefault.Foreground(tcell.NewRGBColor(60, 60, 60))
					}
					screen.SetContent(screenX, y, displayChar, nil, displayStyle)
					screenX++
				}
				visualCol++
			}
			realCol++
		} else {
			var combiningRunes []rune
			for i+1 < len(runes) && isVariationSelector(runes[i+1]) {
				i++
				combiningRunes = append(combiningRunes, runes[i])
				realCol++
			}

			charWidth := runewidth.RuneWidth(ch)
			if len(combiningRunes) > 0 && hasEmojiVariationSelector(combiningRunes) {
				charWidth = 2
			}
			if charWidth < 1 {
				charWidth = 1
			}

			if visualCol >= e.offsetCol && screenX < x+maxWidth {
				if ch == ' ' && e.shouldShowIndentGuide(lineNum) && visualCol == e.indentGuideCol {
					guideStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(60, 60, 60))
					screen.SetContent(screenX, y, '│', nil, guideStyle)
				} else {
					screen.SetContent(screenX, y, ch, combiningRunes, style)
				}
				screenX += charWidth
			}
			visualCol += charWidth
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
			charWidth := runewidth.RuneWidth(ch)
			if charWidth < 1 {
				charWidth = 1
			}
			if startX >= x && startX < x+maxWidth {
				screen.SetContent(startX, y, ch, nil, ghostStyle)
				if charWidth == 2 && startX+1 < x+maxWidth {
					screen.SetContent(startX+1, y, ' ', nil, ghostStyle)
				}
			}
			startX += charWidth
		}
		screenX = startX
	}

	indentGuideStyle := tcell.StyleDefault.Foreground(tcell.NewRGBColor(60, 60, 60)).Background(tcell.NewRGBColor(0, 0, 0))
	for screenX < x+maxWidth {
		currentVisualCol := screenX - x + e.offsetCol
		if e.shouldShowIndentGuide(lineNum) && currentVisualCol == e.indentGuideCol {
			screen.SetContent(screenX, y, '│', nil, indentGuideStyle)
		} else {
			screen.SetContent(screenX, y, ' ', nil, defaultStyle)
		}
		screenX++
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

		if isLineBreakKey(event) {
			e.insertNewline()
			return
		}

		switch event.Key() {
		case tcell.KeyEscape:
			return

		case tcell.KeyRune:
			modifiers := event.Modifiers()
			r := event.Rune()

			if modifiers&(tcell.ModCtrl|tcell.ModMeta) != 0 {
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
				switch r {
				case 'b', 'B':
					e.wordLeft()
					return
				case 'f', 'F':
					e.wordRight()
					return
				}
			}

			e.insertRune(r)

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

func (e *CodeEditor) PasteHandler() func(pastedText string, setFocus func(p tview.Primitive)) {
	return func(pastedText string, setFocus func(p tview.Primitive)) {
		pastedText = normalizePastedText(pastedText)
		if !e.hasFocus || e.readOnly || pastedText == "" {
			return
		}

		e.buffer.SaveState(e.cursorRow, e.cursorCol)

		if e.selection.IsActive() {
			e.deleteSelection()
		}

		newRow, newCol := e.buffer.InsertText(e.cursorRow, e.cursorCol, pastedText)
		e.cursorRow = newRow
		e.cursorCol = newCol

		e.setModified(true)
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
		e.notifySelectionChange()
	} else if !shiftHeld && e.selection.IsActive() {
		e.selection.Clear()
		e.notifySelectionChange()
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
		visualLength := e.getVisualColumn(i, len([]rune(line)))
		if visualLength > maxVisualLength {
			maxVisualLength = visualLength
		}
	}
	extraOffset := 40
	maxOffset := maxVisualLength - e.viewportWidth + extraOffset
	if maxOffset < 0 {
		maxOffset = 0
	}
	if e.offsetCol > maxOffset {
		e.offsetCol = maxOffset
	}
}

func (e *CodeEditor) getVisualColumn(row, col int) int {
	line := e.buffer.GetLine(row)
	runes := []rune(line)
	visualCol := 0
	tabSize := e.buffer.GetTabSize()

	for i := 0; i < col && i < len(runes); i++ {
		ch := runes[i]
		if ch == '\t' {
			visualCol = ((visualCol / tabSize) + 1) * tabSize
		} else if isVariationSelector(ch) {
			continue
		} else {
			charWidth := runewidth.RuneWidth(ch)
			if i+1 < len(runes) && isVariationSelector(runes[i+1]) && runes[i+1] == 0xFE0F {
				charWidth = 2
			}
			if charWidth < 1 {
				charWidth = 1
			}
			visualCol += charWidth
		}
	}

	return visualCol
}

func (e *CodeEditor) getRealColumn(row, targetVisualCol int) int {
	line := e.buffer.GetLine(row)
	runes := []rune(line)
	visualCol := 0
	realCol := 0
	tabSize := e.buffer.GetTabSize()

	for realCol < len(runes) && visualCol < targetVisualCol {
		ch := runes[realCol]
		if ch == '\t' {
			nextTabStop := ((visualCol / tabSize) + 1) * tabSize
			if nextTabStop > targetVisualCol {
				break
			}
			visualCol = nextTabStop
		} else if isVariationSelector(ch) {
			realCol++
			continue
		} else {
			charWidth := runewidth.RuneWidth(ch)
			if realCol+1 < len(runes) && isVariationSelector(runes[realCol+1]) && runes[realCol+1] == 0xFE0F {
				charWidth = 2
			}
			if charWidth < 1 {
				charWidth = 1
			}
			if visualCol+charWidth > targetVisualCol {
				break
			}
			visualCol += charWidth
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
	if modified {
		e.clearBlame()
		e.scheduleHighlighterUpdate()
		e.highlightsDirty = true
	}
}

func (e *CodeEditor) scheduleHighlighterUpdate() {
	e.highlightMutex.Lock()
	defer e.highlightMutex.Unlock()

	if e.highlightTimer != nil {
		e.highlightTimer.Stop()
	}

	e.highlightTimer = time.AfterFunc(150*time.Millisecond, func() {
		e.updateHighlighter()
		if e.app != nil {
			e.app.QueueUpdateDraw(func() {})
		}
	})
}

func (e *CodeEditor) updateHighlighter() {
	if e.buffer == nil || e.highlighter == nil {
		return
	}
	content := e.buffer.GetContent()
	lines := strings.Split(content, "\n")
	e.highlighter.UpdateContent(lines)
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
	e.notifySelectionChange()
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
	text = normalizePastedText(text)
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

// isVariationSelector returns true if the rune is a variation selector.
func isVariationSelector(r rune) bool {
	return r == 0xFE0E || r == 0xFE0F ||
		(r >= 0xE0100 && r <= 0xE01EF)
}

// hasEmojiVariationSelector returns true if any rune is VS16 (emoji presentation).
func hasEmojiVariationSelector(runes []rune) bool {
	for _, r := range runes {
		if r == 0xFE0F {
			return true
		}
	}
	return false
}

func (e *CodeEditor) wordLeft() {
	row := e.cursorRow
	col := e.cursorCol
	lineCount := e.buffer.GetLineCount()
	if lineCount == 0 {
		return
	}

	if row >= lineCount {
		row = lineCount - 1
	}

	for {
		line := e.buffer.GetLine(row)
		runes := []rune(line)
		if col > len(runes) {
			col = len(runes)
		}

		for col > 0 && !isWordChar(runes[col-1]) {
			col--
		}
		if col > 0 {
			for col > 0 && isWordChar(runes[col-1]) {
				col--
			}
			break
		}

		if row == 0 {
			break
		}
		row--
		col = e.buffer.GetLineLength(row)
	}

	e.cursorRow = row
	e.cursorCol = col

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
	e.scheduleBlameUpdate()
	e.ensureCursorVisible()
}

func (e *CodeEditor) wordRight() {
	row := e.cursorRow
	col := e.cursorCol
	lineCount := e.buffer.GetLineCount()
	if lineCount == 0 {
		return
	}

	if row < 0 {
		row = 0
	}

	for {
		line := e.buffer.GetLine(row)
		runes := []rune(line)
		if col > len(runes) {
			col = len(runes)
		}

		if col < len(runes) && isWordChar(runes[col]) {
			for col < len(runes) && isWordChar(runes[col]) {
				col++
			}
		}

		for {
			for col < len(runes) && !isWordChar(runes[col]) {
				col++
			}
			if col < len(runes) {
				e.cursorRow = row
				e.cursorCol = col
				goto done
			}
			if row >= lineCount-1 {
				e.cursorRow = row
				e.cursorCol = len(runes)
				goto done
			}
			row++
			col = 0
			line = e.buffer.GetLine(row)
			runes = []rune(line)
		}
	}

done:

	if e.onCursorMove != nil {
		e.onCursorMove(e.cursorRow, e.cursorCol)
	}
	e.scheduleBlameUpdate()
	e.ensureCursorVisible()
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
				if modifiers&(tcell.ModAlt|tcell.ModShift) != 0 {
					e.scrollLeft(5)
				} else {
					e.scrollUp(1)
				}
				return true, nil

			case tview.MouseScrollDown:
				modifiers := event.Modifiers()
				if modifiers&(tcell.ModAlt|tcell.ModShift) != 0 {
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

				lineNumWidth := e.getLineNumberWidth() + 1
				clickX := x - rectX - lineNumWidth
				clickY := y - rectY

				// Calculate click position first
				clickRow := e.offsetRow + clickY
				if clickRow >= e.buffer.GetLineCount() {
					clickRow = e.buffer.GetLineCount() - 1
				}
				if clickRow < 0 {
					clickRow = 0
				}
				clickCol := e.getRealColumn(clickRow, e.offsetCol+clickX)

				// Multi-click detection
				now := time.Now()
				colDiff := e.lastClickCol - clickCol
				if colDiff < 0 {
					colDiff = -colDiff
				}
				isQuickClick := now.Sub(e.lastClickTime) < 400*time.Millisecond &&
					e.lastClickRow == clickRow &&
					colDiff <= 1

				if isQuickClick {
					e.clickCount++
				} else {
					e.clickCount = 1
				}

				e.lastClickTime = now
				e.lastClickRow = clickRow
				e.lastClickCol = clickCol
				e.cursorRow = clickRow
				e.cursorCol = clickCol

				switch e.clickCount {
				case 2: // Double-click: select word
					e.selectWordUnderCursor()
					return true, nil
				case 3: // Triple-click: select line
					e.selectCurrentLine()
					return true, nil
				case 4: // Quadruple-click: select all
					e.selectAll()
					e.clickCount = 0 // Reset
					return true, nil
				}

				// Single click: start drag selection
				e.selection.Clear()
				e.adjustCursorColumn()
				e.selection.StartMouseDrag(e.cursorRow, e.cursorCol)
				return true, e

			case tview.MouseLeftUp:
				e.selection.StopMouseDrag()
				e.notifySelectionChange()
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

// SetOnSelectionChange sets the callback for when the selection changes.
func (e *CodeEditor) SetOnSelectionChange(handler func()) *CodeEditor {
	e.onSelectionChange = handler
	return e
}

// GetSelectionInfo returns the current selection info or nil if no selection.
func (e *CodeEditor) GetSelectionInfo() (text string, startRow, startCol, endRow, endCol int, active bool) {
	if e.selection == nil || !e.selection.IsActive() {
		return "", 0, 0, 0, 0, false
	}
	startRow, startCol, endRow, endCol = e.selection.GetNormalizedRange()
	text = e.buffer.GetTextRange(startRow, startCol, endRow, endCol)
	return text, startRow, startCol, endRow, endCol, true
}

// GetCursorPosition returns the current cursor position.
func (e *CodeEditor) GetCursorPosition() (row, col int) {
	return e.cursorRow, e.cursorCol
}

func (e *CodeEditor) notifySelectionChange() {
	if e.onSelectionChange != nil {
		e.onSelectionChange()
	}
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
		e.updateHighlighter()
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
		e.updateHighlighter()
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
		e.blameTimer = nil
	}
	e.blameMutex.Unlock()

	e.highlightMutex.Lock()
	if e.highlightTimer != nil {
		e.highlightTimer.Stop()
		e.highlightTimer = nil
	}
	e.highlightMutex.Unlock()

	e.buffer = nil
	e.selection = nil
	e.ghostTexts = nil
	e.onModified = nil
	e.onCursorMove = nil
	e.filePath = ""
	e.blameText = ""
	e.blameEnabled = false
}

func (e *CodeEditor) scheduleBlameUpdate() {
	if !e.blameEnabled || e.filePath == "" {
		return
	}

	if e.buffer != nil && e.buffer.IsModified() {
		e.clearBlame()
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

func (e *CodeEditor) clearBlame() {
	e.blameMutex.Lock()
	e.blameText = ""
	e.blameRow = -1
	e.blameMutex.Unlock()
	e.ClearGhostTexts()
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

	e.offsetCol = 0

	visualCol := e.getVisualColumn(e.cursorRow, e.cursorCol)
	if e.viewportWidth > 0 && visualCol >= e.viewportWidth {
		e.offsetCol = visualCol - e.viewportWidth/2
		if e.offsetCol < 0 {
			e.offsetCol = 0
		}
	}
}

// updateBracketMatch finds and stores the matching bracket position.
// It highlights brackets when cursor is ON them or BETWEEN a pair.
func (e *CodeEditor) updateBracketMatch() {
	e.hasBracketMatch = false
	e.bracketOpenRow = -1
	e.bracketOpenCol = -1
	e.bracketCloseRow = -1
	e.bracketCloseCol = -1

	line := e.buffer.GetLine(e.cursorRow)
	runes := []rune(line)

	// First check if cursor is directly on a bracket
	if e.cursorCol < len(runes) {
		ch := runes[e.cursorCol]

		if closingBracket, isOpening := bracketPairs[ch]; isOpening {
			row, col := e.findMatchingBracket(e.cursorRow, e.cursorCol, ch, closingBracket, 1)
			if row >= 0 {
				e.hasBracketMatch = true
				e.bracketOpenRow = e.cursorRow
				e.bracketOpenCol = e.cursorCol
				e.bracketCloseRow = row
				e.bracketCloseCol = col
			}
			return
		}

		if openingBracket, isClosing := closingBrackets[ch]; isClosing {
			row, col := e.findMatchingBracket(e.cursorRow, e.cursorCol, ch, openingBracket, -1)
			if row >= 0 {
				e.hasBracketMatch = true
				e.bracketOpenRow = row
				e.bracketOpenCol = col
				e.bracketCloseRow = e.cursorRow
				e.bracketCloseCol = e.cursorCol
			}
			return
		}
	}

	// If not on a bracket, find the closest enclosing pair
	openRow, openCol, openBracket := e.findEnclosingOpenBracket()
	if openRow >= 0 {
		closingBracket := bracketPairs[openBracket]
		closeRow, closeCol := e.findMatchingBracket(openRow, openCol, openBracket, closingBracket, 1)
		if closeRow >= 0 {
			// Verify cursor is actually between them
			cursorAfterOpen := e.cursorRow > openRow || (e.cursorRow == openRow && e.cursorCol > openCol)
			cursorBeforeClose := e.cursorRow < closeRow || (e.cursorRow == closeRow && e.cursorCol < closeCol)
			if cursorAfterOpen && cursorBeforeClose {
				e.hasBracketMatch = true
				e.bracketOpenRow = openRow
				e.bracketOpenCol = openCol
				e.bracketCloseRow = closeRow
				e.bracketCloseCol = closeCol
			}
		}
	}
}

// findEnclosingOpenBracket searches backwards for the nearest unmatched opening bracket.
func (e *CodeEditor) findEnclosingOpenBracket() (row, col int, bracket rune) {
	lines := e.buffer.GetLines()
	row = e.cursorRow
	col = e.cursorCol - 1
	linesSearched := 0

	// Track depth for each bracket type
	depth := make(map[rune]int)

	for row >= 0 && linesSearched < maxBracketSearchLines {
		line := []rune(lines[row])

		for col >= 0 {
			ch := line[col]

			// If it's a closing bracket, increase depth
			if openBracket, isClosing := closingBrackets[ch]; isClosing {
				depth[openBracket]++
			}

			// If it's an opening bracket
			if _, isOpening := bracketPairs[ch]; isOpening {
				if depth[ch] > 0 {
					depth[ch]--
				} else {
					// Found an unmatched opening bracket
					return row, col, ch
				}
			}

			col--
		}

		row--
		linesSearched++
		if row >= 0 {
			col = len([]rune(lines[row])) - 1
		}
	}

	return -1, -1, 0
}

// maxBracketSearchLines limits bracket matching search distance.
const maxBracketSearchLines = 500

// findMatchingBracket searches for the matching bracket in the given direction.
func (e *CodeEditor) findMatchingBracket(startRow, startCol int, currentBracket, targetBracket rune, direction int) (int, int) {
	depth := 1
	lines := e.buffer.GetLines()
	row := startRow
	col := startCol
	linesSearched := 0

	for row >= 0 && row < len(lines) && linesSearched < maxBracketSearchLines {
		line := []rune(lines[row])

		if direction > 0 {
			col++
		} else {
			col--
		}

		for col >= 0 && col < len(line) {
			ch := line[col]
			if ch == currentBracket {
				depth++
			} else if ch == targetBracket {
				depth--
				if depth == 0 {
					return row, col
				}
			}

			if direction > 0 {
				col++
			} else {
				col--
			}
		}

		row += direction
		linesSearched++
		if row >= 0 && row < len(lines) {
			if direction > 0 {
				col = 0
			} else {
				col = len([]rune(lines[row])) - 1
			}
		}
	}

	return -1, -1
}

// updateWordOccurrences finds all occurrences of the word under cursor.
func (e *CodeEditor) updateWordOccurrences() {
	e.currentWord = ""
	e.wordOccurrences = nil

	word := e.getWordUnderCursor()
	if word == "" || len(word) < 2 {
		return
	}

	e.currentWord = word
	lines := e.buffer.GetLines()

	startLine := e.offsetRow
	endLine := e.offsetRow + e.viewportHeight
	if endLine > len(lines) {
		endLine = len(lines)
	}

	for row := startLine; row < endLine; row++ {
		line := lines[row]
		runes := []rune(line)
		wordRunes := []rune(word)

		for col := 0; col <= len(runes)-len(wordRunes); col++ {
			if e.matchesWordAt(runes, wordRunes, col) {
				e.wordOccurrences = append(e.wordOccurrences, wordOccurrence{
					row:      row,
					startCol: col,
					endCol:   col + len(wordRunes),
				})
			}
		}
	}
}

// selectCurrentLine selects the entire current line.
func (e *CodeEditor) selectCurrentLine() {
	lineLen := e.buffer.GetLineLength(e.cursorRow)
	e.selection.Start(e.cursorRow, 0)
	e.selection.Extend(e.cursorRow, lineLen)
	e.cursorCol = lineLen
	e.notifySelectionChange()
}

// selectWordUnderCursor selects the word at the cursor position.
func (e *CodeEditor) selectWordUnderCursor() {
	line := e.buffer.GetLine(e.cursorRow)

	if len(line) == 0 {
		return
	}

	col := e.cursorCol
	if col >= len(line) {
		col = len(line) - 1
	}
	if col < 0 {
		return
	}

	if !isWordChar(line[col]) {
		return
	}

	start := col
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}

	end := col
	for end < len(line)-1 && isWordChar(line[end+1]) {
		end++
	}
	end++ // end is exclusive

	e.selection.Start(e.cursorRow, start)
	e.selection.Extend(e.cursorRow, end)
	e.cursorCol = end
	e.notifySelectionChange()
}

// getWordUnderCursor returns the word at the cursor position.
func (e *CodeEditor) getWordUnderCursor() string {
	line := e.buffer.GetLine(e.cursorRow)
	runes := []rune(line)

	if len(runes) == 0 {
		return ""
	}

	col := e.cursorCol
	if col >= len(runes) {
		col = len(runes) - 1
	}
	if col < 0 {
		return ""
	}

	if !isWordChar(runes[col]) {
		return ""
	}

	start := col
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}

	end := col
	for end < len(runes)-1 && isWordChar(runes[end+1]) {
		end++
	}

	return string(runes[start : end+1])
}

// matchesWordAt checks if a word matches at the given position with word boundaries.
func (e *CodeEditor) matchesWordAt(line, word []rune, pos int) bool {
	if pos > 0 && isWordChar(line[pos-1]) {
		return false
	}

	endPos := pos + len(word)
	if endPos < len(line) && isWordChar(line[endPos]) {
		return false
	}

	for i, r := range word {
		if line[pos+i] != r {
			return false
		}
	}

	return true
}

// isWordOccurrence checks if a position is part of a word occurrence.
func (e *CodeEditor) isWordOccurrence(row, col int) bool {
	if e.currentWord == "" {
		return false
	}

	for _, occ := range e.wordOccurrences {
		if occ.row == row && col >= occ.startCol && col < occ.endCol {
			return true
		}
	}
	return false
}

// updateIndentGuide calculates the indentation guide column and block range.
func (e *CodeEditor) updateIndentGuide() {
	e.indentGuideCol = -1
	e.indentGuideStartRow = -1
	e.indentGuideEndRow = -1

	// Use bracket match for block boundaries if available
	if e.hasBracketMatch && e.bracketOpenRow >= 0 && e.bracketCloseRow >= 0 {
		// Get indentation from the line with the opening bracket
		openLine := e.buffer.GetLine(e.bracketOpenRow)
		indent := e.getLineIndent(openLine)

		if indent >= 0 {
			e.indentGuideCol = indent
			e.indentGuideStartRow = e.bracketOpenRow
			e.indentGuideEndRow = e.bracketCloseRow
		}
		return
	}

	// Fallback: use current line's indentation (no block range)
	line := e.buffer.GetLine(e.cursorRow)
	indent := e.getLineIndent(line)
	if indent > 0 {
		e.indentGuideCol = indent
	}
}

// shouldShowIndentGuide returns true if the indent guide should be shown for the given row.
func (e *CodeEditor) shouldShowIndentGuide(row int) bool {
	if e.indentGuideCol <= 0 {
		return false
	}

	// If we have block boundaries, only show within the block
	if e.indentGuideStartRow >= 0 && e.indentGuideEndRow >= 0 {
		return row >= e.indentGuideStartRow && row <= e.indentGuideEndRow
	}

	// Fallback: always show (no block boundaries)
	return true
}

// getLineIndent returns the visual indentation level of a line.
func (e *CodeEditor) getLineIndent(line []rune) int {
	if len(line) == 0 {
		return -1
	}

	indent := 0
	tabSize := e.buffer.GetTabSize()
	for _, ch := range line {
		if ch == ' ' {
			indent++
		} else if ch == '\t' {
			indent = ((indent / tabSize) + 1) * tabSize
		} else {
			break
		}
	}

	return indent
}

// isBracketHighlight checks if a position should be highlighted as a matching bracket.
func (e *CodeEditor) isBracketHighlight(row, col int) bool {
	if !e.hasBracketMatch {
		return false
	}

	if row == e.bracketOpenRow && col == e.bracketOpenCol {
		return true
	}

	if row == e.bracketCloseRow && col == e.bracketCloseCol {
		return true
	}

	return false
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
