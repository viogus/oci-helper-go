package handler

import (
	"net/http"

	"github.com/viogus/oci-helper-go/internal/system"
)

// handleSystemMetrics returns host-level resource metrics (CPU, memory, disk,
// network rates). Equivalent of the Java original's /metrics dashboard data,
// served over plain HTTP instead of a WebSocket — the frontend polls it.
func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, err := system.Collect()
	if err != nil {
		jsonErr(w, "collect system metrics: "+err.Error())
		return
	}
	jsonOK(w, snap)
}
