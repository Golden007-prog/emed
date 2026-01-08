// Package ui provides user interface components and managers for the emed editor.
package ui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type memStatsModal struct {
	app         *tview.Application
	modal       *modalWrapper
	content     *tview.TextView
	fileWatcher *FileWatcher
	stopChan    chan struct{}
	stopped     bool
	mu          sync.Mutex
}

func newMemStatsModal(app *tview.Application, fw *FileWatcher) *memStatsModal {
	return &memStatsModal{
		app:         app,
		fileWatcher: fw,
		stopChan:    make(chan struct{}),
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

	rss := getRSSBytes()

	goroutines := runtime.NumGoroutine()

	var watchedDirs int
	if m.fileWatcher != nil {
		watchedDirs = m.fileWatcher.GetWatchedDirsCount()
	}

	text := fmt.Sprintf(`[yellow]Memory[-]
  Heap Alloc:     %s
  Heap In-Use:    %s
  Heap Objects:   %s
  Stack In-Use:   %s
  Total Alloc:    %s
  Sys (reserved): %s
  RSS (process):  %s

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
		formatCount(ms.HeapObjects),
		formatBytes(ms.StackInuse),
		formatBytes(ms.TotalAlloc),
		formatBytes(ms.Sys),
		formatBytes(rss),
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

func getRSSBytes() uint64 {
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		return 0
	}
	return uint64(rusage.Maxrss)
}

// ShowMemStatsModal displays a modal with memory and performance statistics.
func ShowMemStatsModal(app *tview.Application, fw *FileWatcher) {
	modal := newMemStatsModal(app, fw)
	modal.Show()
}
