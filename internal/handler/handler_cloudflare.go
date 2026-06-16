package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/viogus/oci-helper-go/internal/cloudflare"
)

// --- cloudflare ---

func (s *Server) handleCloudflare(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cloudflare/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")

	token, _ := s.store.GetConfig("cloudflare_token")
	if token == "" {
		jsonErr(w, "cloudflare_token not configured")
		return
	}
	cf := cloudflare.New(token)

	switch {
	case path == "zones" && r.Method == http.MethodGet:
		zones, err := cf.ListZones()
		if err != nil {
			jsonErr(w, "list zones: "+err.Error())
			return
		}
		jsonOK(w, zones)

	case len(parts) == 2 && parts[1] == "records" && r.Method == http.MethodGet:
		zoneID := parts[0]
		records, err := cf.ListDNSRecords(zoneID)
		if err != nil {
			jsonErr(w, "list records: "+err.Error())
			return
		}
		jsonOK(w, records)

	case len(parts) == 2 && parts[1] == "records" && r.Method == http.MethodPost:
		zoneID := parts[0]
		var record cloudflare.DNSRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		created, err := cf.CreateDNSRecord(zoneID, record)
		if err != nil {
			jsonErr(w, "create record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:create", record.Name, r)
		jsonOK(w, created)

	case len(parts) == 3 && parts[1] == "records" && r.Method == http.MethodPut:
		zoneID, recordID := parts[0], parts[2]
		var record cloudflare.DNSRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		updated, err := cf.UpdateDNSRecord(zoneID, recordID, record)
		if err != nil {
			jsonErr(w, "update record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:update", record.Name, r)
		jsonOK(w, updated)

	case len(parts) == 3 && parts[1] == "records" && r.Method == http.MethodDelete:
		zoneID, recordID := parts[0], parts[2]
		if err := cf.DeleteDNSRecord(zoneID, recordID); err != nil {
			jsonErr(w, "delete record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:delete", recordID, r)
		jsonOK(w, map[string]string{"status": "ok"})

	case path == "update-ip" && r.Method == http.MethodPost:
		var req struct {
			ZoneID string `json:"zoneId"`
			Name   string `json:"name"`
			NewIP  string `json:"newIp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if err := cf.UpdateDNSRecordIP(req.ZoneID, req.Name, req.NewIP); err != nil {
			jsonErr(w, "update ip: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:ip:update", req.Name+" → "+maskIP(req.NewIP), r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		jsonErr(w, "unknown cloudflare endpoint")
	}
}
