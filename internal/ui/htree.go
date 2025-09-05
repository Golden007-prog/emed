// Package ui provides user interface components and managers for the emed editor.
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

// HTreeNode represents a node in the hierarchical tree structure.
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

// naturalLess compares two strings using natural sorting (numbers are sorted numerically).
func naturalLess(a, b string) bool {
	aRunes := []rune(a)
	bRunes := []rune(b)
	aLen := len(aRunes)
	bLen := len(bRunes)

	i, j := 0, 0
	for i < aLen && j < bLen {
		if unicode.IsDigit(aRunes[i]) && unicode.IsDigit(bRunes[j]) {
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

			intA, _ := strconv.Atoi(numA)
			intB, _ := strconv.Atoi(numB)

			if intA != intB {
				return intA < intB
			}
		} else {
			if unicode.ToLower(aRunes[i]) != unicode.ToLower(bRunes[j]) {
				return unicode.ToLower(aRunes[i]) < unicode.ToLower(bRunes[j])
			}
			i++
			j++
		}
	}

	return aLen < bLen
}

// hasCodeExtension checks if a filename has a code-related extension.
func hasCodeExtension(filename string) bool {
	extensions := []string{
		".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rb", ".java", ".c", ".cpp", ".h", ".hpp",
		".rs", ".sh", ".bash", ".zsh", ".fish", ".php", ".html", ".css", ".scss", ".sass",
		".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf", ".config",
		".md", ".markdown", ".rst", ".txt", ".vim", ".lua", ".sql", ".r", ".R",
	}

	for _, ext := range extensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			return true
		}
	}

	return false
}

// sortNodes sorts tree nodes with directories first, then by natural order.
func sortNodes(nodes []*HTreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		nodeA := nodes[i]
		nodeB := nodes[j]

		aIsDotDir := nodeA.IsDir && strings.HasPrefix(nodeA.Text, ".")
		bIsDotDir := nodeB.IsDir && strings.HasPrefix(nodeB.Text, ".")
		aIsNormalDir := nodeA.IsDir && !strings.HasPrefix(nodeA.Text, ".")
		bIsNormalDir := nodeB.IsDir && !strings.HasPrefix(nodeB.Text, ".")
		aIsDotFile := !nodeA.IsDir && strings.HasPrefix(nodeA.Text, ".")
		bIsDotFile := !nodeB.IsDir && strings.HasPrefix(nodeB.Text, ".")

		aCat := 4
		if aIsDotDir {
			aCat = 1
		} else if aIsNormalDir {
			aCat = 2
		} else if aIsDotFile {
			aCat = 3
		}

		bCat := 4
		if bIsDotDir {
			bCat = 1
		} else if bIsNormalDir {
			bCat = 2
		} else if bIsDotFile {
			bCat = 3
		}

		if aCat != bCat {
			return aCat < bCat
		}

		return naturalLess(nodeA.Text, nodeB.Text)
	})
}

// HTree is a hierarchical tree widget for displaying file system structures.
type HTree struct {
	*tview.Box

	root         *HTreeNode
	selected     *HTreeNode
	visibleNodes []*HTreeNode

	scrollY int
	scrollX int

	onSelected        func(node *HTreeNode)
	onInfo            func(node *HTreeNode)
	onCreateFile      func(node *HTreeNode)
	onCreateDirectory func(node *HTreeNode)
	onCopy            func(node *HTreeNode)
	onCut             func(node *HTreeNode)
	onPaste           func(node *HTreeNode)
	onDelete          func(node *HTreeNode)
	onRename          func(node *HTreeNode)
	hasFocus          bool
	app               *tview.Application
}

// NewHTree creates a new hierarchical tree widget.
func NewHTree() *HTree {
	return &HTree{
		Box:     tview.NewBox(),
		scrollY: 0,
		scrollX: 0,
	}
}

// SetRoot sets the root node for the tree.
func (t *HTree) SetRoot(node *HTreeNode) *HTree {
	t.root = node
	t.selected = node
	t.buildVisibleNodes()
	return t
}

func (t *HTree) buildVisibleNodes() {
	t.visibleNodes = nil
	if t.root != nil {
		t.addVisibleNode(t.root, 0)
	}
}

func (t *HTree) addVisibleNode(node *HTreeNode, depth int) {
	node.depth = depth
	t.visibleNodes = append(t.visibleNodes, node)

	if node.IsExpanded {
		for _, child := range node.Children {
			t.addVisibleNode(child, depth+1)
		}
	}
}

// Draw renders the tree to the screen.
func (t *HTree) Draw(screen tcell.Screen) {
	t.Box.DrawForSubclass(screen, t)

	x, y, width, height := t.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	t.buildVisibleNodes()

	if t.selected != nil {

		if t.selected.depth <= 3 {
			t.scrollX = 0
		} else {

			minVisibleDepth := t.selected.depth - 2
			if minVisibleDepth < 0 {
				minVisibleDepth = 0
			}

			targetScrollX := minVisibleDepth * 2

			diff := targetScrollX - t.scrollX
			if diff > 10 || diff < -10 {
				t.scrollX = targetScrollX
			} else if diff > 0 {
				t.scrollX += 2
			} else if diff < 0 && t.scrollX > 0 {
				t.scrollX -= 2
			}
		}

		if t.scrollX < 0 {
			t.scrollX = 0
		}
	}

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

	for i := 0; i < height && t.scrollY+i < len(t.visibleNodes); i++ {
		node := t.visibleNodes[t.scrollY+i]

		var line strings.Builder

		indent := node.depth * 2
		for j := 0; j < indent; j++ {
			line.WriteRune(' ')
		}

		if node.IsDir {
			if node.IsExpanded {
				line.WriteString("▼ ")
			} else {
				line.WriteString("▶ ")
			}
		} else if node.depth > 0 {
			line.WriteString("  ")
		}

		line.WriteString(node.Text)

		fullLine := line.String()

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

		drawX := 0
		for j, ch := range fullLine {
			if j < t.scrollX {
				continue
			}

			if drawX >= width {
				break
			}

			screen.SetContent(x+drawX, y+i, ch, nil, tcell.StyleDefault.Foreground(fgColor).Background(bgColor))
			drawX++
		}

		if node == t.selected {
			for ; drawX < width; drawX++ {
				screen.SetContent(x+drawX, y+i, ' ', nil, tcell.StyleDefault.Background(bgColor))
			}
		}
	}

}

func (t *HTree) getRelativePath(node *HTreeNode) string {
	if node == nil || node == t.root {
		return "/"
	}

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

// GetRelativePath returns the relative path of a node from the root.
func (t *HTree) GetRelativePath(node *HTreeNode) string {
	return t.getRelativePath(node)
}

// InputHandler returns the input handler for keyboard events.
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
		case tcell.KeyCtrlN:
			if event.Modifiers() == (tcell.ModCtrl | tcell.ModAlt) {
				if t.onCreateDirectory != nil && t.selected != nil {
					t.onCreateDirectory(t.selected)
				}
			} else if event.Modifiers() == tcell.ModCtrl {
				if t.onCreateFile != nil && t.selected != nil {
					t.onCreateFile(t.selected)
				}
			}
		case tcell.KeyCtrlC:
			if event.Modifiers() == tcell.ModCtrl {
				if t.onCopy != nil && t.selected != nil {
					t.onCopy(t.selected)
				}
			}
		case tcell.KeyCtrlX:
			if event.Modifiers() == tcell.ModCtrl {
				if t.onCut != nil && t.selected != nil {
					t.onCut(t.selected)
				}
			}
		case tcell.KeyCtrlV:
			if event.Modifiers() == tcell.ModCtrl {
				if t.onPaste != nil && t.selected != nil {
					t.onPaste(t.selected)
				}
			}
		case tcell.KeyCtrlD:
			if event.Modifiers() == tcell.ModCtrl {
				if t.onDelete != nil && t.selected != nil {
					t.onDelete(t.selected)
				}
			}
		case tcell.KeyCtrlR:
			if event.Modifiers() == tcell.ModCtrl {
				if t.onRename != nil && t.selected != nil {
					t.onRename(t.selected)
				}
			}
		case tcell.KeyRune:
			if event.Modifiers() == tcell.ModCtrl && event.Rune() == 'c' {
				if t.onCopy != nil && t.selected != nil {
					t.onCopy(t.selected)
				}
			} else {
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
		}
	})
}

func (t *HTree) moveUp() {
	t.buildVisibleNodes()
	for i, node := range t.visibleNodes {
		if node == t.selected && i > 0 {
			t.selected = t.visibleNodes[i-1]
			break
		}
	}
}

func (t *HTree) moveDown() {
	t.buildVisibleNodes()
	for i, node := range t.visibleNodes {
		if node == t.selected && i < len(t.visibleNodes)-1 {
			t.selected = t.visibleNodes[i+1]
			break
		}
	}
}

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

		if file.Name()[0] == '.' {
			child.Color = tcell.NewRGBColor(80, 80, 80)
		} else if file.IsDir() {
			child.Color = tcell.ColorBlue
		} else if hasCodeExtension(file.Name()) {
			child.Color = tcell.ColorLightBlue
		} else {
			child.Color = tcell.ColorSilver
		}

		node.Children = append(node.Children, child)
	}

	sortNodes(node.Children)
}

// Focus sets focus to the tree.
func (t *HTree) Focus(delegate func(p tview.Primitive)) {
	t.hasFocus = true
	if t.Box.Focus != nil {
		t.Box.Focus(delegate)
	}
}

// Blur removes focus from the tree.
func (t *HTree) Blur() {
	t.hasFocus = false
	if t.Box.Blur != nil {
		t.Box.Blur()
	}
}

// HasFocus returns whether the tree has focus.
func (t *HTree) HasFocus() bool {
	return t.hasFocus
}

// SetSelectedFunc sets the callback for node selection.
func (t *HTree) SetSelectedFunc(fn func(node *HTreeNode)) *HTree {
	t.onSelected = fn
	return t
}

// GetCurrentNode returns the currently selected node.
func (t *HTree) GetCurrentNode() *HTreeNode {
	return t.selected
}

// SetCurrentNode sets the currently selected node.
func (t *HTree) SetCurrentNode(node *HTreeNode) *HTree {
	t.selected = node
	return t
}

// FindNodeByPath finds a node by its file system path.
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

// ExpandPath expands all nodes along the given path.
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

		if current.IsDir && !current.IsExpanded {
			current.IsExpanded = true
			t.loadChildren(current)
		}

		for _, child := range current.Children {
			if strings.HasPrefix(child.Path, partialPath) {
				current = child
				break
			}
		}
	}
}

// MouseHandler returns the mouse handler for mouse events.
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

			if relY >= 0 && relY < height {
				index := t.scrollY + relY
				if index < len(t.visibleNodes) {
					clickedNode := t.visibleNodes[index]

					relX := mx - x
					nodeX := clickedNode.depth * 2

					if clickedNode.IsDir && relX >= nodeX-t.scrollX && relX <= nodeX+2-t.scrollX {
						clickedNode.IsExpanded = !clickedNode.IsExpanded
						if clickedNode.IsExpanded {
							t.loadChildren(clickedNode)
						}
					} else {
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

// RefreshNode refreshes a directory node by reloading its children.
func (t *HTree) RefreshNode(node *HTreeNode) {
	if node == nil || !node.IsDir {
		return
	}

	var selectedPath string
	if t.selected != nil {
		selectedPath = t.selected.Path
	}

	expandedPaths := make(map[string]bool)
	t.saveExpansionState(node, expandedPaths)

	node.Children = nil
	t.loadChildren(node)

	t.restoreExpansionState(node, expandedPaths)

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

// SetCreateFileFunc sets the callback for creating new file
func (t *HTree) SetCreateFileFunc(fn func(node *HTreeNode)) *HTree {
	t.onCreateFile = fn
	return t
}

// SetCreateDirectoryFunc sets the callback for creating new directory
func (t *HTree) SetCreateDirectoryFunc(fn func(node *HTreeNode)) *HTree {
	t.onCreateDirectory = fn
	return t
}

// SetCopyFunc sets the callback for copy
func (t *HTree) SetCopyFunc(fn func(node *HTreeNode)) *HTree {
	t.onCopy = fn
	return t
}

// SetCutFunc sets the callback for cut
func (t *HTree) SetCutFunc(fn func(node *HTreeNode)) *HTree {
	t.onCut = fn
	return t
}

// SetPasteFunc sets the callback for paste
func (t *HTree) SetPasteFunc(fn func(node *HTreeNode)) *HTree {
	t.onPaste = fn
	return t
}

// SetDeleteFunc sets the callback for delete
func (t *HTree) SetDeleteFunc(fn func(node *HTreeNode)) *HTree {
	t.onDelete = fn
	return t
}

// SetRenameFunc sets the callback for rename
func (t *HTree) SetRenameFunc(fn func(node *HTreeNode)) *HTree {
	t.onRename = fn
	return t
}
