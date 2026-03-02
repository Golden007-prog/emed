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
