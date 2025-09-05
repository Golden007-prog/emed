// Package clipboard provides cross-platform clipboard operations.
package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Clipboard provides system clipboard operations.
type Clipboard struct{}

// New creates a new Clipboard instance.
func New() *Clipboard {
	return &Clipboard{}
}

// Copy puts text on the system clipboard.
func (c *Clipboard) Copy(text string) error {
	if text == "" {
		return nil
	}
	
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
			return fmt.Errorf("no clipboard utility found (xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// Paste retrieves text from the system clipboard.
func (c *Clipboard) Paste() (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard utility found (xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("powershell", "-command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	text := string(output)
	if runtime.GOOS == "windows" {
		text = strings.TrimSuffix(text, "\r\n")
	}
	
	return text, nil
}