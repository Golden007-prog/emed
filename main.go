package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	
	"github.com/laborin/emed/internal/ui"
	"github.com/rivo/tview"
)

func main() {
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
	
	// Create the main layout
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