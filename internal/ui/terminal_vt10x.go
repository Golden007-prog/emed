package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
	
	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
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
	
	// Callbacks
	onModeChange func(mode TerminalMode)
	
	// Terminal size
	rows int
	cols int
	
	// Refresh control
	needsRedraw bool
}

// NewVT10XTerminal creates a new VT10x-based terminal emulator
func NewVT10XTerminal() *VT10XTerminal {
	t := &VT10XTerminal{
		Box:   tview.NewBox(),
		shell: os.Getenv("SHELL"),
		mode:  ModeApplication,
		rows:  24,  // Default size, will be updated on first draw
		cols:  80,  // Default size, will be updated on first draw
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
			return
		}
		
		if n > 0 {
			// Pass data directly to vt10x
			// Note: vt10x doesn't support RGB colors, so they will be ignored
			t.vt.Write(buf[:n])
			t.needsRedraw = true
		}
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
	
	// Draw terminal content
	// Lock the vt10x terminal for reading
	t.vt.Lock()
	defer t.vt.Unlock()
	
	// Draw each cell from the terminal
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
		
		// Check if click is within the terminal bounds
		if x >= rectX && x < rectX+width && y >= rectY && y < rectY+height {
			// Calculate terminal-relative coordinates (1-based for terminal)
			termX := x - rectX + 1
			termY := y - rectY + 1
			
			// In passthrough mode, send mouse events to the terminal
			if t.mode == ModePassthrough && t.ptm != nil {
				// Only set focus on click actions, not move
				switch action {
				case tview.MouseLeftClick, tview.MouseLeftDown, tview.MouseLeftDoubleClick,
					 tview.MouseRightClick, tview.MouseRightDown,
					 tview.MouseMiddleClick, tview.MouseMiddleDown:
					setFocus(t)
				}
				
				// Convert mouse action to terminal escape sequences
				// This supports applications like vim with mouse mode, htop, etc.
				mouseSeq := t.generateMouseSequence(action, event, termX, termY)
				if mouseSeq != "" {
					t.ptm.Write([]byte(mouseSeq))
				}
				return true, nil
			}
			
			// In application mode, just handle focus
			switch action {
			case tview.MouseLeftClick, tview.MouseLeftDoubleClick,
				 tview.MouseRightClick, tview.MouseMiddleClick:
				// Request focus on click actions
				setFocus(t)
				// Consume the click to prevent propagation
				return true, nil
			case tview.MouseLeftDown:
				// Request focus and capture for potential drag operations
				setFocus(t)
				return true, t
			case tview.MouseLeftUp, tview.MouseMove:
				// Handle these to prevent unexpected behavior (but don't set focus)
				return true, nil
			}
		}
		
		return false, nil
	}
}

// generateMouseSequence generates terminal mouse escape sequences
func (t *VT10XTerminal) generateMouseSequence(action tview.MouseAction, event *tcell.EventMouse, x, y int) string {
	// Most terminals use SGR (1006) or normal (1000) mouse tracking
	// We'll use SGR format: ESC[<button;x;y;M (press) or m (release)
	
	var button int
	var suffix string
	
	switch action {
	case tview.MouseLeftClick, tview.MouseLeftDown:
		button = 0
		suffix = "M"
	case tview.MouseLeftUp:
		button = 0
		suffix = "m"
	case tview.MouseMiddleClick, tview.MouseMiddleDown:
		button = 1
		suffix = "M"
	case tview.MouseMiddleUp:
		button = 1
		suffix = "m"
	case tview.MouseRightClick, tview.MouseRightDown:
		button = 2
		suffix = "M"
	case tview.MouseRightUp:
		button = 2
		suffix = "m"
	case tview.MouseScrollUp:
		button = 64
		suffix = "M"
	case tview.MouseScrollDown:
		button = 65
		suffix = "M"
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
		suffix = "M"
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
	
	// Return SGR mouse sequence
	return fmt.Sprintf("\x1b[<%d;%d;%d%s", button, x, y, suffix)
}

// InputHandler handles input events
func (t *VT10XTerminal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if !t.hasFocus || !t.isRunning {
			return
		}
		
		// Handle mode toggle
		if event.Key() == tcell.KeyEsc && event.Modifiers() == tcell.ModCtrl {
			if t.mode == ModeApplication {
				t.SetMode(ModePassthrough)
			} else {
				t.SetMode(ModeApplication)
			}
			return
		}
		
		// In application mode, let some keys through to the app
		if t.mode == ModeApplication {
			// Let app handle navigation and control keys
			switch event.Key() {
			case tcell.KeyCtrlA, tcell.KeyCtrlE, tcell.KeyCtrlT,
				tcell.KeyCtrlW, tcell.KeyF1, tcell.KeyF2, tcell.KeyF3,
				tcell.KeyF4, tcell.KeyF5, tcell.KeyF6, tcell.KeyF7:
				// Pass to app
				return
			}
			
			// Also check for Ctrl+Shift combinations
			if event.Modifiers()&tcell.ModCtrl != 0 && event.Modifiers()&tcell.ModShift != 0 {
				return // Pass to app
			}
		}
		
		// Send input to terminal
		var data []byte
		
		switch event.Key() {
		case tcell.KeyRune:
			data = []byte(string(event.Rune()))
		case tcell.KeyEnter:
			data = []byte("\r")
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			data = []byte("\x7f")
		case tcell.KeyTab:
			data = []byte("\t")
		case tcell.KeyEscape:
			data = []byte("\x1b")
		case tcell.KeyUp:
			data = []byte("\x1b[A")
		case tcell.KeyDown:
			data = []byte("\x1b[B")
		case tcell.KeyRight:
			data = []byte("\x1b[C")
		case tcell.KeyLeft:
			data = []byte("\x1b[D")
		case tcell.KeyHome:
			data = []byte("\x1b[H")
		case tcell.KeyEnd:
			data = []byte("\x1b[F")
		case tcell.KeyPgUp:
			data = []byte("\x1b[5~")
		case tcell.KeyPgDn:
			data = []byte("\x1b[6~")
		case tcell.KeyDelete:
			data = []byte("\x1b[3~")
		case tcell.KeyCtrlC:
			data = []byte("\x03")
		case tcell.KeyCtrlD:
			data = []byte("\x04")
		case tcell.KeyCtrlZ:
			data = []byte("\x1a")
		case tcell.KeyCtrlL:
			data = []byte("\x0c")
		case tcell.KeyCtrlU:
			data = []byte("\x15")
		case tcell.KeyCtrlK:
			data = []byte("\x0b")
		}
		
		if len(data) > 0 {
			t.Write(data)
		}
	})
}