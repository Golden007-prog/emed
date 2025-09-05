// Package pty manages pseudo-terminal interfaces for shell processes.
package pty

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	
	"github.com/creack/pty"
)

// PTYManager handles pseudo-terminal creation and management.
type PTYManager struct {
	ptm      *os.File
	cmd      *exec.Cmd
	shell    string
	mu       sync.RWMutex
	running  bool
	onData   func([]byte)
	onExit   func()
}

// New creates a new PTYManager with the specified shell.
func New(shell string) *PTYManager {
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}
	
	return &PTYManager{
		shell: shell,
	}
}

// Start initiates the PTY with specified dimensions.
func (p *PTYManager) Start(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return fmt.Errorf("PTY already running")
	}
	
	p.cmd = exec.Command(p.shell, "-i")
	// Set environment to prevent OSC sequences from leaking
	env := os.Environ()
	// Filter out any existing TERM_PROGRAM variables that might cause issues
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "TERM_PROGRAM") && 
		   !strings.HasPrefix(e, "ITERM_") &&
		   !strings.HasPrefix(e, "LC_TERMINAL") {
			filtered = append(filtered, e)
		}
	}
	p.cmd.Env = append(filtered, "TERM=xterm-256color")
	
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

// SetOnData sets the callback for receiving PTY output.
func (p *PTYManager) SetOnData(fn func([]byte)) {
	p.onData = fn
}

// SetOnExit sets the callback for PTY termination.
func (p *PTYManager) SetOnExit(fn func()) {
	p.onExit = fn
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