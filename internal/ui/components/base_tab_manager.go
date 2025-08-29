package components

import (
	"fmt"
	"sync"
	
	"github.com/laborin/emed/internal/config"
	"github.com/laborin/emed/pkg/interfaces"
	"github.com/rivo/tview"
)

// BaseTabManager provides common tab management functionality
type BaseTabManager struct {
	tabs              []interfaces.Tab
	activeTab         int
	nextTabID         int
	mu                sync.RWMutex
	
	// Callbacks
	onTabClosed       func(id int)
	onLastTabClosed   func()
	onTabSwitched     func(id int)
	
	// UI components
	tabBar            *tview.TextView
	container         *tview.Flex
}

// NewBaseTabManager creates a new base tab manager
func NewBaseTabManager() *BaseTabManager {
	return &BaseTabManager{
		tabs:      make([]interfaces.Tab, 0),
		activeTab: -1,
		nextTabID: 1,
		tabBar:    tview.NewTextView(),
		container: tview.NewFlex(),
	}
}

// AddTab adds a new tab to the manager
func (m *BaseTabManager) AddTab(tab interfaces.Tab) {
	m.mu.Lock()
	m.tabs = append(m.tabs, tab)
	m.activeTab = len(m.tabs) - 1
	tabID := tab.GetID()
	callback := m.onTabSwitched
	m.mu.Unlock()
	
	// Call callback after releasing lock
	if callback != nil {
		callback(tabID)
	}
}

// RemoveTab removes a tab by ID
func (m *BaseTabManager) RemoveTab(id int) error {
	m.mu.Lock()
	
	for i, tab := range m.tabs {
		if tab.GetID() == id {
			// Remove the tab
			m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
			
			// Adjust active tab
			var lastTabCallback func()
			var tabClosedCallback func(int)
			var switchCallback func(int)
			var newActiveID int = -1
			
			if len(m.tabs) == 0 {
				m.activeTab = -1
				lastTabCallback = m.onLastTabClosed
			} else if m.activeTab >= len(m.tabs) {
				m.activeTab = len(m.tabs) - 1
				newActiveID = m.tabs[m.activeTab].GetID()
				switchCallback = m.onTabSwitched
			} else if m.activeTab > i {
				// Active tab was after the removed tab, adjust index
				m.activeTab--
			}
			
			tabClosedCallback = m.onTabClosed
			m.mu.Unlock()
			
			// Call callbacks after releasing lock
			if tabClosedCallback != nil {
				tabClosedCallback(id)
			}
			if switchCallback != nil && newActiveID != -1 {
				switchCallback(newActiveID)
			}
			if lastTabCallback != nil {
				lastTabCallback()
			}
			
			return nil
		}
	}
	
	m.mu.Unlock()
	return fmt.Errorf("tab with ID %d not found", id)
}

// SwitchToTab switches to a tab by ID
func (m *BaseTabManager) SwitchToTab(id int) error {
	m.mu.Lock()
	
	for i, tab := range m.tabs {
		if tab.GetID() == id {
			if m.activeTab == i {
				// Already active
				m.mu.Unlock()
				return nil
			}
			m.activeTab = i
			callback := m.onTabSwitched
			m.mu.Unlock()
			
			// Call callback after releasing lock
			if callback != nil {
				callback(id)
			}
			return nil
		}
	}
	
	m.mu.Unlock()
	return fmt.Errorf("tab with ID %d not found", id)
}

// GetActiveTabID returns the ID of the active tab
func (m *BaseTabManager) GetActiveTabID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		return m.tabs[m.activeTab].GetID()
	}
	return -1
}

// GetActiveTab returns the active tab
func (m *BaseTabManager) GetActiveTab() interfaces.Tab {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		return m.tabs[m.activeTab]
	}
	return nil
}

// GetTabCount returns the number of tabs
func (m *BaseTabManager) GetTabCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tabs)
}

// HasTabs returns true if there are any tabs
func (m *BaseTabManager) HasTabs() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tabs) > 0
}

// GetTabs returns a copy of all tabs
func (m *BaseTabManager) GetTabs() []interfaces.Tab {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]interfaces.Tab, len(m.tabs))
	copy(result, m.tabs)
	return result
}

// NextTab switches to the next tab
func (m *BaseTabManager) NextTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if len(m.tabs) > 1 {
		m.activeTab = (m.activeTab + 1) % len(m.tabs)
		if m.onTabSwitched != nil && m.activeTab < len(m.tabs) {
			m.onTabSwitched(m.tabs[m.activeTab].GetID())
		}
	}
}

// PreviousTab switches to the previous tab
func (m *BaseTabManager) PreviousTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if len(m.tabs) > 1 {
		m.activeTab--
		if m.activeTab < 0 {
			m.activeTab = len(m.tabs) - 1
		}
		if m.onTabSwitched != nil && m.activeTab < len(m.tabs) {
			m.onTabSwitched(m.tabs[m.activeTab].GetID())
		}
	}
}

// GoToTabByIndex switches to a tab by index
func (m *BaseTabManager) GoToTabByIndex(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
		if m.onTabSwitched != nil {
			m.onTabSwitched(m.tabs[m.activeTab].GetID())
		}
	}
}

// SetOnTabClosed sets the callback for when a tab is closed
func (m *BaseTabManager) SetOnTabClosed(callback func(id int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTabClosed = callback
}

// SetOnLastTabClosed sets the callback for when the last tab is closed
func (m *BaseTabManager) SetOnLastTabClosed(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLastTabClosed = callback
}

// SetOnTabSwitched sets the callback for when a tab is switched
func (m *BaseTabManager) SetOnTabSwitched(callback func(id int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTabSwitched = callback
}

// GenerateTabBarText generates the tab bar text with proper formatting
func (m *BaseTabManager) GenerateTabBarText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.tabs) == 0 {
		return ""
	}
	
	maxVisible := config.MaxVisibleTabs
	totalTabs := len(m.tabs)
	
	// Calculate visible range
	startIdx := 0
	endIdx := totalTabs
	
	if totalTabs > maxVisible {
		// Center the active tab in the visible range
		startIdx = m.activeTab - maxVisible/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + maxVisible
		if endIdx > totalTabs {
			endIdx = totalTabs
			startIdx = endIdx - maxVisible
		}
	}
	
	// Build tab bar text
	var tabText string
	
	// Add left indicator if there are hidden tabs on the left
	if startIdx > 0 {
		tabText += "[gray]◀ "
	}
	
	// Add visible tabs
	for i := startIdx; i < endIdx; i++ {
		tab := m.tabs[i]
		title := tab.GetTitle()
		
		// Add modified indicator
		if tab.IsModified() {
			title = "● " + title
		}
		
		// Format based on whether it's active
		if i == m.activeTab {
			tabText += fmt.Sprintf("[white:black:b] %s [-:-:-] ", title)
		} else {
			tabText += fmt.Sprintf("[gray] %s [-] ", title)
		}
	}
	
	// Add right indicator if there are hidden tabs on the right
	if endIdx < totalTabs {
		tabText += "[gray]▶"
	}
	
	return tabText
}

// GetNextTabID returns the next available tab ID
func (m *BaseTabManager) GetNextTabID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	id := m.nextTabID
	m.nextTabID++
	return id
}