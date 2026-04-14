package ui

import (
	"strings"
	"testing"
)

func TestComputeDiffOpsFallsBackForLargeInputs(t *testing.T) {
	oldLines := make([]string, 3000)
	newLines := make([]string, 3000)
	for i := range oldLines {
		oldLines[i] = "old " + strings.Repeat("x", 4)
		newLines[i] = "new " + strings.Repeat("x", 4)
	}

	ops := computeDiffOps(oldLines, newLines)
	if len(ops) == 0 {
		t.Fatalf("expected diff ops, got none")
	}
	if len(ops) > maxRenderedDiffOps+1 {
		t.Fatalf("expected truncated preview, got %d ops", len(ops))
	}
	last := ops[len(ops)-1]
	if !strings.Contains(last.text, "truncated") {
		t.Fatalf("expected truncated marker in last op, got %q", last.text)
	}
}
