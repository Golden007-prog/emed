package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/laborin/emed/internal/theme"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// SearchResult represents a single search match in a file
type SearchResult struct {
	FilePath      string
	Line          int
	Column        int
	MatchText     string
	ContextBefore []string
	ContextAfter  []string
	MatchLength   int
}

// ProjectSearchModal provides project-wide search and replace functionality
type ProjectSearchModal struct {
	app         *tview.Application
	form        *tview.Flex
	toggles     *searchToggles
	resultsList *tview.List
	previewText *tview.TextView
	container   *tview.Flex

	searchTerm    string
	replaceTerm   string
	caseSensitive bool
	wholeWord     bool
	useRegex      bool

	results          []SearchResult
	selectedIdx      int
	searchTimer      *time.Timer
	searchMutex      sync.Mutex
	searching        bool
	searchCtx        context.Context
	searchCancelFunc context.CancelFunc

	onFileOpen   func(path string, line int, column int)
	getOpenFiles func() map[string]*EditorTab
	onClose      func()

	searchField  *tview.InputField
	replaceField *tview.InputField
	updatingList bool
}

// NewProjectSearchModal creates a new project search modal
func NewProjectSearchModal(app *tview.Application) *ProjectSearchModal {
	modal := &ProjectSearchModal{
		app:         app,
		results:     make([]SearchResult, 0),
		selectedIdx: -1,
	}

	modal.setupUI()
	return modal
}

func (m *ProjectSearchModal) setupUI() {
	bg := theme.Current.ModalBg

	newInput := func(label string) *tview.InputField {
		input := tview.NewInputField().
			SetLabel(label).
			SetLabelColor(theme.Current.TextMuted).
			SetFieldBackgroundColor(tcell.NewRGBColor(45, 45, 45)).
			SetFieldTextColor(tcell.ColorWhite)
		return input
	}

	m.searchField = newInput("Search  ")
	m.searchField.SetText(m.searchTerm)

	m.replaceField = newInput("Replace ")
	m.replaceField.SetText(m.replaceTerm)
	m.replaceField.SetChangedFunc(func(text string) {
		m.replaceTerm = text
	})

	m.toggles = newSearchToggles(bg)
	m.toggles.SetStates(m.caseSensitive, m.wholeWord, m.useRegex)
	m.toggles.SetOnChange(func() {
		m.caseSensitive, m.wholeWord, m.useRegex = m.toggles.States()
		m.debouncedSearch()
	})

	hint := tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	hint.SetBackgroundColor(bg)
	hint.SetText("[#666666]Ctrl+Enter replace all[-]")

	searchRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(m.searchField, 0, 1, true).
		AddItem(tview.NewBox().SetBackgroundColor(bg), 1, 0, false).
		AddItem(m.toggles, searchTogglesWidth, 0, false)
	searchRow.SetBackgroundColor(bg)

	replaceRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(m.replaceField, 0, 1, false).
		AddItem(tview.NewBox().SetBackgroundColor(bg), 1, 0, false).
		AddItem(hint, 24, 0, false)
	replaceRow.SetBackgroundColor(bg)

	m.form = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(searchRow, 1, 0, true).
		AddItem(replaceRow, 1, 0, false)
	m.form.SetBackgroundColor(bg)
	m.form.SetBorderPadding(1, 0, 1, 1)

	m.resultsList = tview.NewList()
	m.resultsList.SetBorder(true)
	m.resultsList.SetTitle(" Results ")
	m.resultsList.ShowSecondaryText(false)
	m.resultsList.SetHighlightFullLine(true)
	m.resultsList.SetSelectedFunc(func(index int, main string, secondary string, shortcut rune) {
		if index >= 0 && index < len(m.results) {
			m.openResult(m.results[index])
		}
	})
	m.resultsList.SetChangedFunc(func(index int, main string, secondary string, shortcut rune) {
		if m.updatingList {
			return
		}
		m.selectedIdx = index
		m.updatePreview()
	})

	m.previewText = tview.NewTextView()
	m.previewText.SetBorder(true)
	m.previewText.SetTitle(" Preview ")
	m.previewText.SetDynamicColors(true)
	m.previewText.SetWrap(false)
	m.previewText.SetBorderColor(theme.Current.Border)

	resultsPanel := tview.NewFlex().
		AddItem(m.resultsList, 0, 40, false).
		AddItem(m.previewText, 0, 60, false)

	m.container = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.form, 3, 0, true).
		AddItem(resultsPanel, 0, 1, false)

	m.setupKeyboardHandlers()
}

func (m *ProjectSearchModal) setupKeyboardHandlers() {

	m.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Modifiers()&tcell.ModAlt != 0 && event.Key() == tcell.KeyRune {
			if m.toggles.HandleAltRune(event.Rune()) {
				return nil
			}
		}

		switch event.Key() {
		case tcell.KeyEscape:
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case tcell.KeyTab, tcell.KeyBacktab:
			if m.searchField.HasFocus() {
				m.app.SetFocus(m.replaceField)
			} else {
				m.app.SetFocus(m.searchField)
			}
			return nil
		case tcell.KeyUp:
			if m.replaceField.HasFocus() {
				m.app.SetFocus(m.searchField)
				return nil
			}
			return event
		case tcell.KeyDown:
			if m.searchField.HasFocus() && len(m.results) > 0 {
				m.app.SetFocus(m.resultsList)
				if m.resultsList.GetItemCount() > 0 {
					m.resultsList.SetCurrentItem(0)
				}
				return nil
			}
			if m.searchField.HasFocus() {
				m.app.SetFocus(m.replaceField)
				return nil
			}
			return event
		case tcell.KeyCtrlL:

			m.searchField.SetText("")
			m.replaceField.SetText("")
			m.searchTerm = ""
			m.replaceTerm = ""
			m.results = []SearchResult{}
			m.updateResultsList()
			m.updatePreview()
			return nil
		case tcell.KeyEnter:
			if event.Modifiers()&tcell.ModCtrl != 0 {

				m.performReplaceAll()
				return nil
			}
			return event
		}
		return event
	})

	m.resultsList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case tcell.KeyUp:
			if m.resultsList.GetCurrentItem() == 0 {

				m.app.SetFocus(m.form)
				return nil
			}
			return event
		case tcell.KeyEnter:
			if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
				m.openResult(m.results[m.selectedIdx])
			}
			return nil
		}
		return event
	})

	m.previewText.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
				m.openResult(m.results[m.selectedIdx])
			}
		}
		return action, event
	})
}

// openResult jumps to a search result in the editor. The stored column
// is a byte offset from the regex engine, the editor cursor works with
// runes, so it gets converted here.
func (m *ProjectSearchModal) openResult(result SearchResult) {
	if m.onFileOpen == nil {
		return
	}

	column := result.Column
	if column > len(result.MatchText) {
		column = len(result.MatchText)
	}
	runeColumn := utf8.RuneCountInString(result.MatchText[:column])

	m.onFileOpen(result.FilePath, result.Line, runeColumn)
	if m.onClose != nil {
		m.onClose()
	}
}

func (m *ProjectSearchModal) debouncedSearch() {
	m.searchMutex.Lock()
	if m.searchTimer != nil {
		m.searchTimer.Stop()
		m.searchTimer = nil
	}
	term := m.searchTerm
	if term != "" {
		m.searchTimer = time.AfterFunc(500*time.Millisecond, func() {
			m.performSearch()
		})
	}
	m.searchMutex.Unlock()

	if term == "" {
		m.cancelSearch()
		m.results = []SearchResult{}
		m.selectedIdx = -1
		m.updateResultsList()
		m.updatePreview()
	}
}

func (m *ProjectSearchModal) performSearch() {
	if m.searchTerm == "" {
		m.results = []SearchResult{}
		m.app.QueueUpdateDraw(func() {
			m.updateResultsList()
			m.updatePreview()
		})
		return
	}

	m.cancelSearch()

	m.searchMutex.Lock()
	m.searching = true
	ctx, cancel := context.WithCancel(context.Background())
	m.searchCtx = ctx
	m.searchCancelFunc = cancel
	m.searchMutex.Unlock()

	m.app.QueueUpdateDraw(func() {
		if m.searchCtx != ctx {
			return
		}
		m.results = nil
		m.selectedIdx = -1
		m.resultsList.Clear()
		m.previewText.Clear()
		m.resultsList.AddItem("Searching...", "", 0, nil)
	})

	go m.runProjectSearch(ctx)
}

func (m *ProjectSearchModal) runProjectSearch(ctx context.Context) {
	defer func() {
		m.searchMutex.Lock()
		if m.searchCtx == ctx {
			m.searchCtx = nil
			m.searchCancelFunc = nil
		}
		m.searching = false
		m.searchMutex.Unlock()
	}()

	regex, err := m.buildSearchRegex()
	if err != nil {
		m.publishResults(ctx, nil, true)
		return
	}

	fileCh := make(chan string, 128)
	resultCh := make(chan SearchResult, 256)
	workerCount := runtime.NumCPU()
	var workers sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for path := range fileCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				m.searchFile(ctx, path, regex, resultCh)
			}
		}()
	}

	var walkerErr error
	walkDone := make(chan struct{})
	go func() {
		defer close(walkDone)
		walkerErr = filepath.WalkDir(".", func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return context.Canceled
			default:
			}

			if d.IsDir() {
				if isExcludedPath(path) {
					return filepath.SkipDir
				}
				return nil
			}

			if isExcludedPath(path) || isBinaryFile(path) {
				return nil
			}

			select {
			case <-ctx.Done():
				return context.Canceled
			case fileCh <- path:
			}

			return nil
		})
		close(fileCh)
	}()

	go func() {
		workers.Wait()
		close(resultCh)
	}()

	results := make([]SearchResult, 0, 128)
	lastPublishCount := 0
	lastPublishTime := time.Now()
	ticker := time.NewTicker(75 * time.Millisecond)
	defer ticker.Stop()

	flush := func(final bool) {
		if ctx.Err() != nil {
			return
		}

		out := make([]SearchResult, len(results))
		copy(out, results)
		if final {
			m.sortResultsByRelevance(out)
		}

		m.publishResults(ctx, out, final)
		lastPublishCount = len(results)
		lastPublishTime = time.Now()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case res, ok := <-resultCh:
			if !ok {
				if errors.Is(walkerErr, context.Canceled) || ctx.Err() != nil {
					return
				}
				flush(true)
				return
			}
			results = append(results, res)
			if len(results)-lastPublishCount >= 25 || time.Since(lastPublishTime) > 200*time.Millisecond {
				flush(false)
			}
		case <-ticker.C:
			if len(results) != lastPublishCount {
				flush(false)
			}
		}
	}
}

func (m *ProjectSearchModal) publishResults(ctx context.Context, results []SearchResult, final bool) {
	if ctx.Err() != nil {
		return
	}

	m.app.QueueUpdateDraw(func() {
		if m.searchCtx != ctx {
			return
		}

		m.results = results
		if final {
			m.searchMutex.Lock()
			m.searching = false
			m.searchMutex.Unlock()
		}

		m.updateResultsList()
		if len(m.results) == 0 {
			if final && m.previewText != nil {
				m.previewText.Clear()
			}
			return
		}

		if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
			m.selectedIdx = 0
		}

		if m.resultsList != nil {
			m.resultsList.SetCurrentItem(m.selectedIdx)
		}

		m.updatePreview()
	})
}

func (m *ProjectSearchModal) searchFile(ctx context.Context, filePath string, regex *regexp.Regexp, resultCh chan<- SearchResult) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	type lineEntry struct {
		number int
		text   string
	}

	lineNumber := 0
	readNext := func() (lineEntry, bool) {
		if !scanner.Scan() {
			return lineEntry{}, false
		}
		lineNumber++
		return lineEntry{number: lineNumber, text: scanner.Text()}, true
	}

	current, ok := readNext()
	if !ok {
		return
	}

	nextLines := make([]lineEntry, 0, 8)
	for len(nextLines) < 8 {
		next, hasNext := readNext()
		if !hasNext {
			break
		}
		nextLines = append(nextLines, next)
	}

	prevLines := make([]lineEntry, 0, 8)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		matches := regex.FindAllStringIndex(current.text, -1)
		for _, match := range matches {
			contextBefore := make([]string, len(prevLines))
			for i := range prevLines {
				contextBefore[i] = prevLines[i].text
			}

			contextAfter := make([]string, len(nextLines))
			for i := range nextLines {
				contextAfter[i] = nextLines[i].text
			}

			result := SearchResult{
				FilePath:      filePath,
				Line:          current.number,
				Column:        match[0],
				MatchText:     current.text,
				ContextBefore: contextBefore,
				ContextAfter:  contextAfter,
				MatchLength:   match[1] - match[0],
			}

			select {
			case <-ctx.Done():
				return
			case resultCh <- result:
			}
		}

		if len(nextLines) == 0 {
			break
		}

		prevLines = append(prevLines, current)
		if len(prevLines) > 8 {
			prevLines = prevLines[1:]
		}

		current = nextLines[0]
		nextLines = nextLines[1:]
		for len(nextLines) < 8 {
			next, hasNext := readNext()
			if !hasNext {
				break
			}
			nextLines = append(nextLines, next)
		}
	}

	if err := scanner.Err(); err != nil {
		return
	}
}

func (m *ProjectSearchModal) cancelSearch() {
	m.searchMutex.Lock()
	defer m.searchMutex.Unlock()

	if m.searchTimer != nil {
		m.searchTimer.Stop()
		m.searchTimer = nil
	}

	if m.searchCancelFunc != nil {
		m.searchCancelFunc()
		m.searchCancelFunc = nil
	}
	m.searchCtx = nil
	m.searching = false
}

func (m *ProjectSearchModal) sortResultsByRelevance(results []SearchResult) {

	sort.Slice(results, func(i, j int) bool {

		iAtStart := results[i].Column == 0
		jAtStart := results[j].Column == 0

		if iAtStart != jAtStart {
			return iAtStart
		}

		if results[i].FilePath != results[j].FilePath {
			return results[i].FilePath < results[j].FilePath
		}

		return results[i].Line < results[j].Line
	})
}

func isBinaryFile(path string) bool {

	binaryExts := []string{
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".svg",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".zip", ".tar", ".gz", ".bz2", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".a", ".o",
		".pyc", ".pyo", ".class", ".jar",
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".mp3", ".mp4", ".avi", ".mov", ".wmv", ".flv",
		".db", ".sqlite", ".sqlite3",
	}

	ext := strings.ToLower(filepath.Ext(path))
	for _, binExt := range binaryExts {
		if ext == binExt {
			return true
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return true
	}

	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}

	return false
}

func (m *ProjectSearchModal) updateResultsList() {
	m.updatingList = true
	defer func() {
		m.updatingList = false
	}()

	currentIndex := m.selectedIdx
	m.resultsList.Clear()

	if len(m.results) == 0 {
		m.selectedIdx = -1
		m.resultsList.SetTitle(" Results ")
		if m.searchTerm != "" {
			if m.searching {
				m.resultsList.AddItem("Searching...", "", 0, nil)
			} else {
				m.resultsList.AddItem("No results found", "", 0, nil)
			}
		}
		return
	}

	_, _, listWidth, _ := m.resultsList.GetInnerRect()
	if listWidth <= 0 {
		listWidth = 60
	}
	pathBudget := listWidth / 3
	if pathBudget < 16 {
		pathBudget = 16
	}

	files := make(map[string]struct{}, len(m.results))
	for _, result := range m.results {
		files[result.FilePath] = struct{}{}
		relPath, _ := filepath.Rel(".", result.FilePath)
		location := m.truncatePathFromLeft(fmt.Sprintf("%s:%d", relPath, result.Line), pathBudget)
		snippetBudget := listWidth - runewidth.StringWidth(location) - 2
		item := "[#8a8a8a]" + tview.Escape(location) + "[-] " + m.buildSnippet(result, snippetBudget)
		m.resultsList.AddItem(item, "", 0, nil)
	}

	if m.searching {
		m.resultsList.SetTitle(fmt.Sprintf(" Results (%d so far…) ", len(m.results)))
	} else {
		m.resultsList.SetTitle(fmt.Sprintf(" Results (%d in %d files) ", len(m.results), len(files)))
	}

	itemCount := m.resultsList.GetItemCount()
	if currentIndex < 0 || currentIndex >= itemCount {
		if itemCount > 0 {
			currentIndex = 0
		} else {
			currentIndex = -1
		}
	}

	if currentIndex >= 0 && currentIndex < itemCount {
		m.resultsList.SetCurrentItem(currentIndex)
		m.selectedIdx = currentIndex
	} else {
		m.selectedIdx = -1
	}
}

// expandTabs replaces tabs with spaces so width math matches what the
// text views render (they expand tabs and would break truncation).
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}

// buildSnippet returns the matched line fragment for the results list,
// with leading indentation removed and the match highlighted.
func (m *ProjectSearchModal) buildSnippet(result SearchResult, maxWidth int) string {
	if maxWidth < 12 {
		maxWidth = 12
	}

	line := result.MatchText
	col := result.Column
	length := result.MatchLength
	if col < 0 || col+length > len(line) {
		return tview.Escape(m.truncateLine(expandTabs(strings.TrimLeft(line, " \t")), maxWidth))
	}

	before := expandTabs(strings.TrimLeft(line[:col], " \t"))
	match := expandTabs(line[col : col+length])
	after := expandTabs(line[col+length:])

	beforeMax := maxWidth / 3
	matchMax := maxWidth / 2

	if w := runewidth.StringWidth(before); w > beforeMax {
		before = runewidth.TruncateLeft(before, w-beforeMax+1, "…")
	}
	if runewidth.StringWidth(match) > matchMax {
		match = runewidth.Truncate(match, matchMax, "…")
	}
	afterMax := maxWidth - runewidth.StringWidth(before) - runewidth.StringWidth(match)
	if afterMax <= 1 {
		after = ""
	} else if runewidth.StringWidth(after) > afterMax {
		after = runewidth.Truncate(after, afterMax, "…")
	}

	return tview.Escape(before) + "[black:yellow]" + tview.Escape(match) + "[-:-]" + tview.Escape(after)
}

func (m *ProjectSearchModal) updatePreview() {
	m.previewText.Clear()

	if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
		m.previewText.SetTitle(" Preview ")
		return
	}

	result := m.results[m.selectedIdx]

	relPath, _ := filepath.Rel(".", result.FilePath)
	displayPath := m.truncatePathFromLeft(relPath, 50)
	m.previewText.SetTitle(fmt.Sprintf(" %s:%d ", displayPath, result.Line))

	_, _, previewWidth, previewHeight := m.previewText.GetInnerRect()
	if previewWidth <= 0 {
		previewWidth = 60
	}
	if previewHeight <= 0 {
		previewHeight = 10
	}

	contextBefore, contextAfter := m.loadExtendedContext(result, previewHeight)

	lineNumWidth := 4
	maxLineWidth := previewWidth - lineNumWidth - 3
	if maxLineWidth < 20 {
		maxLineWidth = 20
	}

	startLine := result.Line - len(contextBefore)

	for i, line := range contextBefore {
		lineNum := startLine + i
		truncated := m.truncateLine(expandTabs(line), maxLineWidth)
		fmt.Fprintf(m.previewText, "[#666666]%*d│[-] %s\n", lineNumWidth, lineNum, tview.Escape(truncated))
	}

	matchLine := result.MatchText
	if result.Column >= 0 && result.Column+result.MatchLength <= len(matchLine) {
		before := expandTabs(matchLine[:result.Column])
		match := expandTabs(matchLine[result.Column : result.Column+result.MatchLength])
		after := expandTabs(matchLine[result.Column+result.MatchLength:])

		visibleBefore := before
		visibleMatch := match
		visibleAfter := after
		matchWidth := runewidth.StringWidth(match)
		totalWidth := runewidth.StringWidth(before) + matchWidth + runewidth.StringWidth(after)

		if totalWidth > maxLineWidth {
			if matchWidth > maxLineWidth {
				visibleMatch = runewidth.Truncate(match, maxLineWidth, "…")
			}
			availableForContext := maxLineWidth - matchWidth
			if availableForContext < 0 {
				availableForContext = 0
			}
			beforeAlloc := availableForContext / 2
			afterAlloc := availableForContext - beforeAlloc

			if beforeWidth := runewidth.StringWidth(before); beforeWidth > beforeAlloc {
				visibleBefore = runewidth.TruncateLeft(before, beforeWidth-beforeAlloc+1, "…")
			}
			if runewidth.StringWidth(after) > afterAlloc {
				if afterAlloc <= 1 {
					visibleAfter = ""
				} else {
					visibleAfter = runewidth.Truncate(after, afterAlloc, "…")
				}
			}
		}

		fmt.Fprintf(m.previewText, "[white]%*d│[-] %s[black:yellow]%s[-:-]%s\n",
			lineNumWidth, result.Line,
			tview.Escape(visibleBefore), tview.Escape(visibleMatch), tview.Escape(visibleAfter))
	} else {
		truncated := m.truncateLine(expandTabs(matchLine), maxLineWidth)
		fmt.Fprintf(m.previewText, "[white]%*d│[-] %s\n", lineNumWidth, result.Line, tview.Escape(truncated))
	}

	for i, line := range contextAfter {
		lineNum := result.Line + 1 + i
		truncated := m.truncateLine(expandTabs(line), maxLineWidth)
		fmt.Fprintf(m.previewText, "[#666666]%*d│[-] %s\n", lineNumWidth, lineNum, tview.Escape(truncated))
	}
}

func (m *ProjectSearchModal) loadExtendedContext(result SearchResult, availableHeight int) (before, after []string) {
	neededLines := availableHeight - 1
	if neededLines < 3 {
		neededLines = 3
	}

	beforeCount := neededLines / 2
	afterCount := neededLines - beforeCount

	if len(result.ContextBefore) >= beforeCount && len(result.ContextAfter) >= afterCount {
		return result.ContextBefore[len(result.ContextBefore)-beforeCount:], result.ContextAfter[:afterCount]
	}

	file, err := os.Open(result.FilePath)
	if err != nil {
		return result.ContextBefore, result.ContextAfter
	}
	defer file.Close()

	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	matchIdx := result.Line - 1
	if matchIdx < 0 || matchIdx >= len(allLines) {
		return result.ContextBefore, result.ContextAfter
	}

	startIdx := matchIdx - beforeCount
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := matchIdx + afterCount + 1
	if endIdx > len(allLines) {
		endIdx = len(allLines)
	}

	before = allLines[startIdx:matchIdx]
	if matchIdx+1 < len(allLines) {
		after = allLines[matchIdx+1 : endIdx]
	}

	return before, after
}

func (m *ProjectSearchModal) truncateLine(line string, maxWidth int) string {
	if runewidth.StringWidth(line) <= maxWidth {
		return line
	}
	if maxWidth <= 1 {
		return "…"
	}
	return runewidth.Truncate(line, maxWidth, "…")
}

func (m *ProjectSearchModal) truncatePathFromLeft(path string, maxWidth int) string {
	width := runewidth.StringWidth(path)
	if width <= maxWidth {
		return path
	}
	if maxWidth <= 4 {
		return "…"
	}
	return runewidth.TruncateLeft(path, width-maxWidth+1, "…")
}

func (m *ProjectSearchModal) performReplaceAll() {
	if m.replaceTerm == "" || len(m.results) == 0 {
		return
	}

	var replaceRegex *regexp.Regexp
	if m.useRegex {
		regex, err := m.buildSearchRegex()
		if err != nil {
			return
		}
		replaceRegex = regex
	}

	fileResults := make(map[string][]SearchResult)
	for _, result := range m.results {
		fileResults[result.FilePath] = append(fileResults[result.FilePath], result)
	}

	openFiles := make(map[string]*EditorTab)
	if m.getOpenFiles != nil {
		openFiles = m.getOpenFiles()
	}

	bufferReplacements := 0
	filesystemReplacements := 0
	errorCount := 0

	for filePath, results := range fileResults {
		if tab, isOpen := openFiles[filePath]; isOpen && tab.IsModified() {

			count := m.replaceInBuffer(tab, results, replaceRegex)
			bufferReplacements += count
		} else {

			count, err := m.replaceInFile(filePath, results, replaceRegex)
			if err != nil {
				errorCount++
			} else {
				filesystemReplacements += count

				if tab, isOpen := openFiles[filePath]; isOpen && !tab.IsModified() {

				}
			}
		}
	}

	m.showReplaceSummary(bufferReplacements, filesystemReplacements, errorCount)
}

func (m *ProjectSearchModal) replaceInBuffer(tab *EditorTab, results []SearchResult, regex *regexp.Regexp) int {
	if tab.codeEditor == nil || tab.codeEditor.buffer == nil {
		return 0
	}

	buffer := tab.codeEditor.buffer
	replacements := 0

	sort.Slice(results, func(i, j int) bool {
		if results[i].Line != results[j].Line {
			return results[i].Line > results[j].Line
		}
		return results[i].Column > results[j].Column
	})

	for _, result := range results {
		lineIdx := result.Line - 1
		if lineIdx >= 0 && lineIdx < buffer.GetLineCount() {
			line := string(buffer.GetLine(lineIdx))

			var newLine string
			if regex != nil {

				newLine = regex.ReplaceAllString(line, m.replaceTerm)
			} else {

				if result.Column+result.MatchLength <= len(line) {
					before := line[:result.Column]
					after := line[result.Column+result.MatchLength:]
					newLine = before + m.replaceTerm + after
				} else {
					continue
				}
			}

			buffer.SetLine(lineIdx, []rune(newLine))
			replacements++
		}
	}

	if replacements > 0 {
		buffer.SetModified(true)
		tab.SetModified(true)
	}

	return replacements
}

func (m *ProjectSearchModal) replaceInFile(filePath string, results []SearchResult, regex *regexp.Regexp) (int, error) {

	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(content), "\n")
	replacements := 0

	sort.Slice(results, func(i, j int) bool {
		if results[i].Line != results[j].Line {
			return results[i].Line > results[j].Line
		}
		return results[i].Column > results[j].Column
	})

	for _, result := range results {
		lineIdx := result.Line - 1
		if lineIdx >= 0 && lineIdx < len(lines) {
			line := lines[lineIdx]

			var newLine string
			if regex != nil {

				newLine = regex.ReplaceAllString(line, m.replaceTerm)
			} else {

				if result.Column+result.MatchLength <= len(line) {
					before := line[:result.Column]
					after := line[result.Column+result.MatchLength:]
					newLine = before + m.replaceTerm + after
				} else {
					continue
				}
			}

			lines[lineIdx] = newLine
			replacements++
		}
	}

	if replacements > 0 {

		newContent := strings.Join(lines, "\n")
		err = os.WriteFile(filePath, []byte(newContent), 0644)
		if err != nil {
			return 0, err
		}
	}

	return replacements, nil
}

func (m *ProjectSearchModal) showReplaceSummary(bufferCount, filesystemCount, errorCount int) {
	modal := tview.NewModal()

	text := fmt.Sprintf("Replace All completed:\n\n• %d replacements in open buffers\n• %d replacements in filesystem",
		bufferCount, filesystemCount)

	if errorCount > 0 {
		text += fmt.Sprintf("\n• %d files could not be updated", errorCount)
	}

	modal.SetText(text)
	modal.AddButtons([]string{"OK"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		m.app.SetRoot(m.container, true)
		if m.onClose != nil {
			m.onClose()
		}
	})

	m.app.SetRoot(modal, true)
}

// SetCallbacks sets the callback functions
func (m *ProjectSearchModal) SetCallbacks(onFileOpen func(string, int, int), getOpenFiles func() map[string]*EditorTab, onClose func()) {
	m.onFileOpen = onFileOpen
	m.getOpenFiles = getOpenFiles
	m.onClose = onClose
}

// GetContainer returns the container for the modal
func (m *ProjectSearchModal) GetContainer() *tview.Flex {
	return m.container
}

// RestoreState restores the persistent state
func (m *ProjectSearchModal) RestoreState(searchTerm, replaceTerm string, caseSensitive, wholeWord, useRegex bool) {

	m.searchTerm = searchTerm
	m.replaceTerm = replaceTerm
	m.caseSensitive = caseSensitive
	m.wholeWord = wholeWord
	m.useRegex = useRegex

	if m.toggles != nil {
		m.toggles.SetStates(caseSensitive, wholeWord, useRegex)
	}

	m.results = []SearchResult{}
	m.selectedIdx = -1

	if m.resultsList != nil {
		m.resultsList.Clear()
	}
	if m.previewText != nil {
		m.previewText.Clear()
	}

	if m.searchField != nil {
		m.searchField.SetText(searchTerm)
	}
	if m.replaceField != nil {
		m.replaceField.SetText(replaceTerm)
	}
}

// GetState returns the current persistent state
func (m *ProjectSearchModal) GetState() (searchTerm, replaceTerm string, caseSensitive, wholeWord, useRegex bool) {
	return m.searchTerm, m.replaceTerm, m.caseSensitive, m.wholeWord, m.useRegex
}

// Focus sets focus to the search field
func (m *ProjectSearchModal) Focus() {
	m.app.SetFocus(m.searchField)
}

// ActivateHandlers sets up the change handlers after state is restored
func (m *ProjectSearchModal) ActivateHandlers() {
	if m.searchField != nil {
		m.searchField.SetChangedFunc(func(text string) {
			m.searchTerm = text
			m.debouncedSearch()
		})
	}
}

// Stop cancels any ongoing operations
func (m *ProjectSearchModal) Stop() {
	m.cancelSearch()

	if m.resultsList != nil {
		m.resultsList.Clear()
	}
	if m.previewText != nil {
		m.previewText.Clear()
	}

	m.selectedIdx = -1
}

func isExcludedPath(path string) bool {
	excludedDirs := []string{
		"node_modules",
		"vendor",
		".git",
		"dist",
		"build",
		"target",
		".idea",
		".vscode",
	}

	excludedExts := []string{
		".min.js",
		".min.css",
		".map",
		".lock",
		"go.sum",
		".pyc",
		".pyo",
		".class",
		".o",
		".a",
		".so",
		".dylib",
		".dll",
		".exe",
	}

	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		for _, excluded := range excludedDirs {
			if part == excluded {
				return true
			}
		}
	}

	for _, ext := range excludedExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

func (m *ProjectSearchModal) buildSearchRegex() (*regexp.Regexp, error) {
	pattern := m.searchTerm

	if !m.useRegex {

		pattern = regexp.QuoteMeta(pattern)
	}

	if m.wholeWord {
		pattern = `\b` + pattern + `\b`
	}

	flags := ""
	if !m.caseSensitive {
		flags = "(?i)"
	}

	return regexp.Compile(flags + pattern)
}
