package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/laborin/emed/internal/config"
)

// FileChangeType represents the type of file system change
type FileChangeType int

const (
	FileCreated FileChangeType = iota
	FileModified
	FileDeleted
	FileRenamed
)

// FileChange represents a file system change event
type FileChange struct {
	Path    string
	OldPath string // For rename events
	Type    FileChangeType
}

// FileWatcher watches for file system changes using fsnotify
// It implements an efficient watching strategy similar to VS Code:
// - Only watches directories up to a certain depth
// - Excludes common build/dependency directories
// - Uses selective watching for better performance
type FileWatcher struct {
	rootPath   string
	watcher    *fsnotify.Watcher
	mu         sync.RWMutex
	onChange   func([]FileChange)
	stopChan   chan bool
	isWatching bool
	
	// Track directories we're watching for recursive monitoring
	watchedDirs map[string]bool
	dirMu       sync.RWMutex
	
	// Performance settings
	maxDepth       int      // Maximum depth to watch (relative to root)
	maxWatchedDirs int      // Maximum number of directories to watch
	excludePatterns []string // Patterns to exclude from watching
	
	// Track open files for selective watching
	openFiles   map[string]bool
	openFilesMu sync.RWMutex
}

// Default exclusion patterns (similar to VS Code's defaults)
var defaultExclusions = []string{
	"node_modules",
	".git/objects", // Watch .git but not its large subdirs
	".git/logs",
	".git/refs",
	".git/hooks",
	".git/info",
	".hg",
	".svn",
	"CVS",
	".DS_Store",
	"Thumbs.db",
	"*.pyc",
	"__pycache__",
	".pytest_cache",
	".mypy_cache",
	".tox",
	".eggs",
	"*.egg-info",
	"dist",
	"build",
	"target",       // Maven/Rust
	"bin",
	"obj",         // .NET
	"vendor",      // Go vendor
	".idea",       // IntelliJ
	".vscode",     // VS Code workspace settings
	"*.swp",
	"*.swo",
	"*~",
	".cache",
	".temp",
	".tmp",
}

// NewFileWatcher creates a new file watcher with sensible defaults
func NewFileWatcher(rootPath string) *FileWatcher {
	return &FileWatcher{
		rootPath:        rootPath,
		stopChan:        make(chan bool),
		watchedDirs:     make(map[string]bool),
		openFiles:       make(map[string]bool),
		maxDepth:        config.MaxWatchDepth,  // Only watch limited levels deep
		maxWatchedDirs:  config.MaxWatchedDirs, // Limit total watched directories
		excludePatterns: defaultExclusions,
	}
}

// SetOnChange sets the callback for file changes
func (fw *FileWatcher) SetOnChange(handler func([]FileChange)) {
	fw.onChange = handler
}

// AddOpenFile adds a file to the selective watch list
// When a file is opened in the editor, we ensure its directory is watched
func (fw *FileWatcher) AddOpenFile(path string) {
	fw.openFilesMu.Lock()
	fw.openFiles[path] = true
	fw.openFilesMu.Unlock()
	
	// Ensure the file's directory is watched
	if fw.isWatching {
		dir := filepath.Dir(path)
		fw.ensureDirectoryWatched(dir)
	}
}

// RemoveOpenFile removes a file from the selective watch list
func (fw *FileWatcher) RemoveOpenFile(path string) {
	fw.openFilesMu.Lock()
	delete(fw.openFiles, path)
	fw.openFilesMu.Unlock()
}

// Start starts watching for changes
func (fw *FileWatcher) Start() error {
	if fw.isWatching {
		return nil
	}
	
	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	fw.watcher = watcher
	
	// Add root directory (non-recursive initially)
	err = fw.watcher.Add(fw.rootPath)
	if err != nil {
		watcher.Close()
		return err
	}
	
	fw.dirMu.Lock()
	fw.watchedDirs[fw.rootPath] = true
	fw.dirMu.Unlock()
	
	// Add first level of subdirectories (shallow watching)
	fw.addShallowDirectories(fw.rootPath, 0)
	
	// Watch directories of any open files
	fw.openFilesMu.RLock()
	for filePath := range fw.openFiles {
		dir := filepath.Dir(filePath)
		fw.ensureDirectoryWatched(dir)
	}
	fw.openFilesMu.RUnlock()
	
	fw.isWatching = true
	
	// Start event processing loop
	go fw.processEvents()
	
	return nil
}

// shouldExclude checks if a path should be excluded from watching
func (fw *FileWatcher) shouldExclude(path string) bool {
	base := filepath.Base(path)
	
	// Check exact matches and patterns
	for _, pattern := range fw.excludePatterns {
		// Exact match
		if base == pattern {
			return true
		}
		// Pattern match (simple glob)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		// Check if path contains excluded directory
		if strings.Contains(path, "/"+pattern+"/") {
			return true
		}
	}
	
	return false
}

// addShallowDirectories adds directories up to maxDepth
func (fw *FileWatcher) addShallowDirectories(dir string, currentDepth int) {
	if currentDepth >= fw.maxDepth {
		return
	}
	
	fw.dirMu.RLock()
	watchedCount := len(fw.watchedDirs)
	fw.dirMu.RUnlock()
	
	if watchedCount >= fw.maxWatchedDirs {
		return // Stop if we've hit the limit
	}
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		fullPath := filepath.Join(dir, entry.Name())
		
		// Skip excluded directories
		if fw.shouldExclude(fullPath) {
			continue
		}
		
		// Skip hidden directories except .git root
		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".git" {
			continue
		}
		
		// Add directory to watcher
		fw.dirMu.Lock()
		if !fw.watchedDirs[fullPath] {
			fw.watchedDirs[fullPath] = true
			fw.dirMu.Unlock()
			
			err := fw.watcher.Add(fullPath)
			if err == nil {
				// Recursively add subdirectories up to maxDepth
				fw.addShallowDirectories(fullPath, currentDepth+1)
			}
		} else {
			fw.dirMu.Unlock()
		}
	}
}

// ensureDirectoryWatched ensures a specific directory is watched
// This is used for directories containing open files
func (fw *FileWatcher) ensureDirectoryWatched(dir string) {
	// Don't watch if it's outside our root
	if !strings.HasPrefix(dir, fw.rootPath) {
		return
	}
	
	fw.dirMu.RLock()
	isWatched := fw.watchedDirs[dir]
	fw.dirMu.RUnlock()
	
	if !isWatched && !fw.shouldExclude(dir) {
		fw.dirMu.Lock()
		fw.watchedDirs[dir] = true
		fw.dirMu.Unlock()
		
		fw.watcher.Add(dir)
		
		// Also ensure parent directories are watched
		parent := filepath.Dir(dir)
		if parent != dir && parent != "." && strings.HasPrefix(parent, fw.rootPath) {
			fw.ensureDirectoryWatched(parent)
		}
	}
}

// removeDirectory removes a directory from the watcher
func (fw *FileWatcher) removeDirectory(dir string) {
	fw.dirMu.Lock()
	delete(fw.watchedDirs, dir)
	fw.dirMu.Unlock()
	
	fw.watcher.Remove(dir)
}

// processEvents processes fsnotify events with debouncing
func (fw *FileWatcher) processEvents() {
	// Batch events for better performance
	eventBatch := make([]FileChange, 0)
	batchMu := sync.Mutex{}
	
	// Debounce timer
	var debounceTimer *time.Timer
	sendBatch := func() {
		batchMu.Lock()
		if len(eventBatch) > 0 && fw.onChange != nil {
			changes := make([]FileChange, len(eventBatch))
			copy(changes, eventBatch)
			eventBatch = eventBatch[:0]
			batchMu.Unlock()
			fw.onChange(changes)
		} else {
			batchMu.Unlock()
		}
	}
	
	for {
		select {
		case <-fw.stopChan:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
			
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			
			// Convert fsnotify event to our FileChange type
			change := fw.convertEvent(event)
			if change != nil {
				batchMu.Lock()
				eventBatch = append(eventBatch, *change)
				batchMu.Unlock()
				
				// Reset debounce timer
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(config.WatcherDebounceDelay, sendBatch)
			}
			
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
			_ = err
		}
	}
}

// convertEvent converts fsnotify event to FileChange
func (fw *FileWatcher) convertEvent(event fsnotify.Event) *FileChange {
	// Skip excluded files
	if fw.shouldExclude(event.Name) {
		return nil
	}
	
	// Allow .git/HEAD for branch detection, but skip other .git internals
	if strings.Contains(event.Name, "/.git/") && !strings.HasSuffix(event.Name, ".git/HEAD") {
		return nil
	}
	
	// Skip other hidden files
	base := filepath.Base(event.Name)
	if strings.HasPrefix(base, ".") && base != ".gitignore" && base != ".gitmodules" {
		return nil
	}
	
	var changeType FileChangeType
	
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		changeType = FileCreated
		
		// If it's a directory, consider adding it to the watcher
		// but only if it's shallow enough or contains an open file
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			// Calculate depth
			relPath, _ := filepath.Rel(fw.rootPath, event.Name)
			depth := strings.Count(relPath, string(filepath.Separator))
			
			if depth < fw.maxDepth && !fw.shouldExclude(event.Name) {
				fw.dirMu.Lock()
				watchedCount := len(fw.watchedDirs)
				if watchedCount < fw.maxWatchedDirs && !fw.watchedDirs[event.Name] {
					fw.watchedDirs[event.Name] = true
					fw.dirMu.Unlock()
					fw.watcher.Add(event.Name)
				} else {
					fw.dirMu.Unlock()
				}
			}
		}
		
	case event.Op&fsnotify.Write == fsnotify.Write:
		changeType = FileModified
		
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		changeType = FileDeleted
		
		// If it's a directory, remove it from the watcher
		fw.dirMu.RLock()
		if fw.watchedDirs[event.Name] {
			fw.dirMu.RUnlock()
			fw.removeDirectory(event.Name)
		} else {
			fw.dirMu.RUnlock()
		}
		
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		changeType = FileRenamed
		
	default:
		return nil
	}
	
	return &FileChange{
		Path: event.Name,
		Type: changeType,
	}
}

// Stop stops watching for changes
func (fw *FileWatcher) Stop() {
	if !fw.isWatching {
		return
	}
	
	fw.isWatching = false
	close(fw.stopChan)
	
	if fw.watcher != nil {
		fw.watcher.Close()
	}
}

// IsWatchingFile checks if a specific file is being watched
func (fw *FileWatcher) IsWatchingFile(path string) bool {
	// Check if the file's directory is being watched
	dir := filepath.Dir(path)
	
	fw.dirMu.RLock()
	defer fw.dirMu.RUnlock()
	
	return fw.watchedDirs[dir]
}

// GetFileInfo gets the file info for a path (no longer cached)
func (fw *FileWatcher) GetFileInfo(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

// SetMaxDepth sets the maximum directory depth to watch
func (fw *FileWatcher) SetMaxDepth(depth int) {
	fw.maxDepth = depth
}

// SetMaxWatchedDirs sets the maximum number of directories to watch
func (fw *FileWatcher) SetMaxWatchedDirs(max int) {
	fw.maxWatchedDirs = max
}

// AddExclusionPattern adds a pattern to exclude from watching
func (fw *FileWatcher) AddExclusionPattern(pattern string) {
	fw.excludePatterns = append(fw.excludePatterns, pattern)
}

// GetWatchedDirsCount returns the current number of watched directories
func (fw *FileWatcher) GetWatchedDirsCount() int {
	fw.dirMu.RLock()
	defer fw.dirMu.RUnlock()
	return len(fw.watchedDirs)
}