package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/config"
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
	fileWatcher *FileWatcher // File system watcher
	stopTicker  chan bool    // Channel to stop the status update ticker
	// No longer need pages or modal tracking
	
	// Track which panel is currently focused
	focusedPanel int // 0: fileManager, 1: editor, 2: terminal
	
	// Layout configuration
	terminalPosition string // "bottom" or "right"
	proportionIndex  int    // 0: 80/20, 1: 70/30, 2: 60/40, 3: 50/50
	proportionDir    int    // 1: increasing, -1: decreasing
}

func NewLayoutWithDependencies(app *tview.Application, fileManager *FileManager, editor *Editor, terminal *Terminal, statusBar *StatusBar) *Layout {
	l := &Layout{
		app:              app,
		fileManager:      fileManager,
		editor:          editor,
		terminal:        terminal,
		statusBar:       statusBar,
		focusedPanel:     0,        // Start with file manager focused
		terminalPosition: "bottom", // Start with terminal at bottom
		proportionIndex:  1,        // Start with 70/30
		proportionDir:    1,        // Start going forward
	}
	
	l.initialize()
	return l
}

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
	
	l.initialize()
	return l
}

func (l *Layout) initialize() {
	// Set app reference
	l.editor.SetApp(l.app)
	l.terminal.SetApp(l.app)
	l.statusBar.SetApp(l.app)
	
	// Set callback for when last tab is closed
	l.editor.SetOnLastTabClosed(func() {
		// Rebuild layout to collapse editor
		l.rebuildLayout()
		// Focus file manager
		l.focusFileManager()
	})
	
	// Set callback for when any tab is closed
	l.editor.SetOnTabClosed(func(path string) {
		// Notify file watcher that file is no longer open
		if l.fileWatcher != nil {
			l.fileWatcher.RemoveOpenFile(path)
			// Update status bar with new watched dirs count
			l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
		}
	})
	
	// Set callback for when last terminal tab is closed
	l.terminal.SetOnLastTabClosed(func() {
		// Rebuild layout to collapse terminal
		l.rebuildLayout()
		// Focus file manager or editor
		if l.editor.HasTabs() {
			l.focusEditor()
		} else {
			l.focusFileManager()
		}
	})
	
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
	l.mainArea.SetBackgroundColor(tcell.ColorBlack)
	
	// Create right side container
	l.rightSide = tview.NewFlex()
	l.rightSide.SetBackgroundColor(tcell.ColorBlack)
	
	// Build initial layout
	l.rebuildLayout()
	
	// Create main layout with status bar
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.mainArea, 0, 1, true).          // Main area takes all space except status bar
		AddItem(l.statusBar.GetView(), 1, 0, false) // Status bar is 1 line high
	mainLayout.SetBackgroundColor(tcell.ColorBlack)
	
	// Set root to main layout
	l.root = mainLayout
	
	// Set the main app root for modal restoration
	SetMainAppRoot(l.root)
	
	// NOW set app reference for filemanager (after l.root is created)
	l.fileManager.SetApp(l.app)
	
	// Set up keybindings
	l.setupKeybindings()
	
	// Set up file watcher
	l.setupFileWatcher()
	
	// Focus the file manager initially after a brief delay to ensure UI is ready
	go func() {
		time.Sleep(10 * time.Millisecond)
		l.app.QueueUpdateDraw(func() {
			l.focusFileManager()
		})
	}()
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
	
	// Check what we have open
	hasEditorTabs := l.editor.HasTabs()
	hasTerminalTabs := l.terminal.HasTabs()
	
	if !hasEditorTabs && !hasTerminalTabs {
		// Neither editor nor terminal has tabs - show no-docs view without border
		// Get the no-docs view from editor and remove its border
		noDocsView := l.editor.GetNoDocsView()
		if box, ok := noDocsView.(*tview.Box); ok {
			box.SetBorder(false)
		}
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(noDocsView, 0, 1, false)
	} else if hasEditorTabs && !hasTerminalTabs {
		// Only editor has tabs - editor takes all space
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(l.editor.GetView(), 0, 1, false)
	} else if !hasEditorTabs && hasTerminalTabs {
		// Only terminal has tabs - terminal takes all space
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(l.terminal.GetView(), 0, 1, false)
	} else {
		// Both have tabs - use proportion system
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
	}
	
	// Rebuild main area
	l.mainArea.
		AddItem(l.fileManager.GetView(), config.LayoutFileManagerProportion+10, 0, false). // File manager fixed width
		AddItem(l.rightSide, 0, 1, false)                // Editor and terminal take remaining space
}

// cycleProportion cycles through proportions in a back-and-forth pattern
// 80/20 -> 70/30 -> 60/40 -> 50/50 -> 40/60 -> 30/70 -> 20/80 -> 30/70 -> ...
func (l *Layout) cycleProportion() {
	// Only cycle proportions if terminal has tabs
	if !l.terminal.HasTabs() {
		return
	}
	
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
	// Only toggle position if terminal has tabs
	if !l.terminal.HasTabs() {
		return
	}
	
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

[blue]FILE MANAGER (when focused):[-]
  i              File actions (info, copy, rename, delete, etc.)
  Enter          Open File / Toggle Directory
  
[blue]EDITOR:[-]
  Ctrl+S         Save File (coming soon)
  Ctrl+W         Close Current Tab
  Ctrl+F         Search in Document
  Ctrl+Z         Undo (coming soon)
  Ctrl+Y         Redo (coming soon)
  
[blue]NAVIGATION:[-]
  Ctrl+A         Focus File Manager
  Ctrl+E         Focus Editor
  Ctrl+T         Focus/Create Terminal
  F2             Cycle Focus Between Panels
  
[blue]TERMINAL:[-]
  Alt+T          Toggle Terminal Mode (Passthrough/Application)
  
[blue]TABS:[-]
  Ctrl+Shift+←   Previous Tab (or F3)
  Ctrl+Shift+→   Next Tab (or F4)
  F3             Previous Tab
  F4             Next Tab
  
[blue]LAYOUT:[-]
  Ctrl+L         Cycle Proportions (or F7)
  Ctrl+P         Toggle Terminal Position (or F6)
  F6             Toggle Terminal Position
  F7             Cycle Proportions
  
[blue]GENERAL:[-]
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
		
		// Check if terminal is in passthrough mode - if so, let it handle ALL input
		if l.focusedPanel == 2 && l.terminal.IsInPassthroughMode() {
			// In passthrough mode, ALL input goes to the terminal
			// The terminal's input handler will only intercept the mode toggle (Alt+T)
			return event
		}
		
		// Handle Q key for quit when file manager is focused
		if l.focusedPanel == 0 && event.Key() == tcell.KeyRune && event.Rune() == 'q' && event.Modifiers() == 0 {
			// Q pressed with no modifiers in file manager
			l.tryQuit()
			return nil
		}
		
		// Handle F-keys
		switch event.Key() {
		case tcell.KeyF1:
			// F1 - Show help
			l.showHelp()
			return nil
		case tcell.KeyF2:
			// F2 - Cycle focus between panels
			// Skip panels that have no tabs
			hasEditor := l.editor.HasTabs()
			hasTerminal := l.terminal.HasTabs()
			
			if !hasEditor && !hasTerminal {
				// Only file manager is available
				l.focusFileManager()
			} else if !hasEditor {
				// Only file manager and terminal
				if l.focusedPanel == 0 {
					l.focusTerminal()
				} else {
					l.focusFileManager()
				}
			} else if !hasTerminal {
				// Only file manager and editor
				if l.focusedPanel == 0 {
					l.focusEditor()
				} else {
					l.focusFileManager()
				}
			} else {
				// All three panels available
				nextPanel := (l.focusedPanel + 1) % 3
				switch nextPanel {
				case 0:
					l.focusFileManager()
				case 1:
					l.focusEditor()
				case 2:
					l.focusTerminal()
				}
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
			// F6 - Toggle terminal position (only if terminal has tabs)
			if l.terminal.HasTabs() {
				l.togglePosition()
			}
			return nil
		case tcell.KeyF7:
			// F7 - Cycle proportions (only if terminal has tabs)
			if l.terminal.HasTabs() {
				l.cycleProportion()
			}
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
				// Only focus editor if there are tabs
				if l.editor.HasTabs() {
					l.focusEditor()
				}
				return nil
			case tcell.KeyCtrlT:
				// Check if terminal has focus
				if l.terminal.HasFocus(l.app.GetFocus()) {
					// If terminal is focused, create new tab
					l.terminal.NewTab()
				} else {
					// Otherwise focus terminal
					// If there are no terminals, create one first
					if !l.terminal.HasTabs() {
						l.terminal.NewTab()
						// Rebuild layout to show terminal
						l.rebuildLayout()
					}
					l.focusTerminal()
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
			case tcell.KeyCtrlZ:
				// Undo
				// TODO: Implement undo in editor
				return nil
			case tcell.KeyCtrlY:
				// Redo
				// TODO: Implement redo in editor
				return nil
			case tcell.KeyCtrlL:
				// Cycle through proportions (only if terminal has tabs)
				if l.terminal.HasTabs() {
					l.cycleProportion()
				}
				return nil
			case tcell.KeyCtrlP:
				// Toggle terminal position (only if terminal has tabs)
				if l.terminal.HasTabs() {
					l.togglePosition()
				}
				return nil
			case tcell.KeyCtrlQ:
				// Quit with confirmation
				l.tryQuit()
				return nil
			case tcell.KeyCtrlC:
				// Prevent Ctrl+C from stopping the application
				// We need to handle it here and NOT forward it as KeyCtrlC
				// because tview has a default handler that quits on KeyCtrlC
				currentFocus := l.app.GetFocus()
				if l.editor.HasFocus(currentFocus) {
					// Editor has focus - convert to a rune event with value 3 (Ctrl+C)
					// This prevents tview from seeing it as KeyCtrlC
					return tcell.NewEventKey(tcell.KeyRune, 3, tcell.ModCtrl)
				} else if l.terminal.HasFocus(currentFocus) {
					// Terminal needs the actual Ctrl+C for SIGINT
					return tcell.NewEventKey(tcell.KeyRune, 3, tcell.ModNone)
				}
				// Block it for other components
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
	// Check if editor had no tabs before opening
	hadNoTabs := !l.editor.HasTabs()
	
	l.editor.OpenFile(path)
	
	// Notify file watcher that we opened a file
	if l.fileWatcher != nil {
		l.fileWatcher.AddOpenFile(path)
		// Update status bar with new watched dirs count
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
	}
	
	// If editor was collapsed and now has tabs, rebuild layout
	if hadNoTabs && l.editor.HasTabs() {
		l.rebuildLayout()
	}
	
	l.focusEditor()
}

// setupFileWatcher sets up the file system watcher
func (l *Layout) setupFileWatcher() {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	
	// Create file watcher
	l.fileWatcher = NewFileWatcher(cwd)
	
	// Set up change handler
	l.fileWatcher.SetOnChange(func(changes []FileChange) {
		// Handle changes in the UI thread
		l.app.QueueUpdateDraw(func() {
			l.handleFileChanges(changes)
		})
	})
	
	// Start watching
	err = l.fileWatcher.Start()
	if err != nil {
		// File watching is optional, so we just log the error silently
		// The editor will still work without file watching
		return
	}
	
	// Update status bar with watched directories count
	if l.fileWatcher != nil {
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
		
		// Set up periodic update for watched dirs count  
		l.stopTicker = make(chan bool)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-l.stopTicker:
					return
				case <-ticker.C:
					if l.fileWatcher != nil {
						count := l.fileWatcher.GetWatchedDirsCount()
						l.app.QueueUpdateDraw(func() {
							l.statusBar.SetWatchedDirs(count)
						})
					}
				}
			}
		}()
	}
}

// handleFileChanges processes file system changes
func (l *Layout) handleFileChanges(changes []FileChange) {
	// Track which directories need refresh
	dirsToRefresh := make(map[string]bool)
	
	// Track which files were modified
	modifiedFiles := make(map[string]bool)
	
	// Check if git HEAD changed (branch switch)
	gitHeadChanged := false
	
	for _, change := range changes {
		// Check if this is the .git/HEAD file
		if strings.HasSuffix(change.Path, ".git/HEAD") {
			gitHeadChanged = true
			// Don't try to refresh .git directory in file manager
			continue
		}
		
		// Get the directory of the changed file
		dir := filepath.Dir(change.Path)
		dirsToRefresh[dir] = true
		
		switch change.Type {
		case FileModified:
			if !strings.HasSuffix(change.Path, "/") {
				// It's a file, not a directory
				modifiedFiles[change.Path] = true
			}
		case FileDeleted:
			// Check if any editor tabs have this file open
			l.editor.HandleFileDeleted(change.Path)
		}
	}
	
	// If git HEAD changed, refresh the branch in status bar
	if gitHeadChanged {
		l.statusBar.RefreshGitBranch()
	}
	
	// Refresh affected directories in file manager
	for dir := range dirsToRefresh {
		l.fileManager.RefreshDirectory(dir)
	}
	
	// Handle modified files in editor
	for filePath := range modifiedFiles {
		l.editor.HandleFileModified(filePath)
	}
}

// tryQuit attempts to quit the application with confirmation if needed
func (l *Layout) tryQuit() {
	// Check for unsaved changes in editor
	hasUnsaved := l.editor.HasUnsavedChanges()
	
	// Check for open terminal tabs
	hasTerminals := l.terminal.HasTabs()
	
	if hasUnsaved || hasTerminals {
		// Build confirmation message
		var messages []string
		if hasUnsaved {
			messages = append(messages, "You have unsaved changes in the editor.")
		}
		if hasTerminals {
			messages = append(messages, "You have open terminal sessions.")
		}
		messages = append(messages, "\n\nAre you sure you want to quit?")
		
		message := strings.Join(messages, " ")
		
		// Show confirmation dialog
		ShowConfirmModal(l.app, "Confirm Quit", message, 
			func() {
				// Yes callback - quit the app
				l.app.Stop()
			}, 
			func() {
				// No callback - do nothing, just close modal
			})
	} else {
		// No unsaved changes or terminals, quit directly
		l.app.Stop()
	}
}

// Stop stops the layout and cleans up resources
func (l *Layout) Stop() {
	if l.fileWatcher != nil {
		l.fileWatcher.Stop()
	}
	if l.stopTicker != nil {
		close(l.stopTicker)
	}
}