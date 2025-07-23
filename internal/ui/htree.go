package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HTreeNode represents a node in the horizontal scrolling tree
type HTreeNode struct {
	Text       string
	Path       string
	IsDir      bool
	IsExpanded bool
	Children   []*HTreeNode
	Parent     *HTreeNode
	Color      tcell.Color
	depth      int
}

// naturalLess compares two strings using natural order sorting
// (e.g., "file2" comes before "file10")
func naturalLess(a, b string) bool {
	aRunes := []rune(a)
	bRunes := []rune(b)
	aLen := len(aRunes)
	bLen := len(bRunes)
	
	i, j := 0, 0
	for i < aLen && j < bLen {
		// Check if we're at the start of a number in both strings
		if unicode.IsDigit(aRunes[i]) && unicode.IsDigit(bRunes[j]) {
			// Extract the numbers
			numA := ""
			for i < aLen && unicode.IsDigit(aRunes[i]) {
				numA += string(aRunes[i])
				i++
			}
			
			numB := ""
			for j < bLen && unicode.IsDigit(bRunes[j]) {
				numB += string(bRunes[j])
				j++
			}
			
			// Compare numerically
			intA, _ := strconv.Atoi(numA)
			intB, _ := strconv.Atoi(numB)
			
			if intA != intB {
				return intA < intB
			}
		} else {
			// Compare character by character (case-insensitive)
			if unicode.ToLower(aRunes[i]) != unicode.ToLower(bRunes[j]) {
				return unicode.ToLower(aRunes[i]) < unicode.ToLower(bRunes[j])
			}
			i++
			j++
		}
	}
	
	return aLen < bLen
}

// sortNodes sorts nodes according to the specified order:
// 1. dot directories, 2. normal directories, 3. dot files, 4. normal files
// Each category is sorted with natural order
func sortNodes(nodes []*HTreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		nodeA := nodes[i]
		nodeB := nodes[j]
		
		// Determine categories
		aIsDotDir := nodeA.IsDir && strings.HasPrefix(nodeA.Text, ".")
		bIsDotDir := nodeB.IsDir && strings.HasPrefix(nodeB.Text, ".")
		aIsNormalDir := nodeA.IsDir && !strings.HasPrefix(nodeA.Text, ".")
		bIsNormalDir := nodeB.IsDir && !strings.HasPrefix(nodeB.Text, ".")
		aIsDotFile := !nodeA.IsDir && strings.HasPrefix(nodeA.Text, ".")
		bIsDotFile := !nodeB.IsDir && strings.HasPrefix(nodeB.Text, ".")
		
		// Category priorities
		aCat := 4 // normal file
		if aIsDotDir {
			aCat = 1
		} else if aIsNormalDir {
			aCat = 2
		} else if aIsDotFile {
			aCat = 3
		}
		
		bCat := 4 // normal file
		if bIsDotDir {
			bCat = 1
		} else if bIsNormalDir {
			bCat = 2
		} else if bIsDotFile {
			bCat = 3
		}
		
		// Compare categories first
		if aCat != bCat {
			return aCat < bCat
		}
		
		// Within same category, use natural order
		return naturalLess(nodeA.Text, nodeB.Text)
	})
}

// HTree is a custom tree widget with horizontal scrolling
type HTree struct {
	*tview.Box
	
	root         *HTreeNode
	selected     *HTreeNode
	visibleNodes []*HTreeNode
	
	scrollY      int  // Vertical scroll offset
	scrollX      int  // Horizontal scroll offset
	
	onSelected   func(node *HTreeNode)
	onInfo       func(node *HTreeNode)  // Callback for info modal
	hasFocus     bool
	app          *tview.Application
}

// NewHTree creates a new horizontal scrolling tree
func NewHTree() *HTree {
	return &HTree{
		Box:      tview.NewBox(),
		scrollY:  0,
		scrollX:  0,
	}
}

// SetRoot sets the root node
func (t *HTree) SetRoot(node *HTreeNode) *HTree {
	t.root = node
	t.selected = node
	t.buildVisibleNodes()
	return t
}

// buildVisibleNodes rebuilds the list of visible nodes based on expansion state
func (t *HTree) buildVisibleNodes() {
	t.visibleNodes = nil
	if t.root != nil {
		t.addVisibleNode(t.root, 0)
	}
}

// addVisibleNode recursively adds visible nodes
func (t *HTree) addVisibleNode(node *HTreeNode, depth int) {
	node.depth = depth
	t.visibleNodes = append(t.visibleNodes, node)
	
	if node.IsExpanded {
		for _, child := range node.Children {
			t.addVisibleNode(child, depth+1)
		}
	}
}

// Draw draws the tree with horizontal scrolling
func (t *HTree) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)
	
	x, y, width, height := t.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}
	
	// Reserve bottom line for status bar
	statusY := y + height - 1
	height = height - 1  // Reduce height for tree content
	
	// Rebuild visible nodes
	t.buildVisibleNodes()
	
	// Auto-scroll horizontally based on depth level
	if t.selected != nil {
		// Calculate desired scroll based on depth
		// Keep nodes at depth <= 3 fully visible (no scroll)
		// For deeper nodes, scroll to show them with reasonable indentation visible
		
		if t.selected.depth <= 3 {
			// For shallow nodes, reset scroll to 0
			t.scrollX = 0
		} else {
			// For deeper nodes, calculate scroll to keep a comfortable view
			// We want to show at least 2 levels of indentation before the current node
			// This gives context while maximizing space for the filename
			
			minVisibleDepth := t.selected.depth - 2
			if minVisibleDepth < 0 {
				minVisibleDepth = 0
			}
			
			// Calculate the X position where we want to start showing
			targetScrollX := minVisibleDepth * 2
			
			// Smooth scrolling: only adjust if we're too far off
			diff := targetScrollX - t.scrollX
			if diff > 10 || diff < -10 {
				// Big jump in depth, snap to target
				t.scrollX = targetScrollX
			} else if diff > 0 {
				// Gradually scroll right
				t.scrollX += 2
			} else if diff < 0 && t.scrollX > 0 {
				// Gradually scroll left
				t.scrollX -= 2
			}
		}
		
		// Ensure scrollX is not negative
		if t.scrollX < 0 {
			t.scrollX = 0
		}
	}
	
	// Adjust vertical scroll to keep selected item visible
	selectedIndex := -1
	for i, node := range t.visibleNodes {
		if node == t.selected {
			selectedIndex = i
			break
		}
	}
	
	if selectedIndex >= 0 {
		if selectedIndex < t.scrollY {
			t.scrollY = selectedIndex
		} else if selectedIndex >= t.scrollY+height {
			t.scrollY = selectedIndex - height + 1
		}
	}
	
	// Draw visible nodes
	for i := 0; i < height && t.scrollY+i < len(t.visibleNodes); i++ {
		node := t.visibleNodes[t.scrollY+i]
		
		// Build the line to render
		var line strings.Builder
		
		// Add indentation
		indent := node.depth * 2
		for j := 0; j < indent; j++ {
			line.WriteRune(' ')
		}
		
		// Add tree graphics
		if node.IsDir {
			if node.IsExpanded {
				line.WriteString("▼ ")
			} else {
				line.WriteString("▶ ")
			}
		} else if node.depth > 0 {
			line.WriteString("  ")
		}
		
		// Add text
		line.WriteString(node.Text)
		
		// Get the full line
		fullLine := line.String()
		
		// Determine colors
		fgColor := node.Color
		bgColor := tcell.ColorBlack
		
		if node == t.selected {
			if t.hasFocus {
				fgColor = tcell.ColorBlack
				bgColor = tcell.ColorWhite
			} else {
				fgColor = tcell.ColorWhite
				bgColor = tcell.NewRGBColor(60, 60, 60)
			}
		}
		
		// Draw the line with horizontal scrolling
		drawX := 0
		for j, ch := range fullLine {
			// Skip characters before scroll position
			if j < t.scrollX {
				continue
			}
			
			// Stop if we've reached the edge of the widget
			if drawX >= width {
				break
			}
			
			// Draw the character
			screen.SetContent(x+drawX, y+i, ch, nil, tcell.StyleDefault.Foreground(fgColor).Background(bgColor))
			drawX++
		}
		
		// Fill the rest of the line if selected
		if node == t.selected {
			for ; drawX < width; drawX++ {
				screen.SetContent(x+drawX, y+i, ' ', nil, tcell.StyleDefault.Background(bgColor))
			}
		}
	}
	
	// Draw status bar with help message
	if t.hasFocus {
		helpText := "Press i for file actions"
		tview.Print(screen, helpText, x+(width-len(helpText))/2, statusY, width, tview.AlignLeft, tcell.ColorGray)
	}
}

// getRelativePath builds the relative path from root to node
func (t *HTree) getRelativePath(node *HTreeNode) string {
	if node == nil || node == t.root {
		return "/"
	}
	
	// Build path components from node to root
	var parts []string
	current := node
	for current != nil && current != t.root {
		parts = append([]string{current.Text}, parts...)
		current = current.Parent
	}
	
	if len(parts) == 0 {
		return "/"
	}
	
	return filepath.Join(parts...)
}

// GetRelativePath exports the relative path for a node
func (t *HTree) GetRelativePath(node *HTreeNode) string {
	return t.getRelativePath(node)
}

// InputHandler handles keyboard input
func (t *HTree) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return t.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if t.selected == nil {
			return
		}
		
		switch event.Key() {
		case tcell.KeyUp:
			t.moveUp()
		case tcell.KeyDown:
			t.moveDown()
		case tcell.KeyRight:
			if t.selected.IsDir && !t.selected.IsExpanded {
				t.selected.IsExpanded = true
				t.loadChildren(t.selected)
			}
		case tcell.KeyLeft:
			if t.selected.IsDir && t.selected.IsExpanded {
				t.selected.IsExpanded = false
			} else if t.selected.Parent != nil {
				t.selected = t.selected.Parent
			}
		case tcell.KeyEnter:
			if t.selected.IsDir {
				t.selected.IsExpanded = !t.selected.IsExpanded
				if t.selected.IsExpanded {
					t.loadChildren(t.selected)
				}
			} else if t.onSelected != nil {
				t.onSelected(t.selected)
			}
		case tcell.KeyRune:
			switch event.Rune() {
			case ' ':
				if t.selected.IsDir {
					t.selected.IsExpanded = !t.selected.IsExpanded
					if t.selected.IsExpanded {
						t.loadChildren(t.selected)
					}
				}
			case 'i', 'I':
				if t.onInfo != nil && t.selected != nil {
					t.onInfo(t.selected)
				}
			}
		}
	})
}

// moveUp moves selection up
func (t *HTree) moveUp() {
	t.buildVisibleNodes()
	for i, node := range t.visibleNodes {
		if node == t.selected && i > 0 {
			t.selected = t.visibleNodes[i-1]
			break
		}
	}
}

// moveDown moves selection down
func (t *HTree) moveDown() {
	t.buildVisibleNodes()
	for i, node := range t.visibleNodes {
		if node == t.selected && i < len(t.visibleNodes)-1 {
			t.selected = t.visibleNodes[i+1]
			break
		}
	}
}

// loadChildren loads children for a directory node
func (t *HTree) loadChildren(node *HTreeNode) {
	if !node.IsDir || len(node.Children) > 0 {
		return
	}
	
	files, err := os.ReadDir(node.Path)
	if err != nil {
		return
	}
	
	for _, file := range files {
		childPath := filepath.Join(node.Path, file.Name())
		child := &HTreeNode{
			Text:   file.Name(),
			Path:   childPath,
			IsDir:  file.IsDir(),
			Parent: node,
		}
		
		// Set colors
		if file.Name()[0] == '.' {
			child.Color = tcell.NewRGBColor(80, 80, 80)
		} else if file.IsDir() {
			child.Color = tcell.ColorBlue
		} else if isCodeFile(file.Name()) {
			child.Color = tcell.ColorSkyblue
		} else {
			child.Color = tcell.ColorSilver
		}
		
		node.Children = append(node.Children, child)
	}
	
	// Sort children according to our custom order
	sortNodes(node.Children)
}

// Focus is called when the primitive receives focus
func (t *HTree) Focus(delegate func(p tview.Primitive)) {
	t.hasFocus = true
	if t.Box.Focus != nil {
		t.Box.Focus(delegate)
	}
}

// Blur is called when the primitive loses focus
func (t *HTree) Blur() {
	t.hasFocus = false
	if t.Box.Blur != nil {
		t.Box.Blur()
	}
}

// HasFocus returns whether the primitive has focus
func (t *HTree) HasFocus() bool {
	return t.hasFocus
}

// SetSelectedFunc sets the callback for when a node is selected
func (t *HTree) SetSelectedFunc(fn func(node *HTreeNode)) *HTree {
	t.onSelected = fn
	return t
}

// GetCurrentNode returns the currently selected node
func (t *HTree) GetCurrentNode() *HTreeNode {
	return t.selected
}

// SetCurrentNode sets the currently selected node
func (t *HTree) SetCurrentNode(node *HTreeNode) *HTree {
	t.selected = node
	return t
}

// FindNodeByPath finds a node by its path
func (t *HTree) FindNodeByPath(path string) *HTreeNode {
	if t.root == nil {
		return nil
	}
	return t.findNodeByPathRecursive(t.root, path)
}

func (t *HTree) findNodeByPathRecursive(node *HTreeNode, path string) *HTreeNode {
	if node.Path == path {
		return node
	}
	for _, child := range node.Children {
		if found := t.findNodeByPathRecursive(child, path); found != nil {
			return found
		}
	}
	return nil
}

// ExpandPath expands all nodes to show the given path
func (t *HTree) ExpandPath(path string) {
	parts := strings.Split(path, string(filepath.Separator))
	current := t.root
	
	for i := range parts {
		if current == nil {
			break
		}
		
		partialPath := filepath.Join(parts[:i+1]...)
		if partialPath == "" {
			partialPath = "/"
		}
		
		// Expand current node
		if current.IsDir && !current.IsExpanded {
			current.IsExpanded = true
			t.loadChildren(current)
		}
		
		// Find child with matching path
		for _, child := range current.Children {
			if strings.HasPrefix(child.Path, partialPath) {
				current = child
				break
			}
		}
	}
}

// MouseHandler handles mouse input
func (t *HTree) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return t.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !t.InRect(event.Position()) {
			return false, nil
		}
		
		x, y, _, height := t.GetInnerRect()
		mx, my := event.Position()
		relY := my - y
		
		switch action {
		case tview.MouseLeftClick:
			setFocus(t)
			
			// Find clicked node
			if relY >= 0 && relY < height {
				index := t.scrollY + relY
				if index < len(t.visibleNodes) {
					clickedNode := t.visibleNodes[index]
					
					// Check if click was on the expand/collapse icon
					relX := mx - x
					nodeX := clickedNode.depth * 2
					
					if clickedNode.IsDir && relX >= nodeX-t.scrollX && relX <= nodeX+2-t.scrollX {
						// Toggle expansion
						clickedNode.IsExpanded = !clickedNode.IsExpanded
						if clickedNode.IsExpanded {
							t.loadChildren(clickedNode)
						}
					} else {
						// Select the node
						t.selected = clickedNode
						if !clickedNode.IsDir && t.onSelected != nil {
							t.onSelected(clickedNode)
						}
					}
				}
			}
			return true, nil
			
		case tview.MouseScrollUp:
			if t.scrollY > 0 {
				t.scrollY--
			}
			return true, nil
			
		case tview.MouseScrollDown:
			if t.scrollY < len(t.visibleNodes)-height {
				t.scrollY++
			}
			return true, nil
			
		case tview.MouseScrollLeft:
			if t.scrollX > 0 {
				t.scrollX -= 3
				if t.scrollX < 0 {
					t.scrollX = 0
				}
			}
			return true, nil
			
		case tview.MouseScrollRight:
			t.scrollX += 3
			return true, nil
		}
		
		return false, nil
	})
}

// RefreshNode refreshes a node and its children
func (t *HTree) RefreshNode(node *HTreeNode) {
	if node == nil || !node.IsDir {
		return
	}
	
	// Save current selection path
	var selectedPath string
	if t.selected != nil {
		selectedPath = t.selected.Path
	}
	
	// Save expansion state of children
	expandedPaths := make(map[string]bool)
	t.saveExpansionState(node, expandedPaths)
	
	// Reload children
	node.Children = nil
	t.loadChildren(node)
	
	// Restore expansion state
	t.restoreExpansionState(node, expandedPaths)
	
	// Restore selection if it still exists
	if selectedPath != "" {
		if selectedNode := t.FindNodeByPath(selectedPath); selectedNode != nil {
			t.selected = selectedNode
		}
	}
}

func (t *HTree) saveExpansionState(node *HTreeNode, expanded map[string]bool) {
	for _, child := range node.Children {
		if child.IsDir && child.IsExpanded {
			expanded[child.Path] = true
			t.saveExpansionState(child, expanded)
		}
	}
}

func (t *HTree) restoreExpansionState(node *HTreeNode, expanded map[string]bool) {
	for _, child := range node.Children {
		if child.IsDir && expanded[child.Path] {
			child.IsExpanded = true
			t.loadChildren(child)
			t.restoreExpansionState(child, expanded)
		}
	}
}

// SetApp sets the application reference
func (t *HTree) SetApp(app *tview.Application) *HTree {
	t.app = app
	return t
}

// SetInfoFunc sets the callback for info modal
func (t *HTree) SetInfoFunc(fn func(node *HTreeNode)) *HTree {
	t.onInfo = fn
	return t
}