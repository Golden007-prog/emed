package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	
	"github.com/laborin/emed/internal/config"
	"github.com/laborin/emed/internal/core/container"
	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/emed/internal/ui"
	"github.com/rivo/tview"
)

func main() {
	// Enable true color support
	os.Setenv("COLORTERM", "truecolor")
	
	// Determine the working directory
	var workingDir string
	
	if len(os.Args) > 1 {
		// Argument provided - use it
		argPath := os.Args[1]
		
		// Check if it's a file or directory
		info, err := os.Stat(argPath)
		if err != nil {
			// Path doesn't exist or can't be accessed
			fmt.Fprintf(os.Stderr, "Error: Cannot access '%s': %v\n", argPath, err)
			os.Exit(1)
		}
		
		if info.IsDir() {
			// It's a directory - use it directly
			workingDir, err = filepath.Abs(argPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", argPath, err)
				os.Exit(1)
			}
		} else {
			// It's a file - use its parent directory
			absPath, err := filepath.Abs(argPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", argPath, err)
				os.Exit(1)
			}
			workingDir = filepath.Dir(absPath)
		}
	} else {
		// No argument - use current working directory
		cwd, err := os.Getwd()
		if err != nil {
			// Can't get current directory - fall back to home directory
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot determine working directory: %v\n", err)
				os.Exit(1)
			}
			workingDir = homeDir
		} else {
			workingDir = cwd
		}
	}
	
	// Change to the working directory
	if err := os.Chdir(workingDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot change to directory '%s': %v\n", workingDir, err)
		os.Exit(1)
	}
	
	// Initialize logging
	logger := logging.GetLogger()
	defer logger.Close()
	logging.Info("Emed started in directory: %s", workingDir)
	
	diContainer := container.New()
	app := tview.NewApplication()
	if err := diContainer.RegisterInstance(
		reflect.TypeOf((*tview.Application)(nil)),
		app,
	); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to register application: %v\n", err)
		os.Exit(1)
	}
	
	if err := container.RegisterServices(diContainer); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to register services: %v\n", err)
		os.Exit(1)
	}
	
	if err := diContainer.Build(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to build container: %v\n", err)
		os.Exit(1)
	}
	
	layoutInterface, err := diContainer.Resolve(reflect.TypeOf((*ui.Layout)(nil)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to resolve layout: %v\n", err)
		os.Exit(1)
	}
	layout := layoutInterface.(*ui.Layout)
	
	// Set up signal handling for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	
	go func() {
		<-sigChan
		// Clean up resources
		layout.Stop()
		// Reset terminal first (in case app is frozen)
		fmt.Print(config.TerminalExitAltScreen) // Exit alternate screen
		fmt.Print(config.TerminalShowCursor)   // Show cursor
		fmt.Print(config.TerminalResetColors)  // Reset colors
		// Then try to stop the application
		app.Stop()
		os.Exit(0)
	}()
	
	// Set up the application with panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Clean up resources
			layout.Stop()
			// Reset terminal on panic
			fmt.Print(config.TerminalExitAltScreen) // Exit alternate screen
			fmt.Print(config.TerminalShowCursor)   // Show cursor
			fmt.Print(config.TerminalResetColors)  // Reset colors
			fmt.Printf("Application error: %v\n", r)
			// Print stack trace
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Printf("Stack trace:\n%s\n", buf[:n])
		}
	}()
	
	// Run the application
	if err := app.SetRoot(layout.GetRoot(), true).EnableMouse(true).Run(); err != nil {
		// Reset terminal on error
		fmt.Print(config.TerminalExitAltScreen) // Exit alternate screen
		fmt.Print(config.TerminalShowCursor)   // Show cursor
		panic(err)
	}
	
	// Clean up on normal exit
	layout.Stop()
}