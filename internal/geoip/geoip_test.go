package geoip

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// pointProviders swaps the provider endpoints for the duration of a test.
func pointProviders(t *testing.T, whoisURL, apiURL string) {
	t.Helper()
	oldWhois, oldAPI := ipwhoisBase, ipapiBase
	ipwhoisBase, ipapiBase = whoisURL, apiURL
	t.Cleanup(func() {
		ipwhoisBase, ipapiBase = oldWhois, oldAPI
	})
}

func TestLookup_PrivateIPNeedsNoNetwork(t *testing.T) {
	for _, ip := range []string{"10.0.0.5", "192.168.1.10", "127.0.0.1", "::1"} {
		info, err := Lookup(ip)
		if err == nil || info != nil {
			t.Fatalf("Lookup(%q) = %v, %v; want nil, error", ip, info, err)
		}
	}
}

func TestLookup_IPWhois(t *testing.T) {
	whois := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":true,"latitude":35.68,"longitude":139.76,`+
			`"country":"Japan","region":"Tokyo","city":"Tokyo",`+
			`"connection":{"asn":2516,"org":"KDDI"}}`)
	}))
	defer whois.Close()
	// ip-api fallback deliberately unreachable: the primary must win.
	pointProviders(t, whois.URL+"/", "http://127.0.0.1:1/json/")

	info, err := Lookup("203.0.113.10")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Lat != 35.68 || info.Lng != 139.76 {
		t.Fatalf("lat/lng = %v/%v, want 35.68/139.76", info.Lat, info.Lng)
	}
	if info.City != "Tokyo" || info.Area != "Tokyo" || info.Country != "Japan" {
		t.Fatalf("city/area/country = %q/%q/%q", info.City, info.Area, info.Country)
	}
	if info.Org != "KDDI" || info.Asn != "AS2516" {
		t.Fatalf("org/asn = %q/%q, want KDDI/AS2516", info.Org, info.Asn)
	}
}

func TestLookup_FallsBackToIPAPI(t *testing.T) {
	whois := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":false,"message":"reserved range"}`)
	}))
	defer whois.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"status":"success","country":"Japan","regionName":"Tokyo",`+
			`"city":"Tokyo","lat":35.68,"lon":139.76,"org":"KDDI","as":"AS2516 KDDI"}`)
	}))
	defer api.Close()
	pointProviders(t, whois.URL+"/", api.URL+"/")

	info, err := Lookup("203.0.113.10")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Lat != 35.68 || info.Lng != 139.76 {
		t.Fatalf("lat/lng = %v/%v, want 35.68/139.76", info.Lat, info.Lng)
	}
	// Only the AS number is kept, not the org suffix.
	if info.Asn != "AS2516" {
		t.Fatalf("asn = %q, want AS2516", info.Asn)
	}
}

func TestLookup_AllProvidersFail(t *testing.T) {
	whois := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":false,"message":"reserved range"}`)
	}))
	defer whois.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"status":"fail"}`)
	}))
	defer api.Close()
	pointProviders(t, whois.URL+"/", api.URL+"/")

	if info, err := Lookup("203.0.113.10"); err == nil || info != nil {
		t.Fatalf("Lookup = %v, %v; want nil, error", info, err)
	}
}
