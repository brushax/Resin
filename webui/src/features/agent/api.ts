import { apiRequest } from "../../lib/api-client";
import type { AgentInvestigationReport, AgentRunDetail, AgentRunSummary } from "./types";
import type { PageResponse } from "../platforms/types";

export type TriggerDiagnoseInput = {
  goal: string;
  allow_mutation?: boolean;
  base_url?: string;
  api_key?: string;
  model?: string;
  max_steps?: number;
};

// A diagnosis is a multi-step ReAct loop over an LLM endpoint and routinely
// takes minutes, so it needs a far longer deadline than a normal API call.
const DIAGNOSE_TIMEOUT_MS = 300_000;

export async function triggerDiagnose(input: TriggerDiagnoseInput): Promise<AgentInvestigationReport> {
  return apiRequest<AgentInvestigationReport>("/api/v1/agent/diagnose", {
    method: "POST",
    // apiRequest serializes the body itself; passing a pre-stringified value
    // sent a JSON string literal and the server rejected every request with
    // "cannot unmarshal string into Go value of type agent.InvestigationRequest".
    body: input,
    timeoutMs: DIAGNOSE_TIMEOUT_MS,
  });
}

export async function listAgentRuns(limit = 20, offset = 0): Promise<PageResponse<AgentRunSummary>> {
  return apiRequest<PageResponse<AgentRunSummary>>(`/api/v1/agent/runs?limit=${limit}&offset=${offset}`);
}

export async function getAgentRun(id: string): Promise<AgentRunDetail> {
  return apiRequest<AgentRunDetail>(`/api/v1/agent/runs/${id}`);
}
