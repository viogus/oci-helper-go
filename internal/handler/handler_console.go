package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"strconv"
	"time"
)

// handleStopVNC deletes the OCI Instance Console Connection identified by
// console_id, or all console connections for the given instance.
//
// Request body: { tenant_id, instance_id, console_id }
func (s *Server) handleStopVNC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		ConsoleID  string `json:"console_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}

	// If a specific console_id is given, delete it
	if req.ConsoleID != "" {
		if err := client.DeleteConsoleConnection(r.Context(), req.ConsoleID); err != nil {
			jsonErr(w, "delete console connection: "+err.Error())
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
		return
	}

	// Otherwise delete all console connections for the instance
	instanceID := req.InstanceID
	if idx := strings.IndexByte(instanceID, ':'); idx >= 0 {
		instanceID = instanceID[idx+1:]
	}
	conns, err := client.ListConsoleConnections(r.Context(), instanceID)
	if err != nil {
		jsonErr(w, "list console connections: "+err.Error())
		return
	}
	deleted := 0
	for _, c := range conns {
		if c.Id != nil {
			if delErr := client.DeleteConsoleConnection(r.Context(), *c.Id); delErr != nil {
				log.Printf("[VNC] delete console %s: %v", *c.Id, delErr)
				continue
			}
			deleted++
		}
	}

	s.audit(req.TenantID, "console:stop", "instance="+req.InstanceID+" deleted="+strconv.Itoa(deleted), r)
	jsonOK(w, map[string]interface{}{
		"status":  "deleted",
		"deleted": deleted,
	})
}

// handleConsoleWait polls an existing console connection until active.
// Used by the frontend after handleStartVNC returns status "creating".
//
// GET /api/instances/vnc/wait?console_id=<ocid>&tenant_id=<id>
func (s *Server) handleConsoleWait(w http.ResponseWriter, r *http.Request) {
	consoleID := r.URL.Query().Get("console_id")
	if consoleID == "" {
		jsonErr(w, "console_id query param required")
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	if tenantID == 0 {
		jsonErr(w, "tenant_id query param required")
		return
	}

	client, _, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := client.GetConsoleConnection(r.Context(), consoleID)
		if err != nil {
			jsonErr(w, "get console: "+err.Error())
			return
		}
		state := ""
		if conn.LifecycleState != "" {
			state = string(conn.LifecycleState)
		}
		connStr := ""
		if conn.ConnectionString != nil {
			connStr = *conn.ConnectionString
		}
		vncStr := ""
		if conn.VncConnectionString != nil {
			vncStr = *conn.VncConnectionString
		}
		fp := ""
		if conn.Fingerprint != nil {
			fp = *conn.Fingerprint
		}

		if state == "ACTIVE" {
			jsonOK(w, map[string]interface{}{
				"status":                "active",
				"connection_string":     connStr,
				"vnc_connection_string": vncStr,
				"vnc_url":               vncStr,
				"connection_id":         consoleID,
				"fingerprint":           fp,
			})
			return
		}
		if state == "FAILED" || state == "DELETED" {
			jsonOK(w, map[string]string{"status": "failed"})
			return
		}

		select {
		case <-r.Context().Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
	jsonOK(w, map[string]string{"status": "pending"})
}
