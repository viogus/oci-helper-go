package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

type parsedOciCfg struct {
	Name        string
	UserOCID    string
	TenancyOCID string
	Fingerprint string
	Region      string
	KeyFile     string
}

// parseOciConfigText parses one or more [section] OCI config blocks. Section
// headers become the panel tenant name, matching the Java original.
func parseOciConfigText(content string) []parsedOciCfg {
	var out []parsedOciCfg
	var cur *parsedOciCfg
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if cur != nil {
				out = append(out, *cur)
			}
			name := strings.TrimSpace(line[1 : len(line)-1])
			cur = &parsedOciCfg{Name: name}
			continue
		}
		if cur == nil || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "user":
			cur.UserOCID = value
		case "fingerprint":
			cur.Fingerprint = value
		case "tenancy":
			cur.TenancyOCID = value
		case "region":
			cur.Region = value
		case "key_file":
			cur.KeyFile = value
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// handleTenantUploadBatch imports .ini/.txt OCI config files in bulk. Key
// files may be uploaded alongside them or must already exist in OCI_KEYS_DIR.
func (s *Server) handleTenantUploadBatch(w http.ResponseWriter, r *http.Request, files []*multipart.FileHeader) {
	keyFiles := map[string][]byte{}
	if r.MultipartForm != nil {
		for _, h := range r.MultipartForm.File["key_file"] {
			f, err := h.Open()
			if err != nil {
				continue
			}
			buf := make([]byte, h.Size)
			if _, err := io.ReadFull(f, buf); err != nil {
				f.Close()
				log.Printf("[uploadcfg] short read on %s: %v", h.Filename, err)
				continue
			}
			f.Close()
			keyFiles[filepath.Base(h.Filename)] = buf
		}
	}

	type itemResult struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}
	var results []itemResult
	seen := map[string]bool{}
	for _, h := range files {
		f, err := h.Open()
		if err != nil {
			results = append(results, itemResult{Name: h.Filename, Status: "failed", Error: err.Error()})
			continue
		}
		buf := make([]byte, h.Size)
		if _, err := io.ReadFull(f, buf); err != nil {
			f.Close()
			results = append(results, itemResult{Name: h.Filename, Status: "failed", Error: "read file: " + err.Error()})
			continue
		}
		f.Close()
		for _, cfg := range parseOciConfigText(string(buf)) {
			if cfg.Name == "" || cfg.UserOCID == "" || cfg.TenancyOCID == "" || cfg.Fingerprint == "" || cfg.Region == "" || cfg.KeyFile == "" {
				results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: "配置字段不完整"})
				continue
			}
			if seen[cfg.Name] {
				results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: "配置名称重复"})
				continue
			}
			seen[cfg.Name] = true
			keyName := filepath.Base(cfg.KeyFile)
			keyPath := filepath.Join(s.cfg.KeysDir, keyName)
			if content, ok := keyFiles[keyName]; ok {
				if err := os.MkdirAll(s.cfg.KeysDir, 0700); err != nil {
					results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: err.Error()})
					continue
				}
				if err := os.WriteFile(keyPath, content, 0600); err != nil {
					results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: err.Error()})
					continue
				}
			}
			tenant := &db.Tenant{
				Name:        cfg.Name,
				UserOCID:    cfg.UserOCID,
				TenancyOCID: cfg.TenancyOCID,
				Fingerprint: cfg.Fingerprint,
				Region:      cfg.Region,
				KeyFile:     keyName,
			}
			client, err := ociclient.NewClient(tenant, "")
			if err == nil {
				err = client.ValidateCredentials(r.Context(), cfg.TenancyOCID)
			}
			if err != nil {
				results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: err.Error()})
				continue
			}
			if err := s.store.CreateTenant(tenant); err != nil {
				results = append(results, itemResult{Name: cfg.Name, Status: "failed", Error: err.Error()})
				continue
			}
			go func(t *db.Tenant) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				client, err := s.clientFor(t)
				if err != nil {
					return
				}
				if err := s.store.SetTenantAccountStatus(t.ID, "ACTIVE"); err != nil {
					log.Printf("[uploadcfg] SetTenantAccountStatus: %v", err)
				}
				if sub, err := client.GetSubscriptionInfo(ctx); err == nil && sub != nil && sub.PlanType != "" {
					_ = s.store.SetConfig(fmt.Sprintf("tenant_plan_type_%d", t.ID), string(sub.PlanType))
				}
			}(tenant)
			s.audit(tenant.ID, "tenant:upload-batch", cfg.Name, r)
			results = append(results, itemResult{Name: cfg.Name, Status: "success"})
		}
	}
	success, failed := 0, 0
	for _, res := range results {
		if res.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	jsonOK(w, map[string]interface{}{"success": success, "failed": failed, "results": results})
}
