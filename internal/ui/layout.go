// Package ui provides the user interface components for the text editor.
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/config"
	"github.com/rivo/tview"
)

// MouseFocusDetector wraps a primitive to detect mouse focus changes.
type MouseFocusDetector struct {
	tview.Primitive
	layout *Layout
}

// MouseHandler returns a mouse handler that detects focus changes.
func (m *MouseFocusDetector) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if handler := m.Primitive.MouseHandler(); handler != nil {
			consumed, capture = handler(action, event, setFocus)
		}
		
		if action == tview.MouseLeftClick || action == tview.MouseLeftDown {
			go func() {
				time.Sleep(10 * time.Millisecond)
				m.layout.app.QueueUpdateDraw(func() {
					m.layout.checkFocusChange()
				})
			}()
		}
		
		return consumed, capture
	}
}

// Layout manages the overall application layout and component coordination.
type Layout struct {
	app         *tview.Application
	root        tview.Primitive
	mainArea    *tview.Flex
	rightSide   *tview.Flex
	fileManager *FileManager
	editor      *Editor
	terminal    *Terminal
	statusBar   *StatusBar
	fileWatcher *FileWatcher
	stopTicker  chan bool
	stopFocus   chan bool
	
	focusedPanel int
	lastFocus    tview.Primitive
	
	terminalPosition string
	proportionIndex  int
	proportionDir    int
}

// NewLayout creates a new Layout with all UI components.
func NewLayout(app *tview.Application) *Layout {
	l := &Layout{
		app:              app,
		focusedPanel:     0,
		terminalPosition: "bottom",
		proportionIndex:  1,
		proportionDir:    1,
	}
	
	l.fileManager = NewFileManager()
	l.editor = NewEditor()
	l.terminal = NewTerminal()
	l.statusBar = NewStatusBar()
	
	l.initialize()
	return l
}

func (l *Layout) initialize() {
	l.editor.SetApp(l.app)
	l.terminal.SetApp(l.app)
	l.statusBar.SetApp(l.app)
	
	l.editor.SetOnLastTabClosed(func() {
		l.rebuildLayout()
		// Defer focus setting to ensure UI is ready
		go func() {
			l.app.QueueUpdate(func() {
				l.focusFileManager()
			})
		}()
	})
	
	l.editor.SetOnTabClosed(func(path string) {
		if l.fileWatcher != nil {
			l.fileWatcher.RemoveOpenFile(path)
			l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
		}
	})
	
	l.terminal.SetOnLastTabClosed(func() {
		l.rebuildLayout()
		if l.editor.HasTabs() {
			l.focusEditor()
		} else {
			l.focusFileManager()
		}
	})
	
	
	l.fileManager.SetFileSelectedHandler(func(path string) {
		l.OpenFile(path)
	})
	
	l.fileManager.SetClipboardChangeHandler(func(path string, isCut bool) {
		l.statusBar.SetClipboard(path, isCut)
	})
	
	l.fileManager.SetFileRenamedHandler(func(oldPath, newPath string) {
		l.editor.UpdateFilePath(oldPath, newPath)
	})
	
	l.fileManager.SetFileDeletedHandler(func(path string) {
		l.editor.CloseFileTab(path)
	})
	
	l.root = tview.NewFlex().SetDirection(tview.FlexRow)
	
	l.mainArea = tview.NewFlex().SetDirection(tview.FlexColumn)
	l.mainArea.SetBackgroundColor(tcell.ColorBlack)
	
	l.rightSide = tview.NewFlex()
	l.rightSide.SetBackgroundColor(tcell.ColorBlack)
	
	l.rebuildLayout()
	
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.mainArea, 0, 1, true).
		AddItem(l.statusBar.GetView(), 1, 0, false)
	mainLayout.SetBackgroundColor(tcell.ColorBlack)
	
	rootWithMouse := &MouseFocusDetector{
		Primitive: mainLayout,
		layout:    l,
	}
	
	l.root = rootWithMouse
	
	SetMainAppRoot(l.root)
	
	l.fileManager.SetApp(l.app)
	
	l.setupKeybindings()
	
	l.setupFileWatcher()
	
	l.setupFocusDetection()
	
	go func() {
		time.Sleep(10 * time.Millisecond)
		l.app.QueueUpdateDraw(func() {
			l.focusFileManager()
		})
	}()
}

// GetRoot returns the root primitive for the layout.
func (l *Layout) GetRoot() tview.Primitive {
	return l.root
}

func (l *Layout) rebuildLayout() {
	l.rightSide.Clear()
	
	l.mainArea.Clear()
	
	hasEditorTabs := l.editor.HasTabs()
	hasTerminalTabs := l.terminal.HasTabs()
	
	if !hasEditorTabs && !hasTerminalTabs {
		noDocsView := l.editor.GetNoDocsView()
		if box, ok := noDocsView.(*tview.Box); ok {
			box.SetBorder(false)
		}
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(noDocsView, 0, 1, false)
	} else if hasEditorTabs && !hasTerminalTabs {
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(l.editor.GetView(), 0, 1, false)
	} else if !hasEditorTabs && hasTerminalTabs {
		l.rightSide.SetDirection(tview.FlexRow).
			Clear().
			AddItem(l.terminal.GetView(), 0, 1, false)
	} else {
		proportions := [][2]int{
			{8, 2},
			{7, 3},
			{6, 4},
			{5, 5},
			{4, 6},
			{3, 7},
			{2, 8},
		}
		
		editorProp := proportions[l.proportionIndex][0]
		terminalProp := proportions[l.proportionIndex][1]
		
		if l.terminalPosition == "bottom" {
			l.rightSide.SetDirection(tview.FlexRow).
				Clear().
				AddItem(l.editor.GetView(), 0, editorProp, false).
				AddItem(l.terminal.GetView(), 0, terminalProp, false)
		} else {
			l.rightSide.SetDirection(tview.FlexColumn).
				Clear().
				AddItem(l.editor.GetView(), 0, editorProp, false).
				AddItem(l.terminal.GetView(), 0, terminalProp, false)
		}
	}
	
	l.mainArea.
		AddItem(l.fileManager.GetView(), config.LayoutFileManagerProportion+10, 0, false).
		AddItem(l.rightSide, 0, 1, false)
}

func (l *Layout) cycleProportion() {
	if !l.terminal.HasTabs() {
		return
	}
	
	l.proportionIndex += l.proportionDir
	
	if l.proportionIndex >= 7 {
		l.proportionIndex = 5
		l.proportionDir = -1
	} else if l.proportionIndex < 0 {
		l.proportionIndex = 1
		l.proportionDir = 1
	}
	
	l.rebuildLayout()
}

func (l *Layout) togglePosition() {
	if !l.terminal.HasTabs() {
		return
	}
	
	if l.terminalPosition == "bottom" {
		l.terminalPosition = "right"
	} else {
		l.terminalPosition = "bottom"
	}
	l.rebuildLayout()
}

func (l *Layout) getHelpText() string {
	return `[::b]KEYBOARD SHORTCUTS[-::-]

[blue]FILE MANAGER (when focused):[-]
  Enter          Open File / Toggle Directory
  i              File Info Dialog
  Ctrl+N         Create New File
  Ctrl+Alt+N     Create New Directory
  Ctrl+C         Copy File/Directory
  Ctrl+X         Cut File/Directory  
  Ctrl+V         Paste File/Directory
  Ctrl+D         Delete File/Directory
  Ctrl+R         Rename File/Directory
  q              Quit Application
  
[blue]EDITOR:[-]
  Ctrl+S         Save File (coming soon)
  Ctrl+W         Close Current Tab
  Ctrl+F         Search in Document
  Ctrl+Z         Undo (coming soon)
  Ctrl+Y         Redo (coming soon)
  
[blue]NAVIGATION:[-]
  Ctrl+A         Focus File Manager
  Ctrl+E         Focus Editor
  Ctrl+T         Focus/Create Terminal
  F2             Cycle Focus Between Panels
  
[blue]TERMINAL:[-]
  Alt+T          Toggle Terminal Mode (Passthrough/Application)
  
[blue]TABS:[-]
  Ctrl+Shift+←   Previous Tab (or F3)
  Ctrl+Shift+→   Next Tab (or F4)
  F3             Previous Tab
  F4             Next Tab
  
[blue]LAYOUT:[-]
  Ctrl+L         Cycle Proportions (or F7)
  Ctrl+P         Toggle Terminal Position (or F6)
  F6             Toggle Terminal Position
  F7             Cycle Proportions
  
[blue]GENERAL:[-]
  F1             Show This Help
  Ctrl+Q         Quit

[blue]FILE MANAGEMENT:[-]
When the file manager has focus, you can manage your files easily. Use arrow keys to navegate through your project tree. Press Ctrl+N to create new files right were you need them. Copy files with Ctrl+C, then move to another folder and paste with Ctrl+V. Delete unwanted files with Ctrl+D - but be carefull, there's no undo for this! Rename files with Ctrl+R and use Ctrl+X to cut (move) files in combination with Ctrl+V.

[blue]TERMINAL PASSTHROUGH MODE:[-]
The terminal has two modes. Normal mode lets you use keyboard shortcuts normaly. But sometimes you need to send special keys directly to the terminal program (like when using vim or other CLI tools). Press Alt+T to toggle passthrough mode. In this mode, all keystrokes go straight to the terminal - only Alt+T works to switch back. The terminal tab title shows when passthru mode is active for that particular tab with an asterisk.

[::d]Press ESC or click outside to close[-::-]`
}

func (l *Layout) showHelp() {
	ShowHelpModal(l.app, l.getHelpText())
}

func (l *Layout) setupKeybindings() {
	l.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if IsModalOpen() {
			return event
		}
		
		currentFocus := l.app.GetFocus()
		if l.fileManager.HasFocus(currentFocus) {
			l.focusedPanel = 0
		} else if l.editor.HasFocus(currentFocus) {
			l.focusedPanel = 1
		} else if l.terminal.HasFocus(currentFocus) {
			l.focusedPanel = 2
		}
		
		if l.focusedPanel == 2 && l.terminal.IsInPassthroughMode() {
			return event
		}
		
		if l.focusedPanel == 0 && event.Key() == tcell.KeyRune && event.Rune() == 'q' && event.Modifiers() == 0 {
			l.tryQuit()
			return nil
		}
		
		switch event.Key() {
		case tcell.KeyF1:
			l.showHelp()
			return nil
		case tcell.KeyF2:
			hasEditor := l.editor.HasTabs()
			hasTerminal := l.terminal.HasTabs()
			
			if !hasEditor && !hasTerminal {
				l.focusFileManager()
			} else if !hasEditor {
				if l.focusedPanel == 0 {
					l.focusTerminal()
				} else {
					l.focusFileManager()
				}
			} else if !hasTerminal {
				if l.focusedPanel == 0 {
					l.focusEditor()
				} else {
					l.focusFileManager()
				}
			} else {
				nextPanel := (l.focusedPanel + 1) % 3
				switch nextPanel {
				case 0:
					l.focusFileManager()
				case 1:
					l.focusEditor()
				case 2:
					l.focusTerminal()
				}
			}
			return nil
		case tcell.KeyF3:
			if l.focusedPanel == 1 {
				l.editor.PreviousTab()
			} else if l.focusedPanel == 2 {
				l.terminal.PreviousTab()
			}
			return nil
		case tcell.KeyF4:
			if l.focusedPanel == 1 {
				l.editor.NextTab()
			} else if l.focusedPanel == 2 {
				l.terminal.NextTab()
			}
			return nil
		case tcell.KeyF5:
			if l.focusedPanel == 0 {
				l.fileManager.RefreshCurrentDirectory()
			}
			return nil
		case tcell.KeyF6:
			if l.terminal.HasTabs() {
				l.togglePosition()
			}
			return nil
		case tcell.KeyF7:
			if l.terminal.HasTabs() {
				l.cycleProportion()
			}
			return nil
		}
		
		if event.Modifiers() == tcell.ModCtrl|tcell.ModShift {
			switch event.Key() {
			case tcell.KeyLeft:
				if l.focusedPanel == 1 {
					l.editor.PreviousTab()
				} else if l.focusedPanel == 2 {
					l.terminal.PreviousTab()
				}
				return nil
			case tcell.KeyRight:
				if l.focusedPanel == 1 {
					l.editor.NextTab()
				} else if l.focusedPanel == 2 {
					l.terminal.NextTab()
				}
				return nil
			}
		}
		
		if event.Modifiers() == tcell.ModCtrl {
			switch event.Key() {
			case tcell.KeyCtrlA:
				l.focusFileManager()
				return nil
			case tcell.KeyCtrlE:
				if l.editor.HasTabs() {
					l.focusEditor()
				}
				return nil
			case tcell.KeyCtrlT:
				if l.terminal.HasFocus(l.app.GetFocus()) {
					l.terminal.NewTab()
				} else {
					if !l.terminal.HasTabs() {
						l.terminal.NewTab()
						l.rebuildLayout()
					}
					l.focusTerminal()
				}
				return nil
			case tcell.KeyCtrlS:
				if l.focusedPanel == 1 {
					l.editor.SaveCurrentFile()
				}
				return nil
			case tcell.KeyCtrlF:
				if l.focusedPanel == 0 {
					l.fileManager.ShowSearch()
				} else if l.focusedPanel == 1 {
					l.editor.ShowSearch()
				}
				return nil
			case tcell.KeyCtrlW:
				if l.focusedPanel == 1 {
					l.editor.CloseCurrentTab()
				} else if l.focusedPanel == 2 {
					l.terminal.CloseCurrentTab()
				}
				return nil
			case tcell.KeyCtrlN:
				if l.focusedPanel == 0 {
					return event
				}
				return nil
			case tcell.KeyCtrlX, tcell.KeyCtrlV, tcell.KeyCtrlD, tcell.KeyCtrlR:
				if l.focusedPanel == 0 && event.Modifiers() == tcell.ModCtrl {
					return event
				}
				if l.focusedPanel == 1 && (event.Key() == tcell.KeyCtrlX || event.Key() == tcell.KeyCtrlV) {
					return event
				}
				return nil
			case tcell.KeyCtrlZ, tcell.KeyCtrlY:
				return event
			case tcell.KeyCtrlL:
				if l.terminal.HasTabs() {
					l.cycleProportion()
				}
				return nil
			case tcell.KeyCtrlP:
				if l.terminal.HasTabs() {
					l.togglePosition()
				}
				return nil
			case tcell.KeyCtrlQ:
				l.tryQuit()
				return nil
			case tcell.KeyCtrlC:
				// tview quits on KeyCtrlC by default, prevent that
				currentFocus := l.app.GetFocus()
				if l.focusedPanel == 0 {
					return tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModCtrl)
				} else if l.editor.HasFocus(currentFocus) {
					return tcell.NewEventKey(tcell.KeyRune, 3, tcell.ModCtrl)
				} else if l.terminal.HasFocus(currentFocus) {
					return tcell.NewEventKey(tcell.KeyRune, 3, tcell.ModNone)
				}
				return nil
			}
		}
		
		return event
	})
}

func (l *Layout) focusFileManager() {
	l.focusedPanel = 0
	view := l.fileManager.GetView()
	l.app.SetFocus(view)
	l.lastFocus = view
	l.editor.UpdateTabBar()
	l.terminal.UpdateTabBar()
}

func (l *Layout) focusEditor() {
	if l.focusedPanel == 0 {
		l.fileManager.ClearClipboard()
	}
	l.focusedPanel = 1
	view := l.editor.GetView()
	l.app.SetFocus(view)
	l.lastFocus = view
	l.editor.UpdateTabBar()
	l.terminal.UpdateTabBar()
}

func (l *Layout) focusTerminal() {
	if l.focusedPanel == 0 {
		l.fileManager.ClearClipboard()
	}
	l.focusedPanel = 2
	view := l.terminal.GetView()
	l.app.SetFocus(view)
	l.lastFocus = view
	l.editor.UpdateTabBar()
	l.terminal.UpdateTabBar()
}

// OpenFile opens a file in the editor and updates the layout if needed.
func (l *Layout) OpenFile(path string) {
	hadNoTabs := !l.editor.HasTabs()
	
	l.editor.OpenFile(path)
	
	if l.fileWatcher != nil {
		l.fileWatcher.AddOpenFile(path)
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
	}
	
	if hadNoTabs && l.editor.HasTabs() {
		l.rebuildLayout()
	}
	
	l.focusEditor()
}

func (l *Layout) setupFocusDetection() {
	l.stopFocus = make(chan bool)
}

func (l *Layout) checkFocusChange() {
	currentFocus := l.app.GetFocus()
	
	if currentFocus == l.lastFocus {
		return
	}
	
	l.lastFocus = currentFocus
	
	oldPanel := l.focusedPanel
	
	if l.fileManager.HasFocus(currentFocus) {
		l.focusedPanel = 0
	} else if l.editor.HasFocus(currentFocus) {
		l.focusedPanel = 1
	} else if l.terminal.HasFocus(currentFocus) {
		l.focusedPanel = 2
	}
	
	if oldPanel != l.focusedPanel {
		l.editor.UpdateTabBar()
		l.terminal.UpdateTabBar()
	}
}

func (l *Layout) setupFileWatcher() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	
	l.fileWatcher = NewFileWatcher(cwd)
	
	l.fileWatcher.SetOnChange(func(changes []FileChange) {
		l.app.QueueUpdateDraw(func() {
			l.handleFileChanges(changes)
		})
	})
	
	err = l.fileWatcher.Start()
	if err != nil {
		return
	}
	
	if l.fileWatcher != nil {
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
		
		l.stopTicker = make(chan bool)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-l.stopTicker:
					return
				case <-ticker.C:
					if l.fileWatcher != nil {
						count := l.fileWatcher.GetWatchedDirsCount()
						l.app.QueueUpdateDraw(func() {
							l.statusBar.SetWatchedDirs(count)
						})
					}
				}
			}
		}()
	}
}

func (l *Layout) handleFileChanges(changes []FileChange) {
	dirsToRefresh := make(map[string]bool)
	
	modifiedFiles := make(map[string]bool)
	
	gitHeadChanged := false
	
	for _, change := range changes {
		if strings.HasSuffix(change.Path, ".git/HEAD") {
			gitHeadChanged = true
			continue
		}
		
		dir := filepath.Dir(change.Path)
		dirsToRefresh[dir] = true
		
		switch change.Type {
		case FileModified:
			if !strings.HasSuffix(change.Path, "/") {
				modifiedFiles[change.Path] = true
			}
		case FileDeleted:
			l.editor.HandleFileDeleted(change.Path)
		}
	}
	
	if gitHeadChanged {
		l.statusBar.RefreshGitBranch()
	}
	
	for dir := range dirsToRefresh {
		l.fileManager.RefreshDirectory(dir)
	}
	
	for filePath := range modifiedFiles {
		l.editor.HandleFileModified(filePath)
	}
}

func (l *Layout) tryQuit() {
	hasUnsaved := l.editor.HasUnsavedChanges()
	
	hasTerminals := l.terminal.HasTabs()
	
	if hasUnsaved || hasTerminals {
		var messages []string
		if hasUnsaved {
			messages = append(messages, "You have unsaved changes in the editor.")
		}
		if hasTerminals {
			messages = append(messages, "You have open terminal sessions.")
		}
		messages = append(messages, "\n\nAre you sure you want to quit?")
		
		message := strings.Join(messages, " ")
		
		ShowConfirmModal(l.app, "Confirm Quit", message, 
			func() {
				l.app.Stop()
			}, 
			func() {
			})
	} else {
		l.app.Stop()
	}
}

// Stop shuts down the layout and cleans up resources.
func (l *Layout) Stop() {
	if l.fileWatcher != nil {
		l.fileWatcher.Stop()
	}
	if l.stopTicker != nil {
		close(l.stopTicker)
	}
	if l.stopFocus != nil {
		close(l.stopFocus)
	}
}