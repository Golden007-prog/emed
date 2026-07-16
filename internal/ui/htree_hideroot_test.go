package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func newHiddenRootTree() (*HTree, *HTreeNode) {
	root := &HTreeNode{Text: "proj", Path: "/proj", IsDir: true, IsExpanded: true}
	childA := &HTreeNode{Text: "a.txt", Path: "/proj/a.txt", Parent: root}
	childB := &HTreeNode{Text: "sub", Path: "/proj/sub", IsDir: true, Parent: root}
	root.Children = []*HTreeNode{childA, childB}

	tree := NewHTree()
	tree.SetRoot(root).SetHideRoot(true)
	return tree, root
}

func TestHideRootExcludesRootFromVisibleNodes(t *testing.T) {
	tree, root := newHiddenRootTree()

	tree.buildVisibleNodes()

	if len(tree.visibleNodes) != 2 {
		t.Fatalf("expected 2 visible nodes, got %d", len(tree.visibleNodes))
	}
	for _, node := range tree.visibleNodes {
		if node == root {
			t.Fatal("root node must not be visible")
		}
	}
	if tree.visibleNodes[0].depth != 0 {
		t.Errorf("top level children must have depth 0, got %d", tree.visibleNodes[0].depth)
	}
	if tree.selected != root.Children[0] {
		t.Errorf("selection should move from hidden root to first child")
	}
}

func TestHideRootLeftKeyDoesNotSelectRoot(t *testing.T) {
	tree, root := newHiddenRootTree()
	tree.buildVisibleNodes()
	tree.SetCurrentNode(root.Children[0])

	handler := tree.InputHandler()
	handler(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone), func(p tview.Primitive) {})

	if tree.GetCurrentNode() == root {
		t.Fatal("left key must not move selection to the hidden root")
	}
}
