package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

// ── Instance DNS Binding CRUD ──────────────────────────────────────────

// handleDNSBindings lists or creates instance→DNS bindings.
// GET  /api/cloudflare/bindings           -> list all bindings (optionally ?instance_id=)
// POST /api/cloudflare/bindings           -> create a binding
func (s *Server) handleDNSBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		instanceID := r.URL.Query().Get("instance_id")
		list, err := s.store.ListInstanceDNSBindings(instanceID)
		if err != nil {
			jsonErr(w, "list dns bindings: "+err.Error())
			return
		}
		if list == nil {
			list = []db.InstanceDNSBinding{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	case http.MethodPost:
		var b db.InstanceDNSBinding
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if b.InstanceID == "" {
			jsonErr(w, "instance_id required")
			return
		}
		if b.Name == "" {
			jsonErr(w, "name (DNS record name) required")
			return
		}
		if !validDNSName(b.Name) {
			jsonErr(w, "invalid DNS record name")
			return
		}
		if b.ZoneID == "" {
			jsonErr(w, "zone_id required")
			return
		}
		if b.TTL == 0 {
			b.TTL = 120
		}
		if err := s.store.CreateInstanceDNSBinding(&b); err != nil {
			jsonErr(w, "create dns binding: "+err.Error())
			return
		}
		s.audit(b.TenantID, "cloudflare:binding:create", b.InstanceID+" -> "+b.Name, r)
		jsonOK(w, b)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleDNSBindingByID updates or deletes a single binding.
// PUT    /api/cloudflare/bindings/{id}
// DELETE /api/cloudflare/bindings/{id}
func (s *Server) handleDNSBindingByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/cloudflare/bindings/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := parseInt64(idStr)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid binding id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var b db.InstanceDNSBinding
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		cur, _ := s.store.GetInstanceDNSBinding(id)
		if cur == nil {
			jsonErr(w, "binding not found")
			return
		}
		b.ID = id
		if b.InstanceID == "" {
			b.InstanceID = cur.InstanceID
		}
		if b.TenantID == 0 {
			b.TenantID = cur.TenantID
		}
		if b.Name == "" {
			b.Name = cur.Name
		}
		if !validDNSName(b.Name) {
			jsonErr(w, "invalid DNS record name")
			return
		}
		if b.ZoneID == "" {
			b.ZoneID = cur.ZoneID
		}
		if b.TTL == 0 {
			b.TTL = cur.TTL
		}
		if err := s.store.UpdateInstanceDNSBinding(&b); err != nil {
			jsonErr(w, "update dns binding: "+err.Error())
			return
		}
		s.audit(b.TenantID, "cloudflare:binding:update", b.InstanceID+" -> "+b.Name, r)
		jsonOK(w, b)

	case http.MethodDelete:
		if err := s.store.DeleteInstanceDNSBinding(id); err != nil {
			jsonErr(w, "delete dns binding: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:binding:delete", fmt.Sprintf("%d", id), r)
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
		TenantID    int64    `json:"tenant_id"`
		ZoneID      string   `json:"zone_id"`
		Domain      string   `json:"domain"`
		Action      string   `json:"action"` // add, remove, update
		CfgID       int64    `json:"cfg_id"`
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

		// If explicit bindings exist for this instance, operate on them
		// instead of the implicit name+domain convention.
		bindings, bErr := s.instanceBindings(inst.ID)
		if bErr == nil && len(bindings) > 0 {
			for _, b := range bindings {
				if req.Action == "remove" {
					bindingCF, bZone, bErr := s.cloudflareClientFor(b.CfCfgID, b.ZoneID)
					if bErr != nil {
						results = append(results, map[string]interface{}{
							"instance": inst.Name, "dns": b.Name, "action": "error", "error": bErr.Error(),
						})
						continue
					}
					recs, err := bindingCF.ListDNSRecords(bZone)
					if err != nil {
						results = append(results, map[string]interface{}{
							"instance": inst.Name, "dns": b.Name, "action": "error", "error": err.Error(),
						})
						continue
					}
					found := false
					for _, rec := range recs {
						if strings.EqualFold(strings.TrimRight(rec.Name, "."), strings.TrimRight(b.Name, ".")) && rec.Type == "A" {
							if err := bindingCF.DeleteDNSRecord(bZone, rec.ID); err != nil {
								results = append(results, map[string]interface{}{
									"instance": inst.Name, "dns": b.Name, "action": "error", "error": err.Error(),
								})
							} else {
								results = append(results, map[string]interface{}{
									"instance": inst.Name, "dns": b.Name, "action": "remove", "ip": inst.PublicIP,
								})
							}
							found = true
							break
						}
					}
					if !found {
						results = append(results, map[string]interface{}{
							"instance": inst.Name, "dns": b.Name, "action": "skip", "reason": "no matching record found",
						})
					}
					continue
				}
				entry := s.syncBindingToDNS(inst, b)
				if req.Action == "update" && entry["action"] == "create" {
					entry["action"] = "update"
				}
				results = append(results, entry)
			}
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

// validDNSName does a light sanity check on a DNS record name: it must be
// non-empty and contain no spaces or wildcards. The apex name "@" is allowed.
func validDNSName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '*' {
			return false
		}
	}
	return true
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// updateCfDNSAfterChangeIP mirrors the Java change-IP flow's optional
// Cloudflare DNS update. If the instance has one or more explicit DNS
// bindings, it updates each of them to point at the new public IP. Otherwise
// it falls back to the legacy prefix.domain convention using the given cfgID.
func (s *Server) updateCfDNSAfterChangeIP(tenantID, cfgID int64, instanceID, prefix, newIP string, proxied *bool, ttl int, remark string) error {
	// Prefer explicit per-instance bindings, if any.
	if instanceID != "" {
		bindings, _ := s.instanceBindings(instanceID)
		if len(bindings) > 0 {
			var lastErr error
			for _, b := range bindings {
				inst := db.Instance{ID: instanceID, Name: prefix, PublicIP: newIP}
				entry := s.syncBindingToDNS(inst, b)
				if entry["error"] != nil {
					lastErr = fmt.Errorf("update %s -> %s: %v", instanceID, b.Name, entry["error"])
					log.Printf("[change-ip] binding %s -> %s: %v", instanceID, b.Name, entry["error"])
				}
			}
			if lastErr != nil {
				return lastErr
			}
			return nil
		}
	}

	cfg, err := s.store.GetCfCfg(cfgID)
	if err != nil || cfg == nil || cfg.Token == "" {
		return fmt.Errorf("cloudflare config not found")
	}
	if prefix == "" || cfg.ZoneID == "" {
		return fmt.Errorf("domain prefix and zone required")
	}
	dnsName := prefix + "." + cfg.Name
	cf := cloudflare.New(cfg.Token)
	records, err := cf.ListDNSRecords(cfg.ZoneID)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if strings.EqualFold(strings.TrimRight(rec.Name, "."), strings.TrimRight(dnsName, ".")) {
			if err := cf.DeleteDNSRecord(cfg.ZoneID, rec.ID); err != nil {
				return err
			}
			break
		}
	}
	record := cloudflare.DNSRecord{
		Type:    "A",
		Name:    dnsName,
		Content: newIP,
		Proxied: proxied,
		TTL:     ttl,
		Comment: remark,
	}
	if record.TTL == 0 {
		record.TTL = 120
	}
	_, err = cf.CreateDNSRecord(cfg.ZoneID, record)
	return err
}

// ── DNS Auto-Sync Monitor ───────────────────────────────────────────────

// cloudflareClientFor resolves a Cloudflare client for a binding/config ID.
// If cfgID > 0 it looks up a named CfCfg; otherwise it falls back to the
// global cloudflare_token config. Returns a nil client and the zone ID only
// when a token is available.
func (s *Server) cloudflareClientFor(cfgID int64, fallbackZone string) (*cloudflare.Client, string, error) {
	var token string
	cfg, err := s.store.GetCfCfg(cfgID)
	if err == nil && cfg != nil && cfg.Token != "" {
		token = cfg.Token
	}
	if token == "" {
		token, _ = s.store.GetConfig("cloudflare_token")
	}
	if token == "" {
		return nil, "", fmt.Errorf("no cloudflare token available")
	}
	// The caller's explicit zone (from a binding) takes precedence over the
	// named config's own zone, so bindings can target a different zone than
	// the config's default.
	zone := fallbackZone
	if zone == "" && cfg != nil {
		zone = cfg.ZoneID
	}
	return cloudflare.New(token), zone, nil
}

// instanceBindings returns bindings for an instance (only enabled ones),
// paged directly from the store.
func (s *Server) instanceBindings(instanceID string) ([]db.InstanceDNSBinding, error) {
	list, err := s.store.ListInstanceDNSBindings(instanceID)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, b := range list {
		if b.Enabled && b.Name != "" {
			out = append(out, b)
		}
	}
	return out, nil
}

// dnsAutoSyncState holds runtime state for the background DNS auto-sync monitor.
type dnsAutoSyncState struct {
	mu          sync.Mutex
	lastRun     time.Time
	lastResults []map[string]interface{}
	running     bool
}

// startDNSAutoSync runs the background DNS auto-sync monitor in a goroutine.
// It polls every 60 seconds, checks dns_auto_sync_enabled config, and updates
// Cloudflare DNS records when public IPs change.
func (s *Server) startDNSAutoSync() {
	log.Println("[dns-auto-sync] monitor started")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopping:
			log.Println("[dns-auto-sync] monitor stopped")
			return
		case <-ticker.C:
			s.runDNSAutoSync()
		case <-s.dnsAutoSyncTrigger:
			s.runDNSAutoSync()
		}
	}
}

// runDNSAutoSync performs one cycle of the auto-sync: reads config, fetches
// all instances with public IPs, and creates or updates Cloudflare DNS records
// when an IP has changed since the last sync.
func (s *Server) runDNSAutoSync() {
	s.dnsSyncState.mu.Lock()
	if s.dnsSyncState.running {
		s.dnsSyncState.mu.Unlock()
		return // prevent overlapping runs
	}
	s.dnsSyncState.running = true
	s.dnsSyncState.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[dns-auto-sync] panic: %v", r)
		}
		s.dnsSyncState.mu.Lock()
		s.dnsSyncState.running = false
		s.dnsSyncState.lastRun = time.Now()
		s.dnsSyncState.mu.Unlock()
	}()

	enabled, _ := s.store.GetConfig("dns_auto_sync_enabled")
	if enabled != "true" {
		return
	}

	zoneID, _ := s.store.GetConfig("dns_auto_sync_zone_id")
	domain, _ := s.store.GetConfig("dns_auto_sync_domain")
	cfgIDStr, _ := s.store.GetConfig("dns_auto_sync_cfg_id")

	if zoneID == "" {
		log.Println("[dns-auto-sync] zone_id not configured, skipping")
		return
	}

	// Resolve Cloudflare token: prefer named CfCfg, fall back to global config.
	var token string
	if cfgID, err := strconv.ParseInt(cfgIDStr, 10, 64); err == nil && cfgID > 0 {
		cfg, cfgErr := s.store.GetCfCfg(cfgID)
		if cfgErr == nil && cfg != nil {
			token = cfg.Token
		}
	}
	if token == "" {
		token, _ = s.store.GetConfig("cloudflare_token")
	}
	if token == "" {
		log.Println("[dns-auto-sync] no Cloudflare token configured, skipping")
		return
	}

	cf := cloudflare.New(token)

	// Get ALL instances across all tenants that have public IPs.
	instances, err := s.store.ListInstances(0) // tenantID=0 means all
	if err != nil {
		log.Printf("[dns-auto-sync] list instances: %v", err)
		return
	}

	existingRecords, err := cf.ListDNSRecords(zoneID)
	if err != nil {
		log.Printf("[dns-auto-sync] list dns records: %v", err)
		return
	}

	var results []map[string]interface{}

	for _, inst := range instances {
		if inst.PublicIP == "" {
			continue
		}

		// Prefer explicit per-instance DNS bindings over the implicit
		// "name + global domain" convention. Bindings are always reconciled
		// (even if dns_last_ip is unchanged) because a binding may have been
		// added after the instance was last synced.
		bindings, bErr := s.instanceBindings(inst.ID)
		if bErr == nil && len(bindings) > 0 {
			for _, b := range bindings {
				entry := s.syncBindingToDNS(inst, b)
				results = append(results, entry)
				if entry["error"] != nil {
					log.Printf("[dns-auto-sync] binding %s -> %s: %v", inst.Name, b.Name, entry["error"])
				}
			}
			// Persist last known IP (bindings share the instance's dns_last_ip).
			if err := s.store.UpdateInstanceDNSIP(inst.ID, inst.PublicIP); err != nil {
				log.Printf("[dns-auto-sync] save last IP for %s: %v", inst.Name, err)
			}
			continue
		}

		// Implicit convention: fast path — nothing changed since last sync.
		if inst.DNSLastIP == inst.PublicIP {
			continue
		}

		// Build DNS name: instance name + optional domain suffix.
		dnsName := inst.Name
		if domain != "" {
			dnsName = inst.Name + "." + domain
		}

		lastIP := inst.DNSLastIP

		if lastIP == inst.PublicIP {
			continue // no change
		}

		// Normalize names for comparison (strip trailing dot, lowercase).
		normalize := func(s string) string {
			return strings.ToLower(strings.TrimRight(s, "."))
		}
		target := normalize(dnsName)

		// Check if a DNS record already exists for this instance.
		var existingRecord *cloudflare.DNSRecord
		for _, rec := range existingRecords {
			if normalize(rec.Name) == target {
				recCopy := rec
				existingRecord = &recCopy
				break
			}
		}

		if existingRecord != nil {
			// Update existing record.
			_, err := cf.UpdateDNSRecord(zoneID, existingRecord.ID, cloudflare.DNSRecord{
				Type:    "A",
				Name:    dnsName,
				Content: inst.PublicIP,
				TTL:     120,
			})
			if err != nil {
				log.Printf("[dns-auto-sync] update %s: %v", inst.Name, err)
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"dns":      dnsName,
					"ip":       inst.PublicIP,
					"action":   "update",
					"error":    err.Error(),
				})
				continue
			}
			log.Printf("[dns-auto-sync] %s: %s -> %s (updated)", inst.Name, lastIP, inst.PublicIP)
			results = append(results, map[string]interface{}{
				"instance": inst.Name,
				"dns":      dnsName,
				"ip":       inst.PublicIP,
				"action":   "update",
				"old_ip":   lastIP,
			})
		} else {
			// Create new record.
			_, err := cf.CreateDNSRecord(zoneID, cloudflare.DNSRecord{
				Type:    "A",
				Name:    dnsName,
				Content: inst.PublicIP,
				TTL:     120,
			})
			if err != nil {
				log.Printf("[dns-auto-sync] create %s: %v", inst.Name, err)
				results = append(results, map[string]interface{}{
					"instance": inst.Name,
					"dns":      dnsName,
					"ip":       inst.PublicIP,
					"action":   "create",
					"error":    err.Error(),
				})
				continue
			}
			log.Printf("[dns-auto-sync] %s: (new) %s (created)", inst.Name, inst.PublicIP)
			results = append(results, map[string]interface{}{
				"instance": inst.Name,
				"dns":      dnsName,
				"ip":       inst.PublicIP,
				"action":   "create",
			})
		}

		// Persist last known IP.
		if err := s.store.UpdateInstanceDNSIP(inst.ID, inst.PublicIP); err != nil {
			log.Printf("[dns-auto-sync] save last IP for %s: %v", inst.Name, err)
		}
	}

	// Save last sync time and results summary.
	s.setConfig("dns_auto_sync_last_time", time.Now().UTC().Format(time.RFC3339))
	if len(results) > 0 {
		summary, _ := json.Marshal(results)
		s.setConfig("dns_auto_sync_last_results", string(summary))
	}

	s.dnsSyncState.mu.Lock()
	s.dnsSyncState.lastResults = results
	s.dnsSyncState.mu.Unlock()
}

// syncBindingToDNS ensures the DNS record named by binding.Name (inside
// binding.ZoneID) points at inst.PublicIP. Uses the binding's own CfCfg (or
// falls back to the global token) and record settings. Returns a result entry
// describing the action taken.
func (s *Server) syncBindingToDNS(inst db.Instance, b db.InstanceDNSBinding) map[string]interface{} {
	entry := map[string]interface{}{
		"instance": inst.Name,
		"dns":      b.Name,
		"ip":       inst.PublicIP,
	}
	cf, zoneID, err := s.cloudflareClientFor(b.CfCfgID, b.ZoneID)
	if err != nil {
		entry["action"] = "error"
		entry["error"] = err.Error()
		return entry
	}
	if zoneID == "" {
		zoneID = b.ZoneID
	}

	// Find an existing record with the same name in this zone.
	records, err := cf.ListDNSRecords(zoneID)
	if err != nil {
		entry["action"] = "error"
		entry["error"] = "list records: " + err.Error()
		return entry
	}
	normalize := func(s string) string { return strings.ToLower(strings.TrimRight(s, ".")) }
	target := normalize(b.Name)
	ttl := b.TTL
	if ttl == 0 {
		ttl = 120
	}
	// Cloudflare requires TTL=1 (auto) when a record is proxied.
	if b.Proxied {
		ttl = 1
	}
	var existing *cloudflare.DNSRecord
	for i := range records {
		// Only match A records for the same name — an AAAA/CNAME/MX with the
		// same name must not be overwritten or deleted.
		if normalize(records[i].Name) == target && records[i].Type == "A" {
			existing = &records[i]
			break
		}
	}

	if existing != nil && existing.Content == inst.PublicIP {
		entry["action"] = "skip"
		entry["reason"] = "ip unchanged"
		return entry
	}

	if existing != nil {
		_, err = cf.UpdateDNSRecord(zoneID, existing.ID, cloudflare.DNSRecord{
			Type:    "A",
			Name:    b.Name,
			Content: inst.PublicIP,
			Proxied: &b.Proxied,
			TTL:     ttl,
		})
		entry["action"] = "update"
	} else {
		_, err = cf.CreateDNSRecord(zoneID, cloudflare.DNSRecord{
			Type:    "A",
			Name:    b.Name,
			Content: inst.PublicIP,
			Proxied: &b.Proxied,
			TTL:     ttl,
		})
		entry["action"] = "create"
	}
	if err != nil {
		entry["error"] = err.Error()
	}
	return entry
}

// ── DNS Auto-Sync API Handlers ──────────────────────────────────────────

// handleCloudflareAutoSyncStatus returns the current auto-sync configuration and state.
func (s *Server) handleCloudflareAutoSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	enabled, _ := s.store.GetConfig("dns_auto_sync_enabled")
	zoneID, _ := s.store.GetConfig("dns_auto_sync_zone_id")
	domain, _ := s.store.GetConfig("dns_auto_sync_domain")
	cfgIDStr, _ := s.store.GetConfig("dns_auto_sync_cfg_id")
	lastTime, _ := s.store.GetConfig("dns_auto_sync_last_time")
	lastResultsStr, _ := s.store.GetConfig("dns_auto_sync_last_results")

	var lastResults []map[string]interface{}
	if lastResultsStr != "" {
		_ = json.Unmarshal([]byte(lastResultsStr), &lastResults)
	}
	// Fill from in-memory state if config is empty.
	if len(lastResults) == 0 {
		s.dnsSyncState.mu.Lock()
		lastResults = s.dnsSyncState.lastResults
		s.dnsSyncState.mu.Unlock()
	}

	jsonOK(w, map[string]interface{}{
		"enabled":         enabled == "true",
		"zoneId":          zoneID,
		"domain":          domain,
		"cfgId":           cfgIDStr,
		"lastSync":        lastTime,
		"lastCount":       len(lastResults),
		"lastSyncResults": lastResults,
	})
}

// handleCloudflareAutoSyncTrigger manually triggers a DNS auto-sync cycle.
func (s *Server) handleCloudflareAutoSyncTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// Non-blocking send to trigger channel; if the monitor is busy, it will
	// pick up the next tick anyway.
	select {
	case s.dnsAutoSyncTrigger <- struct{}{}:
	default:
	}
	jsonOK(w, map[string]string{"status": "triggered"})
}
