// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type memStatsModal struct {
	app          *tview.Application
	modal        *modalWrapper
	content      *tview.TextView
	watcherStats WatcherStatsProvider
	stopChan     chan struct{}
	stopped      bool
	mu           sync.Mutex
}

type WatcherStatsProvider interface {
	GetWatchedDirsCount() int
}

func newMemStatsModal(app *tview.Application, watcherStats WatcherStatsProvider) *memStatsModal {
	return &memStatsModal{
		app:          app,
		watcherStats: watcherStats,
		stopChan:     make(chan struct{}),
	}
}

func (m *memStatsModal) Show() {
	config := EmModalConfig{
		UseFixedSize:    true,
		FixedWidth:      55,
		FixedHeight:     22,
		BackgroundColor: tcell.ColorBlack,
		BorderColor:     tcell.NewRGBColor(102, 102, 102),
		TitleColor:      tcell.ColorLightGray,
		Title:           " Performance Stats (F8) ",
		TitleAlign:      tview.AlignCenter,
	}

	modal, contentFlex := NewConfiguredModal(m.app, config, true)
	modal.SetPreviousRoot(mainAppRoot)
	m.modal = modal

	m.content = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)

	gcButton := tview.NewButton("Force GC").
		SetSelectedFunc(func() {
			runtime.GC()
			debug.FreeOSMemory()
			m.updateStats()
		})
	gcButton.SetBackgroundColor(tcell.NewRGBColor(60, 60, 60))
	gcButton.SetLabelColor(tcell.ColorWhite)
	gcButton.SetStyle(tcell.StyleDefault.Background(tcell.NewRGBColor(60, 60, 60)))

	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(nil, 0, 1, false).
		AddItem(gcButton, 12, 0, true).
		AddItem(nil, 0, 1, false)

	contentFlex.AddItem(m.content, 0, 1, false)
	contentFlex.AddItem(buttonRow, 1, 0, true)

	modal.modal.SetOnClose(func() {
		m.stop()
	})

	m.updateStats()

	go m.refreshLoop()

	modal.Show()
	m.app.SetFocus(gcButton)
}

func (m *memStatsModal) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopChan)
	}
}

func (m *memStatsModal) refreshLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.mu.Lock()
			stopped := m.stopped
			m.mu.Unlock()
			if stopped {
				return
			}
			m.app.QueueUpdateDraw(func() {
				m.updateStats()
			})
		}
	}
}

func (m *memStatsModal) updateStats() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	currentRSS, currentRSSOK := getCurrentRSSBytes()
	currentRSSText := "n/a"
	if currentRSSOK {
		currentRSSText = formatBytes(currentRSS)
	}

	maxRSS := getRSSBytes()
	maxRSSText := formatBytes(maxRSS)

	goroutines := runtime.NumGoroutine()

	var watchedDirs int
	if m.watcherStats != nil {
		watchedDirs = m.watcherStats.GetWatchedDirsCount()
	}

	text := fmt.Sprintf(`[yellow]Memory[-]
  Heap Alloc:     %s
  Heap In-Use:    %s
  Heap Idle:      %s
  Heap Released:  %s
  Heap Objects:   %s
  Stack In-Use:   %s
  Total Alloc:    %s
  Sys (reserved): %s
  RSS (current):  %s
  RSS (max):      %s

[yellow]Garbage Collector[-]
  Last GC:        %s ago
  GC Cycles:      %d
  GC CPU:         %.2f%%

[yellow]Runtime[-]
  Goroutines:     %d
  Watched Dirs:   %d
`,
		formatBytes(ms.HeapAlloc),
		formatBytes(ms.HeapInuse),
		formatBytes(ms.HeapIdle),
		formatBytes(ms.HeapReleased),
		formatCount(ms.HeapObjects),
		formatBytes(ms.StackInuse),
		formatBytes(ms.TotalAlloc),
		formatBytes(ms.Sys),
		currentRSSText,
		maxRSSText,
		formatDuration(time.Since(time.Unix(0, int64(ms.LastGC)))),
		ms.NumGC,
		ms.GCCPUFraction*100,
		goroutines,
		watchedDirs,
	)

	m.content.SetText(text)
}

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/KB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatCount(n uint64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.2fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "never"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func getCurrentRSSBytes() (uint64, bool) {
	pid := os.Getpid()
	if runtime.GOOS == "linux" {
		if rss, err := readLinuxRSSBytes(); err == nil {
			return rss, true
		}
		if rss, err := rssFromPS(pid); err == nil {
			return rss, true
		}
		return 0, false
	}
	if runtime.GOOS == "darwin" {
		if rss, err := rssFromPS(pid); err == nil {
			return rss, true
		}
		return 0, false
	}
	if rss, err := rssFromPS(pid); err == nil {
		return rss, true
	}
	return 0, false
}

func readLinuxRSSBytes() (uint64, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, errors.New("invalid statm")
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * uint64(os.Getpagesize()), nil
}

func rssFromPS(pid int) (uint64, error) {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, errors.New("empty rss")
	}
	value, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return value * 1024, nil
}

func getRSSBytes() uint64 {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0
	}
	rss := uint64(rusage.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss
}

// ShowMemStatsModal displays a modal with memory and performance statistics.
func ShowMemStatsModal(app *tview.Application, watcherStats WatcherStatsProvider) {
	modal := newMemStatsModal(app, watcherStats)
	modal.Show()
}
