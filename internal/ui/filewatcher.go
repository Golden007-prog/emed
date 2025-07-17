package ui

import (
	"os"
	"path/filepath"
	"sync"
	"time"
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

// FileWatcher watches for file system changes
type FileWatcher struct {
	rootPath      string
	files         map[string]os.FileInfo // path -> info for tracking
	mu            sync.RWMutex
	onChange      func([]FileChange)
	stopChan      chan bool
	isWatching    bool
	checkInterval time.Duration
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(rootPath string) *FileWatcher {
	return &FileWatcher{
		rootPath:      rootPath,
		files:         make(map[string]os.FileInfo),
		stopChan:      make(chan bool),
		checkInterval: 500 * time.Millisecond, // Check every 500ms
	}
}

// SetOnChange sets the callback for file changes
func (fw *FileWatcher) SetOnChange(handler func([]FileChange)) {
	fw.onChange = handler
}

// Start starts watching for changes
func (fw *FileWatcher) Start() {
	if fw.isWatching {
		return
	}
	
	fw.isWatching = true
	
	// Initial scan
	fw.scanDirectory()
	
	// Start watching loop
	go fw.watchLoop()
}

// Stop stops watching for changes
func (fw *FileWatcher) Stop() {
	if !fw.isWatching {
		return
	}
	
	fw.isWatching = false
	fw.stopChan <- true
}

// watchLoop periodically scans for changes
func (fw *FileWatcher) watchLoop() {
	ticker := time.NewTicker(fw.checkInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-fw.stopChan:
			return
		case <-ticker.C:
			fw.checkForChanges()
		}
	}
}

// scanDirectory scans the directory tree and updates the file map
func (fw *FileWatcher) scanDirectory() {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	
	newFiles := make(map[string]os.FileInfo)
	
	// Walk the directory tree
	filepath.Walk(fw.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		// Handle .git directory specially - include HEAD file
		if info.IsDir() && info.Name() == ".git" {
			// Check the HEAD file specifically
			headPath := filepath.Join(path, "HEAD")
			if headInfo, err := os.Stat(headPath); err == nil {
				newFiles[headPath] = headInfo
			}
			return filepath.SkipDir
		}
		
		// Skip other hidden directories (but include hidden files)
		if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' && path != fw.rootPath {
			return filepath.SkipDir
		}
		
		newFiles[path] = info
		return nil
	})
	
	fw.files = newFiles
}

// checkForChanges compares current state with cached state
func (fw *FileWatcher) checkForChanges() {
	fw.mu.Lock()
	oldFiles := fw.files
	fw.mu.Unlock()
	
	// Scan current state
	newFiles := make(map[string]os.FileInfo)
	changes := []FileChange{}
	
	// Walk the directory tree
	filepath.Walk(fw.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}
		
		// Handle .git directory specially - include HEAD file
		if info.IsDir() && info.Name() == ".git" {
			// Check the HEAD file specifically
			headPath := filepath.Join(path, "HEAD")
			if headInfo, err := os.Stat(headPath); err == nil {
				newFiles[headPath] = headInfo
				
				// Check if HEAD file was modified
				if oldInfo, exists := oldFiles[headPath]; exists {
					// File existed, check if modified
					if headInfo.ModTime().After(oldInfo.ModTime()) {
						changes = append(changes, FileChange{
							Path: headPath,
							Type: FileModified,
						})
					}
				} else {
					// New file (first time seeing it)
					changes = append(changes, FileChange{
						Path: headPath,
						Type: FileCreated,
					})
				}
			}
			return filepath.SkipDir
		}
		
		// Skip other hidden directories (but include hidden files)
		if info.IsDir() && len(info.Name()) > 0 && info.Name()[0] == '.' && path != fw.rootPath {
			return filepath.SkipDir
		}
		
		newFiles[path] = info
		
		// Check if file existed before
		if oldInfo, exists := oldFiles[path]; exists {
			// File existed, check if modified
			if !info.IsDir() && info.ModTime().After(oldInfo.ModTime()) {
				changes = append(changes, FileChange{
					Path: path,
					Type: FileModified,
				})
			}
		} else {
			// New file
			changes = append(changes, FileChange{
				Path: path,
				Type: FileCreated,
			})
		}
		
		return nil
	})
	
	// Check for deleted files
	for oldPath := range oldFiles {
		if _, exists := newFiles[oldPath]; !exists {
			changes = append(changes, FileChange{
				Path: oldPath,
				Type: FileDeleted,
			})
		}
	}
	
	// Update cached state
	fw.mu.Lock()
	fw.files = newFiles
	fw.mu.Unlock()
	
	// Notify if there are changes
	if len(changes) > 0 && fw.onChange != nil {
		fw.onChange(changes)
	}
}

// IsWatchingFile checks if a specific file is being watched
func (fw *FileWatcher) IsWatchingFile(path string) bool {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	
	_, exists := fw.files[path]
	return exists
}

// GetFileInfo gets the cached file info for a path
func (fw *FileWatcher) GetFileInfo(path string) (os.FileInfo, bool) {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	
	info, exists := fw.files[path]
	return info, exists
}