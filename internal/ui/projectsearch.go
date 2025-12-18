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

	"github.com/gdamore/tcell/v2"
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
	form        *tview.Form
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
	statusText   string
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

	m.form = tview.NewForm()
	m.form.SetBorderPadding(0, 0, 1, 1)
	m.form.SetFieldBackgroundColor(tcell.ColorBlack)
	m.form.SetItemPadding(0)

	m.searchField = tview.NewInputField()
	m.searchField.SetLabel("Search: ")
	m.searchField.SetFieldWidth(50)
	m.searchField.SetText(m.searchTerm)

	m.form.AddFormItem(m.searchField)

	m.replaceField = tview.NewInputField()
	m.replaceField.SetLabel("Replace: ")
	m.replaceField.SetFieldWidth(50)
	m.replaceField.SetText(m.replaceTerm)
	m.replaceField.SetChangedFunc(func(text string) {
		m.replaceTerm = text
	})
	m.form.AddFormItem(m.replaceField)

	m.form.AddCheckbox("Case sensitive", m.caseSensitive, func(checked bool) {
		m.caseSensitive = checked
		m.debouncedSearch()
	})

	m.form.AddCheckbox("Whole word", m.wholeWord, func(checked bool) {
		m.wholeWord = checked
		m.debouncedSearch()
	})

	m.form.AddCheckbox("Use regex", m.useRegex, func(checked bool) {
		m.useRegex = checked
		m.debouncedSearch()
	})

	m.form.AddButton("Replace All", func() {
		m.performReplaceAll()
	})

	m.form.AddButton("Cancel", func() {
		if m.onClose != nil {
			m.onClose()
		}
	})

	m.resultsList = tview.NewList()
	m.resultsList.SetBorder(true)
	m.resultsList.SetTitle(" Results ")
	m.resultsList.ShowSecondaryText(false)
	m.resultsList.SetHighlightFullLine(true)
	m.resultsList.SetSelectedFunc(func(index int, main string, secondary string, shortcut rune) {
		if index >= 0 && index < len(m.results) {
			result := m.results[index]
			if m.onFileOpen != nil {
				m.onFileOpen(result.FilePath, result.Line, result.Column)
				if m.onClose != nil {
					m.onClose()
				}
			}
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
	m.previewText.SetWordWrap(false)
	m.previewText.SetBorderColor(tcell.NewRGBColor(102, 102, 102))

	resultsPanel := tview.NewFlex().
		AddItem(m.resultsList, 0, 30, false).
		AddItem(m.previewText, 0, 70, false)

	m.container = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.form, 7, 0, true).
		AddItem(resultsPanel, 0, 1, false)

	m.setupKeyboardHandlers()
}

func (m *ProjectSearchModal) setupKeyboardHandlers() {

	m.form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case tcell.KeyTab:

			return event
		case tcell.KeyBacktab:

			return event
		case tcell.KeyDown:

			if len(m.results) > 0 {
				m.app.SetFocus(m.resultsList)
				if m.resultsList.GetItemCount() > 0 {
					m.resultsList.SetCurrentItem(0)
				}
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
				result := m.results[m.selectedIdx]
				if m.onFileOpen != nil {
					m.onFileOpen(result.FilePath, result.Line, result.Column)
					if m.onClose != nil {
						m.onClose()
					}
				}
			}
			return nil
		}
		return event
	})

	m.previewText.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {

			if m.selectedIdx >= 0 && m.selectedIdx < len(m.results) {
				result := m.results[m.selectedIdx]
				if m.onFileOpen != nil {
					m.onFileOpen(result.FilePath, result.Line, result.Column)
					if m.onClose != nil {
						m.onClose()
					}
				}
			}
		}
		return action, event
	})
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

	nextLines := make([]lineEntry, 0, 2)
	for len(nextLines) < 2 {
		next, hasNext := readNext()
		if !hasNext {
			break
		}
		nextLines = append(nextLines, next)
	}

	prevLines := make([]lineEntry, 0, 2)

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
		if len(prevLines) > 2 {
			prevLines = prevLines[1:]
		}

		current = nextLines[0]
		nextLines = nextLines[1:]
		for len(nextLines) < 2 {
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
		if m.searchTerm != "" {
			if m.searching {
				m.resultsList.AddItem("Searching...", "", 0, nil)
			} else {
				m.resultsList.AddItem("No results found", "", 0, nil)
			}
		}
		return
	}

	for _, result := range m.results {
		relPath, _ := filepath.Rel(".", result.FilePath)
		text := fmt.Sprintf("%s:%d", relPath, result.Line)
		m.resultsList.AddItem(text, "", 0, nil)
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

	if m.searching {
		m.statusText = fmt.Sprintf("Searching... %d matches", len(m.results))
	} else {
		m.statusText = fmt.Sprintf("Found %d matches", len(m.results))
	}
}

func (m *ProjectSearchModal) updatePreview() {
	m.previewText.Clear()

	if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
		m.previewText.SetTitle(" Preview ")
		return
	}

	result := m.results[m.selectedIdx]

	relPath, _ := filepath.Rel(".", result.FilePath)
	m.previewText.SetTitle(fmt.Sprintf(" %s ", relPath))

	lineNumWidth := 4
	startLine := result.Line - len(result.ContextBefore)

	for i, line := range result.ContextBefore {
		lineNum := startLine + i
		fmt.Fprintf(m.previewText, "[#666666]%*d│[-] %s\n", lineNumWidth, lineNum, tview.Escape(line))
	}

	matchLine := result.MatchText
	if result.Column >= 0 && result.Column+result.MatchLength <= len(matchLine) {
		before := tview.Escape(matchLine[:result.Column])
		match := tview.Escape(matchLine[result.Column : result.Column+result.MatchLength])
		after := tview.Escape(matchLine[result.Column+result.MatchLength:])
		fmt.Fprintf(m.previewText, "[white]%*d│[-] %s[black:yellow]%s[-:-]%s\n", lineNumWidth, result.Line, before, match, after)
	} else {
		fmt.Fprintf(m.previewText, "[white]%*d│[-] %s\n", lineNumWidth, result.Line, tview.Escape(matchLine))
	}

	for i, line := range result.ContextAfter {
		lineNum := result.Line + 1 + i
		fmt.Fprintf(m.previewText, "[#666666]%*d│[-] %s\n", lineNumWidth, lineNum, tview.Escape(line))
	}
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
