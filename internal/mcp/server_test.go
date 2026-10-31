package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPServer_InitializeAndListTools(t *testing.T) {
	srv := NewServer(nil, nil)

	// 1. Test initialize
	initReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	body, _ := json.Marshal(initReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("initialize expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var initResp JSONRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &initResp); err != nil {
		t.Fatal(err)
	}
	if initResp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", initResp.Error)
	}

	// 2. Test tools/list
	listReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	body, _ = json.Marshal(listReq)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list expected status 200, got %d", rec.Code)
	}
	var listResp struct {
		Result struct {
			Tools []Tool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Result.Tools) < 8 {
		t.Fatalf("expected at least 8 tools, got %d", len(listResp.Result.Tools))
	}

	// Verify key tools exist
	toolMap := make(map[string]bool)
	for _, tool := range listResp.Result.Tools {
		toolMap[tool.Name] = true
	}
	for _, expected := range []string{
		"resin_search_logs",
		"resin_summarize_logs",
		"resin_list_nodes",
		"resin_get_node",
		"resin_diagnose_target",
		"resin_isolate_node",
		"resin_recover_node",
	} {
		if !toolMap[expected] {
			t.Errorf("missing expected tool: %s", expected)
		}
	}
}

func TestMCPServer_UnknownMethod(t *testing.T) {
	srv := NewServer(nil, nil)
	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      99,
		Method:  "non_existent_method",
	}
	body, _ := json.Marshal(rpcReq)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("expected -32601 method not found error, got %v", resp.Error)
	}
}
