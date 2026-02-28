package ui

import (
	"os"
	"sync"
	"time"
)

type SingleFileWatcher struct {
	filePath string
	onChange func([]FileChange)
	stopChan chan struct{}

	mu      sync.RWMutex
	running bool
	exists  bool
	modTime time.Time
}

func NewSingleFileWatcher(filePath string) *SingleFileWatcher {
	return &SingleFileWatcher{
		filePath: filePath,
		stopChan: make(chan struct{}),
	}
}

func (w *SingleFileWatcher) SetOnChange(handler func([]FileChange)) {
	w.mu.Lock()
	w.onChange = handler
	w.mu.Unlock()
}

func (w *SingleFileWatcher) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}

	info, err := os.Stat(w.filePath)
	if err == nil {
		w.exists = true
		w.modTime = info.ModTime()
	} else if os.IsNotExist(err) {
		w.exists = false
		w.modTime = time.Time{}
	} else {
		w.mu.Unlock()
		return err
	}

	w.running = true
	w.mu.Unlock()

	go w.watchLoop()
	return nil
}

func (w *SingleFileWatcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopChan)
	w.mu.Unlock()
}

func (w *SingleFileWatcher) GetWatchedDirsCount() int {
	return 0
}

func (w *SingleFileWatcher) watchLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *SingleFileWatcher) poll() {
	info, err := os.Stat(w.filePath)
	currentExists := err == nil
	currentModTime := time.Time{}
	if currentExists {
		currentModTime = info.ModTime()
	}
	if err != nil && !os.IsNotExist(err) {
		return
	}

	var change *FileChange

	w.mu.Lock()
	prevExists := w.exists
	prevModTime := w.modTime

	switch {
	case !prevExists && currentExists:
		change = &FileChange{Path: w.filePath, Type: FileCreated}
	case prevExists && !currentExists:
		change = &FileChange{Path: w.filePath, Type: FileDeleted}
	case prevExists && currentExists && !currentModTime.Equal(prevModTime):
		change = &FileChange{Path: w.filePath, Type: FileModified}
	}

	w.exists = currentExists
	w.modTime = currentModTime
	handler := w.onChange
	w.mu.Unlock()

	if change != nil && handler != nil {
		handler([]FileChange{*change})
	}
}
