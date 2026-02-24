package ui

import "strings"

func normalizePastedText(text string) string {
	if strings.IndexByte(text, '\r') < 0 {
		return text
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}
