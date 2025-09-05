// Package filemanager provides file management functionality for the editor.
package filemanager

import (
	"strings"
	"sync"
	"time"
)

// TypeAheadBuffer manages keyboard input for quick file navigation.
type TypeAheadBuffer struct {
	buffer      string
	lastKeyTime time.Time
	timeout     time.Duration
	mu          sync.Mutex
	timer       *time.Timer
}

// NewTypeAheadBuffer creates a new type-ahead buffer with specified timeout.
func NewTypeAheadBuffer(timeout time.Duration) *TypeAheadBuffer {
	return &TypeAheadBuffer{
		timeout: timeout,
	}
}

// AddChar adds a character to the buffer and resets the timeout.
func (t *TypeAheadBuffer) AddChar(char rune) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.timer != nil {
		t.timer.Stop()
	}
	
	t.buffer += string(char)
	t.lastKeyTime = time.Now()
	
	t.timer = time.AfterFunc(t.timeout, func() {
		t.Clear()
	})
	
	return t.buffer
}

// GetBuffer returns the current buffer content.
func (t *TypeAheadBuffer) GetBuffer() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.buffer
}

// Clear empties the buffer and stops any pending timer.
func (t *TypeAheadBuffer) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.buffer = ""
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}

// FileMatch determines if a filename matches the current buffer.
// Returns true if there's a match, with prefix matches taking priority.
func (t *TypeAheadBuffer) FileMatch(filename string, searchTerm string) bool {
	if searchTerm == "" {
		return false
	}
	
	filenameLower := strings.ToLower(filename)
	searchLower := strings.ToLower(searchTerm)
	
	return strings.HasPrefix(filenameLower, searchLower)
}

// FileContains checks if a filename contains the search term anywhere.
func (t *TypeAheadBuffer) FileContains(filename string, searchTerm string) bool {
	if searchTerm == "" {
		return false
	}
	
	filenameLower := strings.ToLower(filename)
	searchLower := strings.ToLower(searchTerm)
	
	return strings.Contains(filenameLower, searchLower)
}