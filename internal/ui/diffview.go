package ui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/logging"
	"github.com/rivo/tview"
)

type DiffLineType int

const (
	DiffUnchanged DiffLineType = iota
	DiffAdded
	DiffRemoved
	DiffModified
	DiffPlaceholder
)

type AlignedLine struct {
	LeftText  string
	RightText string
	LeftType  DiffLineType
	RightType DiffLineType
	LeftNum   int
	RightNum  int
}

type DiffView struct {
	*tview.Box

	filePath    string
	tabName     string
	oldContent  string
	editedLines []string

	alignedLines  []AlignedLine
	changeIndices []int

	offsetRow         int
	currentChangeIdx  int
	focusedPane       int // 0 = none/modal, 1 = right (editor)
	cursorRow         int // cursor row in aligned lines
	cursorCol         int // cursor column in right pane content
	hasFocus          bool
	viewportHeight    int
	initialScrollDone bool

	resultChan chan string
	resolved   bool
	mu         sync.Mutex

	app *tview.Application
}

func NewDiffView(filePath, oldContent, newContent, tabName string) *DiffView {
	d := &DiffView{
		Box:         tview.NewBox(),
		filePath:    filePath,
		tabName:     tabName,
		oldContent:  oldContent,
		editedLines: strings.Split(newContent, "\n"),
		resultChan:  make(chan string, 1),
		focusedPane: 0,
	}

	d.computeAlignedDiff()
	d.findChangeIndices()
	return d
}

func (d *DiffView) SetApp(app *tview.Application) {
	d.app = app
}

func (d *DiffView) GetFilePath() string {
	return d.filePath
}

func (d *DiffView) GetTabName() string {
	if d.tabName != "" {
		return d.tabName
	}
	return "Diff: " + d.filePath
}

func (d *DiffView) WaitForResult() string {
	return <-d.resultChan
}

func (d *DiffView) Accept() {
	logging.Error("DiffView.Accept() called")
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.resolved {
		return
	}
	d.resolved = true
	d.resultChan <- "FILE_SAVED"
}

func (d *DiffView) Reject() {
	logging.Error("DiffView.Reject() called")
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.resolved {
		return
	}
	d.resolved = true
	d.resultChan <- "DIFF_REJECTED"
}

func (d *DiffView) GetNewContent() string {
	return strings.Join(d.editedLines, "\n")
}

func (d *DiffView) computeAlignedDiff() {
	oldLines := strings.Split(d.oldContent, "\n")
	newLines := d.editedLines

	lcs := computeLCSMatrix(oldLines, newLines)
	d.alignedLines = buildAlignedLines(oldLines, newLines, lcs)
}

func computeLCSMatrix(oldLines, newLines []string) [][]int {
	m := len(oldLines)
	n := len(newLines)

	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else {
				if lcs[i-1][j] > lcs[i][j-1] {
					lcs[i][j] = lcs[i-1][j]
				} else {
					lcs[i][j] = lcs[i][j-1]
				}
			}
		}
	}

	return lcs
}

func buildAlignedLines(oldLines, newLines []string, lcs [][]int) []AlignedLine {
	var result []AlignedLine
	i, j := len(oldLines), len(newLines)

	var stack []AlignedLine

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			stack = append(stack, AlignedLine{
				LeftText:  oldLines[i-1],
				RightText: newLines[j-1],
				LeftType:  DiffUnchanged,
				RightType: DiffUnchanged,
				LeftNum:   i,
				RightNum:  j,
			})
			i--
			j--
		} else if i > 0 && j > 0 && lcs[i-1][j] == lcs[i][j-1] {
			stack = append(stack, AlignedLine{
				LeftText:  oldLines[i-1],
				RightText: newLines[j-1],
				LeftType:  DiffModified,
				RightType: DiffModified,
				LeftNum:   i,
				RightNum:  j,
			})
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			stack = append(stack, AlignedLine{
				LeftText:  "",
				RightText: newLines[j-1],
				LeftType:  DiffPlaceholder,
				RightType: DiffAdded,
				LeftNum:   0,
				RightNum:  j,
			})
			j--
		} else if i > 0 {
			stack = append(stack, AlignedLine{
				LeftText:  oldLines[i-1],
				RightText: "",
				LeftType:  DiffRemoved,
				RightType: DiffPlaceholder,
				LeftNum:   i,
				RightNum:  0,
			})
			i--
		}
	}

	for k := len(stack) - 1; k >= 0; k-- {
		result = append(result, stack[k])
	}

	return result
}

func (d *DiffView) findChangeIndices() {
	d.changeIndices = nil
	for i, line := range d.alignedLines {
		if line.LeftType != DiffUnchanged || line.RightType != DiffUnchanged {
			if len(d.changeIndices) == 0 || d.changeIndices[len(d.changeIndices)-1] != i-1 {
				d.changeIndices = append(d.changeIndices, i)
			}
		}
	}
}

func (d *DiffView) Draw(screen tcell.Screen) {
	d.Box.DrawForSubclass(screen, d)

	x, y, width, height := d.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	buttonHeight := 1
	contentHeight := height - buttonHeight - 1
	d.viewportHeight = contentHeight - 2

	if !d.initialScrollDone && len(d.changeIndices) > 0 {
		d.initialScrollDone = true
		d.centerOnChange(0)
	}

	halfWidth := (width - 1) / 2
	leftX := x
	rightX := x + halfWidth + 1

	headerStyle := tcell.StyleDefault.Bold(true).Foreground(tcell.ColorWhite)
	leftHeader := "ORIGINAL"
	rightHeader := "MODIFIED"
	if d.focusedPane == 1 {
		rightHeader = "MODIFIED (editing)"
	}
	d.drawText(screen, leftX, y, halfWidth, leftHeader, headerStyle)
	d.drawText(screen, rightX, y, halfWidth, rightHeader, headerStyle)

	for col := x; col < x+width; col++ {
		screen.SetContent(col, y+1, '─', nil, tcell.StyleDefault.Foreground(tcell.ColorGray))
	}

	d.clampOffset()

	lineY := y + 2
	for i := d.offsetRow; i < len(d.alignedLines) && lineY < y+contentHeight; i++ {
		line := d.alignedLines[i]
		d.drawAlignedLine(screen, leftX, rightX, lineY, halfWidth, line, i)
		lineY++
	}

	for col := x; col < x+width; col++ {
		screen.SetContent(col, y+contentHeight, '─', nil, tcell.StyleDefault.Foreground(tcell.ColorGray))
	}

	buttonY := y + height - 1
	helpText := " [Tab] edit | [n/p] nav changes | [Enter] accept | [Esc] cancel "
	if d.focusedPane == 1 {
		helpText = " editing... | [Esc] stop editing "
	}

	helpStyle := tcell.StyleDefault.Foreground(tcell.ColorGray)
	d.drawText(screen, x, buttonY, width, helpText, helpStyle)

	screen.SetContent(x+halfWidth, y, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorGray))
	for row := y + 1; row < y+contentHeight; row++ {
		screen.SetContent(x+halfWidth, row, '│', nil, tcell.StyleDefault.Foreground(tcell.ColorGray))
	}

	if d.focusedPane == 1 && d.hasFocus {
		cursorScreenRow := d.cursorRow - d.offsetRow + y + 2
		if cursorScreenRow >= y+2 && cursorScreenRow < y+contentHeight {
			cursorScreenCol := rightX + 6 + d.cursorCol
			if cursorScreenCol < rightX+halfWidth-1 {
				screen.ShowCursor(cursorScreenCol, cursorScreenRow)
			}
		}
	}
}

func (d *DiffView) drawAlignedLine(screen tcell.Screen, leftX, rightX, y, width int, line AlignedLine, rowIdx int) {
	leftStyle := d.getStyleForType(line.LeftType)
	rightStyle := d.getStyleForType(line.RightType)

	leftNum := "     "
	rightNum := "     "

	if line.LeftNum > 0 {
		leftNum = d.formatLineNum(line.LeftNum, line.LeftType)
	}
	if line.RightNum > 0 {
		rightNum = d.formatLineNum(line.RightNum, line.RightType)
	}

	numStyle := tcell.StyleDefault.Foreground(tcell.ColorGray)

	if line.LeftType == DiffPlaceholder {
		leftStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
	}
	if line.RightType == DiffPlaceholder {
		rightStyle = tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
	}

	d.drawText(screen, leftX, y, 5, leftNum, numStyle)
	d.drawTextWithStyle(screen, leftX+5, y, width-6, line.LeftText, leftStyle, line.LeftType == DiffPlaceholder)

	d.drawText(screen, rightX, y, 5, rightNum, numStyle)
	d.drawTextWithStyle(screen, rightX+5, y, width-6, line.RightText, rightStyle, line.RightType == DiffPlaceholder)
}

func (d *DiffView) formatLineNum(num int, lineType DiffLineType) string {
	switch lineType {
	case DiffAdded:
		return formatNum(num, '+')
	case DiffRemoved:
		return formatNum(num, '-')
	case DiffModified:
		return formatNum(num, '~')
	default:
		return formatNum(num, ' ')
	}
}

func formatNum(num int, suffix rune) string {
	return fmt.Sprintf("%4d%c", num, suffix)
}

func (d *DiffView) getStyleForType(t DiffLineType) tcell.Style {
	switch t {
	case DiffAdded:
		return tcell.StyleDefault.Foreground(tcell.ColorGreen)
	case DiffRemoved:
		return tcell.StyleDefault.Foreground(tcell.ColorRed)
	case DiffModified:
		return tcell.StyleDefault.Foreground(tcell.ColorYellow)
	case DiffPlaceholder:
		return tcell.StyleDefault.Foreground(tcell.ColorDarkGray)
	default:
		return tcell.StyleDefault.Foreground(tcell.ColorWhite)
	}
}

func (d *DiffView) drawText(screen tcell.Screen, x, y, maxWidth int, text string, style tcell.Style) {
	col := 0
	for _, ch := range text {
		if col >= maxWidth {
			break
		}
		screen.SetContent(x+col, y, ch, nil, style)
		col++
	}
	for col < maxWidth {
		screen.SetContent(x+col, y, ' ', nil, style)
		col++
	}
}

func (d *DiffView) drawTextWithStyle(screen tcell.Screen, x, y, maxWidth int, text string, style tcell.Style, isPlaceholder bool) {
	if isPlaceholder {
		for col := 0; col < maxWidth; col++ {
			screen.SetContent(x+col, y, '·', nil, style)
		}
		return
	}
	d.drawText(screen, x, y, maxWidth, text, style)
}

func (d *DiffView) clampOffset() {
	if d.offsetRow < 0 {
		d.offsetRow = 0
	}
	maxOffset := len(d.alignedLines) - d.viewportHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if d.offsetRow > maxOffset {
		d.offsetRow = maxOffset
	}
}

func (d *DiffView) centerOnChange(changeIdx int) {
	if changeIdx < 0 || changeIdx >= len(d.changeIndices) {
		return
	}
	d.currentChangeIdx = changeIdx
	targetRow := d.changeIndices[changeIdx]

	d.offsetRow = targetRow - d.viewportHeight/2
	d.clampOffset()

	d.cursorRow = targetRow
	d.cursorCol = 0
}

func (d *DiffView) nextChange() {
	if len(d.changeIndices) == 0 {
		return
	}
	d.currentChangeIdx++
	if d.currentChangeIdx >= len(d.changeIndices) {
		d.currentChangeIdx = 0
	}
	d.centerOnChange(d.currentChangeIdx)
}

func (d *DiffView) prevChange() {
	if len(d.changeIndices) == 0 {
		return
	}
	d.currentChangeIdx--
	if d.currentChangeIdx < 0 {
		d.currentChangeIdx = len(d.changeIndices) - 1
	}
	d.centerOnChange(d.currentChangeIdx)
}

func (d *DiffView) Focus(delegate func(p tview.Primitive)) {
	d.hasFocus = true
	d.Box.Focus(delegate)
}

func (d *DiffView) Blur() {
	d.hasFocus = false
	d.Box.Blur()
}

func (d *DiffView) HasFocus() bool {
	return d.hasFocus
}

func (d *DiffView) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return d.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		key := event.Key()
		ch := event.Rune()

		if d.focusedPane == 1 {
			d.handleEditingInput(event)
			return
		}

		switch {
		case key == tcell.KeyEscape:
			d.Reject()
		case key == tcell.KeyEnter:
			d.Accept()
		case key == tcell.KeyTab:
			d.focusedPane = 1
			d.moveCursorToEditableRow()
		case key == tcell.KeyUp:
			d.offsetRow--
			d.clampOffset()
		case key == tcell.KeyDown:
			d.offsetRow++
			d.clampOffset()
		case key == tcell.KeyPgUp:
			d.offsetRow -= d.viewportHeight
			d.clampOffset()
		case key == tcell.KeyPgDn:
			d.offsetRow += d.viewportHeight
			d.clampOffset()
		case ch == 'n' || ch == 'N':
			d.nextChange()
		case ch == 'p' || ch == 'P':
			d.prevChange()
		}
	})
}

func (d *DiffView) handleEditingInput(event *tcell.EventKey) {
	key := event.Key()
	ch := event.Rune()

	switch key {
	case tcell.KeyEscape:
		d.focusedPane = 0
		return
	case tcell.KeyTab:
		d.focusedPane = 0
		return
	case tcell.KeyUp:
		d.moveCursorUp()
	case tcell.KeyDown:
		d.moveCursorDown()
	case tcell.KeyLeft:
		d.moveCursorLeft()
	case tcell.KeyRight:
		d.moveCursorRight()
	case tcell.KeyHome:
		d.cursorCol = 0
	case tcell.KeyEnd:
		d.cursorCol = d.getCurrentLineLength()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		d.deleteCharBackward()
	case tcell.KeyDelete:
		d.deleteCharForward()
	case tcell.KeyEnter:
		d.insertNewline()
	case tcell.KeyPgUp:
		d.offsetRow -= d.viewportHeight
		d.clampOffset()
	case tcell.KeyPgDn:
		d.offsetRow += d.viewportHeight
		d.clampOffset()
	case tcell.KeyRune:
		d.insertChar(ch)
	}

	d.ensureCursorVisible()
}

func (d *DiffView) moveCursorToEditableRow() {
	for i := d.offsetRow; i < len(d.alignedLines); i++ {
		if d.alignedLines[i].RightType != DiffPlaceholder {
			d.cursorRow = i
			d.cursorCol = 0
			return
		}
	}
	for i := 0; i < d.offsetRow && i < len(d.alignedLines); i++ {
		if d.alignedLines[i].RightType != DiffPlaceholder {
			d.cursorRow = i
			d.cursorCol = 0
			d.ensureCursorVisible()
			return
		}
	}
}

func (d *DiffView) moveCursorUp() {
	for i := d.cursorRow - 1; i >= 0; i-- {
		if d.alignedLines[i].RightType != DiffPlaceholder {
			d.cursorRow = i
			d.clampCursorCol()
			return
		}
	}
}

func (d *DiffView) moveCursorDown() {
	for i := d.cursorRow + 1; i < len(d.alignedLines); i++ {
		if d.alignedLines[i].RightType != DiffPlaceholder {
			d.cursorRow = i
			d.clampCursorCol()
			return
		}
	}
}

func (d *DiffView) moveCursorLeft() {
	if d.cursorCol > 0 {
		d.cursorCol--
	} else {
		for i := d.cursorRow - 1; i >= 0; i-- {
			if d.alignedLines[i].RightType != DiffPlaceholder {
				d.cursorRow = i
				d.cursorCol = len([]rune(d.alignedLines[i].RightText))
				return
			}
		}
	}
}

func (d *DiffView) moveCursorRight() {
	lineLen := d.getCurrentLineLength()
	if d.cursorCol < lineLen {
		d.cursorCol++
	} else {
		for i := d.cursorRow + 1; i < len(d.alignedLines); i++ {
			if d.alignedLines[i].RightType != DiffPlaceholder {
				d.cursorRow = i
				d.cursorCol = 0
				return
			}
		}
	}
}

func (d *DiffView) getCurrentLineLength() int {
	if d.cursorRow < 0 || d.cursorRow >= len(d.alignedLines) {
		return 0
	}
	return len([]rune(d.alignedLines[d.cursorRow].RightText))
}

func (d *DiffView) clampCursorCol() {
	lineLen := d.getCurrentLineLength()
	if d.cursorCol > lineLen {
		d.cursorCol = lineLen
	}
}

func (d *DiffView) ensureCursorVisible() {
	if d.cursorRow < d.offsetRow {
		d.offsetRow = d.cursorRow
	} else if d.cursorRow >= d.offsetRow+d.viewportHeight {
		d.offsetRow = d.cursorRow - d.viewportHeight + 1
	}
	d.clampOffset()
}

func (d *DiffView) getEditedLineIndex() int {
	if d.cursorRow < 0 || d.cursorRow >= len(d.alignedLines) {
		return -1
	}
	line := d.alignedLines[d.cursorRow]
	if line.RightNum <= 0 {
		return -1
	}
	return line.RightNum - 1
}

func (d *DiffView) insertChar(ch rune) {
	lineIdx := d.getEditedLineIndex()
	if lineIdx < 0 || lineIdx >= len(d.editedLines) {
		return
	}

	runes := []rune(d.editedLines[lineIdx])
	if d.cursorCol > len(runes) {
		d.cursorCol = len(runes)
	}

	newRunes := make([]rune, 0, len(runes)+1)
	newRunes = append(newRunes, runes[:d.cursorCol]...)
	newRunes = append(newRunes, ch)
	newRunes = append(newRunes, runes[d.cursorCol:]...)

	d.editedLines[lineIdx] = string(newRunes)
	d.cursorCol++

	d.recomputeDiff()
}

func (d *DiffView) deleteCharBackward() {
	lineIdx := d.getEditedLineIndex()
	if lineIdx < 0 {
		return
	}

	if d.cursorCol > 0 {
		runes := []rune(d.editedLines[lineIdx])
		if d.cursorCol <= len(runes) {
			newRunes := append(runes[:d.cursorCol-1], runes[d.cursorCol:]...)
			d.editedLines[lineIdx] = string(newRunes)
			d.cursorCol--
		}
	} else if lineIdx > 0 {
		prevLineLen := len([]rune(d.editedLines[lineIdx-1]))
		d.editedLines[lineIdx-1] += d.editedLines[lineIdx]
		d.editedLines = append(d.editedLines[:lineIdx], d.editedLines[lineIdx+1:]...)
		d.cursorCol = prevLineLen
	}

	d.recomputeDiff()
}

func (d *DiffView) deleteCharForward() {
	lineIdx := d.getEditedLineIndex()
	if lineIdx < 0 {
		return
	}

	runes := []rune(d.editedLines[lineIdx])
	if d.cursorCol < len(runes) {
		newRunes := append(runes[:d.cursorCol], runes[d.cursorCol+1:]...)
		d.editedLines[lineIdx] = string(newRunes)
	} else if lineIdx < len(d.editedLines)-1 {
		d.editedLines[lineIdx] += d.editedLines[lineIdx+1]
		d.editedLines = append(d.editedLines[:lineIdx+1], d.editedLines[lineIdx+2:]...)
	}

	d.recomputeDiff()
}

func (d *DiffView) insertNewline() {
	lineIdx := d.getEditedLineIndex()
	if lineIdx < 0 {
		return
	}

	runes := []rune(d.editedLines[lineIdx])
	if d.cursorCol > len(runes) {
		d.cursorCol = len(runes)
	}

	beforeCursor := string(runes[:d.cursorCol])
	afterCursor := string(runes[d.cursorCol:])

	d.editedLines[lineIdx] = beforeCursor

	newLines := make([]string, 0, len(d.editedLines)+1)
	newLines = append(newLines, d.editedLines[:lineIdx+1]...)
	newLines = append(newLines, afterCursor)
	newLines = append(newLines, d.editedLines[lineIdx+1:]...)
	d.editedLines = newLines

	d.cursorCol = 0

	d.recomputeDiff()
}

func (d *DiffView) recomputeDiff() {
	oldCursorRow := d.cursorRow
	var oldRightNum int
	if oldCursorRow >= 0 && oldCursorRow < len(d.alignedLines) {
		oldRightNum = d.alignedLines[oldCursorRow].RightNum
	}

	d.computeAlignedDiff()
	d.findChangeIndices()

	if oldRightNum > 0 {
		for i, line := range d.alignedLines {
			if line.RightNum == oldRightNum {
				d.cursorRow = i
				break
			}
		}
	}
}

func (d *DiffView) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y, width, height := d.GetInnerRect()
		mouseX, mouseY := event.Position()

		if mouseX < x || mouseX >= x+width || mouseY < y || mouseY >= y+height {
			return false, nil
		}

		halfWidth := (width - 1) / 2
		rightX := x + halfWidth + 1

		switch action {
		case tview.MouseLeftClick:
			setFocus(d)

			if mouseX >= rightX && mouseY >= y+2 && mouseY < y+height-2 {
				d.focusedPane = 1
				clickedRow := d.offsetRow + (mouseY - y - 2)
				if clickedRow >= 0 && clickedRow < len(d.alignedLines) {
					if d.alignedLines[clickedRow].RightType != DiffPlaceholder {
						d.cursorRow = clickedRow
						d.cursorCol = mouseX - rightX - 5
						if d.cursorCol < 0 {
							d.cursorCol = 0
						}
						d.clampCursorCol()
					}
				}
			} else {
				d.focusedPane = 0
			}
			return true, nil

		case tview.MouseScrollUp:
			d.offsetRow--
			d.clampOffset()
			return true, nil

		case tview.MouseScrollDown:
			d.offsetRow++
			d.clampOffset()
			return true, nil
		}

		return false, nil
	}
}
