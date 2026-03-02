// Package config defines application constants and configuration values.
package config

import "time"

// File Watcher Constants
const (
	// Watcher limits
	MaxWatchDepth        = 3
	MaxWatchedDirs       = 500
	WatcherDebounceDelay = 100 * time.Millisecond
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
)

// CodeFileExtensions contains file extensions recognized as code/text files.
var CodeFileExtensions = []string{
	".go", ".py", ".js", ".ts", ".jsx", ".tsx",
	".java", ".c", ".cpp", ".h", ".hpp", ".cs",
	".rb", ".php", ".swift", ".kt", ".rs", ".scala",
	".sh", ".bash", ".zsh", ".fish", ".ps1",
	".html", ".css", ".scss", ".sass", ".less",
	".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf", ".config",
	".sql", ".graphql", ".proto", ".lua", ".vim", ".r",
	".md", ".markdown", ".rst", ".txt", ".log",
}
