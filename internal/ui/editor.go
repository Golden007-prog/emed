package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/emed/internal/ui/components"
	"github.com/rivo/tview"
)

// EditorTab represents a single open file and implements the Tab interface
type EditorTab struct {
	id                 int
	path               string
	modified           bool
	codeEditor         *CodeEditor
	externallyModified bool
	lastModTime        time.Time
}

// GetID returns the tab ID
func (t *EditorTab) GetID() int {
	return t.id
}

// GetTitle returns the tab title
func (t *EditorTab) GetTitle() string {
	if t.path == "" {
		return "Untitled"
	}
	return filepath.Base(t.path)
}

// GetContent returns the code editor as content
func (t *EditorTab) GetContent() tview.Primitive {
	return t.codeEditor
}

// SetModified sets the modified flag
func (t *EditorTab) SetModified(modified bool) {
	t.modified = modified
	// The codeEditor tracks its own modified state through its buffer
}

// IsModified returns the modified flag
func (t *EditorTab) IsModified() bool {
	return t.modified
}

// Editor represents the main editor panel with tabs using BaseTabManager
type Editor struct {
	container         *tview.Flex
	contentArea       *tview.Pages
	tabManager        *components.BaseTabManager
	tabs              []*EditorTab // Keep for direct access to EditorTab-specific functionality
	noDocsView        tview.Primitive // View shown when no documents are open (now a Flex container)
	app               *tview.Application
	onLastTabClosed   func()          // Callback when last tab is closed
	onTabClosed       func(path string) // Callback when any tab is closed
	fileChangeHandler *components.FileChangeHandler // Handles external file changes
	dialogManager     *EditorDialogManager // Handles all dialog operations
}

// NewEditor creates a new editor panel
func NewEditor() *Editor {
	e := &Editor{
		container:         tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea:       tview.NewPages(),
		tabManager:        components.NewBaseTabManager(),
		tabs:              make([]*EditorTab, 0),
		fileChangeHandler: components.NewFileChangeHandler(),
		dialogManager:     nil, // Will be set when app is set
	}
	
	// Set background color for content area
	e.contentArea.SetBackgroundColor(tcell.ColorBlack)

	// Create splash screen for no documents view
	splashScreen := components.NewSplashScreen()
	e.noDocsView = splashScreen.CreateNoDocsView()
	
	// Add the no docs view as a page
	e.contentArea.AddPage("no-docs", e.noDocsView, true, false)
	
	// Set up container (no tab bar, tabs will be in title)
	e.container.
		AddItem(e.contentArea, 0, 1, true).  // Content takes all space
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Editor ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)
	
	// Show no docs view initially
	e.showNoDocsView()
	
	// Set up tab manager callbacks
	e.tabManager.SetOnTabSwitched(func(id int) {
		e.switchToTabByID(id)
	})
	
	e.tabManager.SetOnLastTabClosed(func() {
		e.showNoDocsView()
		if e.onLastTabClosed != nil {
			e.onLastTabClosed()
		}
	})
	
	e.tabManager.SetOnTabClosed(func(id int) {
		// Find and remove the tab from our local list
		for i, tab := range e.tabs {
			if tab.id == id {
				// Call tab closed callback with path
				if e.onTabClosed != nil && tab.path != "" {
					e.onTabClosed(tab.path)
				}
				// Remove from content area
				e.contentArea.RemovePage(fmt.Sprintf("editor-%d", id))
				// Remove from tabs list
				e.tabs = append(e.tabs[:i], e.tabs[i+1:]...)
				// Update the tab bar to reflect the change
				e.updateTabBar()
				break
			}
		}
	})
	
	return e
}

// showNoDocsView shows the "no documents" message
func (e *Editor) showNoDocsView() {
	e.contentArea.SwitchToPage("no-docs")
	e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
}

// switchToTabByID switches to a tab by its ID (called by BaseTabManager)
func (e *Editor) switchToTabByID(id int) {
	// Find tab with this ID and switch to it
	for _, tab := range e.tabs {
		if tab.id == id {
			e.contentArea.SwitchToPage(fmt.Sprintf("editor-%d", id))
			e.updateTabBar()
			break
		}
	}
}

// OpenFile opens a file in a new tab or focuses existing tab
func (e *Editor) OpenFile(path string) {
	// Check if file is already open
	for _, tab := range e.tabs {
		if tab.path == path {
			e.tabManager.SwitchToTab(tab.id)
			return
		}
	}
	
	// Create new code editor
	codeEditor := NewCodeEditor()
	if e.app != nil {
		codeEditor.SetApp(e.app)
	}
	
	// Load the file
	err := codeEditor.LoadFile(path)
	if err != nil {
		// Show error
		return
	}
	
	// Get next tab ID from tab manager
	tabID := e.tabManager.GetNextTabID()
	
	// Set up modified callback
	codeEditor.SetOnModified(func(modified bool) {
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				// Always update tab bar when modified state changes
				e.updateTabBar()
				break
			}
		}
	})
	
	// Get file modification time
	var lastModTime time.Time
	if info, err := os.Stat(path); err == nil {
		lastModTime = info.ModTime()
	}
	
	tab := &EditorTab{
		id:                 tabID,
		path:               path,
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        lastModTime,
	}
	
	// Add to our local list and content area first
	e.tabs = append(e.tabs, tab)
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), codeEditor, true, false)
	
	// Then add to manager (which will trigger callbacks)
	e.tabManager.AddTab(tab)
}

// NewDocument creates a new untitled document
func (e *Editor) NewDocument() {
	// Create new code editor
	codeEditor := NewCodeEditor()
	if e.app != nil {
		codeEditor.SetApp(e.app)
	}
	
	// Get next tab ID from tab manager
	tabID := e.tabManager.GetNextTabID()
	
	// Set up modified callback
	codeEditor.SetOnModified(func(modified bool) {
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				// Always update tab bar when modified state changes
				e.updateTabBar()
				break
			}
		}
	})
	
	tab := &EditorTab{
		id:                 tabID,
		path:               "", // Empty path for untitled
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        time.Time{}, // No mod time for new files
	}
	
	// Add to our local list and content area first
	e.tabs = append(e.tabs, tab)
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), codeEditor, true, false)
	
	// Then add to manager (which will trigger callbacks)
	e.tabManager.AddTab(tab)
}

// ShowOpenDialog shows a file open dialog (placeholder)
func (e *Editor) ShowOpenDialog() {
	// TODO: Implement file open dialog
	// For now, just a placeholder
}

// switchToTab switches to the specified tab by index (legacy support)
func (e *Editor) switchToTab(index int) {
	if index < 0 || index >= len(e.tabs) {
		return
	}
	
	// Use BaseTabManager to switch
	e.tabManager.GoToTabByIndex(index)
}

// updateTabBar updates the title with tab information
func (e *Editor) updateTabBar() {
	if !e.tabManager.HasTabs() {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
		return
	}
	
	// Use BaseTabManager to generate tab bar text
	tabBarText := e.tabManager.GenerateTabBarText()
	if tabBarText != "" {
		title := fmt.Sprintf(" %s ", tabBarText)
		e.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
	}
}

// NextTab switches to the next tab
func (e *Editor) NextTab() {
	e.tabManager.NextTab()
}

// PreviousTab switches to the previous tab
func (e *Editor) PreviousTab() {
	e.tabManager.PreviousTab()
}

// CloseCurrentTab closes the current tab
func (e *Editor) CloseCurrentTab() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
	// Find the active tab
	var tab *EditorTab
	for _, t := range e.tabs {
		if t.id == activeID {
			tab = t
			break
		}
	}
	
	if tab == nil {
		return
	}
	
	// Check if the tab has unsaved changes
	if tab.IsModified() {
		// Show confirmation dialog
		fileName := filepath.Base(tab.path)
		if e.dialogManager != nil {
			e.dialogManager.ShowUnsavedChangesDialog(fileName,
				func() {
					// Yes - close the tab
					e.doCloseCurrentTab()
				},
				func() {
					// No - do nothing, just close modal
				})
		}
	} else {
		// No unsaved changes, close directly
		e.doCloseCurrentTab()
	}
}

// doCloseCurrentTab performs the actual tab closing
func (e *Editor) doCloseCurrentTab() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
	// Use BaseTabManager to remove the tab
	e.tabManager.RemoveTab(activeID)
}

// rebuildPages rebuilds the page indices after tab removal (no longer needed with BaseTabManager)
func (e *Editor) rebuildPages() {
	// This method is no longer needed with BaseTabManager handling tab management
}

// SaveCurrentFile saves the current file
func (e *Editor) SaveCurrentFile() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
	// Find the active tab
	var tab *EditorTab
	for _, t := range e.tabs {
		if t.id == activeID {
			tab = t
			break
		}
	}
	
	if tab == nil || tab.codeEditor == nil {
		return
	}
	
	if true {
		// For untitled files, we'd need a save dialog
		// For now, just save to the existing path
		if tab.path == "" {
			// TODO: Show save dialog
			return
		}
		
		// Check if file was modified or deleted externally
		if tab.externallyModified {
			// Check if file still exists
			if _, err := os.Stat(tab.path); os.IsNotExist(err) {
				// File was deleted externally
				if e.dialogManager != nil {
					e.showDeletedFileDialog(tab)
				}
			} else {
				// File was modified externally
				if e.dialogManager != nil {
					e.showSaveConflictDialog(tab)
				}
			}
			return
		}
		
		err := tab.codeEditor.SaveFile(tab.path)
		if err != nil {
			// TODO: Show error
			return
		}
		
		// Update modification time BEFORE the file watcher sees the change
		// This prevents the file from being reloaded unnecessarily
		if info, err := os.Stat(tab.path); err == nil {
			tab.lastModTime = info.ModTime()
		}
		
		// Update modified state in tab
		tab.modified = false
		e.updateTabBar()
	}
}

// ShowSearch shows search in current document
func (e *Editor) ShowSearch() {
	// TODO: Implement search functionality
	// For now, just a placeholder
}

// GetView returns the primitive for the editor
func (e *Editor) GetView() tview.Primitive {
	return e.container
}

// Focus is not needed - tview handles focus automatically

// HasFocus checks if the editor or any of its tabs has focus
func (e *Editor) HasFocus(focused tview.Primitive) bool {
	if focused == e.container {
		return true
	}
	// Check if any tab content has focus
	for _, tab := range e.tabs {
		if focused == tab.codeEditor {
			return true
		}
	}
	// Check the no-docs view
	if focused == e.noDocsView {
		return true
	}
	return false
}

// UpdateFilePath updates the path of an open file (for renames)
func (e *Editor) UpdateFilePath(oldPath, newPath string) {
	for _, tab := range e.tabs {
		if tab.path == oldPath {
			tab.path = newPath
			e.updateTabBar()
			return
		}
	}
}

// SetApp sets the application reference
func (e *Editor) SetApp(app *tview.Application) {
	e.app = app
	e.dialogManager = NewEditorDialogManager(app)
}

// SetOnLastTabClosed sets the callback for when the last tab is closed
func (e *Editor) SetOnLastTabClosed(handler func()) {
	e.onLastTabClosed = handler
}

// SetOnTabClosed sets the callback for when any tab is closed
func (e *Editor) SetOnTabClosed(handler func(path string)) {
	e.onTabClosed = handler
}

// HasTabs returns true if there are open tabs
func (e *Editor) HasTabs() bool {
	return e.tabManager.HasTabs()
}

// HasUnsavedChanges returns whether any tabs have unsaved changes
func (e *Editor) HasUnsavedChanges() bool {
	for _, tab := range e.tabs {
		if tab.modified {
			return true
		}
	}
	return false
}

// CloseFileTab closes a tab by file path (for deletions)
func (e *Editor) CloseFileTab(path string) {
	for _, tab := range e.tabs {
		if tab.path == path {
			// Use BaseTabManager to remove the tab
			e.tabManager.RemoveTab(tab.id)
			return
		}
	}
}

// GetNoDocsView returns the no-docs view primitive for external use
func (e *Editor) GetNoDocsView() tview.Primitive {
	return e.noDocsView
}

// showSaveConflictDialog shows a dialog when trying to save an externally modified file
func (e *Editor) showSaveConflictDialog(tab *EditorTab) {
	if e.dialogManager == nil {
		return
	}
	
	e.dialogManager.ShowSaveConflictDialog(
		tab.path,
		func() {
			// Overwrite
			tab.externallyModified = false
			err := tab.codeEditor.SaveFile(tab.path)
			if err == nil {
				// Update modification time
				if info, err := os.Stat(tab.path); err == nil {
					tab.lastModTime = info.ModTime()
				}
				tab.modified = false
				e.updateTabBar()
			}
		},
		func(copyPath string) {
			// Save as copy
			err := tab.codeEditor.SaveFile(copyPath)
			if err == nil {
				// Update tab to point to the new file
				tab.path = copyPath
				tab.externallyModified = false
				tab.modified = false
				if info, err := os.Stat(copyPath); err == nil {
					tab.lastModTime = info.ModTime()
				}
				e.updateTabBar()
			}
		},
		func() {
			// Cancel - do nothing
		},
	)
}

// HandleFileModified handles when a file is modified externally
func (e *Editor) HandleFileModified(filePath string) {
	// Check each tab
	for _, tab := range e.tabs {
		if tab.path == filePath {
			// Use FileChangeHandler to determine action
			fileInfo := &components.FileInfo{
				Path:               tab.path,
				Modified:           tab.modified,
				ExternallyModified: tab.externallyModified,
				LastModTime:        tab.lastModTime,
			}
			
			action := e.fileChangeHandler.HandleFileModified(fileInfo)
			
			switch action {
			case components.FileChangeActionReload:
				e.reloadTab(tab)
			case components.FileChangeActionMarkConflict:
				tab.externallyModified = true
			}
			break
		}
	}
}

// HandleFileDeleted handles when a file is deleted externally
func (e *Editor) HandleFileDeleted(filePath string) {
	// Find and mark the tab
	for _, tab := range e.tabs {
		if tab.path == filePath {
			// Use FileChangeHandler to determine action
			fileInfo := &components.FileInfo{
				Path:               tab.path,
				Modified:           tab.modified,
				ExternallyModified: tab.externallyModified,
				LastModTime:        tab.lastModTime,
			}
			
			action := e.fileChangeHandler.HandleFileDeleted(fileInfo)
			
			switch action {
			case components.FileChangeActionClose:
				e.tabManager.RemoveTab(tab.id)
			case components.FileChangeActionMarkDeleted:
				tab.externallyModified = true
			}
			break
		}
	}
}

// reloadTab reloads the content of a tab from disk
func (e *Editor) reloadTab(tab *EditorTab) {
	// Read the file
	content, err := os.ReadFile(tab.path)
	if err != nil {
		logging.Error("Failed to reload file %s: %v", tab.path, err)
		return
	}
	
	// Update the code editor
	tab.codeEditor.SetContent(string(content))
	tab.modified = false
	tab.externallyModified = false
	
	// Update last mod time
	if info, err := os.Stat(tab.path); err == nil {
		tab.lastModTime = info.ModTime()
	}
	
	// Redraw if this is the active tab
	if tab.id == e.tabManager.GetActiveTabID() {
		e.updateTabBar()
	}
}

// showDeletedFileDialog shows a dialog when trying to save a file that was deleted externally
func (e *Editor) showDeletedFileDialog(tab *EditorTab) {
	if e.dialogManager == nil {
		return
	}
	
	e.dialogManager.ShowDeletedFileDialog(
		tab.path,
		func() {
			// Create - save the file, recreating it
			tab.externallyModified = false
			err := tab.codeEditor.SaveFile(tab.path)
			if err == nil {
				// Update modification time
				if info, err := os.Stat(tab.path); err == nil {
					tab.lastModTime = info.ModTime()
				}
				tab.modified = false
				e.updateTabBar()
			}
		},
		func() {
			// Discard - close the tab directly, discarding all changes
			e.tabManager.RemoveTab(tab.id)
		},
		func() {
			// Cancel - keep the tab open with unsaved changes
		},
	)
}

