// Package emulator provides terminal emulation functionality using vt10x.
package emulator

import (
	"strings"
	"sync"
	
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/vt10x"
)

// TerminalEmulator wraps vt10x terminal emulator with additional features.
type TerminalEmulator struct {
	vt              *vt10x.VT
	mouseReporting  bool
	mouseSGRMode    bool
	alternateScreen bool
	mu              sync.RWMutex
	onUpdate        func()
}

// New creates a new terminal emulator with the given dimensions.
func New(cols, rows int) *TerminalEmulator {
	vt := vt10x.New(vt10x.WithSize(cols, rows))
	
	return &TerminalEmulator{
		vt: vt,
	}
}

// Write processes incoming data and updates the terminal state.
func (e *TerminalEmulator) Write(data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.scanForControlSequences(data)
	
	e.vt.Write(data)
	
	if e.onUpdate != nil {
		e.onUpdate()
	}
}

// Resize changes the terminal dimensions.
func (e *TerminalEmulator) Resize(cols, rows int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vt.Resize(cols, rows)
}

// GetState returns the current terminal state in a thread-safe manner.
func (e *TerminalEmulator) GetState() *vt10x.State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.vt.State()
}

// Lock acquires a read lock on the terminal state.
func (e *TerminalEmulator) Lock() {
	e.mu.RLock()
}

// Unlock releases the read lock on the terminal state.
func (e *TerminalEmulator) Unlock() {
	e.mu.RUnlock()
}

// Cell returns the character and attributes at the given position.
func (e *TerminalEmulator) Cell(col, row int) vt10x.Glyph {
	// Note: this assumes state is already locked by caller
	state := e.vt.State()
	if row < 0 || row >= len(state.Screen) {
		return vt10x.Glyph{}
	}
	if col < 0 || col >= len(state.Screen[row]) {
		return vt10x.Glyph{}
	}
	cell := state.Screen[row][col]
	return vt10x.Glyph{
		Char: cell.Char,
		FG:   uint32(cell.Attr.FG),
		BG:   uint32(cell.Attr.BG),
		Mode: uint32(cell.Attr),
	}
}

// Cursor returns the current cursor position.
func (e *TerminalEmulator) Cursor() vt10x.Cursor {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := e.vt.State()
	return vt10x.Cursor{
		X: state.Cursor.Col,
		Y: state.Cursor.Row,
	}
}

// CursorVisible returns true if the cursor should be displayed.
func (e *TerminalEmulator) CursorVisible() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := e.vt.State()
	return state.CursorVisible
}

// Mode returns the current terminal mode flags.
func (e *TerminalEmulator) Mode() uint32 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := e.vt.State()
	// Convert vt10x modes to uint32
	var mode uint32
	// Add specific mode flags as needed
	// This would need to be mapped based on the actual vt10x.State structure
	_ = state // placeholder
	return mode
}

// SetMaxScrollback sets the maximum number of scrollback lines.
func (e *TerminalEmulator) SetMaxScrollback(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// TODO: Implement scrollback limit in the underlying vt10x if supported
	_ = lines
}

// ScrollbackLines returns the current number of scrollback lines available.
func (e *TerminalEmulator) ScrollbackLines() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// TODO: Get actual scrollback size from vt10x
	return 0
}

// GetScrollbackLine returns a specific line from the scrollback buffer.
func (e *TerminalEmulator) GetScrollbackLine(index int) []vt10x.Glyph {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// TODO: Implement scrollback line retrieval
	_ = index
	return nil
}

// SetOnUpdate sets a callback function to be called when the terminal updates.
func (e *TerminalEmulator) SetOnUpdate(fn func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onUpdate = fn
}

// IsMouseReporting returns true if mouse events are being reported.
func (e *TerminalEmulator) IsMouseReporting() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mouseReporting
}

// IsMouseSGRMode returns true if SGR mouse reporting mode is active.
func (e *TerminalEmulator) IsMouseSGRMode() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mouseSGRMode
}

// IsAlternateScreen returns true if alternate screen buffer is active.
func (e *TerminalEmulator) IsAlternateScreen() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alternateScreen
}

func (e *TerminalEmulator) scanForControlSequences(data []byte) {
	str := string(data)
	
	if strings.Contains(str, "\x1b[?1000h") || strings.Contains(str, "\x1b[?1002h") || strings.Contains(str, "\x1b[?1003h") {
		e.mouseReporting = true
	}
	if strings.Contains(str, "\x1b[?1000l") || strings.Contains(str, "\x1b[?1002l") || strings.Contains(str, "\x1b[?1003l") {
		e.mouseReporting = false
	}
	
	if strings.Contains(str, "\x1b[?1006h") {
		e.mouseSGRMode = true
	}
	if strings.Contains(str, "\x1b[?1006l") {
		e.mouseSGRMode = false
	}
	
	if strings.Contains(str, "\x1b[?1049h") || strings.Contains(str, "\x1b[?47h") || strings.Contains(str, "\x1b[?1047h") {
		e.alternateScreen = true
	}
	if strings.Contains(str, "\x1b[?1049l") || strings.Contains(str, "\x1b[?47l") || strings.Contains(str, "\x1b[?1047l") {
		e.alternateScreen = false
	}
}

// ConvertVT10xToTcellStyle converts vt10x attributes to tcell style.
func ConvertVT10xToTcellStyle(attr vt10x.Attr) tcell.Style {
	style := tcell.StyleDefault
	
	fg := attr.FG
	if fg != vt10x.DefaultFG {
		if fg < 8 {
			colors := []tcell.Color{
				tcell.ColorBlack,
				tcell.ColorMaroon,
				tcell.ColorGreen,
				tcell.ColorOlive,
				tcell.ColorNavy,
				tcell.ColorPurple,
				tcell.ColorTeal,
				tcell.ColorSilver,
			}
			style = style.Foreground(colors[fg])
		} else if fg < 16 {
			colors := []tcell.Color{
				tcell.ColorGray,
				tcell.ColorRed,
				tcell.ColorLime,
				tcell.ColorYellow,
				tcell.ColorBlue,
				tcell.ColorFuchsia,
				tcell.ColorAqua,
				tcell.ColorWhite,
			}
			style = style.Foreground(colors[fg-8])
		} else if fg < 232 {
			r := ((fg - 16) / 36) * 51
			g := (((fg - 16) % 36) / 6) * 51
			b := ((fg - 16) % 6) * 51
			style = style.Foreground(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
		} else if fg < 256 {
			gray := 8 + (fg-232)*10
			style = style.Foreground(tcell.NewRGBColor(int32(gray), int32(gray), int32(gray)))
		}
	}
	
	bg := attr.BG
	if bg != vt10x.DefaultBG {
		if bg < 8 {
			colors := []tcell.Color{
				tcell.ColorBlack,
				tcell.ColorMaroon,
				tcell.ColorGreen,
				tcell.ColorOlive,
				tcell.ColorNavy,
				tcell.ColorPurple,
				tcell.ColorTeal,
				tcell.ColorSilver,
			}
			style = style.Background(colors[bg])
		} else if bg < 16 {
			colors := []tcell.Color{
				tcell.ColorGray,
				tcell.ColorRed,
				tcell.ColorLime,
				tcell.ColorYellow,
				tcell.ColorBlue,
				tcell.ColorFuchsia,
				tcell.ColorAqua,
				tcell.ColorWhite,
			}
			style = style.Background(colors[bg-8])
		} else if bg < 232 {
			r := ((bg - 16) / 36) * 51
			g := (((bg - 16) % 36) / 6) * 51
			b := ((bg - 16) % 6) * 51
			style = style.Background(tcell.NewRGBColor(int32(r), int32(g), int32(b)))
		} else if bg < 256 {
			gray := 8 + (bg-232)*10
			style = style.Background(tcell.NewRGBColor(int32(gray), int32(gray), int32(gray)))
		}
	}
	
	if attr&vt10x.AttrBold != 0 {
		style = style.Bold(true)
	}
	if attr&vt10x.AttrUnderline != 0 {
		style = style.Underline(true)
	}
	if attr&vt10x.AttrReverse != 0 {
		style = style.Reverse(true)
	}
	if attr&vt10x.AttrBlink != 0 {
		style = style.Blink(true)
	}
	if attr&vt10x.AttrDim != 0 {
		style = style.Dim(true)
	}
	
	return style
}

// GenerateMouseSequence creates mouse event sequences for terminal input.
func GenerateMouseSequence(col, row int, button tcell.ButtonMask, sgrMode bool) []byte {
	if sgrMode {
		buttonCode := 0
		switch button {
		case tcell.Button1:
			buttonCode = 0
		case tcell.Button2:
			buttonCode = 1
		case tcell.Button3:
			buttonCode = 2
		case tcell.WheelUp:
			buttonCode = 64
		case tcell.WheelDown:
			buttonCode = 65
		}
		
		return []byte(strings.Join([]string{
			"\x1b[<",
			string(rune(buttonCode)),
			";",
			string(rune(col + 1)),
			";",
			string(rune(row + 1)),
			"M",
		}, ""))
	} else {
		btn := 32
		switch button {
		case tcell.Button1:
			btn = 32
		case tcell.Button2:
			btn = 33
		case tcell.Button3:
			btn = 34
		case tcell.WheelUp:
			btn = 96
		case tcell.WheelDown:
			btn = 97
		}
		
		return []byte{
			'\x1b', '[', 'M',
			byte(btn),
			byte(col + 33),
			byte(row + 33),
		}
	}
}