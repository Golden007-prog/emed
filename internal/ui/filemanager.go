package ui

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FileManager represents the file tree panel
type FileManager struct {
	tree           *HTree
	root           *HTreeNode
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
		tree: NewHTree(),
	}
	
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	fm.rootPath = cwd
	
	// Create root node
	fm.root = &HTreeNode{
		Text:       filepath.Base(cwd),
		Path:       cwd,
		IsDir:      true,
		IsExpanded: true,
		Color:      tcell.ColorBlue,
	}
	
	// Set up tree view
	fm.tree.
		SetRoot(fm.root).
		SetCurrentNode(fm.root).
		SetBorder(true).
		SetBorderColor(tcell.ColorGray).
		SetTitle(" Project ").
		SetTitleColor(tcell.ColorLightGray).
		SetTitleAlign(tview.AlignLeft)
	
	// Load initial directory structure
	fm.tree.loadChildren(fm.root)
	
	// Set up selection handler
	fm.tree.SetSelectedFunc(func(node *HTreeNode) {
		if !node.IsDir && fm.onFileSelected != nil {
			fm.onFileSelected(node.Path)
		}
	})
	
	// Set up info handler
	fm.tree.SetInfoFunc(func(node *HTreeNode) {
		fm.showInfoModal(node)
	})
	
	return fm
}

// isCodeFile checks if a file is a code file based on its extension
func isCodeFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	codeExtensions := []string{
		".go", ".rs", ".c", ".cpp", ".cc", ".h", ".hpp",
		".java", ".kt", ".scala", ".groovy",
		".py", ".rb", ".php", ".pl", ".lua",
		".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs",
		".html", ".css", ".scss", ".sass", ".less",
		".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat",
		".sql", ".r", ".m", ".swift", ".vb", ".cs",
		".clj", ".elm", ".ex", ".exs", ".erl", ".hrl",
		".hs", ".ml", ".fs", ".nim", ".cr", ".d",
		".zig", ".v", ".dart", ".julia", ".asm", ".s",
	}
	
	for _, codeExt := range codeExtensions {
		if ext == codeExt {
			return true
		}
	}
	return false
}


// refreshDirectory refreshes a directory while preserving expansion state
func (fm *FileManager) refreshDirectory(node *HTreeNode) {
	if node != nil {
		fm.tree.RefreshNode(node)
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
	
	// Get the directory node to refresh
	var dirNode *HTreeNode
	if node.IsDir {
		dirNode = node
	} else if node.Parent != nil {
		dirNode = node.Parent
	} else {
		return
	}
	
	fm.tree.RefreshNode(dirNode)
}

// SetApp sets the application reference
func (fm *FileManager) SetApp(app *tview.Application) {
	fm.app = app
	fm.tree.SetApp(app)
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
	
	// Get target directory
	var targetDir string
	if node.IsDir {
		targetDir = node.Path
	} else {
		targetDir = filepath.Dir(node.Path)
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
				targetNode := fm.tree.FindNodeByPath(capturedTargetDir)
				
				if targetNode != nil {
					// Expand the directory if not already expanded
					if !targetNode.IsExpanded {
						targetNode.IsExpanded = true
					}
					
					// Refresh the directory
					fm.tree.RefreshNode(targetNode)
					
					// Select the new file
					newNode := fm.tree.FindNodeByPath(capturedFullPath)
					if newNode != nil {
						fm.tree.SetCurrentNode(newNode)
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
	
	fm.clipboardPath = node.Path
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
	
	fm.clipboardPath = node.Path
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
	
	// Get target directory
	var targetDir string
	if node.IsDir {
		targetDir = node.Path
	} else {
		targetDir = filepath.Dir(node.Path)
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
	var err error
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
					sourceNode := fm.tree.FindNodeByPath(capturedSourceDir)
					if sourceNode != nil && sourceNode.IsExpanded {
						fm.tree.RefreshNode(sourceNode)
					}
				}
				
				// Find and refresh target directory
				targetNode := fm.tree.FindNodeByPath(capturedTargetDir)
				if targetNode != nil {
					// Make sure it's expanded
					if !targetNode.IsExpanded {
						targetNode.IsExpanded = true
					}
					
					// Refresh the directory
					fm.tree.RefreshNode(targetNode)
					
					// Find and select the pasted item
					pastedNode := fm.tree.FindNodeByPath(capturedTargetPath)
					if pastedNode != nil {
						fm.tree.SetCurrentNode(pastedNode)
					}
				}
			})
		}()
	}
}

// RenameSelected shows rename dialog for selected file/directory
// parentModal is optional - if provided, the rename modal will be shown on top of it
func (fm *FileManager) RenameSelected(parentModal tview.Primitive) {
	node := fm.tree.GetCurrentNode()
	if node == nil || node == fm.root {
		return
	}
	
	oldPath := node.Path
	oldName := filepath.Base(oldPath)
	
	// Create modal configuration - compact size for single input
	config := EmModalConfig{
		Width:           50,
		Height:          8,   // Very compact - just enough for border, label, input
		BackgroundColor: tcell.ColorDefault,
		BorderColor:     tcell.ColorBlue,
		TitleColor:      tcell.ColorBlue,
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
	
	// Create input field
	inputField := tview.NewInputField().
		SetLabel("New name: ").
		SetText(oldName).
		SetFieldWidth(40)
	
	// Add input field to modal content with minimal padding
	content.
		AddItem(nil, 1, 0, false).  // Small top padding
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(inputField, 50, 1, true).
			AddItem(nil, 0, 1, false), 1, 0, true).
		AddItem(nil, 1, 0, false)  // Small bottom padding
	
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
						
						// Close both modals if rename was successful and we have a parent
						modal.Close()
						if parentModalWrapper != nil {
							parentModalWrapper.Close()
						}
						
						// Schedule the UI update from a goroutine
						if fm.app != nil {
							go func() {
								fm.app.QueueUpdateDraw(func() {
									// Find parent node and refresh it
									parentPath := filepath.Dir(newPath)
									parentNode := fm.tree.FindNodeByPath(parentPath)
									if parentNode != nil {
										// Refresh parent's children
										fm.tree.RefreshNode(parentNode)
										// Find and select the renamed item
										renamedNode := fm.tree.FindNodeByPath(newPath)
										if renamedNode != nil {
											fm.tree.SetCurrentNode(renamedNode)
										}
									}
								})
							}()
						}
					} else {
						// Rename failed, just close rename modal
						modal.Close()
					}
				} else {
					// File already exists, just close rename modal
					modal.Close()
				}
			} else {
				// No change or empty name, just close rename modal
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
// parentModal is optional - if provided, the delete confirmation will be shown on top of it
func (fm *FileManager) DeleteSelected(parentModal tview.Primitive) {
	node := fm.tree.GetCurrentNode()
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
	
	// Find the parent node and current index before deletion
	parentNode := fm.tree.FindNodeByPath(parentPath)
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
	
	// If we have a parent modal, use the custom confirm modal
	if parentModal != nil {
		if parentModalWrapper, ok := parentModal.(*modalWrapper); ok {
			fm.showConfirmModalWithParent(parentModalWrapper, "Delete Confirmation", message,
				func() {
					// User confirmed - perform delete and close parent modal
					fm.performDelete(path, parentPath, currentIndex, siblingCount)
					parentModalWrapper.Close()
				},
				nil) // Cancel just closes the confirm modal, keeping parent open
		} else {
			// Parent modal is not a modalWrapper, use regular confirm
			ShowConfirmModal(fm.app, "Delete Confirmation", message,
				func() {
					fm.performDelete(path, parentPath, currentIndex, siblingCount)
				},
				nil)
		}
	} else {
		// No parent modal, use regular confirm
		ShowConfirmModal(fm.app, "Delete Confirmation", message,
			func() {
				fm.performDelete(path, parentPath, currentIndex, siblingCount)
			},
			nil)
	}
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
				parentNode := fm.tree.FindNodeByPath(capturedParentPath)
				if parentNode != nil {
					// Refresh parent's children
					fm.tree.RefreshNode(parentNode)
					
					// Smart selection logic
					children := parentNode.Children
					newChildCount := len(children)
					
					if newChildCount == 0 {
						// No children left, select parent
						fm.tree.SetCurrentNode(parentNode)
					} else if capturedIndex >= 0 && capturedSiblingCount > 1 {
						// There were siblings, select the appropriate one
						var nodeToSelect *HTreeNode
						
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

// copyToSystemClipboard copies text to system clipboard
func copyToSystemClipboard(text string) error {
	var cmd *exec.Cmd
	
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
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



// showConfirmModalWithParent shows a confirmation dialog with a specific parent modal
func (fm *FileManager) showConfirmModalWithParent(parentModal *modalWrapper, title, message string, onConfirm, onCancel func()) {
	// Create outer flex (borderless, titleless)
	outerFlex := tview.NewFlex()
	
	// Create a Box that will fill the entire area with black background
	backgroundBox := tview.NewBox().
		SetBorder(true).
		SetTitle(" " + title + " ").
		SetTitleAlign(tview.AlignCenter).
		SetBackgroundColor(tcell.ColorBlack).
		SetBorderColor(tcell.ColorYellow).
		SetTitleColor(tcell.ColorYellow)
	
	// Create content flex for message and buttons
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	
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
	
	buttonsFlex.
		AddItem(nil, 0, 1, false).
		AddItem(yesButton, 10, 0, true).
		AddItem(nil, 2, 0, false).
		AddItem(noButton, 10, 0, false).
		AddItem(nil, 0, 1, false)
	
	// Add message and buttons to content flex
	contentFlex.
		AddItem(nil, 0, 1, false).
		AddItem(messageView, 0, 2, false).
		AddItem(buttonsFlex, 3, 0, true).
		AddItem(nil, 0, 1, false)
	
	// Create a custom primitive that combines background and content
	modalContent := &backgroundWithContent{
		Box:        tview.NewBox(),
		background: backgroundBox,
		content:    contentFlex,
	}
	
	// Add the combined primitive to outer flex
	outerFlex.AddItem(modalContent, 0, 1, true)
	
	// Create the modal with the outer flex
	modal := NewEmModal(fm.app, outerFlex, true)
	modal.SetPreviousRoot(parentModal) // Set parent modal, not mainAppRoot
	
	// Set button handlers with access to modal
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
		case tcell.KeyTab, tcell.KeyRight:
			fm.app.SetFocus(noButton)
			return nil
		case tcell.KeyLeft:
			fm.app.SetFocus(noButton)
			return nil
		}
		return event
	})
	
	noButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab, tcell.KeyRight:
			fm.app.SetFocus(yesButton)
			return nil
		case tcell.KeyLeft:
			fm.app.SetFocus(yesButton)
			return nil
		}
		return event
	})
	
	modal.SetOnCancel(onCancel)
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
	relativePath := fm.tree.GetRelativePath(node)
	
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
	
	// Create modal configuration
	config := EmModalConfig{
		Width:           60,
		Height:          25,
		BackgroundColor: tcell.ColorDefault,
		BorderColor:     tcell.ColorBlue,
		TitleColor:      tcell.ColorBlue,
		Title:           " File Information ",
		TitleAlign:      tview.AlignCenter,
	}
	
	modal, content := NewConfiguredModal(fm.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	
	// Create flex layout for content
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Create info text
	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetText(fmt.Sprintf("[yellow]Name:[-] %s\n[yellow]Type:[-] %s\n[yellow]Size:[-] %s\n\n[yellow]Relative Path:[-]\n%s\n",
			node.Text,
			fileType,
			sizeStr,
			relativePath))
	
	// Add info text to flex
	flex.AddItem(infoText, 0, 1, false)
	
	// Create button list as a flex container
	buttonList := tview.NewFlex().SetDirection(tview.FlexRow)
	
	// Store all buttons for navigation
	var buttons []*tview.Button
	var currentButtonIndex int
	
	// Helper to create styled buttons with navigation
	createButton := func(label string, handler func()) *tview.Button {
		btn := tview.NewButton(label).SetSelectedFunc(handler)
		btn.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
		btn.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
		
		// Add to buttons array
		buttons = append(buttons, btn)
		btnIndex := len(buttons) - 1
		
		// Set up keyboard navigation
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
	
	// Add buttons to list
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
	
	// Add separator (empty space)
	buttonList.AddItem(nil, 1, 0, false)
	
	// Add Create New File button - only show if current node is a directory
	// or if it's a file (we'll create in the parent directory)
	buttonList.AddItem(createButton("Create New File", func() {
		fm.CreateNewFile()
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
	
	if fm.clipboardPath != "" {
		buttonList.AddItem(createButton("Paste", func() {
			fm.PasteClipboard()
			modal.Close()
		}), 1, 0, false)
	}
	
	buttonList.AddItem(createButton("Rename", func() {
		// Show rename modal on top of info modal
		// The rename modal will close both if successful
		fm.RenameSelected(modal)
	}), 1, 0, false)
	
	buttonList.AddItem(createButton("Delete", func() {
		// Show delete confirmation on top of info modal
		// Only close info modal if user confirms deletion
		fm.DeleteSelected(modal)
	}), 1, 0, false)
	
	// Add separator before cancel
	buttonList.AddItem(nil, 1, 0, false)
	
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