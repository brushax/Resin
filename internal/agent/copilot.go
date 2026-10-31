package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Resinat/Resin/internal/mcp"
)

const defaultSystemPrompt = `You are the Resin Autonomous Network Copilot, an expert site-reliability and proxy network engineer.
Your mission is to autonomously investigate network anomalies, diagnose root causes, conduct live hypothesis tests, and apply or recommend closed-loop hot remediations.

INVESTIGATION METHODOLOGY:
1. OVERVIEW: Start by calling 'resin_summarize_logs' to understand global failure clusters, error codes, and slow requests.
2. DRILL DOWN: Call 'resin_search_logs' with targeted filters (host, status, duration) to inspect failure stages and error messages.
3. INSPECT NODES: For suspicious nodes, call 'resin_get_node' to check active connections, throughput, failure count, and latency history.
4. LIVE HYPOTHESIS TESTING: Call 'resin_diagnose_target' to probe failing domains right now through specific nodes or platforms to verify whether the node is truly dead, poisoned, or blocking traffic.
5. CLOSED-LOOP REMEDIATION: If a node is confirmed failing/poisoned or rules are misconfigured (and mutations are allowed):
   - Apply hot fixes via 'resin_isolate_node', 'resin_add_platform_exclusion', or 'resin_patch_platform'.
   - IMPORTANT: After applying any remediation, you MUST call 'resin_verify_remediation' to test the failing domain again and confirm that the fix succeeded!
6. CONCLUDE: When you have sufficient evidence, stop calling tools and provide your final comprehensive diagnosis with:
   - Root Cause Analysis (with concrete evidence)
   - Scope of Impact (affected platforms/targets)
   - Remediations Applied & Verification Status
   - Advice for the operator.
`

type Copilot struct {
	mcpServer  *mcp.Server
	httpClient *http.Client
	auditStore *AuditStore
}

func NewCopilot(mcpServer *mcp.Server, store *AuditStore) *Copilot {
	return &Copilot{
		mcpServer:  mcpServer,
		auditStore: store,
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Copilot) AuditStore() *AuditStore {
	return c.auditStore
}

// Investigate executes an autonomous ReAct loop using the provided OpenAI-compatible endpoint.
func (c *Copilot) Investigate(ctx context.Context, req InvestigationRequest) (*InvestigationReport, error) {
	if strings.TrimSpace(req.Goal) == "" {
		return nil, fmt.Errorf("investigation goal is required")
	}

	startTime := time.Now()
	runID := randomHexID(12)

	baseURL := req.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("RESIN_AGENT_BASE_URL")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required (or set RESIN_AGENT_BASE_URL)")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	model := req.Model
	if model == "" {
		model = os.Getenv("RESIN_AGENT_MODEL")
		if model == "" {
			model = "gpt-4o-mini"
		}
	}

	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("RESIN_AGENT_API_KEY")
	}

	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 6
	}
	if maxSteps > 12 {
		maxSteps = 12
	}

	report := &InvestigationReport{
		Goal:  req.Goal,
		Trace: make([]StepRecord, 0),
	}

	defer func() {
		if report != nil && c.auditStore != nil {
			status := "diagnosed"
			if len(report.ActionsTaken) > 0 {
				status = "remediated"
			}
			traceBytes, _ := json.Marshal(report.Trace)
			_ = c.auditStore.RecordRun(RunDetail{
				RunSummary: RunSummary{
					ID:                runID,
					CreatedAt:         startTime,
					DurationMs:        time.Since(startTime).Milliseconds(),
					TriggerType:       "api",
					Goal:              req.Goal,
					Model:             model,
					Status:            status,
					RemediationsCount: len(report.ActionsTaken),
					TotalSteps:        report.TotalSteps,
					Summary:           report.Summary,
				},
				TraceJSON:        string(traceBytes),
				RemediationsList: report.ActionsTaken,
			})
			log.Printf("[agent] run_id=%s steps=%d remediations=%d status=%s duration=%dms",
				runID, report.TotalSteps, len(report.ActionsTaken), status, time.Since(startTime).Milliseconds())
		}
	}()

	// Build MCP tools schema in OpenAI Chat tool format
	tools := c.buildOpenAITools(req.AllowMutation)

	messages := []chatMessage{
		{Role: "system", Content: defaultSystemPrompt},
		{Role: "user", Content: fmt.Sprintf("INVESTIGATION GOAL: %s\n(Allow Safe Mutations: %v)", req.Goal, req.AllowMutation)},
	}

	for step := 1; step <= maxSteps; step++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		chatReq := chatCompletionRequest{
			Model:       model,
			Messages:    messages,
			Tools:       tools,
			Temperature: 0.1,
		}

		respMsg, err := c.callLLM(ctx, baseURL, req.APIKey, chatReq)
		if err != nil {
			return nil, fmt.Errorf("llm call failed at step %d: %w", step, err)
		}

		// If no tool calls, the model has concluded its investigation.
		if len(respMsg.ToolCalls) == 0 {
			report.Summary = respMsg.Content
			report.TotalSteps = step
			return report, nil
		}

		// Append assistant response to message history
		messages = append(messages, *respMsg)

		// Execute each tool call
		for _, tc := range respMsg.ToolCalls {
			toolName := tc.Function.Name
			toolArgs := tc.Function.Arguments

			// Mutation guard
			if !req.AllowMutation && isMutationTool(toolName) {
				toolResult := `{"error": "Mutations are disabled for this investigation run."}`
				messages = append(messages, chatMessage{
					Role:       "tool",
					Name:       toolName,
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
				report.Trace = append(report.Trace, StepRecord{
					Step:      step,
					Thought:   respMsg.Content,
					ToolName:  toolName,
					Arguments: toolArgs,
					Result:    toolResult,
				})
				continue
			}

			// Invoke tool via internal MCP server
			toolResult, isErr := c.invokeMCPTool(ctx, toolName, []byte(toolArgs))
			if !isErr && isMutationTool(toolName) {
				report.ActionsTaken = append(report.ActionsTaken, fmt.Sprintf("%s(%s)", toolName, toolArgs))
			}

			messages = append(messages, chatMessage{
				Role:       "tool",
				Name:       toolName,
				ToolCallID: tc.ID,
				Content:    toolResult,
			})

			report.Trace = append(report.Trace, StepRecord{
				Step:      step,
				Thought:   respMsg.Content,
				ToolName:  toolName,
				Arguments: toolArgs,
				Result:    toolResult,
			})
		}
	}

	report.TotalSteps = maxSteps
	report.Summary = "Investigation reached maximum allowable steps without final conclusion. See trace for details."
	return report, nil
}

func (c *Copilot) invokeMCPTool(ctx context.Context, name string, args []byte) (string, bool) {
	rpcReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
	}
	params := mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	}
	pBytes, _ := json.Marshal(params)
	rpcReq.Params = pBytes

	reqBytes, _ := json.Marshal(rpcReq)

	// Direct execution through in-memory MCP handler
	recorder := &fakeHTTPResponseWriter{header: make(http.Header), body: &bytes.Buffer{}}
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/mcp", bytes.NewReader(reqBytes))
	c.mcpServer.ServeHTTP(recorder, httpReq)

	var rpcResp mcp.JSONRPCResponse
	if err := json.Unmarshal(recorder.body.Bytes(), &rpcResp); err != nil {
		return fmt.Sprintf("tool execution failed: %v", err), true
	}
	if rpcResp.Error != nil {
		return fmt.Sprintf("RPC Error: %s", rpcResp.Error.Message), true
	}

	resBytes, _ := json.Marshal(rpcResp.Result)
	var callRes mcp.CallToolResult
	_ = json.Unmarshal(resBytes, &callRes)

	if len(callRes.Content) > 0 {
		return callRes.Content[0].Text, callRes.IsError
	}
	return string(resBytes), callRes.IsError
}

func (c *Copilot) callLLM(ctx context.Context, baseURL, apiKey string, reqPayload chatCompletionRequest) (*chatMessage, error) {
	url := fmt.Sprintf("%s/chat/completions", baseURL)
	b, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("model error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices from LLM")
	}

	return &chatResp.Choices[0].Message, nil
}

func (c *Copilot) buildOpenAITools(allowMutation bool) []chatTool {
	out := make([]chatTool, 0)
	for _, t := range c.mcpServer.ToolDefinitions() {
		if !allowMutation && isMutationTool(t.Name) {
			continue
		}
		out = append(out, chatTool{
			Type: "function",
			Function: chatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}
	return out
}

func isMutationTool(name string) bool {
	return name == "resin_isolate_node" ||
		name == "resin_recover_node" ||
		name == "resin_patch_platform" ||
		name == "resin_delete_lease" ||
		name == "resin_add_platform_exclusion" ||
		name == "resin_refresh_subscription"
}

func randomHexID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type fakeHTTPResponseWriter struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (f *fakeHTTPResponseWriter) Header() http.Header { return f.header }
func (f *fakeHTTPResponseWriter) Write(b []byte) (int, error) {
	return f.body.Write(b)
}
func (f *fakeHTTPResponseWriter) WriteHeader(statusCode int) { f.code = statusCode }
