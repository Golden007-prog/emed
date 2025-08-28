package selection

type Position struct {
	Row int
	Col int
}

type Selection struct {
	start  Position
	end    Position
	active bool
}

func New() *Selection {
	return &Selection{
		active: false,
	}
}

func (s *Selection) Start(row, col int) {
	s.start = Position{Row: row, Col: col}
	s.end = Position{Row: row, Col: col}
	s.active = true
}

func (s *Selection) Extend(row, col int) {
	if s.active {
		s.end = Position{Row: row, Col: col}
	}
}

func (s *Selection) Clear() {
	s.active = false
	s.start = Position{}
	s.end = Position{}
}

func (s *Selection) IsActive() bool {
	return s.active
}

func (s *Selection) GetStart() Position {
	if !s.active {
		return Position{}
	}
	
	if s.start.Row < s.end.Row || (s.start.Row == s.end.Row && s.start.Col <= s.end.Col) {
		return s.start
	}
	return s.end
}

func (s *Selection) GetEnd() Position {
	if !s.active {
		return Position{}
	}
	
	if s.start.Row > s.end.Row || (s.start.Row == s.end.Row && s.start.Col > s.end.Col) {
		return s.start
	}
	return s.end
}

func (s *Selection) GetNormalizedRange() (startRow, startCol, endRow, endCol int) {
	if !s.active {
		return 0, 0, 0, 0
	}
	
	start := s.GetStart()
	end := s.GetEnd()
	return start.Row, start.Col, end.Row, end.Col
}

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

func (s *Selection) SelectAll(totalRows int, lastRowLength int) {
	s.start = Position{Row: 0, Col: 0}
	s.end = Position{Row: totalRows - 1, Col: lastRowLength}
	s.active = true
}

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