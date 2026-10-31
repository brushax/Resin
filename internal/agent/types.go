package agent

import "encoding/json"

// OpenAI兼容格式请求与响应定义
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// InvestigationRequest triggers an autonomous agent investigation.
type InvestigationRequest struct {
	Goal          string `json:"goal"`
	BaseURL       string `json:"base_url,omitempty"` // OpenAI-compatible endpoint (e.g. https://api.example.com/v1)
	APIKey        string `json:"api_key,omitempty"`
	Model         string `json:"model,omitempty"`          // e.g. gpt-4o-mini or local model name
	MaxSteps      int    `json:"max_steps,omitempty"`      // maximum tool loop iterations (default 6)
	AllowMutation bool   `json:"allow_mutation,omitempty"` // whether the agent can execute isolation/patch actions
}

// StepRecord records an individual step in the agent's investigation.
type StepRecord struct {
	Step      int    `json:"step"`
	Thought   string `json:"thought,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
}

// InvestigationReport is the final structured report produced by the agent.
type InvestigationReport struct {
	Goal         string       `json:"goal"`
	TotalSteps   int          `json:"total_steps"`
	Trace        []StepRecord `json:"trace"`
	Summary      string       `json:"summary"`
	RootCause    string       `json:"root_cause,omitempty"`
	ActionsTaken []string     `json:"actions_taken,omitempty"`
}
