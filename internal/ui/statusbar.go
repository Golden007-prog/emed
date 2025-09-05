// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/config"
	"github.com/rivo/tview"
)

// StatusBar displays project information, git status, and application info.
type StatusBar struct {
	*tview.TextView
	projectPath   string
	gitBranch     string
	clipboardPath string
	clipboardCut  bool
	watchedDirs   int
	app           *tview.Application
	leftText      string
	rightText     string
	gitUpdating   bool
}

// NewStatusBar creates a new status bar with project information.
func NewStatusBar() *StatusBar {
	sb := &StatusBar{
		TextView: tview.NewTextView(),
	}

	sb.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetBackgroundColor(tcell.NewRGBColor(40, 40, 40))

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	sb.projectPath = filepath.Base(cwd)

	go sb.updateGitBranch()

	sb.updateTextParts()

	return sb
}

func (sb *StatusBar) updateGitBranch() {
	if sb.gitUpdating {
		return
	}
	sb.gitUpdating = true
	defer func() { sb.gitUpdating = false }()

	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		sb.gitBranch = ""
		sb.updateTextParts()
		if sb.app != nil {
			sb.app.QueueUpdateDraw(func() {})
		}
		return
	}

	cmd = exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		sb.gitBranch = strings.TrimSpace(string(output))
	} else {
		sb.gitBranch = "detached"
	}

	sb.updateTextParts()
	if sb.app != nil {
		sb.app.QueueUpdateDraw(func() {})
	}
}

func (sb *StatusBar) updateTextParts() {
	var parts []string

	parts = append(parts, fmt.Sprintf("project: %s", sb.projectPath))

	if sb.gitBranch != "" {
		parts = append(parts, fmt.Sprintf("git: %s", sb.gitBranch))
	}

	if sb.clipboardPath != "" {
		clipboardName := filepath.Base(sb.clipboardPath)
		if sb.clipboardCut {
			parts = append(parts, fmt.Sprintf("file operation: [red]cut[-] (%s)", clipboardName))
		} else {
			parts = append(parts, fmt.Sprintf("file operation: [green]copy[-] (%s)", clipboardName))
		}
	}

	if sb.watchedDirs > 0 {
		parts = append(parts, fmt.Sprintf("watching: %d dirs", sb.watchedDirs))
	}

	parts = append(parts, "[blue]F1 for help[-]")

	sb.leftText = strings.Join(parts, " | ")

	sb.rightText = fmt.Sprintf("%s v%s", config.AppName, config.Version)
}

// Draw renders the status bar to the screen.
func (sb *StatusBar) Draw(screen tcell.Screen) {
	sb.TextView.Draw(screen)

	x, y, width, _ := sb.GetInnerRect()
	if width <= 0 {
		return
	}

	leftLen := tview.TaggedStringWidth(sb.leftText)
	rightLen := len(sb.rightText)

	spacingNeeded := width - leftLen - rightLen - 1

	var fullText string
	if spacingNeeded > 0 {
		spacing := strings.Repeat(" ", spacingNeeded)
		fullText = sb.leftText + spacing + sb.rightText
	} else {
		if width > rightLen+1 {
			fullText = sb.leftText + " " + sb.rightText
		} else {
			fullText = sb.rightText
		}
	}

	style := tcell.StyleDefault.Background(tcell.NewRGBColor(40, 40, 40))
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}

	tview.Print(screen, fullText, x, y, width, tview.AlignLeft, tcell.ColorWhite)
}

// GetView returns the status bar as a primitive.
func (sb *StatusBar) GetView() tview.Primitive {
	return sb
}

// SetProjectPath sets the project path displayed in the status bar.
func (sb *StatusBar) SetProjectPath(path string) {
	sb.projectPath = filepath.Base(path)
	sb.updateTextParts()
}

// RefreshGitBranch updates the git branch information.
func (sb *StatusBar) RefreshGitBranch() {
	go sb.updateGitBranch()
}

// SetClipboard updates the clipboard information in the status bar.
func (sb *StatusBar) SetClipboard(path string, isCut bool) {
	sb.clipboardPath = path
	sb.clipboardCut = isCut
	sb.updateTextParts()
}

// SetApp sets the application reference for the status bar.
func (sb *StatusBar) SetApp(app *tview.Application) {
	sb.app = app
}

// SetWatchedDirs updates the count of watched directories.
func (sb *StatusBar) SetWatchedDirs(count int) {
	sb.watchedDirs = count
	sb.updateTextParts()
}
