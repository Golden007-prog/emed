package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// StatusBar represents the status bar at the bottom
type StatusBar struct {
	view          *tview.TextView
	projectPath   string
	gitBranch     string
	clipboardPath string
	clipboardCut  bool
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	sb := &StatusBar{
		view: tview.NewTextView(),
	}
	
	// Configure the view
	sb.view.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetBackgroundColor(tcell.NewRGBColor(40, 40, 40))
	
	// Get current project path
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	sb.projectPath = filepath.Base(cwd)
	
	// Get git branch if in git repository
	sb.updateGitBranch()
	
	// Start updating the status bar
	sb.update()
	go sb.startUpdater()
	
	return sb
}

// updateGitBranch updates the current git branch
func (sb *StatusBar) updateGitBranch() {
	cmd := exec.Command("git", "branch", "--show-current")
	output, err := cmd.Output()
	if err == nil {
		sb.gitBranch = strings.TrimSpace(string(output))
	} else {
		// Check if we're in a git repo at all
		cmd = exec.Command("git", "rev-parse", "--git-dir")
		if err := cmd.Run(); err != nil {
			sb.gitBranch = ""
		} else {
			sb.gitBranch = "detached"
		}
	}
}

// update refreshes the status bar content
func (sb *StatusBar) update() {
	// Build status text
	var parts []string
	
	// Project path
	parts = append(parts, fmt.Sprintf("project: %s", sb.projectPath))
	
	// Git branch if available
	if sb.gitBranch != "" {
		parts = append(parts, fmt.Sprintf("git: %s", sb.gitBranch))
	}
	
	// Clipboard status
	if sb.clipboardPath != "" {
		clipboardName := filepath.Base(sb.clipboardPath)
		if sb.clipboardCut {
			parts = append(parts, fmt.Sprintf("file operation: [red]cut[-] (%s)", clipboardName))
		} else {
			parts = append(parts, fmt.Sprintf("file operation: [green]copy[-] (%s)", clipboardName))
		}
	}
	
	// Help hint
	parts = append(parts, "[yellow]Alt+H or F1 for help[-]")

	fullText := strings.Join(parts, " | ")
	
	sb.view.SetText(fullText)
}

// startUpdater starts a goroutine to update the clock
func (sb *StatusBar) startUpdater() {
	ticker := time.NewTicker(30 * time.Second) // Update every 30 seconds
	defer ticker.Stop()
	
	for range ticker.C {
		sb.update()
		// Note: In a real app, we'd need to trigger a redraw
	}
}

// GetView returns the primitive for the status bar
func (sb *StatusBar) GetView() tview.Primitive {
	return sb.view
}

// SetProjectPath updates the project path shown
func (sb *StatusBar) SetProjectPath(path string) {
	sb.projectPath = filepath.Base(path)
	sb.update()
}

// RefreshGitBranch refreshes the git branch information
func (sb *StatusBar) RefreshGitBranch() {
	sb.updateGitBranch()
	sb.update()
}

// SetClipboard updates the clipboard status
func (sb *StatusBar) SetClipboard(path string, isCut bool) {
	sb.clipboardPath = path
	sb.clipboardCut = isCut
	sb.update()
}