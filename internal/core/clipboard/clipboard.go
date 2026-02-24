// Package clipboard provides cross-platform clipboard operations.
package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

var errNoSystemClipboard = errors.New("no system clipboard available")

type clipboardMode string

const (
	clipboardModeAuto     clipboardMode = "auto"
	clipboardModeSystem   clipboardMode = "system"
	clipboardModeOSC52    clipboardMode = "osc52"
	clipboardModeInternal clipboardMode = "internal"
)

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

	mode := getClipboardMode()

	switch mode {
	case clipboardModeInternal:
		return nil
	case clipboardModeSystem:
		return ignoreNoSystemClipboard(copyToSystemClipboard(text))
	case clipboardModeOSC52:
		return ignoreNoSystemClipboard(copyToOSC52Clipboard(text))
	default:
		osc52Err := copyToOSC52Clipboard(text)
		systemErr := copyToSystemClipboard(text)

		if !isNoSystemClipboardError(systemErr) {
			return systemErr
		}
		if !isNoSystemClipboardError(osc52Err) {
			return osc52Err
		}
		return nil
	}
}

// Paste retrieves text from the system clipboard.
func (c *Clipboard) Paste() (string, error) {
	mode := getClipboardMode()
	systemErr := errNoSystemClipboard

	if mode != clipboardModeInternal {
		text, err := pasteFromSystemClipboard()
		if err == nil {
			return normalizeLineEndings(text), nil
		}
		systemErr = err
	}

	fallback := c.store.get()
	if fallback != "" {
		return normalizeLineEndings(fallback), nil
	}

	return "", systemErr
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

func copyToOSC52Clipboard(text string) error {
	sequence, err := buildOSC52Sequence(text)
	if err != nil {
		return err
	}

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err == nil {
		defer tty.Close()
		_, err = tty.WriteString(sequence)
		return err
	}

	_, err = os.Stdout.WriteString(sequence)
	return err
}

func buildOSC52Sequence(text string) (string, error) {
	if runtime.GOOS == "windows" {
		return "", errNoSystemClipboard
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "dumb" {
		return "", errNoSystemClipboard
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	sequence := "\x1b]52;c;" + encoded + "\a"

	if os.Getenv("TMUX") != "" {
		return "\x1bPtmux;\x1b" + sequence + "\x1b\\", nil
	}

	if isScreenSession() {
		return "\x1bP" + sequence + "\x1b\\", nil
	}

	return sequence, nil
}

func isScreenSession() bool {
	if os.Getenv("STY") != "" {
		return true
	}

	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	return strings.HasPrefix(term, "screen")
}

func getClipboardMode() clipboardMode {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("EMED_CLIPBOARD")))
	switch clipboardMode(mode) {
	case clipboardModeSystem, clipboardModeOSC52, clipboardModeInternal:
		return clipboardMode(mode)
	default:
		return clipboardModeAuto
	}
}

func isNoSystemClipboardError(err error) bool {
	return err == nil || errors.Is(err, errNoSystemClipboard)
}

func ignoreNoSystemClipboard(err error) error {
	if isNoSystemClipboardError(err) {
		return nil
	}
	return err
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

func normalizeLineEndings(text string) string {
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}
