import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Bot,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  Play,
  RotateCcw,
  Sparkles,
  Wrench,
} from "lucide-react";
import { triggerDiagnose, listAgentRuns, getAgentRun } from "./api";
import type { AgentInvestigationReport } from "./types";
import { Button } from "../../components/ui/Button";

const presetGoals = [
  "排查最近 1 小时网络超时与慢连接集中在哪些站点",
  "诊断 Docker 镜像拉取与 CDN 连接质量，必要时执行自愈",
  "检查 Gemini 平台账号出口连通性与会话黏性状态",
  "扫描全网是否有被 Cloudflare 拦截或持续 504 的坏节点并处理",
];

export function CopilotPage() {
  const queryClient = useQueryClient();
  const [goal, setGoal] = useState("");
  const [allowMutation, setAllowMutation] = useState(true);
  const [activeReport, setActiveReport] = useState<AgentInvestigationReport | null>(null);
  const [expandedSteps, setExpandedSteps] = useState<Record<number, boolean>>({});

  const runsQuery = useQuery({
    queryKey: ["agent-runs"],
    queryFn: () => listAgentRuns(20, 0),
    refetchInterval: 30_000,
  });

  const diagnoseMutation = useMutation({
    mutationFn: triggerDiagnose,
    onSuccess: (data) => {
      setActiveReport(data);
      queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
    },
  });

  const handleRun = (selectedGoal?: string) => {
    const targetGoal = selectedGoal || goal;
    if (!targetGoal.trim()) return;
    diagnoseMutation.mutate({
      goal: targetGoal.trim(),
      allow_mutation: allowMutation,
    });
  };

  const toggleStep = (step: number) => {
    setExpandedSteps((prev) => ({ ...prev, [step]: !prev[step] }));
  };

  return (
    <div className="space-y-6">
      {/* 顶部 Header */}
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <Bot className="h-7 w-7 text-indigo-500" />
            AI 智能体诊断与自愈中心 (Copilot)
          </h1>
          <p className="text-sm text-neutral-400">
            基于原生 MCP 协议驱动的自主运维智能体，具备自主查询日志、现场探测验证与闭环热修复能力。
          </p>
        </div>
      </div>

      {/* 触发控制台 */}
      <div className="rounded-xl border border-neutral-800 bg-neutral-900/60 p-5 shadow-sm space-y-4">
        <div className="flex flex-col gap-3">
          <label className="text-sm font-medium text-neutral-200 flex items-center gap-2">
            <Sparkles className="h-4 w-4 text-amber-400" />
            输入调查目标或异常现象
          </label>
          <div className="flex flex-col md:flex-row gap-3">
            <input
              type="text"
              value={goal}
              onChange={(e) => setGoal(e.target.value)}
              placeholder="例如：排查最近 1 小时拉取镜像卡顿的原因，若有坏节点则自动热修复"
              className="flex-1 rounded-lg border border-neutral-700 bg-neutral-950 px-3.5 py-2.5 text-sm text-neutral-100 placeholder:text-neutral-500 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            />
            <Button
              onClick={() => handleRun()}
              disabled={diagnoseMutation.isPending || !goal.trim()}
              className="flex items-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white font-medium px-5"
            >
              {diagnoseMutation.isPending ? (
                <>
                  <Activity className="h-4 w-4 animate-spin" />
                  智能体自主排查中...
                </>
              ) : (
                <>
                  <Play className="h-4 w-4" />
                  发起排查
                </>
              )}
            </Button>
          </div>
        </div>

        {/* 快捷 Presets */}
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <span className="text-xs text-neutral-500">快速诊断预设：</span>
          {presetGoals.map((p, idx) => (
            <button
              key={idx}
              type="button"
              onClick={() => {
                setGoal(p);
                handleRun(p);
              }}
              className="rounded-full border border-neutral-800 bg-neutral-950/80 px-3 py-1 text-xs text-neutral-300 hover:border-indigo-500/50 hover:bg-neutral-800 transition"
            >
              {p}
            </button>
          ))}
        </div>

        {/* 权限开关 */}
        <div className="flex items-center justify-between border-t border-neutral-800/80 pt-3 text-xs">
          <label className="flex items-center gap-2 cursor-pointer select-none text-neutral-300">
            <input
              type="checkbox"
              checked={allowMutation}
              onChange={(e) => setAllowMutation(e.target.checked)}
              className="rounded border-neutral-700 bg-neutral-900 text-indigo-600 focus:ring-indigo-500"
            />
            <span>允许闭环热修复 (开启后若发现坏节点或规则错配，智能体可自主执行隔离、规则追加与驱逐，并自动复测验证)</span>
          </label>
        </div>
      </div>

      {/* 当前报告展示 */}
      {activeReport && (
        <div className="rounded-xl border border-indigo-500/30 bg-neutral-900/80 p-5 space-y-4 shadow-lg">
          <div className="flex items-center justify-between border-b border-neutral-800 pb-3">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="h-5 w-5 text-emerald-400" />
              <h2 className="text-base font-semibold text-neutral-100">
                诊断完成 · 共执行 {activeReport.total_steps} 轮自主探索
              </h2>
            </div>
            {activeReport.actions_taken && activeReport.actions_taken.length > 0 && (
              <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-xs font-medium text-emerald-400 border border-emerald-500/20">
                <Wrench className="h-3.5 w-3.5" />
                已执行 {activeReport.actions_taken.length} 项热修复
              </span>
            )}
          </div>

          {/* 总结卡片 */}
          <div className="rounded-lg bg-neutral-950 p-4 border border-neutral-800 text-sm leading-relaxed text-neutral-200 whitespace-pre-wrap font-sans">
            {activeReport.summary}
          </div>

          {/* 执行热修复列表 */}
          {activeReport.actions_taken && activeReport.actions_taken.length > 0 && (
            <div className="space-y-1.5">
              <div className="text-xs font-semibold text-emerald-400 uppercase tracking-wider flex items-center gap-1">
                <Wrench className="h-3 w-3" />
                生效的热修复动作
              </div>
              <ul className="space-y-1">
                {activeReport.actions_taken.map((act, i) => (
                  <li key={i} className="text-xs font-mono bg-emerald-950/30 border border-emerald-800/40 text-emerald-300 rounded px-2.5 py-1">
                    {act}
                  </li>
                ))}
              </ul>
            </div>
          )}

          {/* 步骤时间线 */}
          <div className="space-y-2 pt-2 border-t border-neutral-800/60">
            <div className="text-xs font-medium text-neutral-400 flex items-center gap-1">
              <Activity className="h-3.5 w-3.5 text-indigo-400" />
              智能体多轮 ReAct 执行 Trace
            </div>
            <div className="space-y-2">
              {activeReport.trace.map((tr) => (
                <div key={tr.step} className="rounded-lg border border-neutral-800/80 bg-neutral-950 text-xs">
                  <button
                    type="button"
                    onClick={() => toggleStep(tr.step)}
                    className="w-full flex items-center justify-between p-2.5 text-left text-neutral-300 hover:bg-neutral-900/50"
                  >
                    <div className="flex items-center gap-2">
                      <span className="rounded bg-neutral-800 px-1.5 py-0.5 font-mono text-[10px] text-neutral-400">
                        Step {tr.step}
                      </span>
                      <span className="font-mono text-indigo-300 font-medium">{tr.tool_name}</span>
                    </div>
                    {expandedSteps[tr.step] ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                  </button>
                  {expandedSteps[tr.step] && (
                    <div className="p-3 border-t border-neutral-800/60 space-y-2 bg-neutral-900/30">
                      {tr.thought && (
                        <div>
                          <span className="text-[10px] uppercase text-neutral-500 font-semibold">Thought</span>
                          <p className="text-neutral-300 mt-0.5">{tr.thought}</p>
                        </div>
                      )}
                      <div>
                        <span className="text-[10px] uppercase text-neutral-500 font-semibold">Arguments</span>
                        <pre className="font-mono text-[11px] bg-neutral-950 p-2 rounded border border-neutral-800 text-neutral-400 overflow-x-auto mt-0.5">
                          {tr.arguments}
                        </pre>
                      </div>
                      <div>
                        <span className="text-[10px] uppercase text-neutral-500 font-semibold">Observation</span>
                        <pre className="font-mono text-[11px] bg-neutral-950 p-2 rounded border border-neutral-800 text-emerald-400/90 overflow-x-auto mt-0.5 max-h-40">
                          {tr.result}
                        </pre>
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* 历史审计台账 */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-neutral-200 flex items-center gap-2">
            <Clock className="h-4 w-4 text-neutral-400" />
            历史自愈审计台账 (Audit Trail)
          </h2>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => runsQuery.refetch()}
            className="text-xs h-7"
          >
            <RotateCcw className="h-3 w-3 mr-1" />
            刷新
          </Button>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/40 overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-neutral-800 bg-neutral-900/80 text-neutral-400 font-medium">
                <tr>
                  <th className="py-2.5 px-3">时间</th>
                  <th className="py-2.5 px-3">调查目标</th>
                  <th className="py-2.5 px-3">状态</th>
                  <th className="py-2.5 px-3">步数</th>
                  <th className="py-2.5 px-3">热修复</th>
                  <th className="py-2.5 px-3">耗时</th>
                  <th className="py-2.5 px-3">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-800/60 text-neutral-300">
                {runsQuery.data?.items && runsQuery.data.items.length > 0 ? (
                  runsQuery.data.items.map((r) => (
                    <tr key={r.id} className="hover:bg-neutral-800/30">
                      <td className="py-2.5 px-3 font-mono text-neutral-400 whitespace-nowrap">
                        {new Date(r.created_at).toLocaleString()}
                      </td>
                      <td className="py-2.5 px-3 max-w-xs truncate font-medium text-neutral-200">
                        {r.goal}
                      </td>
                      <td className="py-2.5 px-3">
                        {r.status === "remediated" ? (
                          <span className="inline-flex items-center rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-medium text-emerald-400 border border-emerald-500/20">
                            已自愈
                          </span>
                        ) : (
                          <span className="inline-flex items-center rounded-full bg-neutral-800 px-2 py-0.5 text-[10px] font-medium text-neutral-400">
                            已诊断
                          </span>
                        )}
                      </td>
                      <td className="py-2.5 px-3">{r.total_steps} 步</td>
                      <td className="py-2.5 px-3 font-medium text-emerald-400">{r.remediations_count} 项</td>
                      <td className="py-2.5 px-3 font-mono text-neutral-400">{(r.duration_ms / 1000).toFixed(1)}s</td>
                      <td className="py-2.5 px-3">
                        <button
                          type="button"
                          onClick={() => {
                            // 将选中的记录设为 activeReport
                            getAgentRun(r.id).then((d) => {
                              try {
                                const tr = JSON.parse(d.trace_json);
                                setActiveReport({
                                  goal: d.goal,
                                  total_steps: d.total_steps,
                                  trace: tr,
                                  summary: d.summary,
                                  actions_taken: d.remediations_list,
                                });
                              } catch {
                                // fallback
                              }
                            });
                          }}
                          className="text-indigo-400 hover:text-indigo-300 font-medium"
                        >
                          查看 Trace
                        </button>
                      </td>
                    </tr>
                  ))
                ) : (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-neutral-500">
                      暂无历史自愈记录。在上方发起一次排查即可生成实时审计台账。
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
