package components

import (
	"os"
	"time"

	"github.com/laborin/emed/internal/logging"
)

// FileChangeHandler handles external file modifications detection
type FileChangeHandler struct {
	logger *logging.Logger
}

// NewFileChangeHandler creates a new file change handler
func NewFileChangeHandler() *FileChangeHandler {
	return &FileChangeHandler{
		logger: logging.GetLogger(),
	}
}

// FileInfo contains file information for change detection
type FileInfo struct {
	Path               string
	Modified           bool
	ExternallyModified bool
	LastModTime        time.Time
}

// CheckFileModified checks if a file was modified externally
func (h *FileChangeHandler) CheckFileModified(info *FileInfo) (bool, error) {
	// Check if file still exists
	stat, err := os.Stat(info.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, err // File deleted
		}
		return false, err
	}
	
	// Check if modification time changed
	if !info.LastModTime.Equal(stat.ModTime()) {
		// File was modified externally
		return true, nil
	}
	
	return false, nil
}

// HandleFileModified processes an external file modification
func (h *FileChangeHandler) HandleFileModified(info *FileInfo) FileChangeAction {
	// Check if we just saved this file ourselves
	if stat, err := os.Stat(info.Path); err == nil {
		// If the modification time matches what we recorded during save,
		// this is our own save, not an external modification
		if info.LastModTime.Equal(stat.ModTime()) {
			return FileChangeActionNone
		}
	}
	
	// Check if the file has unsaved changes
	if info.Modified {
		// Mark as externally modified, will warn on save
		info.ExternallyModified = true
		return FileChangeActionMarkConflict
	} else {
		// No unsaved changes, reload the file
		return FileChangeActionReload
	}
}

// HandleFileDeleted processes an external file deletion
func (h *FileChangeHandler) HandleFileDeleted(info *FileInfo) FileChangeAction {
	// If the file has unsaved changes, keep the tab but mark it
	if info.Modified {
		// Mark as deleted externally
		info.ExternallyModified = true
		return FileChangeActionMarkDeleted
	} else {
		// No unsaved changes, close the tab
		return FileChangeActionClose
	}
}

// UpdateFileModTime updates the last modification time after save
func (h *FileChangeHandler) UpdateFileModTime(info *FileInfo) error {
	if stat, err := os.Stat(info.Path); err == nil {
		info.LastModTime = stat.ModTime()
		info.ExternallyModified = false
		return nil
	} else {
		return err
	}
}

// FileChangeAction represents the action to take for a file change
type FileChangeAction int

const (
	FileChangeActionNone FileChangeAction = iota
	FileChangeActionReload
	FileChangeActionClose
	FileChangeActionMarkConflict
	FileChangeActionMarkDeleted
)