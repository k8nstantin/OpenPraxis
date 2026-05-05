package web

import (
	"encoding/json"
	"net/http"

	"github.com/k8nstantin/OpenPraxis/internal/task"
)

// writeJSON sends a JSON response with 200 OK.
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// writeError sends a JSON error response with the given status code.
func writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// decodeBody reads and JSON-decodes the request body into v.
// Returns false and writes a 400 error response if decoding fails.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// apiHostStats returns a live snapshot of host CPU/RSS metrics.
// Feeds the node stats chip on the overview dashboard.
func apiHostStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=5")
		sample, err := task.ReadHostMetrics()
		if err != nil {
			writeError(w, err.Error(), 500)
			return
		}
		writeJSON(w, sample)
	}
}
