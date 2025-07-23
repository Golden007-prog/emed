package ui

import (
	"fmt"
	"strings"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TerminalTab represents a single terminal session
type TerminalTab struct {
	id       int
	emulator *VT10XTerminal
}

// Terminal represents the terminal panel with tabs
type Terminal struct {
	container        *tview.Flex
	contentArea      *tview.Pages
	tabs             []*TerminalTab
	activeTab        int
	nextID           int
	noTermsView      *tview.TextView // View shown when no terminals are open
	app              *tview.Application // Reference to app for refresh
	onLastTabClosed  func() // Callback when last tab is closed
}

// NewTerminal creates a new terminal panel
func NewTerminal() *Terminal {
	t := &Terminal{
		container:   tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea: tview.NewPages(),
		tabs:        make([]*TerminalTab, 0),
		activeTab:   -1,
		nextID:      1,
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
	
	// Set up container (no tab bar, tabs will be in title)
	t.container.
		AddItem(t.contentArea, 0, 1, true).  // Content takes all space
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Terminal ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)
	// Show no terminals view initially
	t.showNoTermsView()
	
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
	
	// Set mode change callback to update status
	emulator.SetOnModeChange(func(mode TerminalMode) {
		t.updateTabBar() // Refresh to show mode indicator
	})
	
	// Set exit callback to auto-close tab when shell exits
	tabID := t.nextID // Capture the ID before it's incremented
	emulator.SetOnExit(func() {
		// Find and close the tab with this emulator
		if t.app != nil {
			t.app.QueueUpdateDraw(func() {
				for i, tab := range t.tabs {
					if tab.id == tabID {
						t.activeTab = i
						t.CloseCurrentTab()
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
		t.contentArea.AddPage(fmt.Sprintf("term-%d", t.nextID), errorView, true, false)
		return
	}
	
	tab := &TerminalTab{
		id:       t.nextID,
		emulator: emulator,
	}
	t.nextID++
	
	t.tabs = append(t.tabs, tab)
	tabIndex := len(t.tabs) - 1
	t.contentArea.AddPage(fmt.Sprintf("term-%d", tabIndex), emulator, true, false)
	
	// Switch to new tab
	t.activeTab = tabIndex
	t.switchToTab(tabIndex)
}

// showNoTermsView shows the "no terminals" message
func (t *Terminal) showNoTermsView() {
	t.contentArea.SwitchToPage("no-terms")
	t.container.SetTitle(" Terminal ")
	t.activeTab = -1
	// Reset terminal numbering when all tabs are closed
	t.nextID = 1
}

// switchToTab switches to the specified tab
func (t *Terminal) switchToTab(index int) {
	if index < 0 || index >= len(t.tabs) {
		return
	}
	
	t.activeTab = index
	t.contentArea.SwitchToPage(fmt.Sprintf("term-%d", index))
	t.updateTabBar()
}

// updateTabBar updates the title with tab information
func (t *Terminal) updateTabBar() {
	if len(t.tabs) == 0 {
		t.container.SetTitle(" Terminal ").SetTitleColor(tcell.ColorLightGray)
		return
	}
	
	// Build tab display centered around active tab
	var tabTexts []string
	maxVisible := 4 // Maximum number of tabs to show (reduced to leave room for indicators and prevent trimming)
	
	// Calculate visible range, ensuring active tab is always visible
	start := t.activeTab - 2
	end := t.activeTab + 2
	
	// Adjust if we're near the beginning
	if start < 0 {
		start = 0
		end = start + maxVisible
		if end > len(t.tabs) {
			end = len(t.tabs)
		}
	}
	
	// Adjust if we're near the end
	if end > len(t.tabs) {
		end = len(t.tabs)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}
	
	// Ensure active tab is within visible range
	if t.activeTab < start {
		start = t.activeTab
		end = start + maxVisible
		if end > len(t.tabs) {
			end = len(t.tabs)
		}
	}
	if t.activeTab >= end {
		end = t.activeTab + 1
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
		name := fmt.Sprintf("Term %d", t.tabs[i].id)
		
		// Add mode indicator for active tab in passthrough mode
		if i == t.activeTab {
			modeIndicator := ""
			if t.tabs[i].emulator != nil && t.tabs[i].emulator.GetMode() == ModePassthrough {
				modeIndicator = "[blue]*[-]"
			}
			// Highlight active tab with brackets (no color, matching editor style)
			tabTexts = append(tabTexts, fmt.Sprintf("[%s[]%s", name, modeIndicator))
		} else {
			tabTexts = append(tabTexts, name)
		}
	}
	
	// Add indicator if there are tabs after
	if end < len(t.tabs) {
		tabTexts = append(tabTexts, "...")
	}
	
	// Set title with tabs
	title := fmt.Sprintf(" %s ", strings.Join(tabTexts, " | "))
	t.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
}

// NextTab switches to the next tab
func (t *Terminal) NextTab() {
	if len(t.tabs) <= 1 {
		// Nothing to switch to
		return
	}
	next := (t.activeTab + 1) % len(t.tabs)
	t.switchToTab(next)
}

// PreviousTab switches to the previous tab
func (t *Terminal) PreviousTab() {
	if len(t.tabs) <= 1 {
		// Nothing to switch to
		return
	}
	prev := t.activeTab - 1
	if prev < 0 {
		prev = len(t.tabs) - 1
	}
	t.switchToTab(prev)
}

// CloseCurrentTab closes the current tab
func (t *Terminal) CloseCurrentTab() {
	if t.activeTab < 0 || t.activeTab >= len(t.tabs) {
		return
	}
	
	// Stop the terminal emulator
	if t.tabs[t.activeTab].emulator != nil {
		t.tabs[t.activeTab].emulator.Stop()
	}
	
	// Remove tab
	t.contentArea.RemovePage(fmt.Sprintf("term-%d", t.activeTab))
	t.tabs = append(t.tabs[:t.activeTab], t.tabs[t.activeTab+1:]...)
	
	// Check if there are no more tabs
	if len(t.tabs) == 0 {
		t.showNoTermsView()
		// Call the callback if set
		if t.onLastTabClosed != nil {
			t.onLastTabClosed()
		}
		return
	}
	
	// Adjust active tab
	if t.activeTab >= len(t.tabs) {
		t.activeTab = len(t.tabs) - 1
	}
	
	// Rebuild pages (to fix indices)
	t.rebuildPages()
	t.switchToTab(t.activeTab)
}

// rebuildPages rebuilds the page indices after tab removal
func (t *Terminal) rebuildPages() {
	// Remove all pages
	for i := range t.tabs {
		t.contentArea.RemovePage(fmt.Sprintf("term-%d", i))
	}
	// Re-add with correct indices
	for i, tab := range t.tabs {
		t.contentArea.AddPage(fmt.Sprintf("term-%d", i), tab.emulator, true, i == t.activeTab)
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
	for _, tab := range t.tabs {
		if focused == tab.emulator {
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
	return len(t.tabs) > 0
}

// SetOnLastTabClosed sets the callback to invoke when the last tab is closed
func (t *Terminal) SetOnLastTabClosed(handler func()) {
	t.onLastTabClosed = handler
}

// IsInPassthroughMode returns true if the active terminal is in passthrough mode
func (t *Terminal) IsInPassthroughMode() bool {
	if t.activeTab >= 0 && t.activeTab < len(t.tabs) {
		if t.tabs[t.activeTab].emulator != nil {
			return t.tabs[t.activeTab].emulator.GetMode() == ModePassthrough
		}
	}
	return false
}

