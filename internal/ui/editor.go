package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	tabsMu            sync.RWMutex // Protects access to tabs slice
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
	
	e.tabManager.SetOnLastTabClosed(func() {
		e.showNoDocsView()
		if e.onLastTabClosed != nil {
			e.onLastTabClosed()
		}
	})
	
	e.tabManager.SetOnTabClosed(func(id int) {
		// Find and remove the tab from our local list
		e.tabsMu.Lock()
		var path string
		for i, tab := range e.tabs {
			if tab.id == id {
				path = tab.path
				// Remove from tabs list
				e.tabs = append(e.tabs[:i], e.tabs[i+1:]...)
				break
			}
		}
		e.tabsMu.Unlock()
		
		// Call tab closed callback with path
		if e.onTabClosed != nil && path != "" {
			e.onTabClosed(path)
		}
		// Remove from content area
		e.contentArea.RemovePage(fmt.Sprintf("editor-%d", id))
		// Update the tab bar to reflect the change
		e.UpdateTabBar()
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
	e.tabsMu.RLock()
	var found bool
	for _, tab := range e.tabs {
		if tab.id == id {
			found = true
			break
		}
	}
	e.tabsMu.RUnlock()
	
	if found {
		e.contentArea.SwitchToPage(fmt.Sprintf("editor-%d", id))
		e.UpdateTabBar()
	}
}

// OpenFile opens a file in a new tab or focuses existing tab
func (e *Editor) OpenFile(path string) {
	// Check if file is already open
	e.tabsMu.RLock()
	var existingTabID int = -1
	for _, tab := range e.tabs {
		if tab.path == path {
			existingTabID = tab.id
			break
		}
	}
	e.tabsMu.RUnlock()
	
	if existingTabID != -1 {
		// Switch to the existing tab
		e.tabManager.SwitchToTab(existingTabID)
		e.switchToTabByID(existingTabID)
		return
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
		e.tabsMu.RLock()
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				e.tabsMu.RUnlock()
				// Always update tab bar when modified state changes
				e.UpdateTabBar()
				return
			}
		}
		e.tabsMu.RUnlock()
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
	e.tabsMu.Lock()
	e.tabs = append(e.tabs, tab)
	e.tabsMu.Unlock()
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), codeEditor, true, false)
	
	// Add to manager and switch to the new tab
	e.tabManager.AddTab(tab)
	e.switchToTabByID(tabID)
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
		e.tabsMu.RLock()
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				e.tabsMu.RUnlock()
				// Always update tab bar when modified state changes
				e.UpdateTabBar()
				return
			}
		}
		e.tabsMu.RUnlock()
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
	e.tabsMu.Lock()
	e.tabs = append(e.tabs, tab)
	e.tabsMu.Unlock()
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), codeEditor, true, false)
	
	// Add to manager and switch to the new tab
	e.tabManager.AddTab(tab)
	e.switchToTabByID(tabID)
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
	tabID := e.tabManager.GoToTabByIndexWithID(index)
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// updateTabBar updates the title with tab information
// UpdateTabBar updates the tab bar display
func (e *Editor) UpdateTabBar() {
	if !e.tabManager.HasTabs() {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
		return
	}
	
	// Get the container width to make tabs responsive
	_, _, width, _ := e.container.GetRect()
	// tview adds padding and may add ellipsis, so we need extra buffer
	// 2 chars for tview padding + 3 chars safety margin for ellipsis
	availableWidth := width - 5
	if availableWidth < 0 {
		availableWidth = 0
	}
	
	// Use BaseTabManager to generate tab bar text
	// Check if editor has focus
	hasFocus := false
	if e.app != nil {
		hasFocus = e.HasFocus(e.app.GetFocus())
	}
	tabBarText := e.tabManager.GenerateTabBarText(availableWidth, hasFocus)
	if tabBarText != "" {
		title := fmt.Sprintf(" %s ", tabBarText)
		e.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
	}
}

// NextTab switches to the next tab
func (e *Editor) NextTab() {
	tabID := e.tabManager.NextTabWithID()
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// PreviousTab switches to the previous tab
func (e *Editor) PreviousTab() {
	tabID := e.tabManager.PreviousTabWithID()
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// CloseCurrentTab closes the current tab
func (e *Editor) CloseCurrentTab() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
	// Find the active tab
	e.tabsMu.RLock()
	var tab *EditorTab
	for _, t := range e.tabs {
		if t.id == activeID {
			tab = t
			break
		}
	}
	e.tabsMu.RUnlock()
	
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
	e.tabsMu.RLock()
	var tab *EditorTab
	for _, t := range e.tabs {
		if t.id == activeID {
			tab = t
			break
		}
	}
	e.tabsMu.RUnlock()
	
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
		e.UpdateTabBar()
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
	e.tabsMu.RLock()
	for _, tab := range e.tabs {
		if focused == tab.codeEditor {
			e.tabsMu.RUnlock()
			return true
		}
	}
	e.tabsMu.RUnlock()
	// Check the no-docs view
	if focused == e.noDocsView {
		return true
	}
	return false
}

// UpdateFilePath updates the path of an open file (for renames)
func (e *Editor) UpdateFilePath(oldPath, newPath string) {
	e.tabsMu.Lock()
	for _, tab := range e.tabs {
		if tab.path == oldPath {
			tab.path = newPath
			e.tabsMu.Unlock()
			e.UpdateTabBar()
			return
		}
	}
	e.tabsMu.Unlock()
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
	e.tabsMu.RLock()
	var tabID int = -1
	for _, tab := range e.tabs {
		if tab.path == path {
			tabID = tab.id
			break
		}
	}
	e.tabsMu.RUnlock()
	
	if tabID != -1 {
		// Use BaseTabManager to remove the tab
		e.tabManager.RemoveTab(tabID)
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
				e.UpdateTabBar()
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
				e.UpdateTabBar()
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
	e.tabsMu.Lock()
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
				e.tabsMu.Unlock()
				e.reloadTab(tab)
				return
			case components.FileChangeActionMarkConflict:
				tab.externallyModified = true
			}
			e.tabsMu.Unlock()
			return
		}
	}
	e.tabsMu.Unlock()
}

// HandleFileDeleted handles when a file is deleted externally
func (e *Editor) HandleFileDeleted(filePath string) {
	// Find and mark the tab
	e.tabsMu.Lock()
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
				tabID := tab.id
				e.tabsMu.Unlock()
				e.tabManager.RemoveTab(tabID)
				return
			case components.FileChangeActionMarkDeleted:
				tab.externallyModified = true
			}
			e.tabsMu.Unlock()
			return
		}
	}
	e.tabsMu.Unlock()
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
		e.UpdateTabBar()
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
				e.UpdateTabBar()
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

