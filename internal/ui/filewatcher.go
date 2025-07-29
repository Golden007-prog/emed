package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
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
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(rootPath string) *FileWatcher {
	return &FileWatcher{
		rootPath:    rootPath,
		stopChan:    make(chan bool),
		watchedDirs: make(map[string]bool),
	}
}

// SetOnChange sets the callback for file changes
func (fw *FileWatcher) SetOnChange(handler func([]FileChange)) {
	fw.onChange = handler
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
	
	// Add root directory and all subdirectories
	err = fw.addDirectoryRecursive(fw.rootPath)
	if err != nil {
		watcher.Close()
		return err
	}
	
	fw.isWatching = true
	
	// Start event processing loop
	go fw.processEvents()
	
	return nil
}

// addDirectoryRecursive adds a directory and all its subdirectories to the watcher
func (fw *FileWatcher) addDirectoryRecursive(dir string) error {
	// Walk the directory tree
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}
		
		// Skip hidden directories except .git (needed for branch detection)
		if info.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && base != ".git" && path != fw.rootPath {
				return filepath.SkipDir
			}
			
			// Add directory to watcher
			fw.dirMu.Lock()
			fw.watchedDirs[path] = true
			fw.dirMu.Unlock()
			
			return fw.watcher.Add(path)
		}
		
		return nil
	})
}

// removeDirectory removes a directory from the watcher
func (fw *FileWatcher) removeDirectory(dir string) {
	fw.dirMu.Lock()
	delete(fw.watchedDirs, dir)
	fw.dirMu.Unlock()
	
	fw.watcher.Remove(dir)
}

// processEvents processes fsnotify events
func (fw *FileWatcher) processEvents() {
	for {
		select {
		case <-fw.stopChan:
			return
			
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			
			// Convert fsnotify event to our FileChange type
			change := fw.convertEvent(event)
			if change != nil && fw.onChange != nil {
				// Send changes immediately (the UI already batches them)
				fw.onChange([]FileChange{*change})
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
	// Allow .git/HEAD for branch detection, but skip other hidden files
	base := filepath.Base(event.Name)
	if strings.HasPrefix(base, ".") && !strings.HasSuffix(event.Name, ".git/HEAD") {
		return nil
	}
	
	var changeType FileChangeType
	
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		changeType = FileCreated
		
		// If it's a directory, add it to the watcher
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			fw.addDirectoryRecursive(event.Name)
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