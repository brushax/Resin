package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Resinat/Resin/internal/requestlog"
	"github.com/Resinat/Resin/internal/service"
)

// Server handles MCP Streamable HTTP JSON-RPC 2.0 requests.
type Server struct {
	cp             *service.ControlPlaneService
	requestlogRepo *requestlog.Repo
	tools          map[string]toolHandler
}

type toolHandler func(ctx context.Context, args json.RawMessage) (any, error)

func NewServer(cp *service.ControlPlaneService, repo *requestlog.Repo) *Server {
	s := &Server{
		cp:             cp,
		requestlogRepo: repo,
		tools:          make(map[string]toolHandler),
	}
	s.registerTools()
	return s
}

// ServeHTTP handles incoming POST requests for Streamable HTTP MCP.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed (MCP requires POST)", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONRPCError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	ctx := r.Context()
	switch req.Method {
	case "initialize":
		s.handleInitialize(w, req.ID)
	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
	case "tools/list":
		s.handleListTools(w, req.ID)
	case "tools/call":
		s.handleCallTool(ctx, w, req.ID, req.Params)
	default:
		writeJSONRPCError(w, req.ID, -32601, "Method not found", fmt.Sprintf("Unknown method: %s", req.Method))
	}
}

func (s *Server) handleInitialize(w http.ResponseWriter, id any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: ServerCaps{
				Tools: &ToolsCap{ListChanged: false},
			},
			ServerInfo: ServerInfo{
				Name:    "resin-copilot",
				Version: "1.0.0",
			},
			Instructions: "Resin Intelligent Network Proxy Gateway MCP Server. Allows AI Agents to dynamically observe network metrics, query request logs, probe target domains, and adjust platform policies for self-healing operations.",
		},
	}
	writeJSON(w, resp)
}

func (s *Server) handleListTools(w http.ResponseWriter, id any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: ListToolsResult{
			Tools: s.ToolDefinitions(),
		},
	}
	writeJSON(w, resp)
}

func (s *Server) handleCallTool(ctx context.Context, w http.ResponseWriter, id any, rawParams json.RawMessage) {
	var params CallToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		writeJSONRPCError(w, id, -32602, "Invalid params", err.Error())
		return
	}

	handler, ok := s.tools[params.Name]
	if !ok {
		writeJSONRPCError(w, id, -32601, "Tool not found", fmt.Sprintf("Tool %s does not exist", params.Name))
		return
	}

	res, err := handler(ctx, params.Arguments)
	if err != nil {
		callRes := CallToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("ERROR: %v", err)}},
			IsError: true,
		}
		writeJSON(w, JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: callRes})
		return
	}

	textRes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		textRes = []byte(fmt.Sprintf("%v", res))
	}

	callRes := CallToolResult{
		Content: []ContentItem{{Type: "text", Text: string(textRes)}},
		IsError: false,
	}
	writeJSON(w, JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: callRes})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}
