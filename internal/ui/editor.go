package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	
	"github.com/rivo/tview"
)

// EditorTab represents a single open file
type EditorTab struct {
	path       string
	modified   bool
	codeEditor *CodeEditor
}

// Editor represents the main editor panel with tabs
type Editor struct {
	container        *tview.Flex
	contentArea      *tview.Pages
	tabs             []*EditorTab
	activeTab        int
	noDocsView       *tview.TextView // View shown when no documents are open
	app              *tview.Application
	onLastTabClosed  func()          // Callback when last tab is closed
}

// NewEditor creates a new editor panel
func NewEditor() *Editor {
	e := &Editor{
		container:   tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea: tview.NewPages(),
		tabs:        make([]*EditorTab, 0),
		activeTab:   -1,
	}
	
	// Create no documents view
	helpText := `Help: ALt+H or F1

███████╗███╗   ███╗███████╗██████╗ 
██╔════╝████╗ ████║██╔════╝██╔══██╗
█████╗  ██╔████╔██║█████╗  ██║  ██║
██╔══╝  ██║╚██╔╝██║██╔══╝  ██║  ██║
███████╗██║ ╚═╝ ██║███████╗██████╔╝
╚══════╝╚═╝     ╚═╝╚══════╝╚═════╝ 
`
	
	e.noDocsView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(helpText)
	
	// Add the no docs view as a page
	e.contentArea.AddPage("no-docs", e.noDocsView, true, false)
	
	// Set up container (no tab bar, tabs will be in title)
	e.container.
		AddItem(e.contentArea, 0, 1, true).  // Content takes all space
		SetBorder(true).
		SetTitle(" Editor ").
		SetTitleAlign(tview.AlignLeft)
	
	// Show no docs view initially
	e.showNoDocsView()
	
	return e
}

// showNoDocsView shows the "no documents" message
func (e *Editor) showNoDocsView() {
	e.contentArea.SwitchToPage("no-docs")
	e.container.SetTitle(" Editor ")
	e.activeTab = -1
}

// OpenFile opens a file in a new tab or focuses existing tab
func (e *Editor) OpenFile(path string) {
	// Check if file is already open
	for i, tab := range e.tabs {
		if tab.path == path {
			e.activeTab = i
			e.switchToTab(i)
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
	
	// Set up modified callback
	codeEditor.SetOnModified(func(modified bool) {
		for i, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.modified = modified
				if i == e.activeTab {
					e.updateTabBar()
				}
				break
			}
		}
	})
	
	tab := &EditorTab{
		path:       path,
		modified:   false,
		codeEditor: codeEditor,
	}
	
	e.tabs = append(e.tabs, tab)
	tabIndex := len(e.tabs) - 1
	e.contentArea.AddPage(fmt.Sprintf("tab-%d", tabIndex), codeEditor, true, false)
	
	// Switch to new tab
	e.activeTab = tabIndex
	e.switchToTab(tabIndex)
}

// NewDocument creates a new untitled document
func (e *Editor) NewDocument() {
	// Generate a unique untitled name
	untitledNum := 1
	untitledName := fmt.Sprintf("untitled-%d", untitledNum)
	
	// Find a unique name
	for {
		exists := false
		for _, tab := range e.tabs {
			if tab.path == untitledName {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		untitledNum++
		untitledName = fmt.Sprintf("untitled-%d", untitledNum)
	}
	
	// Create new code editor
	codeEditor := NewCodeEditor()
	if e.app != nil {
		codeEditor.SetApp(e.app)
	}
	
	// Set up modified callback
	codeEditor.SetOnModified(func(modified bool) {
		for i, t := range e.tabs {
			if t.codeEditor == codeEditor {
				t.modified = modified
				if i == e.activeTab {
					e.updateTabBar()
				}
				break
			}
		}
	})
	
	tab := &EditorTab{
		path:       untitledName,
		modified:   false,
		codeEditor: codeEditor,
	}
	
	e.tabs = append(e.tabs, tab)
	tabIndex := len(e.tabs) - 1
	e.contentArea.AddPage(fmt.Sprintf("tab-%d", tabIndex), codeEditor, true, false)
	
	// Switch to new tab
	e.activeTab = tabIndex
	e.switchToTab(tabIndex)
}

// ShowOpenDialog shows a file open dialog (placeholder)
func (e *Editor) ShowOpenDialog() {
	// TODO: Implement file open dialog
	// For now, just a placeholder
}

// switchToTab switches to the specified tab
func (e *Editor) switchToTab(index int) {
	if index < 0 || index >= len(e.tabs) {
		return
	}
	
	e.activeTab = index
	e.contentArea.SwitchToPage(fmt.Sprintf("tab-%d", index))
	e.updateTabBar()
	
	// Don't set focus here - let the layout handle it
}

// updateTabBar updates the title with tab information
func (e *Editor) updateTabBar() {
	if len(e.tabs) == 0 {
		e.container.SetTitle(" Editor ")
		return
	}
	
	// Build tab display centered around active tab
	var tabTexts []string
	maxVisible := 4 // Maximum number of tabs to show (reduced to leave room for indicators and prevent trimming)
	
	// Simple approach for small number of tabs
	if len(e.tabs) <= maxVisible {
		// Show all tabs
		for i := 0; i < len(e.tabs); i++ {
			name := filepath.Base(e.tabs[i].path)
			if e.tabs[i].modified {
				name = name + "*"
			}
			
			if i == e.activeTab {
				// Highlight active tab with escaped brackets to avoid tview interpreting as tags
				tabTexts = append(tabTexts, fmt.Sprintf("[%s[]", name))
			} else {
				tabTexts = append(tabTexts, name)
			}
		}
	} else {
		// Complex logic for many tabs
		// Calculate visible range, ensuring active tab is always visible
		start := e.activeTab - 2
		end := e.activeTab + 2
		
		// Adjust if we're near the beginning
		if start < 0 {
			start = 0
			end = start + maxVisible
			if end > len(e.tabs) {
				end = len(e.tabs)
			}
		}
		
		// Adjust if we're near the end
		if end > len(e.tabs) {
			end = len(e.tabs)
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}
		
		// Ensure active tab is within visible range
		if e.activeTab < start {
			start = e.activeTab
			end = start + maxVisible
			if end > len(e.tabs) {
				end = len(e.tabs)
			}
		}
		if e.activeTab >= end {
			end = e.activeTab + 1
			start = end - maxVisible
			if start < 0 {
				start = 0
			}
		}
		
		// Add indicator if there are tabs before
		if start > 0 {
			tabTexts = append(tabTexts, "...")
		}
		
		// Add visible tabs
		for i := start; i < end; i++ {
			name := filepath.Base(e.tabs[i].path)
			if e.tabs[i].modified {
				name = name + "*"
			}
			
			if i == e.activeTab {
				// Highlight active tab with escaped brackets to avoid tview interpreting as tags
				tabTexts = append(tabTexts, fmt.Sprintf("[%s[]", name))
			} else {
				tabTexts = append(tabTexts, name)
			}
		}
		
		// Add indicator if there are tabs after
		if end < len(e.tabs) {
			tabTexts = append(tabTexts, "...")
		}
	}
	
	// Set title with tabs
	if len(tabTexts) > 0 {
		// Join the tabs and create title
		tabString := strings.Join(tabTexts, " | ")
		title := fmt.Sprintf(" %s ", tabString)
		
		// Debug: write to a temp file what we're setting
		// ioutil.WriteFile("/tmp/emed-debug.txt", []byte(fmt.Sprintf("Tabs: %d, Active: %d, Title: '%s', TabTexts: %v\n", len(e.tabs), e.activeTab, title, tabTexts)), 0644)
		
		e.container.SetTitle(title)
	} else {
		// Fallback - should never happen now
		e.container.SetTitle(" Editor ")
	}
}

// NextTab switches to the next tab
func (e *Editor) NextTab() {
	if len(e.tabs) <= 1 {
		// Nothing to switch to
		return
	}
	next := (e.activeTab + 1) % len(e.tabs)
	e.switchToTab(next)
}

// PreviousTab switches to the previous tab
func (e *Editor) PreviousTab() {
	if len(e.tabs) <= 1 {
		// Nothing to switch to
		return
	}
	prev := e.activeTab - 1
	if prev < 0 {
		prev = len(e.tabs) - 1
	}
	e.switchToTab(prev)
}

// CloseCurrentTab closes the current tab
func (e *Editor) CloseCurrentTab() {
	if e.activeTab < 0 || e.activeTab >= len(e.tabs) {
		return
	}
	
	// Remove tab
	e.contentArea.RemovePage(fmt.Sprintf("tab-%d", e.activeTab))
	e.tabs = append(e.tabs[:e.activeTab], e.tabs[e.activeTab+1:]...)
	
	// Check if there are no more tabs
	if len(e.tabs) == 0 {
		e.showNoDocsView()
		// Return true to indicate we need to focus file manager
		if e.onLastTabClosed != nil {
			e.onLastTabClosed()
		}
		return
	}
	
	// Adjust active tab
	if e.activeTab >= len(e.tabs) {
		e.activeTab = len(e.tabs) - 1
	}
	
	// Rebuild pages (to fix indices)
	e.rebuildPages()
	e.switchToTab(e.activeTab)
}

// rebuildPages rebuilds the page indices after tab removal
func (e *Editor) rebuildPages() {
	// Remove all pages
	for i := range e.tabs {
		e.contentArea.RemovePage(fmt.Sprintf("tab-%d", i))
	}
	// Re-add with correct indices
	for i, tab := range e.tabs {
		e.contentArea.AddPage(fmt.Sprintf("tab-%d", i), tab.codeEditor, true, i == e.activeTab)
	}
}

// SaveCurrentFile saves the current file
func (e *Editor) SaveCurrentFile() {
	if e.activeTab < 0 || e.activeTab >= len(e.tabs) {
		return
	}
	
	tab := e.tabs[e.activeTab]
	if tab.codeEditor != nil {
		// For untitled files, we'd need a save dialog
		// For now, just save to the existing path
		if strings.HasPrefix(tab.path, "untitled") {
			// TODO: Show save dialog
			return
		}
		
		err := tab.codeEditor.SaveFile(tab.path)
		if err != nil {
			// TODO: Show error
			return
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
}

// SetOnLastTabClosed sets the callback for when the last tab is closed
func (e *Editor) SetOnLastTabClosed(handler func()) {
	e.onLastTabClosed = handler
}

// HasTabs returns true if there are open tabs
func (e *Editor) HasTabs() bool {
	return len(e.tabs) > 0
}

// CloseFileTab closes a tab by file path (for deletions)
func (e *Editor) CloseFileTab(path string) {
	for i, tab := range e.tabs {
		if tab.path == path {
			// Remove tab
			e.contentArea.RemovePage(fmt.Sprintf("tab-%d", i))
			e.tabs = append(e.tabs[:i], e.tabs[i+1:]...)
			
			// Check if there are no more tabs
			if len(e.tabs) == 0 {
				e.showNoDocsView()
				// Call the callback when last tab is closed
				if e.onLastTabClosed != nil {
					e.onLastTabClosed()
				}
				return
			}
			
			// Adjust active tab
			if e.activeTab >= len(e.tabs) {
				e.activeTab = len(e.tabs) - 1
			} else if e.activeTab > i {
				e.activeTab--
			}
			
			// Rebuild pages
			e.rebuildPages()
			e.switchToTab(e.activeTab)
			return
		}
	}
}

