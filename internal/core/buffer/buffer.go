// Package buffer provides text buffer functionality with undo/redo support.
package buffer

import (
	"bufio"
	"os"
	"strings"
)

// BufferState represents a snapshot of the buffer at a point in time
type BufferState struct {
	lines    [][]rune
	cursorX  int
	cursorY  int
}

// copyLines creates a deep copy of the lines
func copyLines(lines [][]rune) [][]rune {
	copied := make([][]rune, len(lines))
	for i, line := range lines {
		copied[i] = make([]rune, len(line))
		copy(copied[i], line)
	}
	return copied
}

// linesEqual checks if two sets of lines are identical
func linesEqual(a, b [][]rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// TextBuffer represents a text buffer with undo/redo functionality.
type TextBuffer struct {
	lines       [][]rune
	lineEndings string
	modified    bool
	tabSize     int
	
	// Undo/Redo stacks
	undoStack   []BufferState
	redoStack   []BufferState
	maxUndoSize int
	initialState [][]rune
	
	// Track if we should record this change
	recordingUndo bool
}

// New creates a new empty text buffer.
func New() *TextBuffer {
	initialLines := [][]rune{{}}
	return &TextBuffer{
		lines:         copyLines(initialLines),
		lineEndings:   "\n",
		tabSize:       4,
		modified:      false,
		undoStack:     make([]BufferState, 0),
		redoStack:     make([]BufferState, 0),
		maxUndoSize:   100, // Keep last 100 undo states
		recordingUndo: true,
		initialState:  initialLines,
	}
}

// LoadFile loads content from a file into the buffer.
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
	b.initialState = copyLines(lines) // Save initial state
	b.ClearUndoHistory() // Clear undo history when loading a new file
	return nil
}

// SaveFile saves the buffer content to a file.
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

// SetContent sets the buffer content from a string.
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

// GetContent returns the buffer content as a string.
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

// GetLines returns all lines in the buffer.
func (b *TextBuffer) GetLines() [][]rune {
	return b.lines
}

// GetLine returns a specific line from the buffer.
func (b *TextBuffer) GetLine(row int) []rune {
	if row < 0 || row >= len(b.lines) {
		return []rune{}
	}
	return b.lines[row]
}

// GetLineCount returns the total number of lines in the buffer.
func (b *TextBuffer) GetLineCount() int {
	return len(b.lines)
}

// GetLineLength returns the length of a specific line.
func (b *TextBuffer) GetLineLength(row int) int {
	if row < 0 || row >= len(b.lines) {
		return 0
	}
	return len(b.lines[row])
}

// InsertRune inserts a rune at the specified position.
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

// InsertNewline inserts a newline at the specified position.
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

// DeleteCharBackward deletes the character before the cursor position.
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

// DeleteCharForward deletes the character at the cursor position.
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

// DeleteRange deletes text between two positions.
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

// InsertText inserts text at the specified position.
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
	
	before := make([]rune, col)
	copy(before, currentLine[:col])
	after := make([]rune, len(currentLine)-col)
	copy(after, currentLine[col:])
	
	newLines := make([][]rune, len(b.lines)+len(lines)-1)
	copy(newLines[:row], b.lines[:row])
	
	firstLine := make([]rune, 0, len(before)+len(lines[0]))
	firstLine = append(firstLine, before...)
	firstLine = append(firstLine, []rune(lines[0])...)
	newLines[row] = firstLine
	
	for i := 1; i < len(lines)-1; i++ {
		newLines[row+i] = []rune(lines[i])
	}
	
	lastLineIndex := row + len(lines) - 1
	if lastLineIndex != row {
		lastLine := make([]rune, 0, len(lines[len(lines)-1])+len(after))
		lastLine = append(lastLine, []rune(lines[len(lines)-1])...)
		lastLine = append(lastLine, after...)
		newLines[lastLineIndex] = lastLine
	}
	
	copy(newLines[row+len(lines):], b.lines[row+1:])
	b.lines = newLines
	
	b.modified = true
	return row + len(lines) - 1, len([]rune(lines[len(lines)-1]))
}

// IsModified returns true if the buffer has been modified.
func (b *TextBuffer) IsModified() bool {
	return b.modified
}

// SetModified sets the modified state of the buffer.
func (b *TextBuffer) SetModified(modified bool) {
	b.modified = modified
}

// GetTabSize returns the tab size setting.
func (b *TextBuffer) GetTabSize() int {
	return b.tabSize
}

// SetTabSize sets the tab size setting.
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

// GetTextRange returns text content between two positions.
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

// SaveState saves the current buffer state to the undo stack.
func (b *TextBuffer) SaveState(cursorRow, cursorCol int) {
	if !b.recordingUndo {
		return
	}
	
	
	// Create a snapshot of current state
	state := BufferState{
		lines:   copyLines(b.lines),
		cursorX: cursorCol,
		cursorY: cursorRow,
	}
	
	// Add to undo stack
	b.undoStack = append(b.undoStack, state)
	
	// Limit undo stack size
	if len(b.undoStack) > b.maxUndoSize {
		b.undoStack = b.undoStack[1:]
	}
	
	// Clear redo stack when new change is made
	b.redoStack = b.redoStack[:0]
}

// Undo reverts to the previous state.
func (b *TextBuffer) Undo() (cursorX, cursorY int, success bool) {
	
	if len(b.undoStack) == 0 {
		return 0, 0, false
	}
	
	// Save current state to redo stack before undoing
	currentState := BufferState{
		lines:   copyLines(b.lines),
		cursorX: 0, // Will be set by caller
		cursorY: 0,
	}
	b.redoStack = append(b.redoStack, currentState)
	
	// Pop from undo stack
	state := b.undoStack[len(b.undoStack)-1]
	b.undoStack = b.undoStack[:len(b.undoStack)-1]
	
	
	// Restore state without recording undo
	b.recordingUndo = false
	b.lines = copyLines(state.lines)
	
	// Check if we're back to the initial state
	if b.initialState != nil && linesEqual(b.lines, b.initialState) {
		b.modified = false
	} else {
		b.modified = true
	}
	
	b.recordingUndo = true
	
	return state.cursorX, state.cursorY, true
}

// Redo reapplies a previously undone change.
func (b *TextBuffer) Redo() (cursorX, cursorY int, success bool) {
	if len(b.redoStack) == 0 {
		return 0, 0, false
	}
	
	// Save current state to undo stack before redoing
	currentState := BufferState{
		lines:   copyLines(b.lines),
		cursorX: 0,
		cursorY: 0,
	}
	b.undoStack = append(b.undoStack, currentState)
	
	// Pop from redo stack
	state := b.redoStack[len(b.redoStack)-1]
	b.redoStack = b.redoStack[:len(b.redoStack)-1]
	
	// Restore state without recording undo
	b.recordingUndo = false
	b.lines = copyLines(state.lines)
	
	// Check if we're back to the initial state
	if b.initialState != nil && linesEqual(b.lines, b.initialState) {
		b.modified = false
	} else {
		b.modified = true
	}
	
	b.recordingUndo = true
	
	return state.cursorX, state.cursorY, true
}

// CanUndo returns true if undo is available.
func (b *TextBuffer) CanUndo() bool {
	return len(b.undoStack) > 0
}

// CanRedo returns true if redo is available.
func (b *TextBuffer) CanRedo() bool {
	return len(b.redoStack) > 0
}

// ClearUndoHistory clears both undo and redo stacks.
func (b *TextBuffer) ClearUndoHistory() {
	b.undoStack = b.undoStack[:0]
	b.redoStack = b.redoStack[:0]
}