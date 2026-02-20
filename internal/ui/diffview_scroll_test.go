package ui

import "testing"

func TestFirstChangedOp(t *testing.T) {
	ops := []diffOp{
		{kind: diffContext, oldNum: 1, newNum: 1},
		{kind: diffContext, oldNum: 2, newNum: 2},
		{kind: diffRemoved, oldNum: 3, newNum: 0},
		{kind: diffAdded, oldNum: 0, newNum: 3},
	}

	idx, oldNum, newNum, ok := firstChangedOp(ops)
	if !ok {
		t.Fatalf("expected first change to be found")
	}
	if idx != 2 || oldNum != 3 || newNum != 0 {
		t.Fatalf("unexpected first change data: idx=%d old=%d new=%d", idx, oldNum, newNum)
	}
}

func TestExtractDeltaLineNumbers(t *testing.T) {
	line := "[#5f87af:-:]  24 [#5f87af:-:]⋮[#5f87af:-:]  25 [#5f87af:-:]│text"
	oldNum, newNum, ok := extractDeltaLineNumbers(line)
	if !ok {
		t.Fatalf("expected gutter numbers to be parsed")
	}
	if oldNum != 24 || newNum != 25 {
		t.Fatalf("unexpected numbers: old=%d new=%d", oldNum, newNum)
	}
}

func TestFindDeltaChangeRow(t *testing.T) {
	rendered := "" +
		"[#5f87af:-:]  21 [#5f87af:-:]⋮[#5f87af:-:]  21 [#5f87af:-:]│same\n" +
		"[#5f87af:-:]  22 [#5f87af:-:]⋮[#5f87af:-:]    [#5f87af:-:]│old line\n" +
		"[#5f87af:-:]    [#5f87af:-:]⋮[#5f87af:-:]  22 [#5f87af:-:]│new line\n"

	row := findDeltaChangeRow(rendered, 22, 0)
	if row != 1 {
		t.Fatalf("unexpected row for removed line: got=%d want=1", row)
	}

	row = findDeltaChangeRow(rendered, 0, 22)
	if row != 2 {
		t.Fatalf("unexpected row for added line: got=%d want=2", row)
	}
}

