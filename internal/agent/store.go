package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// RunSummary represents a high-level summary of an agent investigation run.
type RunSummary struct {
	ID                string    `json:"id"`
	CreatedAt         time.Time `json:"created_at"`
	DurationMs        int64     `json:"duration_ms"`
	TriggerType       string    `json:"trigger_type"`
	Goal              string    `json:"goal"`
	Model             string    `json:"model"`
	Status            string    `json:"status"` // "remediated", "diagnosed", "failed", "no_action_needed"
	RemediationsCount int       `json:"remediations_count"`
	TotalSteps        int       `json:"total_steps"`
	Summary           string    `json:"summary"`
}

// RunDetail includes the full execution trace and raw JSON data.
type RunDetail struct {
	RunSummary
	TraceJSON        string   `json:"trace_json"`
	RemediationsList []string `json:"remediations_list"`
}

// AuditStore manages SQLite persistence of all AI Agent investigation and remediation runs.
type AuditStore struct {
	mu     sync.Mutex
	db     *sql.DB
	logDir string
}

func NewAuditStore(logDir string) (*AuditStore, error) {
	if logDir == "" {
		logDir = "./data/log"
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("agent store: failed to create dir %s: %w", logDir, err)
	}

	dbPath := filepath.Join(logDir, "agent_audit.db")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_journal=WAL&_busy_timeout=5000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("agent store: failed to open sqlite %s: %w", dbPath, err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS agent_runs (
		id                 TEXT PRIMARY KEY,
		created_at_ns      INTEGER NOT NULL,
		duration_ms        INTEGER NOT NULL,
		trigger_type       TEXT NOT NULL,
		goal               TEXT NOT NULL,
		model              TEXT NOT NULL,
		status             TEXT NOT NULL,
		total_steps        INTEGER NOT NULL,
		trace_json         TEXT NOT NULL,
		remediations_json  TEXT NOT NULL,
		summary            TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_agent_runs_created ON agent_runs(created_at_ns DESC);
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("agent store: failed to init schema: %w", err)
	}

	return &AuditStore{
		db:     db,
		logDir: logDir,
	}, nil
}

func (s *AuditStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// RecordRun persists an entire agent run record with complete execution trace.
func (s *AuditStore) RecordRun(detail RunDetail) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	remedJSON, _ := json.Marshal(detail.RemediationsList)

	query := `
	INSERT INTO agent_runs (
		id, created_at_ns, duration_ms, trigger_type, goal, model,
		status, total_steps, trace_json, remediations_json, summary
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := s.db.Exec(
		query,
		detail.ID,
		detail.CreatedAt.UnixNano(),
		detail.DurationMs,
		detail.TriggerType,
		detail.Goal,
		detail.Model,
		detail.Status,
		detail.TotalSteps,
		detail.TraceJSON,
		string(remedJSON),
		detail.Summary,
	)
	return err
}

// ListRuns returns recent agent runs for developer analysis and audits.
func (s *AuditStore) ListRuns(limit, offset int) ([]RunSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT id, created_at_ns, duration_ms, trigger_type, goal, model, status, total_steps, remediations_json, summary
		 FROM agent_runs ORDER BY created_at_ns DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RunSummary
	for rows.Next() {
		var item RunSummary
		var createdAtNs int64
		var remedRaw string
		if err := rows.Scan(
			&item.ID, &createdAtNs, &item.DurationMs, &item.TriggerType,
			&item.Goal, &item.Model, &item.Status, &item.TotalSteps,
			&remedRaw, &item.Summary,
		); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(0, createdAtNs)
		var remedList []string
		_ = json.Unmarshal([]byte(remedRaw), &remedList)
		item.RemediationsCount = len(remedList)
		list = append(list, item)
	}
	return list, nil
}

// GetRun returns full execution details including JSON trace for a specific run.
func (s *AuditStore) GetRun(id string) (*RunDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT id, created_at_ns, duration_ms, trigger_type, goal, model, status, total_steps, trace_json, remediations_json, summary
		 FROM agent_runs WHERE id = ?`, id,
	)

	var detail RunDetail
	var createdAtNs int64
	var remedRaw string
	if err := row.Scan(
		&detail.ID, &createdAtNs, &detail.DurationMs, &detail.TriggerType,
		&detail.Goal, &detail.Model, &detail.Status, &detail.TotalSteps,
		&detail.TraceJSON, &remedRaw, &detail.Summary,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	detail.CreatedAt = time.Unix(0, createdAtNs)
	_ = json.Unmarshal([]byte(remedRaw), &detail.RemediationsList)
	detail.RemediationsCount = len(detail.RemediationsList)
	return &detail, nil
}
