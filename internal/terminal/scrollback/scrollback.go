// Package scrollback manages terminal scrollback buffer functionality.
package scrollback

import (
	"sync"
)

// ScrollbackBuffer manages scrolling through terminal history.
type ScrollbackBuffer struct {
	maxLines       int
	viewportOffset int
	mu             sync.RWMutex
}

// New creates a new scrollback buffer with the given maximum lines.
func New(maxLines int) *ScrollbackBuffer {
	if maxLines <= 0 {
		maxLines = 10000
	}
	return &ScrollbackBuffer{
		maxLines:       maxLines,
		viewportOffset: 0,
	}
}

// ScrollUp scrolls up by the given amount without limits.
func (s *ScrollbackBuffer) ScrollUp(amount int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset += amount
	// Don't limit here - let the caller determine the actual limit
	
	return s.viewportOffset
}

// ScrollUpWithLimit scrolls up by the given amount with a maximum limit.
func (s *ScrollbackBuffer) ScrollUpWithLimit(amount int, actualLimit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset += amount
	if s.viewportOffset > actualLimit {
		s.viewportOffset = actualLimit
	}
	
	return s.viewportOffset
}

// ScrollDown scrolls down by the given amount, stopping at zero.
func (s *ScrollbackBuffer) ScrollDown(amount int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset -= amount
	if s.viewportOffset < 0 {
		s.viewportOffset = 0
	}
	
	return s.viewportOffset
}

// ScrollToTop scrolls to the top of the scrollback buffer.
func (s *ScrollbackBuffer) ScrollToTop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = s.maxLines
}

// ScrollToBottom scrolls to the bottom of the terminal output.
func (s *ScrollbackBuffer) ScrollToBottom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = 0
}

// ResetScroll resets the scroll position to the bottom.
func (s *ScrollbackBuffer) ResetScroll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = 0
}

// GetOffset returns the current viewport offset from the bottom.
func (s *ScrollbackBuffer) GetOffset() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewportOffset
}

// IsScrolled returns true if the viewport is scrolled up from the bottom.
func (s *ScrollbackBuffer) IsScrolled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewportOffset > 0
}

// SetMaxLines changes the maximum number of scrollback lines.
func (s *ScrollbackBuffer) SetMaxLines(maxLines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if maxLines <= 0 {
		maxLines = 10000
	}
	s.maxLines = maxLines
	
	if s.viewportOffset > maxLines {
		s.viewportOffset = maxLines
	}
}

// GetMaxLines returns the maximum number of scrollback lines allowed.
func (s *ScrollbackBuffer) GetMaxLines() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxLines
}