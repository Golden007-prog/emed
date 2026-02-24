package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

func normalizePastedText(text string) string {
	if strings.IndexAny(text, "\r\u0085\u2028\u2029\f\v") < 0 {
		return text
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\u0085", "\n")
	text = strings.ReplaceAll(text, "\u2028", "\n")
	text = strings.ReplaceAll(text, "\u2029", "\n")
	text = strings.ReplaceAll(text, "\f", "\n")
	text = strings.ReplaceAll(text, "\v", "\n")
	return text
}

func isLineBreakRune(r rune) bool {
	switch r {
	case '\n', '\r', '\u0085', '\u2028', '\u2029', '\f', '\v':
		return true
	default:
		return false
	}
}

func isLineBreakKey(event *tcell.EventKey) bool {
	if event == nil {
		return false
	}

	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlJ:
		return true
	case tcell.KeyRune:
		return isLineBreakRune(event.Rune())
	default:
		return false
	}
}
