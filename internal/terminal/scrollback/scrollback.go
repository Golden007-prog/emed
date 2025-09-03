package scrollback

import (
	"sync"
)

type ScrollbackBuffer struct {
	maxLines       int
	viewportOffset int
	mu             sync.RWMutex
}

func New(maxLines int) *ScrollbackBuffer {
	if maxLines <= 0 {
		maxLines = 10000
	}
	return &ScrollbackBuffer{
		maxLines:       maxLines,
		viewportOffset: 0,
	}
}

func (s *ScrollbackBuffer) ScrollUp(amount int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset += amount
	// Don't limit here - let the caller determine the actual limit
	
	return s.viewportOffset
}

func (s *ScrollbackBuffer) ScrollUpWithLimit(amount int, actualLimit int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset += amount
	if s.viewportOffset > actualLimit {
		s.viewportOffset = actualLimit
	}
	
	return s.viewportOffset
}

func (s *ScrollbackBuffer) ScrollDown(amount int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.viewportOffset -= amount
	if s.viewportOffset < 0 {
		s.viewportOffset = 0
	}
	
	return s.viewportOffset
}

func (s *ScrollbackBuffer) ScrollToTop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = s.maxLines
}

func (s *ScrollbackBuffer) ScrollToBottom() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = 0
}

func (s *ScrollbackBuffer) ResetScroll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewportOffset = 0
}

func (s *ScrollbackBuffer) GetOffset() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewportOffset
}

func (s *ScrollbackBuffer) IsScrolled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.viewportOffset > 0
}

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

func (s *ScrollbackBuffer) GetMaxLines() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxLines
}