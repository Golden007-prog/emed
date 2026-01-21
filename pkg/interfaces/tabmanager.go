package interfaces

import "github.com/rivo/tview"

// Tab represents a generic tab that can be managed
type Tab interface {
	GetID() int
	GetTitle() string
	GetContent() tview.Primitive
	SetModified(modified bool)
	IsModified() bool
	IsPreview() bool
}

// TabManager defines the interface for components that manage tabs
type TabManager interface {
	// Tab operations
	NewTab() int
	CloseTab(id int) error
	SwitchToTab(id int) error
	GetActiveTabID() int
	GetTabCount() int
	HasTabs() bool
	GetTabs() []Tab

	// Navigation
	NextTab()
	PreviousTab()
	GoToTabByIndex(index int)

	// UI updates
	UpdateTabBar()
	GetView() tview.Primitive

	// Callbacks
	SetOnTabClosed(callback func(id int))
	SetOnLastTabClosed(callback func())
	SetOnTabSwitched(callback func(id int))
}
