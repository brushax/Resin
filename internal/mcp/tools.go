package mcp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Resinat/Resin/internal/node"
	"github.com/Resinat/Resin/internal/requestlog"
	"github.com/Resinat/Resin/internal/service"
	M "github.com/sagernet/sing/common/metadata"
)

func (s *Server) ToolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "resin_search_logs",
			Description: "Search recent proxy request logs with filters (target host, platform, status, duration, error state). Essential for diagnosing failures and stalls.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"target_host": {"type": "string", "description": "Filter by target host (e.g. api.github.com)"},
					"platform_name": {"type": "string", "description": "Filter by platform name (e.g. Docker, Gemini)"},
					"net_ok": {"type": "boolean", "description": "Filter by network success (true or false)"},
					"http_status": {"type": "integer", "description": "Filter by HTTP response status code"},
					"limit": {"type": "integer", "description": "Maximum number of logs to return (default 20, max 100)"}
				}
			}`),
		},
		{
			Name:        "resin_summarize_logs",
			Description: "Get an aggregated overview of recent traffic (total requests, failure count, top failed hosts, top errors, and slow requests). Start here when investigating network issues.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"limit": {"type": "integer", "description": "Number of recent logs to sample for the summary (default 200, max 1000)"}
				}
			}`),
		},
		{
			Name:        "resin_list_nodes",
			Description: "List all nodes in the global pool with their display tag, health status, region, active connections, and reference latency.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"platform_name": {"type": "string", "description": "Optional platform name to filter nodes within that platform's routable view"}
				}
			}`),
		},
		{
			Name:        "resin_get_node",
			Description: "Get deep diagnostics for a single node (by node_hash) including EWMA throughput, active in-flight connections, failure count, circuit breaker state, and per-domain latencies.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["node_hash"],
				"properties": {
					"node_hash": {"type": "string", "description": "Node hash hex string"}
				}
			}`),
		},
		{
			Name:        "resin_diagnose_target",
			Description: "Perform an on-demand live probe against a target host/domain via a specific platform or specific node. Returns TCP dial latency, TLS handshake latency, HTTP status, and headers.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["target"],
				"properties": {
					"target": {"type": "string", "description": "Target endpoint e.g. api.github.com:443 or https://auth.docker.io/token"},
					"platform_name": {"type": "string", "description": "Platform to route the probe through (default: Default)"},
					"node_hash": {"type": "string", "description": "Optional specific node hash hex to test directly"}
				}
			}`),
		},
		{
			Name:        "resin_list_platforms",
			Description: "List all configured platforms, their routable node counts, allocation policies, and active filter rules.",
			InputSchema: json.RawMessage(`{"type": "object"}`),
		},
		{
			Name:        "resin_patch_platform",
			Description: "Update a platform's routing filters or allocation policy (e.g. adjust regex, exclude slow regions, or switch policy to PREFER_THROUGHPUT).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["platform_id"],
				"properties": {
					"platform_id": {"type": "string", "description": "Platform UUID"},
					"regex_filters": {"type": "array", "items": {"type": "string"}, "description": "Tag regex filters"},
					"region_filters": {"type": "array", "items": {"type": "string"}, "description": "ISO country codes e.g. ['us', 'hk']"},
					"allocation_policy": {"type": "string", "enum": ["BALANCED", "PREFER_LOW_LATENCY", "PREFER_IDLE_IP", "ROUND_ROBIN", "PREFER_THROUGHPUT"]},
					"max_latency_ms": {"type": "integer", "description": "Maximum allowed reference latency in milliseconds (0 to disable)"}
				}
			}`),
		},
		{
			Name:        "resin_list_leases",
			Description: "List active sticky session leases (account -> node binding) for a platform.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["platform_id"],
				"properties": {
					"platform_id": {"type": "string", "description": "Platform UUID"}
				}
			}`),
		},
		{
			Name:        "resin_delete_lease",
			Description: "Evict a sticky lease for an account to force re-binding to a new IP on the next request.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["platform_id", "account"],
				"properties": {
					"platform_id": {"type": "string", "description": "Platform UUID"},
					"account": {"type": "string", "description": "Account identifier e.g. account_1"}
				}
			}`),
		},
		{
			Name:        "resin_isolate_node",
			Description: "Emergency intervention: open the circuit breaker on a failing or poisoned node, immediately removing it from all routable platforms.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["node_hash", "reason"],
				"properties": {
					"node_hash": {"type": "string", "description": "Node hash hex string"},
					"reason": {"type": "string", "description": "Explanation for why this node was isolated"}
				}
			}`),
		},
		{
			Name:        "resin_recover_node",
			Description: "Clear failure counts and circuit breaker on an isolated node, restoring it to the pool.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["node_hash"],
				"properties": {
					"node_hash": {"type": "string", "description": "Node hash hex string"}
				}
			}`),
		},
		{
			Name:        "resin_add_platform_exclusion",
			Description: "Hot remediation: Append an exclusion regex pattern (e.g. '!.*slow-provider.*') to an existing platform's filters and trigger routable view rebuild.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["platform_id", "exclude_pattern"],
				"properties": {
					"platform_id": {"type": "string", "description": "Platform UUID"},
					"exclude_pattern": {"type": "string", "description": "Exclusion regex pattern e.g. !.*LGA.* or !.*AnyTLS.*"}
				}
			}`),
		},
		{
			Name:        "resin_refresh_subscription",
			Description: "Hot remediation: Trigger an on-demand refresh of a remote proxy subscription to pull fresh endpoints.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["subscription_id"],
				"properties": {
					"subscription_id": {"type": "string", "description": "Subscription UUID"}
				}
			}`),
		},
		{
			Name:        "resin_verify_remediation",
			Description: "Closed-loop verification: Probe a target domain multiple times after applying hot remediations to confirm that the outage/stall has been resolved.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"required": ["target", "platform_name"],
				"properties": {
					"target": {"type": "string", "description": "Target endpoint to verify e.g. registry-1.docker.io:443"},
					"platform_name": {"type": "string", "description": "Platform name"},
					"rounds": {"type": "integer", "description": "Number of consecutive verification probes (default 3, max 5)"}
				}
			}`),
		},
	}
}

func (s *Server) registerTools() {
	s.tools["resin_search_logs"] = s.handleSearchLogs
	s.tools["resin_summarize_logs"] = s.handleSummarizeLogs
	s.tools["resin_list_nodes"] = s.handleListNodes
	s.tools["resin_get_node"] = s.handleGetNode
	s.tools["resin_diagnose_target"] = s.handleDiagnoseTarget
	s.tools["resin_list_platforms"] = s.handleListPlatforms
	s.tools["resin_patch_platform"] = s.handlePatchPlatform
	s.tools["resin_list_leases"] = s.handleListLeases
	s.tools["resin_delete_lease"] = s.handleDeleteLease
	s.tools["resin_isolate_node"] = s.handleIsolateNode
	s.tools["resin_recover_node"] = s.handleRecoverNode
	s.tools["resin_add_platform_exclusion"] = s.handleAddPlatformExclusion
	s.tools["resin_refresh_subscription"] = s.handleRefreshSubscription
	s.tools["resin_verify_remediation"] = s.handleVerifyRemediation
}

func (s *Server) handleSearchLogs(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.requestlogRepo == nil {
		return nil, fmt.Errorf("request log repository is not initialized")
	}

	var req struct {
		TargetHost   string `json:"target_host"`
		PlatformName string `json:"platform_name"`
		NetOK        *bool  `json:"net_ok"`
		HTTPStatus   *int   `json:"http_status"`
		Limit        int    `json:"limit"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	filter := requestlog.ListFilter{
		TargetHost:   req.TargetHost,
		PlatformName: req.PlatformName,
		NetOK:        req.NetOK,
		HTTPStatus:   req.HTTPStatus,
		Limit:        req.Limit,
		Fuzzy:        true,
	}

	logs, _, _, err := s.requestlogRepo.List(filter)
	if err != nil {
		return nil, err
	}

	type LogView struct {
		Time          string  `json:"time"`
		Platform      string  `json:"platform"`
		Account       string  `json:"account,omitempty"`
		Target        string  `json:"target"`
		NodeTag       string  `json:"node_tag"`
		EgressIP      string  `json:"egress_ip"`
		DurationSec   float64 `json:"duration_sec"`
		NetOK         bool    `json:"net_ok"`
		HTTPStatus    int     `json:"http_status"`
		Error         string  `json:"error,omitempty"`
		UpstreamStage string  `json:"upstream_stage,omitempty"`
	}

	view := make([]LogView, 0, len(logs))
	for _, l := range logs {
		t := time.Unix(0, l.TsNs).Format("2006-01-02 15:04:05")
		view = append(view, LogView{
			Time:          t,
			Platform:      l.PlatformName,
			Account:       l.Account,
			Target:        l.TargetHost,
			NodeTag:       l.NodeTag,
			EgressIP:      l.EgressIP,
			DurationSec:   float64(l.DurationNs) / 1e9,
			NetOK:         l.NetOK,
			HTTPStatus:    l.HTTPStatus,
			Error:         l.ResinError,
			UpstreamStage: l.UpstreamStage,
		})
	}
	return map[string]any{"count": len(view), "logs": view}, nil
}

func (s *Server) handleSummarizeLogs(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.requestlogRepo == nil {
		return nil, fmt.Errorf("request log repository is not initialized")
	}

	var req struct {
		Limit int `json:"limit"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	if req.Limit <= 0 {
		req.Limit = 200
	}
	if req.Limit > 1000 {
		req.Limit = 1000
	}

	logs, _, _, err := s.requestlogRepo.List(requestlog.ListFilter{Limit: req.Limit})
	if err != nil {
		return nil, err
	}

	total := len(logs)
	failed := 0
	slowCount := 0
	failedHosts := make(map[string]int)
	errors := make(map[string]int)
	nodeFailures := make(map[string]int)

	for _, l := range logs {
		if !l.NetOK {
			failed++
			failedHosts[l.TargetHost]++
			errKey := l.ResinError
			if errKey == "" {
				errKey = l.UpstreamStage
			}
			errors[errKey]++
			if l.NodeTag != "" {
				nodeFailures[l.NodeTag]++
			}
		}
		if l.DurationNs > 10*int64(time.Second) {
			slowCount++
		}
	}

	return map[string]any{
		"sampled_requests":    total,
		"failed_requests":     failed,
		"failure_rate_pct":    fmt.Sprintf("%.1f%%", float64(failed)/float64(max(total, 1))*100),
		"slow_requests_gt10s": slowCount,
		"top_failed_targets":  failedHosts,
		"top_errors":          errors,
		"top_failed_nodes":    nodeFailures,
	}, nil
}

func (s *Server) handleListNodes(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}

	var req struct {
		PlatformName string `json:"platform_name"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}

	var filters service.NodeFilters
	if req.PlatformName != "" {
		if p, ok := s.cp.Pool.GetPlatformByName(req.PlatformName); ok && p != nil {
			pid := p.ID
			filters.PlatformID = &pid
		}
	}
	nodes, err := s.cp.ListNodes(filters)
	if err != nil {
		return nil, err
	}

	type NodeBrief struct {
		Hash         string   `json:"node_hash"`
		DisplayTag   string   `json:"display_tag"`
		EgressIP     string   `json:"egress_ip"`
		Region       string   `json:"region"`
		CircuitOpen  bool     `json:"circuit_open"`
		FailureCount int      `json:"failure_count"`
		RefLatencyMs *float64 `json:"ref_latency_ms,omitempty"`
	}

	briefs := make([]NodeBrief, 0, len(nodes))
	for _, n := range nodes {
		briefs = append(briefs, NodeBrief{
			Hash:         n.NodeHash,
			DisplayTag:   n.DisplayTag,
			EgressIP:     n.EgressIP,
			Region:       n.Region,
			CircuitOpen:  n.CircuitOpenSince != nil,
			FailureCount: n.FailureCount,
			RefLatencyMs: n.ReferenceLatencyMs,
		})
	}
	return map[string]any{"count": len(briefs), "nodes": briefs}, nil
}

func (s *Server) handleGetNode(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}

	var req struct {
		NodeHash string `json:"node_hash"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}

	h, err := node.ParseHex(strings.TrimSpace(req.NodeHash))
	if err != nil {
		return nil, fmt.Errorf("invalid node hash: %w", err)
	}

	entry, ok := s.cp.Pool.GetEntry(h)
	if !ok {
		return nil, fmt.Errorf("node not found: %s", req.NodeHash)
	}

	summary, err := s.cp.GetNode(req.NodeHash)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"node":                 summary,
		"active_conns":         entry.ActiveConns.Load(),
		"throughput_bytes_sec": entry.GetThroughputBytesPerSec(),
		"throughput_mb_s":      fmt.Sprintf("%.2f MB/s", float64(entry.GetThroughputBytesPerSec())/(1024*1024)),
	}, nil
}

func (s *Server) handleDiagnoseTarget(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}

	var req struct {
		Target       string `json:"target"`
		PlatformName string `json:"platform_name"`
		NodeHash     string `json:"node_hash"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}
	if !strings.Contains(target, ":") && !strings.HasPrefix(target, "http") {
		target = target + ":443"
	}

	platName := req.PlatformName
	if platName == "" {
		platName = "Default"
	}

	var selectedHash node.Hash
	if req.NodeHash != "" {
		h, err := node.ParseHex(req.NodeHash)
		if err != nil {
			return nil, err
		}
		selectedHash = h
	} else {
		res, err := s.cp.Router.RouteRequest(platName, "", target)
		if err != nil {
			return nil, fmt.Errorf("routing failed for target: %w", err)
		}
		selectedHash = res.NodeHash
	}

	entry, ok := s.cp.Pool.GetEntry(selectedHash)
	if !ok {
		return nil, fmt.Errorf("node entry not found")
	}
	obPtr := entry.Outbound.Load()
	if obPtr == nil {
		return nil, fmt.Errorf("outbound not ready for node %s", selectedHash.Hex())
	}
	ob := *obPtr

	// Perform live probe dial
	tDialStart := time.Now()
	conn, err := ob.DialContext(ctx, "tcp", M.ParseSocksaddr(target))
	tDial := time.Since(tDialStart)
	if err != nil {
		return map[string]any{
			"status":      "FAILED",
			"target":      target,
			"node_tag":    entry.LastError,
			"node_hash":   selectedHash.Hex(),
			"dial_error":  err.Error(),
			"dial_time_s": tDial.Seconds(),
		}, nil
	}
	defer conn.Close()

	// If port is 443, test TLS handshake directly
	var tlsTime time.Duration
	var tlsState *tls.ConnectionState
	hostOnly := target
	if idx := strings.Index(hostOnly, ":"); idx > 0 {
		hostOnly = hostOnly[:idx]
	}
	if strings.HasSuffix(target, ":443") {
		tTLSStart := time.Now()
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         hostOnly,
			InsecureSkipVerify: true,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
		tlsErr := tlsConn.HandshakeContext(ctx)
		tlsTime = time.Since(tTLSStart)
		if tlsErr != nil {
			return map[string]any{
				"status":      "TLS_FAILED",
				"target":      target,
				"node_hash":   selectedHash.Hex(),
				"dial_time_s": tDial.Seconds(),
				"tls_error":   tlsErr.Error(),
				"tls_time_s":  tlsTime.Seconds(),
			}, nil
		}
		state := tlsConn.ConnectionState()
		tlsState = &state
	}

	res := map[string]any{
		"status":      "HEALTHY",
		"target":      target,
		"node_hash":   selectedHash.Hex(),
		"egress_ip":   entry.GetEgressIP().String(),
		"dial_time_s": tDial.Seconds(),
	}
	if tlsState != nil {
		res["tls_time_s"] = tlsTime.Seconds()
		res["tls_version"] = tls.VersionName(tlsState.Version)
		res["alpn"] = tlsState.NegotiatedProtocol
	}
	return res, nil
}

func (s *Server) handleListPlatforms(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	return s.cp.ListPlatforms()
}

func (s *Server) handlePatchPlatform(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		PlatformID string `json:"platform_id"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if req.PlatformID == "" {
		return nil, fmt.Errorf("platform_id is required")
	}
	return s.cp.UpdatePlatform(req.PlatformID, raw)
}

func (s *Server) handleListLeases(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		PlatformID string `json:"platform_id"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return s.cp.ListLeases(req.PlatformID)
}

func (s *Server) handleDeleteLease(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		PlatformID string `json:"platform_id"`
		Account    string `json:"account"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	err := s.cp.DeleteLease(req.PlatformID, req.Account)
	return map[string]string{"status": "deleted"}, err
}

func (s *Server) handleIsolateNode(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		NodeHash string `json:"node_hash"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	h, err := node.ParseHex(strings.TrimSpace(req.NodeHash))
	if err != nil {
		return nil, err
	}
	entry, ok := s.cp.Pool.GetEntry(h)
	if !ok {
		return nil, fmt.Errorf("node not found")
	}
	entry.CircuitOpenSince.Store(time.Now().UnixNano())
	entry.SetLastError("AI Copilot Isolated: " + req.Reason)
	s.cp.Pool.NotifyNodeDirty(h)
	return map[string]any{"status": "isolated", "node_hash": req.NodeHash, "reason": req.Reason}, nil
}

func (s *Server) handleRecoverNode(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		NodeHash string `json:"node_hash"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	h, err := node.ParseHex(strings.TrimSpace(req.NodeHash))
	if err != nil {
		return nil, err
	}
	s.cp.Pool.RecordResult(h, true)
	return map[string]any{"status": "recovered", "node_hash": req.NodeHash}, nil
}

func (s *Server) handleAddPlatformExclusion(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		PlatformID     string `json:"platform_id"`
		ExcludePattern string `json:"exclude_pattern"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	pattern := strings.TrimSpace(req.ExcludePattern)
	if pattern == "" {
		return nil, fmt.Errorf("exclude_pattern is required")
	}
	if !strings.HasPrefix(pattern, "!") {
		pattern = "!" + pattern
	}

	p, err := s.cp.GetPlatform(req.PlatformID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch platform: %w", err)
	}

	filters := append([]string(nil), p.RegexFilters...)
	// Avoid duplicates
	for _, f := range filters {
		if f == pattern {
			return map[string]any{"status": "already_present", "pattern": pattern}, nil
		}
	}
	filters = append(filters, pattern)

	patchBytes, _ := json.Marshal(map[string]any{
		"regex_filters": filters,
	})
	updated, err := s.cp.UpdatePlatform(req.PlatformID, patchBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to patch platform: %w", err)
	}
	return map[string]any{
		"status":        "remediated",
		"platform_id":   req.PlatformID,
		"pattern_added": pattern,
		"active_nodes":  updated.RoutableNodeCount,
	}, nil
}

func (s *Server) handleRefreshSubscription(ctx context.Context, raw json.RawMessage) (any, error) {
	if s.cp == nil {
		return nil, fmt.Errorf("control plane is not initialized")
	}
	var req struct {
		SubscriptionID string `json:"subscription_id"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if req.SubscriptionID == "" {
		return nil, fmt.Errorf("subscription_id is required")
	}
	err := s.cp.RefreshSubscription(req.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh subscription: %w", err)
	}
	return map[string]any{"status": "refreshed", "subscription_id": req.SubscriptionID}, nil
}

func (s *Server) handleVerifyRemediation(ctx context.Context, raw json.RawMessage) (any, error) {
	var req struct {
		Target       string `json:"target"`
		PlatformName string `json:"platform_name"`
		Rounds       int    `json:"rounds"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	rounds := req.Rounds
	if rounds <= 0 {
		rounds = 3
	}
	if rounds > 5 {
		rounds = 5
	}

	results := make([]any, 0, rounds)
	successes := 0
	for i := 0; i < rounds; i++ {
		diagRaw, _ := json.Marshal(map[string]any{
			"target":        req.Target,
			"platform_name": req.PlatformName,
		})
		res, err := s.handleDiagnoseTarget(ctx, diagRaw)
		if err == nil {
			if m, ok := res.(map[string]any); ok && m["status"] == "HEALTHY" {
				successes++
			}
			results = append(results, res)
		} else {
			results = append(results, map[string]string{"error": err.Error()})
		}
		time.Sleep(200 * time.Millisecond)
	}

	return map[string]any{
		"verification_rounds": rounds,
		"success_count":       successes,
		"verified_healthy":    successes == rounds,
		"details":             results,
	}, nil
}
