package ui

// bellScanner detects audible BEL characters in a PTY output stream.
// It keeps a minimal parser state across chunks, so BEL bytes that act as
// terminator of OSC/DCS style strings are not counted as bells.
type bellScanner struct {
	state bellScanState
}

type bellScanState int

const (
	bellStateGround bellScanState = iota
	bellStateEscape
	bellStateCSI
	bellStateString
	bellStateStringEscape
)

const (
	byteBEL = 0x07
	byteESC = 0x1b
)

// Scan consumes a chunk of terminal output and returns how many audible
// bells it contains. State is kept between calls, so sequences splitted
// across chunks are handled correctly.
func (s *bellScanner) Scan(data []byte) int {
	bells := 0

	for _, b := range data {
		switch s.state {
		case bellStateGround:
			switch b {
			case byteBEL:
				bells++
			case byteESC:
				s.state = bellStateEscape
			}

		case bellStateEscape:
			switch b {
			case '[':
				s.state = bellStateCSI
			case ']', 'P', '^', '_', 'X', 'k':
				s.state = bellStateString
			default:
				s.state = bellStateGround
			}

		case bellStateCSI:
			switch {
			case b == byteBEL:
				bells++
			case b == byteESC:
				s.state = bellStateEscape
			case b >= 0x40 && b <= 0x7e:
				s.state = bellStateGround
			}

		case bellStateString:
			switch b {
			case byteBEL:
				s.state = bellStateGround
			case byteESC:
				s.state = bellStateStringEscape
			}

		case bellStateStringEscape:
			s.state = bellStateGround
		}
	}

	return bells
}
