// Package keybindings provides keyboard shortcut handlers and management for the editor.
package keybindings

import "github.com/gdamore/tcell/v2"

// FileOperationHandlers contains handlers for file operation shortcuts.
type FileOperationHandlers struct {
	Save         func() error
	SaveAs       func() error
	NewFile      func() error
	NewFolder    func() error
	DeleteFile   func() error
	RenameFile   func() error
	CopyFile     func() error
	CutFile      func() error
	PasteFile    func() error
}

// RegisterFileOperationHandlers registers file operation keyboard shortcuts with the manager.
func RegisterFileOperationHandlers(manager *KeyBindingManager, handlers FileOperationHandlers) {
	if handlers.Save != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        's',
			Modifiers:   tcell.ModCtrl,
			Description: "Save file",
			Handler:     handlers.Save,
		})
	}
	
	if handlers.NewFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'n',
			Modifiers:   tcell.ModCtrl,
			Description: "New file",
			Handler:     handlers.NewFile,
		})
	}
	
	if handlers.NewFolder != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'N',
			Modifiers:   tcell.ModCtrl | tcell.ModShift,
			Description: "New folder",
			Handler:     handlers.NewFolder,
		})
	}
	
	if handlers.DeleteFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyDelete,
			Modifiers:   tcell.ModNone,
			Description: "Delete file",
			Handler:     handlers.DeleteFile,
		})
	}
	
	if handlers.RenameFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyF2,
			Modifiers:   tcell.ModNone,
			Description: "Rename file",
			Handler:     handlers.RenameFile,
		})
	}
	
	if handlers.CopyFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'c',
			Modifiers:   tcell.ModCtrl,
			Description: "Copy file",
			Handler:     handlers.CopyFile,
		})
	}
	
	if handlers.CutFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'x',
			Modifiers:   tcell.ModCtrl,
			Description: "Cut file",
			Handler:     handlers.CutFile,
		})
	}
	
	if handlers.PasteFile != nil {
		manager.RegisterHandler(KeyHandler{
			Key:         tcell.KeyRune,
			Rune:        'v',
			Modifiers:   tcell.ModCtrl,
			Description: "Paste file",
			Handler:     handlers.PasteFile,
		})
	}
}