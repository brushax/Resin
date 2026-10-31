package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Resinat/Resin/internal/mcp"
	"github.com/Resinat/Resin/internal/service"
)

func TestAPIServer_MCPRoute_AuthenticationAndTools(t *testing.T) {
	adminToken := "test-secret-token"
	srv := NewServerWithAddress("127.0.0.1", 2260, adminToken, service.SystemInfo{}, nil, nil, nil, 1048576, nil, nil)

	initPayload, _ := json.Marshal(mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})

	// 1. Unauthenticated request should receive 401
	unauthReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(initPayload))
	unauthRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", unauthRec.Code)
	}

	// 2. Authenticated request with Bearer token should succeed with 200
	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(initPayload))
	authReq.Header.Set("Authorization", "Bearer "+adminToken)
	authRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", authRec.Code, authRec.Body.String())
	}

	var rpcResp mcp.JSONRPCResponse
	if err := json.Unmarshal(authRec.Body.Bytes(), &rpcResp); err != nil {
		t.Fatal(err)
	}
	if rpcResp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", rpcResp.Error)
	}
}
