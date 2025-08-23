package ui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/logging"
	"github.com/rivo/tview"
)

// EditorTab represents a single open file
type EditorTab struct {
	path               string
	modified           bool
	codeEditor         *CodeEditor
	externallyModified bool
	lastModTime        time.Time
}

// Editor represents the main editor panel with tabs
type Editor struct {
	container        *tview.Flex
	contentArea      *tview.Pages
	tabs             []*EditorTab
	activeTab        int
	noDocsView       tview.Primitive // View shown when no documents are open (now a Flex container)
	app              *tview.Application
	onLastTabClosed  func()          // Callback when last tab is closed
	onTabClosed      func(path string) // Callback when any tab is closed
}

// NewEditor creates a new editor panel
func NewEditor() *Editor {
	e := &Editor{
		container:   tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea: tview.NewPages(),
		tabs:        make([]*EditorTab, 0),
		activeTab:   -1,
	}
	
	// Set background color for content area
	e.contentArea.SetBackgroundColor(tcell.ColorBlack)

	var emedTaglines = []string{
	"Efficiently Manage, Elegantly Develop.",
	"Edit More, Endlessly Debug.",
	"Every Method, Elegantly Documented.",
	"Effortlessly Merge, Easily Diff.",
	"Escape Mainframe, Enter DOS.",
	"Edit. Modify. Enhance. Deploy.",
	"Even More Errors Detected.",

	"Emmanuel's Masterfully Engineered Distraction.",
	"Eventually, My Editor Dominates.",
	"Erase My Endless Despair.",
	"Every Mistake, Eventually Discovered.",
	"Exists Mainly Expecting Donations.",
	"Emed Means Emed, Duh.",

	"Eating Many Electric Donuts.",
	"Existential Machine, Endless Dread.",
	"Every Macro Evokes Destiny.",
	"Entertaining My Electric Duck.",
	"Even My Editor Dreams.",
	"Exploding Monkeys, Every Day.",
	"Enter My Elegant Dungeon.",

	// Spanglish mix
	"Editor Magnífico, Excelente Diseño.",
	"Edita Más, Escribe Doble.",
	"El Mundo Es Digital.",
	"Error Mayor, Error Digital.",
	"Este Modo Es Diferente.",
	"El Mejor Editor, Definitivo.",
	"Escribe Más, Extra Despacio.",
	"Emmanuel's Muy Especial Device.",
	"Entra, Modifica, Elimina, Done.",
	"El Macho Editor Definitivo.",
}
	
	// Create no documents view with blue ASCII art
	helpText := `[blue]███████╗███╗   ███╗███████╗██████╗ 
██╔════╝████╗ ████║██╔════╝██╔══██╗
█████╗  ██╔████╔██║█████╗  ██║  ██║
██╔══╝  ██║╚██╔╝██║██╔══╝  ██║  ██║
███████╗██║ ╚═╝ ██║███████╗██████╔╝
╚══════╝╚═╝     ╚═╝╚══════╝╚═════╝[-]
[darkgray]{tagline}
made with ♥ in Sonora[-]

[gray]Ayuda / Help: Alt+H / F1[-]
`
	tagline := "Emmanuel's Editor"

	// Use a fun tagline instead most of the time
	if rand.Intn(10) > 0 {
		tagline = emedTaglines[rand.Intn(len(emedTaglines))]
	}

	// Replace {tagline} with actual tagline
	helpText = strings.ReplaceAll(helpText, "{tagline}", tagline)

	// Create the text view for the help text
	helpTextView := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText(helpText).
		SetDynamicColors(true)
	helpTextView.SetBackgroundColor(tcell.ColorBlack)
	
	// Create spacer boxes with black background
	topSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	bottomSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	
	// Create a flex container to center vertically
	centerFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topSpacer, 0, 1, false).  // Top spacer
		AddItem(helpTextView, 10, 0, false).  // ASCII art (fixed height)
		AddItem(bottomSpacer, 0, 1, false)  // Bottom spacer
	centerFlex.SetBackgroundColor(tcell.ColorBlack)
	
	e.noDocsView = centerFlex
	
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
	
	return e
}

// showNoDocsView shows the "no documents" message
func (e *Editor) showNoDocsView() {
	e.contentArea.SwitchToPage("no-docs")
	e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
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
	
	// Get file modification time
	var lastModTime time.Time
	if info, err := os.Stat(path); err == nil {
		lastModTime = info.ModTime()
	}
	
	tab := &EditorTab{
		path:               path,
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        lastModTime,
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
		path:               untitledName,
		modified:           false,
		codeEditor:         codeEditor,
		externallyModified: false,
		lastModTime:        time.Time{}, // No mod time for new files
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
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
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
		
		e.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		// Fallback - should never happen now
		e.container.SetTitle(" Editor ").SetTitleColor(tcell.ColorLightGray)
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
	
	// Check if the tab has unsaved changes
	tab := e.tabs[e.activeTab]
	if tab.modified {
		// Show confirmation dialog
		fileName := filepath.Base(tab.path)
		message := fmt.Sprintf("'%s' has unsaved changes.\n\nDo you want to close it anyway?", fileName)
		
		ShowConfirmModal(e.app, "Unsaved Changes", message,
			func() {
				// Yes - close the tab
				e.doCloseCurrentTab()
			},
			func() {
				// No - do nothing, just close modal
			})
	} else {
		// No unsaved changes, close directly
		e.doCloseCurrentTab()
	}
}

// doCloseCurrentTab performs the actual tab closing
func (e *Editor) doCloseCurrentTab() {
	if e.activeTab < 0 || e.activeTab >= len(e.tabs) {
		return
	}
	
	// Get the path of the file being closed
	closedPath := e.tabs[e.activeTab].path
	
	// Remove tab
	e.contentArea.RemovePage(fmt.Sprintf("tab-%d", e.activeTab))
	e.tabs = append(e.tabs[:e.activeTab], e.tabs[e.activeTab+1:]...)
	
	// Notify callback if set
	if e.onTabClosed != nil && closedPath != "" {
		e.onTabClosed(closedPath)
	}
	
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
		
		// Check if file was modified or deleted externally
		if tab.externallyModified {
			// Check if file still exists
			if _, err := os.Stat(tab.path); os.IsNotExist(err) {
				// File was deleted externally
				e.showDeletedFileDialog(tab)
			} else {
				// File was modified externally
				e.showSaveConflictDialog(tab)
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
	return len(e.tabs) > 0
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

// GetNoDocsView returns the no-docs view primitive for external use
func (e *Editor) GetNoDocsView() tview.Primitive {
	return e.noDocsView
}

// showSaveConflictDialog shows a dialog when trying to save an externally modified file
func (e *Editor) showSaveConflictDialog(tab *EditorTab) {
	if e.app == nil {
		return
	}
	
	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Conflict ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(e.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create message
	messageText := fmt.Sprintf(
		"The file '%s' has been modified externally.\n\n" +
		"Saving will overwrite the external changes.\n\n" +
		"What would you like to do?",
		filepath.Base(tab.path))
	
	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	
	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Overwrite button
	overwriteBtn := tview.NewButton("Overwrite").
		SetSelectedFunc(func() {
			// Save the file, overwriting external changes
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
			modal.Close()
		})
	overwriteBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	overwriteBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Save as Copy button
	saveCopyBtn := tview.NewButton("Save as Copy").
		SetSelectedFunc(func() {
			// Save with _copy suffix
			base := strings.TrimSuffix(tab.path, filepath.Ext(tab.path))
			ext := filepath.Ext(tab.path)
			copyPath := base + "_copy" + ext
			
			// Find unique name
			counter := 1
			for {
				if _, err := os.Stat(copyPath); os.IsNotExist(err) {
					break
				}
				copyPath = fmt.Sprintf("%s_copy%d%s", base, counter, ext)
				counter++
			}
			
			// Save to the copy path
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
			modal.Close()
		})
	saveCopyBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	saveCopyBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Cancel button
	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Add buttons to flex
	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(overwriteBtn, 12, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(saveCopyBtn, 14, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)
	
	// Set up button navigation
	overwriteBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(saveCopyBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(cancelBtn)
			return nil
		}
		return event
	})
	
	saveCopyBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(cancelBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(overwriteBtn)
			return nil
		}
		return event
	})
	
	cancelBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(overwriteBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(saveCopyBtn)
			return nil
		}
		return event
	})
	
	// Add to content
	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)
	
	// Show modal
	modal.Show()
	e.app.SetFocus(overwriteBtn)
}

// HandleFileModified handles when a file is modified externally
func (e *Editor) HandleFileModified(filePath string) {
	// Check each tab
	for _, tab := range e.tabs {
		if tab.path == filePath {
			// Check if we just saved this file ourselves
			if info, err := os.Stat(filePath); err == nil {
				// If the modification time matches what we recorded during save,
				// this is our own save, not an external modification
				if tab.lastModTime.Equal(info.ModTime()) {
					return
				}
			}
			
			// Check if the file has unsaved changes
			if tab.modified {
				// Mark as externally modified, will warn on save
				tab.externallyModified = true
			} else {
				// No unsaved changes, reload the file
				e.reloadTab(tab)
			}
			break
		}
	}
}

// HandleFileDeleted handles when a file is deleted externally
func (e *Editor) HandleFileDeleted(filePath string) {
	// Find and mark the tab
	for i, tab := range e.tabs {
		if tab.path == filePath {
			// If the file has unsaved changes, keep the tab but mark it
			if tab.modified {
				// Mark as deleted externally
				tab.externallyModified = true
			} else {
				// No unsaved changes, close the tab
				e.activeTab = i
				e.CloseCurrentTab()
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
	for i, t := range e.tabs {
		if t == tab && i == e.activeTab {
			e.updateTabBar()
			break
		}
	}
}

// showDeletedFileDialog shows a dialog when trying to save a file that was deleted externally
func (e *Editor) showDeletedFileDialog(tab *EditorTab) {
	if e.app == nil {
		return
	}
	
	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      60,
		FixedHeight:     12,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Deleted ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(e.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create message
	messageText := fmt.Sprintf(
		"The file '%s' was deleted externally.\n\n" +
		"If you save, you will recreate the file.\n\n" +
		"What would you like to do?",
		filepath.Base(tab.path))
	
	message := tview.NewTextView().
		SetText(messageText).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	
	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	// Create button
	createBtn := tview.NewButton("Create").
		SetSelectedFunc(func() {
			// Save the file, recreating it
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
			modal.Close()
		})
	createBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	createBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Discard button
	discardBtn := tview.NewButton("Discard").
		SetSelectedFunc(func() {
			// Close the tab directly, discarding all changes
			for i, t := range e.tabs {
				if t == tab {
					e.activeTab = i
					// Force close without confirmation
					e.doCloseCurrentTab()
					break
				}
			}
			modal.Close()
		})
	discardBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	discardBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Cancel button
	cancelBtn := tview.NewButton("Cancel").
		SetSelectedFunc(func() {
			// Keep the tab open with unsaved changes
			// User can try to save again later
			modal.Close()
		})
	cancelBtn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	cancelBtn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Add buttons to flex
	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(createBtn, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(discardBtn, 10, 0, false).
		AddItem(nil, 2, 0, false).
		AddItem(cancelBtn, 10, 0, false).
		AddItem(nil, 0, 1, false)
	
	// Set up button navigation
	createBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(discardBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(cancelBtn)
			return nil
		}
		return event
	})
	
	discardBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(cancelBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(createBtn)
			return nil
		}
		return event
	})
	
	cancelBtn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			e.app.SetFocus(createBtn)
			return nil
		case tcell.KeyLeft:
			e.app.SetFocus(discardBtn)
			return nil
		}
		return event
	})
	
	// Add to content
	content.
		AddItem(nil, 1, 0, false).
		AddItem(message, 0, 1, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 1, 0, false)
	
	// Show modal
	modal.Show()
	e.app.SetFocus(createBtn)
}

