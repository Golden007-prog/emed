package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/laborin/emed/internal/claude"
	"github.com/laborin/emed/internal/logging"
	"github.com/rivo/tview"
)

type ClaudeIntegration struct {
	server       *claude.Server
	toolsHandler *claude.ToolsHandler

	editor     *Editor
	terminal   *Terminal
	layout     *Layout
	app        *tview.Application
	workingDir string

	diffTabs   map[string]*DiffView
	diffTabsMu sync.RWMutex

	latestSelection   *claude.Selection
	latestSelectionMu sync.RWMutex
}

func NewClaudeIntegration(layout *Layout, editor *Editor, terminal *Terminal, app *tview.Application) *ClaudeIntegration {
	workingDir, _ := os.Getwd()

	ci := &ClaudeIntegration{
		editor:     editor,
		terminal:   terminal,
		layout:     layout,
		app:        app,
		workingDir: workingDir,
		diffTabs:   make(map[string]*DiffView),
	}

	ci.toolsHandler = claude.NewToolsHandler()
	ci.toolsHandler.Editor = ci
	ci.toolsHandler.Diff = ci
	ci.toolsHandler.Terminal = ci
	ci.toolsHandler.Workspace = ci

	return ci
}

func (ci *ClaudeIntegration) Start() error {
	ci.server = claude.NewServer([]string{ci.workingDir}, ci.toolsHandler)

	ci.server.SetOnConnect(func() {
		ci.sendInitialState()
	})

	if err := ci.server.Start(); err != nil {
		return fmt.Errorf("failed to start Claude server: %w", err)
	}

	ci.terminal.SetClaudePort(ci.server.Port())

	logging.Error("Claude Code integration started on port %d", ci.server.Port())
	return nil
}

func (ci *ClaudeIntegration) sendInitialState() {
	ci.NotifyOpenEditorsChanged()

	if sel := ci.GetCurrentSelection(); sel != nil {
		ci.server.SendNotification("selection_changed", claude.SelectionChangedNotification{
			Selections: []claude.Selection{*sel},
		})
	}
}

func (ci *ClaudeIntegration) Stop() {
	if ci.server != nil {
		ci.server.Stop()
	}
}

func (ci *ClaudeIntegration) IsConnected() bool {
	if ci.server == nil {
		return false
	}
	return ci.server.IsConnected()
}

func (ci *ClaudeIntegration) OpenFile(path string, startLine, endLine int, selectPattern string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	ci.app.QueueUpdateDraw(func() {
		ci.layout.OpenFile(absPath)

		if startLine > 0 {
			ci.editor.GoToLine(startLine)
		}
	})

	return nil
}

func (ci *ClaudeIntegration) CloseTab(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	ci.app.QueueUpdateDraw(func() {
		ci.editor.CloseTabByPath(absPath)
	})

	return nil
}

func (ci *ClaudeIntegration) SaveDocument(path string, content string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return err
	}

	ci.app.QueueUpdateDraw(func() {
		ci.editor.ReloadFile(absPath)
	})

	return nil
}

func (ci *ClaudeIntegration) GetOpenEditors() []claude.OpenEditor {
	var editors []claude.OpenEditor

	tabs := ci.editor.GetAllTabs()
	activeTab := ci.editor.GetActiveTab()

	for _, tab := range tabs {
		editor := claude.OpenEditor{
			FilePath: tab.GetPath(),
			IsActive: activeTab != nil && tab.GetPath() == activeTab.GetPath(),
			IsDirty:  tab.IsDirty(),
			Language: getLanguageFromPath(tab.GetPath()),
		}
		editors = append(editors, editor)
	}

	return editors
}

func (ci *ClaudeIntegration) CheckDocumentDirty(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	tab := ci.editor.GetTabByPath(absPath)
	if tab == nil {
		return false
	}

	return tab.IsDirty()
}

func (ci *ClaudeIntegration) GetCurrentSelection() *claude.Selection {
	tab := ci.editor.GetActiveTab()
	if tab == nil || tab.GetCodeEditor() == nil {
		return nil
	}

	text, startRow, startCol, endRow, endCol, active := tab.GetCodeEditor().GetSelectionInfo()
	if !active {
		return nil
	}

	return &claude.Selection{
		FilePath: tab.GetPath(),
		Text:     text,
		Range: claude.Range{
			Start: claude.Position{Line: startRow, Character: startCol},
			End:   claude.Position{Line: endRow, Character: endCol},
		},
	}
}

func (ci *ClaudeIntegration) GetLatestSelection() *claude.Selection {
	ci.latestSelectionMu.RLock()
	defer ci.latestSelectionMu.RUnlock()

	if ci.latestSelection != nil {
		return ci.latestSelection
	}

	return ci.GetCurrentSelection()
}

func (ci *ClaudeIntegration) UpdateLatestSelection(sel *claude.Selection) {
	ci.latestSelectionMu.Lock()
	ci.latestSelection = sel
	ci.latestSelectionMu.Unlock()

	if ci.server != nil && sel != nil {
		ci.server.SendNotification("selection_changed", claude.SelectionChangedNotification{
			Selections: []claude.Selection{*sel},
		})
		ci.server.SendNotification("notifications/resources/list_changed", map[string]interface{}{})
	}
}

func (ci *ClaudeIntegration) NotifyOpenEditorsChanged() {
	if ci.server == nil {
		return
	}

	editors := ci.GetOpenEditors()
	ci.server.SendNotification("openEditors_changed", claude.OpenEditorsChangedNotification{
		OpenEditors: editors,
	})

	// Also notify that resources have changed (MCP standard notification)
	ci.server.SendNotification("notifications/resources/list_changed", map[string]interface{}{})
}

func (ci *ClaudeIntegration) NotifyFileSaved(path string) {
	if ci.server == nil {
		return
	}

	ci.server.SendNotification("fileSaved", claude.FileSavedNotification{
		FilePath: path,
	})
}

func (ci *ClaudeIntegration) NotifyFileCreated(path string) {
	if ci.server == nil {
		return
	}

	ci.server.SendNotification("fileCreated", claude.FileCreatedNotification{
		FilePath: path,
	})
}

func (ci *ClaudeIntegration) NotifyFileDeleted(path string) {
	if ci.server == nil {
		return
	}

	ci.server.SendNotification("fileDeleted", claude.FileDeletedNotification{
		FilePath: path,
	})
}

func (ci *ClaudeIntegration) NotifyFileRenamed(oldPath, newPath string) {
	if ci.server == nil {
		return
	}

	ci.server.SendNotification("fileRenamed", claude.FileRenamedNotification{
		OldPath: oldPath,
		NewPath: newPath,
	})
}

type diffResult struct {
	result  string
	content string
}

func (ci *ClaudeIntegration) OpenDiff(filePath, oldContent, newContent, tabName string) (string, string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	diffView := NewDiffView(absPath, oldContent, newContent, tabName)
	diffView.SetApp(ci.app)

	ci.diffTabsMu.Lock()
	ci.diffTabs[absPath] = diffView
	ci.diffTabsMu.Unlock()

	resultChan := make(chan diffResult, 1)

	ci.app.QueueUpdateDraw(func() {
		ci.layout.ShowDiffView(diffView)
	})

	go func() {
		result := diffView.WaitForResult()
		content := ""

		if result == claude.DiffResultSaved {
			content = diffView.GetNewContent()
		}

		ci.app.QueueUpdateDraw(func() {
			ci.layout.CloseDiffView(diffView)
		})

		ci.diffTabsMu.Lock()
		delete(ci.diffTabs, absPath)
		ci.diffTabsMu.Unlock()

		resultChan <- diffResult{result: result, content: content}
	}()

	res := <-resultChan
	return res.result, res.content, nil
}

func (ci *ClaudeIntegration) CloseAllDiffTabs() {
	ci.diffTabsMu.Lock()
	tabs := make([]*DiffView, 0, len(ci.diffTabs))
	for _, tab := range ci.diffTabs {
		tabs = append(tabs, tab)
	}
	ci.diffTabsMu.Unlock()

	for _, tab := range tabs {
		tab.Reject()
	}
}

func (ci *ClaudeIntegration) CloseDiffTabByName(tabName string) bool {
	ci.diffTabsMu.Lock()
	tabs := make([]*DiffView, 0, len(ci.diffTabs))
	for _, tab := range ci.diffTabs {
		tabs = append(tabs, tab)
	}
	ci.diffTabsMu.Unlock()

	for _, tab := range tabs {
		if tab.GetTabName() == tabName {
			tab.Reject()
			return true
		}
	}
	return false
}

func (ci *ClaudeIntegration) ExecuteCode(code string, language string) error {
	ci.app.QueueUpdateDraw(func() {
		ci.terminal.EnsureClaudeExecTerminal(ci.workingDir)
		ci.terminal.WriteToClaudeExec(code + "\n")
	})
	return nil
}

func (ci *ClaudeIntegration) GetWorkspaceFolders() []claude.WorkspaceFolder {
	return []claude.WorkspaceFolder{
		{
			URI:  "file://" + ci.workingDir,
			Name: filepath.Base(ci.workingDir),
		},
	}
}

func getLanguageFromPath(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".hpp", ".cc":
		return "cpp"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".sh", ".bash":
		return "shell"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".html":
		return "html"
	case ".css":
		return "css"
	default:
		return ""
	}
}
