package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/laborin/emed/internal/config"
	"github.com/laborin/emed/internal/logging"
	"github.com/laborin/emed/internal/ui"
	"github.com/rivo/tview"
)

func main() {
	os.Setenv("COLORTERM", "truecolor")

	if pprofPort := os.Getenv("EMED_PPROF_PORT"); pprofPort != "" {
		go func() {
			addr := "localhost:" + pprofPort
			http.ListenAndServe(addr, nil)
		}()
	}

	var workingDir string
	var launchClaude bool

	args := os.Args[1:]
	var pathArg string

	for _, arg := range args {
		if arg == "-c" || arg == "--claude" {
			launchClaude = true
		} else if !strings.HasPrefix(arg, "-") {
			pathArg = arg
		}
	}

	if pathArg != "" {
		info, err := os.Stat(pathArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Cannot access '%s': %v\n", pathArg, err)
			os.Exit(1)
		}

		if info.IsDir() {
			workingDir, err = filepath.Abs(pathArg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", pathArg, err)
				os.Exit(1)
			}
		} else {
			absPath, err := filepath.Abs(pathArg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Cannot resolve path '%s': %v\n", pathArg, err)
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

	app := tview.NewApplication()
	layout := ui.NewLayout(app)

	if launchClaude {
		layout.SetLaunchClaudeOnStart(true)
	}

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

	if err := app.SetRoot(layout.GetRoot(), true).EnableMouse(true).EnablePaste(true).Run(); err != nil {
		fmt.Print(config.TerminalExitAltScreen)
		fmt.Print(config.TerminalShowCursor)
		panic(err)
	}

	layout.Stop()
}
