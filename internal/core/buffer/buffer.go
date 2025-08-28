package buffer

import (
	"bufio"
	"os"
	"strings"
)

type TextBuffer struct {
	lines       [][]rune
	lineEndings string
	modified    bool
	tabSize     int
}

func New() *TextBuffer {
	return &TextBuffer{
		lines:       [][]rune{{}},
		lineEndings: "\n",
		tabSize:     4,
		modified:    false,
	}
}

func (b *TextBuffer) LoadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines [][]rune
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	hasContent := false
	for scanner.Scan() {
		hasContent = true
		line := scanner.Text()
		lines = append(lines, []rune(line))
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	if !hasContent {
		lines = [][]rune{{}}
	}

	b.lines = lines
	b.detectLineEndings(path)
	b.modified = false
	return nil
}

func (b *TextBuffer) SaveFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for i, line := range b.lines {
		_, err := writer.WriteString(string(line))
		if err != nil {
			return err
		}
		if i < len(b.lines)-1 {
			_, err := writer.WriteString(b.lineEndings)
			if err != nil {
				return err
			}
		}
	}

	err = writer.Flush()
	if err == nil {
		b.modified = false
	}
	return err
}

func (b *TextBuffer) SetContent(content string) {
	if content == "" {
		b.lines = [][]rune{{}}
	} else {
		lines := strings.Split(content, "\n")
		b.lines = make([][]rune, len(lines))
		for i, line := range lines {
			b.lines[i] = []rune(line)
		}
	}
	b.modified = true
}

func (b *TextBuffer) GetContent() string {
	var builder strings.Builder
	for i, line := range b.lines {
		builder.WriteString(string(line))
		if i < len(b.lines)-1 {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func (b *TextBuffer) GetLines() [][]rune {
	return b.lines
}

func (b *TextBuffer) GetLine(row int) []rune {
	if row < 0 || row >= len(b.lines) {
		return []rune{}
	}
	return b.lines[row]
}

func (b *TextBuffer) GetLineCount() int {
	return len(b.lines)
}

func (b *TextBuffer) GetLineLength(row int) int {
	if row < 0 || row >= len(b.lines) {
		return 0
	}
	return len(b.lines[row])
}

func (b *TextBuffer) InsertRune(row, col int, r rune) {
	if row < 0 || row >= len(b.lines) {
		return
	}
	
	line := b.lines[row]
	if col > len(line) {
		col = len(line)
	}
	
	newLine := make([]rune, 0, len(line)+1)
	newLine = append(newLine, line[:col]...)
	newLine = append(newLine, r)
	newLine = append(newLine, line[col:]...)
	b.lines[row] = newLine
	b.modified = true
}

func (b *TextBuffer) InsertNewline(row, col int) (newRow, newCol int) {
	if row < 0 || row >= len(b.lines) {
		return row, col
	}
	
	currentLine := b.lines[row]
	if col > len(currentLine) {
		col = len(currentLine)
	}
	
	before := currentLine[:col]
	after := currentLine[col:]
	
	b.lines[row] = before
	
	newLines := make([][]rune, len(b.lines)+1)
	copy(newLines[:row+1], b.lines[:row+1])
	newLines[row+1] = after
	copy(newLines[row+2:], b.lines[row+1:])
	b.lines = newLines
	
	b.modified = true
	return row + 1, 0
}

func (b *TextBuffer) DeleteCharBackward(row, col int) (newRow, newCol int, deleted bool) {
	if row < 0 || row >= len(b.lines) {
		return row, col, false
	}
	
	if col > 0 {
		line := b.lines[row]
		if col > len(line) {
			col = len(line)
		}
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:col-1]...)
		newLine = append(newLine, line[col:]...)
		b.lines[row] = newLine
		b.modified = true
		return row, col - 1, true
	} else if row > 0 {
		prevLine := b.lines[row-1]
		currentLine := b.lines[row]
		b.lines[row-1] = append(prevLine, currentLine...)
		
		newLines := make([][]rune, len(b.lines)-1)
		copy(newLines[:row], b.lines[:row])
		copy(newLines[row:], b.lines[row+1:])
		b.lines = newLines
		
		b.modified = true
		return row - 1, len(prevLine), true
	}
	
	return row, col, false
}

func (b *TextBuffer) DeleteCharForward(row, col int) (deleted bool) {
	if row < 0 || row >= len(b.lines) {
		return false
	}
	
	line := b.lines[row]
	if col < len(line) {
		newLine := make([]rune, 0, len(line)-1)
		newLine = append(newLine, line[:col]...)
		newLine = append(newLine, line[col+1:]...)
		b.lines[row] = newLine
		b.modified = true
		return true
	} else if row < len(b.lines)-1 {
		currentLine := b.lines[row]
		nextLine := b.lines[row+1]
		b.lines[row] = append(currentLine, nextLine...)
		
		newLines := make([][]rune, len(b.lines)-1)
		copy(newLines[:row+1], b.lines[:row+1])
		copy(newLines[row+1:], b.lines[row+2:])
		b.lines = newLines
		
		b.modified = true
		return true
	}
	
	return false
}

func (b *TextBuffer) DeleteRange(startRow, startCol, endRow, endCol int) {
	if startRow < 0 || startRow >= len(b.lines) || endRow < 0 || endRow >= len(b.lines) {
		return
	}
	
	if startRow == endRow {
		line := b.lines[startRow]
		if startCol > len(line) {
			startCol = len(line)
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		newLine := make([]rune, 0, len(line)-(endCol-startCol))
		newLine = append(newLine, line[:startCol]...)
		newLine = append(newLine, line[endCol:]...)
		b.lines[startRow] = newLine
	} else {
		startLine := b.lines[startRow]
		endLine := b.lines[endRow]
		
		if startCol > len(startLine) {
			startCol = len(startLine)
		}
		if endCol > len(endLine) {
			endCol = len(endLine)
		}
		
		newLine := make([]rune, 0, len(startLine[:startCol])+len(endLine[endCol:]))
		newLine = append(newLine, startLine[:startCol]...)
		newLine = append(newLine, endLine[endCol:]...)
		
		linesToDelete := endRow - startRow
		newLines := make([][]rune, len(b.lines)-linesToDelete)
		copy(newLines[:startRow], b.lines[:startRow])
		newLines[startRow] = newLine
		copy(newLines[startRow+1:], b.lines[endRow+1:])
		b.lines = newLines
	}
	
	b.modified = true
}

func (b *TextBuffer) InsertText(row, col int, text string) (newRow, newCol int) {
	if row < 0 || row >= len(b.lines) {
		return row, col
	}
	
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		for _, r := range text {
			b.InsertRune(row, col, r)
			col++
		}
		return row, col
	}
	
	currentLine := b.lines[row]
	if col > len(currentLine) {
		col = len(currentLine)
	}
	
	before := currentLine[:col]
	after := currentLine[col:]
	
	b.lines[row] = append(before, []rune(lines[0])...)
	
	newLines := make([][]rune, len(b.lines)+len(lines)-1)
	copy(newLines[:row+1], b.lines[:row+1])
	
	for i := 1; i < len(lines)-1; i++ {
		newLines[row+i] = []rune(lines[i])
	}
	
	lastLine := []rune(lines[len(lines)-1])
	newLines[row+len(lines)-1] = append(lastLine, after...)
	
	copy(newLines[row+len(lines):], b.lines[row+1:])
	b.lines = newLines
	
	b.modified = true
	return row + len(lines) - 1, len(lastLine)
}

func (b *TextBuffer) IsModified() bool {
	return b.modified
}

func (b *TextBuffer) SetModified(modified bool) {
	b.modified = modified
}

func (b *TextBuffer) GetTabSize() int {
	return b.tabSize
}

func (b *TextBuffer) SetTabSize(size int) {
	if size > 0 {
		b.tabSize = size
	}
}

func (b *TextBuffer) detectLineEndings(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	
	content := string(data)
	if strings.Contains(content, "\r\n") {
		b.lineEndings = "\r\n"
	} else if strings.Contains(content, "\r") {
		b.lineEndings = "\r"
	} else {
		b.lineEndings = "\n"
	}
}

func (b *TextBuffer) GetTextRange(startRow, startCol, endRow, endCol int) string {
	if startRow < 0 || startRow >= len(b.lines) || endRow < 0 || endRow >= len(b.lines) {
		return ""
	}
	
	if startRow == endRow {
		line := b.lines[startRow]
		if startCol > len(line) {
			startCol = len(line)
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		return string(line[startCol:endCol])
	}
	
	var result strings.Builder
	
	startLine := b.lines[startRow]
	if startCol > len(startLine) {
		startCol = len(startLine)
	}
	result.WriteString(string(startLine[startCol:]))
	result.WriteString("\n")
	
	for i := startRow + 1; i < endRow; i++ {
		result.WriteString(string(b.lines[i]))
		result.WriteString("\n")
	}
	
	endLine := b.lines[endRow]
	if endCol > len(endLine) {
		endCol = len(endLine)
	}
	result.WriteString(string(endLine[:endCol]))
	
	return result.String()
}