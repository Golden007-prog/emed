package interfaces

import "github.com/gdamore/tcell/v2"

// KeybindingManager manages keyboard shortcuts and their handlers
type KeybindingManager interface {
	// Registration
	Register(key KeyBinding) error
	RegisterMultiple(bindings []KeyBinding) error
	Unregister(key string) error
	UnregisterAll()
	
	// Context management
	PushContext(context KeyContext)
	PopContext() KeyContext
	GetCurrentContext() KeyContext
	SetGlobalContext(context KeyContext)
	
	// Handling
	HandleKey(event *tcell.EventKey) bool
	
	// Queries
	GetBinding(key string) (KeyBinding, bool)
	GetBindingsForContext(context KeyContext) []KeyBinding
	GetAllBindings() []KeyBinding
	IsRegistered(key string) bool
	
	// Import/Export
	LoadFromFile(path string) error
	SaveToFile(path string) error
	LoadDefaults()
}

// KeyBinding represents a keyboard shortcut binding
type KeyBinding struct {
	Key         string      // Key combination (e.g., "Ctrl+S")
	Command     string      // Command identifier
	Handler     KeyHandler  // Function to execute
	Description string      // Human-readable description
	Context     KeyContext  // Context where this binding is active
	When        string      // Additional condition expression
}

// KeyHandler handles a key event
type KeyHandler func() error

// KeyContext defines the context where keybindings are active
type KeyContext string

const (
	KeyContextGlobal      KeyContext = "global"
	KeyContextEditor      KeyContext = "editor"
	KeyContextTerminal    KeyContext = "terminal"
	KeyContextFileManager KeyContext = "filemanager"
	KeyContextModal       KeyContext = "modal"
)