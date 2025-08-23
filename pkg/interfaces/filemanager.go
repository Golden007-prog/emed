package interfaces

import "github.com/gdamore/tcell/v2"

// FileManagerInterface defines the contract for file manager implementations
type FileManagerInterface interface {
	// Navigation
	SetRootPath(path string) error
	GetCurrentPath() string
	ChangeDirectory(path string) error
	Refresh() error
	
	// Selection
	GetSelectedPath() string
	GetSelectedPaths() []string
	SelectItem(path string)
	SelectAll()
	ClearSelection()
	
	// File operations
	CreateFile(path string) error
	CreateDirectory(path string) error
	Delete(paths []string) error
	Rename(oldPath, newPath string) error
	Copy(paths []string) error
	Cut(paths []string) error
	Paste(targetPath string) error
	
	// Clipboard
	HasClipboard() bool
	ClearClipboard()
	
	// Search
	Search(pattern string) []string
	Filter(pattern string)
	ClearFilter()
	
	// Events
	OnFileSelected(handler func(path string))
	OnDirectoryChanged(handler func(path string))
	OnFileOperation(handler func(op FileOperation))
	
	// UI integration
	HandleKey(event *tcell.EventKey) bool
	HandleMouse(event *tcell.EventMouse) bool
	Render() // For custom rendering if needed
	
	// Tree operations
	ExpandNode(path string)
	CollapseNode(path string)
	ToggleNode(path string)
	IsExpanded(path string) bool
	
	// File watching
	EnableWatching(enabled bool)
	IsWatchingEnabled() bool
}

// FileOperation represents a file system operation
type FileOperation struct {
	Type      FileOperationType
	Source    []string
	Target    string
	Timestamp int64
}

// FileOperationType defines types of file operations
type FileOperationType int

const (
	FileOpCreate FileOperationType = iota
	FileOpDelete
	FileOpRename
	FileOpCopy
	FileOpMove
)