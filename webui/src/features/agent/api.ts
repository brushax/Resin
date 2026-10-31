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

export async function triggerDiagnose(input: TriggerDiagnoseInput): Promise<AgentInvestigationReport> {
  return apiRequest<AgentInvestigationReport>("/api/v1/agent/diagnose", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function listAgentRuns(limit = 20, offset = 0): Promise<PageResponse<AgentRunSummary>> {
  return apiRequest<PageResponse<AgentRunSummary>>(`/api/v1/agent/runs?limit=${limit}&offset=${offset}`);
}

export async function getAgentRun(id: string): Promise<AgentRunDetail> {
  return apiRequest<AgentRunDetail>(`/api/v1/agent/runs/${id}`);
}
