package ui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SearchResult represents a single search match in a file
type SearchResult struct {
	FilePath    string
	Line        int
	Column      int
	MatchText   string
	ContextBefore []string
	ContextAfter  []string
	MatchLength int
}

// ProjectSearchModal provides project-wide search and replace functionality
type ProjectSearchModal struct {
	app          *tview.Application
	form         *tview.Form
	resultsList  *tview.List
	previewText  *tview.TextView
	container    *tview.Flex
	
	
	searchTerm    string
	replaceTerm   string
	caseSensitive bool
	wholeWord     bool
	useRegex      bool
	
	
	results       []SearchResult
	selectedIdx   int
	searchTimer   *time.Timer
	searchMutex   sync.Mutex
	searching     bool
	searchCancel  chan bool
	
	
	onFileOpen    func(path string, line int, column int)
	getOpenFiles  func() map[string]*EditorTab
	onClose       func()
	
	
	searchField   *tview.InputField
	replaceField  *tview.InputField
	statusText    string
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
		m.selectedIdx = index
		m.updatePreview()
	})
	
	
	m.previewText = tview.NewTextView()
	m.previewText.SetBorder(true)
	m.previewText.SetTitle(" Preview ")
	m.previewText.SetDynamicColors(true)
	m.previewText.SetWordWrap(false)
	
	
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
	defer m.searchMutex.Unlock()
	
	
	if m.searchTimer != nil {
		m.searchTimer.Stop()
		m.searchTimer = nil
	}
	
	
	if m.searchTerm != "" {
		m.searchTimer = time.AfterFunc(500*time.Millisecond, func() {
			m.performSearch()
		})
	} else {
		
		m.results = []SearchResult{}
		m.app.QueueUpdateDraw(func() {
			m.updateResultsList()
			m.updatePreview()
		})
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
	m.searchCancel = make(chan bool, 1)
	cancelChan := m.searchCancel
	m.searchMutex.Unlock()
	
	
	m.app.QueueUpdateDraw(func() {
		m.resultsList.Clear()
		m.resultsList.AddItem("Searching...", "", 0, nil)
	})
	
	
	go func() {
		results := m.searchInProjectWithCancel(cancelChan)
		
		
		select {
		case <-cancelChan:
			return
		default:
		}
		
		m.searchMutex.Lock()
		m.searching = false
		m.results = results
		m.searchMutex.Unlock()
		
		m.app.QueueUpdateDraw(func() {
			
			select {
			case <-cancelChan:
				return
			default:
			}
			
			m.updateResultsList()
			if len(m.results) > 0 {
				m.selectedIdx = 0
				m.resultsList.SetCurrentItem(0)
				m.updatePreview()
			}
		})
	}()
}

func (m *ProjectSearchModal) cancelSearch() {
	m.searchMutex.Lock()
	defer m.searchMutex.Unlock()
	
	
	if m.searchTimer != nil {
		m.searchTimer.Stop()
		m.searchTimer = nil
	}
	
	
	if m.searchCancel != nil {
		select {
		case m.searchCancel <- true:
		default:
		}
		close(m.searchCancel)
		m.searchCancel = nil
	}
	
	m.searching = false
}

func (m *ProjectSearchModal) searchInProjectWithCancel(cancel <-chan bool) []SearchResult {
	results := []SearchResult{}
	searchRegex, err := m.buildSearchRegex()
	if err != nil {
		return results
	}
	
	
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		
		select {
		case <-cancel:
			return filepath.SkipDir
		default:
		}
		
		if err != nil {
			return nil 
		}
		
		
		if info.IsDir() || isExcludedPath(path) {
			if info.IsDir() && isExcludedPath(path) {
				return filepath.SkipDir
			}
			return nil
		}
		
		
		if isBinaryFile(path) {
			return nil
		}
		
		
		fileResults := m.searchInFile(path, searchRegex)
		results = append(results, fileResults...)
		
		return nil
	})
	
	
	select {
	case <-cancel:
		return []SearchResult{}
	default:
	}
	
	
	m.sortResultsByRelevance(results)
	
	return results
}

func (m *ProjectSearchModal) searchInFile(filePath string, regex *regexp.Regexp) []SearchResult {
	results := []SearchResult{}
	
	file, err := os.Open(filePath)
	if err != nil {
		return results
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	lineNum := 0
	lines := []string{}
	
	
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	
	
	for i, line := range lines {
		lineNum = i + 1
		matches := regex.FindAllStringIndex(line, -1)
		
		for _, match := range matches {
			
			contextBefore := []string{}
			contextAfter := []string{}
			
			for j := i - 2; j < i; j++ {
				if j >= 0 && j < len(lines) {
					contextBefore = append(contextBefore, lines[j])
				}
			}
			
			for j := i + 1; j <= i+2; j++ {
				if j < len(lines) {
					contextAfter = append(contextAfter, lines[j])
				}
			}
			
			result := SearchResult{
				FilePath:      filePath,
				Line:          lineNum,
				Column:        match[0],
				MatchText:     line,
				ContextBefore: contextBefore,
				ContextAfter:  contextAfter,
				MatchLength:   match[1] - match[0],
			}
			results = append(results, result)
		}
	}
	
	return results
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
	m.resultsList.Clear()
	
	if len(m.results) == 0 {
		if m.searchTerm != "" && !m.searching {
			m.resultsList.AddItem("No results found", "", 0, nil)
		}
		return
	}
	
	
	for i, result := range m.results {
		
		relPath, _ := filepath.Rel(".", result.FilePath)
		text := fmt.Sprintf("%s:%d", relPath, result.Line)
		
		
		if i == m.selectedIdx {
			text = "▶ " + text
		}
		
		m.resultsList.AddItem(text, "", 0, nil)
	}
	
	
	m.statusText = fmt.Sprintf("Found %d matches", len(m.results))
}

func (m *ProjectSearchModal) updatePreview() {
	m.previewText.Clear()
	
	if m.selectedIdx < 0 || m.selectedIdx >= len(m.results) {
		return
	}
	
	result := m.results[m.selectedIdx]
	
	
	fmt.Fprintf(m.previewText, "[gray]%s:%d[white]\n\n", result.FilePath, result.Line)
	
	
	for _, line := range result.ContextBefore {
		fmt.Fprintf(m.previewText, "  %s\n", line)
	}
	
	
	matchLine := result.MatchText
	if result.Column > 0 && result.Column+result.MatchLength <= len(matchLine) {
		before := matchLine[:result.Column]
		match := matchLine[result.Column:result.Column+result.MatchLength]
		after := matchLine[result.Column+result.MatchLength:]
		fmt.Fprintf(m.previewText, "▶ %s[yellow::b]%s[white::-]%s\n", before, match, after)
	} else {
		fmt.Fprintf(m.previewText, "▶ %s\n", matchLine)
	}
	
	
	for _, line := range result.ContextAfter {
		fmt.Fprintf(m.previewText, "  %s\n", line)
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