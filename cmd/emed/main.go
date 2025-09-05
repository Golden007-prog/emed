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
	os.Setenv("COLORTERM", "truecolor")
	
	var workingDir string
	
	if len(os.Args) > 1 {
		argPath := os.Args[1]
		
		info, err := os.Stat(argPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Cannot access '%s': %v\n", argPath, err)
			os.Exit(1)
		}
		
		if info.IsDir() {
			workingDir, err = filepath.Abs(argPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", argPath, err)
				os.Exit(1)
			}
		} else {
			absPath, err := filepath.Abs(argPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", argPath, err)
				os.Exit(1)
			}
			workingDir = filepath.Dir(absPath)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
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
	
	if err := os.Chdir(workingDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot change to directory '%s': %v\n", workingDir, err)
		os.Exit(1)
	}
	
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
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGKILL)
	
	go func() {
		<-sigChan
		layout.Stop()
		fmt.Print(config.TerminalExitAltScreen)
		fmt.Print(config.TerminalShowCursor)
		fmt.Print(config.TerminalResetColors)
		app.Stop()
		os.Exit(0)
	}()
	
	defer func() {
		if r := recover(); r != nil {
			layout.Stop()
			fmt.Print(config.TerminalExitAltScreen)
			fmt.Print(config.TerminalShowCursor)
			fmt.Print(config.TerminalResetColors)
			fmt.Printf("Application error: %v\n", r)
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			fmt.Printf("Stack trace:\n%s\n", buf[:n])
		}
	}()
	
	if err := app.SetRoot(layout.GetRoot(), true).EnableMouse(true).Run(); err != nil {
		fmt.Print(config.TerminalExitAltScreen)
		fmt.Print(config.TerminalShowCursor)
		panic(err)
	}
	
	layout.Stop()
}