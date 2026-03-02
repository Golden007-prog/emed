// Package components provides reusable UI components.
package components

import (
	"fmt"
	"sync"

	"github.com/rivo/tview"
)

// Tab represents the minimal contract needed by BaseTabManager.
type Tab interface {
	GetID() int
	GetTitle() string
	GetContent() tview.Primitive
	SetModified(modified bool)
	IsModified() bool
	IsPreview() bool
}

// BaseTabManager manages tabs and provides tab switching functionality.
type BaseTabManager struct {
	tabs      []Tab
	activeTab int
	nextTabID int
	mu        sync.RWMutex

	onTabClosed     func(id int)
	onLastTabClosed func()

	tabBar    *tview.TextView
	container *tview.Flex
}

// NewBaseTabManager creates a new tab manager.
func NewBaseTabManager() *BaseTabManager {
	return &BaseTabManager{
		tabs:      make([]Tab, 0),
		activeTab: -1,
		nextTabID: 1,
		tabBar:    tview.NewTextView(),
		container: tview.NewFlex(),
	}
}

// AddTab adds a new tab and makes it active.
func (m *BaseTabManager) AddTab(tab Tab) {
	m.mu.Lock()
	m.tabs = append(m.tabs, tab)
	m.activeTab = len(m.tabs) - 1
	m.mu.Unlock()
}

// RemoveTab removes a tab by its ID.
func (m *BaseTabManager) RemoveTab(id int) error {
	m.mu.Lock()

	for i, tab := range m.tabs {
		if tab.GetID() == id {
			m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)

			var lastTabCallback func()
			var tabClosedCallback func(int)

			if len(m.tabs) == 0 {
				m.activeTab = -1
				lastTabCallback = m.onLastTabClosed
			} else if m.activeTab >= len(m.tabs) {
				m.activeTab = len(m.tabs) - 1
			} else if m.activeTab > i {
				m.activeTab--
			}

			tabClosedCallback = m.onTabClosed
			m.mu.Unlock()

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

// RemoveTabSilently removes a tab without triggering callbacks.
// Used when replacing preview tabs to avoid triggering onLastTabClosed.
func (m *BaseTabManager) RemoveTabSilently(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, tab := range m.tabs {
		if tab.GetID() == id {
			m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)

			if len(m.tabs) == 0 {
				m.activeTab = -1
			} else if m.activeTab >= len(m.tabs) {
				m.activeTab = len(m.tabs) - 1
			} else if m.activeTab > i {
				m.activeTab--
			}
			return
		}
	}
}

// SwitchToTab switches to the tab with the given ID.
func (m *BaseTabManager) SwitchToTab(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, tab := range m.tabs {
		if tab.GetID() == id {
			if m.activeTab == i {
				return nil
			}
			m.activeTab = i
			return nil
		}
	}

	return fmt.Errorf("tab with ID %d not found", id)
}

// GetActiveTabID returns the ID of the currently active tab.
func (m *BaseTabManager) GetActiveTabID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		return m.tabs[m.activeTab].GetID()
	}
	return -1
}

// GetActiveTab returns the currently active tab.
func (m *BaseTabManager) GetActiveTab() Tab {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		return m.tabs[m.activeTab]
	}
	return nil
}

// GetTabCount returns the number of open tabs.
func (m *BaseTabManager) GetTabCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tabs)
}

// HasTabs returns true if there are any open tabs.
func (m *BaseTabManager) HasTabs() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tabs) > 0
}

// GetTabs returns a copy of all tabs.
func (m *BaseTabManager) GetTabs() []Tab {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Tab, len(m.tabs))
	copy(result, m.tabs)
	return result
}

// NextTab switches to the next tab.
func (m *BaseTabManager) NextTab() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.tabs) > 1 {
		m.activeTab = (m.activeTab + 1) % len(m.tabs)
	}
}

// NextTabWithID switches to the next tab and returns its ID.
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

// PreviousTab switches to the previous tab.
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

// PreviousTabWithID switches to the previous tab and returns its ID.
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

// GoToTabByIndex switches to the tab at the specified index.
func (m *BaseTabManager) GoToTabByIndex(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
	}
}

// GoToTabByIndexWithID switches to the tab at the specified index and returns its ID.
func (m *BaseTabManager) GoToTabByIndexWithID(index int) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.tabs) {
		m.activeTab = index
		return m.tabs[m.activeTab].GetID()
	}
	return -1
}

// SetOnTabClosed sets the callback for when a tab is closed.
func (m *BaseTabManager) SetOnTabClosed(callback func(id int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTabClosed = callback
}

// SetOnLastTabClosed sets the callback for when the last tab is closed.
func (m *BaseTabManager) SetOnLastTabClosed(callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onLastTabClosed = callback
}

// SetOnTabSwitched sets the callback for when a tab is switched (currently not implemented).
func (m *BaseTabManager) SetOnTabSwitched(callback func(id int)) {
}

// TabClickInfo contains information about clickable tab areas.
type TabClickInfo struct {
	TabIndex int
	StartX   int
	EndX     int
}

// GenerateTabBarText generates the text representation of the tab bar.
func (m *BaseTabManager) GenerateTabBarText(maxWidth int, hasFocus bool) string {
	text, _ := m.GenerateTabBarTextWithPositions(maxWidth, hasFocus)
	return text
}

// GenerateTabBarTextWithPositions generates tab bar text with click position info.
func (m *BaseTabManager) GenerateTabBarTextWithPositions(maxWidth int, hasFocus bool) (string, []TabClickInfo) {
	m.mu.RLock()

	type tabData struct {
		title     string
		modified  bool
		isActive  bool
		isPreview bool
	}
	tabDataList := make([]tabData, len(m.tabs))
	for i, tab := range m.tabs {
		tabDataList[i] = tabData{
			title:     tab.GetTitle(),
			modified:  tab.IsModified(),
			isActive:  i == m.activeTab,
			isPreview: tab.IsPreview(),
		}
	}
	m.mu.RUnlock()

	if len(tabDataList) == 0 {
		return "", nil
	}

	type tabInfo struct {
		text  string
		width int
	}
	var allTabs []tabInfo
	totalWidth := 0

	for i, data := range tabDataList {
		title := data.title

		if data.modified {
			title = "● " + title
		}

		var tabStr string
		var displayWidth int
		if data.isActive {
			if hasFocus {
				if data.isPreview {
					tabStr = fmt.Sprintf("[blue::i][ %s ][-::-]", title)
				} else {
					tabStr = fmt.Sprintf("[blue][ %s ][-]", title)
				}
			} else {
				if data.isPreview {
					tabStr = fmt.Sprintf("[white::i][ %s ][-::-]", title)
				} else {
					tabStr = fmt.Sprintf("[white][ %s ][-]", title)
				}
			}
			displayWidth = len(title) + 4 // "[ " + title + " ]"
		} else {
			if data.isPreview {
				tabStr = fmt.Sprintf("[#666666::i]%s[-::-]", title)
			} else {
				tabStr = fmt.Sprintf("[#666666]%s[-]", title)
			}
			displayWidth = len(title)
		}

		if i < len(tabDataList)-1 {
			nextIsActive := tabDataList[i+1].isActive

			if !data.isActive && !nextIsActive {
				tabStr += " [#666666]|[-] "
				displayWidth += 3 // " | "
			} else {
				tabStr += " "
				displayWidth += 1
			}
		}

		allTabs = append(allTabs, tabInfo{text: tabStr, width: displayWidth})
		totalWidth += displayWidth
	}

	if maxWidth <= 0 || totalWidth <= maxWidth {
		var result string
		var clickInfo []TabClickInfo
		currentX := 0

		for i, tab := range allTabs {
			clickInfo = append(clickInfo, TabClickInfo{
				TabIndex: i,
				StartX:   currentX,
				EndX:     currentX + tab.width,
			})
			result += tab.text
			currentX += tab.width
		}
		return result, clickInfo
	}

	var visibleTabs []int
	usedWidth := 0

	activeIdx := -1
	for i, data := range tabDataList {
		if data.isActive {
			activeIdx = i
			break
		}
	}
	if activeIdx >= 0 {
		visibleTabs = append(visibleTabs, activeIdx)
		usedWidth += allTabs[activeIdx].width

		left := activeIdx - 1
		right := activeIdx + 1
		for (left >= 0 || right < len(tabDataList)) && usedWidth < maxWidth {
			addedTab := false

			// Try to add tab on the right
			if right < len(tabDataList) && usedWidth+allTabs[right].width <= maxWidth {
				visibleTabs = append(visibleTabs, right)
				usedWidth += allTabs[right].width
				right++
				addedTab = true
			}

			// Try to add tab on the left
			if left >= 0 && usedWidth+allTabs[left].width <= maxWidth {
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

	var tabText string
	var clickInfo []TabClickInfo
	currentX := 0

	if len(visibleTabs) > 0 && visibleTabs[0] > 0 {
		tabText += "[#666666]◀ [-]"
		currentX += 2
	}

	for _, idx := range visibleTabs {
		clickInfo = append(clickInfo, TabClickInfo{
			TabIndex: idx,
			StartX:   currentX,
			EndX:     currentX + allTabs[idx].width,
		})
		tabText += allTabs[idx].text
		currentX += allTabs[idx].width
	}

	if len(visibleTabs) > 0 && visibleTabs[len(visibleTabs)-1] < len(tabDataList)-1 {
		tabText += " [#666666]▶[-]"
	}

	return tabText, clickInfo
}

// GetNextTabID returns and increments the next available tab ID.
func (m *BaseTabManager) GetNextTabID() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextTabID
	m.nextTabID++
	return id
}
