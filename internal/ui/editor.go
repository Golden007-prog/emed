package ui

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// EditorTab represents a single open file
type EditorTab struct {
	path     string
	content  string
	modified bool
	textView *tview.TextView
}

// Editor represents the main editor panel with tabs
type Editor struct {
	container   *tview.Flex
	contentArea *tview.Pages
	tabs        []*EditorTab
	activeTab   int
	noDocsView  *tview.TextView // View shown when no documents are open
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
	helpText := `No document open.

Select a document from the Files pane.

Keyboard Shortcuts:
  Ctrl+A         - Focus File Manager
  Ctrl+E         - Focus Editor
  Ctrl+T         - Focus/Create Terminal
  Ctrl+L         - Cycle Proportions
  Ctrl+P         - Toggle Position
  Ctrl+Shift+←/→ - Switch Tabs
  Ctrl+W         - Close Tab
  Alt+H          - Show Help
  Ctrl+Q         - Quit`
	
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
		SetTitleAlign(tview.AlignLeft).
		SetBorderAttributes(tcell.AttrBold)
	
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
	
	// Read file content
	content, err := ioutil.ReadFile(path)
	if err != nil {
		// Show error
		return
	}
	
	// Create new tab
	textView := tview.NewTextView().
		SetText(string(content)).
		SetScrollable(true)
	
	tab := &EditorTab{
		path:     path,
		content:  string(content),
		modified: false,
		textView: textView,
	}
	
	e.tabs = append(e.tabs, tab)
	tabIndex := len(e.tabs) - 1
	e.contentArea.AddPage(fmt.Sprintf("tab-%d", tabIndex), textView, true, false)
	
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
	
	// Create new tab with empty content
	textView := tview.NewTextView().
		SetText("").
		SetScrollable(true)
	
	tab := &EditorTab{
		path:     untitledName,
		content:  "",
		modified: false,
		textView: textView,
	}
	
	e.tabs = append(e.tabs, tab)
	tabIndex := len(e.tabs) - 1
	e.contentArea.AddPage(fmt.Sprintf("tab-%d", tabIndex), textView, true, false)
	
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
		e.contentArea.AddPage(fmt.Sprintf("tab-%d", i), tab.textView, true, i == e.activeTab)
	}
}

// SaveCurrentFile saves the current file
func (e *Editor) SaveCurrentFile() {
	// TODO: Implement save functionality
	// For now, just a placeholder
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

// HasFocus checks if the editor or any of its tabs has focus
func (e *Editor) HasFocus(focused tview.Primitive) bool {
	if focused == e.container {
		return true
	}
	// Check if any tab content has focus
	for _, tab := range e.tabs {
		if focused == tab.textView {
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

