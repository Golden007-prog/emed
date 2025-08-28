package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/filemanager/operations"
	"github.com/laborin/emed/internal/filemanager/clipboard"
	"github.com/laborin/emed/internal/logging"
	"github.com/rivo/tview"
)

// FileManager represents the file tree panel using new components
type FileManager struct {
	htree          *HTree
	root           *HTreeNode
	rootPath       string
	onFileSelected func(path string)
	app            *tview.Application
	
	// New components
	fileOps        *operations.FileOperations
	clipboard      *clipboard.Clipboard
	
	// Callbacks
	onClipboardChange func(path string, isCut bool)
	onFileRenamed     func(oldPath, newPath string)
	onFileDeleted     func(path string)
}

// NewFileManager creates a new file manager panel using new components
func NewFileManager() *FileManager {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	fm := &FileManager{
		htree:    NewHTree(),
		rootPath: cwd,
		fileOps:  operations.New(cwd),
		clipboard: clipboard.New(),
	}

	// Initialize tree manager (but we'll use it differently due to interface mismatch)
	// We can't use the tree manager directly because HTreeNode doesn't have the expected interface
	// Instead we'll create our own root node and manage it directly
	
	// Create root node using existing structure
	fm.root = &HTreeNode{
		Text:       filepath.Base(cwd),
		Path:       cwd,
		IsDir:      true,
		IsExpanded: true,
		Color:      tcell.ColorBlue,
	}

	// Set up tree view
	fm.htree.
		SetRoot(fm.root).
		SetCurrentNode(fm.root).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Project ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)

	// Load initial directory structure
	fm.loadInitialDirectory()

	// Set up selection handler
	fm.htree.SetSelectedFunc(func(node *HTreeNode) {
		if !node.IsDir && fm.onFileSelected != nil {
			fm.onFileSelected(node.Path)
		}
	})

	// Set up info handler
	fm.htree.SetInfoFunc(func(node *HTreeNode) {
		fm.showInfoModal(node)
	})

	return fm
}

// loadInitialDirectory loads the initial directory structure with proper colors
func (fm *FileManager) loadInitialDirectory() {
	files, err := os.ReadDir(fm.root.Path)
	if err != nil {
		return
	}
	
	for _, file := range files {
		childPath := filepath.Join(fm.root.Path, file.Name())
		child := &HTreeNode{
			Text:   file.Name(),
			Path:   childPath,
			IsDir:  file.IsDir(),
			Parent: fm.root,
		}
		
		// Set colors
		if file.Name()[0] == '.' {
			child.Color = tcell.NewRGBColor(80, 80, 80)
		} else if file.IsDir() {
			child.Color = tcell.ColorBlue
		} else if fm.isCodeFile(file.Name()) {
			child.Color = tcell.ColorLightBlue
		} else {
			child.Color = tcell.ColorSilver
		}
		
		fm.root.Children = append(fm.root.Children, child)
	}
	
	// Sort the children
	sortNodes(fm.root.Children)
}

// isCodeFile checks if a file has a code extension
func (fm *FileManager) isCodeFile(filename string) bool {
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

// GetView returns the primitive for the file manager
func (fm *FileManager) GetView() tview.Primitive {
	return fm.htree
}

// HasFocus checks if the file manager has focus
func (fm *FileManager) HasFocus(focused tview.Primitive) bool {
	return focused == fm.htree
}

// SetFileSelectedHandler sets the callback for when a file is selected
func (fm *FileManager) SetFileSelectedHandler(handler func(path string)) {
	fm.onFileSelected = handler
}

// SetApp sets the application reference
func (fm *FileManager) SetApp(app *tview.Application) {
	fm.app = app
	fm.htree.SetApp(app)
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

// ShowSearch shows a fuzzy search dialog
func (fm *FileManager) ShowSearch() {
	// TODO: Implement fuzzy search
	// For now, just a placeholder
}

// RefreshCurrentDirectory refreshes the current directory in the tree
func (fm *FileManager) RefreshCurrentDirectory() {
	node := fm.htree.GetCurrentNode()
	if node == nil {
		return
	}
	
	// Get the directory node to refresh
	var dirNode *HTreeNode
	if node.IsDir {
		dirNode = node
	} else if node.Parent != nil {
		dirNode = node.Parent
	} else {
		return
	}
	
	fm.htree.RefreshNode(dirNode)
}

// RefreshDirectory refreshes a specific directory path in the tree
func (fm *FileManager) RefreshDirectory(dirPath string) {
	// Find the directory node
	dirNode := fm.htree.FindNodeByPath(dirPath)
	if dirNode != nil && dirNode.IsDir {
		// Only refresh if the directory is expanded (visible)
		if dirNode.IsExpanded {
			fm.htree.RefreshNode(dirNode)
		}
	}
}

// ClearClipboard clears the clipboard
func (fm *FileManager) ClearClipboard() {
	fm.clipboard.Clear()
	if fm.onClipboardChange != nil {
		fm.onClipboardChange("", false)
	}
}

// CreateNewDirectory creates a new folder in the selected directory
func (fm *FileManager) CreateNewDirectory() {
	node := fm.htree.GetCurrentNode()
	if node == nil {
		return
	}
	
	// Get target directory
	var targetDir string
	if node.IsDir {
		targetDir = node.Path
	} else {
		targetDir = filepath.Dir(node.Path)
	}
	
	// Generate unique directory name
	baseName := "new_folder"
	dirName := baseName
	counter := 1
	for {
		err := fm.fileOps.CreateDirectory(targetDir, dirName)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "already exists") {
			return // Other error
		}
		dirName = fmt.Sprintf("%s%d", baseName, counter)
		counter++
	}
	
	fullPath := filepath.Join(targetDir, dirName)
	
	// Schedule the UI update
	if fm.app != nil {
		fm.scheduleRefreshAndSelect(targetDir, fullPath)
	}
}

// CreateNewFile creates a new untitled file in the selected directory
func (fm *FileManager) CreateNewFile() {
	node := fm.htree.GetCurrentNode()
	if node == nil {
		return
	}
	
	// Get target directory
	var targetDir string
	if node.IsDir {
		targetDir = node.Path
	} else {
		targetDir = filepath.Dir(node.Path)
	}
	
	// Generate unique filename
	baseName := "untitled"
	fileName := baseName
	counter := 1
	for {
		err := fm.fileOps.CreateFile(targetDir, fileName)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "already exists") {
			logging.Error("Failed to create file: %v", err)
			return
		}
		fileName = fmt.Sprintf("%s%d", baseName, counter)
		counter++
	}
	
	fullPath := filepath.Join(targetDir, fileName)
	
	// Schedule the UI update
	if fm.app != nil {
		fm.scheduleRefreshAndSelect(targetDir, fullPath)
	}
}

// scheduleRefreshAndSelect schedules a UI update to refresh a directory and select an item
func (fm *FileManager) scheduleRefreshAndSelect(targetDir, fullPath string) {
	go func() {
		fm.app.QueueUpdateDraw(func() {
			// Find the target directory node
			targetNode := fm.htree.FindNodeByPath(targetDir)
			
			if targetNode != nil {
				// Expand the directory if not already expanded
				if !targetNode.IsExpanded {
					targetNode.IsExpanded = true
				}
				
				// Refresh the directory
				fm.htree.RefreshNode(targetNode)
				
				// Select the new item
				newNode := fm.htree.FindNodeByPath(fullPath)
				if newNode != nil {
					fm.htree.SetCurrentNode(newNode)
					fm.app.SetFocus(fm.htree)
				}
			}
		})
	}()
}

// CopySelected copies the selected file/directory to clipboard
func (fm *FileManager) CopySelected() {
	node := fm.htree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	fm.clipboard.Copy(node.Path)
	
	if fm.onClipboardChange != nil {
		fm.onClipboardChange(node.Path, false)
	}
}

// CutSelected cuts the selected file/directory to clipboard
func (fm *FileManager) CutSelected() {
	node := fm.htree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	fm.clipboard.Cut(node.Path)
	
	if fm.onClipboardChange != nil {
		fm.onClipboardChange(node.Path, true)
	}
}

// PasteClipboard pastes the clipboard content
func (fm *FileManager) PasteClipboard() {
	clipboardItem := fm.clipboard.Get()
	if clipboardItem == nil {
		return
	}
	
	node := fm.htree.GetCurrentNode()
	if node == nil {
		return
	}
	
	// Get target directory
	var targetDir string
	if node.IsDir {
		targetDir = node.Path
	} else {
		targetDir = filepath.Dir(node.Path)
	}
	
	// Store source directory for cut operations
	var sourceDir string
	if clipboardItem.IsCut {
		sourceDir = filepath.Dir(clipboardItem.Path)
		
		// If cutting and pasting to the same directory, it's a no-op
		if sourceDir == targetDir {
			fm.ClearClipboard()
			return
		}
	}
	
	// Get source name and prepare target
	sourceName := filepath.Base(clipboardItem.Path)
	targetPath := filepath.Join(targetDir, sourceName)
	
	// Handle name conflicts
	if _, err := os.Stat(targetPath); err == nil {
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
	
	// Perform file operation using new components
	var err error
	if clipboardItem.IsCut {
		err = fm.fileOps.MovePath(clipboardItem.Path, targetPath)
		if err == nil && fm.onFileRenamed != nil {
			fm.onFileRenamed(clipboardItem.Path, targetPath)
		}
	} else {
		err = fm.fileOps.CopyPath(clipboardItem.Path, targetPath)
	}
	
	if err != nil {
		return
	}
	
	// Clear clipboard
	wasCut := clipboardItem.IsCut
	fm.ClearClipboard()
	
	// Schedule the UI update
	if fm.app != nil {
		go func() {
			fm.app.QueueUpdateDraw(func() {
				// For cut operations, refresh both source and target directories
				if wasCut && sourceDir != "" && sourceDir != targetDir {
					sourceNode := fm.htree.FindNodeByPath(sourceDir)
					if sourceNode != nil && sourceNode.IsExpanded {
						fm.htree.RefreshNode(sourceNode)
					}
				}
				
				// Find and refresh target directory
				targetNode := fm.htree.FindNodeByPath(targetDir)
				if targetNode != nil {
					if !targetNode.IsExpanded {
						targetNode.IsExpanded = true
					}
					
					fm.htree.RefreshNode(targetNode)
					
					// Find and select the pasted item
					pastedNode := fm.htree.FindNodeByPath(targetPath)
					if pastedNode != nil {
						fm.htree.SetCurrentNode(pastedNode)
						fm.app.SetFocus(fm.htree)
					}
				}
			})
		}()
	}
}

// RenameSelected shows rename dialog for selected file/directory
func (fm *FileManager) RenameSelected(parentModal tview.Primitive) {
	node := fm.htree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	oldPath := node.Path
	oldName := filepath.Base(oldPath)
	
	// Create modal configuration
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,
		FixedHeight:     7,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " Rename ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(fm.app, config, true)
	
	// Set appropriate previous root
	if parentModal != nil {
		modal.SetPreviousRoot(parentModal)
	} else {
		modal.SetPreviousRoot(mainAppRoot)
	}
	
	// Store parent modal for closing later if needed
	var parentModalWrapper *modalWrapper
	if pm, ok := parentModal.(*modalWrapper); ok {
		parentModalWrapper = pm
	}
	
	// Create label and input field
	label := tview.NewTextView().
		SetText("New name:").
		SetTextAlign(tview.AlignLeft)
	
	inputField := tview.NewInputField().
		SetText(oldName).
		SetFieldWidth(45)
	
	// Add components to modal
	content.
		AddItem(nil, 1, 0, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 2, 0, false).
			AddItem(label, 0, 1, false).
			AddItem(nil, 2, 0, false), 1, 0, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 2, 0, false).
			AddItem(inputField, 0, 1, true).
			AddItem(nil, 2, 0, false), 1, 0, true).
		AddItem(nil, 1, 0, false)
	
	// Handle input
	inputField.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			newName := inputField.GetText()
			if newName != "" && newName != oldName {
				// Use file operations component for rename
				err := fm.fileOps.RenamePath(oldPath, newName)
				if err == nil {
					newPath := filepath.Join(filepath.Dir(oldPath), newName)
					
					// Notify listeners
					if fm.onFileRenamed != nil {
						fm.onFileRenamed(oldPath, newPath)
					}
					
					// Close both modals if rename was successful
					modal.Close()
					if parentModalWrapper != nil {
						parentModalWrapper.Close()
					}
					
					// Schedule UI update
					if fm.app != nil {
						go func() {
							fm.app.QueueUpdateDraw(func() {
								parentPath := filepath.Dir(newPath)
								parentNode := fm.htree.FindNodeByPath(parentPath)
								if parentNode != nil {
									fm.htree.RefreshNode(parentNode)
									renamedNode := fm.htree.FindNodeByPath(newPath)
									if renamedNode != nil {
										fm.htree.SetCurrentNode(renamedNode)
									}
								}
							})
						}()
					}
				} else {
					modal.Close()
				}
			} else {
				modal.Close()
			}
		} else if key == tcell.KeyEscape {
			modal.Cancel()
		}
	})
	
	modal.Show()
	fm.app.SetFocus(inputField)
}

// DeleteSelected shows delete confirmation for selected file/directory
func (fm *FileManager) DeleteSelected(parentModal tview.Primitive) {
	node := fm.htree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	path := node.Path
	fileName := filepath.Base(path)
	parentPath := filepath.Dir(path)
	
	// Check if it's a directory
	itemType := "file"
	if node.IsDir {
		itemType = "directory"
	}
	
	// Find the parent node and current index for smart selection
	parentNode := fm.htree.FindNodeByPath(parentPath)
	var currentIndex int = -1
	var siblingCount int = 0
	
	if parentNode != nil {
		children := parentNode.Children
		siblingCount = len(children)
		for i, child := range children {
			if child.Path == path {
				currentIndex = i
				break
			}
		}
	}
	
	// Show confirmation dialog
	message := fmt.Sprintf("Are you sure you want to delete the %s:\n\n%s\n\nThis action cannot be undone.\n(unless you know about code versioning =))", itemType, fileName)
	
	// Handle different modal scenarios
	if parentModal != nil {
		if parentModalWrapper, ok := parentModal.(*modalWrapper); ok {
			fm.showConfirmModalWithParent(parentModalWrapper, "Delete Confirmation", message,
				func() {
					fm.performDelete(path, parentPath, currentIndex, siblingCount)
					parentModalWrapper.Close()
				},
				nil)
		} else {
			ShowConfirmModal(fm.app, "Delete Confirmation", message,
				func() {
					fm.performDelete(path, parentPath, currentIndex, siblingCount)
				},
				nil)
		}
	} else {
		ShowConfirmModal(fm.app, "Delete Confirmation", message,
			func() {
				fm.performDelete(path, parentPath, currentIndex, siblingCount)
			},
			nil)
	}
}

// performDelete actually performs the file deletion and UI update
func (fm *FileManager) performDelete(path, parentPath string, currentIndex, siblingCount int) {
	// Delete using file operations component
	err := fm.fileOps.DeletePath(path)
	if err != nil {
		return
	}
	
	// Notify listeners
	if fm.onFileDeleted != nil {
		fm.onFileDeleted(path)
	}
	
	// Schedule UI update
	if fm.app != nil {
		go func() {
			fm.app.QueueUpdateDraw(func() {
				// Find parent node again after deletion
				parentNode := fm.htree.FindNodeByPath(parentPath)
				if parentNode != nil {
					// Refresh parent's children
					fm.htree.RefreshNode(parentNode)
					
					// Smart selection logic
					children := parentNode.Children
					newChildCount := len(children)
					
					if newChildCount == 0 {
						// No children left, select parent
						fm.htree.SetCurrentNode(parentNode)
						fm.app.SetFocus(fm.htree)
					} else if currentIndex >= 0 && siblingCount > 1 {
						// There were siblings, select the appropriate one
						var nodeToSelect *HTreeNode
						
						if currentIndex < newChildCount {
							nodeToSelect = children[currentIndex]
						} else if currentIndex > 0 && newChildCount > 0 {
							nodeToSelect = children[newChildCount-1]
						} else {
							nodeToSelect = children[0]
						}
						
						if nodeToSelect != nil {
							fm.htree.SetCurrentNode(nodeToSelect)
							fm.app.SetFocus(fm.htree)
						}
					} else {
						// Fallback: select parent
						fm.htree.SetCurrentNode(parentNode)
						fm.app.SetFocus(fm.htree)
					}
				}
			})
		}()
	}
}

// showConfirmModalWithParent shows a confirmation dialog with a specific parent modal
func (fm *FileManager) showConfirmModalWithParent(parentModal *modalWrapper, title, message string, onConfirm, onCancel func()) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      50,
		FixedHeight:     10,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " " + title + " ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, contentFlex := NewConfiguredModal(fm.app, config, true)
	modal.SetPreviousRoot(parentModal)
	
	// Create message text
	messageView := tview.NewTextView().
		SetText(message).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	
	// Create buttons
	buttonsFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
	
	yesButton := tview.NewButton("YES")
	yesButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	yesButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	noButton := tview.NewButton("NO")
	noButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	noButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	
	// Create spacers
	leftSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	middleSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	rightSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	topSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	bottomSpacer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	
	buttonsFlex.
		AddItem(leftSpacer, 0, 1, false).
		AddItem(yesButton, 10, 0, true).
		AddItem(middleSpacer, 2, 0, false).
		AddItem(noButton, 10, 0, false).
		AddItem(rightSpacer, 0, 1, false)
	
	// Add components to content
	contentFlex.
		AddItem(topSpacer, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(bottomSpacer, 0, 1, false)
	
	// Set button handlers
	yesButton.SetSelectedFunc(func() {
		modal.Close()
		if onConfirm != nil {
			onConfirm()
		}
	})
	
	noButton.SetSelectedFunc(func() {
		modal.Cancel()
	})
	
	// Set up button navigation
	yesButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight, tcell.KeyLeft:
			fm.app.SetFocus(noButton)
			return nil
		}
		return event
	})
	
	noButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight, tcell.KeyLeft:
			fm.app.SetFocus(yesButton)
			return nil
		}
		return event
	})
	
	modal.modal.SetOnCancel(onCancel)
	modal.Show()
	fm.app.SetFocus(yesButton)
}

// showInfoModal shows the info modal for a file/directory
func (fm *FileManager) showInfoModal(node *HTreeNode) {
	if fm.app == nil || node == nil {
		return
	}
	
	// Check if file exists
	_, err := os.Stat(node.Path)
	if err != nil {
		return
	}
	
	// Build relative path
	relativePath := fm.htree.GetRelativePath(node)
	
	// Determine type
	fileType := "File"
	if node.IsDir {
		fileType = "Directory"
	}
	
	// Get size (only for files)
	sizeStr := "N/A"
	if !node.IsDir {
		sizeStr = getFileSize(node.Path)
	}
	
	// Create modal
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      65,
		FixedHeight:     20,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.ColorGray,
		TitleColor:      tcell.ColorLightGray,
		Title:           " File Information ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(fm.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Create info text
	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[blue]Name:[-] %s\n[blue]Type:[-] %s\n[blue]Size:[-] %s\n[blue]Relative Path:[-]\n%s\n",
			node.Text,
			fileType,
			sizeStr,
			relativePath))
	
	flex.AddItem(infoText, 6, 0, false)
	
	// Create button list
	buttonList := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Store buttons for navigation
	var buttons []*tview.Button
	var currentButtonIndex int
	
	// Helper to create buttons with navigation
	createButton := func(label string, handler func()) *tview.Button {
		btn := tview.NewButton(label).SetSelectedFunc(handler)
		btn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
		btn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
		
		buttons = append(buttons, btn)
		btnIndex := len(buttons) - 1
		
		btn.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyTab, tcell.KeyDown, tcell.KeyRight:
				currentButtonIndex = (btnIndex + 1) % len(buttons)
				fm.app.SetFocus(buttons[currentButtonIndex])
				return nil
			case tcell.KeyBacktab, tcell.KeyUp, tcell.KeyLeft:
				currentButtonIndex = btnIndex - 1
				if currentButtonIndex < 0 {
					currentButtonIndex = len(buttons) - 1
				}
				fm.app.SetFocus(buttons[currentButtonIndex])
				return nil
			}
			return event
		})
		
		return btn
	}
	
	// Add action buttons
	buttonList.AddItem(createButton("Copy Name", func() {
		copyToSystemClipboard(node.Text)
		modal.Close()
	}), 1, 0, true)
	
	buttonList.AddItem(createButton("Copy Relative Path", func() {
		copyToSystemClipboard(relativePath)
		modal.Close()
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Copy Full Path", func() {
		copyToSystemClipboard(node.Path)
		modal.Close()
	}), 1, 0, false)
	
	buttonList.AddItem(nil, 1, 0, false) // Separator
	
	buttonList.AddItem(createButton("Create New File", func() {
		fm.CreateNewFile()
		modal.Close()
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Create New Directory", func() {
		fm.CreateNewDirectory()
		modal.Close()
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Copy", func() {
		fm.CopySelected()
		modal.Close()
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Cut", func() {
		fm.CutSelected()
		modal.Close()
	}), 1, 0, false)
	
	// Only show paste if clipboard has content
	if fm.clipboard.HasContent() {
		buttonList.AddItem(createButton("Paste", func() {
			fm.PasteClipboard()
			modal.Close()
		}), 1, 0, false)
	}
	
	buttonList.AddItem(createButton("Rename", func() {
		fm.RenameSelected(modal)
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Delete", func() {
		fm.DeleteSelected(modal)
	}), 1, 0, false)
	
	buttonList.AddItem(nil, 1, 0, false) // Separator
	
	buttonList.AddItem(createButton("Cancel", func() {
		modal.Close()
	}), 1, 0, false)
	
	// Add button list to flex
	flex.AddItem(buttonList, 0, 1, true)
	
	// Add flex to modal content
	content.AddItem(flex, 0, 1, true)
	
	// Show modal and focus first button
	modal.Show()
	if len(buttons) > 0 {
		fm.app.SetFocus(buttons[0])
		currentButtonIndex = 0
	}
}

// copyToSystemClipboard copies text to system clipboard
func copyToSystemClipboard(text string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		return fmt.Errorf("unsupported platform")
	}
	
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	if _, err := pipe.Write([]byte(text)); err != nil {
		return err
	}
	
	if err := pipe.Close(); err != nil {
		return err
	}
	
	return cmd.Wait()
}

// getFileSize returns formatted file size
func getFileSize(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	
	size := info.Size()
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}