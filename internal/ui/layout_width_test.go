package ui

import "testing"

func TestCalculateFileManagerWidth(t *testing.T) {
	l := &Layout{}

	tests := []struct {
		name          string
		terminalWidth int
		expect        int
	}{
		{name: "narrow terminal keeps fixed width", terminalWidth: 80, expect: 30},
		{name: "boundary at 150 keeps fixed width", terminalWidth: 150, expect: 30},
		{name: "wide terminal uses fifth of width", terminalWidth: 200, expect: 40},
		{name: "cap boundary exact", terminalWidth: 225, expect: 45},
		{name: "very wide terminal is capped", terminalWidth: 300, expect: maxFileManagerWidth},
		{name: "ultra wide terminal is capped", terminalWidth: 500, expect: maxFileManagerWidth},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := l.calculateFileManagerWidth(tc.terminalWidth); got != tc.expect {
				t.Fatalf("width for terminal %d: got %d want %d", tc.terminalWidth, got, tc.expect)
			}
		})
	}
}
