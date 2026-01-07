// Package ui provides user interface components and managers for the emed editor.
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

// FileChangeType represents the type of file system change.
type FileChangeType int

// File change types.
const (
	FileCreated FileChangeType = iota
	FileModified
	FileDeleted
	FileRenamed
)

// FileChange represents a file system change event.
type FileChange struct {
	Path    string
	OldPath string
	Type    FileChangeType
}

// FileWatcher monitors file system changes in a directory tree.
type FileWatcher struct {
	rootPath   string
	watcher    *fsnotify.Watcher
	mu         sync.RWMutex
	onChange   func([]FileChange)
	stopChan   chan bool
	isWatching bool

	watchedDirs map[string]bool
	dirMu       sync.RWMutex

	maxDepth        int
	maxWatchedDirs  int
	excludePatterns []string

	openFiles   map[string]bool
	openFilesMu sync.RWMutex
}

var defaultExclusions = []string{
	"node_modules",
	".git/objects",
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
	"target",
	"bin",
	"obj",
	"vendor",
	".idea",
	".vscode",
	"*.swp",
	"*.swo",
	"*~",
	".cache",
	".temp",
	".tmp",
}

// NewFileWatcher creates a new file system watcher for the specified root path.
func NewFileWatcher(rootPath string) *FileWatcher {
	return &FileWatcher{
		rootPath:        rootPath,
		stopChan:        make(chan bool),
		watchedDirs:     make(map[string]bool),
		openFiles:       make(map[string]bool),
		maxDepth:        config.MaxWatchDepth,
		maxWatchedDirs:  config.MaxWatchedDirs,
		excludePatterns: defaultExclusions,
	}
}

// SetOnChange sets the callback function for file system changes.
func (fw *FileWatcher) SetOnChange(handler func([]FileChange)) {
	fw.onChange = handler
}

// AddOpenFile adds a file to the watch list and ensures its directory is watched.
func (fw *FileWatcher) AddOpenFile(path string) {
	fw.openFilesMu.Lock()
	fw.openFiles[path] = true
	fw.openFilesMu.Unlock()

	if fw.isWatching {
		dir := filepath.Dir(path)
		fw.ensureDirectoryWatched(dir)
	}
}

// RemoveOpenFile removes a file from the watch list.
func (fw *FileWatcher) RemoveOpenFile(path string) {
	fw.openFilesMu.Lock()
	delete(fw.openFiles, path)
	fw.openFilesMu.Unlock()
}

// Start begins file system monitoring on the root path.
func (fw *FileWatcher) Start() error {
	if fw.isWatching {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	fw.watcher = watcher

	err = fw.watcher.Add(fw.rootPath)
	if err != nil {
		watcher.Close()
		return err
	}

	fw.dirMu.Lock()
	fw.watchedDirs[fw.rootPath] = true
	fw.dirMu.Unlock()

	fw.addShallowDirectories(fw.rootPath, 0)

	fw.openFilesMu.RLock()
	for filePath := range fw.openFiles {
		dir := filepath.Dir(filePath)
		fw.ensureDirectoryWatched(dir)
	}
	fw.openFilesMu.RUnlock()

	fw.isWatching = true

	go fw.processEvents()

	return nil
}

func (fw *FileWatcher) shouldExclude(path string) bool {
	base := filepath.Base(path)

	for _, pattern := range fw.excludePatterns {
		if base == pattern {
			return true
		}
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
		if strings.Contains(path, "/"+pattern+"/") {
			return true
		}
	}

	return false
}

func (fw *FileWatcher) addShallowDirectories(dir string, currentDepth int) {
	if currentDepth >= fw.maxDepth {
		return
	}

	fw.dirMu.RLock()
	watchedCount := len(fw.watchedDirs)
	fw.dirMu.RUnlock()

	if watchedCount >= fw.maxWatchedDirs {
		return
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

		if fw.shouldExclude(fullPath) {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") && name != ".git" {
			continue
		}

		fw.dirMu.Lock()
		if !fw.watchedDirs[fullPath] {
			fw.watchedDirs[fullPath] = true
			fw.dirMu.Unlock()

			err := fw.watcher.Add(fullPath)
			if err == nil {
				fw.addShallowDirectories(fullPath, currentDepth+1)
			}
		} else {
			fw.dirMu.Unlock()
		}
	}
}

func (fw *FileWatcher) ensureDirectoryWatched(dir string) {
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

		parent := filepath.Dir(dir)
		if parent != dir && parent != "." && strings.HasPrefix(parent, fw.rootPath) {
			fw.ensureDirectoryWatched(parent)
		}
	}
}

func (fw *FileWatcher) removeDirectory(dir string) {
	fw.dirMu.Lock()
	delete(fw.watchedDirs, dir)
	fw.dirMu.Unlock()

	fw.watcher.Remove(dir)
}

func (fw *FileWatcher) processEvents() {
	eventBatch := make([]FileChange, 0)
	batchMu := sync.Mutex{}

	var debounceTimer *time.Timer
	sendBatch := func() {
		batchMu.Lock()
		if len(eventBatch) > 0 && fw.onChange != nil {
			changes := make([]FileChange, len(eventBatch))
			copy(changes, eventBatch)
			for i := range eventBatch {
				eventBatch[i] = FileChange{}
			}
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

			change := fw.convertEvent(event)
			if change != nil {
				batchMu.Lock()
				eventBatch = append(eventBatch, *change)
				batchMu.Unlock()

				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(config.WatcherDebounceDelay, sendBatch)
			}

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			_ = err
		}
	}
}

func (fw *FileWatcher) convertEvent(event fsnotify.Event) *FileChange {
	if fw.shouldExclude(event.Name) {
		return nil
	}

	if strings.Contains(event.Name, "/.git/") && !strings.HasSuffix(event.Name, ".git/HEAD") {
		return nil
	}

	base := filepath.Base(event.Name)
	if strings.HasPrefix(base, ".") && base != ".gitignore" && base != ".gitmodules" {
		return nil
	}

	var changeType FileChangeType

	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		changeType = FileCreated

		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
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

// Stop terminates file system monitoring and cleans up resources.
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

// IsWatchingFile returns true if the directory containing the file is being watched.
func (fw *FileWatcher) IsWatchingFile(path string) bool {
	dir := filepath.Dir(path)

	fw.dirMu.RLock()
	defer fw.dirMu.RUnlock()

	return fw.watchedDirs[dir]
}

// GetFileInfo returns file information and whether the file exists.
func (fw *FileWatcher) GetFileInfo(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

// SetMaxDepth sets the maximum directory depth to watch.
func (fw *FileWatcher) SetMaxDepth(depth int) {
	fw.maxDepth = depth
}

// SetMaxWatchedDirs sets the maximum number of directories to watch.
func (fw *FileWatcher) SetMaxWatchedDirs(max int) {
	fw.maxWatchedDirs = max
}

// AddExclusionPattern adds a pattern to exclude from watching.
func (fw *FileWatcher) AddExclusionPattern(pattern string) {
	fw.excludePatterns = append(fw.excludePatterns, pattern)
}

// GetWatchedDirsCount returns the number of directories currently being watched.
func (fw *FileWatcher) GetWatchedDirsCount() int {
	fw.dirMu.RLock()
	defer fw.dirMu.RUnlock()
	return len(fw.watchedDirs)
}
