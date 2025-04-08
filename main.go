package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	
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
	
	app := tview.NewApplication()
	
	// Set up signal handling for clean shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	
	go func() {
		<-sigChan
		// Reset terminal first (in case app is frozen)
		fmt.Print("\033[?1049l") // Exit alternate screen
		fmt.Print("\033[?25h")   // Show cursor
		fmt.Print("\033[0m")     // Reset colors
		// Then try to stop the application
		app.Stop()
		os.Exit(0)
	}()
	
	// Create the main layout (working directory is already set via os.Chdir)
	layout := ui.NewLayout(app)
	
	// Set up the application with panic recovery
	defer func() {
		if r := recover(); r != nil {
			// Reset terminal on panic
			fmt.Print("\033[?1049l") // Exit alternate screen
			fmt.Print("\033[?25h")   // Show cursor
			fmt.Printf("Application error: %v\n", r)
		}
	}()
	
	// Run the application
	if err := app.SetRoot(layout.GetRoot(), true).EnableMouse(true).Run(); err != nil {
		// Reset terminal on error
		fmt.Print("\033[?1049l") // Exit alternate screen
		fmt.Print("\033[?25h")   // Show cursor
		panic(err)
	}
}