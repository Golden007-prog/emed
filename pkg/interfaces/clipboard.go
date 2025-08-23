package interfaces

// ClipboardInterface defines the contract for clipboard operations
type ClipboardInterface interface {
	// Text operations
	SetText(text string) error
	GetText() (string, error)
	HasText() bool
	Clear() error
	
	// File operations (for file manager)
	SetFiles(paths []string, cut bool) error
	GetFiles() (paths []string, cut bool, err error)
	HasFiles() bool
	ClearFiles() error
	
	// Format checking
	GetAvailableFormats() []string
	HasFormat(format string) bool
	
	// System clipboard integration
	UseSystemClipboard(enabled bool)
	IsUsingSystemClipboard() bool
	
	// Events
	OnChange(handler func())
}

// ClipboardFormat defines clipboard data formats
type ClipboardFormat string

const (
	ClipboardFormatText  ClipboardFormat = "text/plain"
	ClipboardFormatFiles ClipboardFormat = "text/uri-list"
	ClipboardFormatRTF   ClipboardFormat = "text/rtf"
	ClipboardFormatHTML  ClipboardFormat = "text/html"
)