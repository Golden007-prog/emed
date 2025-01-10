package ui

import (
	"time"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Layout manages the main application layout
type Layout struct {
	app         *tview.Application
	root        *tview.Flex
	mainArea    *tview.Flex  // Container for all panels except status bar
	rightSide   *tview.Flex  // Container for editor and terminal
	fileManager *FileManager
	editor      *Editor
	terminal    *Terminal
	statusBar   *StatusBar
	// No longer need pages or modal tracking
	
	// Track which panel is currently focused
	focusedPanel int // 0: fileManager, 1: editor, 2: terminal
	
	// Layout configuration
	terminalPosition string // "bottom" or "right"
	proportionIndex  int    // 0: 80/20, 1: 70/30, 2: 60/40, 3: 50/50
	proportionDir    int    // 1: increasing, -1: decreasing
}

// NewLayout creates the main application layout
func NewLayout(app *tview.Application) *Layout {
	l := &Layout{
		app:              app,
		focusedPanel:     0,        // Start with file manager focused
		terminalPosition: "bottom", // Start with terminal at bottom
		proportionIndex:  1,        // Start with 70/30
		proportionDir:    1,        // Start going forward
	}
	
	// Create the three main panels
	l.fileManager = NewFileManager()
	l.editor = NewEditor()
	l.terminal = NewTerminal()
	l.statusBar = NewStatusBar()
	
	// No longer need pages for modals
	
	// Connect file manager to editor
	l.fileManager.SetFileSelectedHandler(func(path string) {
		l.OpenFile(path)
	})
	
	// Handle clipboard changes
	l.fileManager.SetClipboardChangeHandler(func(path string, isCut bool) {
		l.statusBar.SetClipboard(path, isCut)
	})
	
	// Handle file renames
	l.fileManager.SetFileRenamedHandler(func(oldPath, newPath string) {
		l.editor.UpdateFilePath(oldPath, newPath)
	})
	
	// Handle file deletions
	l.fileManager.SetFileDeletedHandler(func(path string) {
		l.editor.CloseFileTab(path)
	})
	
	// Create the layout structure
	// Root contains main area and status bar
	l.root = tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Main area contains file manager and right side
	l.mainArea = tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Create right side container
	l.rightSide = tview.NewFlex()
	
	// Build initial layout
	l.rebuildLayout()
	
	// Create main layout with status bar
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.mainArea, 0, 1, true).          // Main area takes all space except status bar
		AddItem(l.statusBar.GetView(), 1, 0, false) // Status bar is 1 line high
	
	// Set root to main layout
	l.root = mainLayout
	
	// Set the main app root for modal restoration
	SetMainAppRoot(l.root)
	
	// NOW set app reference for filemanager (after l.root is created)
	l.fileManager.SetApp(app)
	
	// Set up keybindings
	l.setupKeybindings()
	
	// Focus the file manager initially after a brief delay to ensure UI is ready
	go func() {
		time.Sleep(10 * time.Millisecond)
		l.app.QueueUpdateDraw(func() {
			l.focusFileManager()
		})
	}()
	
	return l
}

// GetRoot returns the root element for the application
func (l *Layout) GetRoot() tview.Primitive {
	return l.root
}

// rebuildLayout rebuilds the layout based on current configuration
func (l *Layout) rebuildLayout() {
	// Clear right side
	l.rightSide.Clear()
	
	// Clear main area
	l.mainArea.Clear()
	
	// Get proportions based on index
	proportions := [][2]int{
		{8, 2}, // 80/20 - editor large
		{7, 3}, // 70/30
		{6, 4}, // 60/40
		{5, 5}, // 50/50 - equal
		{4, 6}, // 40/60
		{3, 7}, // 30/70
		{2, 8}, // 20/80 - terminal large
	}
	
	editorProp := proportions[l.proportionIndex][0]
	terminalProp := proportions[l.proportionIndex][1]
	
	// Configure right side based on terminal position
	if l.terminalPosition == "bottom" {
		// Terminal at bottom (vertical split)
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(l.editor.GetView(), 0, editorProp, false).
			AddItem(l.terminal.GetView(), 0, terminalProp, false)
	} else {
		// Terminal at right (horizontal split)
		l.rightSide.SetDirection(tview.FlexColumn).
			Clear().
			AddItem(l.editor.GetView(), 0, editorProp, false).
			AddItem(l.terminal.GetView(), 0, terminalProp, false)
	}
	
	// Rebuild main area
	l.mainArea.
		AddItem(l.fileManager.GetView(), 30, 0, false). // File manager fixed width 30
		AddItem(l.rightSide, 0, 1, false)                // Editor and terminal take remaining space
}

// cycleProportion cycles through proportions in a back-and-forth pattern
// 80/20 -> 70/30 -> 60/40 -> 50/50 -> 40/60 -> 30/70 -> 20/80 -> 30/70 -> ...
func (l *Layout) cycleProportion() {
	// Move in current direction
	l.proportionIndex += l.proportionDir
	
	// Check bounds and reverse direction if needed
	if l.proportionIndex >= 7 {
		l.proportionIndex = 5  // Back to 30/70
		l.proportionDir = -1    // Reverse direction
	} else if l.proportionIndex < 0 {
		l.proportionIndex = 1  // Back to 70/30
		l.proportionDir = 1     // Reverse direction
	}
	
	l.rebuildLayout()
}

// togglePosition toggles between bottom and right terminal position
func (l *Layout) togglePosition() {
	if l.terminalPosition == "bottom" {
		l.terminalPosition = "right"
	} else {
		l.terminalPosition = "bottom"
	}
	l.rebuildLayout()
}

// getHelpText returns the help text
func (l *Layout) getHelpText() string {
	return `[::b]KEYBOARD SHORTCUTS[-::-]

[yellow]FILE MANAGER (when focused):[-]
  Ctrl+N         New File (creates untitled in selected dir)
  Ctrl+M         Rename File/Directory
  Ctrl+R         Delete File/Directory
  Ctrl+C         Copy File/Directory
  Ctrl+X         Cut File/Directory
  Ctrl+V         Paste File/Directory
  Enter          Open File / Toggle Directory
  
[yellow]EDITOR:[-]
  Ctrl+S         Save File (coming soon)
  Ctrl+W         Close Current Tab
  Ctrl+F         Search in Document
  Ctrl+Z         Undo (coming soon)
  Ctrl+Y         Redo (coming soon)
  
[yellow]NAVIGATION:[-]
  Ctrl+A         Focus File Manager
  Ctrl+E         Focus Editor
  Ctrl+T         Focus/Create Terminal
  F2             Cycle Focus Between Panels
  
[yellow]TABS:[-]
  Ctrl+Shift+←   Previous Tab (or F3)
  Ctrl+Shift+→   Next Tab (or F4)
  F3             Previous Tab
  F4             Next Tab
  
[yellow]LAYOUT:[-]
  Ctrl+L         Cycle Proportions (or F7)
  Ctrl+P         Toggle Terminal Position (or F6)
  F6             Toggle Terminal Position
  F7             Cycle Proportions
  
[yellow]GENERAL:[-]
  Alt+H          Show This Help (or F1)
  F1             Show This Help
  F5             Refresh (coming soon)
  Ctrl+Q         Quit

[::d]Press ESC or click outside to close[-::-]`
}

// showHelp displays the help modal
func (l *Layout) showHelp() {
	ShowHelpModal(l.app, l.getHelpText())
}

// setupKeybindings configures global keyboard shortcuts
func (l *Layout) setupKeybindings() {
	// Global keybindings
	l.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// If a modal is open, don't process any app keybindings
		if IsModalOpen() {
			return event
		}
		
		// Determine which panel is currently focused
		currentFocus := l.app.GetFocus()
		if l.fileManager.HasFocus(currentFocus) {
			l.focusedPanel = 0
		} else if l.editor.HasFocus(currentFocus) {
			l.focusedPanel = 1
		} else if l.terminal.HasFocus(currentFocus) {
			l.focusedPanel = 2
		}
		
		// Handle F-keys
		switch event.Key() {
		case tcell.KeyF1:
			// F1 - Show help
			l.showHelp()
			return nil
		case tcell.KeyF2:
			// F2 - Cycle focus between panels
			nextPanel := (l.focusedPanel + 1) % 3
			switch nextPanel {
			case 0:
				l.focusFileManager()
			case 1:
				l.focusEditor()
			case 2:
				l.focusTerminal()
			}
			return nil
		case tcell.KeyF3:
			// F3 - Previous tab
			if l.focusedPanel == 1 {
				l.editor.PreviousTab()
			} else if l.focusedPanel == 2 {
				l.terminal.PreviousTab()
			}
			return nil
		case tcell.KeyF4:
			// F4 - Next tab
			if l.focusedPanel == 1 {
				l.editor.NextTab()
			} else if l.focusedPanel == 2 {
				l.terminal.NextTab()
			}
			return nil
		case tcell.KeyF5:
			// F5 - Refresh current directory in file manager
			if l.focusedPanel == 0 {
				l.fileManager.RefreshCurrentDirectory()
			}
			return nil
		case tcell.KeyF6:
			// F6 - Toggle terminal position
			l.togglePosition()
			return nil
		case tcell.KeyF7:
			// F7 - Cycle proportions
			l.cycleProportion()
			return nil
		}
		
		// Handle Ctrl+Shift combinations
		if event.Modifiers() == tcell.ModCtrl|tcell.ModShift {
			switch event.Key() {
			case tcell.KeyLeft:
				// Previous tab with Ctrl+Shift+Left
				if l.focusedPanel == 1 {
					l.editor.PreviousTab()
				} else if l.focusedPanel == 2 {
					l.terminal.PreviousTab()
				}
				return nil
			case tcell.KeyRight:
				// Next tab with Ctrl+Shift+Right
				if l.focusedPanel == 1 {
					l.editor.NextTab()
				} else if l.focusedPanel == 2 {
					l.terminal.NextTab()
				}
				return nil
			}
		}
		
		// Handle Ctrl combinations
		if event.Modifiers() == tcell.ModCtrl {
			switch event.Key() {
			case tcell.KeyCtrlA:
				l.focusFileManager()
				return nil
			case tcell.KeyCtrlE:
				l.focusEditor()
				return nil
			case tcell.KeyCtrlT:
				// Check if terminal has focus
				if l.terminal.HasFocus(l.app.GetFocus()) {
					// If terminal is focused, create new tab
					l.terminal.NewTab()
				} else {
					// Otherwise focus terminal
					// If there are no terminals, create one first
					if len(l.terminal.tabs) == 0 {
						l.terminal.NewTab()
					}
					l.focusTerminal()
				}
				return nil
			case tcell.KeyCtrlN:
				// New document in selected directory
				if l.focusedPanel == 0 {
					// Call directly - the method will handle queueing UI updates
					l.fileManager.CreateNewFile()
				}
				return nil
			case tcell.KeyCtrlS:
				// Save file
				if l.focusedPanel == 1 {
					l.editor.SaveCurrentFile()
				}
				return nil
			case tcell.KeyCtrlF:
				// Search
				if l.focusedPanel == 0 {
					// Fuzzy search in file manager
					l.fileManager.ShowSearch()
				} else if l.focusedPanel == 1 {
					// Search in current document
					l.editor.ShowSearch()
				}
				return nil
			case tcell.KeyCtrlW:
				// Close tab
				if l.focusedPanel == 1 {
					l.editor.CloseCurrentTab()
				} else if l.focusedPanel == 2 {
					l.terminal.CloseCurrentTab()
				}
				return nil
			case tcell.KeyCtrlC:
				if l.focusedPanel == 0 {
					// Copy in file manager
					l.fileManager.CopySelected()
				} else if l.focusedPanel == 1 {
					// Copy in editor (TODO)
				}
				return nil
			case tcell.KeyCtrlV:
				if l.focusedPanel == 0 {
					// Paste in file manager
					l.fileManager.PasteClipboard()
				} else if l.focusedPanel == 1 {
					// Paste in editor (TODO)
				}
				return nil
			case tcell.KeyCtrlX:
				if l.focusedPanel == 0 {
					// Cut in file manager
					l.fileManager.CutSelected()
				} else if l.focusedPanel == 1 {
					// Cut in editor (TODO)
				}
				return nil
			case tcell.KeyRune:
				if event.Modifiers() == tcell.ModCtrl && event.Rune() == 'm' {
					if l.focusedPanel == 0 {
						// Rename in file manager
						l.fileManager.RenameSelected()
					}
					return nil
				}
			case tcell.KeyCtrlR:
				if l.focusedPanel == 0 {
					// Delete in file manager
					l.fileManager.DeleteSelected()
				}
				return nil
			case tcell.KeyCtrlZ:
				// Undo
				// TODO: Implement undo in editor
				return nil
			case tcell.KeyCtrlY:
				// Redo
				// TODO: Implement redo in editor
				return nil
			case tcell.KeyCtrlL:
				// Cycle through proportions
				l.cycleProportion()
				return nil
			case tcell.KeyCtrlP:
				// Toggle terminal position
				l.togglePosition()
				return nil
			case tcell.KeyCtrlQ:
				// Quit
				l.app.Stop()
				return nil
			}
		}
		
		// Handle Alt combinations
		if event.Modifiers() == tcell.ModAlt {
			switch event.Key() {
			case tcell.KeyRune:
				switch event.Rune() {
				case 'h', 'H':
					// Show help with Alt+H
					l.showHelp()
					return nil
				}
			}
		}
		
		// Handle Escape to quit (alternative to Ctrl+Q)
		if event.Key() == tcell.KeyEscape {
			// Could add a confirmation dialog here
			// For now, just let it pass through
		}
		
		// Let the event pass through to the focused component
		return event
	})
}

// Focus management methods
func (l *Layout) focusFileManager() {
	l.focusedPanel = 0
	l.app.SetFocus(l.fileManager.GetView())
}

func (l *Layout) focusEditor() {
	// Clear filemanager clipboard when losing focus
	if l.focusedPanel == 0 {
		l.fileManager.ClearClipboard()
	}
	l.focusedPanel = 1
	l.app.SetFocus(l.editor.GetView())
}

func (l *Layout) focusTerminal() {
	// Clear filemanager clipboard when losing focus
	if l.focusedPanel == 0 {
		l.fileManager.ClearClipboard()
	}
	l.focusedPanel = 2
	l.app.SetFocus(l.terminal.GetView())
}

// OpenFile opens a file in the editor
func (l *Layout) OpenFile(path string) {
	l.editor.OpenFile(path)
	l.focusEditor()
}