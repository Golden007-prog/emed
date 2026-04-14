// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/git"
	"github.com/laborin/emed/internal/terminal/pty"
	"github.com/laborin/emed/internal/terminal/scrollback"
	"github.com/laborin/vt10x"
	"github.com/rivo/tview"
)

// LazygitModal wraps a terminal running lazygit in a modal dialog.
type LazygitModal struct {
	terminal   *VT10XTerminal
	modal      *modalWrapper
	app        *tview.Application
	commitHash string
}

// NewLazygitModal creates a new lazygit modal for the given repository.
// If commitHash is provided, lazygit will navigate to the commits panel and search for that commit.
func NewLazygitModal(app *tview.Application, repoPath string, commitHash string) *LazygitModal {
	cmd := []string{"lazygit"}

	ptyMgr := pty.NewWithCommand(cmd, repoPath)

	term := &VT10XTerminal{
		Box:       tview.NewBox(),
		ptyMgr:    ptyMgr,
		scrollbuf: scrollback.New(1000),
		mode:      ModePassthrough,
		rows:      24,
		cols:      80,
		done:      make(chan struct{}),
		writeChan: make(chan []byte, 1000),
	}
	term.Box.Primitive = term

	term.vt = vt10x.New(vt10x.WithSize(term.cols, term.rows))
	ptyMgr.SetOnData(term.onPTYData)

	go term.writeProcessor()
	go term.refreshLoop()

	return &LazygitModal{
		terminal:   term,
		app:        app,
		commitHash: commitHash,
	}
}

// Show displays the lazygit modal.
func (l *LazygitModal) Show(onClose func()) {
	config := EmModalConfig{
		Width:          90,
		Height:         90,
		BorderColor:    tcell.NewRGBColor(102, 102, 102),
		TitleColor:     tcell.ColorWhite,
		Title:          " Lazygit ",
		TitleAlign:     tview.AlignCenter,
		EscPassthrough: true,
	}

	var content *tview.Flex
	l.modal, content = NewConfiguredModal(l.app, config, true)
	l.modal.SetPreviousRoot(mainAppRoot)

	termContainer := tview.NewBox().SetBackgroundColor(tcell.ColorBlack)
	termContainer.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		l.terminal.SetRect(x, y, width, height)
		l.terminal.Draw(screen)
		return x, y, width, height
	})

	content.AddItem(termContainer, 0, 1, true)

	l.terminal.SetApp(l.app)

	l.terminal.ptyMgr.SetOnExit(func() {
		l.app.QueueUpdateDraw(func() {
			l.terminal.Stop()
			l.Close()
			if onClose != nil {
				onClose()
			}
		})
	})

	l.modal.modal.SetOnCancel(func() {
		l.Stop()
		if onClose != nil {
			onClose()
		}
	})

	l.modal.modal.SetOnClose(func() {
		l.Stop()
	})

	l.modal.modal.content = &lazygitModalContent{
		Primitive: l.modal.modal.content,
		terminal:  l.terminal,
	}

	if err := l.terminal.Start(); err != nil {
		ShowAlertModal(l.app, "Error", "Failed to start lazygit: "+err.Error())
		return
	}

	l.terminal.hasFocus = true
	l.modal.Show()
	l.app.SetFocus(l.terminal)

	if l.commitHash != "" {
		go l.navigateToCommit()
	}
}

// navigateToCommit sends keystrokes to lazygit to navigate to a specific commit.
func (l *LazygitModal) navigateToCommit() {
	// Wait for lazygit to fully initialize
	time.Sleep(800 * time.Millisecond)

	if !l.terminal.ptyMgr.IsRunning() {
		return
	}

	// Press '4' to focus the commits panel
	l.terminal.ptyMgr.Write([]byte("4"))
	time.Sleep(150 * time.Millisecond)

	if !l.terminal.ptyMgr.IsRunning() {
		return
	}

	// Press '/' to open the search/filter prompt
	l.terminal.ptyMgr.Write([]byte("/"))
	time.Sleep(150 * time.Millisecond)

	if !l.terminal.ptyMgr.IsRunning() {
		return
	}

	// Type the commit hash
	l.terminal.ptyMgr.Write([]byte(l.commitHash))
	time.Sleep(100 * time.Millisecond)

	if !l.terminal.ptyMgr.IsRunning() {
		return
	}

	// Press Enter to confirm the search
	l.terminal.ptyMgr.Write([]byte("\r"))
}

// Close closes the modal without stopping the terminal (for process exit).
func (l *LazygitModal) Close() {
	if l.modal != nil {
		l.modal.Close()
	}
}

// Stop terminates the lazygit process and closes the modal.
func (l *LazygitModal) Stop() {
	if l.terminal != nil {
		l.terminal.Stop()
	}
}

type lazygitModalContent struct {
	tview.Primitive
	terminal *VT10XTerminal
}

func (c *lazygitModalContent) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if handler := c.terminal.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	}
}

func (c *lazygitModalContent) Focus(delegate func(p tview.Primitive)) {
	c.terminal.hasFocus = true
	delegate(c.terminal)
}

func (c *lazygitModalContent) HasFocus() bool {
	return c.terminal.hasFocus
}

func (c *lazygitModalContent) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if handler := c.terminal.MouseHandler(); handler != nil {
			return handler(action, event, setFocus)
		}
		return false, nil
	}
}

// RepoSelectorModal displays a list of repositories for selection.
type RepoSelectorModal struct {
	list          *tview.List
	searchInput   *tview.InputField
	modal         *modalWrapper
	app           *tview.Application
	repos         []git.Repository
	filteredRepos []git.Repository
	currentRepo   *git.Repository
	onSelect      func(repo git.Repository)
}

// NewRepoSelectorModal creates a new repository selector modal.
func NewRepoSelectorModal(app *tview.Application, repos []git.Repository, currentRepo *git.Repository) *RepoSelectorModal {
	list := tview.NewList().
		ShowSecondaryText(true).
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.ColorDarkBlue).
		SetSelectedTextColor(tcell.ColorWhite).
		SetSecondaryTextColor(tcell.ColorGray)

	searchInput := tview.NewInputField().
		SetLabel(" Filter: ").
		SetFieldBackgroundColor(tcell.ColorBlack).
		SetFieldTextColor(tcell.ColorWhite).
		SetLabelColor(tcell.ColorGray)

	r := &RepoSelectorModal{
		list:          list,
		searchInput:   searchInput,
		app:           app,
		repos:         repos,
		filteredRepos: repos,
		currentRepo:   currentRepo,
	}

	r.updateList()

	return r
}

func (r *RepoSelectorModal) updateList() {
	r.list.Clear()

	selectedIndex := 0
	for i, repo := range r.filteredRepos {
		displayName := repo.Name
		r.list.AddItem(displayName, repo.Path, 0, nil)
		if r.currentRepo != nil && repo.Path == r.currentRepo.Path {
			selectedIndex = i
		}
	}

	if len(r.filteredRepos) > 0 {
		r.list.SetCurrentItem(selectedIndex)
	}
}

func (r *RepoSelectorModal) filterRepos(query string) {
	if query == "" {
		r.filteredRepos = r.repos
	} else {
		query = strings.ToLower(query)
		r.filteredRepos = nil
		for _, repo := range r.repos {
			if strings.Contains(strings.ToLower(repo.Name), query) ||
				strings.Contains(strings.ToLower(repo.Path), query) {
				r.filteredRepos = append(r.filteredRepos, repo)
			}
		}
	}
	r.updateList()
}

// Show displays the repository selector modal.
func (r *RepoSelectorModal) Show(onSelect func(repo git.Repository), onCancel func()) {
	r.onSelect = onSelect

	_, _, termWidth, termHeight := GetMainAppRoot().(*MouseFocusDetector).GetRect()
	modalWidth := termWidth * 60 / 100
	modalHeight := termHeight * 50 / 100

	if modalWidth < 60 {
		modalWidth = 60
	}
	if modalWidth > 120 {
		modalWidth = 120
	}
	if modalHeight < 10 {
		modalHeight = 10
	}
	if modalHeight > 25 {
		modalHeight = 25
	}

	config := EmModalConfig{
		UseFixedSize: true,
		FixedWidth:   modalWidth,
		FixedHeight:  modalHeight,
		BorderColor:  tcell.NewRGBColor(102, 102, 102),
		TitleColor:   tcell.ColorWhite,
		Title:        " Select Repository ",
		TitleAlign:   tview.AlignCenter,
	}

	var content *tview.Flex
	r.modal, content = NewConfiguredModal(r.app, config, true)
	r.modal.SetPreviousRoot(mainAppRoot)

	r.searchInput.SetChangedFunc(func(text string) {
		r.filterRepos(text)
	})

	r.list.SetSelectedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(r.filteredRepos) {
			r.modal.Close()
			if r.onSelect != nil {
				r.onSelect(r.filteredRepos[index])
			}
		}
	})

	r.list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			r.searchInput.SetText(string(event.Rune()))
			r.app.SetFocus(r.searchInput)
			return nil
		}
		if event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2 {
			text := r.searchInput.GetText()
			if len(text) > 0 {
				r.searchInput.SetText(text[:len(text)-1])
			}
			return nil
		}
		return event
	})

	r.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown, tcell.KeyUp, tcell.KeyEnter:
			r.app.SetFocus(r.list)
			if event.Key() == tcell.KeyEnter && len(r.filteredRepos) > 0 {
				idx := r.list.GetCurrentItem()
				if idx >= 0 && idx < len(r.filteredRepos) {
					r.modal.Close()
					if r.onSelect != nil {
						r.onSelect(r.filteredRepos[idx])
					}
				}
			}
			return nil
		case tcell.KeyEsc:
			if r.searchInput.GetText() != "" {
				r.searchInput.SetText("")
				r.filterRepos("")
				return nil
			}
		}
		return event
	})

	r.modal.modal.SetOnCancel(onCancel)

	content.AddItem(r.searchInput, 1, 0, false)
	content.AddItem(r.list, 0, 1, true)

	r.modal.Show()
	r.app.SetFocus(r.list)
}

// Close closes the modal.
func (r *RepoSelectorModal) Close() {
	if r.modal != nil {
		r.modal.Close()
	}
}

// ShowLazygitNotInstalledModal displays an alert when lazygit is not found.
func ShowLazygitNotInstalledModal(app *tview.Application) {
	ShowAlertModal(app, "Lazygit Not Found", "Please install lazygit first.\nhttps://github.com/jesseduffield/lazygit")
}

// ShowNoReposModal displays an alert when no repositories are found.
func ShowNoReposModal(app *tview.Application) {
	ShowAlertModal(app, "Info", "No git repositories found")
}

// GetTerminalCWD attempts to get the current working directory of a terminal.
func GetTerminalCWD(pid int) string {
	if pid <= 0 {
		return ""
	}

	// On macOS, use lsof to get the cwd
	// On Linux, could use /proc/<pid>/cwd
	// For now, return empty - this is a best-effort feature
	return ""
}

// showBlameCommitInLazygit opens lazygit to show a specific commit.
func showBlameCommitInLazygit(app *tview.Application, filePath string, commitHash string) {
	repoRoot := git.FindRepoRoot(filePath)
	if repoRoot == "" {
		ShowAlertModal(app, "Error", "Could not find git repository")
		return
	}

	modal := NewLazygitModal(app, repoRoot, commitHash)
	modal.Show(nil)
}

// UpdateBlameModalWithShowCommit is called to create a blame modal with the show commit button.
func ShowBlameModalWithCommit(app *tview.Application, blameText string, commitHash string, filePath string) {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      55,
		FixedHeight:     19,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " Git Blame ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, content := NewConfiguredModal(app, config, true)
	modal.SetPreviousRoot(mainAppRoot)

	blameView := tview.NewTextView().
		SetDynamicColors(true).
		SetText(blameText).
		SetScrollable(true).
		SetTextAlign(tview.AlignLeft)

	closeButton := tview.NewButton("Close")
	closeButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	closeButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	closeButton.SetSelectedFunc(func() {
		modal.Close()
	})

	copyButton := tview.NewButton("Copy Hash")
	copyButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	copyButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	copyButton.SetSelectedFunc(func() {
		copyToSystemClipboard(commitHash)
		modal.Close()
	})

	showCommitButton := tview.NewButton("Show Commit")
	showCommitButton.SetStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack))
	showCommitButton.SetActivatedStyle(tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite))
	showCommitButton.SetSelectedFunc(func() {
		modal.Close()
		if !git.IsLazygitInstalled() {
			ShowLazygitNotInstalledModal(app)
			return
		}
		showBlameCommitInLazygit(app, filePath, commitHash)
	})

	focusOrder := []tview.Primitive{closeButton, copyButton, showCommitButton}

	setButtonFocus := func(index int) {
		for _, button := range focusOrder {
			button.Blur()
		}
		app.SetFocus(focusOrder[index])
	}

	closeButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyRight {
			setButtonFocus(1)
			return nil
		}
		if event.Key() == tcell.KeyLeft {
			setButtonFocus(2)
			return nil
		}
		return event
	})

	copyButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyRight {
			setButtonFocus(2)
			return nil
		}
		if event.Key() == tcell.KeyLeft {
			setButtonFocus(0)
			return nil
		}
		return event
	})

	showCommitButton.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyRight {
			setButtonFocus(0)
			return nil
		}
		if event.Key() == tcell.KeyLeft {
			setButtonFocus(1)
			return nil
		}
		return event
	})

	buttonFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(closeButton, 9, 0, true).
		AddItem(nil, 1, 0, false).
		AddItem(copyButton, 12, 0, false).
		AddItem(nil, 1, 0, false).
		AddItem(showCommitButton, 14, 0, false).
		AddItem(nil, 0, 1, false)

	content.AddItem(blameView, 0, 1, false)
	content.AddItem(buttonFlex, 3, 0, true)

	modal.Show()
	setButtonFocus(0)
}
