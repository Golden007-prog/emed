package ui

import "testing"

func TestBellScannerSingleChunk(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{name: "plain bell", input: "hello\aworld", expect: 1},
		{name: "multiple bells", input: "\a\a\a", expect: 3},
		{name: "no bell", input: "just text", expect: 0},
		{name: "osc title terminated by bel is not a bell", input: "\x1b]0;my title\a", expect: 0},
		{name: "osc 52 clipboard terminated by bel is not a bell", input: "\x1b]52;c;aGVsbG8=\a", expect: 0},
		{name: "osc with st terminator then real bell", input: "\x1b]0;title\x1b\\\a", expect: 1},
		{name: "bell after osc title", input: "\x1b]0;title\adone\a", expect: 1},
		{name: "dcs string ignored", input: "\x1bPsome data\x1b\\", expect: 0},
		{name: "bell inside csi executes", input: "\x1b[3\a1m", expect: 1},
		{name: "csi sequence without bell", input: "\x1b[31mred\x1b[0m", expect: 0},
		{name: "tmux screen title", input: "\x1bktitle\x1b\\\a", expect: 1},
		{name: "apc string ignored", input: "\x1b_payload\x1b\\", expect: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s bellScanner
			if got := s.Scan([]byte(tc.input)); got != tc.expect {
				t.Fatalf("bells for %q: got %d want %d", tc.input, got, tc.expect)
			}
		})
	}
}

func TestBellScannerAcrossChunks(t *testing.T) {
	var s bellScanner

	if got := s.Scan([]byte("\x1b]0;my ti")); got != 0 {
		t.Fatalf("first chunk: got %d bells want 0", got)
	}
	if got := s.Scan([]byte("tle\a")); got != 0 {
		t.Fatalf("second chunk terminates osc: got %d bells want 0", got)
	}
	if got := s.Scan([]byte("\a")); got != 1 {
		t.Fatalf("third chunk is a real bell: got %d bells want 1", got)
	}
}

func TestBellScannerEscSplitAcrossChunks(t *testing.T) {
	var s bellScanner

	if got := s.Scan([]byte("text\x1b")); got != 0 {
		t.Fatalf("chunk ending in esc: got %d bells want 0", got)
	}
	if got := s.Scan([]byte("]0;t\a\a")); got != 1 {
		t.Fatalf("osc continue plus real bell: got %d bells want 1", got)
	}
}
