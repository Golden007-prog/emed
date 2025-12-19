// Package ui provides the main user interface components for the emed text editor.
package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/ui/components"
	"github.com/rivo/tview"
)

// TerminalTab represents a single terminal tab within the Terminal component.
type TerminalTab struct {
	id       int
	emulator *VT10XTerminal
	modified bool
	title    string
}

// GetID returns the unique identifier for this terminal tab.
func (t *TerminalTab) GetID() int {
	return t.id
}

// GetTitle returns the display title for this terminal tab.
func (t *TerminalTab) GetTitle() string {
	title := t.title
	if title == "" {
		title = fmt.Sprintf("Term %d", t.id)
	}
	if t.emulator != nil && t.emulator.GetMode() == ModePassthrough {
		title += " *"
	}
	return title
}

// GetContent returns the tview primitive that represents the tab content.
func (t *TerminalTab) GetContent() tview.Primitive {
	return t.emulator
}

// SetModified sets the modified state of the terminal tab.
func (t *TerminalTab) SetModified(modified bool) {
	t.modified = modified
}

// IsModified returns whether the terminal tab has been modified.
func (t *TerminalTab) IsModified() bool {
	return t.modified
}

// Terminal manages multiple terminal tabs and provides terminal functionality.
type Terminal struct {
	container       *tview.Flex
	contentArea     *tview.Pages
	tabManager      *components.BaseTabManager
	noTermsView     *tview.TextView
	app             *tview.Application
	onLastTabClosed func()
	onEmedCommand   func(cmd string, args []string)
	tabClickInfo    []components.TabClickInfo
}

// NewTerminal creates a new Terminal component with no tabs.
func NewTerminal() *Terminal {
	t := &Terminal{
		container:   tview.NewFlex().SetDirection(tview.FlexRow),
		contentArea: tview.NewPages(),
		tabManager:  components.NewBaseTabManager(),
	}

	t.contentArea.SetBackgroundColor(tcell.ColorBlack)

	t.noTermsView = tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("\n\n\nNo terminal open.\n\nPress Ctrl+T to create a new terminal.")
	t.noTermsView.SetBackgroundColor(tcell.ColorBlack)

	t.contentArea.AddPage("no-terms", t.noTermsView, true, false)

	t.container.
		AddItem(t.contentArea, 0, 1, true).
		SetBorder(true).
		SetBorderColor(tcell.NewRGBColor(102, 102, 102)).
		SetTitle(" Terminal ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)

	t.container.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			x, y := event.Position()
			containerX, containerY, containerWidth, _ := t.container.GetRect()
			
			if y == containerY && x >= containerX && x < containerX+containerWidth {
				relativeX := x - containerX - 1
				
				for _, clickInfo := range t.tabClickInfo {
					if relativeX >= clickInfo.StartX && relativeX < clickInfo.EndX {
						tabs := t.tabManager.GetTabs()
						if clickInfo.TabIndex < len(tabs) {
							tabID := tabs[clickInfo.TabIndex].GetID()
							t.tabManager.SwitchToTab(tabID)
							t.switchToTabByID(tabID)
							if t.app != nil {
								t.app.SetFocus(t.container)
							}
						}
						return action, nil
					}
				}
			}
		}
		return action, event
	})

	t.showNoTermsView()


	t.tabManager.SetOnLastTabClosed(func() {
		t.showNoTermsView()
		if t.onLastTabClosed != nil {
			t.onLastTabClosed()
		}
	})

	t.tabManager.SetOnTabClosed(func(id int) {
		t.contentArea.RemovePage(fmt.Sprintf("term-%d", id))
		t.UpdateTabBar()
	})

	return t
}

// SetApp sets the application reference for the terminal.
func (t *Terminal) SetApp(app *tview.Application) {
	t.app = app
}

// NewTab creates and adds a new terminal tab.
func (t *Terminal) NewTab() {
	emulator := NewVT10XTerminal()

	if t.app != nil {
		emulator.SetApp(t.app)
	}

	tabID := t.tabManager.GetNextTabID()

	emulator.SetOnModeChange(func(mode TerminalMode) {
		t.UpdateTabBar()
	})

	emulator.SetOnExit(func() {
		if t.app != nil {
			t.app.QueueUpdateDraw(func() {
				t.tabManager.RemoveTab(tabID)
			})
		}
	})
	
	emulator.SetOnTitleChange(func(title string) {
		if t.app != nil {
			t.app.QueueUpdateDraw(func() {
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

	emulator.SetOnEmedCommand(func(cmd string, args []string) {
		if t.onEmedCommand != nil {
			t.onEmedCommand(cmd, args)
		}
	})

	if err := emulator.Start(); err != nil {
		errorView := tview.NewTextView().
			SetText(fmt.Sprintf("Failed to start terminal: %v", err)).
			SetTextColor(tcell.ColorRed)
		t.contentArea.AddPage(fmt.Sprintf("term-%d", tabID), errorView, true, false)
		return
	}

	tab := &TerminalTab{
		id:       tabID,
		emulator: emulator,
	}

	t.contentArea.AddPage(fmt.Sprintf("term-%d", tabID), emulator, true, false)

	t.tabManager.AddTab(tab)
	t.switchToTabByID(tabID)
}

func (t *Terminal) showNoTermsView() {
	t.contentArea.SwitchToPage("no-terms")
	t.container.SetTitle(" Terminal ")
}

func (t *Terminal) switchToTabByID(id int) {
	t.contentArea.SwitchToPage(fmt.Sprintf("term-%d", id))
	t.UpdateTabBar()
}

func (t *Terminal) switchToTabByIndex(index int) {
	tabs := t.tabManager.GetTabs()
	if index >= 0 && index < len(tabs) {
		t.switchToTabByID(tabs[index].GetID())
	}
}

// UpdateTabBar refreshes the tab bar display with current tabs.
func (t *Terminal) UpdateTabBar() {
	if !t.tabManager.HasTabs() {
		t.container.SetTitle(" Terminal ").SetTitleColor(tcell.ColorLightGray)
		return
	}

	_, _, width, _ := t.container.GetRect()
	availableWidth := width - 5
	if availableWidth < 0 {
		availableWidth = 0
	}
	
	hasFocus := false
	if t.app != nil {
		hasFocus = t.HasFocus(t.app.GetFocus())
	}
	tabBarText, clickInfo := t.tabManager.GenerateTabBarTextWithPositions(availableWidth, hasFocus)
	t.tabClickInfo = clickInfo
	if tabBarText != "" {
		title := fmt.Sprintf(" %s ", tabBarText)
		t.container.SetTitle(title).SetTitleColor(tcell.ColorLightGray)
	} else {
		t.container.SetTitle(" Terminal ").SetTitleColor(tcell.ColorLightGray)
	}
}

// NextTab switches to the next terminal tab.
func (t *Terminal) NextTab() {
	tabID := t.tabManager.NextTabWithID()
	if tabID != -1 {
		t.switchToTabByID(tabID)
	}
}

// PreviousTab switches to the previous terminal tab.
func (t *Terminal) PreviousTab() {
	tabID := t.tabManager.PreviousTabWithID()
	if tabID != -1 {
		t.switchToTabByID(tabID)
	}
}

// CloseCurrentTab closes the currently active terminal tab.
func (t *Terminal) CloseCurrentTab() {
	activeTab := t.tabManager.GetActiveTab()
	if activeTab != nil {
		if termTab, ok := activeTab.(*TerminalTab); ok && termTab.emulator != nil {
			termTab.emulator.Stop()
		}
		t.tabManager.RemoveTab(activeTab.GetID())
	}
}

// GetView returns the root primitive for the terminal component.
func (t *Terminal) GetView() tview.Primitive {
	return t.container
}

// HasFocus checks if the given primitive is part of the terminal component.
func (t *Terminal) HasFocus(focused tview.Primitive) bool {
	if focused == t.container {
		return true
	}
	tabs := t.tabManager.GetTabs()
	for _, tab := range tabs {
		if focused == tab.GetContent() {
			return true
		}
	}
	if focused == t.noTermsView {
		return true
	}
	return false
}

// HasTabs returns true if there are any open terminal tabs.
func (t *Terminal) HasTabs() bool {
	return t.tabManager.HasTabs()
}

// SetOnLastTabClosed sets a callback for when the last terminal tab is closed.
func (t *Terminal) SetOnLastTabClosed(handler func()) {
	t.onLastTabClosed = handler
}

// SetOnEmedCommand sets a callback for emed OSC commands from any terminal tab.
func (t *Terminal) SetOnEmedCommand(handler func(cmd string, args []string)) {
	t.onEmedCommand = handler
}

// IsInPassthroughMode returns true if the active terminal is in passthrough mode.
func (t *Terminal) IsInPassthroughMode() bool {
	activeTab := t.tabManager.GetActiveTab()
	if termTab, ok := activeTab.(*TerminalTab); ok && termTab.emulator != nil {
		return termTab.emulator.GetMode() == ModePassthrough
	}
	return false
}

// GetCurrentPID returns the process ID of the active terminal, or 0 if none.
func (t *Terminal) GetCurrentPID() int {
	activeTab := t.tabManager.GetActiveTab()
	if termTab, ok := activeTab.(*TerminalTab); ok && termTab.emulator != nil {
		return termTab.emulator.GetPID()
	}
	return 0
}