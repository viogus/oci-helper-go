// Package geoip provides key-less IP geolocation lookups.
package geoip

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Info holds geolocation data for a single IP address.
type Info struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Country string  `json:"country"`
	Area    string  `json:"regionName"`
	City    string  `json:"city"`
	Org     string  `json:"org"`
	Asn     string  `json:"as"`
}

// Provider endpoints. Variables so tests can point them at a local server.
var (
	// ipwho.is: free, key-less and served over HTTPS, so it is the primary
	// source.
	ipwhoisBase = "https://ipwho.is/"
	// ip-api.com: the free plan is plain HTTP only — its HTTPS endpoint
	// answers 403 {"status":"fail"} without a paid key. Kept as a fallback for
	// networks that cannot reach ipwho.is.
	ipapiBase = "http://ip-api.com/json/"
)

// ipwhoisResponse matches the JSON returned by ipwho.is.
type ipwhoisResponse struct {
	Success    bool    `json:"success"`
	Message    string  `json:"message"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Country    string  `json:"country"`
	Region     string  `json:"region"`
	City       string  `json:"city"`
	Connection struct {
		ASN int64  `json:"asn"`
		Org string `json:"org"`
	} `json:"connection"`
}

// ipAPIResponse matches the JSON returned by ip-api.com.
type ipAPIResponse struct {
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Country    string  `json:"country"`
	RegionName string  `json:"regionName"`
	City       string  `json:"city"`
	Org        string  `json:"org"`
	As         string  `json:"as"`
	Status     string  `json:"status"`
	Message    string  `json:"message,omitempty"`
}

// Lookup resolves the geolocation of a public IP address.
// Returns nil if the IP is private/loopback or every provider fails.
func Lookup(ip string) (*Info, error) {
	// Skip private and loopback addresses — no geolocation data.
	if isPrivate(ip) {
		return nil, fmt.Errorf("private IP address")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	info, errWhois := lookupIPWhois(client, ip)
	if errWhois == nil {
		return info, nil
	}
	info, errAPI := lookupIPApi(client, ip)
	if errAPI == nil {
		return info, nil
	}
	return nil, fmt.Errorf("geoip: ipwho.is: %v; ip-api.com: %v", errWhois, errAPI)
}

func lookupIPWhois(client *http.Client, ip string) (*Info, error) {
	resp, err := client.Get(ipwhoisBase + ip)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var raw ipwhoisResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !raw.Success {
		if raw.Message == "" {
			raw.Message = "lookup failed"
		}
		return nil, fmt.Errorf("%s", raw.Message)
	}

	return &Info{
		Lat:     raw.Latitude,
		Lng:     raw.Longitude,
		Country: raw.Country,
		Area:    raw.Region,
		City:    raw.City,
		Org:     raw.Connection.Org,
		Asn:     asnString(raw.Connection.ASN),
	}, nil
}

func lookupIPApi(client *http.Client, ip string) (*Info, error) {
	resp, err := client.Get(ipapiBase + ip + "?fields=status,message,country,regionName,city,lat,lon,org,as")
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var raw ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if raw.Status != "success" {
		return nil, fmt.Errorf("%s", raw.Message)
	}

	// Extract just the AS number from "AS15169 Google LLC"
	asn := raw.As
	if idx := strings.Index(asn, " "); idx > 0 {
		asn = asn[:idx]
	}

	return &Info{
		Lat:     raw.Lat,
		Lng:     raw.Lon,
		Country: raw.Country,
		Area:    raw.RegionName,
		City:    raw.City,
		Org:     raw.Org,
		Asn:     asn,
	}, nil
}

func asnString(n int64) string {
	if n <= 0 {
		return ""
	}
	return "AS" + strconv.FormatInt(n, 10)
}

func isPrivate(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false // let the upstream API reject invalid IPs with a clear error
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
}
