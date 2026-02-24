// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/emed/internal/terminal/pty"
	"github.com/laborin/emed/internal/terminal/scrollback"
	"github.com/laborin/vt10x"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// TerminalMode represents the current terminal mode.
type TerminalMode int

// Terminal modes.
const (
	ModeApplication TerminalMode = iota
	ModePassthrough
)

// ptyWriter forwards terminal responses back to the PTY.
type ptyWriter struct {
	ptyMgr *pty.PTYManager
}

func (w *ptyWriter) Write(p []byte) (n int, err error) {
	if w.ptyMgr != nil && w.ptyMgr.IsRunning() {
		err = w.ptyMgr.Write(p)
		if err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// VT10XTerminal provides VT10X terminal emulation functionality.
type VT10XTerminal struct {
	*tview.Box

	ptyMgr    *pty.PTYManager
	scrollbuf *scrollback.ScrollbackBuffer

	vt        vt10x.Terminal
	vtMutex   sync.Mutex
	writeChan chan []byte

	mode     TerminalMode
	hasFocus bool
	app      *tview.Application

	mouseReporting     bool
	mouseSGRMode       bool
	alternateScreen    bool
	bracketedPasteMode bool

	onModeChange  func(mode TerminalMode)
	onExit        func()
	onTitleChange func(title string)
	onEmedCommand func(cmd string, args []string)

	rows int
	cols int

	lastTitle string

	needsRedraw bool
	redrawMutex sync.Mutex
	done        chan struct{}

	writeBuf   []byte
	writeMutex sync.Mutex
	writeTimer *time.Timer

	// Text selection state
	selectionActive bool
	selecting       bool
	selStartRow     int
	selStartCol     int
	selEndRow       int
	selEndCol       int
}

// NewVT10XTerminal creates a new VT10X terminal emulator instance.
func NewVT10XTerminal(workingDir string) *VT10XTerminal {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	t := &VT10XTerminal{
		Box:       tview.NewBox(),
		ptyMgr:    pty.New(shell, workingDir),
		scrollbuf: scrollback.New(1000),
		mode:      ModeApplication,
		rows:      24,
		cols:      80,
		done:      make(chan struct{}),
		writeChan: make(chan []byte, 1000),
	}

	t.vt = vt10x.New(
		vt10x.WithSize(t.cols, t.rows),
		vt10x.WithWriter(&ptyWriter{ptyMgr: t.ptyMgr}),
	)

	t.ptyMgr.SetOnData(t.onPTYData)
	t.ptyMgr.SetOnExit(t.onPTYExit)

	go t.writeProcessor()

	go t.refreshLoop()

	return t
}

// SetApp sets the tview application reference for the terminal.
func (t *VT10XTerminal) SetApp(app *tview.Application) {
	t.app = app
}

// SetExtraEnv sets extra environment variables for the terminal process.
func (t *VT10XTerminal) SetExtraEnv(env []string) {
	t.ptyMgr.SetExtraEnv(env)
}

// Start initializes and starts the terminal emulator.
func (t *VT10XTerminal) Start() error {
	if t.ptyMgr.IsRunning() {
		return nil
	}

	if err := t.ptyMgr.Start(t.cols, t.rows); err != nil {
		logging.Error("Failed to start PTY: %v", err)
		return err
	}

	t.vt.SetMaxScrollback(1000)

	return nil
}

// Stop terminates the terminal emulator and cleans up resources.
func (t *VT10XTerminal) Stop() {
	if t.done != nil {
		select {
		case <-t.done:
		default:
			close(t.done)
		}
	}
	t.ptyMgr.Stop()
}

func (t *VT10XTerminal) flushWrites() {
	t.writeMutex.Lock()
	if len(t.writeBuf) > 0 {
		data := t.writeBuf
		t.writeBuf = nil
		t.writeMutex.Unlock()

		if err := t.ptyMgr.Write(data); err != nil {
			logging.Error("PTY write error: %v", err)
		}
	} else {
		t.writeMutex.Unlock()
	}
}

// Write sends data to the terminal's stdin.
func (t *VT10XTerminal) Write(data []byte) (int, error) {
	if !t.ptyMgr.IsRunning() {
		return 0, nil
	}

	t.writeMutex.Lock()
	t.writeBuf = append(t.writeBuf, data...)

	if t.writeTimer != nil {
		t.writeTimer.Stop()
	}

	if len(t.writeBuf) > 128 {
		t.writeMutex.Unlock()
		t.flushWrites()
	} else {
		t.writeTimer = time.AfterFunc(time.Millisecond, t.flushWrites)
		t.writeMutex.Unlock()
	}

	return len(data), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetMode returns the current terminal mode.
func (t *VT10XTerminal) GetMode() TerminalMode {
	return t.mode
}

// GetPID returns the process ID of the terminal process.
func (t *VT10XTerminal) GetPID() int {
	if t.ptyMgr != nil {
		return t.ptyMgr.GetPID()
	}
	return 0
}

// SetMode sets the terminal mode (application or passthrough).
func (t *VT10XTerminal) SetMode(mode TerminalMode) {
	if t.mode != mode {
		t.mode = mode
		if t.onModeChange != nil {
			t.onModeChange(mode)
		}
	}
}

// SetOnModeChange sets a callback for terminal mode changes.
func (t *VT10XTerminal) SetOnModeChange(handler func(TerminalMode)) {
	t.onModeChange = handler
}

// SetOnExit sets a callback for when the terminal process exits.
func (t *VT10XTerminal) SetOnExit(handler func()) {
	t.onExit = handler
}

// SetOnTitleChange sets a callback for terminal title changes.
func (t *VT10XTerminal) SetOnTitleChange(handler func(string)) {
	t.onTitleChange = handler
}

// SetOnEmedCommand sets a callback for emed OSC commands from terminal output.
func (t *VT10XTerminal) SetOnEmedCommand(handler func(cmd string, args []string)) {
	t.onEmedCommand = handler
}

func (t *VT10XTerminal) pollTitle() {
	t.vtMutex.Lock()
	title := t.vt.Title()
	t.vtMutex.Unlock()

	if title != t.lastTitle && title != "" {
		t.lastTitle = title
		if t.onTitleChange != nil {
			t.onTitleChange(title)
		}
	}
}

func (t *VT10XTerminal) writeProcessor() {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("writeProcessor panic: %v", r)
		}
	}()

	for {
		select {
		case <-t.done:
			return
		case data := <-t.writeChan:
			t.vtMutex.Lock()
			t.vt.Write(data)
			t.vtMutex.Unlock()

			t.pollTitle()
			t.markNeedsRedraw()
		}
	}
}

func (t *VT10XTerminal) onPTYData(data []byte) {

	t.scanForControlSequences(data)

	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	select {
	case t.writeChan <- dataCopy:
	default:
	}
}

func (t *VT10XTerminal) onPTYExit() {
	if t.onExit != nil {
		go t.onExit()
	}
}

func (t *VT10XTerminal) markNeedsRedraw() {
	t.redrawMutex.Lock()
	t.needsRedraw = true
	t.redrawMutex.Unlock()
}

func (t *VT10XTerminal) refreshLoop() {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("refreshLoop panic: %v", r)
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			if !t.ptyMgr.IsRunning() {
				return
			}
			t.redrawMutex.Lock()
			if t.needsRedraw && t.app != nil {
				t.app.QueueUpdateDraw(func() {})
				t.needsRedraw = false
			}
			t.redrawMutex.Unlock()
		}
	}
}

func (t *VT10XTerminal) scanForControlSequences(data []byte) {
	str := string(data)

	// Mouse reporting modes - use full escape sequences
	mouseEnablePatterns := []string{
		"\x1b[?1000h", "\x1b[?1001h", "\x1b[?1002h", "\x1b[?1003h", "\x1b[?1005h", "\x1b[?1015h",
	}
	mouseDisablePatterns := []string{
		"\x1b[?1000l", "\x1b[?1001l", "\x1b[?1002l", "\x1b[?1003l", "\x1b[?1005l", "\x1b[?1015l",
	}

	lastEnablePos := -1
	lastDisablePos := -1

	for _, pattern := range mouseEnablePatterns {
		if pos := strings.LastIndex(str, pattern); pos > lastEnablePos {
			lastEnablePos = pos
		}
	}
	for _, pattern := range mouseDisablePatterns {
		if pos := strings.LastIndex(str, pattern); pos > lastDisablePos {
			lastDisablePos = pos
		}
	}

	if lastEnablePos > lastDisablePos && lastEnablePos >= 0 {
		t.mouseReporting = true
	} else if lastDisablePos > lastEnablePos && lastDisablePos >= 0 {
		t.mouseReporting = false
	}

	// SGR mouse mode
	lastSGREnablePos := strings.LastIndex(str, "\x1b[?1006h")
	lastSGRDisablePos := strings.LastIndex(str, "\x1b[?1006l")

	if lastSGREnablePos > lastSGRDisablePos && lastSGREnablePos >= 0 {
		t.mouseSGRMode = true
	} else if lastSGRDisablePos > lastSGREnablePos && lastSGRDisablePos >= 0 {
		t.mouseSGRMode = false
	}

	// Alternate screen buffer
	if strings.Contains(str, "\x1b[?1049h") || strings.Contains(str, "\x1b[?47h") ||
		strings.Contains(str, "\x1b[?1047h") {
		t.alternateScreen = true
		t.scrollbuf.ResetScroll()
	}
	if strings.Contains(str, "\x1b[?1049l") || strings.Contains(str, "\x1b[?47l") ||
		strings.Contains(str, "\x1b[?1047l") {
		t.alternateScreen = false
	}

	// Bracketed paste mode
	lastBracketEnablePos := strings.LastIndex(str, "\x1b[?2004h")
	lastBracketDisablePos := strings.LastIndex(str, "\x1b[?2004l")

	if lastBracketEnablePos > lastBracketDisablePos && lastBracketEnablePos >= 0 {
		t.bracketedPasteMode = true
	} else if lastBracketDisablePos > lastBracketEnablePos && lastBracketDisablePos >= 0 {
		t.bracketedPasteMode = false
	}

	t.parseEmedOSC(str)
}

func (t *VT10XTerminal) parseEmedOSC(str string) {
	const prefix = "\x1b]emed:"
	const suffix = "\x07"

	start := 0
	for {
		idx := strings.Index(str[start:], prefix)
		if idx == -1 {
			break
		}
		idx += start
		end := strings.Index(str[idx:], suffix)
		if end == -1 {
			break
		}
		end += idx

		cmdStr := str[idx+len(prefix) : end]
		parts := strings.Split(cmdStr, ":")
		if len(parts) > 0 && t.onEmedCommand != nil {
			t.onEmedCommand(parts[0], parts[1:])
		}

		start = end + len(suffix)
	}
}

// Draw renders the terminal content to the screen.
func (t *VT10XTerminal) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)

	x, y, width, height := t.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	if !t.vtMutex.TryLock() {
		return
	}

	currentCols, currentRows := t.vt.Size()
	needsResize := height != currentRows || width != currentCols
	if needsResize {
		t.vt.Resize(width, height)
		t.rows = height
		t.cols = width
		if t.ptyMgr.IsRunning() {
			t.ptyMgr.Resize(width, height)
		}
	}
	t.vtMutex.Unlock()

	if t.mode == ModePassthrough && t.hasFocus {
		modeText := "[TERMINAL MODE]"
		modeX := x + width - len(modeText)
		if modeX > x {
			for i, ch := range modeText {
				screen.SetContent(modeX+i, y, ch, nil,
					tcell.StyleDefault.Foreground(tcell.ColorBlue).Bold(true))
			}
		}
	}

	if t.scrollbuf.IsScrolled() {
		scrollText := fmt.Sprintf("[SCROLLED: -%d lines]", t.scrollbuf.GetOffset())
		scrollX := x + width - len(scrollText)
		scrollY := y + height - 1
		if scrollX > x {
			for i, ch := range scrollText {
				screen.SetContent(scrollX+i, scrollY, ch, nil,
					tcell.StyleDefault.Foreground(tcell.ColorAqua).Bold(true))
			}
		}
	}

	t.drawTerminalContent(screen, x, y, width, height)
}

func isControlChar(ch rune) bool {
	return ch < 0x20 && ch != '\t' && ch != '\n' && ch != '\r'
}

func (t *VT10XTerminal) drawTerminalContent(screen tcell.Screen, x, y, width, height int) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("drawTerminalContent panic: %v", r)
		}
	}()

	if !t.vtMutex.TryLock() {
		return
	}
	defer t.vtMutex.Unlock()

	vtCols, vtRows := t.vt.Size()
	if vtCols <= 0 || vtRows <= 0 {
		return
	}

	if t.scrollbuf.IsScrolled() {
		scrollbackSize := t.vt.ScrollbackLines()
		totalLines := scrollbackSize + vtRows

		viewportOffset := t.scrollbuf.GetOffset()
		viewEnd := totalLines - viewportOffset
		viewStart := viewEnd - height
		if viewStart < 0 {
			viewStart = 0
		}
		if viewEnd > totalLines {
			viewEnd = totalLines
		}

		screenRow := 0
		for i := viewStart; i < viewEnd && screenRow < height; i++ {
			if i >= 0 {
				if i < scrollbackSize {
					line := t.vt.GetScrollbackLine(i)
					for col := 0; col < width && col < len(line); col++ {
						glyph := line[col]
						style := t.vt10xToTcellStyle(glyph)
						ch := glyph.Char
						if ch == 0 || isControlChar(ch) {
							ch = ' '
						}
						screen.SetContent(x+col, y+screenRow, ch, nil, style)
					}
					for col := len(line); col < width; col++ {
						screen.SetContent(x+col, y+screenRow, ' ', nil, tcell.StyleDefault)
					}
				} else {
					row := i - scrollbackSize
					if row >= 0 && row < vtRows {
						for col := 0; col < width && col < vtCols; col++ {
							glyph := t.vt.Cell(col, row)
							style := t.vt10xToTcellStyle(glyph)
							ch := glyph.Char
							if ch == 0 || isControlChar(ch) {
								ch = ' '
							}
							screen.SetContent(x+col, y+screenRow, ch, nil, style)
						}
						for col := vtCols; col < width; col++ {
							screen.SetContent(x+col, y+screenRow, ' ', nil, tcell.StyleDefault)
						}
					}
				}
			}
			screenRow++
		}

		for ; screenRow < height; screenRow++ {
			for col := 0; col < width; col++ {
				screen.SetContent(x+col, y+screenRow, ' ', nil, tcell.StyleDefault)
			}
		}

		return
	}

	drawRows := min(height, vtRows)
	drawCols := min(width, vtCols)

	for row := 0; row < drawRows; row++ {
		col := 0
		for col < drawCols {
			glyph := t.vt.Cell(col, row)
			ch := glyph.Char
			if ch == 0 || isControlChar(ch) {
				ch = ' '
			}

			charWidth := runewidth.RuneWidth(ch)
			if charWidth == 0 {
				charWidth = 1
			}

			hasReverse := (glyph.Mode & (1 << 0)) != 0
			isInverted := (glyph.FG == vt10x.DefaultBG && glyph.BG == vt10x.DefaultFG)
			inSelection := t.isInSelection(row, col)

			var style tcell.Style
			if inSelection {
				style = tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)
			} else if hasReverse || isInverted {
				style = tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)
			} else {
				style = t.vt10xToTcellStyle(glyph)
			}

			screen.SetContent(x+col, y+row, ch, nil, style)

			// For wide characters, fill the next cell with a space
			if charWidth == 2 && col+1 < drawCols {
				screen.SetContent(x+col+1, y+row, ' ', nil, style)
			}

			col += charWidth
		}
		for ; col < width; col++ {
			selStyle := tcell.StyleDefault
			if t.isInSelection(row, col) {
				selStyle = tcell.StyleDefault.Background(tcell.ColorWhite).Foreground(tcell.ColorBlack)
			}
			screen.SetContent(x+col, y+row, ' ', nil, selStyle)
		}
	}

	for row := drawRows; row < height; row++ {
		for col := 0; col < width; col++ {
			screen.SetContent(x+col, y+row, ' ', nil, tcell.StyleDefault)
		}
	}

	if t.hasFocus {
		cursorX, cursorY := t.getCursorPos()
		cursorVisible := t.vt.CursorVisible()

		if cursorVisible {
			if cursorX >= 0 && cursorX < drawCols && cursorY >= 0 && cursorY < drawRows {
				glyph := t.vt.Cell(cursorX, cursorY)
				ch := glyph.Char
				if ch == 0 || isControlChar(ch) {
					ch = ' '
				}

				cursorStyle := tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)

				screen.SetContent(x+cursorX, y+cursorY, ch, nil, cursorStyle)
			}
		}
	}
}

func (t *VT10XTerminal) getCursorPos() (int, int) {
	cur := t.vt.Cursor()
	return cur.X, cur.Y
}

// Focus is called when the terminal gains focus.
func (t *VT10XTerminal) Focus(delegate func(p tview.Primitive)) {
	t.hasFocus = true
	if t.Box != nil {
		t.Box.Focus(delegate)
	}
}

// HasFocus returns whether the terminal currently has focus.
func (t *VT10XTerminal) HasFocus() bool {
	return t.hasFocus
}

// Blur is called when the terminal loses focus.
func (t *VT10XTerminal) Blur() {
	t.hasFocus = false
	if t.Box != nil {
		t.Box.Blur()
	}
}

// MouseHandler returns the mouse event handler for the terminal.
func (t *VT10XTerminal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !t.ptyMgr.IsRunning() {
			return false, nil
		}

		x, y := event.Position()
		rectX, rectY, width, height := t.GetInnerRect()

		termX := x - rectX
		termY := y - rectY

		if termX < 0 {
			termX = 0
		}
		if termY < 0 {
			termY = 0
		}
		if termX >= width {
			termX = width - 1
		}
		if termY >= height {
			termY = height - 1
		}

		inBounds := x >= rectX && x < rectX+width && y >= rectY && y < rectY+height

		if inBounds {
			switch action {
			case tview.MouseLeftClick, tview.MouseLeftDown, tview.MouseLeftDoubleClick,
				tview.MouseRightClick, tview.MouseRightDown,
				tview.MouseMiddleClick, tview.MouseMiddleDown:
				setFocus(t)
			}
		}

		if t.ptyMgr.IsRunning() && (t.mouseReporting || t.alternateScreen) {
			mouseSeq := t.generateMouseSequence(action, event, termX+1, termY+1)
			if mouseSeq != "" {
				t.ptyMgr.Write([]byte(mouseSeq))
				return true, nil
			}
		}

		// Text selection handling (only when app inside doesn't capture mouse)
		if !t.mouseReporting && !t.alternateScreen {
			switch action {
			case tview.MouseLeftDown:
				if inBounds {
					t.selecting = true
					t.selectionActive = false
					t.selStartRow = termY
					t.selStartCol = termX
					t.selEndRow = termY
					t.selEndCol = termX
				}

			case tview.MouseMove:
				if t.selecting {
					t.selEndRow = termY
					t.selEndCol = termX
					t.selectionActive = true
				}

			case tview.MouseLeftUp:
				if t.selecting {
					t.selecting = false
					t.selEndRow = termY
					t.selEndCol = termX
					if t.selStartRow != t.selEndRow || t.selStartCol != t.selEndCol {
						t.selectionActive = true
					}
				}
			}
		}

		if inBounds {
			switch action {
			case tview.MouseScrollUp:
				if !t.alternateScreen {
					t.vtMutex.Lock()
					scrollbackSize := t.vt.ScrollbackLines()
					t.vtMutex.Unlock()

					maxOffset := scrollbackSize
					if maxOffset < 0 {
						maxOffset = 0
					}

					t.scrollbuf.ScrollUpWithLimit(1, maxOffset)
					t.ClearSelection()
				}
				return true, nil
			case tview.MouseScrollDown:
				if !t.alternateScreen {
					t.scrollbuf.ScrollDown(1)
					t.ClearSelection()
				}
				return true, nil
			case tview.MouseLeftClick, tview.MouseLeftDoubleClick,
				tview.MouseRightClick, tview.MouseMiddleClick:
				if !t.alternateScreen {
					t.scrollbuf.ResetScroll()
				}
				t.ClearSelection()
				return true, nil
			case tview.MouseLeftDown:
				return true, t
			case tview.MouseLeftUp, tview.MouseMove:
				return true, nil
			}
		}

		return false, nil
	}
}

func (t *VT10XTerminal) generateMouseSequence(action tview.MouseAction, event *tcell.EventMouse, x, y int) string {
	var button int
	var isRelease bool
	isScrollEvent := false

	switch action {
	case tview.MouseLeftClick, tview.MouseLeftDown, tview.MouseLeftDoubleClick:
		button = 0
	case tview.MouseLeftUp:
		button = 0
		isRelease = true
	case tview.MouseMiddleClick, tview.MouseMiddleDown:
		button = 1
	case tview.MouseMiddleUp:
		button = 1
		isRelease = true
	case tview.MouseRightClick, tview.MouseRightDown:
		button = 2
	case tview.MouseRightUp:
		button = 2
		isRelease = true
	case tview.MouseScrollUp:
		button = 64
		isScrollEvent = true
	case tview.MouseScrollDown:
		button = 65
		isScrollEvent = true
	case tview.MouseMove:
		buttons := event.Buttons()
		if buttons&tcell.Button1 != 0 {
			button = 32
		} else if buttons&tcell.Button2 != 0 {
			button = 33
		} else if buttons&tcell.Button3 != 0 {
			button = 34
		} else {
			return ""
		}
	default:
		return ""
	}

	modifiers := event.Modifiers()
	if modifiers&tcell.ModShift != 0 {
		button += 4
	}
	if modifiers&tcell.ModAlt != 0 {
		button += 8
	}
	if modifiers&tcell.ModCtrl != 0 {
		button += 16
	}

	useSGR := t.mouseSGRMode || t.alternateScreen || isScrollEvent

	if useSGR {
		suffix := "M"
		if isRelease {
			suffix = "m"
		}
		return fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x, y, suffix)
	} else {
		if isRelease {
			button += 3
		}

		if x > 223 {
			x = 223
		}
		if y > 223 {
			y = 223
		}

		return fmt.Sprintf("\x1b[M%c%c%c", byte(button+32), byte(x+32), byte(y+32))
	}
}

// ClearSelection clears the current text selection.
func (t *VT10XTerminal) ClearSelection() {
	t.selectionActive = false
	t.selecting = false
}

// HasSelection returns true if there is an active selection.
func (t *VT10XTerminal) HasSelection() bool {
	return t.selectionActive
}

func (t *VT10XTerminal) normalizeSelection() (startRow, startCol, endRow, endCol int) {
	startRow, startCol = t.selStartRow, t.selStartCol
	endRow, endCol = t.selEndRow, t.selEndCol

	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, endRow = endRow, startRow
		startCol, endCol = endCol, startCol
	}
	return
}

func (t *VT10XTerminal) isInSelection(row, col int) bool {
	if !t.selectionActive {
		return false
	}

	startRow, startCol, endRow, endCol := t.normalizeSelection()

	if row < startRow || row > endRow {
		return false
	}
	if row == startRow && row == endRow {
		return col >= startCol && col <= endCol
	}
	if row == startRow {
		return col >= startCol
	}
	if row == endRow {
		return col <= endCol
	}
	return true
}

func (t *VT10XTerminal) getSelectedText() string {
	if !t.selectionActive {
		return ""
	}

	startRow, startCol, endRow, endCol := t.normalizeSelection()

	if !t.vtMutex.TryLock() {
		return ""
	}
	defer t.vtMutex.Unlock()

	var result strings.Builder
	_, _, width, _ := t.GetInnerRect()

	for row := startRow; row <= endRow; row++ {
		lineStart := 0
		lineEnd := width
		if row == startRow {
			lineStart = startCol
		}
		if row == endRow {
			lineEnd = endCol + 1
		}
		if lineEnd > width {
			lineEnd = width
		}

		var line strings.Builder
		for col := lineStart; col < lineEnd; col++ {
			glyph := t.vt.Cell(col, row)
			ch := glyph.Char
			if ch == 0 {
				ch = ' '
			}
			line.WriteRune(ch)
		}

		text := strings.TrimRight(line.String(), " ")
		result.WriteString(text)

		if row < endRow {
			result.WriteString("\n")
		}
	}

	return result.String()
}

func (t *VT10XTerminal) copySelectionToClipboard() {
	text := t.getSelectedText()
	if text != "" {
		copyToSystemClipboard(text)
	}
}

// CopySelection copies the current selection to the clipboard.
func (t *VT10XTerminal) CopySelection() {
	t.copySelectionToClipboard()
}

// PasteHandler returns the paste handler for the terminal.
func (t *VT10XTerminal) PasteHandler() func(text string, setFocus func(p tview.Primitive)) {
	return func(text string, setFocus func(p tview.Primitive)) {
		text = normalizePastedText(text)
		if !t.hasFocus || !t.ptyMgr.IsRunning() || text == "" {
			return
		}

		if t.scrollbuf.IsScrolled() {
			t.scrollbuf.ResetScroll()
		}

		var data []byte
		if t.bracketedPasteMode {
			data = make([]byte, 0, len(text)+12)
			data = append(data, "\x1b[200~"...)
			data = append(data, text...)
			data = append(data, "\x1b[201~"...)
		} else {
			data = []byte(text)
		}

		if err := t.ptyMgr.Write(data); err != nil {
			logging.Error("PTY paste write error: %v", err)
		}
	}
}

// InputHandler returns the keyboard input handler for the terminal.
func (t *VT10XTerminal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !t.hasFocus || !t.ptyMgr.IsRunning() {
			return
		}

		if t.scrollbuf.IsScrolled() {
			t.scrollbuf.ResetScroll()
		}

		if event.Key() == tcell.KeyRune && event.Modifiers()&tcell.ModMeta != 0 {
			if strings.ToLower(string(event.Rune())) == "c" && t.selectionActive {
				t.copySelectionToClipboard()
				t.ClearSelection()
				return
			}
		}

		// Handle Ctrl+C: if there's a selection, copy it instead of sending SIGINT
		if event.Key() == tcell.KeyCtrlC && t.selectionActive {
			t.copySelectionToClipboard()
			t.ClearSelection()
			return
		}

		// Clear selection on any other keyboard input
		if t.selectionActive {
			t.ClearSelection()
		}

		if event.Key() == tcell.KeyRune && strings.ToLower(string(event.Rune())) == "t" && event.Modifiers()&tcell.ModAlt != 0 {
			if t.mode == ModeApplication {
				t.SetMode(ModePassthrough)
			} else {
				t.SetMode(ModeApplication)
			}
			return
		}

		if t.mode == ModeApplication {
			switch event.Key() {
			case tcell.KeyF1, tcell.KeyF2, tcell.KeyF3, tcell.KeyF4, tcell.KeyF5, tcell.KeyF6, tcell.KeyF7:
				return
			case tcell.KeyCtrlA, tcell.KeyCtrlE, tcell.KeyCtrlT, tcell.KeyCtrlW, tcell.KeyCtrlL, tcell.KeyCtrlP, tcell.KeyCtrlQ:
				return
			}

			if event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0 {
				return
			}
		}

		data := t.convertInputToBytes(event)
		if len(data) > 0 {
			t.Write(data)
		}
	})
}

func (t *VT10XTerminal) convertInputToBytes(event *tcell.EventKey) []byte {
	if event.Key() == tcell.KeyEnter {
		if event.Modifiers()&tcell.ModAlt != 0 {
			return []byte("\x1b\r")
		}
		return []byte{0x0d}
	}

	if event.Key() == tcell.KeyBackspace2 {
		if event.Modifiers()&tcell.ModAlt != 0 {
			return []byte("\x1b\x7f")
		}
		return []byte{0x7f}
	}

	switch event.Key() {
	case tcell.KeyCtrlSpace:
		return []byte{0x00}
	case tcell.KeyCtrlA:
		return []byte{0x01}
	case tcell.KeyCtrlB:
		return []byte{0x02}
	case tcell.KeyCtrlC:
		return []byte{0x03}
	case tcell.KeyCtrlD:
		return []byte{0x04}
	case tcell.KeyCtrlE:
		return []byte{0x05}
	case tcell.KeyCtrlF:
		return []byte{0x06}
	case tcell.KeyCtrlG:
		return []byte{0x07}
	case tcell.KeyCtrlH:
		return []byte{0x08}
	case tcell.KeyCtrlI:
		return []byte{0x09}
	case tcell.KeyCtrlJ:
		return []byte{0x0a}
	case tcell.KeyCtrlK:
		return []byte{0x0b}
	case tcell.KeyCtrlL:
		return []byte{0x0c}
	case tcell.KeyCtrlM:
		return []byte{0x0d}
	case tcell.KeyCtrlN:
		return []byte{0x0e}
	case tcell.KeyCtrlO:
		return []byte{0x0f}
	case tcell.KeyCtrlP:
		return []byte{0x10}
	case tcell.KeyCtrlQ:
		return []byte{0x11}
	case tcell.KeyCtrlR:
		return []byte{0x12}
	case tcell.KeyCtrlS:
		return []byte{0x13}
	case tcell.KeyCtrlT:
		return []byte{0x14}
	case tcell.KeyCtrlU:
		return []byte{0x15}
	case tcell.KeyCtrlV:
		return []byte{0x16}
	case tcell.KeyCtrlW:
		return []byte{0x17}
	case tcell.KeyCtrlX:
		return []byte{0x18}
	case tcell.KeyCtrlY:
		return []byte{0x19}
	case tcell.KeyCtrlZ:
		return []byte{0x1a}
	case tcell.KeyCtrlBackslash:
		return []byte{0x1c}
	case tcell.KeyCtrlRightSq:
		return []byte{0x1d}
	case tcell.KeyCtrlCarat:
		return []byte{0x1e}
	case tcell.KeyCtrlUnderscore:
		return []byte{0x1f}

	case tcell.KeyEscape:
		return []byte{0x1b}

	case tcell.KeyUp:
		return t.arrowKeySequence('A', event.Modifiers())
	case tcell.KeyDown:
		return t.arrowKeySequence('B', event.Modifiers())
	case tcell.KeyRight:
		return t.arrowKeySequence('C', event.Modifiers())
	case tcell.KeyLeft:
		return t.arrowKeySequence('D', event.Modifiers())

	case tcell.KeyHome:
		mod := t.modifierCode(event.Modifiers())
		if mod > 1 {
			return []byte(fmt.Sprintf("\x1b[1;%dH", mod))
		}
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOH")
		}
		return []byte("\x1b[H")
	case tcell.KeyEnd:
		mod := t.modifierCode(event.Modifiers())
		if mod > 1 {
			return []byte(fmt.Sprintf("\x1b[1;%dF", mod))
		}
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOF")
		}
		return []byte("\x1b[F")

	case tcell.KeyInsert:
		return []byte("\x1b[2~")

	case tcell.KeyPgUp:
		mod := t.modifierCode(event.Modifiers())
		if mod > 1 {
			return []byte(fmt.Sprintf("\x1b[5;%d~", mod))
		}
		return []byte("\x1b[5~")
	case tcell.KeyPgDn:
		mod := t.modifierCode(event.Modifiers())
		if mod > 1 {
			return []byte(fmt.Sprintf("\x1b[6;%d~", mod))
		}
		return []byte("\x1b[6~")

	case tcell.KeyDelete:
		mod := t.modifierCode(event.Modifiers())
		if mod > 1 {
			return []byte(fmt.Sprintf("\x1b[3;%d~", mod))
		}
		return []byte("\x1b[3~")

	case tcell.KeyBacktab:
		return []byte("\x1b[Z")

	// Function keys
	case tcell.KeyF1:
		return []byte("\x1bOP")
	case tcell.KeyF2:
		return []byte("\x1bOQ")
	case tcell.KeyF3:
		return []byte("\x1bOR")
	case tcell.KeyF4:
		return []byte("\x1bOS")
	case tcell.KeyF5:
		return []byte("\x1b[15~")
	case tcell.KeyF6:
		return []byte("\x1b[17~")
	case tcell.KeyF7:
		return []byte("\x1b[18~")
	case tcell.KeyF8:
		return []byte("\x1b[19~")
	case tcell.KeyF9:
		return []byte("\x1b[20~")
	case tcell.KeyF10:
		return []byte("\x1b[21~")
	case tcell.KeyF11:
		return []byte("\x1b[23~")
	case tcell.KeyF12:
		return []byte("\x1b[24~")

	// Regular characters
	case tcell.KeyRune:
		r := event.Rune()
		if isLineBreakRune(r) {
			return []byte{0x0d}
		}
		if event.Modifiers()&tcell.ModAlt != 0 {
			return []byte{0x1b, byte(r)}
		}
		if event.Modifiers()&tcell.ModCtrl != 0 {
			if r >= 'a' && r <= 'z' {
				return []byte{byte(r - 'a' + 1)}
			} else if r >= 'A' && r <= 'Z' {
				return []byte{byte(r - 'A' + 1)}
			} else {
				return []byte(string(r))
			}
		} else {
			return []byte(string(r))
		}
	}

	return nil
}

func (t *VT10XTerminal) modifierCode(mod tcell.ModMask) int {
	code := 1
	if mod&tcell.ModShift != 0 {
		code += 1
	}
	if mod&tcell.ModAlt != 0 {
		code += 2
	}
	if mod&tcell.ModCtrl != 0 {
		code += 4
	}
	return code
}

func (t *VT10XTerminal) arrowKeySequence(arrow byte, mod tcell.ModMask) []byte {
	modCode := t.modifierCode(mod)
	if modCode > 1 {
		return []byte(fmt.Sprintf("\x1b[1;%d%c", modCode, arrow))
	}
	if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
		return []byte{0x1b, 'O', arrow}
	}
	return []byte{0x1b, '[', arrow}
}

// vt10xToTcellStyle converts vt10x glyph to tcell style
func (t *VT10XTerminal) vt10xToTcellStyle(glyph vt10x.Glyph) tcell.Style {
	// Start with a clean style
	style := tcell.StyleDefault.Italic(false)

	// Convert foreground color
	fg := glyph.FG

	if fg == vt10x.DefaultFG {
		style = style.Foreground(tcell.ColorLightGray)
	} else if fg == vt10x.DefaultBG {
		style = style.Foreground(tcell.ColorBlack)
	} else if fg == vt10x.DefaultCursor {
		style = style.Foreground(tcell.ColorWhite)
	} else if fg >= 0 && fg <= 7 {
		// Standard colors
		colors := []tcell.Color{
			tcell.ColorBlack, tcell.ColorRed, tcell.ColorGreen, tcell.ColorYellow,
			tcell.ColorBlue, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorWhite,
		}
		style = style.Foreground(colors[fg])
	} else if fg >= 8 && fg <= 15 {
		// Bright colors (8-15) - use palette for proper terminal colors
		style = style.Foreground(tcell.PaletteColor(int(fg)))
	} else if fg >= 16 && fg <= 255 {
		// 256 color palette
		style = style.Foreground(tcell.PaletteColor(int(fg)))
	} else if fg > 255 && fg < vt10x.DefaultFG {
		// RGB color encoding
		r := uint8((fg >> 16) & 0xFF)
		g := uint8((fg >> 8) & 0xFF)
		b := uint8(fg & 0xFF)

		if r == 153 && g == 153 && b == 153 {
			style = style.Foreground(tcell.ColorDarkGray)
		} else {
			style = style.Foreground(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
		}
	} else {
		style = style.Foreground(tcell.ColorWhite)
	}

	// Convert background color
	bg := glyph.BG

	if bg == vt10x.DefaultBG {
		style = style.Background(tcell.ColorBlack)
	} else if bg == vt10x.DefaultFG {
		style = style.Background(tcell.ColorWhite)
	} else if bg == vt10x.DefaultCursor {
		style = style.Background(tcell.ColorBlack)
	} else if bg >= 0 && bg <= 7 {
		colors := []tcell.Color{
			tcell.ColorBlack, tcell.ColorRed, tcell.ColorGreen, tcell.ColorYellow,
			tcell.ColorBlue, tcell.ColorPurple, tcell.ColorTeal, tcell.ColorWhite,
		}
		style = style.Background(colors[bg])
	} else if bg >= 8 && bg <= 15 {
		// Bright colors (8-15) - use palette for proper terminal colors
		style = style.Background(tcell.PaletteColor(int(bg)))
	} else if bg >= 16 && bg <= 255 {
		style = style.Background(tcell.PaletteColor(int(bg)))
	} else if bg > 255 && bg < vt10x.DefaultBG {
		r := uint8((bg >> 16) & 0xFF)
		g := uint8((bg >> 8) & 0xFF)
		b := uint8(bg & 0xFF)
		style = style.Background(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
	} else {
		style = style.Background(tcell.ColorBlack)
	}

	// Apply text attributes from Mode field
	// vt10x bit positions: 0=reverse, 1=underline, 2=bold, 3=gfx(internal), 4=italic, 5=blink
	mode := glyph.Mode
	if mode&(1<<0) != 0 { // Reverse
		style = style.Reverse(true)
	}
	if mode&(1<<1) != 0 { // Underline
		style = style.Underline(true)
	}
	if mode&(1<<2) != 0 { // Bold
		style = style.Bold(true)
		if fg == vt10x.DefaultFG {
			style = style.Foreground(tcell.ColorWhite)
		}
	}
	if mode&(1<<4) != 0 { // Italic
		style = style.Italic(true)
	}
	if mode&(1<<5) != 0 { // Blink
		style = style.Blink(true)
	}

	return style
}
