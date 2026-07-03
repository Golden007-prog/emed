package claude

import (
	"encoding/json"
	"testing"
)

type testEditorProvider struct {
	lastClosedPath string
}

func (p *testEditorProvider) OpenFile(path string, startLine, endLine int, selectPattern string) error {
	return nil
}

func (p *testEditorProvider) CloseTab(path string) error {
	p.lastClosedPath = path
	return nil
}

func (p *testEditorProvider) SaveDocument(path string, content string) error {
	return nil
}

func (p *testEditorProvider) GetOpenEditors() []OpenEditor {
	return nil
}

func (p *testEditorProvider) CheckDocumentDirty(path string) bool {
	return false
}

func (p *testEditorProvider) GetCurrentSelection() *Selection {
	return nil
}

func (p *testEditorProvider) GetLatestSelection() *Selection {
	return nil
}

type testDiffProvider struct {
	lastClosedName string
	closedAll      bool
}

func (p *testDiffProvider) OpenDiff(filePath, oldContent, newContent, tabName string, sessionID int) (string, string, error) {
	return DiffResultRejected, "", nil
}

func (p *testDiffProvider) CloseAllDiffTabs() {
	p.closedAll = true
}

func (p *testDiffProvider) CloseDiffTabByName(tabName string) bool {
	p.lastClosedName = tabName
	return true
}

func TestHandleResourcesTemplatesList(t *testing.T) {
	h := NewToolsHandler()

	result, err := h.HandleTool(1, "resources/templates/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, ok := decoded["resourceTemplates"]; !ok {
		t.Fatalf("expected resourceTemplates in response, got %s", string(data))
	}
}

func TestHandleCloseTabWithTabName(t *testing.T) {
	diff := &testDiffProvider{}
	h := NewToolsHandler()
	h.Diff = diff

	params := json.RawMessage(`{"name":"close_tab","arguments":{"tab_name":"tab-1"}}`)
	result, err := h.HandleTool(1, "tools/call", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff.lastClosedName != "tab-1" {
		t.Fatalf("expected tab_name to be forwarded, got %q", diff.lastClosedName)
	}

	resp, ok := result.(MCPResponse)
	if !ok || len(resp.Content) == 0 || resp.Content[0].Text != "Tab closed" {
		t.Fatalf("unexpected response: %#v", result)
	}
}
