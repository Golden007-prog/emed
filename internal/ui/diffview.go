package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type diffOpKind int

const (
	diffContext diffOpKind = iota
	diffAdded
	diffRemoved
)

type diffOp struct {
	kind   diffOpKind
	text   string
	oldNum int
	newNum int
}

const (
	maxLCSCells        int64 = 4_000_000
	maxRenderedDiffOps       = 5000
)

type DiffView struct {
	*tview.TextView

	filePath   string
	tabName    string
	oldContent string
	newContent string

	resultChan chan string
	resolved   bool
	mu         sync.Mutex
}

func NewDiffView(filePath, oldContent, newContent, tabName string) *DiffView {
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(true)

	d := &DiffView{
		TextView:   view,
		filePath:   filePath,
		tabName:    tabName,
		oldContent: oldContent,
		newContent: newContent,
		resultChan: make(chan string, 1),
	}

	d.SetBorder(false)
	content, scrollRow := d.renderContent()
	d.SetText(content)
	d.scrollToChange(scrollRow)
	d.SetInputCapture(d.captureInput)

	return d
}

func (d *DiffView) captureInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlS:
		d.Accept()
		return nil
	}

	return event
}

func (d *DiffView) SetApp(app *tview.Application) {
	_ = app
}

func (d *DiffView) GetFilePath() string {
	return d.filePath
}

func (d *DiffView) GetTabName() string {
	if d.tabName != "" {
		return d.tabName
	}
	return "Diff: " + d.filePath
}

func (d *DiffView) WaitForResult() string {
	return <-d.resultChan
}

func (d *DiffView) Accept() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.resolved {
		return
	}
	d.resolved = true
	d.resultChan <- "FILE_SAVED"
}

func (d *DiffView) Reject() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.resolved {
		return
	}
	d.resolved = true
	d.resultChan <- "DIFF_REJECTED"
}

func (d *DiffView) GetNewContent() string {
	return d.newContent
}

func (d *DiffView) renderContent() (string, int) {
	oldLines := splitDiffLines(d.oldContent)
	newLines := splitDiffLines(d.newContent)
	ops := computeDiffOps(oldLines, newLines)
	changeIdx, _, _, hasChanges := firstChangedOp(ops)

	body := renderFallbackDiff(d.filePath, oldLines, newLines, ops)
	scrollRow := -1
	if hasChanges {
		scrollRow = 2 + 3 + changeIdx
	}
	return formatRenderedDiff("internal", body), scrollRow
}

func (d *DiffView) scrollToChange(scrollRow int) {
	if scrollRow < 0 {
		d.ScrollToBeginning()
		return
	}

	row := scrollRow - 2
	if row < 0 {
		row = 0
	}
	d.ScrollTo(row, 0)
}

func formatRenderedDiff(renderer string, body string) string {
	return fmt.Sprintf("[gray]Renderer: %s[-]   [gray]Enter: accept   Esc: reject   Scroll: arrows / PgUp / PgDn[-]\n\n%s", tview.Escape(renderer), body)
}

func splitDiffLines(content string) []string {
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
}

func firstChangedOp(ops []diffOp) (int, int, int, bool) {
	for i, op := range ops {
		if op.kind != diffContext {
			return i, op.oldNum, op.newNum, true
		}
	}
	return -1, 0, 0, false
}

func computeDiffOps(oldLines, newLines []string) []diffOp {
	if shouldUseLinearDiff(oldLines, newLines) {
		return computeLinearDiffOps(oldLines, newLines, maxRenderedDiffOps)
	}

	lcs := buildLCSMatrix(oldLines, newLines)

	i, j := 0, 0
	oldNum, newNum := 1, 1
	ops := make([]diffOp, 0, len(oldLines)+len(newLines))

	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			ops = append(ops, diffOp{
				kind:   diffContext,
				text:   oldLines[i],
				oldNum: oldNum,
				newNum: newNum,
			})
			i++
			j++
			oldNum++
			newNum++
			continue
		}

		if j < len(newLines) && (i == len(oldLines) || lcs[i][j+1] >= lcs[i+1][j]) {
			ops = append(ops, diffOp{
				kind:   diffAdded,
				text:   newLines[j],
				oldNum: 0,
				newNum: newNum,
			})
			j++
			newNum++
			continue
		}

		if i < len(oldLines) {
			ops = append(ops, diffOp{
				kind:   diffRemoved,
				text:   oldLines[i],
				oldNum: oldNum,
				newNum: 0,
			})
			i++
			oldNum++
		}
	}

	return ops
}

func shouldUseLinearDiff(oldLines, newLines []string) bool {
	rows := int64(len(oldLines) + 1)
	cols := int64(len(newLines) + 1)
	return rows*cols > maxLCSCells
}

func computeLinearDiffOps(oldLines, newLines []string, limit int) []diffOp {
	if limit <= 0 {
		limit = maxRenderedDiffOps
	}

	i, j := 0, 0
	oldNum, newNum := 1, 1
	ops := make([]diffOp, 0, min(len(oldLines)+len(newLines), limit+1))
	truncated := false

	appendOp := func(op diffOp) bool {
		if len(ops) >= limit {
			truncated = true
			return false
		}
		ops = append(ops, op)
		return true
	}

	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			if !appendOp(diffOp{kind: diffContext, text: oldLines[i], oldNum: oldNum, newNum: newNum}) {
				break
			}
			i++
			j++
			oldNum++
			newNum++
			continue
		}

		if i < len(oldLines) && (j == len(newLines) || (i+1 < len(oldLines) && oldLines[i+1] == newLines[j])) {
			if !appendOp(diffOp{kind: diffRemoved, text: oldLines[i], oldNum: oldNum}) {
				break
			}
			i++
			oldNum++
			continue
		}

		if j < len(newLines) && (i == len(oldLines) || (j+1 < len(newLines) && oldLines[i] == newLines[j+1])) {
			if !appendOp(diffOp{kind: diffAdded, text: newLines[j], newNum: newNum}) {
				break
			}
			j++
			newNum++
			continue
		}

		if i < len(oldLines) {
			if !appendOp(diffOp{kind: diffRemoved, text: oldLines[i], oldNum: oldNum}) {
				break
			}
			i++
			oldNum++
		}
		if j < len(newLines) {
			if !appendOp(diffOp{kind: diffAdded, text: newLines[j], newNum: newNum}) {
				break
			}
			j++
			newNum++
		}
	}

	if truncated {
		ops = append(ops, diffOp{
			kind: diffContext,
			text: "diff preview truncated for large file; press Enter to accept or Esc to reject",
		})
	}

	return ops
}

func buildLCSMatrix(oldLines, newLines []string) [][]int {
	rows := len(oldLines) + 1
	cols := len(newLines) + 1
	lcs := make([][]int, rows)
	for i := range lcs {
		lcs[i] = make([]int, cols)
	}

	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
				continue
			}
			if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	return lcs
}

func renderFallbackDiff(filePath string, oldLines, newLines []string, ops []diffOp) string {
	oldStart := 1
	newStart := 1
	if len(oldLines) == 0 {
		oldStart = 0
	}
	if len(newLines) == 0 {
		newStart = 0
	}

	oldLabel := "a/" + filepath.ToSlash(filePath)
	newLabel := "b/" + filepath.ToSlash(filePath)

	var b strings.Builder
	fmt.Fprintf(&b, "[teal]--- %s[-]\n", tview.Escape(oldLabel))
	fmt.Fprintf(&b, "[teal]+++ %s[-]\n", tview.Escape(newLabel))
	fmt.Fprintf(&b, "[yellow]@@ -%d,%d +%d,%d @@[-]\n", oldStart, len(oldLines), newStart, len(newLines))

	for _, op := range ops {
		oldNum := ""
		newNum := ""
		if op.oldNum > 0 {
			oldNum = fmt.Sprintf("%d", op.oldNum)
		}
		if op.newNum > 0 {
			newNum = fmt.Sprintf("%d", op.newNum)
		}

		prefix := " "
		color := "white"
		if op.kind == diffAdded {
			prefix = "+"
			color = "green"
		} else if op.kind == diffRemoved {
			prefix = "-"
			color = "red"
		}

		fmt.Fprintf(
			&b,
			"[gray]%6s %6s[-] [%s]%s %s[-]\n",
			oldNum,
			newNum,
			color,
			prefix,
			tview.Escape(op.text),
		)
	}

	return b.String()
}
