package keybindings

import "github.com/gdamore/tcell/v2"

// NavigationHandlers defines keyboard handlers for navigation
type NavigationHandlers struct {
	FocusFileManager func() error
	FocusEditor      func() error
	FocusTerminal    func() error
	NextTab          func() error
	PreviousTab      func() error
	CloseTab         func() error
}

// RegisterNavigationHandlers registers all navigation-related key handlers
func RegisterNavigationHandlers(manager *KeyBindingManager, handlers NavigationHandlers) {
	// Focus navigation
	if handlers.FocusFileManager != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        '1',
			Modifiers:   tcell.ModCtrl,
			Description: "Focus file manager",
			Handler:     handlers.FocusFileManager,
		})
	}
	
	if handlers.FocusEditor != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        '2',
			Modifiers:   tcell.ModCtrl,
			Description: "Focus editor",
			Handler:     handlers.FocusEditor,
		})
	}
	
	if handlers.FocusTerminal != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        '3',
			Modifiers:   tcell.ModCtrl,
			Description: "Focus terminal",
			Handler:     handlers.FocusTerminal,
		})
	}
	
	// Tab navigation
	if handlers.NextTab != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyTab,
			Modifiers:   tcell.ModNone,
			Description: "Next tab",
			Handler:     handlers.NextTab,
		})
	}
	
	if handlers.PreviousTab != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyBacktab,
			Modifiers:   tcell.ModShift,
			Description: "Previous tab",
			Handler:     handlers.PreviousTab,
		})
	}
	
	if handlers.CloseTab != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'w',
			Modifiers:   tcell.ModCtrl,
			Description: "Close tab",
			Handler:     handlers.CloseTab,
		})
	}
}