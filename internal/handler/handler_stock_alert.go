package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/telegram"
)

// ── REST: List / Create ─────────────────────────────────────────────────

func (s *Server) handleStockAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		alerts, err := s.store.ListStockAlerts(tenantID)
		if err != nil {
			jsonErr(w, "list stock alerts: "+err.Error())
			return
		}
		jsonOK(w, alerts)

	case http.MethodPost:
		var a db.StockAlert
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if a.TenantID == 0 || a.Region == "" || a.Shape == "" {
			jsonErr(w, "tenantId, region, and shape are required")
			return
		}
		if err := s.store.CreateStockAlert(&a); err != nil {
			jsonErr(w, "create stock alert: "+err.Error())
			return
		}
		s.audit(a.TenantID, "stock-alert:create",
			fmt.Sprintf("region=%s shape=%s ad=%s", a.Region, a.Shape, a.AvailabilityDomain), r)
		jsonOK(w, a)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// ── REST: Get / Update / Delete by ID ───────────────────────────────────

func (s *Server) handleStockAlertByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/stock-alerts/")
	idStr = strings.TrimSuffix(idStr, "/")

	// Check if the path suffix is "check" (manual trigger).
	if strings.HasSuffix(idStr, "/check") {
		idStr = strings.TrimSuffix(idStr, "/check")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			jsonErr(w, "invalid id")
			return
		}
		s.handleStockAlertCheck(w, r, id)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonErr(w, "invalid id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		a, err := s.store.GetStockAlertByID(id)
		if err != nil {
			jsonErr(w, "get stock alert: "+err.Error())
			return
		}
		if a == nil {
			jsonErr(w, "stock alert not found")
			return
		}
		jsonOK(w, a)

	case http.MethodPut:
		var a db.StockAlert
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		a.ID = id
		if err := s.store.UpdateStockAlert(&a); err != nil {
			jsonErr(w, "update stock alert: "+err.Error())
			return
		}
		s.audit(a.TenantID, "stock-alert:update",
			fmt.Sprintf("id=%d enabled=%v", id, a.Enabled), r)
		jsonOK(w, a)

	case http.MethodDelete:
		if err := s.store.DeleteStockAlert(id); err != nil {
			jsonErr(w, "delete stock alert: "+err.Error())
			return
		}
		s.audit(0, "stock-alert:delete", fmt.Sprintf("id=%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// ── Manual check trigger ────────────────────────────────────────────────

func (s *Server) handleStockAlertCheck(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	a, err := s.store.GetStockAlertByID(id)
	if err != nil || a == nil {
		jsonErr(w, "stock alert not found")
		return
	}

	status, err := s.checkOneStockAlert(a)
	if err != nil {
		jsonErr(w, "check stock: "+err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": status, "shape": a.Shape, "region": a.Region})
}

// ── Background Monitor ──────────────────────────────────────────────────

// startStockMonitor runs in a goroutine. Every 60 seconds it iterates all
// enabled stock alerts, checks OCI stock availability for each, and sends a
// Telegram notification when the status changes.
func (s *Server) startStockMonitor() {
	log.Printf("[stock-monitor] started")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Cache Telegram token — rarely changes, avoid DB read per notification.
	tgToken, _ := s.store.GetConfig("telegram_token")

	for {
		select {
		case <-s.stopping:
			log.Printf("[stock-monitor] stopped")
			return
		case <-ticker.C:
			// Refresh token once per cycle in case config was updated.
			if token, err := s.store.GetConfig("telegram_token"); err == nil {
				tgToken = token
			}
			s.runStockCheck(tgToken)
		}
	}
}

func (s *Server) runStockCheck(tgToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()

	alerts, err := s.store.ListEnabledStockAlerts()
	if err != nil {
		log.Printf("[stock-monitor] list alerts: %v", err)
		return
	}
	if len(alerts) == 0 {
		return
	}
	log.Printf("[stock-monitor] checking %d alerts", len(alerts))

	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup

loop:
	for _, a := range alerts {
		select {
		case <-ctx.Done():
			break loop
		default:
		}
		wg.Add(1)
		go func(alert db.StockAlert) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			status, err := s.checkOneStockAlert(&alert)
			if err != nil {
				log.Printf("[stock-monitor] alert %d (%s/%s): %v", alert.ID, alert.Region, alert.Shape, err)
				return
			}

			if status != alert.LastStockStatus {
				log.Printf("[stock-monitor] alert %d: %s/%s changed from %q to %q",
					alert.ID, alert.Region, alert.Shape, alert.LastStockStatus, status)
				s.store.UpdateStockAlertStatus(alert.ID, status)

				if alert.ChatID != 0 && tgToken != "" {
					msg := buildStockAlertMessage(alert.Region, alert.Shape, alert.AvailabilityDomain, status)
					bot := telegram.New(tgToken)
					if err := bot.SendMessage(alert.ChatID, msg); err != nil {
						log.Printf("[stock-monitor] telegram send: %v", err)
					}
				}
			} else {
				s.store.UpdateStockAlertStatus(alert.ID, status)
			}
		}(a)
	}
	wg.Wait()
}

// checkOneStockAlert creates an OCI client for the tenant and checks stock.
func (s *Server) checkOneStockAlert(a *db.StockAlert) (string, error) {
	tenant, err := s.store.GetTenant(a.TenantID)
	if err != nil || tenant == nil {
		return "unknown", fmt.Errorf("tenant %d not found", a.TenantID)
	}

	client, err := s.clientFor(tenant)
	if err != nil {
		return "unknown", fmt.Errorf("oci client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return client.CheckInstanceStock(ctx, a.Region, a.Shape, a.AvailabilityDomain)
}

// buildStockAlertMessage formats a Telegram notification for a stock status change.
func buildStockAlertMessage(region, shape, ad, status string) string {
	emoji := "⚠️"  // warning
	switch status {
	case "available":
		emoji = "✅" // white check mark
	case "out_of_stock":
		emoji = "❌" // cross mark
	}

	adText := ""
	if ad != "" {
		adText = fmt.Sprintf(" (AD: %s)", ad)
	}

	label := status
	switch status {
	case "available":
		label = "AVAILABLE"
	case "out_of_stock":
		label = "OUT OF STOCK"
	}

	return fmt.Sprintf("%s Stock Alert: %s/%s%s is now **%s**", emoji, region, shape, adText, label)
}