package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/terminal/pty"
	"github.com/laborin/emed/internal/terminal/scrollback"
	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/vt10x"
	"github.com/rivo/tview"
)

// TerminalMode represents the input capture mode
type TerminalMode int

const (
	ModeApplication TerminalMode = iota // Emed shortcuts work, some input goes to terminal
	ModePassthrough                      // All input goes to terminal except escape combo
)

// VT10XTerminal is a terminal emulator using the vt10x library
type VT10XTerminal struct {
	*tview.Box
	
	// Core components
	ptyMgr    *pty.PTYManager
	scrollbuf *scrollback.ScrollbackBuffer
	
	// VT10x terminal emulator
	vt        vt10x.Terminal
	vtMutex   sync.Mutex // Protect vt10x access
	writeChan chan []byte // Channel for vt10x writes
	
	// Terminal state
	mode      TerminalMode
	hasFocus  bool
	app       *tview.Application
	
	// Mouse and screen state - kept for compatibility
	mouseReporting   bool
	mouseSGRMode     bool
	alternateScreen  bool
	
	// Callbacks
	onModeChange func(mode TerminalMode)
	onExit       func() // Called when shell exits
	onTitleChange func(title string) // Called when terminal title changes
	
	// Terminal size
	rows int
	cols int
	
	// Terminal title tracking
	lastTitle   string
	
	// Refresh control
	needsRedraw bool
	redrawMutex sync.Mutex
	done        chan struct{}
	
	// Write batching
	writeBuf    []byte
	writeMutex  sync.Mutex
	writeTimer  *time.Timer
}

// NewVT10XTerminal creates a new VT10x-based terminal emulator
func NewVT10XTerminal() *VT10XTerminal {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	
	t := &VT10XTerminal{
		Box:       tview.NewBox(),
		ptyMgr:    pty.New(shell),
		scrollbuf: scrollback.New(1000),
		mode:      ModeApplication,
		rows:      24,
		cols:      80,
		done:      make(chan struct{}),
		writeChan: make(chan []byte, 1000), // Large buffer to prevent blocking
	}
	
	// Create VT10x terminal with default size
	t.vt = vt10x.New(vt10x.WithSize(t.cols, t.rows))
	
	// Setup callbacks
	t.ptyMgr.SetOnData(t.onPTYData)
	t.ptyMgr.SetOnExit(t.onPTYExit)
	
	// Start write processor
	go t.writeProcessor()
	
	// Start refresh loop
	go t.refreshLoop()
	
	return t
}

// SetApp sets the application reference
func (t *VT10XTerminal) SetApp(app *tview.Application) {
	t.app = app
}

// Start starts the terminal emulator
func (t *VT10XTerminal) Start() error {
	if t.ptyMgr.IsRunning() {
		return nil
	}
	
	if err := t.ptyMgr.Start(t.cols, t.rows); err != nil {
		logging.Error("Failed to start PTY: %v", err)
		return err
	}
	
	// Configure scrollback buffer size
	t.vt.SetMaxScrollback(1000)
	
	return nil
}

// Stop stops the terminal emulator
func (t *VT10XTerminal) Stop() {
	if t.done != nil {
		select {
		case <-t.done:
			// Already closed
		default:
			close(t.done)
		}
	}
	t.ptyMgr.Stop()
}

// flushWrites sends buffered writes to PTY
func (t *VT10XTerminal) flushWrites() {
	t.writeMutex.Lock()
	if len(t.writeBuf) > 0 {
		data := t.writeBuf
		t.writeBuf = nil
		t.writeMutex.Unlock()
		
		// Write to PTY
		if err := t.ptyMgr.Write(data); err != nil {
			logging.Error("PTY write error: %v", err)
		}
	} else {
		t.writeMutex.Unlock()
	}
}

// Write writes data to the terminal with batching for rapid input
func (t *VT10XTerminal) Write(data []byte) (int, error) {
	if !t.ptyMgr.IsRunning() {
		return 0, nil
	}
	
	t.writeMutex.Lock()
	t.writeBuf = append(t.writeBuf, data...)
	
	// Cancel existing timer if any
	if t.writeTimer != nil {
		t.writeTimer.Stop()
	}
	
	// If buffer is getting large, flush immediately
	if len(t.writeBuf) > 128 {
		t.writeMutex.Unlock()
		t.flushWrites()
	} else {
		// Otherwise, wait a tiny bit to batch more input
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

// GetMode returns the current terminal mode
func (t *VT10XTerminal) GetMode() TerminalMode {
	return t.mode
}

// SetMode sets the terminal mode
func (t *VT10XTerminal) SetMode(mode TerminalMode) {
	if t.mode != mode {
		t.mode = mode
		if t.onModeChange != nil {
			t.onModeChange(mode)
		}
	}
}

// SetOnModeChange sets the mode change callback
func (t *VT10XTerminal) SetOnModeChange(handler func(TerminalMode)) {
	t.onModeChange = handler
}

// SetOnExit sets the exit callback
func (t *VT10XTerminal) SetOnExit(handler func()) {
	t.onExit = handler
}

// SetOnTitleChange sets the title change callback
func (t *VT10XTerminal) SetOnTitleChange(handler func(string)) {
	t.onTitleChange = handler
}

// pollTitle checks if vt10x has a new title
func (t *VT10XTerminal) pollTitle() {
	// vt10x captures title sequences internally
	// We need to poll for changes
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



// writeProcessor processes writes to vt10x in a single goroutine
func (t *VT10XTerminal) writeProcessor() {
	for {
		select {
		case <-t.done:
			return
		case data := <-t.writeChan:
			t.vtMutex.Lock()
			t.vt.Write(data)
			t.vtMutex.Unlock()
			
			// Check if title changed after write (outside the mutex)
			t.pollTitle()
			t.markNeedsRedraw()
		}
	}
}

// onPTYData handles data from PTY
func (t *VT10XTerminal) onPTYData(data []byte) {
	// Don't parse OSC sequences - let vt10x handle them
	// vt10x will capture title sequences internally
	
	// Scan for terminal control sequences before passing to vt10x
	t.scanForControlSequences(data)
	
	// Send to write channel (non-blocking with large buffer)
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	
	select {
	case t.writeChan <- dataCopy:
		// Successfully queued
	default:
		// Channel full - this shouldn't happen with 1000 buffer
		// but if it does, just drop the data
	}
}

// onPTYExit handles PTY exit
func (t *VT10XTerminal) onPTYExit() {
	if t.onExit != nil {
		go t.onExit() // Call in goroutine to avoid blocking
	}
}

// markNeedsRedraw marks the terminal as needing a redraw
func (t *VT10XTerminal) markNeedsRedraw() {
	t.redrawMutex.Lock()
	t.needsRedraw = true
	t.redrawMutex.Unlock()
}

// refreshLoop periodically refreshes the display
func (t *VT10XTerminal) refreshLoop() {
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

// scanForControlSequences looks for mouse mode and alternate screen escape sequences
func (t *VT10XTerminal) scanForControlSequences(data []byte) {
	// Quick check - if data is too small to contain these sequences, skip
	if len(data) < 5 {
		return
	}
	
	str := string(data)
	
	// Find the last occurrence of any enable/disable for main mouse modes
	lastEnablePos := -1
	lastDisablePos := -1
	
	// Check for enable positions
	for _, pattern := range []string{"1000h", "1001h", "1002h", "1003h", "1005h", "1015h"} {
		if pos := strings.LastIndex(str, pattern); pos > lastEnablePos {
			lastEnablePos = pos
		}
	}
	
	// Check for disable positions  
	for _, pattern := range []string{"1000l", "1001l", "1002l", "1003l", "1005l", "1015l"} {
		if pos := strings.LastIndex(str, pattern); pos > lastDisablePos {
			lastDisablePos = pos
		}
	}
	
	// Set mouse reporting based on which came last
	if lastEnablePos > lastDisablePos && lastEnablePos >= 0 {
		t.mouseReporting = true
	} else if lastDisablePos > lastEnablePos && lastDisablePos >= 0 {
		t.mouseReporting = false
	}
	
	// Handle SGR mode separately
	lastSGREnablePos := strings.LastIndex(str, "1006h")
	lastSGRDisablePos := strings.LastIndex(str, "1006l")
	
	if lastSGREnablePos > lastSGRDisablePos && lastSGREnablePos >= 0 {
		t.mouseSGRMode = true
	} else if lastSGRDisablePos > lastSGREnablePos && lastSGRDisablePos >= 0 {
		t.mouseSGRMode = false
	}
	
	// Alternate screen buffer
	if strings.Contains(str, "\x1b[?1049h") || strings.Contains(str, "\x1b[?47h") ||
	   strings.Contains(str, "\x1b[?1047h") {
		t.alternateScreen = true
		// When entering alternate screen, reset scroll
		t.scrollbuf.ResetScroll()
	}
	if strings.Contains(str, "\x1b[?1049l") || strings.Contains(str, "\x1b[?47l") ||
	   strings.Contains(str, "\x1b[?1047l") {
		t.alternateScreen = false
	}
}

// Draw draws the terminal
func (t *VT10XTerminal) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)
	
	x, y, width, height := t.GetInnerRect()
	
	// Update terminal size if changed
	if height != t.rows || width != t.cols {
		t.rows = height
		t.cols = width
		
		// Resize the vt10x terminal with mutex protection
		t.vtMutex.Lock()
		t.vt.Resize(width, height)
		t.vtMutex.Unlock()
		
		if t.ptyMgr.IsRunning() {
			t.ptyMgr.Resize(width, height)
		}
		
		t.markNeedsRedraw()
	}
	
	// Draw mode indicator
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
	
	// Draw scroll indicator if scrolled
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
	
	// Draw terminal content
	t.drawTerminalContent(screen, x, y, width, height)
}

// isControlChar checks if a rune is a control character that shouldn't be displayed
func isControlChar(ch rune) bool {
	// Filter out control characters except tab, newline, carriage return
	return ch < 0x20 && ch != '\t' && ch != '\n' && ch != '\r'
}

// drawTerminalContent draws the actual terminal content
func (t *VT10XTerminal) drawTerminalContent(screen tcell.Screen, x, y, width, height int) {
	// Lock our mutex to protect vt10x access
	t.vtMutex.Lock()
	defer t.vtMutex.Unlock()
	
	// If scrolled, implement scrollback rendering
	if t.scrollbuf.IsScrolled() {
		// Get scrollback size from vt10x
		scrollbackSize := t.vt.ScrollbackLines()
		totalLines := scrollbackSize + t.rows
		
		// Calculate viewport
		viewportOffset := t.scrollbuf.GetOffset()
		viewEnd := totalLines - viewportOffset
		viewStart := viewEnd - height
		if viewStart < 0 {
			viewStart = 0
		}
		if viewEnd > totalLines {
			viewEnd = totalLines
		}
		
		// Draw the viewport
		screenRow := 0
		for i := viewStart; i < viewEnd && screenRow < height; i++ {
			if i >= 0 {
				if i < scrollbackSize {
					// Draw from scrollback
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
					// Clear rest of line
					for col := len(line); col < width; col++ {
						screen.SetContent(x+col, y+screenRow, ' ', nil, tcell.StyleDefault)
					}
				} else {
					// Draw from current screen
					row := i - scrollbackSize
					if row >= 0 && row < t.rows {
						for col := 0; col < width; col++ {
							glyph := t.vt.Cell(col, row)
							style := t.vt10xToTcellStyle(glyph)
							ch := glyph.Char
							if ch == 0 || isControlChar(ch) {
								ch = ' '
							}
							screen.SetContent(x+col, y+screenRow, ch, nil, style)
						}
					}
				}
			}
			screenRow++
		}
		
		// Clear any remaining rows
		for ; screenRow < height; screenRow++ {
			for col := 0; col < width; col++ {
				screen.SetContent(x+col, y+screenRow, ' ', nil, tcell.StyleDefault)
			}
		}
		
		return // Don't draw live terminal when scrolled
	}
	
	// Draw live terminal (when not scrolled)
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			// Get glyph from vt10x
			glyph := t.vt.Cell(col, row)
			
			// Check if this cell has the cursor (Reverse mode or inverted colors)
			hasReverse := (glyph.Mode & (1<<0)) != 0
			isInverted := (glyph.FG == vt10x.DefaultBG && glyph.BG == vt10x.DefaultFG)
			
			if hasReverse || isInverted {
				// This is a cursor position marked by vt10x
				// Force it to render as a block cursor
				ch := glyph.Char
				if ch == 0 || isControlChar(ch) {
					ch = ' '
				}
				cursorStyle := tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)
				screen.SetContent(x+col, y+row, ch, nil, cursorStyle)
			} else {
				// Normal rendering
				style := t.vt10xToTcellStyle(glyph)
				ch := glyph.Char
				if ch == 0 || isControlChar(ch) {
					ch = ' '
				}
				screen.SetContent(x+col, y+row, ch, nil, style)
			}
		}
	}
	
	// Draw cursor if focused
	if t.hasFocus {
		cursorX, cursorY := t.getCursorPos()
		cursorVisible := t.vt.CursorVisible()
		
		// Always show cursor if terminal says it should be visible
		if cursorVisible {
			// Ensure cursor is within bounds
			if cursorX >= 0 && cursorX < width && cursorY >= 0 && cursorY < height {
				// Get the character at cursor position
				glyph := t.vt.Cell(cursorX, cursorY)
				ch := glyph.Char
				if ch == 0 || isControlChar(ch) {
					ch = ' '
				}
				
				// Always draw cursor as white background with black text
				cursorStyle := tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)
				
				screen.SetContent(x+cursorX, y+cursorY, ch, nil, cursorStyle)
			}
		}
	}
}

// getCursorPos gets the cursor position from vt10x
func (t *VT10XTerminal) getCursorPos() (int, int) {
	// VT10x tracks cursor internally
	cur := t.vt.Cursor()
	return cur.X, cur.Y
}

// Focus is called when the terminal gains focus
func (t *VT10XTerminal) Focus(delegate func(p tview.Primitive)) {
	t.hasFocus = true
	if t.Box != nil {
		t.Box.Focus(delegate)
	}
}

// HasFocus returns whether the terminal has focus
func (t *VT10XTerminal) HasFocus() bool {
	return t.hasFocus
}

// Blur is called when the terminal loses focus
func (t *VT10XTerminal) Blur() {
	t.hasFocus = false
	if t.Box != nil {
		t.Box.Blur()
	}
}

// MouseHandler handles mouse events
func (t *VT10XTerminal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !t.ptyMgr.IsRunning() {
			return false, nil
		}
		
		// Get the position of the click relative to the box
		x, y := event.Position()
		rectX, rectY, width, height := t.GetInnerRect()
		
		// Calculate terminal-relative coordinates
		termX := x - rectX + 1
		termY := y - rectY + 1
		
		// Clamp coordinates
		if termX < 1 { termX = 1 }
		if termY < 1 { termY = 1 }
		if termX > width { termX = width }
		if termY > height { termY = height }
		
		inBounds := x >= rectX && x < rectX+width && y >= rectY && y < rectY+height
		
		if inBounds {
			// Handle focus on click
			switch action {
			case tview.MouseLeftClick, tview.MouseLeftDown, tview.MouseLeftDoubleClick,
				 tview.MouseRightClick, tview.MouseRightDown,
				 tview.MouseMiddleClick, tview.MouseMiddleDown:
				setFocus(t)
			}
		}
		
		// Handle mouse reporting
		if t.mouseReporting && t.ptyMgr.IsRunning() {
			mouseSeq := t.generateMouseSequence(action, event, termX, termY)
			if mouseSeq != "" {
				t.ptyMgr.Write([]byte(mouseSeq))
			}
			return true, nil
		}
		
		// Handle local mouse events
		if inBounds {
			switch action {
			case tview.MouseScrollUp:
				if !t.alternateScreen {
					t.scrollbuf.ScrollUp(1)
				}
				return true, nil
			case tview.MouseScrollDown:
				if !t.alternateScreen {
					t.scrollbuf.ScrollDown(1)
				}
				return true, nil
			case tview.MouseLeftClick, tview.MouseLeftDoubleClick,
				 tview.MouseRightClick, tview.MouseMiddleClick:
				if !t.alternateScreen {
					t.scrollbuf.ResetScroll()
				}
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

// generateMouseSequence generates terminal mouse escape sequences
func (t *VT10XTerminal) generateMouseSequence(action tview.MouseAction, event *tcell.EventMouse, x, y int) string {
	var button int
	var isRelease bool
	
	// Determine button code
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
	case tview.MouseScrollDown:
		button = 65
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
	
	// Add modifier keys
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
	
	// Generate sequence based on mode
	if t.mouseSGRMode {
		suffix := "M"
		if isRelease {
			suffix = "m"
		}
		return fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x, y, suffix)
	} else {
		if isRelease {
			button += 3
		}
		
		if x > 223 { x = 223 }
		if y > 223 { y = 223 }
		
		return fmt.Sprintf("\x1b[M%c%c%c", byte(button+32), byte(x+32), byte(y+32))
	}
}

// InputHandler handles input events
func (t *VT10XTerminal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !t.hasFocus || !t.ptyMgr.IsRunning() {
			return
		}
		
		// Reset scroll on any keyboard input
		if t.scrollbuf.IsScrolled() {
			t.scrollbuf.ResetScroll()
		}
		
		// Handle mode toggle (Alt+T)
		if (event.Key() == tcell.KeyRune && strings.ToLower(string(event.Rune())) == "t" && event.Modifiers()&tcell.ModAlt != 0) {
			if t.mode == ModeApplication {
				t.SetMode(ModePassthrough)
			} else {
				t.SetMode(ModeApplication)
			}
			return
		}
		
		// Handle mode-specific input
		if t.mode == ModeApplication {
			// Block specific emed shortcuts
			switch event.Key() {
			case tcell.KeyF1, tcell.KeyF2, tcell.KeyF3, tcell.KeyF4, tcell.KeyF5, tcell.KeyF6, tcell.KeyF7:
				return
			case tcell.KeyCtrlA, tcell.KeyCtrlE, tcell.KeyCtrlT, tcell.KeyCtrlW, tcell.KeyCtrlL, tcell.KeyCtrlP, tcell.KeyCtrlQ:
				return
			}
			
			// Block Ctrl+Shift combinations
			if event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0 {
				return
			}
		}
		
		// Convert and send input to terminal
		data := t.convertInputToBytes(event)
		if len(data) > 0 {
			t.Write(data)
		}
	})
}

// convertInputToBytes converts tcell key events to terminal input bytes
func (t *VT10XTerminal) convertInputToBytes(event *tcell.EventKey) []byte {
	switch event.Key() {
	// Control characters
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
	case tcell.KeyCtrlH: // Same as KeyBackspace
		return []byte{0x08}
	case tcell.KeyCtrlI: // Same as KeyTab
		return []byte{0x09}
	case tcell.KeyCtrlJ:
		return []byte{0x0a}
	case tcell.KeyCtrlK:
		return []byte{0x0b}
	case tcell.KeyCtrlL:
		return []byte{0x0c}
	case tcell.KeyCtrlM: // Same as KeyEnter
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
	case tcell.KeyBackspace2:
		return []byte{0x7f}
	
	// Arrow keys
	case tcell.KeyUp:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOA") // Application mode
		} else {
			return []byte("\x1b[A") // Normal mode
		}
	case tcell.KeyDown:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOB")
		} else {
			return []byte("\x1b[B")
		}
	case tcell.KeyRight:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOC")
		} else {
			return []byte("\x1b[C")
		}
	case tcell.KeyLeft:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOD")
		} else {
			return []byte("\x1b[D")
		}
	
	// Home/End
	case tcell.KeyHome:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOH") // Application mode
		} else {
			return []byte("\x1b[H") // Normal mode
		}
	case tcell.KeyEnd:
		if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
			return []byte("\x1bOF") // Application mode
		} else {
			return []byte("\x1b[F") // Normal mode
		}
	
	// Page Up/Down
	case tcell.KeyPgUp:
		return []byte("\x1b[5~")
	case tcell.KeyPgDn:
		return []byte("\x1b[6~")
	
	// Delete
	case tcell.KeyDelete:
		return []byte("\x1b[3~")
	
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
		if event.Modifiers()&tcell.ModCtrl != 0 {
			r := event.Rune()
			if r >= 'a' && r <= 'z' {
				return []byte{byte(r - 'a' + 1)}
			} else if r >= 'A' && r <= 'Z' {
				return []byte{byte(r - 'A' + 1)}
			} else {
				return []byte(string(r))
			}
		} else {
			return []byte(string(event.Rune()))
		}
	}
	
	return nil
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
		// Bright colors
		colors := []tcell.Color{
			tcell.ColorDarkGray, tcell.ColorRed, tcell.ColorLightGreen, tcell.ColorLightYellow,
			tcell.ColorLightBlue, tcell.ColorPurple, tcell.ColorLightCyan, tcell.ColorLightGray,
		}
		style = style.Foreground(colors[fg-8])
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
		colors := []tcell.Color{
			tcell.ColorDarkGray, tcell.ColorRed, tcell.ColorLightGreen, tcell.ColorLightYellow,
			tcell.ColorLightBlue, tcell.ColorPurple, tcell.ColorLightCyan, tcell.ColorLightGray,
		}
		style = style.Background(colors[bg-8])
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
	if mode&(1<<5) != 0 { // Blink
		style = style.Blink(true)
	}
	
	return style
}