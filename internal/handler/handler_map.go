package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/geoip"
)

// mapPoint is one marker on the dashboard world map: a single instance with
// best-effort coordinates for its public IP.
//
// Lat/Lng stay 0 when the IP cannot be geolocated (no public IP, private
// address, or a lookup that has not completed yet); the SPA then falls back to
// the tenant's region coordinates instead of hiding the instance.
type mapPoint struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	TenantID int64   `json:"tenantId"`
	State    string  `json:"state"`
	Region   string  `json:"region"`
	PublicIP string  `json:"publicIp"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Country  string  `json:"country"`
	Area     string  `json:"area"`
	City     string  `json:"city"`
	Org      string  `json:"org"`
	Asn      string  `json:"asn"`
}

const (
	// geoCacheTTL is how long a successful lookup is reused.
	geoCacheTTL = 24 * time.Hour
	// geoCacheNegTTL is how long a failed lookup is reused, so an unresolvable
	// IP is not retried on every dashboard load.
	geoCacheNegTTL = 30 * time.Minute
	// geoFillPerRequest caps lookups per request. ip-api.com's free endpoint
	// allows roughly 45 requests per minute and every dashboard load calls
	// this handler, so the map fills in over the first few loads instead of
	// hitting the rate limit (and instead of blocking the first paint).
	geoFillPerRequest = 8
	// geoFillBudget is the hard time budget for those lookups: whatever has
	// not answered by then is left for the next request (it still gets cached
	// when it arrives).
	geoFillBudget = 3 * time.Second
)

type geoEntry struct {
	info *geoip.Info
	at   time.Time
}

func (e geoEntry) fresh(now time.Time) bool {
	ttl := geoCacheTTL
	if e.info == nil {
		ttl = geoCacheNegTTL
	}
	return now.Sub(e.at) < ttl
}

// handleInstanceMap returns one point per instance across all tenants.
// GET /api/instances/map
func (s *Server) handleInstanceMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	instances, err := s.store.ListInstances(0)
	if err != nil {
		s.apiErr(w, r, "list instances: ", err)
		return
	}

	pending := s.fillGeoCache(instances)
	cache := s.snapshotGeoCache()

	points := make([]mapPoint, 0, len(instances))
	for _, inst := range instances {
		p := mapPoint{
			ID:       inst.ID,
			Name:     inst.Name,
			TenantID: inst.TenantID,
			State:    inst.State,
			Region:   inst.Region,
			PublicIP: inst.PublicIP,
		}
		if e, ok := cache[strings.TrimSpace(inst.PublicIP)]; ok && e.info != nil {
			p.Lat, p.Lng = e.info.Lat, e.info.Lng
			p.Country, p.Area, p.City = e.info.Country, e.info.Area, e.info.City
			p.Org, p.Asn = e.info.Org, e.info.Asn
		}
		points = append(points, p)
	}

	jsonOK(w, map[string]interface{}{
		"points":  points,
		// pending > 0 means some IPs are still unresolved; the SPA polls a few
		// times so the map completes without a manual refresh.
		"pending": pending,
	})
}

// fillGeoCache refreshes the cache for the instances' public IPs and returns
// how many distinct IPs are still unresolved afterwards.
func (s *Server) fillGeoCache(instances []db.Instance) int {
	now := time.Now()

	s.geoMu.Lock()
	if s.geoCache == nil {
		s.geoCache = make(map[string]geoEntry)
	}
	seen := make(map[string]struct{}, len(instances))
	var missing []string
	for _, inst := range instances {
		ip := strings.TrimSpace(inst.PublicIP)
		if ip == "" {
			continue
		}
		if _, dup := seen[ip]; dup {
			continue
		}
		seen[ip] = struct{}{}
		if e, ok := s.geoCache[ip]; ok && e.fresh(now) {
			continue
		}
		missing = append(missing, ip)
	}
	s.geoMu.Unlock()

	todo := missing
	if len(todo) > geoFillPerRequest {
		todo = todo[:geoFillPerRequest]
	}
	s.lookupGeo(todo)

	pending := 0
	now = time.Now()
	s.geoMu.Lock()
	for ip := range seen {
		if e, ok := s.geoCache[ip]; !ok || !e.fresh(now) {
			pending++
		}
	}
	s.geoMu.Unlock()
	return pending
}

// lookupGeo resolves the given IPs concurrently and caches whatever arrives
// within geoFillBudget. Results that land after the caller gave up are still
// cached, so a slow or blocked lookup is not retried on every request.
func (s *Server) lookupGeo(ips []string) {
	if len(ips) == 0 {
		return
	}
	done := make(chan struct{}, len(ips))
	for _, ip := range ips {
		go func(ip string) {
			info, err := geoip.Lookup(ip)
			if err != nil {
				info = nil
			}
			s.geoMu.Lock()
			s.geoCache[ip] = geoEntry{info: info, at: time.Now()}
			s.geoMu.Unlock()
			done <- struct{}{}
		}(ip)
	}

	deadline := time.After(geoFillBudget)
	for range ips {
		select {
		case <-done:
		case <-deadline:
			return
		}
	}
}

func (s *Server) snapshotGeoCache() map[string]geoEntry {
	s.geoMu.Lock()
	defer s.geoMu.Unlock()
	out := make(map[string]geoEntry, len(s.geoCache))
	for k, v := range s.geoCache {
		out[k] = v
	}
	return out
}
