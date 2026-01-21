// Package clipboard provides file operation clipboard functionality.
package clipboard

import (
	"sync"
)

// ClipboardItem represents a file or directory in the clipboard.
type ClipboardItem struct {
	Path  string
	IsCut bool
}

// Clipboard manages cut and copy operations for file management.
type Clipboard struct {
	mu   sync.RWMutex
	item *ClipboardItem
}

// New creates a new empty clipboard.
func New() *Clipboard {
	return &Clipboard{}
}

// Copy adds a file path to the clipboard for copying.
func (c *Clipboard) Copy(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.item = &ClipboardItem{
		Path:  path,
		IsCut: false,
	}
}

// Cut adds a file path to the clipboard for cutting.
func (c *Clipboard) Cut(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.item = &ClipboardItem{
		Path:  path,
		IsCut: true,
	}
}

// Get returns a copy of the current clipboard item.
func (c *Clipboard) Get() *ClipboardItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.item == nil {
		return nil
	}

	return &ClipboardItem{
		Path:  c.item.Path,
		IsCut: c.item.IsCut,
	}
}

// Clear removes all content from the clipboard.
func (c *Clipboard) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.item = nil
}

// HasContent returns true if the clipboard contains an item.
func (c *Clipboard) HasContent() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.item != nil
}

// IsCutOperation returns true if the clipboard item is marked for cutting.
func (c *Clipboard) IsCutOperation() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.item != nil && c.item.IsCut
}
