package ui

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FileManager represents the file tree panel
type FileManager struct {
	tree           *tview.TreeView
	root           *tview.TreeNode
	rootPath       string
	onFileSelected func(path string)
	app            *tview.Application
	
	// Clipboard state
	clipboardPath  string
	clipboardIsCut bool
	
	// Callbacks
	onClipboardChange func(path string, isCut bool)
	onFileRenamed     func(oldPath, newPath string)
	onFileDeleted     func(path string)
}

// NewFileManager creates a new file manager panel
func NewFileManager() *FileManager {
	fm := &FileManager{
		tree: tview.NewTreeView(),
	}
	
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	fm.rootPath = cwd
	
	// Create root node
	fm.root = tview.NewTreeNode(filepath.Base(cwd)).
		SetColor(tcell.ColorYellow).
		SetReference(cwd)
	
	// Set up tree view
	fm.tree.
		SetRoot(fm.root).
		SetCurrentNode(fm.root).
		SetBorder(true).
		//SetTitle(fmt.Sprintf(" %s v%s ", config.AppName, config.Version)).
		SetTitle(" Project ").
		SetTitleAlign(tview.AlignLeft)
	
	// Load directory structure
	fm.loadDirectory(fm.root, cwd)
	
	// Set up selection handler
	fm.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		reference := node.GetReference()
		if reference == nil {
			return
		}
		
		path := reference.(string)
		fileInfo, err := os.Stat(path)
		if err != nil {
			return
		}
		
		if fileInfo.IsDir() {
			// Toggle directory expansion
			if node.IsExpanded() {
				node.SetExpanded(false)
			} else {
				node.SetExpanded(true)
				fm.loadDirectory(node, path)
			}
		} else {
			// File selected - notify listener
			if fm.onFileSelected != nil {
				fm.onFileSelected(path)
			}
		}
	})
	
	return fm
}

// loadDirectory loads the contents of a directory into the tree
func (fm *FileManager) loadDirectory(node *tview.TreeNode, path string) {
	// Clear existing children
	node.ClearChildren()
	
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	
	for _, file := range files {
		// Skip hidden files
		if file.Name()[0] == '.' {
			continue
		}
		
		childPath := filepath.Join(path, file.Name())
		childNode := tview.NewTreeNode(file.Name()).
			SetReference(childPath)
		
		if file.IsDir() {
			childNode.SetColor(tcell.ColorYellow)
			childNode.SetSelectable(true)
			// Explicitly set directories as collapsed by default
			childNode.SetExpanded(false)
		} else {
			childNode.SetColor(tcell.ColorWhite)
		}
		
		node.AddChild(childNode)
	}
}

// refreshDirectory refreshes a directory while preserving expansion state
func (fm *FileManager) refreshDirectory(node *tview.TreeNode, path string) {
	// Save the entire tree's expansion state recursively
	expandedPaths := make(map[string]bool)
	fm.saveTreeExpansion(node, expandedPaths)
	
	// Reload the directory (this clears and recreates all children)
	fm.loadDirectory(node, path)
	
	// Restore the entire tree's expansion state recursively
	fm.restoreTreeExpansion(node, expandedPaths)
}

// saveTreeExpansion recursively saves the expansion state of all directories
func (fm *FileManager) saveTreeExpansion(node *tview.TreeNode, expandedPaths map[string]bool) {
	for _, child := range node.GetChildren() {
		if child.GetReference() != nil {
			childPath := child.GetReference().(string)
			info, err := os.Stat(childPath)
			if err == nil && info.IsDir() {
				if child.IsExpanded() {
					expandedPaths[childPath] = true
					// Recursively save the expansion state of this directory's children
					fm.saveTreeExpansion(child, expandedPaths)
				}
			}
		}
	}
}

// restoreTreeExpansion recursively restores the expansion state of all directories
func (fm *FileManager) restoreTreeExpansion(node *tview.TreeNode, expandedPaths map[string]bool) {
	for _, child := range node.GetChildren() {
		if child.GetReference() != nil {
			childPath := child.GetReference().(string)
			info, err := os.Stat(childPath)
			if err == nil && info.IsDir() {
				if expandedPaths[childPath] {
					// This directory was expanded before
					child.SetExpanded(true)
					// Load its contents
					fm.loadDirectory(child, childPath)
					// Recursively restore its children's expansion state
					fm.restoreTreeExpansion(child, expandedPaths)
				}
				// Otherwise leave it collapsed (which is the default from loadDirectory)
			}
		}
	}
}


// GetView returns the primitive for the file manager
func (fm *FileManager) GetView() tview.Primitive {
	return fm.tree
}

// HasFocus checks if the file manager has focus
func (fm *FileManager) HasFocus(focused tview.Primitive) bool {
	return focused == fm.tree
}

// SetFileSelectedHandler sets the callback for when a file is selected
func (fm *FileManager) SetFileSelectedHandler(handler func(path string)) {
	fm.onFileSelected = handler
}


// ShowSearch shows a fuzzy search dialog
func (fm *FileManager) ShowSearch() {
	// TODO: Implement fuzzy search
	// For now, just a placeholder
}

// RefreshCurrentDirectory refreshes the current directory in the tree
func (fm *FileManager) RefreshCurrentDirectory() {
	node := fm.tree.GetCurrentNode()
	if node == nil {
		return
	}
	
	selectedPath := node.GetReference().(string)
	info, err := os.Stat(selectedPath)
	if err != nil {
		return
	}
	
	// Get the directory node to refresh
	var dirNode *tview.TreeNode
	var dirPath string
	if info.IsDir() {
		dirNode = node
		dirPath = selectedPath
	} else {
		// Need to find parent - for now just return
		return
	}
	
	// Toggle expansion to force refresh
	if dirNode.IsExpanded() {
		dirNode.SetExpanded(false)
		dirNode.SetExpanded(true)
		fm.loadDirectory(dirNode, dirPath)
	}
}

// SetApp sets the application reference
func (fm *FileManager) SetApp(app *tview.Application) {
	fm.app = app
}

// SetClipboardChangeHandler sets the callback for clipboard changes
func (fm *FileManager) SetClipboardChangeHandler(handler func(path string, isCut bool)) {
	fm.onClipboardChange = handler
}

// SetFileRenamedHandler sets the callback for file renames
func (fm *FileManager) SetFileRenamedHandler(handler func(oldPath, newPath string)) {
	fm.onFileRenamed = handler
}

// SetFileDeletedHandler sets the callback for file deletions
func (fm *FileManager) SetFileDeletedHandler(handler func(path string)) {
	fm.onFileDeleted = handler
}

// ClearClipboard clears the clipboard
func (fm *FileManager) ClearClipboard() {
	fm.clipboardPath = ""
	fm.clipboardIsCut = false
	if fm.onClipboardChange != nil {
		fm.onClipboardChange("", false)
	}
}

// CreateNewFile creates a new untitled file in the selected directory
func (fm *FileManager) CreateNewFile() {
	node := fm.tree.GetCurrentNode()
	if node == nil {
		return
	}
	
	selectedPath := node.GetReference().(string)
	info, err := os.Stat(selectedPath)
	if err != nil {
		return
	}
	
	// Get target directory
	var targetDir string
	if info.IsDir() {
		targetDir = selectedPath
	} else {
		targetDir = filepath.Dir(selectedPath)
	}
	
	// Find unique filename and create file
	baseName := "untitled"
	fileName := baseName
	counter := 1
	var fullPath string
	for {
		fullPath = filepath.Join(targetDir, fileName)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// Create the file
			file, err := os.Create(fullPath)
			if err != nil {
				return
			}
			file.Close()
			break
		}
		fileName = fmt.Sprintf("%s%d", baseName, counter)
		counter++
	}
	
	// Schedule the UI update from a goroutine
	if fm.app != nil {
		// Capture the values before the goroutine
		capturedTargetDir := targetDir
		capturedFullPath := fullPath
		
		go func() {
			fm.app.QueueUpdateDraw(func() {
				// Find the target directory node
				targetNode := fm.findNodeByPath(fm.root, capturedTargetDir)
				
				if targetNode != nil {
					// Special case for root node - it's always "expanded"
					// but we still want to preserve child expansion state
					if targetNode == fm.root {
						fm.refreshDirectory(targetNode, capturedTargetDir)
					} else if !targetNode.IsExpanded() {
						// Directory wasn't expanded, expand and load it
						targetNode.SetExpanded(true)
						fm.loadDirectory(targetNode, capturedTargetDir)
					} else {
						// Directory was expanded, refresh while preserving state
						fm.refreshDirectory(targetNode, capturedTargetDir)
					}
					
					// Select the new file - but only set current node, don't trigger expansion
					// The file is in the already loaded directory
					for _, child := range targetNode.GetChildren() {
						if child.GetReference().(string) == capturedFullPath {
							fm.tree.SetCurrentNode(child)
							break
						}
					}
				}
			})
		}()
	}
}

// CopySelected copies the selected file/directory to clipboard
func (fm *FileManager) CopySelected() {
	node := fm.tree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	fm.clipboardPath = node.GetReference().(string)
	fm.clipboardIsCut = false
	
	if fm.onClipboardChange != nil {
		fm.onClipboardChange(fm.clipboardPath, false)
	}
}

// CutSelected cuts the selected file/directory to clipboard
func (fm *FileManager) CutSelected() {
	node := fm.tree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	fm.clipboardPath = node.GetReference().(string)
	fm.clipboardIsCut = true
	
	if fm.onClipboardChange != nil {
		fm.onClipboardChange(fm.clipboardPath, true)
	}
}

// PasteClipboard pastes the clipboard content
func (fm *FileManager) PasteClipboard() {
	if fm.clipboardPath == "" {
		return
	}
	
	node := fm.tree.GetCurrentNode()
	if node == nil {
		return
	}
	
	selectedPath := node.GetReference().(string)
	info, err := os.Stat(selectedPath)
	if err != nil {
		return
	}
	
	// Get target directory
	var targetDir string
	if info.IsDir() {
		targetDir = selectedPath
	} else {
		targetDir = filepath.Dir(selectedPath)
	}
	
	// Store source directory for cut operations
	var sourceDir string
	if fm.clipboardIsCut {
		sourceDir = filepath.Dir(fm.clipboardPath)
	}
	
	// Get source name
	sourceName := filepath.Base(fm.clipboardPath)
	targetPath := filepath.Join(targetDir, sourceName)
	
	// Check if target exists
	if _, err := os.Stat(targetPath); err == nil {
		// File exists, add number suffix
		base := sourceName
		ext := filepath.Ext(sourceName)
		if ext != "" {
			base = sourceName[:len(sourceName)-len(ext)]
		}
		
		counter := 1
		for {
			targetPath = filepath.Join(targetDir, fmt.Sprintf("%s_%d%s", base, counter, ext))
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				break
			}
			counter++
		}
	}
	
	// Perform file operation
	if fm.clipboardIsCut {
		// Move file
		err = os.Rename(fm.clipboardPath, targetPath)
		if err == nil && fm.onFileRenamed != nil {
			fm.onFileRenamed(fm.clipboardPath, targetPath)
		}
	} else {
		// Copy file or directory
		err = fm.copyPath(fm.clipboardPath, targetPath)
	}
	
	if err != nil {
		return
	}
	
	// Clear clipboard
	wasCut := fm.clipboardIsCut
	fm.ClearClipboard()
	
	// Schedule the UI update from a goroutine
	if fm.app != nil {
		// Capture the paths before the goroutine
		capturedTargetDir := targetDir
		capturedTargetPath := targetPath
		capturedSourceDir := sourceDir
		
		go func() {
			fm.app.QueueUpdateDraw(func() {
				// For cut operations, refresh both source and target directories
				if wasCut && capturedSourceDir != "" && capturedSourceDir != capturedTargetDir {
					// Refresh source directory
					sourceNode := fm.findNodeByPath(fm.root, capturedSourceDir)
					if sourceNode != nil && sourceNode.IsExpanded() {
						fm.refreshDirectory(sourceNode, capturedSourceDir)
					}
				}
				
				// Find and refresh target directory
				targetNode := fm.findNodeByPath(fm.root, capturedTargetDir)
				if targetNode != nil {
					// Make sure it's expanded
					if !targetNode.IsExpanded() {
						targetNode.SetExpanded(true)
						// First load since it wasn't expanded
						fm.loadDirectory(targetNode, capturedTargetDir)
					} else {
						// Refresh while preserving expansion state
						fm.refreshDirectory(targetNode, capturedTargetDir)
					}
					
					// Find and select the pasted item
					for _, child := range targetNode.GetChildren() {
						if child.GetReference().(string) == capturedTargetPath {
							fm.tree.SetCurrentNode(child)
							break
						}
					}
				}
			})
		}()
	}
}

// RenameSelected shows rename dialog for selected file/directory
func (fm *FileManager) RenameSelected() {
	node := fm.tree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	oldPath := node.GetReference().(string)
	oldName := filepath.Base(oldPath)
	
	// Create modal configuration
	config := EmModalConfig{
		Width:           50,
		Height:          20,
		BackgroundColor: tcell.ColorDefault,
		BorderColor:     tcell.ColorBlue,
		TitleColor:      tcell.ColorBlue,
		Title:           " Rename ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(fm.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create input field
	inputField := tview.NewInputField().
		SetLabel("New name: ").
		SetText(oldName).
		SetFieldWidth(40)
	
	// Add input field to modal content
	content.
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(inputField, 50, 1, true).
			AddItem(nil, 0, 1, false), 3, 1, true).
		AddItem(nil, 0, 1, false)
	
	// Handle input
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			newName := inputField.GetText()
			if newName != "" && newName != oldName {
				newPath := filepath.Join(filepath.Dir(oldPath), newName)
				
				// Check if already exists
				if _, err := os.Stat(newPath); err != nil {
					// File doesn't exist, safe to rename
					err := os.Rename(oldPath, newPath)
					if err == nil {
						// Notify listeners
						if fm.onFileRenamed != nil {
							fm.onFileRenamed(oldPath, newPath)
						}
						
						// Schedule the UI update from a goroutine
						if fm.app != nil {
							go func() {
								fm.app.QueueUpdateDraw(func() {
									// Find parent node and refresh it
									parentPath := filepath.Dir(newPath)
									parentNode := fm.findNodeByPath(fm.root, parentPath)
									if parentNode != nil {
										// Refresh parent's children while preserving expansion state
										fm.refreshDirectory(parentNode, parentPath)
										// Find and select the renamed item
										for _, child := range parentNode.GetChildren() {
											if child.GetReference().(string) == newPath {
												fm.tree.SetCurrentNode(child)
												break
											}
										}
									}
								})
							}()
						}
					}
				}
			}
			modal.Close()
		} else if key == tcell.KeyEscape {
			modal.Cancel()
		}
	})
	
	modal.Show()
	fm.app.SetFocus(inputField)
}

// DeleteSelected shows delete confirmation for selected file/directory
func (fm *FileManager) DeleteSelected() {
	node := fm.tree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	path := node.GetReference().(string)
	fileName := filepath.Base(path)
	parentPath := filepath.Dir(path)
	
	// Check if it's a directory
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	
	itemType := "file"
	if info.IsDir() {
		itemType = "directory"
	}
	
	// Find the parent node and current index before deletion
	parentNode := fm.findNodeByPath(fm.root, parentPath)
	var currentIndex int = -1
	var siblingCount int = 0
	
	if parentNode != nil {
		children := parentNode.GetChildren()
		siblingCount = len(children)
		for i, child := range children {
			if child.GetReference().(string) == path {
				currentIndex = i
				break
			}
		}
	}
	
	// Show confirmation dialog
	message := fmt.Sprintf("Are you sure you want to delete the %s:\n\n%s\n\nThis action cannot be undone.\n(unless you know about code versioning =))", itemType, fileName)
	ShowConfirmModal(fm.app, "Delete Confirmation", message,
		func() {
			fm.performDelete(path, parentPath, currentIndex, siblingCount)
		},
		nil)
}

// performDelete actually performs the file deletion and UI update
func (fm *FileManager) performDelete(path, parentPath string, currentIndex, siblingCount int) {
	// Delete the file
	err := os.RemoveAll(path)
	if err != nil {
		return
	}
	
	// Notify listeners
	if fm.onFileDeleted != nil {
		fm.onFileDeleted(path)
	}
	
	// Schedule the UI update from a goroutine
	if fm.app != nil {
		// Capture values before the goroutine
		capturedParentPath := parentPath
		capturedIndex := currentIndex
		capturedSiblingCount := siblingCount
		
		go func() {
			fm.app.QueueUpdateDraw(func() {
				// Find parent node again after deletion
				parentNode := fm.findNodeByPath(fm.root, capturedParentPath)
				if parentNode != nil {
					// Refresh parent's children while preserving expansion state
					fm.refreshDirectory(parentNode, capturedParentPath)
					
					// Smart selection logic
					children := parentNode.GetChildren()
					newChildCount := len(children)
					
					if newChildCount == 0 {
						// No children left, select parent
						fm.tree.SetCurrentNode(parentNode)
					} else if capturedIndex >= 0 && capturedSiblingCount > 1 {
						// There were siblings, select the appropriate one
						var nodeToSelect *tview.TreeNode
						
						if capturedIndex < newChildCount {
							// Select the item at the same index (item that moved up)
							nodeToSelect = children[capturedIndex]
						} else if capturedIndex > 0 && newChildCount > 0 {
							// We were at the end, select the new last item
							nodeToSelect = children[newChildCount-1]
						} else {
							// Fallback to first child
							nodeToSelect = children[0]
						}
						
						if nodeToSelect != nil {
							fm.tree.SetCurrentNode(nodeToSelect)
						}
					} else {
						// Fallback: select parent
						fm.tree.SetCurrentNode(parentNode)
					}
				}
			})
		}()
	}
}

// refreshNode refreshes a node's children
func (fm *FileManager) refreshNode(node *tview.TreeNode) {
	if node == nil {
		return
	}
	
	path := node.GetReference().(string)
	fm.loadDirectory(node, path)
	
	// Trigger redraw
	if fm.app != nil {
		fm.app.Draw()
	}
}


// findNodeByPath recursively searches for a node with the given path
func (fm *FileManager) findNodeByPath(node *tview.TreeNode, targetPath string) *tview.TreeNode {
	if node == nil {
		return nil
	}
	
	// Check if this is the target node
	if node.GetReference() != nil && node.GetReference().(string) == targetPath {
		return node
	}
	
	// Only search through already-loaded children
	// Don't load new directories - that's what was causing the expansion
	children := node.GetChildren()
	for _, child := range children {
		if found := fm.findNodeByPath(child, targetPath); found != nil {
			return found
		}
	}
	
	return nil
}

// selectPath recursively searches for and selects a node with the given path
func (fm *FileManager) selectPath(node *tview.TreeNode, targetPath string) bool {
	if node == nil {
		return false
	}
	
	// Check if this is the target node
	if node.GetReference() != nil && node.GetReference().(string) == targetPath {
		fm.tree.SetCurrentNode(node)
		return true
	}
	
	// Search children
	children := node.GetChildren()
	for _, child := range children {
		if fm.selectPath(child, targetPath) {
			return true
		}
	}
	
	return false
}

// expandPath recursively searches for and expands a directory node
func (fm *FileManager) expandPath(node *tview.TreeNode, targetPath string) bool {
	if node == nil {
		return false
	}
	
	// Check if this is the target node
	if node.GetReference() != nil && node.GetReference().(string) == targetPath {
		node.SetExpanded(true)
		// Load its children if not already loaded
		fm.loadDirectory(node, targetPath)
		return true
	}
	
	// Check if target path is a child of this node
	nodePath := ""
	if node.GetReference() != nil {
		nodePath = node.GetReference().(string)
	}
	
	// If the target path starts with this node's path, expand this node and search children
	if nodePath != "" && strings.HasPrefix(targetPath, nodePath+string(filepath.Separator)) {
		node.SetExpanded(true)
		fm.loadDirectory(node, nodePath)
		
		// Search children
		children := node.GetChildren()
		for _, child := range children {
			if fm.expandPath(child, targetPath) {
				return true
			}
		}
	}
	
	// Search children without expanding this node
	children := node.GetChildren()
	for _, child := range children {
		if fm.expandPath(child, targetPath) {
			return true
		}
	}
	
	return false
}

// copyPath recursively copies a file or directory
func (fm *FileManager) copyPath(src, dst string) error {
	// Get source info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	
	// If it's a file, copy it
	if !srcInfo.IsDir() {
		input, err := ioutil.ReadFile(src)
		if err != nil {
			return err
		}
		return ioutil.WriteFile(dst, input, srcInfo.Mode())
	}
	
	// It's a directory, create destination directory
	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return err
	}
	
	// Read directory contents
	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}
	
	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		
		// Recursively copy
		err = fm.copyPath(srcPath, dstPath)
		if err != nil {
			return err
		}
	}
	
	return nil
}