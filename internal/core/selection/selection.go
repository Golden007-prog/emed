// Package selection handles text selection functionality for the editor.
package selection

// Position represents a cursor position with row and column coordinates.
type Position struct {
	Row int
	Col int
}

// Selection manages text selection state including keyboard and mouse selections.
type Selection struct {
	start        Position
	end          Position
	active       bool
	mouseDragging bool // Track if currently dragging with mouse
}

// New creates a new Selection instance with inactive state.
func New() *Selection {
	return &Selection{
		active: false,
	}
}

// Start begins a new keyboard selection at the given position.
func (s *Selection) Start(row, col int) {
	s.start = Position{Row: row, Col: col}
	s.end = Position{Row: row, Col: col}
	s.active = true
	s.mouseDragging = false // This is for keyboard selection
}

// StartMouseDrag begins a new mouse drag selection at the given position.
func (s *Selection) StartMouseDrag(row, col int) {
	s.start = Position{Row: row, Col: col}
	s.end = Position{Row: row, Col: col}
	s.active = true
	s.mouseDragging = true
}

// Extend extends the current keyboard selection to the given position.
func (s *Selection) Extend(row, col int) {
	if s.active && !s.mouseDragging {
		// Keyboard selection - always extend
		s.end = Position{Row: row, Col: col}
	}
}

// ExtendMouse extends the current mouse selection to the given position.
func (s *Selection) ExtendMouse(row, col int) {
	if s.active && s.mouseDragging {
		// Mouse selection - only extend if dragging
		s.end = Position{Row: row, Col: col}
	}
}

// StopMouseDrag stops the mouse drag operation and clears empty selections.
func (s *Selection) StopMouseDrag() {
	s.mouseDragging = false
	// If selection is empty (start == end), clear it
	if s.start == s.end {
		s.Clear()
	}
}

// IsMouseDragging returns true if currently dragging with mouse.
func (s *Selection) IsMouseDragging() bool {
	return s.mouseDragging
}

// Clear resets the selection to inactive state.
func (s *Selection) Clear() {
	s.active = false
	s.mouseDragging = false
	s.start = Position{}
	s.end = Position{}
}

// IsActive returns true if there is an active selection.
func (s *Selection) IsActive() bool {
	return s.active
}

// GetStart returns the normalized start position of the selection.
func (s *Selection) GetStart() Position {
	if !s.active {
		return Position{}
	}
	
	if s.start.Row < s.end.Row || (s.start.Row == s.end.Row && s.start.Col <= s.end.Col) {
		return s.start
	}
	return s.end
}

// GetEnd returns the normalized end position of the selection.
func (s *Selection) GetEnd() Position {
	if !s.active {
		return Position{}
	}
	
	if s.start.Row > s.end.Row || (s.start.Row == s.end.Row && s.start.Col > s.end.Col) {
		return s.start
	}
	return s.end
}

// GetNormalizedRange returns the selection range with start before end.
func (s *Selection) GetNormalizedRange() (startRow, startCol, endRow, endCol int) {
	if !s.active {
		return 0, 0, 0, 0
	}
	
	start := s.GetStart()
	end := s.GetEnd()
	return start.Row, start.Col, end.Row, end.Col
}

// IsPositionSelected checks if the given position is within the selection.
func (s *Selection) IsPositionSelected(row, col int) bool {
	if !s.active {
		return false
	}
	
	startRow, startCol, endRow, endCol := s.GetNormalizedRange()
	
	if row < startRow || row > endRow {
		return false
	}
	
	if row == startRow && row == endRow {
		return col >= startCol && col < endCol
	}
	
	if row == startRow {
		return col >= startCol
	}
	
	if row == endRow {
		return col < endCol
	}
	
	return true
}

// SelectAll selects all content from start to end of the document.
func (s *Selection) SelectAll(totalRows int, lastRowLength int) {
	s.start = Position{Row: 0, Col: 0}
	s.end = Position{Row: totalRows - 1, Col: lastRowLength}
	s.active = true
}

// UpdateAfterEdit adjusts selection positions after text editing operations.
func (s *Selection) UpdateAfterEdit(editRow, editCol int, rowDelta, colDelta int) {
	if !s.active {
		return
	}
	
	if s.start.Row > editRow {
		s.start.Row += rowDelta
	} else if s.start.Row == editRow && s.start.Col > editCol {
		s.start.Col += colDelta
	}
	
	if s.end.Row > editRow {
		s.end.Row += rowDelta
	} else if s.end.Row == editRow && s.end.Col > editCol {
		s.end.Col += colDelta
	}
	
	if s.start.Row < 0 {
		s.start.Row = 0
	}
	if s.start.Col < 0 {
		s.start.Col = 0
	}
	if s.end.Row < 0 {
		s.end.Row = 0
	}
	if s.end.Col < 0 {
		s.end.Col = 0
	}
}