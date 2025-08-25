package keybindings

import (
	"github.com/gdamore/tcell/v2"
)

// KeyHandler represents a keyboard shortcut handler
type KeyHandler struct {
	Key         tcell.Key
	Rune        rune
	Modifiers   tcell.ModMask
	Description string
	Handler     func() error
}

// KeyBindingManager manages keyboard shortcuts
type KeyBindingManager struct {
	handlers       []KeyHandler
	globalHandlers []KeyHandler
}

// NewKeyBindingManager creates a new keybinding manager
func NewKeyBindingManager() *KeyBindingManager {
	return &KeyBindingManager{
		handlers:       make([]KeyHandler, 0),
		globalHandlers: make([]KeyHandler, 0),
	}
}

// RegisterHandler registers a new key handler
func (m *KeyBindingManager) RegisterHandler(handler KeyHandler) {
	m.handlers = append(m.handlers, handler)
}

// RegisterGlobalHandler registers a global key handler
func (m *KeyBindingManager) RegisterGlobalHandler(handler KeyHandler) {
	m.globalHandlers = append(m.globalHandlers, handler)
}

// HandleKeyEvent processes a key event and returns true if handled
func (m *KeyBindingManager) HandleKeyEvent(event *tcell.EventKey) (bool, error) {
	// Check global handlers first
	for _, handler := range m.globalHandlers {
		if m.matchesKey(event, handler) {
			if err := handler.Handler(); err != nil {
				return true, err
			}
			return true, nil
		}
	}
	
	// Check regular handlers
	for _, handler := range m.handlers {
		if m.matchesKey(event, handler) {
			if err := handler.Handler(); err != nil {
				return true, err
			}
			return true, nil
		}
	}
	
	return false, nil
}

// matchesKey checks if an event matches a handler
func (m *KeyBindingManager) matchesKey(event *tcell.EventKey, handler KeyHandler) bool {
	if handler.Key != tcell.KeyRune {
		return event.Key() == handler.Key && event.Modifiers() == handler.Modifiers
	}
	return event.Key() == tcell.KeyRune && event.Rune() == handler.Rune && event.Modifiers() == handler.Modifiers
}

// GetHandlers returns all registered handlers
func (m *KeyBindingManager) GetHandlers() []KeyHandler {
	return m.handlers
}

// GetGlobalHandlers returns all global handlers
func (m *KeyBindingManager) GetGlobalHandlers() []KeyHandler {
	return m.globalHandlers
}

// ClearHandlers clears all handlers
func (m *KeyBindingManager) ClearHandlers() {
	m.handlers = make([]KeyHandler, 0)
}

// ClearGlobalHandlers clears all global handlers
func (m *KeyBindingManager) ClearGlobalHandlers() {
	m.globalHandlers = make([]KeyHandler, 0)
}