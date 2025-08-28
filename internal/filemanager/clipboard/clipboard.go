package clipboard

import (
	"sync"
)

type ClipboardItem struct {
	Path  string
	IsCut bool
}

type Clipboard struct {
	mu   sync.RWMutex
	item *ClipboardItem
}

func New() *Clipboard {
	return &Clipboard{}
}

func (c *Clipboard) Copy(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.item = &ClipboardItem{
		Path:  path,
		IsCut: false,
	}
}

func (c *Clipboard) Cut(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.item = &ClipboardItem{
		Path:  path,
		IsCut: true,
	}
}

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

func (c *Clipboard) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.item = nil
}

func (c *Clipboard) HasContent() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.item != nil
}

func (c *Clipboard) IsCutOperation() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.item != nil && c.item.IsCut
}