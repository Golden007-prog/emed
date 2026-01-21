// Package clipboard provides cross-platform clipboard operations.
package clipboard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var errNoSystemClipboard = errors.New("no system clipboard available")

type clipboardStore struct {
	mu   sync.RWMutex
	text string
}

var defaultStore = &clipboardStore{}

// Clipboard provides system clipboard operations.
type Clipboard struct {
	store *clipboardStore
}

// New creates a new Clipboard instance.
func New() *Clipboard {
	return &Clipboard{store: defaultStore}
}

// Copy puts text on the system clipboard.
func (c *Clipboard) Copy(text string) error {
	if text == "" {
		return nil
	}

	c.store.set(text)

	if err := copyToSystemClipboard(text); err != nil && !errors.Is(err, errNoSystemClipboard) {
		return err
	}
	return nil
}

// Paste retrieves text from the system clipboard.
func (c *Clipboard) Paste() (string, error) {
	text, err := pasteFromSystemClipboard()
	if err == nil {
		return text, nil
	}

	fallback := c.store.get()
	if fallback != "" {
		return fallback, nil
	}

	return "", err
}

func (s *clipboardStore) set(text string) {
	s.mu.Lock()
	s.text = text
	s.mu.Unlock()
}

func (s *clipboardStore) get() string {
	s.mu.RLock()
	text := s.text
	s.mu.RUnlock()
	return text
}

func copyToSystemClipboard(text string) error {
	cmd, err := buildCopyCommand()
	if err != nil {
		return err
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func pasteFromSystemClipboard() (string, error) {
	cmd, err := buildPasteCommand()
	if err != nil {
		return "", err
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

func buildCopyCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "linux":
		return linuxCopyCommand()
	case "windows":
		return exec.Command("clip"), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func buildPasteCommand() (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("pbpaste"), nil
	case "linux":
		return linuxPasteCommand()
	case "windows":
		return exec.Command("powershell", "-command", "Get-Clipboard"), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func linuxCopyCommand() (*exec.Cmd, error) {
	if isWaylandSession() {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command("wl-copy"), nil
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return exec.Command("xclip", "-selection", "clipboard"), nil
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return exec.Command("xsel", "--clipboard", "--input"), nil
	}
	if !isWaylandSession() {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return exec.Command("wl-copy"), nil
		}
	}
	return nil, errNoSystemClipboard
}

func linuxPasteCommand() (*exec.Cmd, error) {
	if isWaylandSession() {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			return exec.Command("wl-paste", "--no-newline"), nil
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		return exec.Command("xclip", "-selection", "clipboard", "-o"), nil
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		return exec.Command("xsel", "--clipboard", "--output"), nil
	}
	if !isWaylandSession() {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			return exec.Command("wl-paste", "--no-newline"), nil
		}
	}
	return nil, errNoSystemClipboard
}

func isWaylandSession() bool {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return true
	}
	return false
}
