// Package claude implements the Claude Code IDE integration protocol.
package claude

import "encoding/json"

// JSON-RPC 2.0 message types

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP response format

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPResponse struct {
	Content []MCPContent `json:"content"`
}

func NewMCPTextResponse(text string) MCPResponse {
	return MCPResponse{
		Content: []MCPContent{{Type: "text", Text: text}},
	}
}

func NewMCPDiffResponse(result, content string) MCPResponse {
	return MCPResponse{
		Content: []MCPContent{
			{Type: "text", Text: result},
			{Type: "text", Text: content},
		},
	}
}

// Lock file structure

type LockFile struct {
	PID              int      `json:"pid"`
	WorkspaceFolders []string `json:"workspaceFolders"`
	IDEName          string   `json:"ideName"`
	Transport        string   `json:"transport"`
	Token            string   `json:"authToken"`
}

// Tool parameter types

type OpenFileParams struct {
	FilePath      string `json:"filePath"`
	Preview       bool   `json:"preview,omitempty"`
	SelectPattern string `json:"selectPattern,omitempty"`
	StartLine     int    `json:"startLine,omitempty"`
	EndLine       int    `json:"endLine,omitempty"`
}

type OpenDiffParams struct {
	OldFilePath    string `json:"old_file_path"`
	NewFilePath    string `json:"new_file_path"`
	NewFileContent string `json:"new_file_contents"`
	TabName        string `json:"tab_name,omitempty"`
}

type CloseTabParams struct {
	FilePath string `json:"filePath"`
}

type SaveDocumentParams struct {
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
}

type ExecuteCodeParams struct {
	Code     string `json:"code"`
	Language string `json:"language,omitempty"`
}

type GetDiagnosticsParams struct {
	FilePath string `json:"filePath,omitempty"`
}

type CheckDocumentDirtyParams struct {
	FilePath string `json:"filePath"`
}

// Tool result types

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Selection struct {
	FilePath string `json:"filePath"`
	Text     string `json:"text"`
	Range    Range  `json:"range"`
}

type OpenEditor struct {
	FilePath string `json:"filePath"`
	IsActive bool   `json:"isActive"`
	IsDirty  bool   `json:"isDirty"`
	Language string `json:"language,omitempty"`
}

type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type Diagnostic struct {
	FilePath string `json:"filePath"`
	Range    Range  `json:"range"`
	Message  string `json:"message"`
	Severity int    `json:"severity"`
	Source   string `json:"source,omitempty"`
}

// Notification types (IDE -> Claude)

type SelectionChangedNotification struct {
	Selections []Selection `json:"selections"`
}

type OpenEditorsChangedNotification struct {
	OpenEditors []OpenEditor `json:"openEditors"`
}

type FileSavedNotification struct {
	FilePath string `json:"filePath"`
}

type FileCreatedNotification struct {
	FilePath string `json:"filePath"`
}

type FileDeletedNotification struct {
	FilePath string `json:"filePath"`
}

type FileRenamedNotification struct {
	OldPath string `json:"oldPath"`
	NewPath string `json:"newPath"`
}

type AtMentionedNotification struct {
	FilePath  string `json:"filePath"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
}

// Tool names as constants
const (
	ToolOpenFile         = "openFile"
	ToolOpenDiff         = "openDiff"
	ToolCloseTab         = "close_tab"
	ToolCloseAllDiffTabs = "closeAllDiffTabs"
	ToolGetCurrentSelect = "getCurrentSelection"
	ToolGetLatestSelect  = "getLatestSelection"
	ToolGetOpenEditors   = "getOpenEditors"
	ToolGetWorkspace     = "getWorkspaceFolders"
	ToolGetDiagnostics   = "getDiagnostics"
	ToolCheckDocDirty    = "checkDocumentDirty"
	ToolSaveDocument     = "saveDocument"
	ToolExecuteCode      = "executeCode"
)

// Diff result constants
const (
	DiffResultSaved    = "FILE_SAVED"
	DiffResultRejected = "DIFF_REJECTED"
)
