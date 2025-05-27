package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/config"
	"github.com/rivo/tview"
)

// StatusBar represents the status bar at the bottom
type StatusBar struct {
	view          *tview.TextView
	projectPath   string
	gitBranch     string
	clipboardPath string
	clipboardCut  bool
	app           *tview.Application
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
	
	// Initial update with basic text (without alignment)
	sb.view.SetText(fmt.Sprintf("project: %s | [yellow]Alt+H or F1 for help[-]", sb.projectPath))
	
	// Start updating the status bar with proper alignment after a brief delay
	go func() {
		time.Sleep(50 * time.Millisecond)  // Quick initial update
		sb.update()
		if sb.app != nil {
			sb.app.Draw()
		}
		sb.startUpdater()
	}()
	
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
	// Build left side text
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

	leftText := strings.Join(parts, " | ")
	
	// Build right side text (app name and version)
	rightText := fmt.Sprintf("%s v%s", config.AppName, config.Version)
	
	// Get terminal width from the view's inner rect
	_, _, width, _ := sb.view.GetInnerRect()
	if width <= 0 {
		width = 80  // Default fallback
	}
	
	// Calculate the visible length of text (without color tags)
	leftLen := tview.TaggedStringWidth(leftText)
	rightLen := len(rightText)
	
	// Calculate spacing needed
	spacingNeeded := width - leftLen - rightLen - 1  // -1 for safety margin
	
	var fullText string
	if spacingNeeded > 0 {
		// We have enough space
		spacing := strings.Repeat(" ", spacingNeeded)
		fullText = leftText + spacing + rightText
	} else {
		// Not enough space, need to handle truncation carefully
		// For now, just show what fits with a simple space
		if width > rightLen + 1 {
			// Show as much of left text as possible
			fullText = leftText + " " + rightText
			// If it's still too long, tview will handle the truncation
		} else {
			// Very small terminal, just show the app info
			fullText = rightText
		}
	}
	
	sb.view.SetText(fullText)
}

// startUpdater starts a goroutine to update the status bar periodically
func (sb *StatusBar) startUpdater() {
	ticker := time.NewTicker(1 * time.Second) // Update every second to handle resizes
	defer ticker.Stop()
	
	for range ticker.C {
		sb.update()
		if sb.app != nil {
			sb.app.Draw()
		}
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

// SetApp sets the application reference for getting terminal dimensions
func (sb *StatusBar) SetApp(app *tview.Application) {
	sb.app = app
}