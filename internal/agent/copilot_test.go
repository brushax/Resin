package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Resinat/Resin/internal/mcp"
)

func TestCopilot_Investigate_ReActLoop(t *testing.T) {
	store, err := NewAuditStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	mcpServer := mcp.NewServer(nil, nil)
	copilot := NewCopilot(mcpServer, store)

	var stepCount atomic.Int32

	// Mock LLM server providing a multi-step dynamic investigation:
	// Step 1: Model calls resin_search_logs
	// Step 2: Model concludes with root cause
	mockLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		currentStep := stepCount.Add(1)

		var resp chatCompletionResponse
		if currentStep == 1 {
			// Model decides to call resin_search_logs
			resp.Choices = []struct {
				Message chatMessage `json:"message"`
			}{
				{
					Message: chatMessage{
						Role:    "assistant",
						Content: "I will first search recent request logs for api.github.com to check failure patterns.",
						ToolCalls: []chatToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: chatFunctionCall{
									Name:      "resin_search_logs",
									Arguments: `{"target_host":"api.github.com","limit":5}`,
								},
							},
						},
					},
				},
			}
		} else {
			// Model concludes investigation
			resp.Choices = []struct {
				Message chatMessage `json:"message"`
			}{
				{
					Message: chatMessage{
						Role:    "assistant",
						Content: "### Root Cause Analysis\nBased on the log query, all 5 requests completed within 200ms with status 200. The network path is currently healthy.",
					},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockLLM.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := InvestigationRequest{
		Goal:     "Diagnose intermittent GitHub timeouts",
		BaseURL:  mockLLM.URL,
		Model:    "test-model",
		MaxSteps: 5,
	}
	report, err := copilot.Investigate(ctx, req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.TotalSteps != 2 {
		t.Fatalf("expected 2 total steps, got %d", report.TotalSteps)
	}
	if len(report.Trace) != 1 {
		t.Fatalf("expected 1 trace tool record, got %d", len(report.Trace))
	}
	if report.Trace[0].ToolName != "resin_search_logs" {
		t.Errorf("expected tool resin_search_logs, got %s", report.Trace[0].ToolName)
	}
	if !bytes.Contains([]byte(report.Summary), []byte("Root Cause Analysis")) {
		t.Errorf("summary does not contain expected conclusion: %s", report.Summary)
	}

	// Verify persistence in AuditStore
	runs, err := store.ListRuns(10, 0)
	if err != nil {
		t.Fatalf("failed to list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 persisted run, got %d", len(runs))
	}
	if runs[0].Goal != req.Goal {
		t.Errorf("expected goal %s, got %s", req.Goal, runs[0].Goal)
	}

	detail, err := store.GetRun(runs[0].ID)
	if err != nil || detail == nil {
		t.Fatalf("failed to fetch run detail: %v", err)
	}
	if len(detail.TraceJSON) == 0 {
		t.Error("expected non-empty trace JSON in persisted detail")
	}
}
