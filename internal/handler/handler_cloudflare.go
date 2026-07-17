package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/cloudflare"
	"github.com/viogus/oci-helper-go/internal/db"
)

// --- cloudflare ---

func (s *Server) handleCloudflare(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cloudflare/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")

	// Resolve token: use cfg_id query param or default config
	token := s.resolveCFToken(r)
	if token == "" {
		// Try the default cloudflare_token config
		var cfErr error
		token, cfErr = s.store.GetConfig("cloudflare_token")
		if cfErr != nil {
			log.Printf("[cloudflare] GetConfig cloudflare_token error: %v", cfErr)
		}
		if token == "" {
			jsonErr(w, "cloudflare not configured — set token or create a CF config")
			return
		}
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

func (s *Server) resolveCFToken(r *http.Request) string {
	cfgIDStr := r.URL.Query().Get("cfg_id")
	if cfgIDStr == "" {
		return ""
	}
	cfgID, err := strconv.ParseInt(cfgIDStr, 10, 64)
	if err != nil || cfgID <= 0 {
		return ""
	}
	cfg, err := s.store.GetCfCfg(int64(cfgID))
	if err != nil || cfg == nil || cfg.Token == "" {
		return ""
	}
	return cfg.Token
}

// ── CfCfg CRUD ────────────────────────────────────────────────────────

func (s *Server) handleCloudflareCfgs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListCfCfgs()
		if err != nil {
			jsonErr(w, "list cf configs: "+err.Error())
			return
		}
		if list == nil {
			list = []db.CfCfg{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	case http.MethodPost:
		var cfg db.CfCfg
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if cfg.Name == "" {
			jsonErr(w, "name required")
			return
		}
		if err := s.store.CreateCfCfg(&cfg); err != nil {
			jsonErr(w, "create cf config: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:cfg:create", cfg.Name, r)
		jsonOK(w, cfg)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCloudflareCfgByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/cloudflare/cfgs/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := parseInt64(idStr)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid config id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var cfg db.CfCfg
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		cfg.ID = id
		if err := s.store.UpdateCfCfg(&cfg); err != nil {
			jsonErr(w, "update cf config: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:cfg:update", cfg.Name, r)
		jsonOK(w, cfg)

	case http.MethodDelete:
		if err := s.store.DeleteCfCfg(id); err != nil {
			jsonErr(w, "delete cf config: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:cfg:delete", fmt.Sprintf("%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// ── OCI Auto-Sync DNS ─────────────────────────────────────────────────

func (s *Server) handleCloudflareOCISync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64    `json:"tenant_id"`
		ZoneID     string   `json:"zone_id"`
		Domain     string   `json:"domain"`
		Action     string   `json:"action"` // add, remove, update
		CfgID      int64    `json:"cfg_id"`
		InstanceIDs []string `json:"instance_ids"` // optional: specific instances
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.ZoneID == "" {
		jsonErr(w, "tenant_id and zone_id required")
		return
	}
	if req.Action != "add" && req.Action != "remove" && req.Action != "update" {
		jsonErr(w, "action must be add, remove, or update")
		return
	}

	// Resolve token
	var token string
	if req.CfgID > 0 {
		cfg, err := s.store.GetCfCfg(req.CfgID)
		if err != nil || cfg == nil {
			jsonErr(w, "cf config not found")
			return
		}
		token = cfg.Token
	} else {
		token, _ = s.store.GetConfig("cloudflare_token")
	}
	if token == "" {
		jsonErr(w, "cloudflare token not available")
		return
	}

	cf := cloudflare.New(token)
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	// Get instances
	var instances []db.Instance
	if len(req.InstanceIDs) > 0 {
		for _, id := range req.InstanceIDs {
			// Support both bare OCIDs and composite IDs (tenantID:ocid).
			lookupID := id
			if !strings.Contains(id, ":") {
				lookupID = fmt.Sprintf("%d:%s", req.TenantID, id)
			}
			inst, _ := s.store.GetInstanceByID(lookupID)
			if inst != nil {
				instances = append(instances, *inst)
			}
		}
	} else {
		list, err := s.store.ListInstances(req.TenantID)
		if err != nil {
			jsonErr(w, "list instances: "+err.Error())
			return
		}
		instances = list
	}

	results := make([]map[string]interface{}, 0, len(instances))
	for _, inst := range instances {
		if inst.PublicIP == "" {
			continue
		}
		dnsName := inst.Name
		if req.Domain != "" {
			dnsName = inst.Name + "." + req.Domain
		}

		switch req.Action {
		case "add":
			// Check existing records to avoid duplicates
			existing, _ := cf.ListDNSRecords(req.ZoneID)
			dup := false
			for _, r := range existing {
				if strings.EqualFold(strings.TrimRight(r.Name, "."), strings.TrimRight(dnsName, ".")) {
					results = append(results, map[string]interface{}{
						"instance": inst.Name,
						"ip":       inst.PublicIP,
						"dns":      dnsName,
						"action":   "skip",
						"reason":   "record already exists",
					})
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			_, err := cf.CreateDNSRecord(req.ZoneID, cloudflare.DNSRecord{
				Type:    "A",
				Name:    dnsName,
				Content: inst.PublicIP,
				TTL:     120,
			})
			results = append(results, map[string]interface{}{
				"instance": inst.Name,
				"ip":       inst.PublicIP,
				"dns":      dnsName,
				"action":   "add",
				"error":    errStr(err),
			})

		case "remove":
			records, err := cf.ListDNSRecords(req.ZoneID)
			if err != nil {
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"error":    err.Error(),
				})
				continue
			}
			found := false
			for _, rec := range records {
				if strings.EqualFold(strings.TrimRight(rec.Name, "."), strings.TrimRight(dnsName, ".")) {
					err := cf.DeleteDNSRecord(req.ZoneID, rec.ID)
					results = append(results, map[string]interface{}{
						"instance": inst.Name,
						"ip":       inst.PublicIP,
						"dns":      rec.Name,
						"action":   "remove",
						"error":    errStr(err),
					})
					found = true
					break // only delete first matching record
				}
			}
			if !found {
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"ip":       inst.PublicIP,
					"dns":      dnsName,
					"action":   "skip",
					"reason":   "no matching record found",
				})
			}

		case "update":
			if err := cf.UpdateDNSRecordIP(req.ZoneID, dnsName, inst.PublicIP); err != nil {
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"ip":       inst.PublicIP,
					"dns":      dnsName,
					"error":    err.Error(),
				})
			} else {
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"ip":       inst.PublicIP,
					"dns":      dnsName,
					"action":   "update",
				})
			}
		}
	}

	s.audit(req.TenantID, "cloudflare:oci-sync", fmt.Sprintf("%s %d records", req.Action, len(results)), r)
	jsonOK(w, map[string]interface{}{"results": results})
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func errStr(err error) string {
	if err == nil { return "" }
	return err.Error()
}
