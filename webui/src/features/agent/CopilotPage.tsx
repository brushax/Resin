import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  Bot,
  ChevronRight,
  Clock,
  ListChecks,
  Play,
  RefreshCw,
  Sparkles,
  Wrench,
  X,
} from "lucide-react";
import { getAgentRun, listAgentRuns, triggerDiagnose } from "./api";
import type { AgentRunSummary, AgentStepRecord } from "./types";
import { Button } from "../../components/ui/Button";
import { Badge, type BadgeVariant } from "../../components/ui/Badge";
import { Card } from "../../components/ui/Card";
import { Switch } from "../../components/ui/Switch";
import { Textarea } from "../../components/ui/Textarea";
import { OffsetPagination } from "../../components/ui/OffsetPagination";
import { useI18n } from "../../i18n";
import { formatApiErrorMessage } from "../../lib/error-message";
import { formatDateTime, formatRelativeTime } from "../../lib/time";

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const;

// Each preset keeps a short chip label separate from the full instruction sent
// to the agent, so the chips stay readable on a phone.
const PRESET_GOALS = [
  { label: "最近 1 小时超时", goal: "排查最近 1 小时网络超时与慢连接集中在哪些站点" },
  { label: "Docker 拉取质量", goal: "诊断 Docker 镜像拉取与 CDN 连接质量，必要时执行自愈" },
  { label: "Gemini 出口与黏性", goal: "检查 Gemini 平台账号出口连通性与会话黏性状态" },
  { label: "Cloudflare 拦截与 504", goal: "扫描全网是否有被 Cloudflare 拦截或持续 504 的坏节点并处理" },
] as const;

type RunView = {
  id?: string;
  goal: string;
  status: string;
  totalSteps: number;
  summary: string;
  actions: string[];
  trace: AgentStepRecord[];
  createdAt?: string;
  durationMs?: number;
  model?: string;
};

function parseTrace(raw: string | undefined): AgentStepRecord[] {
  if (!raw) {
    return [];
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as AgentStepRecord[]) : [];
  } catch {
    return [];
  }
}

function statusVariant(status: string): BadgeVariant {
  switch (status) {
    case "remediated":
      return "success";
    case "diagnosed":
      return "info";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

export function CopilotPage() {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const [goal, setGoal] = useState("");
  const [allowMutation, setAllowMutation] = useState(true);
  const [result, setResult] = useState<RunView | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [freshTrace, setFreshTrace] = useState<RunView | null>(null);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState<number>(PAGE_SIZE_OPTIONS[0]);
  const [expandedSteps, setExpandedSteps] = useState<Record<number, boolean>>({});

  const runsQuery = useQuery({
    queryKey: ["agent-runs", page, pageSize],
    queryFn: () => listAgentRuns(pageSize, page * pageSize),
    refetchInterval: 30_000,
    placeholderData: (previous) => previous,
  });

  const runDetailQuery = useQuery({
    queryKey: ["agent-run", selectedRunId],
    queryFn: () => getAgentRun(selectedRunId as string),
    enabled: Boolean(selectedRunId),
    staleTime: 60_000,
  });

  const diagnoseMutation = useMutation({
    mutationFn: triggerDiagnose,
    onSuccess: (data) => {
      setResult({
        goal: data.goal,
        status: data.actions_taken && data.actions_taken.length > 0 ? "remediated" : "diagnosed",
        totalSteps: data.total_steps,
        summary: data.summary,
        actions: data.actions_taken ?? [],
        trace: Array.isArray(data.trace) ? data.trace : [],
      });
      void queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
    },
  });

  const detailView = useMemo<RunView | null>(() => {
    const detail = runDetailQuery.data;
    if (!detail) {
      return null;
    }
    return {
      id: detail.id,
      goal: detail.goal,
      status: detail.status,
      totalSteps: detail.total_steps,
      summary: detail.summary,
      actions: detail.remediations_list ?? [],
      trace: parseTrace(detail.trace_json),
      createdAt: detail.created_at,
      durationMs: detail.duration_ms,
      model: detail.model,
    };
  }, [runDetailQuery.data]);

  const runs = runsQuery.data?.items ?? [];
  const totalRuns = runsQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(totalRuns / pageSize));
  const drawerView = freshTrace ?? detailView;
  const drawerOpen = Boolean(freshTrace || selectedRunId);

  const statusLabel = (status: string): string => {
    switch (status) {
      case "remediated":
        return t("已自愈");
      case "diagnosed":
        return t("已诊断");
      case "no_action_needed":
        return t("无需处理");
      case "failed":
        return t("失败");
      default:
        return status || t("未知");
    }
  };

  const runDiagnose = (targetGoal?: string) => {
    const nextGoal = (targetGoal ?? goal).trim();
    if (!nextGoal || diagnoseMutation.isPending) {
      return;
    }
    if (targetGoal) {
      setGoal(targetGoal);
    }
    setExpandedSteps({});
    diagnoseMutation.mutate({ goal: nextGoal, allow_mutation: allowMutation });
  };

  const openRun = (id: string) => {
    setExpandedSteps({});
    setFreshTrace(null);
    setSelectedRunId(id);
  };

  const openResultTrace = (view: RunView) => {
    setExpandedSteps({});
    if (view.id) {
      setSelectedRunId(view.id);
      return;
    }
    setSelectedRunId(null);
    setFreshTrace(view);
  };

  const closeDrawer = () => {
    setSelectedRunId(null);
    setFreshTrace(null);
    setExpandedSteps({});
  };

  const toggleStep = (step: number) => {
    setExpandedSteps((previous) => ({ ...previous, [step]: !previous[step] }));
  };

  const renderTrace = (view: RunView) => {
    if (!view.trace.length) {
      return <p className="muted">{t("本次诊断没有记录执行轨迹。")}</p>;
    }
    return (
      <div className="copilot-trace">
        {view.trace.map((step) => {
          const expanded = Boolean(expandedSteps[step.step]);
          return (
            <div className="copilot-step" key={step.step}>
              <button
                type="button"
                className="copilot-step-head"
                onClick={() => toggleStep(step.step)}
                aria-expanded={expanded}
              >
                <span className="copilot-step-index">{t("第 {{step}} 步", { step: step.step })}</span>
                <span className="copilot-step-tool">{step.tool_name || t("最终结论")}</span>
                <ChevronRight
                  size={16}
                  className={expanded ? "copilot-step-chevron open" : "copilot-step-chevron"}
                />
              </button>
              {expanded ? (
                <div className="copilot-step-body">
                  {step.thought ? (
                    <section>
                      <h4>{t("思考")}</h4>
                      <p>{step.thought}</p>
                    </section>
                  ) : null}
                  {step.arguments ? (
                    <section>
                      <h4>{t("参数")}</h4>
                      <pre className="copilot-code">{step.arguments}</pre>
                    </section>
                  ) : null}
                  {step.result ? (
                    <section>
                      <h4>{t("观察结果")}</h4>
                      <pre className="copilot-code">{step.result}</pre>
                    </section>
                  ) : null}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    );
  };

  return (
    <section className="copilot-page">
      <header className="module-header">
        <div>
          <h2>{t("AI 诊断自愈")}</h2>
          <p className="module-description">
            {t("自主运维智能体：检索请求日志、现场探测节点、定位根因，并在授权后执行闭环热修复。")}
          </p>
        </div>
        <div className="copilot-header-actions">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => void runsQuery.refetch()}
            disabled={runsQuery.isFetching}
          >
            <RefreshCw size={16} className={runsQuery.isFetching ? "spin" : undefined} />
            {t("刷新")}
          </Button>
        </div>
      </header>

      <Card className="copilot-console">
        <div className="copilot-console-head">
          <div>
            <h3>
              <Sparkles size={16} />
              {t("发起诊断")}
            </h3>
            <p className="muted">
              {t("描述你观察到的现象或想排查的问题，智能体会按 ReAct 流程自主取证。")}
            </p>
          </div>
        </div>

        <label className="field-label" htmlFor="copilot-goal">
          {t("诊断目标")}
        </label>
        <Textarea
          id="copilot-goal"
          rows={3}
          value={goal}
          maxLength={500}
          onChange={(event) => setGoal(event.target.value)}
          placeholder={t("例如：排查最近 1 小时拉取镜像卡顿的原因，若发现坏节点则自动隔离")}
          disabled={diagnoseMutation.isPending}
        />

        <div className="copilot-presets">
          <span className="copilot-presets-label">{t("快速预设")}</span>
          {PRESET_GOALS.map((preset) => (
            <button
              key={preset.label}
              type="button"
              className="copilot-chip"
              title={t(preset.goal)}
              disabled={diagnoseMutation.isPending}
              onClick={() => runDiagnose(t(preset.goal))}
            >
              {t(preset.label)}
            </button>
          ))}
        </div>

        <div className="copilot-console-foot">
          <label className="copilot-toggle">
            <Switch
              checked={allowMutation}
              onChange={(event) => setAllowMutation(event.target.checked)}
              disabled={diagnoseMutation.isPending}
            />
            <span>
              <strong>{t("允许闭环热修复")}</strong>
              <small>
                {t("开启后可隔离坏节点、追加平台排除规则，并在修复后自动复测；关闭则只诊断不写配置。")}
              </small>
            </span>
          </label>

          <Button
            onClick={() => runDiagnose()}
            disabled={diagnoseMutation.isPending || !goal.trim()}
          >
            {diagnoseMutation.isPending ? (
              <>
                <Activity size={16} className="spin" />
                {t("智能体排查中...")}
              </>
            ) : (
              <>
                <Play size={16} />
                {t("发起排查")}
              </>
            )}
          </Button>
        </div>

        {diagnoseMutation.isPending ? (
          <p className="copilot-pending-hint">
            <Clock size={14} />
            {t("多轮取证通常需要 1-3 分钟，请保持页面在前台。")}
          </p>
        ) : null}

        {diagnoseMutation.isError ? (
          <div className="callout callout-error">
            <AlertTriangle size={14} />
            <span>{formatApiErrorMessage(diagnoseMutation.error, t)}</span>
          </div>
        ) : null}
      </Card>

      {result ? (
        <Card className="copilot-report">
          <div className="copilot-section-title">
            <Bot size={16} />
            <span>{t("本次诊断结果")}</span>
          </div>

          <div className="copilot-report-head">
            <div className="copilot-report-copy">
              <p className="copilot-report-goal">{result.goal}</p>
              <div className="copilot-report-meta">
                <Badge variant={statusVariant(result.status)}>{statusLabel(result.status)}</Badge>
                <span>{t("共 {{count}} 轮探索", { count: result.totalSteps })}</span>
                {result.actions.length ? (
                  <span className="copilot-meta-strong">
                    {t("已执行 {{count}} 项热修复", { count: result.actions.length })}
                  </span>
                ) : null}
              </div>
            </div>
            <Button variant="secondary" size="sm" onClick={() => openResultTrace(result)}>
              <ListChecks size={16} />
              {t("查看 {{count}} 步轨迹", { count: result.trace.length })}
            </Button>
          </div>

          <div className="copilot-summary">
            <h4>{t("诊断结论")}</h4>
            <p>{result.summary || t("智能体没有返回结论文本。")}</p>
          </div>

          {result.actions.length ? (
            <div className="copilot-actions">
              <h4>
                <Wrench size={14} />
                {t("生效的热修复动作")}
              </h4>
              <ul>
                {result.actions.map((action, index) => (
                  <li key={`${index}-${action}`}>{action}</li>
                ))}
              </ul>
            </div>
          ) : null}
        </Card>
      ) : null}

      <Card className="copilot-history">
        <div className="copilot-section-title">
          <Clock size={16} />
          <span>{t("历史诊断台账")}</span>
          {totalRuns > 0 ? (
            <span className="copilot-count">{t("共 {{count}} 条", { count: totalRuns })}</span>
          ) : null}
        </div>

        {runsQuery.isLoading ? <p className="muted">{t("正在加载历史记录...")}</p> : null}

        {runsQuery.isError ? (
          <div className="callout callout-error">
            <AlertTriangle size={14} />
            <span>{formatApiErrorMessage(runsQuery.error, t)}</span>
          </div>
        ) : null}

        {!runsQuery.isLoading && !runsQuery.isError && runs.length === 0 ? (
          <div className="empty-box">
            <Sparkles size={16} />
            <p>{t("暂无历史诊断记录，在上方发起一次排查即可自动归档。")}</p>
          </div>
        ) : null}

        {runs.length ? (
          <div className="copilot-table-wrap">
            <table className="copilot-table">
              <thead>
                <tr>
                  <th scope="col">{t("时间")}</th>
                  <th scope="col">{t("调查目标")}</th>
                  <th scope="col">{t("状态")}</th>
                  <th scope="col">{t("步数")}</th>
                  <th scope="col">{t("热修复")}</th>
                  <th scope="col">{t("运行时长")}</th>
                  <th scope="col">
                    <span className="copilot-sr-only">{t("操作")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {runs.map((run: AgentRunSummary) => (
                  <tr key={run.id} className="copilot-row">
                    <td data-label={t("时间")}>
                      <span className="copilot-time">{formatDateTime(run.created_at)}</span>
                    </td>
                    <td data-label={t("调查目标")} className="copilot-cell-goal">
                      <span className="copilot-goal-cell">{run.goal}</span>
                    </td>
                    <td data-label={t("状态")}>
                      <Badge variant={statusVariant(run.status)}>{statusLabel(run.status)}</Badge>
                    </td>
                    <td data-label={t("步数")} className="copilot-num">
                      {run.total_steps}
                    </td>
                    <td data-label={t("热修复")} className="copilot-num">
                      {run.remediations_count}
                    </td>
                    <td data-label={t("运行时长")} className="copilot-num">
                      {(run.duration_ms / 1000).toFixed(1)}s
                    </td>
                    <td data-label={t("操作")} className="copilot-cell-action">
                      <button type="button" className="copilot-link" onClick={() => openRun(run.id)}>
                        {t("查看轨迹")}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}

        {totalRuns > 0 ? (
          <OffsetPagination
            page={page}
            totalPages={totalPages}
            totalItems={totalRuns}
            pageSize={pageSize}
            pageSizeOptions={PAGE_SIZE_OPTIONS}
            onPageChange={setPage}
            onPageSizeChange={(next) => {
              setPageSize(next);
              setPage(0);
            }}
          />
        ) : null}
      </Card>

      {drawerOpen ? (
        <div
          className="drawer-overlay"
          role="dialog"
          aria-modal="true"
          aria-label={t("诊断记录详情")}
          onClick={closeDrawer}
        >
          <Card className="drawer-panel copilot-drawer" onClick={(event) => event.stopPropagation()}>
            <div className="drawer-header">
              <div>
                <h3>{t("执行轨迹")}</h3>
                <p>{drawerView?.goal}</p>
              </div>
              <div className="drawer-header-actions">
                <Button variant="ghost" size="sm" onClick={closeDrawer} aria-label={t("关闭")}>
                  <X size={16} />
                </Button>
              </div>
            </div>

            {selectedRunId && runDetailQuery.isLoading ? (
              <p className="muted">{t("正在加载轨迹...")}</p>
            ) : null}

            {selectedRunId && runDetailQuery.isError ? (
              <div className="callout callout-error">
                <AlertTriangle size={14} />
                <span>{formatApiErrorMessage(runDetailQuery.error, t)}</span>
              </div>
            ) : null}

            {drawerView ? (
              <>
                <div className="copilot-drawer-meta">
                  <Badge variant={statusVariant(drawerView.status)}>{statusLabel(drawerView.status)}</Badge>
                  <span>{t("共 {{count}} 轮探索", { count: drawerView.totalSteps })}</span>
                  {drawerView.model ? <span>{drawerView.model}</span> : null}
                  {drawerView.createdAt ? (
                    <span>
                      {formatDateTime(drawerView.createdAt)} · {formatRelativeTime(drawerView.createdAt)}
                    </span>
                  ) : null}
                </div>

                <div className="copilot-summary">
                  <h4>{t("诊断结论")}</h4>
                  <p>{drawerView.summary || t("智能体没有返回结论文本。")}</p>
                </div>

                {drawerView.actions.length ? (
                  <div className="copilot-actions">
                    <h4>
                      <Wrench size={14} />
                      {t("已执行 {{count}} 项热修复", { count: drawerView.actions.length })}
                    </h4>
                    <ul>
                      {drawerView.actions.map((action, index) => (
                        <li key={`${index}-${action}`}>{action}</li>
                      ))}
                    </ul>
                  </div>
                ) : null}

                {renderTrace(drawerView)}
              </>
            ) : null}
          </Card>
        </div>
      ) : null}
    </section>
  );
}
