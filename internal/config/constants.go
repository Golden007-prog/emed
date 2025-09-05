// Package config defines application constants and configuration values.
package config

import "time"

// UI Layout Constants
const (
	// Layout proportions
	LayoutFileManagerProportion = 20
	LayoutEditorProportion      = 50
	LayoutTerminalProportion    = 30
	
	// Tab display
	MaxVisibleTabs = 4
	TabBarHeight   = 1
	
	// Modal dimensions
	DefaultModalWidth  = 50
	DefaultModalHeight = 10
	InfoModalWidth     = 60
	InfoModalHeight    = 20
)

// File Watcher Constants  
const (
	// Watcher limits
	MaxWatchDepth        = 3
	MaxWatchedDirs       = 500
	WatcherDebounceDelay = 100 * time.Millisecond
	
	// Event processing
	EventBatchSize    = 100
	EventProcessDelay = 50 * time.Millisecond
)

// Logger Constants
const (
	// Log retention
	LogRetentionDays = 7
	
	// Log file settings
	LogFilePermissions = 0644
	LogDirPermissions  = 0755
)

// Terminal Constants
const (
	// Terminal escape sequences
	TerminalExitAltScreen = "\033[?1049l"
	TerminalShowCursor    = "\033[?25h"
	TerminalResetColors   = "\033[0m"
	
	// Terminal buffer
	DefaultScrollbackSize = 10000
)

// Editor Constants
const (
	// Tab settings
	DefaultTabSize   = 4
	DefaultUseTabs   = false
	DefaultAutoSave  = false
	
	// Search
	SearchHighlightColor = "[yellow::b]"
	
	// Performance
	RenderBatchSize = 1000
)

// CodeFileExtensions contains file extensions recognized as code files.
var CodeFileExtensions = []string{
	".go", ".py", ".js", ".ts", ".jsx", ".tsx",
	".java", ".c", ".cpp", ".h", ".hpp", ".cs",
	".rb", ".php", ".swift", ".kt", ".rs", ".scala",
	".sh", ".bash", ".zsh", ".fish", ".ps1",
	".html", ".css", ".scss", ".sass", ".less",
	".json", ".xml", ".yaml", ".yml", ".toml",
	".sql", ".graphql", ".proto",
	".md", ".txt", ".log",
}

// Git Commands
const (
	GitStatusCommand = "git status --porcelain 2>/dev/null"
	GitBranchCommand = "git branch --show-current 2>/dev/null"
)

// Clipboard Commands (Darwin/macOS)
const (
	ClipboardCopyCommand  = "pbcopy"
	ClipboardPasteCommand = "pbpaste"
)