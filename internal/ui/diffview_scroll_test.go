package ui

import (
	"strings"
	"testing"
)

func TestCountDiffChanges(t *testing.T) {
	ops := []diffOp{
		{kind: diffContext},
		{kind: diffRemoved},
		{kind: diffRemoved},
		{kind: diffAdded},
		{kind: diffContext},
		{kind: diffAdded},
	}

	added, removed := countDiffChanges(ops)
	if added != 2 || removed != 2 {
		t.Fatalf("unexpected stats: +%d -%d", added, removed)
	}
}

func TestChangeStartRows(t *testing.T) {
	ops := []diffOp{
		{kind: diffContext},
		{kind: diffRemoved},
		{kind: diffAdded},
		{kind: diffContext},
		{kind: diffContext},
		{kind: diffAdded},
	}

	rows := changeStartRows(ops, diffBodyHeaderRows)
	if len(rows) != 2 {
		t.Fatalf("expected 2 change blocks, got %v", rows)
	}
	if rows[0] != diffBodyHeaderRows+1 || rows[1] != diffBodyHeaderRows+5 {
		t.Fatalf("unexpected block rows: %v", rows)
	}
}

func TestDiffHeaderShowsSessionOnlyWhenMultiple(t *testing.T) {
	single := NewDiffView("/proj/main.go", "a\n", "b\n", "diff", DiffMeta{
		ProjectName: "proj", ProjectRoot: "/proj", SessionOrdinal: 1, Sessions: 1,
	})
	if strings.Contains(single.header.GetText(false), "claude session") {
		t.Fatalf("session info should be hidden with one session")
	}

	multi := NewDiffView("/proj/main.go", "a\n", "b\n", "diff", DiffMeta{
		ProjectName: "proj", ProjectRoot: "/proj", SessionOrdinal: 2, Sessions: 3,
	})
	header := multi.header.GetText(false)
	if !strings.Contains(header, "claude session") || !strings.Contains(header, "2 of 3") {
		t.Fatalf("expected session info in header, got: %q", header)
	}
}

func TestDiffHeaderRelativizesPathAndShowsStats(t *testing.T) {
	dv := NewDiffView("/proj/internal/app.go", "one\ntwo\n", "one\nchanged\nadded\n", "diff", DiffMeta{
		ProjectName: "proj", ProjectRoot: "/proj",
	})

	header := dv.header.GetText(false)
	if !strings.Contains(header, "internal/app.go") || strings.Contains(header, "/proj/internal") {
		t.Fatalf("expected project relative path, got: %q", header)
	}
	if !strings.Contains(header, "+2") || !strings.Contains(header, "-1") {
		t.Fatalf("expected +2 -1 stats, got: %q", header)
	}
	if !strings.Contains(header, "proj") {
		t.Fatalf("expected project name, got: %q", header)
	}
}

func TestDiffFooterShowsKeys(t *testing.T) {
	dv := NewDiffView("/proj/a.go", "x\n", "y\n", "diff", DiffMeta{})

	footer := dv.footer.GetText(false)
	if !strings.Contains(footer, "Enter") || !strings.Contains(footer, "Esc") {
		t.Fatalf("footer must always show accept/reject keys, got: %q", footer)
	}
}
