package interfaces

import (
	"github.com/rivo/tview"
)

// LayoutManagerInterface defines the contract for layout management
type LayoutManagerInterface interface {
	// Component management
	SetTerminal(terminal TerminalInterface)
	SetFileManager(fileManager FileManagerInterface)
	GetTerminal() TerminalInterface
	GetFileManager() FileManagerInterface
	
	// Focus management
	FocusEditor()
	FocusTerminal()
	FocusFileManager()
	GetFocusedComponent() ComponentType
	CycleFocus()
	
	// Layout control
	SetProportions(editorWidth, terminalHeight int)
	GetProportions() (editorWidth, terminalHeight int)
	CycleProportions()
	SetTerminalPosition(position TerminalPosition)
	GetTerminalPosition() TerminalPosition
	ToggleTerminalPosition()
	
	// Visibility
	ShowTerminal()
	HideTerminal()
	ToggleTerminal()
	IsTerminalVisible() bool
	
	// Status bar
	SetStatusBar(statusBar StatusBarInterface)
	GetStatusBar() StatusBarInterface
	
	// Application
	GetApplication() *tview.Application
	Run() error
	Stop()
	
	// Events
	OnComponentFocused(handler func(component ComponentType))
	OnLayoutChanged(handler func())
}

// ComponentType identifies UI components
type ComponentType int

const (
	ComponentEditor ComponentType = iota
	ComponentTerminal
	ComponentFileManager
	ComponentStatusBar
)

// TerminalPosition defines where the terminal is positioned
type TerminalPosition int

const (
	TerminalBottom TerminalPosition = iota
	TerminalRight
)