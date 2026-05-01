package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/k8nstantin/OpenPraxis/internal/node"
	"github.com/k8nstantin/OpenPraxis/internal/schedule"
)

// HTTP surface for the central SCD-2 schedules table. Mirrors the shape
// of handlers_comments.go: thin wrapper over schedule.Store, sentinel
// errors → JSON {error, code} pairs the UI matches on.
//
// Routes registered by registerScheduleRoutes:
//   GET    /api/schedules?entity_kind=...&entity_id=...&as_of=<rfc3339>
//   GET    /api/schedules?entity_kind=...&entity_id=...&history=true
//   POST   /api/schedules
//   DELETE /api/schedules/{id}?reason=...
//   GET    /api/schedules/templates
//
// The entity_id query param accepts short markers; the resolver
// canonicalizes to full UUID before insert/read so rows are never
// orphaned by visceral rule #14.

// scheduleMaxBodyBytes caps create-schedule POST bodies at 256 KiB.
// Schedules are tiny (a few JSON keys); anything larger is malformed.
const scheduleMaxBodyBytes = 1 << 18

// scheduleTemplates is the recurrence preset list surfaced via
// /api/schedules/templates. The UI's "How frequent" dropdown reads
// from this so we can extend the catalog without a frontend change.
//
// `custom` falls through to a free-text cron input in the UI; cron is
// "" in the template so the UI knows to reveal the input.
var scheduleTemplates = []map[string]string{
	{"key": "once", "label": "One-shot", "cron": ""},
	{"key": "every_15m", "label": "Every 15 minutes", "cron": "*/15 * * * *"},
	{"key": "hourly", "label": "Hourly", "cron": "0 * * * *"},
	{"key": "every_4h", "label": "Every 4 hours", "cron": "0 */4 * * *"},
	{"key": "daily_9am", "label": "Daily at 9am", "cron": "0 9 * * *"},
	{"key": "weekly_mon", "label": "Weekly (Mon 9am)", "cron": "0 9 * * 1"},
	{"key": "monthly_1st", "label": "Monthly (1st 9am)", "cron": "0 9 1 * *"},
	{"key": "custom", "label": "Custom cron…", "cron": ""},
}

// writeScheduleError emits {error, code} at the given status. The
// frontend matches `code` for stable copy; `error` carries the human
// message for debug surfaces. Mirrors writeCommentError.
func writeScheduleError(w http.ResponseWriter, code, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

// scheduleErrorStatus maps a schedule sentinel to (code, status).
func scheduleErrorStatus(err error) (string, int) {
	switch {
	case errors.Is(err, schedule.ErrInvalidKind):
		return "invalid_kind", http.StatusBadRequest
	case errors.Is(err, schedule.ErrEmptyEntityID):
		return "empty_entity_id", http.StatusBadRequest
	case errors.Is(err, schedule.ErrEmptyRunAt):
		return "empty_run_at", http.StatusBadRequest
	}
	return "internal", http.StatusInternalServerError
}

// resolveScheduleEntity canonicalizes the (entity_kind, entity_id) pair
// to a full UUID via the node's existing per-kind resolvers. Same gate
// the comments handler uses — short markers in the URL must not orphan
// schedule rows. Returns "" for an empty entity_id.
func resolveScheduleEntity(n *node.Node, kind, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	switch kind {
	case schedule.KindProduct:
		return n.ResolveProductID(id)
	case schedule.KindManifest:
		return n.ResolveManifestID(id)
	case schedule.KindTask:
		if n.Tasks == nil {
			return id, nil
		}
		t, err := n.Tasks.Get(id)
		if err != nil {
			return "", fmt.Errorf("resolve task %q: %w", id, err)
		}
		if t == nil {
			return "", fmt.Errorf("task not found: %s", id)
		}
		return t.ID, nil
	}
	return id, nil
}

// apiSchedulesList handles
//   GET /api/schedules?entity_kind=...&entity_id=...&history=true|false&as_of=<rfc3339>
//
// Modes (mutually exclusive — server picks first that matches):
//   1. history=true     → ListHistory (closed + active rows; valid_from DESC)
//   2. as_of=<ts>       → ListAsOf    (rows current at the given timestamp)
//   3. (default)        → ListCurrent (only active rows; run_at ASC)
func apiSchedulesList(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind := q.Get("entity_kind")
		rawID := q.Get("entity_id")
		if kind == "" || rawID == "" {
			writeScheduleError(w, "missing_params",
				"entity_kind + entity_id are required",
				http.StatusBadRequest)
			return
		}
		id, err := resolveScheduleEntity(n, kind, rawID)
		if err != nil {
			writeScheduleError(w, "entity_not_found", err.Error(), http.StatusNotFound)
			return
		}

		var rows []*schedule.Schedule
		switch {
		case q.Get("history") == "true":
			rows, err = n.Schedules.ListHistory(r.Context(), kind, id)
		case q.Get("as_of") != "":
			ts, perr := time.Parse(time.RFC3339Nano, q.Get("as_of"))
			if perr != nil {
				// Try RFC3339 (no fractional seconds) as a fallback —
				// most callers won't send nanos.
				ts, perr = time.Parse(time.RFC3339, q.Get("as_of"))
			}
			if perr != nil {
				writeScheduleError(w, "invalid_as_of",
					"as_of must be ISO8601 (RFC3339)",
					http.StatusBadRequest)
				return
			}
			rows, err = n.Schedules.ListAsOf(r.Context(), ts, kind, id)
		default:
			rows, err = n.Schedules.ListCurrent(r.Context(), kind, id)
		}
		if err != nil {
			code, status := scheduleErrorStatus(err)
			writeScheduleError(w, code, err.Error(), status)
			return
		}
		// Always return a JSON array (not null) so the UI can map over
		// the response without a falsy guard.
		if rows == nil {
			rows = []*schedule.Schedule{}
		}
		writeJSON(w, rows)
	}
}

// createScheduleRequest is the POST body shape. Enabled defaults to
// true when the field is omitted (we can't distinguish "absent" from
// "false" via plain bool — see the helper below).
type createScheduleRequest struct {
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	RunAt      string `json:"run_at"`
	CronExpr   string `json:"cron_expr"`
	Timezone   string `json:"timezone"`
	MaxRuns    int    `json:"max_runs"`
	StopAt     string `json:"stop_at"`
	Enabled    *bool  `json:"enabled,omitempty"`
	Metadata   string `json:"metadata"`
	CreatedBy  string `json:"created_by"`
	Reason     string `json:"reason"`
}

// apiSchedulesCreate handles POST /api/schedules. Validates, resolves
// entity_id to full UUID, and inserts. Returns 201 + the persisted row.
func apiSchedulesCreate(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, scheduleMaxBodyBytes)
		var req createScheduleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeScheduleError(w, "invalid_body",
				"invalid request body: "+err.Error(),
				http.StatusBadRequest)
			return
		}
		if req.EntityKind == "" || req.EntityID == "" {
			writeScheduleError(w, "missing_params",
				"entity_kind + entity_id are required",
				http.StatusBadRequest)
			return
		}
		id, err := resolveScheduleEntity(n, req.EntityKind, req.EntityID)
		if err != nil {
			writeScheduleError(w, "entity_not_found", err.Error(), http.StatusNotFound)
			return
		}
		// Enabled defaults to true when the field is omitted in JSON.
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		row, err := n.Schedules.Create(r.Context(), &schedule.Schedule{
			EntityKind: req.EntityKind,
			EntityID:   id,
			RunAt:      req.RunAt,
			CronExpr:   req.CronExpr,
			Timezone:   req.Timezone,
			MaxRuns:    req.MaxRuns,
			StopAt:     req.StopAt,
			Enabled:    enabled,
			Metadata:   req.Metadata,
			CreatedBy:  req.CreatedBy,
			Reason:     req.Reason,
		})
		if err != nil {
			code, status := scheduleErrorStatus(err)
			writeScheduleError(w, code, err.Error(), status)
			return
		}
		// Sync the in-memory cron with the DB so the new row fires on
		// the next tick. Best-effort: log + continue if the runner is
		// not yet wired (test harness path).
		if n.ScheduleRunner != nil {
			if rerr := n.ScheduleRunner.Reload(r.Context()); rerr != nil {
				slog.Warn("schedule.Runner: reload after create failed",
					"schedule_id", row.ID, "error", rerr)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(row)
	}
}

// apiScheduleClose handles DELETE /api/schedules/{id}?reason=...
//
// Closes the row by setting valid_to=now() — never deletes. Idempotent:
// closing an already-closed row returns 200 with the same payload.
func apiScheduleClose(n *node.Node) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := mux.Vars(r)["id"]
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeScheduleError(w, "invalid_id",
				"schedule id must be a positive integer",
				http.StatusBadRequest)
			return
		}
		reason := r.URL.Query().Get("reason")
		by := r.URL.Query().Get("by")
		if by == "" {
			by = "operator"
		}
		if err := n.Schedules.Close(r.Context(), id, reason, by); err != nil {
			writeScheduleError(w, "internal", err.Error(), http.StatusInternalServerError)
			return
		}
		if n.ScheduleRunner != nil {
			if rerr := n.ScheduleRunner.Reload(r.Context()); rerr != nil {
				slog.Warn("schedule.Runner: reload after close failed",
					"schedule_id", id, "error", rerr)
			}
		}
		writeJSON(w, map[string]any{"ok": true, "id": id})
	}
}

// apiScheduleTemplates serves the recurrence preset list. Static —
// rebuilds at server boot from the package-level scheduleTemplates
// slice.
func apiScheduleTemplates() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"templates": scheduleTemplates})
	}
}

// registerScheduleRoutes wires the four endpoints into the /api router.
// Order: /templates BEFORE /{id} catch-all (gorilla/mux matches in
// registration order; "templates" would otherwise be parsed as an int).
func registerScheduleRoutes(api *mux.Router, n *node.Node) {
	api.HandleFunc("/schedules", apiSchedulesList(n)).Methods("GET")
	api.HandleFunc("/schedules", apiSchedulesCreate(n)).Methods("POST")
	api.HandleFunc("/schedules/templates", apiScheduleTemplates()).Methods("GET")
	api.HandleFunc("/schedules/{id}", apiScheduleClose(n)).Methods("DELETE")
}
