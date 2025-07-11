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

// StatusBar represents the status bar at the bottom
type StatusBar struct {
	*tview.TextView
	projectPath   string
	gitBranch     string
	clipboardPath string
	clipboardCut  bool
	app           *tview.Application
	leftText      string  // Cached left text
	rightText     string  // Cached right text
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	sb := &StatusBar{
		TextView: tview.NewTextView(),
	}
	
	// Configure the view
	sb.
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
	
	// Initial update to cache the text parts
	sb.updateTextParts()
	
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

// updateTextParts updates the cached left and right text parts
func (sb *StatusBar) updateTextParts() {
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

	sb.leftText = strings.Join(parts, " | ")
	
	// Build right side text (app name and version)
	sb.rightText = fmt.Sprintf("%s v%s", config.AppName, config.Version)
}

// Draw draws the status bar with proper alignment
func (sb *StatusBar) Draw(screen tcell.Screen) {
	// First, let TextView do its normal drawing
	sb.TextView.Draw(screen)
	
	// Get the inner rect to know the actual width
	x, y, width, _ := sb.GetInnerRect()
	if width <= 0 {
		return
	}
	
	// Calculate the visible length of text (without color tags)
	leftLen := tview.TaggedStringWidth(sb.leftText)
	rightLen := len(sb.rightText)
	
	// Calculate spacing needed
	spacingNeeded := width - leftLen - rightLen - 1  // -1 for safety margin
	
	var fullText string
	if spacingNeeded > 0 {
		// We have enough space
		spacing := strings.Repeat(" ", spacingNeeded)
		fullText = sb.leftText + spacing + sb.rightText
	} else {
		// Not enough space, need to handle truncation carefully
		// For now, just show what fits with a simple space
		if width > rightLen + 1 {
			// Show as much of left text as possible
			fullText = sb.leftText + " " + sb.rightText
			// If it's still too long, tview will handle the truncation
		} else {
			// Very small terminal, just show the app info
			fullText = sb.rightText
		}
	}
	
	// Clear the line first
	style := tcell.StyleDefault.Background(tcell.NewRGBColor(40, 40, 40))
	for i := 0; i < width; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
	
	// Now draw our formatted text
	tview.Print(screen, fullText, x, y, width, tview.AlignLeft, tcell.ColorWhite)
}

// GetView returns the primitive for the status bar
func (sb *StatusBar) GetView() tview.Primitive {
	return sb
}

// SetProjectPath updates the project path shown
func (sb *StatusBar) SetProjectPath(path string) {
	sb.projectPath = filepath.Base(path)
	sb.updateTextParts()
	if sb.app != nil {
		sb.app.Draw()
	}
}

// RefreshGitBranch refreshes the git branch information
func (sb *StatusBar) RefreshGitBranch() {
	sb.updateGitBranch()
	sb.updateTextParts()
	if sb.app != nil {
		sb.app.Draw()
	}
}

// SetClipboard updates the clipboard status
func (sb *StatusBar) SetClipboard(path string, isCut bool) {
	sb.clipboardPath = path
	sb.clipboardCut = isCut
	sb.updateTextParts()
	if sb.app != nil {
		sb.app.Draw()
	}
}

// SetApp sets the application reference for getting terminal dimensions
func (sb *StatusBar) SetApp(app *tview.Application) {
	sb.app = app
}