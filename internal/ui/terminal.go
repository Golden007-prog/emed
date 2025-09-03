package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/ui/components"
	"github.com/rivo/tview"
)

// TerminalTab represents a single terminal session and implements the Tab interface
type TerminalTab struct {
	id       int
	emulator *VT10XTerminal
	modified bool
	title    string // Custom title set by terminal application
}

// GetID returns the tab ID
func (t *TerminalTab) GetID() int {
	return t.id
}

// GetTitle returns the tab title
func (t *TerminalTab) GetTitle() string {
	// Use custom title if set, otherwise default
	title := t.title
	if title == "" {
		title = fmt.Sprintf("Term %d", t.id)
	}
	// Add mode indicator for passthrough mode
	if t.emulator != nil && t.emulator.GetMode() == ModePassthrough {
		title += " *"
	}
	return title
}

// GetContent returns the terminal emulator as content
func (t *TerminalTab) GetContent() tview.Primitive {
	return t.emulator
}

// SetModified sets the modified flag
func (t *TerminalTab) SetModified(modified bool) {
	t.modified = modified
}

// IsModified returns the modified flag
func (t *TerminalTab) IsModified() bool {
	return t.modified
}

// Terminal represents the terminal panel with tabs using BaseTabManager
type Terminal struct {
	container       *tview.Flex
	contentArea     *tview.Pages
	tabManager      *components.BaseTabManager
	noTermsView     *tview.TextView // View shown when no terminals are open
	app             *tview.Application // Reference to app for refresh
	onLastTabClosed func() // Callback when last tab is closed
}

// NewTerminal creates a new terminal panel
func NewTerminal() *Terminal {
	t := &Terminal{
		container:   tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea: tview.NewPages(),
		tabManager:  components.NewBaseTabManager(),
	}

	// Set background color for content area
	t.contentArea.SetBackgroundColor(tcell.ColorBlack)

	// Create no terminals view
	t.noTermsView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("\n\n\nNo terminal open.\n\nPress Ctrl+T to create a new terminal.")
	t.noTermsView.SetBackgroundColor(tcell.ColorBlack)

	// Add the no terminals view as a page
	t.contentArea.AddPage("no-terms", t.noTermsView, true, false)

	// Set up container
	t.container.
		AddItem(t.contentArea, 0, 1, true).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Terminal ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)

	// Show no terminals view initially
	t.showNoTermsView()

	// Set up tab manager callbacks

	t.tabManager.SetOnLastTabClosed(func() {
		t.showNoTermsView()
		if t.onLastTabClosed != nil {
			t.onLastTabClosed()
		}
	})

	t.tabManager.SetOnTabClosed(func(id int) {
		// Remove from content area
		t.contentArea.RemovePage(fmt.Sprintf("term-%d", id))
		// Update the tab bar to reflect the change
		t.UpdateTabBar()
	})

	return t
}

// SetApp sets the application reference for refresh
func (t *Terminal) SetApp(app *tview.Application) {
	t.app = app
}

// NewTab creates a new terminal tab
func (t *Terminal) NewTab() {
	// Create terminal emulator
	emulator := NewVT10XTerminal()

	// Set app reference for refresh
	if t.app != nil {
		emulator.SetApp(t.app)
	}

	// Get next tab ID from tab manager
	tabID := t.tabManager.GetNextTabID()

	// Set mode change callback to update status
	emulator.SetOnModeChange(func(mode TerminalMode) {
		t.UpdateTabBar() // Refresh to show mode indicator
	})

	// Set exit callback to auto-close tab when shell exits
	emulator.SetOnExit(func() {
		// Close the tab with this emulator
		if t.app != nil {
			t.app.QueueUpdateDraw(func() {
				t.tabManager.RemoveTab(tabID)
			})
		}
	})
	
	// Set title change callback to update tab title
	emulator.SetOnTitleChange(func(title string) {
		if t.app != nil {
			t.app.QueueUpdateDraw(func() {
				// Find the tab and update its title
				tabs := t.tabManager.GetTabs()
				for _, tab := range tabs {
					if tab.GetID() == tabID {
						if termTab, ok := tab.(*TerminalTab); ok {
							termTab.title = title
							t.UpdateTabBar()
						}
						break
					}
				}
			})
		}
	})

	// Start the terminal
	if err := emulator.Start(); err != nil {
		// Fall back to a text view with error message
		errorView := tview.NewTextView().
			SetText(fmt.Sprintf("Failed to start terminal: %v", err)).
			SetTextColor(tcell.ColorRed)
		t.contentArea.AddPage(fmt.Sprintf("term-%d", tabID), errorView, true, false)
		return
	}

	// Create terminal tab and add to manager
	tab := &TerminalTab{
		id:       tabID,
		emulator: emulator,
	}

	// Add page to content area
	t.contentArea.AddPage(fmt.Sprintf("term-%d", tabID), emulator, true, false)

	// Add tab to manager and switch to it
	t.tabManager.AddTab(tab)
	t.switchToTabByID(tabID)
}

// showNoTermsView shows the "no terminals" message
func (t *Terminal) showNoTermsView() {
	t.contentArea.SwitchToPage("no-terms")
	t.container.SetTitle(" Terminal ")
}

// switchToTabByID switches to the specified tab by ID
func (t *Terminal) switchToTabByID(id int) {
	t.contentArea.SwitchToPage(fmt.Sprintf("term-%d", id))
	t.UpdateTabBar()
}

// updateTabBar updates the title with tab information
// UpdateTabBar updates the tab bar display
func (t *Terminal) UpdateTabBar() {
	if !t.tabManager.HasTabs() {
		t.container.SetTitle(" Terminal ").SetTitleColor(tcell.ColorLightGray)
		return
	}

	// Get the container width to make tabs responsive
	_, _, width, _ := t.container.GetRect()
	// tview adds padding and may add ellipsis, so we need extra buffer
	// 2 chars for tview padding + 3 chars safety margin for ellipsis
	availableWidth := width - 5
	if availableWidth < 0 {
		availableWidth = 0
	}
	
	// Use BaseTabManager to generate tab bar text (same as Editor for consistency)
	// Check if terminal has focus
	hasFocus := false
	if t.app != nil {
		hasFocus = t.HasFocus(t.app.GetFocus())
	}
	tabBarText := t.tabManager.GenerateTabBarText(availableWidth, hasFocus)
	if tabBarText != "" {
		title := fmt.Sprintf(" %s ", tabBarText)
		t.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		t.container.SetTitle(" Terminal ").SetTitleColor(tcell.ColorLightGray)
	}
}

// NextTab switches to the next tab
func (t *Terminal) NextTab() {
	tabID := t.tabManager.NextTabWithID()
	if tabID != -1 {
		t.switchToTabByID(tabID)
	}
}

// PreviousTab switches to the previous tab
func (t *Terminal) PreviousTab() {
	tabID := t.tabManager.PreviousTabWithID()
	if tabID != -1 {
		t.switchToTabByID(tabID)
	}
}

// CloseCurrentTab closes the current tab
func (t *Terminal) CloseCurrentTab() {
	activeTab := t.tabManager.GetActiveTab()
	if activeTab != nil {
		// Stop the terminal emulator if it's a TerminalTab
		if termTab, ok := activeTab.(*TerminalTab); ok && termTab.emulator != nil {
			termTab.emulator.Stop()
		}
		// Remove from tab manager
		t.tabManager.RemoveTab(activeTab.GetID())
	}
}

// GetView returns the primitive for the terminal
func (t *Terminal) GetView() tview.Primitive {
	return t.container
}

// HasFocus checks if the terminal or any of its tabs has focus
func (t *Terminal) HasFocus(focused tview.Primitive) bool {
	if focused == t.container {
		return true
	}
	// Check if any tab content has focus
	tabs := t.tabManager.GetTabs()
	for _, tab := range tabs {
		if focused == tab.GetContent() {
			return true
		}
	}
	// Check the no-terms view
	if focused == t.noTermsView {
		return true
	}
	return false
}

// HasTabs returns true if there are open terminal tabs
func (t *Terminal) HasTabs() bool {
	return t.tabManager.HasTabs()
}

// SetOnLastTabClosed sets the callback to invoke when the last tab is closed
func (t *Terminal) SetOnLastTabClosed(handler func()) {
	t.onLastTabClosed = handler
}

// IsInPassthroughMode returns true if the active terminal is in passthrough mode
func (t *Terminal) IsInPassthroughMode() bool {
	activeTab := t.tabManager.GetActiveTab()
	if termTab, ok := activeTab.(*TerminalTab); ok && termTab.emulator != nil {
		return termTab.emulator.GetMode() == ModePassthrough
	}
	return false
}