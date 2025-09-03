package components

import (
	"fmt"
	"sync"
	
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
	m.mu.Unlock()
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
			
			if len(m.tabs) == 0 {
				m.activeTab = -1
				lastTabCallback = m.onLastTabClosed
			} else if m.activeTab >= len(m.tabs) {
				m.activeTab = len(m.tabs) - 1
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
	defer m.mu.Unlock()
	
	for i, tab := range m.tabs {
		if tab.GetID() == id {
			if m.activeTab == i {
				// Already active
				return nil
			}
			m.activeTab = i
			return nil
		}
	}
	
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
	}
}

// NextTabWithID switches to the next tab and returns the new active tab ID
func (m *BaseTabManager) NextTabWithID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if len(m.tabs) > 1 {
		m.activeTab = (m.activeTab + 1) % len(m.tabs)
		if m.activeTab < len(m.tabs) {
			return m.tabs[m.activeTab].GetID()
		}
	}
	return -1
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
	}
}

// PreviousTabWithID switches to the previous tab and returns the new active tab ID
func (m *BaseTabManager) PreviousTabWithID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if len(m.tabs) > 1 {
		m.activeTab--
		if m.activeTab < 0 {
			m.activeTab = len(m.tabs) - 1
		}
		if m.activeTab < len(m.tabs) {
			return m.tabs[m.activeTab].GetID()
		}
	}
	return -1
}

// GoToTabByIndex switches to a tab by index
func (m *BaseTabManager) GoToTabByIndex(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
	}
}

// GoToTabByIndexWithID switches to a tab by index and returns the new active tab ID
func (m *BaseTabManager) GoToTabByIndexWithID(index int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
		return m.tabs[m.activeTab].GetID()
	}
	return -1
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
	// Empty - direct UI updates are handled by the parent component instead
}

// GenerateTabBarText generates the tab bar text with proper formatting
func (m *BaseTabManager) GenerateTabBarText(maxWidth int, hasFocus bool) string {
	m.mu.RLock()
	
	// Copy tab data while holding the lock
	type tabData struct {
		title    string
		modified bool
		isActive bool
	}
	tabDataList := make([]tabData, len(m.tabs))
	for i, tab := range m.tabs {
		tabDataList[i] = tabData{
			title:    tab.GetTitle(),
			modified: tab.IsModified(),
			isActive: i == m.activeTab,
		}
	}
	m.mu.RUnlock()
	
	if len(tabDataList) == 0 {
		return ""
	}
	
	// Build all tab texts first to calculate total width
	type tabInfo struct {
		text string
		width int
	}
	var allTabs []tabInfo
	totalWidth := 0
	
	for i, data := range tabDataList {
		title := data.title
		
		// Add modified indicator
		if data.modified {
			title = "● " + title
		}
		
		// Format based on whether it's active
		var tabStr string
		var displayWidth int
		if data.isActive {
			// Only use blue color if the panel has focus
			if hasFocus {
				tabStr = fmt.Sprintf("[blue][ %s ][-]", title)
			} else {
				tabStr = fmt.Sprintf("[gray][ %s ][-]", title)
			}
			displayWidth = len(title) + 4 // "[ " + title + " ]"
		} else {
			tabStr = fmt.Sprintf("[gray]%s[-]", title)
			displayWidth = len(title)
		}
		
		// Add separator after this tab if needed
		if i < len(tabDataList)-1 {
			// Check if next tab is active
			nextIsActive := tabDataList[i+1].isActive
			
			if !data.isActive && !nextIsActive {
				// Both current and next are inactive, add separator
				tabStr += " [gray]|[-] "
				displayWidth += 3 // " | "
			} else {
				// Current or next is active, just add space
				tabStr += " "
				displayWidth += 1
			}
		}
		
		allTabs = append(allTabs, tabInfo{text: tabStr, width: displayWidth})
		totalWidth += displayWidth
	}
	
	// If maxWidth is 0 or all tabs fit, return all tabs
	if maxWidth <= 0 || totalWidth <= maxWidth {
		var result string
		for _, tab := range allTabs {
			result += tab.text
		}
		return result
	}
	
	// Otherwise, show as many tabs as possible, prioritizing the active tab
	var visibleTabs []int
	usedWidth := 0
	
	// Find the active tab index
	activeIdx := -1
	for i, data := range tabDataList {
		if data.isActive {
			activeIdx = i
			break
		}
	}
	if activeIdx >= 0 {
		// Always include the active tab
		visibleTabs = append(visibleTabs, activeIdx)
		usedWidth += allTabs[activeIdx].width
		
		// Add tabs around the active tab
		left := activeIdx - 1
		right := activeIdx + 1
		for (left >= 0 || right < len(tabDataList)) && usedWidth < maxWidth {
			addedTab := false
			
			// Try to add tab on the right
			if right < len(tabDataList) && usedWidth + allTabs[right].width <= maxWidth {
				visibleTabs = append(visibleTabs, right)
				usedWidth += allTabs[right].width
				right++
				addedTab = true
			}
			
			// Try to add tab on the left
			if left >= 0 && usedWidth + allTabs[left].width <= maxWidth {
				visibleTabs = append([]int{left}, visibleTabs...)
				usedWidth += allTabs[left].width
				left--
				addedTab = true
			}
			
			// If we couldn't add any tab, break to avoid infinite loop
			if !addedTab {
				break
			}
		}
	}
	
	// Build the final tab bar text
	var tabText string
	
	// Add left indicator if there are hidden tabs on the left
	if len(visibleTabs) > 0 && visibleTabs[0] > 0 {
		tabText += "[gray]◀ [-]"
	}
	
	// Add visible tabs
	for _, idx := range visibleTabs {
		tabText += allTabs[idx].text
	}
	
	// Add right indicator if there are hidden tabs on the right
	if len(visibleTabs) > 0 && visibleTabs[len(visibleTabs)-1] < len(tabDataList)-1 {
		tabText += " [gray]▶[-]"
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