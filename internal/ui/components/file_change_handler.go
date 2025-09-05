// Package components provides reusable UI components for the emed editor.
package components

import (
	"os"
	"time"

	"github.com/laborin/emed/internal/logging"
)

// FileChangeHandler handles file system changes for open files.
type FileChangeHandler struct {
	logger *logging.Logger
}

// NewFileChangeHandler creates a new file change handler.
func NewFileChangeHandler() *FileChangeHandler {
	return &FileChangeHandler{
		logger: logging.GetLogger(),
	}
}

// FileInfo contains information about a file's state.
type FileInfo struct {
	Path               string
	Modified           bool
	ExternallyModified bool
	LastModTime        time.Time
}

// CheckFileModified checks if a file has been modified externally.
func (h *FileChangeHandler) CheckFileModified(info *FileInfo) (bool, error) {
	stat, err := os.Stat(info.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, err
		}
		return false, err
	}

	if !info.LastModTime.Equal(stat.ModTime()) {
		return true, nil
	}

	return false, nil
}

// HandleFileModified handles when a file is modified externally.
func (h *FileChangeHandler) HandleFileModified(info *FileInfo) FileChangeAction {
	if stat, err := os.Stat(info.Path); err == nil {
		if info.LastModTime.Equal(stat.ModTime()) {
			return FileChangeActionNone
		}
	}

	if info.Modified {
		info.ExternallyModified = true
		return FileChangeActionMarkConflict
	} else {
		return FileChangeActionReload
	}
}

// HandleFileDeleted handles when a file is deleted externally.
func (h *FileChangeHandler) HandleFileDeleted(info *FileInfo) FileChangeAction {
	if info.Modified {
		info.ExternallyModified = true
		return FileChangeActionMarkDeleted
	} else {
		return FileChangeActionClose
	}
}

// UpdateFileModTime updates the file's modification time.
func (h *FileChangeHandler) UpdateFileModTime(info *FileInfo) error {
	if stat, err := os.Stat(info.Path); err == nil {
		info.LastModTime = stat.ModTime()
		info.ExternallyModified = false
		return nil
	} else {
		return err
	}
}

// FileChangeAction represents actions to take when files change.
type FileChangeAction int

// File change actions.
const (
	FileChangeActionNone FileChangeAction = iota
	FileChangeActionReload
	FileChangeActionClose
	FileChangeActionMarkConflict
	FileChangeActionMarkDeleted
)
