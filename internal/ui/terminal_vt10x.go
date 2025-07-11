package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	
	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
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
	
	// PTY components
	ptm       *os.File
	cmd       *exec.Cmd
	shell     string
	
	// VT10x terminal emulator
	vt        vt10x.Terminal
	
	// Terminal state
	mode      TerminalMode
	hasFocus  bool
	isRunning bool
	app       *tview.Application
	
	// Mouse and screen state
	mouseReporting   bool  // Whether the terminal app has requested mouse events
	mouseSGRMode     bool  // Whether using SGR (1006) extended mouse reporting
	alternateScreen  bool  // Whether using alternate screen buffer (no scrollback)
	
	// Callbacks
	onModeChange func(mode TerminalMode)
	onExit       func() // Called when shell exits
	
	// Terminal size
	rows int
	cols int
	
	// Refresh control
	needsRedraw bool
	
	// Scrollback state
	viewportOffset  int              // Offset from bottom (0 = live view)
}

// NewVT10XTerminal creates a new VT10x-based terminal emulator
func NewVT10XTerminal() *VT10XTerminal {
	t := &VT10XTerminal{
		Box:           tview.NewBox(),
		shell:         os.Getenv("SHELL"),
		mode:          ModeApplication,
		rows:          24,  // Default size, will be updated on first draw
		cols:          80,  // Default size, will be updated on first draw
		viewportOffset:  0,
	}
	
	if t.shell == "" {
		t.shell = "/bin/bash"
	}
	
	// Create VT10x terminal with default size
	t.vt = vt10x.New(vt10x.WithSize(t.cols, t.rows))
	
	return t
}

// SetApp sets the application reference
func (t *VT10XTerminal) SetApp(app *tview.Application) {
	t.app = app
}

// Start starts the terminal emulator
func (t *VT10XTerminal) Start() error {
	if t.isRunning {
		return nil
	}
	
	// Create command
	t.cmd = exec.Command(t.shell)
	t.cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	
	// Start PTY
	ptm, err := pty.Start(t.cmd)
	if err != nil {
		return err
	}
	t.ptm = ptm
	t.isRunning = true
	
	// Set initial size
	if err := pty.Setsize(t.ptm, &pty.Winsize{
		Rows: uint16(t.rows),
		Cols: uint16(t.cols),
	}); err != nil {
		// Non-fatal
	}
	
	// Configure scrollback buffer size
	t.vt.SetMaxScrollback(1000)
	
	// Start reading from PTY
	go t.readPTY()
	
	// Start refresh loop
	go t.refreshLoop()
	
	return nil
}

// refreshLoop periodically refreshes the display
func (t *VT10XTerminal) refreshLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	
	for t.isRunning {
		select {
		case <-ticker.C:
			if t.needsRedraw && t.app != nil {
				t.app.QueueUpdateDraw(func() {})
				t.needsRedraw = false
			}
		}
	}
}

// readPTY reads output from PTY and feeds it to vt10x
func (t *VT10XTerminal) readPTY() {
	buf := make([]byte, 32768)
	
	for t.isRunning {
		n, err := t.ptm.Read(buf)
		if err != nil {
			if err != io.EOF {
				// Terminal closed
			}
			t.isRunning = false
			// Notify that the shell has exited
			if t.onExit != nil {
				go t.onExit() // Call in goroutine to avoid blocking
			}
			return
		}
		
		if n > 0 {
			// Scan for terminal control sequences before passing to vt10x
			t.scanForControlSequences(buf[:n])
			
			// Pass data directly to vt10x
			// Note: vt10x doesn't support RGB colors, so they will be ignored
			t.vt.Write(buf[:n])
			t.needsRedraw = true
		}
	}
}

// scanForControlSequences looks for mouse mode and alternate screen escape sequences
func (t *VT10XTerminal) scanForControlSequences(data []byte) {
	str := string(data)
	
	// Mouse reporting modes
	// 1000 = report button press
	// 1001 = highlight tracking (not supported but enable mouse)
	// 1002 = report motion on button press  
	// 1003 = report all mouse motions
	// 1005 = UTF-8 mode (deprecated, but still used by some apps)
	// 1006 = SGR extended reporting mode
	// 1015 = URXVT mode (not fully supported but enable mouse)
	
	// Debug: Log any escape sequences with ? (comment out in production)
	// if strings.Contains(str, "\x1b[?") {
	// 	fmt.Fprintf(os.Stderr, "DEBUG: Escape sequence in output: %q\n", str)
	// }
	
	// IMPORTANT: When a string contains both enable and disable sequences,
	// we need to use the LAST occurrence to determine the final state.
	// Check which comes last in the string.
	
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
		t.viewportOffset = 0
	}
	if strings.Contains(str, "\x1b[?1049l") || strings.Contains(str, "\x1b[?47l") ||
	   strings.Contains(str, "\x1b[?1047l") {
		t.alternateScreen = false
		// When leaving alternate screen, could restore scrollback
	}
}

// Stop stops the terminal emulator
func (t *VT10XTerminal) Stop() {
	if !t.isRunning {
		return
	}
	
	t.isRunning = false
	
	if t.ptm != nil {
		t.ptm.Close()
	}
	
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait()
	}
}

// Write writes data to the terminal
func (t *VT10XTerminal) Write(data []byte) (int, error) {
	if !t.isRunning || t.ptm == nil {
		return 0, nil
	}
	n, err := t.ptm.Write(data)
	// Force a redraw after input to update cursor position
	t.needsRedraw = true
	return n, err
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



// scrollUp scrolls the view up by n lines
func (t *VT10XTerminal) scrollUp(lines int) {
	// Get scrollback size from vt10x
	scrollbackSize := t.vt.ScrollbackLines()
	
	// Maximum scroll is scrollback + current screen - viewport height
	maxScroll := scrollbackSize
	if maxScroll < 0 {
		maxScroll = 0
	}
	
	t.viewportOffset += lines
	if t.viewportOffset > maxScroll {
		t.viewportOffset = maxScroll
	}
}

// scrollDown scrolls the view down by n lines
func (t *VT10XTerminal) scrollDown(lines int) {
	t.viewportOffset -= lines
	if t.viewportOffset < 0 {
		t.viewportOffset = 0
	}
}

// resetScroll returns to live view
func (t *VT10XTerminal) resetScroll() {
	t.viewportOffset = 0
}

// Draw draws the terminal
func (t *VT10XTerminal) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)
	
	x, y, width, height := t.GetInnerRect()
	
	// Update terminal size if changed
	if height != t.rows || width != t.cols {
		oldRows, oldCols := t.rows, t.cols
		t.rows = height
		t.cols = width
		
		// Resize the vt10x terminal
		t.vt.Resize(width, height)
		
		if t.isRunning && t.ptm != nil {
			// Set the PTY size
			err := pty.Setsize(t.ptm, &pty.Winsize{
				Rows: uint16(t.rows),
				Cols: uint16(t.cols),
				X:    uint16(width * 8),  // Pixel width (approximate)
				Y:    uint16(height * 16), // Pixel height (approximate)
			})
			if err == nil && (oldRows != 0 || oldCols != 0) {
				// Force a redraw after resize
				t.needsRedraw = true
			}
		}
	}
	
	// Draw mode indicator
	if t.mode == ModePassthrough && t.hasFocus {
		modeText := "[TERMINAL MODE]"
		modeX := x + width - len(modeText)
		if modeX > x {
			for i, ch := range modeText {
				screen.SetContent(modeX+i, y, ch, nil, 
					tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true))
			}
		}
	}
	
	// Draw scroll indicator if scrolled
	if t.viewportOffset > 0 {
		scrollText := fmt.Sprintf("[SCROLLED: -%d lines]", t.viewportOffset)
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
	// Lock the vt10x terminal for reading
	t.vt.Lock()
	defer t.vt.Unlock()
	
	// If scrolled, draw from scrollback buffer
	if t.viewportOffset > 0 {
		// Get total lines (scrollback + current screen)
		scrollbackSize := t.vt.ScrollbackLines()
		totalLines := scrollbackSize + t.rows
		
		// Calculate viewport
		// viewportOffset 0 = show last 'height' lines (live view)
		// viewportOffset N = show N lines up from bottom
		viewEnd := totalLines - t.viewportOffset
		viewStart := viewEnd - height
		if viewStart < 0 {
			viewStart = 0
		}
		if viewEnd > totalLines {
			viewEnd = totalLines
		}
		
		// Draw the viewport
		screenRow := 0
		scrollbackSize = t.vt.ScrollbackLines()
		
		for i := viewStart; i < viewEnd && screenRow < height; i++ {
			if i >= 0 {
				if i < scrollbackSize {
					// Draw from scrollback
					line := t.vt.GetScrollbackLine(i)
					for col := 0; col < width && col < len(line); col++ {
						glyph := line[col]
						style := vt10xToTcellStyle(glyph)
						ch := glyph.Char
						if ch == 0 {
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
							style := vt10xToTcellStyle(glyph)
							ch := glyph.Char
							if ch == 0 {
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
				if ch == 0 {
					ch = ' '
				}
				cursorStyle := tcell.StyleDefault.
					Background(tcell.ColorWhite).
					Foreground(tcell.ColorBlack)
				screen.SetContent(x+col, y+row, ch, nil, cursorStyle)
			} else {
				// Normal rendering
				style := vt10xToTcellStyle(glyph)
				ch := glyph.Char
				if ch == 0 {
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
				if ch == 0 {
					ch = ' '
				}
				
				// Always draw cursor as white background with black text
				// This creates the classic "block cursor" effect
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

// vt10xToTcellStyle converts vt10x glyph to tcell style
func vt10xToTcellStyle(glyph vt10x.Glyph) tcell.Style {
	// Start with a clean style - explicitly set italic to false
	style := tcell.StyleDefault.Italic(false)
	
	// Debug: Log color values for specific characters (disabled)
	// if glyph.Char == 'T' || glyph.Char == 'r' || glyph.Char == 'y' {
	//     f, _ := os.OpenFile("/tmp/color_debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	//     fmt.Fprintf(f, "Char: %c, FG: %d (0x%x), BG: %d (0x%x)\n", glyph.Char, glyph.FG, glyph.FG, glyph.BG, glyph.BG)
	//     f.Close()
	// }
	
	// Convert foreground color
	fg := glyph.FG
	
	// Check if this is the default color first (vt10x.DefaultFG = 1<<24)
	if fg == vt10x.DefaultFG {
		// Default foreground - use light gray to match typical terminal behavior
		// Most terminals use a slightly dimmed white as the default
		style = style.Foreground(tcell.ColorLightGray)
	} else if fg == vt10x.DefaultBG {
		// When foreground is DefaultBG, it means cursor inversion - use black
		style = style.Foreground(tcell.ColorBlack)
	} else if fg == vt10x.DefaultCursor {
		// Default cursor - use white
		style = style.Foreground(tcell.ColorWhite)
	} else if fg >= 0 && fg <= 7 {
		// Standard colors
		colors := []tcell.Color{
			tcell.ColorBlack,
			tcell.ColorRed,
			tcell.ColorGreen,
			tcell.ColorYellow,
			tcell.ColorBlue,
			tcell.ColorPurple,
			tcell.ColorTeal,
			tcell.ColorWhite,
		}
		style = style.Foreground(colors[fg])
	} else if fg >= 8 && fg <= 15 {
		// Bright colors
		colors := []tcell.Color{
			tcell.ColorDarkGray,    // 8 - bright black (dark gray)
			tcell.ColorRed,         // 9 - bright red
			tcell.ColorLightGreen,  // 10 - bright green
			tcell.ColorLightYellow, // 11 - bright yellow
			tcell.ColorLightBlue,   // 12 - bright blue
			tcell.ColorPurple,      // 13 - bright magenta
			tcell.ColorLightCyan,   // 14 - bright cyan
			tcell.ColorLightGray,   // 15 - bright white
		}
		style = style.Foreground(colors[fg-8])
	} else if fg >= 16 && fg <= 255 {
		// 256 color palette
		style = style.Foreground(tcell.PaletteColor(int(fg)))
	} else if fg > 255 && fg < vt10x.DefaultFG {
		// RGB color encoding for values between 256 and DefaultFG
		// VT10x uses this range for RGB colors encoded as 0xRRGGBB
		// For example, gray 0x999999 means RGB(153,153,153)
		r := uint8((fg >> 16) & 0xFF)
		g := uint8((fg >> 8) & 0xFF)
		b := uint8(fg & 0xFF)
		
		// Create RGB color directly - this should work for true color
		// Map to the closest gray level for better compatibility
		// Gray value 0x999999 (153,153,153) should appear as a medium gray
		if r == 153 && g == 153 && b == 153 {
			// This is the typical placeholder gray
			style = style.Foreground(tcell.ColorDarkGray)
		} else {
			// Other RGB colors
			style = style.Foreground(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
		}
	} else {
		// For any other value, use white as fallback
		style = style.Foreground(tcell.ColorWhite)
	}
	
	// Convert background color
	bg := glyph.BG
	
	// Check if this is the default color first (vt10x.DefaultBG = 1<<24 + 1)
	if bg == vt10x.DefaultBG {
		// Default background - use black
		style = style.Background(tcell.ColorBlack)
	} else if bg == vt10x.DefaultFG {
		// When background is DefaultFG, it means cursor inversion - use white
		style = style.Background(tcell.ColorWhite)
	} else if bg == vt10x.DefaultCursor {
		// Default cursor background - use black
		style = style.Background(tcell.ColorBlack)
	} else if bg >= 0 && bg <= 7 {
		// Standard colors
		colors := []tcell.Color{
			tcell.ColorBlack,
			tcell.ColorRed,
			tcell.ColorGreen,
			tcell.ColorYellow,
			tcell.ColorBlue,
			tcell.ColorPurple,
			tcell.ColorTeal,
			tcell.ColorWhite,
		}
		style = style.Background(colors[bg])
	} else if bg >= 8 && bg <= 15 {
		// Bright colors
		colors := []tcell.Color{
			tcell.ColorDarkGray,
			tcell.ColorRed,
			tcell.ColorLightGreen,
			tcell.ColorLightYellow,
			tcell.ColorLightBlue,
			tcell.ColorPurple,
			tcell.ColorLightCyan,
			tcell.ColorLightGray,
		}
		style = style.Background(colors[bg-8])
	} else if bg >= 16 && bg <= 255 {
		// 256 color palette
		style = style.Background(tcell.PaletteColor(int(bg)))
	} else if bg > 255 && bg < vt10x.DefaultBG {
		// RGB color encoding for values between 256 and DefaultBG
		// VT10x uses this range for RGB colors encoded as 0xRRGGBB
		// For example, gray 0x999999 means RGB(153,153,153)
		r := uint8((bg >> 16) & 0xFF)
		g := uint8((bg >> 8) & 0xFF)
		b := uint8(bg & 0xFF)
		
		// Create RGB color directly - this should work for true color
		// tcell.NewRGBColor expects values in 0-255 range
		style = style.Background(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
	} else {
		// For any other value, use black as fallback
		style = style.Background(tcell.ColorBlack)
	}
	
	// Apply text attributes from Mode field
	// vt10x attribute bits:
	// Bit 0 (0x1): Reverse
	// Bit 1 (0x2): Underline
	// Bit 2 (0x4): Bold
	// Bit 3 (0x8): Gfx
	// Bit 4 (0x10): Italic
	// Bit 5 (0x20): Blink
	mode := glyph.Mode
	if mode&(1<<0) != 0 { // Reverse
		// For reverse video, swap foreground and background colors
		// This should make the cursor appear as inverted
		style = style.Reverse(true)
	}
	if mode&(1<<1) != 0 { // Underline  
		style = style.Underline(true)
	}
	if mode&(1<<2) != 0 { // Bold
		style = style.Bold(true)
		// If using default color with bold, make it bright white
		if fg == vt10x.DefaultFG {
			style = style.Foreground(tcell.ColorWhite)
		}
	}
	if mode&(1<<5) != 0 { // Blink
		style = style.Blink(true)
	}
	// Don't apply dim as it might be misinterpreted
	// if mode&(1<<5) != 0 { // Dim
	//     style = style.Dim(true)
	// }
	
	return style
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
		if !t.isRunning {
			return false, nil
		}
		
		// Get the position of the click relative to the box
		x, y := event.Position()
		rectX, rectY, width, height := t.GetInnerRect()
		
		// Calculate terminal-relative coordinates (1-based for terminal)
		// Even if outside bounds, we might need to report it
		termX := x - rectX + 1
		termY := y - rectY + 1
		
		// Clamp coordinates to valid terminal range if needed
		if termX < 1 {
			termX = 1
		}
		if termY < 1 {
			termY = 1
		}
		if termX > width {
			termX = width
		}
		if termY > height {
			termY = height
		}
		
		// Check if event is within the terminal bounds for focus handling
		inBounds := x >= rectX && x < rectX+width && y >= rectY && y < rectY+height
		
		if inBounds {
			// Handle focus on click actions (but don't block other events)
			switch action {
			case tview.MouseLeftClick, tview.MouseLeftDown, tview.MouseLeftDoubleClick,
				 tview.MouseRightClick, tview.MouseRightDown,
				 tview.MouseMiddleClick, tview.MouseMiddleDown:
				setFocus(t)
			}
		}
		
		
		// If the terminal application has requested mouse reporting, send ALL mouse events to it
		// This includes events outside the terminal bounds (for drag operations)
		if t.mouseReporting && t.ptm != nil {
			// Convert mouse action to terminal escape sequences
			// This includes up events, move events, etc.
			mouseSeq := t.generateMouseSequence(action, event, termX, termY)
			if mouseSeq != "" {
				t.ptm.Write([]byte(mouseSeq))
			}
			// Always consume mouse events when mouse reporting is on
			// This ensures consistent behavior for terminal applications
			return true, nil
		}
		
		// Only handle local mouse events if within bounds
		if inBounds {
			// Otherwise, handle mouse events locally (when mouse reporting is off)
			switch action {
			case tview.MouseScrollUp:
				// Only allow scrollback if not in alternate screen
				if !t.alternateScreen {
					t.scrollUp(1)  // Scroll 1 line at a time for precise control
				}
				return true, nil
			case tview.MouseScrollDown:
				// Only allow scrollback if not in alternate screen
				if !t.alternateScreen {
					t.scrollDown(1)  // Scroll 1 line at a time for precise control
				}
				return true, nil
			case tview.MouseLeftClick, tview.MouseLeftDoubleClick,
				 tview.MouseRightClick, tview.MouseMiddleClick:
				// Clicks reset scroll position
				if !t.alternateScreen {
					t.resetScroll()
				}
				return true, nil
			case tview.MouseLeftDown:
				// Capture for potential drag operations
				return true, t
			case tview.MouseLeftUp, tview.MouseMove:
				// Consume these to prevent unexpected behavior
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
		isRelease = false
	case tview.MouseLeftUp:
		button = 0
		isRelease = true
	case tview.MouseMiddleClick, tview.MouseMiddleDown:
		button = 1
		isRelease = false
	case tview.MouseMiddleUp:
		button = 1
		isRelease = true
	case tview.MouseRightClick, tview.MouseRightDown:
		button = 2
		isRelease = false
	case tview.MouseRightUp:
		button = 2
		isRelease = true
	case tview.MouseScrollUp:
		button = 64  // Scroll up (button 4)
		isRelease = false
	case tview.MouseScrollDown:
		button = 65  // Scroll down (button 5)
		isRelease = false
	case tview.MouseMove:
		// Movement with button held
		buttons := event.Buttons()
		if buttons&tcell.Button1 != 0 {
			button = 32 // Movement with left button
		} else if buttons&tcell.Button2 != 0 {
			button = 33 // Movement with middle button
		} else if buttons&tcell.Button3 != 0 {
			button = 34 // Movement with right button
		} else {
			return "" // No button pressed during movement
		}
		isRelease = false
	default:
		return ""
	}
	
	// Check for modifier keys
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
		// SGR mode (1006): ESC[<button;x;y;M (press) or m (release)
		suffix := "M"
		if isRelease {
			suffix = "m"
		}
		return fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x, y, suffix)
	} else {
		// Normal mode (1000/1002/1003): ESC[Mbxy
		// Normal mode uses a different encoding:
		// - Button code + 32 (space) to make it printable
		// - x and y coordinates + 32 to make them printable
		// - Release adds 3 to the button code
		if isRelease {
			button += 3
		}
		
		// Ensure coordinates are within valid range (1-223 after adding 32)
		if x > 223 {
			x = 223
		}
		if y > 223 {
			y = 223
		}
		
		// In normal mode, button code and coordinates are offset by 32 to be printable
		return fmt.Sprintf("\x1b[M%c%c%c", byte(button+32), byte(x+32), byte(y+32))
	}
}

// InputHandler handles input events
func (t *VT10XTerminal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !t.hasFocus || !t.isRunning {
			return
		}
		
		// Reset scroll on any keyboard input (return to live view)
		if t.viewportOffset > 0 {
			t.resetScroll()
		}
		
		// Handle mode toggle (Alt+T for "Terminal mode")
		// Alt+T is more universal and works on all keyboard layouts
		if (event.Key() == tcell.KeyRune && event.Rune() == 't' && event.Modifiers()&tcell.ModAlt != 0) ||
		   (event.Key() == tcell.KeyRune && event.Rune() == 'T' && event.Modifiers()&tcell.ModAlt != 0) {
			if t.mode == ModeApplication {
				t.SetMode(ModePassthrough)
			} else {
				t.SetMode(ModeApplication)
			}
			return
		}
		
		// In Passthrough mode, ALL input goes to terminal except mode toggle
		if t.mode == ModePassthrough {
			// Everything goes to the terminal in passthrough mode
			// Mode toggle is already handled above
		} else if t.mode == ModeApplication {
			// In application mode, only block specific emed shortcuts
			// Check for specific keys that emed uses
			switch event.Key() {
			// Block F-keys that emed uses
			case tcell.KeyF1, tcell.KeyF2, tcell.KeyF3,
				tcell.KeyF4, tcell.KeyF5, tcell.KeyF6, tcell.KeyF7:
				return // These are emed shortcuts - pass back to app
			// Block specific Ctrl keys that emed uses
			case tcell.KeyCtrlA: // Focus File Manager
				return
			case tcell.KeyCtrlE: // Focus Editor  
				return
			case tcell.KeyCtrlT: // Focus Terminal
				return
			case tcell.KeyCtrlW: // Close Tab
				return
			case tcell.KeyCtrlL: // Cycle Proportions
				return
			case tcell.KeyCtrlP: // Toggle Position
				return
			case tcell.KeyCtrlQ: // Quit
				return
			}
			
			// Also check for Ctrl+Shift combinations (emed shortcuts)
			if event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0 {
				return // Pass to app for emed handling
			}
		}
		
		// Send input to terminal
		var data []byte
		
		switch event.Key() {
		// Handle the tcell KeyCtrl* constants directly
		case tcell.KeyCtrlSpace:
			data = []byte{0x00}
		case tcell.KeyCtrlA:
			data = []byte{0x01}
		case tcell.KeyCtrlB:
			data = []byte{0x02}
		case tcell.KeyCtrlC:
			data = []byte{0x03}
		case tcell.KeyCtrlD:
			data = []byte{0x04}
		case tcell.KeyCtrlE:
			data = []byte{0x05}
		case tcell.KeyCtrlF:
			data = []byte{0x06}
		case tcell.KeyCtrlG:
			data = []byte{0x07}
		case tcell.KeyCtrlH: // Same as KeyBackspace
			data = []byte{0x08}
		case tcell.KeyCtrlI: // Same as KeyTab
			data = []byte{0x09}
		case tcell.KeyCtrlJ:
			data = []byte{0x0a}
		case tcell.KeyCtrlK:
			data = []byte{0x0b}
		case tcell.KeyCtrlL:
			data = []byte{0x0c}
		case tcell.KeyCtrlM: // Same as KeyEnter
			data = []byte{0x0d}
		case tcell.KeyCtrlN:
			data = []byte{0x0e}
		case tcell.KeyCtrlO:
			data = []byte{0x0f}
		case tcell.KeyCtrlP:
			data = []byte{0x10}
		case tcell.KeyCtrlQ:
			data = []byte{0x11}
		case tcell.KeyCtrlR:
			data = []byte{0x12}
		case tcell.KeyCtrlS:
			data = []byte{0x13}
		case tcell.KeyCtrlT:
			data = []byte{0x14}
		case tcell.KeyCtrlU:
			data = []byte{0x15}
		case tcell.KeyCtrlV:
			data = []byte{0x16}
		case tcell.KeyCtrlW:
			data = []byte{0x17}
		case tcell.KeyCtrlX:
			data = []byte{0x18}
		case tcell.KeyCtrlY:
			data = []byte{0x19}
		case tcell.KeyCtrlZ:
			data = []byte{0x1a}
		case tcell.KeyCtrlBackslash:
			data = []byte{0x1c}
		case tcell.KeyCtrlRightSq:
			data = []byte{0x1d}
		case tcell.KeyCtrlCarat:
			data = []byte{0x1e}
		case tcell.KeyCtrlUnderscore:
			data = []byte{0x1f}
		case tcell.KeyBackspace2:
			data = []byte{0x7f}
		case tcell.KeyRune:
			// Check if Ctrl is pressed with a letter (fallback for cases not caught above)
			if event.Modifiers()&tcell.ModCtrl != 0 {
				// Convert to control character (Ctrl+A = 1, Ctrl+B = 2, etc.)
				r := event.Rune()
				if r >= 'a' && r <= 'z' {
					data = []byte{byte(r - 'a' + 1)}
				} else if r >= 'A' && r <= 'Z' {
					data = []byte{byte(r - 'A' + 1)}
				} else {
					// For other characters with Ctrl, just send the character
					data = []byte(string(r))
				}
			} else {
				data = []byte(string(event.Rune()))
			}
		case tcell.KeyUp:
			// Check if we're in application cursor mode
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOA") // Application mode
			} else {
				data = []byte("\x1b[A") // Normal mode
			}
		case tcell.KeyDown:
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOB")
			} else {
				data = []byte("\x1b[B")
			}
		case tcell.KeyRight:
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOC")
			} else {
				data = []byte("\x1b[C")
			}
		case tcell.KeyLeft:
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOD")
			} else {
				data = []byte("\x1b[D")
			}
		case tcell.KeyHome:
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOH") // Application mode
			} else {
				data = []byte("\x1b[H") // Normal mode
			}
		case tcell.KeyEnd:
			if t.vt.Mode()&vt10x.ModeAppCursor != 0 {
				data = []byte("\x1bOF") // Application mode
			} else {
				data = []byte("\x1b[F") // Normal mode
			}
		case tcell.KeyPgUp:
			data = []byte("\x1b[5~")
		case tcell.KeyPgDn:
			data = []byte("\x1b[6~")
		case tcell.KeyDelete:
			data = []byte("\x1b[3~")
		case tcell.KeyF1:
			data = []byte("\x1bOP")  // F1
		case tcell.KeyF2:
			data = []byte("\x1bOQ")  // F2
		case tcell.KeyF3:
			data = []byte("\x1bOR")  // F3
		case tcell.KeyF4:
			data = []byte("\x1bOS")  // F4
		case tcell.KeyF5:
			data = []byte("\x1b[15~") // F5
		case tcell.KeyF6:
			data = []byte("\x1b[17~") // F6
		case tcell.KeyF7:
			data = []byte("\x1b[18~") // F7
		case tcell.KeyF8:
			data = []byte("\x1b[19~") // F8
		case tcell.KeyF9:
			data = []byte("\x1b[20~") // F9
		case tcell.KeyF10:
			data = []byte("\x1b[21~") // F10
		case tcell.KeyF11:
			data = []byte("\x1b[23~") // F11
		case tcell.KeyF12:
			data = []byte("\x1b[24~") // F12
		}
		
		if len(data) > 0 {
			t.Write(data)
		}
	})
}