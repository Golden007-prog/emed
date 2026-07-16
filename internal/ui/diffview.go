package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/theme"
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

// DiffMeta carries context info shown in the diff header, so the user
// knows which file, project and Claude session produced the diff.
type DiffMeta struct {
	ProjectName    string
	ProjectRoot    string
	SessionOrdinal int
	Sessions       int
}

// diffBodyHeaderRows is the count of rows (---/+++/@@) rendered in the
// body before the first diff op.
const diffBodyHeaderRows = 3

type DiffView struct {
	*tview.Flex

	header *tview.TextView
	body   *tview.TextView
	footer *tview.TextView

	filePath   string
	tabName    string
	oldContent string
	newContent string
	meta       DiffMeta

	changeRows []int
	totalRows  int
	shownAt    time.Time

	resultChan chan string
	resolved   bool
	mu         sync.Mutex
}

// diffInputGracePeriod is how long the diff ignores all key input after
// getting visible. The modal appears abruptly, so this avoids accepting
// or rejecting by accident while the user was typing somewhere else.
const diffInputGracePeriod = 500 * time.Millisecond

func NewDiffView(filePath, oldContent, newContent, tabName string, meta DiffMeta) *DiffView {
	d := &DiffView{
		Flex:       tview.NewFlex().SetDirection(tview.FlexRow),
		header:     tview.NewTextView().SetDynamicColors(true).SetWrap(false),
		body:       tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetScrollable(true),
		footer:     tview.NewTextView().SetDynamicColors(true).SetWrap(false),
		filePath:   filePath,
		tabName:    tabName,
		oldContent: oldContent,
		newContent: newContent,
		meta:       meta,
		shownAt:    time.Now(),
		resultChan: make(chan string, 1),
	}

	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)
	ops := computeDiffOps(oldLines, newLines)

	added, removed := countDiffChanges(ops)
	d.changeRows = changeStartRows(ops, diffBodyHeaderRows)
	d.totalRows = diffBodyHeaderRows + len(ops)

	d.header.SetText(d.renderHeader(added, removed))
	d.body.SetText(renderFallbackDiff(filePath, oldLines, newLines, ops))
	d.updateFooter()

	d.SetBorder(false)
	for _, view := range []*tview.TextView{d.header, d.body, d.footer} {
		view.SetBackgroundColor(theme.Current.ModalBg)
	}

	d.Flex.
		AddItem(d.header, 2, 0, false).
		AddItem(d.body, 0, 1, true).
		AddItem(d.footer, 1, 0, false)

	if len(d.changeRows) > 0 {
		d.scrollToChange(d.changeRows[0])
	} else {
		d.body.ScrollToBeginning()
	}

	d.body.SetInputCapture(d.captureInput)

	return d
}

func (d *DiffView) renderHeader(added, removed int) string {
	displayPath := d.filePath
	if d.meta.ProjectRoot != "" {
		if rel, err := filepath.Rel(d.meta.ProjectRoot, d.filePath); err == nil && !strings.HasPrefix(rel, "..") {
			displayPath = rel
		}
	}

	line1 := fmt.Sprintf(" [gray]file:[-] [white]%s[-]", tview.Escape(displayPath))

	parts := []string{}
	if d.meta.ProjectName != "" {
		parts = append(parts, fmt.Sprintf("[gray]project:[-] [white]%s[-]", tview.Escape(d.meta.ProjectName)))
	}
	if d.meta.Sessions > 1 && d.meta.SessionOrdinal > 0 {
		parts = append(parts, fmt.Sprintf("[gray]claude session:[-] [yellow]%d of %d[-]", d.meta.SessionOrdinal, d.meta.Sessions))
	}
	parts = append(parts, fmt.Sprintf("[green]+%d[-] [red]-%d[-]", added, removed))

	line2 := " " + strings.Join(parts, "  [#666666]|[-]  ")
	return line1 + "\n" + line2
}

func (d *DiffView) updateFooter() {
	position := ""
	if _, _, _, height := d.body.GetInnerRect(); height > 0 && d.totalRows > height {
		row, _ := d.body.GetScrollOffset()
		last := min(row+height, d.totalRows)
		position = fmt.Sprintf("[gray]lines %d-%d of %d[-]", row+1, last, d.totalRows)
	}

	keys := " [black:white] Enter [-:-] accept  [black:white] Esc [-:-] reject"
	if len(d.changeRows) > 1 {
		keys += "  [black:white] n/p [-:-] next/prev change"
	}

	d.footer.SetText(keys + "   " + position)
}

// Draw refreshes the scroll position indicator before rendering.
func (d *DiffView) Draw(screen tcell.Screen) {
	d.updateFooter()
	d.Flex.Draw(screen)
}

// MarkShown records when the diff got visible. Input is ignored during
// the grace period counted from this moment, not from creation, since a
// queued diff can be created long before it is presented.
func (d *DiffView) MarkShown() {
	d.shownAt = time.Now()
}

// InputAllowed reports if the grace period already passed and the diff
// accepts user input.
func (d *DiffView) InputAllowed() bool {
	return time.Since(d.shownAt) >= diffInputGracePeriod
}

func (d *DiffView) captureInput(event *tcell.EventKey) *tcell.EventKey {
	if !d.InputAllowed() {
		return nil
	}

	switch event.Key() {
	case tcell.KeyEnter, tcell.KeyCtrlS:
		d.Accept()
		return nil
	case tcell.KeyEscape:
		d.Reject()
		return event
	}

	switch event.Rune() {
	case 'n':
		d.jumpToChange(1)
		return nil
	case 'p':
		d.jumpToChange(-1)
		return nil
	}

	return event
}

func (d *DiffView) jumpToChange(direction int) {
	if len(d.changeRows) == 0 {
		return
	}

	current, _ := d.body.GetScrollOffset()
	currentTarget := current + diffScrollMargin

	if direction > 0 {
		for _, row := range d.changeRows {
			if row > currentTarget {
				d.scrollToChange(row)
				return
			}
		}
		d.scrollToChange(d.changeRows[0])
		return
	}

	for i := len(d.changeRows) - 1; i >= 0; i-- {
		if d.changeRows[i] < currentTarget {
			d.scrollToChange(d.changeRows[i])
			return
		}
	}
	d.scrollToChange(d.changeRows[len(d.changeRows)-1])
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

// diffScrollMargin is how many context rows are kept visible above the
// change we scroll to.
const diffScrollMargin = 2

func (d *DiffView) scrollToChange(row int) {
	target := row - diffScrollMargin
	if target < 0 {
		target = 0
	}
	d.body.ScrollTo(target, 0)
}

// countDiffChanges returns how many lines were added and removed.
func countDiffChanges(ops []diffOp) (added, removed int) {
	for _, op := range ops {
		switch op.kind {
		case diffAdded:
			added++
		case diffRemoved:
			removed++
		}
	}
	return added, removed
}

// changeStartRows returns the body rows where each change block starts.
// Consecutive changed lines count as one block.
func changeStartRows(ops []diffOp, rowOffset int) []int {
	rows := []int{}
	inBlock := false
	for i, op := range ops {
		if op.kind == diffContext {
			inBlock = false
			continue
		}
		if !inBlock {
			rows = append(rows, rowOffset+i)
			inBlock = true
		}
	}
	return rows
}

func splitDiffLines(content string) []string {
	if content == "" {
		return []string{}
	}
	return strings.Split(content, "\n")
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
