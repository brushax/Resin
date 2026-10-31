export type AgentRunSummary = {
  id: string;
  created_at: string;
  duration_ms: number;
  trigger_type: string;
  goal: string;
  model: string;
  status: "remediated" | "diagnosed" | "failed" | string;
  remediations_count: number;
  total_steps: number;
  summary: string;
};

export type AgentStepRecord = {
  step: number;
  thought?: string;
  tool_name?: string;
  arguments?: string;
  result?: string;
};

export type AgentInvestigationReport = {
  goal: string;
  total_steps: number;
  trace: AgentStepRecord[];
  summary: string;
  root_cause?: string;
  actions_taken?: string[];
};

export type AgentRunDetail = AgentRunSummary & {
  trace_json: string;
  remediations_list: string[];
};
