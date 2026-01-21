// Package ui provides the user interface components for the text editor.
package ui

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/git"
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

	// Project search
	projectSearchModal        *ProjectSearchModal
	projectSearchModalWrapper *modalWrapper
	projectSearchState        struct {
		searchTerm    string
		replaceTerm   string
		caseSensitive bool
		wholeWord     bool
		useRegex      bool
	}

	// Claude integration
	claudeStateDir    string
	claudeCmdWatcher  *fsnotify.Watcher
	stopClaudeWatcher chan bool

	// Dynamic sizing
	lastTerminalWidth int
	fileManagerWidth  int

	// Git repositories
	gitRepos []git.Repository
}

// NewLayout creates a new Layout with all UI components.
func NewLayout(app *tview.Application) *Layout {
	l := &Layout{
		app:              app,
		focusedPanel:     0,
		terminalPosition: "bottom",
		proportionIndex:  1,
		proportionDir:    1,
		fileManagerWidth: 30,
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
		l.writeClaudeState("")
	})

	l.terminal.SetOnLastTabClosed(func() {
		l.rebuildLayout()
		if l.editor.HasTabs() {
			l.focusEditor()
		} else {
			l.focusFileManager()
		}
	})

	l.terminal.SetOnEmedCommand(func(cmd string, args []string) {
		l.handleEmedCommand(cmd, args)
	})

	l.fileManager.SetFilePreviewHandler(func(path string) {
		l.OpenFilePreview(path)
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

	l.editor.SetOnActiveFileChanged(func(path string) {
		l.statusBar.UpdateGitBranchForFile(path)
		l.fileManager.RevealFile(path)
	})

	l.root = tview.NewFlex().SetDirection(tview.FlexRow)

	l.mainArea = tview.NewFlex().SetDirection(tview.FlexColumn)
	l.mainArea.SetBackgroundColor(tcell.ColorBlack)
	l.mainArea.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		l.checkAndUpdateWidth()
		return x, y, width, height
	})

	l.rightSide = tview.NewFlex()
	l.rightSide.SetBackgroundColor(tcell.ColorBlack)

	l.rebuildLayout()

	l.startClaudeWatcher()

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

	l.scanGitRepositories()

	l.setupFileWatcher()

	l.setupFocusDetection()

	go func() {
		time.Sleep(10 * time.Millisecond)
		l.app.QueueUpdateDraw(func() {
			if l.editor.HasTabs() {
				l.focusEditor()
			} else {
				l.focusFileManager()
			}
		})
	}()
}

// GetRoot returns the root primitive for the layout.
func (l *Layout) GetRoot() tview.Primitive {
	return l.root
}

func (l *Layout) calculateFileManagerWidth(terminalWidth int) int {
	if terminalWidth > 150 {
		return terminalWidth / 5
	}
	return 30
}

func (l *Layout) checkAndUpdateWidth() {
	_, _, width, _ := l.mainArea.GetRect()
	if width == l.lastTerminalWidth {
		return
	}
	l.lastTerminalWidth = width

	newWidth := l.calculateFileManagerWidth(width)
	if newWidth != l.fileManagerWidth {
		l.fileManagerWidth = newWidth
		l.rebuildLayout()
	}
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
		AddItem(l.fileManager.GetView(), l.fileManagerWidth, 0, false).
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
  Ctrl+I         File Info Dialog
  Ctrl+N         Create New File
  Ctrl+Alt+N     Create New Directory
  Ctrl+C         Copy File/Directory
  Ctrl+X         Cut File/Directory  
  Ctrl+V         Paste File/Directory
  Ctrl+D         Delete File/Directory
  Ctrl+R         Rename File/Directory
  
[blue]EDITOR:[-]
  Ctrl+S         Save File
  Ctrl+W         Close Current Tab
  Ctrl+F         Find/Replace in Document
  Ctrl+Z         Undo
  Ctrl+Y         Redo
  
[blue]NAVIGATION:[-]
  Ctrl+A         Focus File Manager
  Ctrl+E         Focus Editor
  Ctrl+T         Focus/Create Terminal
  F2             Cycle Focus Between Panels
  
[blue]TERMINAL:[-]
  Alt+T          Toggle Terminal Mode (Passthrough/Application)
  
[blue]TABS:[-]
  Ctrl+Shift+←   Previous Tab (or F3, Ctrl+Alt+←)
  Ctrl+Shift+→   Next Tab (or F4, Ctrl+Alt+→)
  F3             Previous Tab
  F4             Next Tab
  
[blue]LAYOUT:[-]
  Ctrl+L         Cycle Proportions (or F7)
  Ctrl+P         Toggle Terminal Position (or F6)
  F6             Toggle Terminal Position
  F7             Cycle Proportions
  
[blue]GIT:[-]
  Ctrl+G         Open Lazygit (requires lazygit installed)
  Ctrl+B         Show Git Blame for Current Line (in editor)

[blue]GENERAL:[-]
  F1             Show This Help
  F8             Show Performance Stats
  Ctrl+Q         Quit

[blue]FILE MANAGEMENT:[-]
When the file manager has focus, you can manage your files easily. Use arrow keys to navegate through your project tree. Press Ctrl+N to create new files right were you need them. Copy files with Ctrl+C, then move to another folder and paste with Ctrl+V. Delete unwanted files with Ctrl+D - but be carefull, there's no undo for this! Rename files with Ctrl+R and use Ctrl+X to cut (move) files in combination with Ctrl+V. Type to highlight the file that matches your input.

[blue]TERMINAL PASSTHROUGH MODE:[-]
The terminal has two modes. Normal mode lets you use keyboard shortcuts normaly. But sometimes you need to send special keys directly to the terminal program (like when using vim or other CLI tools). Press Alt+T to toggle passthrough mode. In this mode, all keystrokes go straight to the terminal - only Alt+T works to switch back. The terminal tab title shows when passthru mode is active for that particular tab with an asterisk.

[::d]Press ESC or click outside to close[-::-]`
}

func (l *Layout) showHelp() {
	ShowHelpModal(l.app, l.getHelpText())
}

func (l *Layout) showBlameModal() {
	filePath, lineNum := l.editor.GetCurrentFileAndLine()
	if filePath == "" || lineNum == 0 {
		ShowAlertModal(l.app, "Info", "No file open")
		return
	}

	blameInfo, err := git.GetBlameForLine(filePath, lineNum)
	if err != nil {
		ShowAlertModal(l.app, "Info", "No git blame data")
		return
	}

	repoRoot := git.FindRepoRoot(filePath)
	repoName := ""
	if repoRoot != "" {
		repoName = filepath.Base(repoRoot)
	}

	blameText := git.FormatDetailedBlameInfo(blameInfo, filePath, lineNum, repoName)
	ShowBlameModalWithCommit(l.app, blameText, blameInfo.Hash, filePath)
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
		case tcell.KeyF8:
			ShowMemStatsModal(l.app, l.fileWatcher)
			return nil
		}

		if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyRight {
			if event.Modifiers()&tcell.ModCtrl != 0 &&
				event.Modifiers()&(tcell.ModShift|tcell.ModAlt|tcell.ModMeta) != 0 {
				if event.Key() == tcell.KeyLeft {
					if l.focusedPanel == 1 {
						l.editor.PreviousTab()
					} else if l.focusedPanel == 2 {
						l.terminal.PreviousTab()
					}
					return nil
				}
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
					l.terminal.NewTab(l.getTerminalWorkingDir())
				} else {
					if !l.terminal.HasTabs() {
						l.terminal.NewTab(l.getTerminalWorkingDir())
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
					l.showProjectSearch()
				} else if l.focusedPanel == 1 {
					l.editor.ShowSearch()
				}
				return nil
			case tcell.KeyCtrlB:
				if l.focusedPanel == 1 {
					l.showBlameModal()
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
			case tcell.KeyCtrlG:
				l.showLazygit()
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
	l.writeClaudeState("")
}

// OpenFilePreview opens a file in preview mode without moving focus from file manager.
func (l *Layout) OpenFilePreview(path string) {
	hadNoTabs := !l.editor.HasTabs()

	convertedToRegular := l.editor.OpenFilePreview(path)

	if l.fileWatcher != nil {
		l.fileWatcher.AddOpenFile(path)
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
	}

	if hadNoTabs && l.editor.HasTabs() {
		l.rebuildLayout()
	}

	if convertedToRegular {
		l.focusEditor()
	}

	l.editor.UpdateTabBar()
	l.writeClaudeState("")
}

func (l *Layout) openFileWithoutFocus(path string) {
	hadNoTabs := !l.editor.HasTabs()

	l.editor.OpenFile(path)

	if l.fileWatcher != nil {
		l.fileWatcher.AddOpenFile(path)
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
	}

	if hadNoTabs && l.editor.HasTabs() {
		l.rebuildLayout()
	}

	l.writeClaudeState("")
}

// OpenFileAtLine opens a file at a specific line and column
func (l *Layout) OpenFileAtLine(path string, line int, column int) {
	hadNoTabs := !l.editor.HasTabs()

	l.editor.OpenFileAtLine(path, line, column)

	if l.fileWatcher != nil {
		l.fileWatcher.AddOpenFile(path)
		l.statusBar.SetWatchedDirs(l.fileWatcher.GetWatchedDirsCount())
	}

	if hadNoTabs && l.editor.HasTabs() {
		l.rebuildLayout()
	}

	l.focusEditor()
	l.writeClaudeState("")
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

	deletedFiles := make(map[string]bool)

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
			deletedFiles[change.Path] = true
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

	for filePath := range deletedFiles {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			l.editor.HandleFileDeleted(filePath)
		} else {
			l.editor.HandleFileModified(filePath)
		}
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

// showProjectSearch displays the project search modal
func (l *Layout) showProjectSearch() {
	if l.projectSearchModal != nil {
		l.projectSearchModal.Stop()
		l.projectSearchModal = nil
	}

	l.projectSearchModal = NewProjectSearchModal(l.app)

	l.projectSearchModal.SetCallbacks(
		func(path string, line int, column int) {
			if l.projectSearchModal != nil {
				l.projectSearchModal.Stop()
			}

			if l.projectSearchModalWrapper != nil {
				l.projectSearchModalWrapper.Close()
				l.projectSearchModalWrapper = nil
			}

			absPath, err := filepath.Abs(path)
			if err == nil {
				l.OpenFileAtLine(absPath, line, column)
			}
		},
		func() map[string]*EditorTab {
			return l.editor.GetOpenTabs()
		},
		func() {
			if l.projectSearchModal != nil {
				search, replace, caseSens, wholeWord, useRegex := l.projectSearchModal.GetState()
				l.projectSearchState.searchTerm = search
				l.projectSearchState.replaceTerm = replace
				l.projectSearchState.caseSensitive = caseSens
				l.projectSearchState.wholeWord = wholeWord
				l.projectSearchState.useRegex = useRegex

				l.projectSearchModal.Stop()
			}

			if l.projectSearchModalWrapper != nil {
				l.projectSearchModalWrapper.Close()
				l.projectSearchModalWrapper = nil
			}

			l.projectSearchModal = nil
		},
	)

	l.projectSearchModal.RestoreState(
		l.projectSearchState.searchTerm,
		l.projectSearchState.replaceTerm,
		l.projectSearchState.caseSensitive,
		l.projectSearchState.wholeWord,
		l.projectSearchState.useRegex,
	)

	l.projectSearchModal.ActivateHandlers()

	_, _, termWidth, termHeight := l.mainArea.GetRect()
	modalWidth := 80
	modalHeight := 25
	if termWidth > 190 {
		modalWidth = termWidth * 60 / 100
		modalHeight = termHeight * 60 / 100
		if modalHeight < 25 {
			modalHeight = 25
		}
	}

	config := EmModalConfig{
		UseFixedSize: true,
		FixedWidth:   modalWidth,
		FixedHeight:  modalHeight,
		BorderColor:  tcell.NewRGBColor(102, 102, 102),
		TitleColor:   tcell.ColorWhite,
		Title:        " Project Search & Replace ",
		TitleAlign:   tview.AlignCenter,
	}

	var content *tview.Flex
	l.projectSearchModalWrapper, content = NewConfiguredModal(l.app, config, true)
	l.projectSearchModalWrapper.SetPreviousRoot(l.root)

	content.AddItem(l.projectSearchModal.GetContainer(), 0, 1, true)

	l.projectSearchModalWrapper.Show()
	l.projectSearchModal.Focus()
}

func (l *Layout) scanGitRepositories() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	l.gitRepos = git.ScanRepositories(cwd)
}

func (l *Layout) getCurrentRepo() *git.Repository {
	if len(l.gitRepos) == 0 {
		return nil
	}

	// Priority 1: Terminal CWD (if terminal is focused)
	if l.focusedPanel == 2 && l.terminal.HasTabs() {
		// GetTerminalCWD is a best-effort function, may return empty
		if termCwd := GetTerminalCWD(l.terminal.GetCurrentPID()); termCwd != "" {
			if repo := git.FindRepoForPath(termCwd, l.gitRepos); repo != nil {
				return repo
			}
		}
	}

	// Priority 2: Active editor file
	if activePath := l.editor.GetActiveFilePath(); activePath != "" {
		if repo := git.FindRepoForPath(activePath, l.gitRepos); repo != nil {
			return repo
		}
	}

	// Priority 3: File manager highlighted file
	if fmPath := l.fileManager.GetHighlightedPath(); fmPath != "" {
		if repo := git.FindRepoForPath(fmPath, l.gitRepos); repo != nil {
			return repo
		}
	}

	// Fallback: first repo
	return &l.gitRepos[0]
}

func (l *Layout) getTerminalWorkingDir() string {
	// Priority 1: Parent directory of the file currently open in the editor
	if activePath := l.editor.GetActiveFilePath(); activePath != "" {
		return filepath.Dir(activePath)
	}

	// Priority 2: Parent directory of the file highlighted in the file manager
	if fmPath := l.fileManager.GetHighlightedPath(); fmPath != "" {
		info, err := os.Stat(fmPath)
		if err == nil {
			if info.IsDir() {
				return fmPath
			}
			return filepath.Dir(fmPath)
		}
	}

	// Fallback: empty string (PTY will use project working directory)
	return ""
}

func (l *Layout) showLazygit() {
	if !git.IsLazygitInstalled() {
		ShowLazygitNotInstalledModal(l.app)
		return
	}

	if len(l.gitRepos) == 0 {
		ShowNoReposModal(l.app)
		return
	}

	currentRepo := l.getCurrentRepo()

	if len(l.gitRepos) == 1 {
		modal := NewLazygitModal(l.app, l.gitRepos[0].Path, "")
		modal.Show(nil)
		return
	}

	selector := NewRepoSelectorModal(l.app, l.gitRepos, currentRepo)
	selector.Show(func(repo git.Repository) {
		modal := NewLazygitModal(l.app, repo.Path, "")
		modal.Show(nil)
	}, nil)
}

// Stop shuts down the layout and cleans up resources.
func (l *Layout) Stop() {
	l.writeClaudeState("")
	if l.fileWatcher != nil {
		l.fileWatcher.Stop()
	}
	if l.stopTicker != nil {
		close(l.stopTicker)
	}
	if l.stopFocus != nil {
		close(l.stopFocus)
	}
	if l.stopClaudeWatcher != nil {
		close(l.stopClaudeWatcher)
	}
	if l.claudeCmdWatcher != nil {
		l.claudeCmdWatcher.Close()
	}
}

func (l *Layout) restoreFromStateFile() {
	stateFile := filepath.Join(l.claudeStateDir, "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	stateInner, ok := state["state"].(map[string]interface{})
	if !ok {
		return
	}

	openFilesRaw, ok := stateInner["openFiles"].([]interface{})
	if !ok {
		return
	}

	for _, f := range openFilesRaw {
		if filePath, ok := f.(string); ok {
			if _, err := os.Stat(filePath); err == nil {
				l.editor.OpenFile(filePath)
			}
		}
	}

	if activeFile, ok := stateInner["activeFile"].(string); ok && activeFile != "" {
		l.editor.OpenFile(activeFile)
	}

	if l.editor.HasTabs() {
		l.rebuildLayout()
	}
}

func (l *Layout) handleEmedCommand(cmd string, args []string) {
	if l.app == nil {
		return
	}

	l.app.QueueUpdateDraw(func() {
		switch cmd {
		case "open":
			l.handleEmedOpen(args)
		case "sync":
			l.handleEmedSync()
		}
	})
}

func (l *Layout) handleEmedOpen(args []string) {
	if len(args) < 1 {
		return
	}

	filePath := args[0]
	if !filepath.IsAbs(filePath) {
		cwd, _ := os.Getwd()
		filePath = filepath.Join(cwd, filePath)
	}

	line := 1
	if len(args) >= 2 {
		if n, err := strconv.Atoi(args[1]); err == nil && n > 0 {
			line = n
		}
	}

	l.OpenFile(filePath)
	l.focusEditor()

	if line > 1 {
		l.editor.GoToLine(line)
	}
}

func (l *Layout) handleEmedSync() {
	l.writeClaudeState("")
}

func (l *Layout) writeClaudeState(lastCmdOutput string) {
	if l.claudeStateDir == "" {
		return
	}

	terminalTabs := 0
	if l.terminal != nil && l.terminal.HasTabs() {
		terminalTabs = len(l.terminal.tabManager.GetTabs())
	}

	focusedPanel := "file_manager"
	if l.focusedPanel == 1 {
		focusedPanel = "editor"
	} else if l.focusedPanel == 2 {
		focusedPanel = "terminal"
	}

	state := map[string]interface{}{
		"state": map[string]interface{}{
			"openFiles":     l.editor.GetOpenFilePaths(),
			"activeFile":    l.editor.GetActiveFilePath(),
			"cursorLine":    l.editor.GetActiveCursorLine(),
			"modifiedFiles": l.editor.GetModifiedFiles(),
			"tabCount":      l.editor.GetTabCount(),
			"terminalTabs":  terminalTabs,
			"focusedPanel":  focusedPanel,
		},
		"commands": map[string]interface{}{
			"init": map[string]interface{}{
				"description": "Initialize communication with emed. Run this first to get state and command list.",
				"params":      map[string]string{},
				"useCases": []string{
					"First command to run when starting inside emed",
					"Discover available commands and current state",
				},
				"example": `{"cmd":"init"}`,
			},
			"open": map[string]interface{}{
				"description": "Open a file in the editor. If the file is already open, switches to that tab.",
				"params": map[string]string{
					"file":   "(required) Path to the file, relative or absolute",
					"line":   "(optional) Line number to jump to (1-based)",
					"column": "(optional) Column number to jump to (1-based)",
				},
				"useCases": []string{
					"Open a file you need to review or edit",
					"Jump to a specific error location",
					"Navigate to a function definition",
				},
				"example": `{"cmd":"open","file":"main.go","line":42,"column":10}`,
			},
			"openMultiple": map[string]interface{}{
				"description": "Open multiple files at once. Avoid opening more than 5 files at once to keep the UI responsive.",
				"params": map[string]string{
					"files": "(required) Array of file paths to open",
				},
				"useCases": []string{
					"Open related files for a feature you're working on",
					"Open test file alongside implementation",
					"Review multiple files at once",
				},
				"example": `{"cmd":"openMultiple","files":["main.go","config.go","utils.go"]}`,
			},
			"close": map[string]interface{}{
				"description": "Close a file tab. Does not prompt for save, use 'save' first if needed.",
				"params": map[string]string{
					"file": "(required) Path to the file to close",
				},
				"useCases": []string{
					"Close a file after you're done reviewing it",
					"Clean up editor tabs to reduce clutter",
				},
				"example": `{"cmd":"close","file":"main.go"}`,
			},
			"save": map[string]interface{}{
				"description": "Save a file. If no file specified, saves the currently active file.",
				"params": map[string]string{
					"file": "(optional) Path to the file to save. If omited, saves the active file.",
				},
				"useCases": []string{
					"Save changes after editing a file",
					"Persist auto-generated code before running tests",
				},
				"example": `{"cmd":"save","file":"main.go"}`,
			},
			"saveAll": map[string]interface{}{
				"description": "Save all modified files at once.",
				"params":      map[string]string{},
				"useCases": []string{
					"Save all changes before running build or tests",
					"Quick save everything before switching tasks",
				},
				"example": `{"cmd":"saveAll"}`,
			},
			"getContent": map[string]interface{}{
				"description": "Get the current buffer content of a file (includes unsaved changes).",
				"params": map[string]string{
					"file": "(optional) Path to the file. If omited, returns content of active file.",
				},
				"useCases": []string{
					"Read the current state of a file with unsaved edits",
					"Get file content without re-reading from disk",
					"Inspect what the user has changed",
				},
				"example": `{"cmd":"getContent","file":"main.go"}`,
			},
			"getSelection": map[string]interface{}{
				"description": "Get the currently selected text in the active editor.",
				"params":      map[string]string{},
				"useCases": []string{
					"Get text the user has highlighted for context",
					"Process user-selected code snippet",
					"Apply operations to selected region only",
				},
				"example": `{"cmd":"getSelection"}`,
			},
			"getDiff": map[string]interface{}{
				"description": "Get list of files with unsaved modifications.",
				"params":      map[string]string{},
				"useCases": []string{
					"Check what files have unsaved changes",
					"Verify changes before commit",
				},
				"example": `{"cmd":"getDiff"}`,
			},
			"goto": map[string]interface{}{
				"description": "Move cursor to a specific line and column. Can optionaly open a file first.",
				"params": map[string]string{
					"file":   "(optional) Path to open before jumping",
					"line":   "(required) Line number to go to (1-based)",
					"column": "(optional) Column number to go to (1-based)",
				},
				"useCases": []string{
					"Navigate to a specific location in the code",
					"Jump to an error or warning location",
					"Move to a function or variable definition",
				},
				"example": `{"cmd":"goto","file":"main.go","line":100,"column":5}`,
			},
			"focus": map[string]interface{}{
				"description": "Focus the editor panel.",
				"params":      map[string]string{},
				"useCases": []string{
					"Switch focus from terminal to editor",
					"Return focus to editor after file operations",
				},
				"example": `{"cmd":"focus"}`,
			},
			"focusTerminal": map[string]interface{}{
				"description": "Focus the terminal panel. Creates a new terminal if none exists.",
				"params":      map[string]string{},
				"useCases": []string{
					"Switch to terminal to run commands",
					"Ensure terminal is visible and focused",
				},
				"example": `{"cmd":"focusTerminal"}`,
			},
			"focusFileManager": map[string]interface{}{
				"description": "Focus the file manager panel.",
				"params":      map[string]string{},
				"useCases": []string{
					"Navigate the file tree",
					"Browse project structure",
				},
				"example": `{"cmd":"focusFileManager"}`,
			},
			"newTerminal": map[string]interface{}{
				"description": "Create a new terminal tab and focus it.",
				"params":      map[string]string{},
				"useCases": []string{
					"Open additional terminal for parallel tasks",
					"Create a fresh terminal environment",
				},
				"example": `{"cmd":"newTerminal"}`,
			},
			"listModified": map[string]interface{}{
				"description": "Get a list of files with unsaved changes.",
				"params":      map[string]string{},
				"useCases": []string{
					"Check which files need saving",
					"Verify pending changes before operations",
				},
				"example": `{"cmd":"listModified"}`,
			},
			"status": map[string]interface{}{
				"description": "Get detailed editor status as JSON including all state information.",
				"params":      map[string]string{},
				"useCases": []string{
					"Get comprehensive editor state in one call",
					"Debug or inspect current editor status",
					"Monitor editor state programmatically",
				},
				"example": `{"cmd":"status"}`,
			},
		},
		"lastCommandOutput": lastCmdOutput,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}

	syncFile := filepath.Join(l.claudeStateDir, "state.json")
	os.WriteFile(syncFile, data, 0644)
}

func (l *Layout) startClaudeWatcher() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	hash := md5.Sum([]byte(cwd))
	l.claudeStateDir = filepath.Join(homeDir, ".emed", hex.EncodeToString(hash[:])[:12])
	os.MkdirAll(l.claudeStateDir, 0755)

	l.restoreFromStateFile()
	os.Remove(filepath.Join(l.claudeStateDir, "state.json"))

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	l.claudeCmdWatcher = watcher
	l.stopClaudeWatcher = make(chan bool)

	if err := watcher.Add(l.claudeStateDir); err != nil {
		watcher.Close()
		return
	}

	go l.claudeWatcherLoop()
}

func (l *Layout) claudeWatcherLoop() {
	cmdFile := filepath.Join(l.claudeStateDir, "command.json")
	for {
		select {
		case <-l.stopClaudeWatcher:
			return
		case event, ok := <-l.claudeCmdWatcher.Events:
			if !ok {
				return
			}
			if (event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write) && event.Name == cmdFile {
				time.Sleep(10 * time.Millisecond) // Let file finish writing
				l.processClaudeCommand(cmdFile)
			}
		case _, ok := <-l.claudeCmdWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (l *Layout) processClaudeCommand(cmdFile string) {
	data, err := os.ReadFile(cmdFile)
	if err != nil {
		return
	}

	os.Remove(cmdFile)

	var cmd struct {
		Cmd    string   `json:"cmd"`
		File   string   `json:"file"`
		Files  []string `json:"files"`
		Line   int      `json:"line"`
		Column int      `json:"column"`
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		l.writeClaudeState("error: invalid JSON")
		return
	}

	if l.app != nil {
		l.app.QueueUpdateDraw(func() {
			output := l.executeClaudeCommand(cmd.Cmd, cmd.File, cmd.Files, cmd.Line, cmd.Column)
			l.writeClaudeState(output)
		})
	}
}

func (l *Layout) executeClaudeCommand(cmd, file string, files []string, line, column int) string {
	switch cmd {
	case "open":
		if file == "" {
			return "error: file path required"
		}
		if !filepath.IsAbs(file) {
			cwd, _ := os.Getwd()
			file = filepath.Join(cwd, file)
		}
		l.openFileWithoutFocus(file)
		if line > 0 {
			if column > 0 {
				l.editor.GoToPosition(line, column)
			} else {
				l.editor.GoToLine(line)
			}
		}
		return "ok: opened " + file

	case "openMultiple":
		if len(files) == 0 {
			return "error: files array required"
		}
		cwd, _ := os.Getwd()
		opened := 0
		for _, f := range files {
			if f == "" {
				continue
			}
			path := f
			if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, path)
			}
			l.openFileWithoutFocus(path)
			opened++
		}
		return "ok: opened " + strconv.Itoa(opened) + " files"

	case "close":
		if file == "" {
			return "error: file path required"
		}
		if !filepath.IsAbs(file) {
			cwd, _ := os.Getwd()
			file = filepath.Join(cwd, file)
		}
		if l.editor.CloseTabByPath(file) {
			l.rebuildLayout()
			return "ok: closed " + file
		}
		return "error: file not open"

	case "save":
		if file == "" {
			l.editor.SaveCurrentFile()
			return "ok: saved current file"
		}
		if !filepath.IsAbs(file) {
			cwd, _ := os.Getwd()
			file = filepath.Join(cwd, file)
		}
		if l.editor.SaveFileByPath(file) {
			return "ok: saved " + file
		}
		return "error: file not open or save failed"

	case "saveAll":
		count := l.editor.SaveAllModifiedFiles()
		return "ok: saved " + strconv.Itoa(count) + " files"

	case "getContent":
		if file == "" {
			content := l.editor.GetCurrentBufferContent()
			if content == "" {
				return "error: no file open"
			}
			return content
		}
		if !filepath.IsAbs(file) {
			cwd, _ := os.Getwd()
			file = filepath.Join(cwd, file)
		}
		content := l.editor.GetBufferContent(file)
		if content == "" {
			return "error: file not open"
		}
		return content

	case "getSelection":
		selection := l.editor.GetSelection()
		if selection == "" {
			return "error: no selection"
		}
		return selection

	case "getDiff":
		modified := l.editor.GetModifiedFiles()
		if len(modified) == 0 {
			return "no modified files"
		}
		var diffs []string
		for _, path := range modified {
			diffs = append(diffs, "modified: "+path)
		}
		return strings.Join(diffs, "\n")

	case "goto":
		if line <= 0 {
			return "error: line number required"
		}
		if file != "" {
			if !filepath.IsAbs(file) {
				cwd, _ := os.Getwd()
				file = filepath.Join(cwd, file)
			}
			l.OpenFile(file)
		}
		if column > 0 {
			l.editor.GoToPosition(line, column)
		} else {
			l.editor.GoToLine(line)
		}
		return "ok: moved to line " + strconv.Itoa(line)

	case "focus":
		l.focusEditor()
		return "ok: focused editor"

	case "focusTerminal":
		if !l.terminal.HasTabs() {
			l.terminal.NewTab(l.getTerminalWorkingDir())
			l.rebuildLayout()
		}
		l.focusTerminal()
		return "ok: focused terminal"

	case "focusFileManager":
		l.focusFileManager()
		return "ok: focused file manager"

	case "newTerminal":
		hadNoTabs := !l.terminal.HasTabs()
		l.terminal.NewTab(l.getTerminalWorkingDir())
		if hadNoTabs {
			l.rebuildLayout()
		}
		l.focusTerminal()
		return "ok: created new terminal"

	case "listModified":
		modified := l.editor.GetModifiedFiles()
		if len(modified) == 0 {
			return "no modified files"
		}
		return strings.Join(modified, "\n")

	case "status":
		return l.getEditorStatus()

	case "init":
		return "ok: initialized"

	default:
		return "error: unknown command " + cmd
	}
}

func (l *Layout) getEditorStatus() string {
	openFiles := l.editor.GetOpenFilePaths()
	activeFile := l.editor.GetActiveFilePath()
	cursorLine := l.editor.GetActiveCursorLine()
	modifiedFiles := l.editor.GetModifiedFiles()
	tabCount := l.editor.GetTabCount()

	terminalTabs := 0
	if l.terminal != nil && l.terminal.HasTabs() {
		terminalTabs = len(l.terminal.tabManager.GetTabs())
	}

	focusedPanel := "file_manager"
	if l.focusedPanel == 1 {
		focusedPanel = "editor"
	} else if l.focusedPanel == 2 {
		focusedPanel = "terminal"
	}

	status := map[string]interface{}{
		"openFiles":     openFiles,
		"activeFile":    activeFile,
		"cursorLine":    cursorLine,
		"modifiedFiles": modifiedFiles,
		"tabCount":      tabCount,
		"terminalTabs":  terminalTabs,
		"focusedPanel":  focusedPanel,
	}

	data, _ := json.Marshal(status)
	return string(data)
}
