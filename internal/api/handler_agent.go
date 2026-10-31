package api

import (
	"net/http"

	"github.com/Resinat/Resin/internal/agent"
)

// HandleAgentDiagnose provides the HTTP API for triggering an autonomous investigation.
func HandleAgentDiagnose(copilot *agent.Copilot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if copilot == nil {
			http.Error(w, "AI copilot is not available", http.StatusServiceUnavailable)
			return
		}

		var req agent.InvestigationRequest
		if err := DecodeBody(r, &req); err != nil {
			writeDecodeBodyError(w, err)
			return
		}

		report, err := copilot.Investigate(r.Context(), req)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "INVESTIGATION_FAILED", err.Error())
			return
		}

		WriteJSON(w, http.StatusOK, report)
	}
}

// HandleListAgentRuns returns recent agent investigation records.
func HandleListAgentRuns(store *agent.AuditStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			WriteJSON(w, http.StatusOK, []any{})
			return
		}
		pg, ok := parsePaginationOrWriteInvalid(w, r)
		if !ok {
			return
		}
		runs, err := store.ListRuns(pg.Limit, pg.Offset)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		WritePage(w, http.StatusOK, runs, pg)
	}
}

// HandleGetAgentRun returns full details and trace JSON for a specific run.
func HandleGetAgentRun(store *agent.AuditStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "audit store is not available")
			return
		}
		id := PathParam(r, "id")
		if id == "" {
			writeInvalidArgument(w, "run_id is required")
			return
		}
		detail, err := store.GetRun(id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		if detail == nil {
			WriteError(w, http.StatusNotFound, "NOT_FOUND", "run not found")
			return
		}
		WriteJSON(w, http.StatusOK, detail)
	}
}
