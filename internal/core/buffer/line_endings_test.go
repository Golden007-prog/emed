package buffer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSetContentCRLFSaveDoesNotDuplicateCR(t *testing.T) {
	input := "line1\r\nline2\r\n"

	b := New()
	b.SetContent(input)

	path := filepath.Join(t.TempDir(), "out.txt")
	if err := b.SaveFile(path); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != input {
		t.Fatalf("unexpected output: got %q want %q", string(data), input)
	}

	if bytes.Contains(data, []byte("\r\r\n")) {
		t.Fatalf("output contains duplicated CR before CRLF: %q", string(data))
	}
}

func TestLoadFileCRLFInsertNewlineSaveKeepsCRLF(t *testing.T) {
	input := "aa\r\nbb\r\n"
	expected := "a\r\na\r\nbb\r\n"

	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "in.txt")
	outputPath := filepath.Join(tempDir, "out.txt")

	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	b := New()
	if err := b.LoadFile(inputPath); err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	b.InsertNewline(0, 1)

	if err := b.SaveFile(outputPath); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != expected {
		t.Fatalf("unexpected output: got %q want %q", string(data), expected)
	}

	if bytes.Contains(data, []byte("\r\r\n")) {
		t.Fatalf("output contains duplicated CR before CRLF: %q", string(data))
	}
}

func TestSetContentCRLineEndingsSavePreserved(t *testing.T) {
	input := "one\rtwo\r"

	b := New()
	b.SetContent(input)

	path := filepath.Join(t.TempDir(), "out.txt")
	if err := b.SaveFile(path); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(data) != input {
		t.Fatalf("unexpected output: got %q want %q", string(data), input)
	}
}
