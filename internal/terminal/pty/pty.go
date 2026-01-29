// Package pty manages pseudo-terminal interfaces for shell processes.
package pty

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// PTYManager handles pseudo-terminal creation and management.
type PTYManager struct {
	ptm        *os.File
	cmd        *exec.Cmd
	shell      string
	customCmd  []string
	workingDir string
	extraEnv   []string
	mu         sync.RWMutex
	running    bool
	onData     func([]byte)
	onExit     func()
}

// New creates a new PTYManager with the specified shell and working directory.
func New(shell string, workingDir string) *PTYManager {
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	return &PTYManager{
		shell:      shell,
		workingDir: workingDir,
	}
}

// NewWithCommand creates a PTYManager that runs a custom command instead of a shell.
func NewWithCommand(command []string, workingDir string) *PTYManager {
	return &PTYManager{
		customCmd:  command,
		workingDir: workingDir,
	}
}

// Start initiates the PTY with specified dimensions.
func (p *PTYManager) Start(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("PTY already running")
	}

	if len(p.customCmd) > 0 {
		p.cmd = exec.Command(p.customCmd[0], p.customCmd[1:]...)
	} else {
		p.cmd = exec.Command(p.shell, "-l", "-i")
	}

	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "TERM_PROGRAM") &&
			!strings.HasPrefix(e, "ITERM_") &&
			!strings.HasPrefix(e, "LC_TERMINAL") {
			filtered = append(filtered, e)
		}
	}

	cwd := p.workingDir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	p.cmd.Dir = cwd

	homeDir, _ := os.UserHomeDir()
	hash := md5.Sum([]byte(cwd))
	stateDir := filepath.Join(homeDir, ".emed", hex.EncodeToString(hash[:])[:12])
	os.MkdirAll(stateDir, 0755)
	p.cmd.Env = append(filtered, "TERM=xterm-256color", "EMED_STATE_DIR="+stateDir)
	p.cmd.Env = append(p.cmd.Env, p.extraEnv...)

	ptm, err := pty.StartWithSize(p.cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
		X:    0,
		Y:    0,
	})
	if err != nil {
		return fmt.Errorf("failed to start PTY: %v", err)
	}

	p.ptm = ptm
	p.running = true

	go p.readLoop()

	return nil
}

// Stop terminates the PTY and associated shell process.
func (p *PTYManager) Stop() error {
	p.mu.Lock()

	if !p.running {
		p.mu.Unlock()
		return nil
	}

	p.running = false
	cmd := p.cmd
	ptm := p.ptm
	p.cmd = nil
	p.ptm = nil
	p.mu.Unlock()

	// Close PTY first to trigger EOF in readLoop
	if ptm != nil {
		ptm.Close()
	}

	// Then terminate the process
	if cmd != nil && cmd.Process != nil {
		// Try graceful termination first
		cmd.Process.Signal(syscall.SIGTERM)

		// Give it a moment to exit cleanly
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		// Wait for process to exit or timeout
		select {
		case <-done:
			// Process exited
		case <-time.After(100 * time.Millisecond):
			// Force kill if it doesn't exit
			cmd.Process.Kill()
			<-done
		}
	}

	return nil
}

// Write sends data to the PTY.
func (p *PTYManager) Write(data []byte) error {
	p.mu.RLock()

	if !p.running || p.ptm == nil {
		p.mu.RUnlock()
		return fmt.Errorf("PTY not running")
	}

	ptm := p.ptm
	p.mu.RUnlock() // Release lock before write to avoid blocking other operations

	_, err := ptm.Write(data)
	return err
}

// Resize adjusts the PTY dimensions.
func (p *PTYManager) Resize(cols, rows int) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.running || p.ptm == nil {
		return fmt.Errorf("PTY not running")
	}

	return pty.Setsize(p.ptm, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// IsRunning returns whether the PTY is active.
func (p *PTYManager) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// GetPID returns the process ID of the PTY process, or 0 if not running.
func (p *PTYManager) GetPID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.running && p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// SetOnData sets the callback for receiving PTY output.
func (p *PTYManager) SetOnData(fn func([]byte)) {
	p.onData = fn
}

// SetOnExit sets the callback for PTY termination.
func (p *PTYManager) SetOnExit(fn func()) {
	p.onExit = fn
}

// SetExtraEnv sets additional environment variables for the PTY process.
func (p *PTYManager) SetExtraEnv(env []string) {
	p.extraEnv = env
}

func (p *PTYManager) readLoop() {
	buf := make([]byte, 4096)

	for {
		p.mu.RLock()
		if !p.running || p.ptm == nil {
			p.mu.RUnlock()
			break
		}
		ptm := p.ptm
		p.mu.RUnlock()

		n, err := ptm.Read(buf)
		if err != nil {
			break
		}
		if n > 0 && p.onData != nil {
			p.onData(buf[:n])
		}
	}

	p.mu.Lock()
	if p.running {
		p.running = false
		if p.onExit != nil {
			p.mu.Unlock()
			p.onExit()
			return
		}
	}
	p.mu.Unlock()
}
