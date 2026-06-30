// Package geoip provides IP geolocation lookup using the free ip-api.com service.
package geoip

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Info holds geolocation data for a single IP address.
type Info struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"` // ip-api uses 'lon' — normalized on decode
	Country string  `json:"country"`
	Area    string  `json:"regionName"`
	City    string  `json:"city"`
	Org     string  `json:"org"`
	Asn     string  `json:"as"`
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

// Lookup queries ip-api.com for the geolocation of an IP address.
// Returns nil if the IP is private/loopback or lookup fails.
func Lookup(ip string) (*Info, error) {
	// Skip private and loopback addresses — no geolocation data.
	if isPrivate(ip) {
		return nil, fmt.Errorf("private IP address")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=status,message,country,regionName,city,lat,lon,org,as")
	if err != nil {
		return nil, fmt.Errorf("geoip request: %w", err)
	}
	defer resp.Body.Close()

	var raw ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("geoip decode: %w", err)
	}
	if raw.Status != "success" {
		return nil, fmt.Errorf("geoip: %s", raw.Message)
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

func isPrivate(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified()
}
