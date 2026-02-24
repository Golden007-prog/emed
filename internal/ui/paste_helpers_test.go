package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestNormalizePastedText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "lf", in: "a\nb\n", out: "a\nb\n"},
		{name: "crlf", in: "a\r\nb\r\n", out: "a\nb\n"},
		{name: "cr", in: "a\rb\r", out: "a\nb\n"},
		{name: "unicode-separators", in: "a\u2028b\u2029c\u0085d", out: "a\nb\nc\nd"},
		{name: "other-separators", in: "a\vb\fc", out: "a\nb\nc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePastedText(tc.in)
			if got != tc.out {
				t.Fatalf("unexpected normalized text: got %q want %q", got, tc.out)
			}
		})
	}
}

func TestIsLineBreakRune(t *testing.T) {
	trueCases := []rune{'\n', '\r', '\u0085', '\u2028', '\u2029', '\f', '\v'}
	for _, r := range trueCases {
		if !isLineBreakRune(r) {
			t.Fatalf("expected rune %U to be considered line break", r)
		}
	}

	falseCases := []rune{'a', ' ', '\t', '-', '\u00a0'}
	for _, r := range falseCases {
		if isLineBreakRune(r) {
			t.Fatalf("expected rune %U to not be considered line break", r)
		}
	}
}

func TestIsLineBreakKey(t *testing.T) {
	if isLineBreakKey(nil) {
		t.Fatal("expected nil event to be false")
	}

	trueCases := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlJ, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyCtrlM, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '\n', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '\r', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, '\u2028', tcell.ModNone),
	}
	for _, event := range trueCases {
		if !isLineBreakKey(event) {
			t.Fatalf("expected key %v rune %U to be line break", event.Key(), event.Rune())
		}
	}

	falseCases := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
	}
	for _, event := range falseCases {
		if isLineBreakKey(event) {
			t.Fatalf("expected key %v rune %U to not be line break", event.Key(), event.Rune())
		}
	}
}
