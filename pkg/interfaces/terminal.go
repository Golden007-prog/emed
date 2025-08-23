package interfaces

import (
	"io"
	
	"github.com/gdamore/tcell/v2"
)

// TerminalInterface defines the contract for terminal emulator implementations
type TerminalInterface interface {
	// Lifecycle
	Start(command string, args []string) error
	Stop() error
	Restart() error
	IsRunning() bool
	
	// I/O operations
	Write(data []byte) (int, error)
	Read(data []byte) (int, error)
	GetReader() io.Reader
	GetWriter() io.Writer
	
	// Terminal control
	Resize(cols, rows int) error
	Clear()
	Reset()
	GetSize() (cols, rows int)
	
	// Scrollback
	ScrollUp(lines int)
	ScrollDown(lines int)
	ScrollToTop()
	ScrollToBottom()
	GetScrollbackBuffer() []string
	
	// Mode management
	SetApplicationMode(enabled bool)
	SetPassthroughMode(enabled bool)
	IsApplicationMode() bool
	IsPassthroughMode() bool
	
	// Events
	OnOutput(handler func(data []byte))
	OnExit(handler func(exitCode int))
	OnBell(handler func())
	
	// UI integration
	HandleKey(event *tcell.EventKey) bool
	HandleMouse(event *tcell.EventMouse) bool
	Render() // For custom rendering if needed
	
	// Working directory
	GetWorkingDirectory() string
	SetWorkingDirectory(dir string) error
}