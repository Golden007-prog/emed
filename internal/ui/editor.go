// Package ui provides the main user interface components for the emed text editor.
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

// EditorTab represents a single file tab within the Editor component.
type EditorTab struct {
	id                 int
	path               string
	modified           bool
	codeEditor         *CodeEditor
	externallyModified bool
	lastModTime        time.Time
	searchBar          *SearchBar
	searchVisible      bool
	container          *tview.Flex
}

// GetID returns the unique identifier for this editor tab.
func (t *EditorTab) GetID() int {
	return t.id
}

// GetTitle returns the display title for this editor tab.
func (t *EditorTab) GetTitle() string {
	if t.path == "" {
		return "Untitled"
	}
	return filepath.Base(t.path)
}

// GetContent returns the tview primitive that represents the tab content.
func (t *EditorTab) GetContent() tview.Primitive {
	return t.container
}

// SetModified sets the modified state of the editor tab.
func (t *EditorTab) SetModified(modified bool) {
	t.modified = modified
}

// IsModified returns whether the editor tab has unsaved changes.
func (t *EditorTab) IsModified() bool {
	return t.modified
}

// Editor manages multiple file tabs and provides text editing functionality.
type Editor struct {
	container           *tview.Flex
	contentArea         *tview.Pages
	tabManager          *components.BaseTabManager
	tabs                []*EditorTab
	tabsMu              sync.RWMutex
	noDocsView          tview.Primitive
	app                 *tview.Application
	onLastTabClosed     func()
	onTabClosed         func(path string)
	onActiveFileChanged func(path string)
	fileChangeHandler   *components.FileChangeHandler
	dialogManager       *EditorDialogManager
	tabClickInfo        []components.TabClickInfo
}

// NewEditor creates a new Editor component with no open files.
func NewEditor() *Editor {
	e := &Editor{
		container:         tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea:       tview.NewPages(),
		tabManager:        components.NewBaseTabManager(),
		tabs:              make([]*EditorTab, 0),
		fileChangeHandler: components.NewFileChangeHandler(),
		dialogManager:     nil,
	}
	
	e.contentArea.SetBackgroundColor(tcell.NewRGBColor(0, 0, 0))

	splashScreen := components.NewSplashScreen()
	e.noDocsView = splashScreen.CreateNoDocsView()
	
	e.contentArea.AddPage("no-docs", e.noDocsView, true, false)
	
	e.container.
		AddItem(e.contentArea, 0, 1, true).
		SetBorder(true).
		SetBorderColor(tcell.NewRGBColor(102, 102, 102)).
		SetTitle(" Editor ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)
	
	e.container.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			x, y := event.Position()
			containerX, containerY, containerWidth, _ := e.container.GetRect()
			
			if y == containerY && x >= containerX && x < containerX+containerWidth {
				relativeX := x - containerX - 1
				
				for _, clickInfo := range e.tabClickInfo {
					if relativeX >= clickInfo.StartX && relativeX < clickInfo.EndX {
						tabs := e.tabManager.GetTabs()
						if clickInfo.TabIndex < len(tabs) {
							tabID := tabs[clickInfo.TabIndex].GetID()
							e.tabManager.SwitchToTab(tabID)
							e.switchToTabByID(tabID)
							if e.app != nil {
								e.app.SetFocus(e.container)
							}
						}
						return action, nil
					}
				}
			}
		}
		return action, event
	})
	
	e.showNoDocsView()
	
	
	e.tabManager.SetOnLastTabClosed(func() {
		e.showNoDocsView()
		if e.onLastTabClosed != nil {
			e.onLastTabClosed()
		}
	})
	
	e.tabManager.SetOnTabClosed(func(id int) {
		e.tabsMu.Lock()
		var path string
		for i, tab := range e.tabs {
			if tab.id == id {
				path = tab.path
				e.tabs = append(e.tabs[:i], e.tabs[i+1:]...)
				break
			}
		}
		e.tabsMu.Unlock()
		
		if e.onTabClosed != nil && path != "" {
			e.onTabClosed(path)
		}
		e.contentArea.RemovePage(fmt.Sprintf("editor-%d", id))
		e.UpdateTabBar()
	})
	
	return e
}

func (e *Editor) showNoDocsView() {
	e.contentArea.SwitchToPage("no-docs")
	e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
}

func (e *Editor) switchToTabByID(id int) {
	e.tabsMu.RLock()
	var found bool
	var filePath string
	for _, tab := range e.tabs {
		if tab.id == id {
			found = true
			filePath = tab.path
			break
		}
	}
	e.tabsMu.RUnlock()

	if found {
		e.contentArea.SwitchToPage(fmt.Sprintf("editor-%d", id))
		e.UpdateTabBar()
		if e.onActiveFileChanged != nil && filePath != "" {
			e.onActiveFileChanged(filePath)
		}
	}
}

func (e *Editor) switchToTabByIndex(index int) {
	tabs := e.tabManager.GetTabs()
	if index >= 0 && index < len(tabs) {
		e.switchToTabByID(tabs[index].GetID())
	}
}

// OpenFile opens a file in a new tab or switches to existing tab.
func (e *Editor) OpenFile(path string) {
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
		e.tabManager.SwitchToTab(existingTabID)
		e.switchToTabByID(existingTabID)
		return
	}
	
	codeEditor := NewCodeEditor()
	if e.app != nil {
		codeEditor.SetApp(e.app)
	}
	
	err := codeEditor.LoadFile(path)
	if err != nil {
		return
	}
	
	tabID := e.tabManager.GetNextTabID()
	
	codeEditor.SetOnModified(func(modified bool) {
		e.tabsMu.RLock()
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				e.tabsMu.RUnlock()
				e.UpdateTabBar()
				return
			}
		}
		e.tabsMu.RUnlock()
	})
	
	var lastModTime time.Time
	if info, err := os.Stat(path); err == nil {
		lastModTime = info.ModTime()
	}
	
	searchBar := NewSearchBar()
	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.AddItem(codeEditor, 0, 1, true)
	
	tab := &EditorTab{
		id:                 tabID,
		path:               path,
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        lastModTime,
		searchBar:          searchBar,
		searchVisible:      false,
		container:          container,
	}
	
	e.setupSearchBar(tab)
	
	e.tabsMu.Lock()
	e.tabs = append(e.tabs, tab)
	e.tabsMu.Unlock()
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), container, true, false)
	
	e.tabManager.AddTab(tab)
	e.switchToTabByID(tabID)
}

// OpenFileAtLine opens a file and jumps to a specific line and column
func (e *Editor) OpenFileAtLine(path string, line int, column int) {
	e.OpenFile(path)
	
	tab := e.getCurrentTab()
	if tab == nil || tab.codeEditor == nil {
		return
	}
	
	if line > 0 {
		row := line - 1
		col := column
		if col < 0 {
			col = 0
		}
		tab.codeEditor.SetCursor(row, col)
		tab.codeEditor.CenterCursor()
	}
}

// NewDocument creates a new untitled document tab.
func (e *Editor) NewDocument() {
	codeEditor := NewCodeEditor()
	if e.app != nil {
		codeEditor.SetApp(e.app)
	}
	
	tabID := e.tabManager.GetNextTabID()
	
	codeEditor.SetOnModified(func(modified bool) {
		e.tabsMu.RLock()
		for _, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.SetModified(modified)
				e.tabsMu.RUnlock()
				e.UpdateTabBar()
				return
			}
		}
		e.tabsMu.RUnlock()
	})
	
	searchBar := NewSearchBar()
	container := tview.NewFlex().SetDirection(tview.FlexRow)
	container.AddItem(codeEditor, 0, 1, true)
	
	tab := &EditorTab{
		id:                 tabID,
		path:               "",
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        time.Time{},
		searchBar:          searchBar,
		searchVisible:      false,
		container:          container,
	}
	
	e.setupSearchBar(tab)
	
	e.tabsMu.Lock()
	e.tabs = append(e.tabs, tab)
	e.tabsMu.Unlock()
	e.contentArea.AddPage(fmt.Sprintf("editor-%d", tabID), container, true, false)
	
	e.tabManager.AddTab(tab)
	e.switchToTabByID(tabID)
}

// ShowOpenDialog displays the file open dialog.
func (e *Editor) ShowOpenDialog() {
	// TODO: Implement file open dialog
}

func (e *Editor) switchToTab(index int) {
	if index < 0 || index >= len(e.tabs) {
		return
	}
	
	tabID := e.tabManager.GoToTabByIndexWithID(index)
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// UpdateTabBar refreshes the tab bar display with current tabs.
func (e *Editor) UpdateTabBar() {
	if !e.tabManager.HasTabs() {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
		return
	}
	
	_, _, width, _ := e.container.GetRect()
	availableWidth := width - 5
	if availableWidth < 0 {
		availableWidth = 0
	}
	
	hasFocus := false
	if e.app != nil {
		hasFocus = e.HasFocus(e.app.GetFocus())
	}
	tabBarText, clickInfo := e.tabManager.GenerateTabBarTextWithPositions(availableWidth, hasFocus)
	e.tabClickInfo = clickInfo
	if tabBarText != "" {
		title := fmt.Sprintf(" %s ", tabBarText)
		e.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
	}
}

// NextTab switches to the next editor tab.
func (e *Editor) NextTab() {
	tabID := e.tabManager.NextTabWithID()
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// PreviousTab switches to the previous editor tab.
func (e *Editor) PreviousTab() {
	tabID := e.tabManager.PreviousTabWithID()
	if tabID != -1 {
		e.switchToTabByID(tabID)
	}
}

// CloseCurrentTab closes the currently active editor tab.
func (e *Editor) CloseCurrentTab() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
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
	
	if tab.IsModified() {
		fileName := filepath.Base(tab.path)
		if e.dialogManager != nil {
			e.dialogManager.ShowUnsavedChangesDialog(fileName,
				func() {
					e.doCloseCurrentTab()
				},
				func() {
				})
		}
	} else {
		e.doCloseCurrentTab()
	}
}

func (e *Editor) doCloseCurrentTab() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
	e.tabManager.RemoveTab(activeID)
}

func (e *Editor) rebuildPages() {
}

// SaveCurrentFile saves the currently active file to disk.
func (e *Editor) SaveCurrentFile() {
	activeID := e.tabManager.GetActiveTabID()
	if activeID < 0 {
		return
	}
	
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
		if tab.path == "" {
			// TODO: Show save dialog
			return
		}
		
		if tab.externallyModified {
			// Check if file still exists
			if _, err := os.Stat(tab.path); os.IsNotExist(err) {
						if e.dialogManager != nil {
					e.showDeletedFileDialog(tab)
				}
			} else {
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
		
		if info, err := os.Stat(tab.path); err == nil {
			tab.lastModTime = info.ModTime()
		}
		
		tab.modified = false
		e.UpdateTabBar()
	}
}


// GetView returns the root primitive for the editor component.
func (e *Editor) GetView() tview.Primitive {
	return e.container
}


// HasFocus checks if the given primitive is part of the editor component.
func (e *Editor) HasFocus(focused tview.Primitive) bool {
	if focused == e.container {
		return true
	}
	e.tabsMu.RLock()
	for _, tab := range e.tabs {
		if focused == tab.codeEditor {
			e.tabsMu.RUnlock()
			return true
		}
	}
	e.tabsMu.RUnlock()
	if focused == e.noDocsView {
		return true
	}
	return false
}

// UpdateFilePath updates the file path for a tab when a file is renamed.
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

// SetApp sets the application reference and initializes the dialog manager.
func (e *Editor) SetApp(app *tview.Application) {
	e.app = app
	e.dialogManager = NewEditorDialogManager(app)
}

// SetOnLastTabClosed sets a callback for when the last editor tab is closed.
func (e *Editor) SetOnLastTabClosed(handler func()) {
	e.onLastTabClosed = handler
}

// SetOnTabClosed sets a callback for when any editor tab is closed.
func (e *Editor) SetOnTabClosed(handler func(path string)) {
	e.onTabClosed = handler
}

// SetOnActiveFileChanged sets a callback for when the active file changes.
func (e *Editor) SetOnActiveFileChanged(handler func(path string)) {
	e.onActiveFileChanged = handler
}

// HasTabs returns true if there are any open editor tabs.
func (e *Editor) HasTabs() bool {
	return e.tabManager.HasTabs()
}

// HasUnsavedChanges returns true if any open tab has unsaved changes.
func (e *Editor) HasUnsavedChanges() bool {
	for _, tab := range e.tabs {
		if tab.modified {
			return true
		}
	}
	return false
}

// CloseFileTab closes the tab for the specified file path.
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
			e.tabManager.RemoveTab(tabID)
	}
}

// GetNoDocsView returns the view shown when no documents are open.
func (e *Editor) GetNoDocsView() tview.Primitive {
	return e.noDocsView
}

func (e *Editor) showSaveConflictDialog(tab *EditorTab) {
	if e.dialogManager == nil {
		return
	}
	
	e.dialogManager.ShowSaveConflictDialog(
		tab.path,
		func() {
			tab.externallyModified = false
			err := tab.codeEditor.SaveFile(tab.path)
			if err == nil {
				if info, err := os.Stat(tab.path); err == nil {
					tab.lastModTime = info.ModTime()
				}
				tab.modified = false
				e.UpdateTabBar()
			}
		},
		func(copyPath string) {
			err := tab.codeEditor.SaveFile(copyPath)
			if err == nil {
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
		},
	)
}

// HandleFileModified handles external modifications to open files.
func (e *Editor) HandleFileModified(filePath string) {
	e.tabsMu.Lock()
	for _, tab := range e.tabs {
		if tab.path == filePath {
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

// HandleFileDeleted handles external deletion of open files.
func (e *Editor) HandleFileDeleted(filePath string) {
	e.tabsMu.Lock()
	for _, tab := range e.tabs {
		if tab.path == filePath {
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

func (e *Editor) reloadTab(tab *EditorTab) {
	content, err := os.ReadFile(tab.path)
	if err != nil {
		logging.Error("Failed to reload file %s: %v", tab.path, err)
		return
	}
	
	tab.codeEditor.SetContent(string(content))
	tab.modified = false
	tab.externallyModified = false
	
	if info, err := os.Stat(tab.path); err == nil {
		tab.lastModTime = info.ModTime()
	}
	
	if tab.id == e.tabManager.GetActiveTabID() {
		e.UpdateTabBar()
	}
}

func (e *Editor) showDeletedFileDialog(tab *EditorTab) {
	if e.dialogManager == nil {
		return
	}
	
	e.dialogManager.ShowDeletedFileDialog(
		tab.path,
		func() {
			tab.externallyModified = false
			err := tab.codeEditor.SaveFile(tab.path)
			if err == nil {
				if info, err := os.Stat(tab.path); err == nil {
					tab.lastModTime = info.ModTime()
				}
				tab.modified = false
				e.UpdateTabBar()
			}
		},
		func() {
			e.tabManager.RemoveTab(tab.id)
		},
		func() {
		},
	)
}

// setupSearchBar configures the search bar for a tab.
func (e *Editor) setupSearchBar(tab *EditorTab) {
	tab.searchBar.SetCallbacks(
		func(text string, caseSensitive, wholeWord, useRegex bool) {
			e.performSearch(tab, text, caseSensitive, wholeWord, useRegex)
		},
		func(searchText, replaceText string) {
			e.replaceCurrentMatch(tab, replaceText)
		},
		func(searchText, replaceText string) {
			e.replaceAllMatches(tab, replaceText)
		},
		func() {
			e.findNext(tab)
		},
		func() {
			e.findPrevious(tab)
		},
		func() {
			e.hideSearch(tab)
		},
	)
}

// ShowSearch shows the search bar for the current tab.
func (e *Editor) ShowSearch() {
	e.tabsMu.RLock()
	currentTab := e.getCurrentTab()
	e.tabsMu.RUnlock()
	
	if currentTab == nil {
		return
	}
	
	if !currentTab.searchVisible {
		currentTab.searchVisible = true
		currentTab.container.RemoveItem(currentTab.codeEditor)
		currentTab.container.AddItem(currentTab.codeEditor, 0, 1, false)
		currentTab.container.AddItem(currentTab.searchBar, 13, 0, true)
		
		selectedText := currentTab.codeEditor.GetSelectedText()
		if selectedText != "" {
			currentTab.searchBar.SetSearchText(selectedText)
		}
		
		if e.app != nil {
			e.app.SetFocus(currentTab.searchBar)
		}
	}
}

// hideSearch hides the search bar for the specified tab.
func (e *Editor) hideSearch(tab *EditorTab) {
	if tab.searchVisible {
		tab.searchVisible = false
		tab.container.RemoveItem(tab.searchBar)
		tab.container.RemoveItem(tab.codeEditor)
		tab.container.AddItem(tab.codeEditor, 0, 1, true)
		
		tab.codeEditor.ClearSearchHighlights()
		tab.codeEditor.buffer.ClearSearchMatches()
		
		if e.app != nil {
			e.app.SetFocus(tab.codeEditor)
		}
	}
}

// performSearch performs a search in the specified tab.
func (e *Editor) performSearch(tab *EditorTab, text string, caseSensitive, wholeWord, useRegex bool) {
	if text == "" {
		tab.codeEditor.ClearSearchHighlights()
		tab.codeEditor.buffer.ClearSearchMatches()
		tab.searchBar.UpdateMatches(0, 0)
		return
	}
	
	matches := tab.codeEditor.Search(text, caseSensitive, wholeWord, useRegex)
	tab.searchBar.UpdateMatches(tab.codeEditor.buffer.GetCurrentMatch())
	
	if len(matches) > 0 {
		row, col, _ := tab.codeEditor.buffer.NextMatch()
		tab.codeEditor.SetCursor(row, col)
		tab.codeEditor.ScrollToCursor()
	}
}

// findNext moves to the next search match.
func (e *Editor) findNext(tab *EditorTab) {
	row, col, found := tab.codeEditor.buffer.NextMatch()
	if found {
		tab.codeEditor.SetCursor(row, col)
		tab.codeEditor.ScrollToCursor()
		tab.searchBar.UpdateMatches(tab.codeEditor.buffer.GetCurrentMatch())
	}
}

// findPrevious moves to the previous search match.
func (e *Editor) findPrevious(tab *EditorTab) {
	row, col, found := tab.codeEditor.buffer.PreviousMatch()
	if found {
		tab.codeEditor.SetCursor(row, col)
		tab.codeEditor.ScrollToCursor()
		tab.searchBar.UpdateMatches(tab.codeEditor.buffer.GetCurrentMatch())
	}
}

// replaceCurrentMatch replaces the current search match.
func (e *Editor) replaceCurrentMatch(tab *EditorTab, replaceText string) {
	if tab.codeEditor.buffer.ReplaceCurrentMatch(replaceText) {
		tab.searchBar.UpdateMatches(tab.codeEditor.buffer.GetCurrentMatch())
		tab.SetModified(true)
		e.UpdateTabBar()
	}
}

// replaceAllMatches replaces all search matches.
func (e *Editor) replaceAllMatches(tab *EditorTab, replaceText string) {
	count := tab.codeEditor.buffer.ReplaceAllMatches(replaceText)
	if count > 0 {
		tab.codeEditor.ClearSearchHighlights()
		tab.searchBar.UpdateMatches(0, 0)
		tab.SetModified(true)
		e.UpdateTabBar()
	}
}

// getCurrentTab returns the currently active tab.
func (e *Editor) getCurrentTab() *EditorTab {
	activeID := e.tabManager.GetActiveTabID()
	for _, tab := range e.tabs {
		if tab.id == activeID {
			return tab
		}
	}
	return nil
}

// GetOpenTabs returns a map of all open tabs keyed by their file path
func (e *Editor) GetOpenTabs() map[string]*EditorTab {
	openTabs := make(map[string]*EditorTab)
	for _, tab := range e.tabs {
		if tab.path != "" {
			openTabs[tab.path] = tab
		}
	}
	return openTabs
}

// GetCurrentFileAndLine returns the current file path and 1-based line number.
func (e *Editor) GetCurrentFileAndLine() (string, int) {
	tab := e.getCurrentTab()
	if tab == nil || tab.codeEditor == nil {
		return "", 0
	}
	row, _ := tab.codeEditor.GetCursor()
	return tab.path, row + 1
}

// GetCursor returns the current cursor position (row, col) for the code editor.
func (c *CodeEditor) GetCursor() (int, int) {
	return c.cursorRow, c.cursorCol
}

