// Package cloudflare provides a lightweight HTTP client for the Cloudflare API v4.
//
package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.cloudflare.com/client/v4"

type Client struct {
	token  string
	client *http.Client
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type,omitempty"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
	Proxied *bool  `json:"proxied,omitempty"`
	TTL     int    `json:"ttl,omitempty"`
}

type apiResponse[T any] struct {
	Success  bool   `json:"success"`
	Errors   []any  `json:"errors"`
	Messages []any  `json:"messages"`
	Result   T      `json:"result"`
}

func New(token string) *Client {
	return &Client{
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ListZones() ([]Zone, error) {
	var all []Zone
	page := 1
	for {
		var resp apiResponse[[]Zone]
		path := "/zones?" + url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}.Encode()
		if err := c.do("GET", path, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Result...)
		if len(resp.Result) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) ListDNSRecords(zoneID string) ([]DNSRecord, error) {
	var all []DNSRecord
	page := 1
	for {
		var resp apiResponse[[]DNSRecord]
		path := "/zones/" + zoneID + "/dns_records?" + url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}.Encode()
		if err := c.do("GET", path, nil, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Result...)
		if len(resp.Result) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) CreateDNSRecord(zoneID string, record DNSRecord) (*DNSRecord, error) {
	var resp apiResponse[DNSRecord]
	if err := c.do("POST", "/zones/"+zoneID+"/dns_records", record, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func (c *Client) UpdateDNSRecord(zoneID, recordID string, record DNSRecord) (*DNSRecord, error) {
	var resp apiResponse[DNSRecord]
	if err := c.do("PATCH", "/zones/"+zoneID+"/dns_records/"+recordID, record, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func (c *Client) DeleteDNSRecord(zoneID, recordID string) error {
	var resp apiResponse[any]
	return c.do("DELETE", "/zones/"+zoneID+"/dns_records/"+recordID, nil, &resp)
}

func (c *Client) UpdateDNSRecordIP(zoneID, name, newIP string) error {
	records, err := c.ListDNSRecords(zoneID)
	if err != nil {
		return err
	}
	// Normalize name for comparison: strip trailing dot, lowercase.
	normalize := func(s string) string {
		s = strings.TrimRight(s, ".")
		return strings.ToLower(s)
	}
	target := normalize(name)
	for _, r := range records {
		if normalize(r.Name) == target {
			r.Content = newIP
			_, err := c.UpdateDNSRecord(zoneID, r.ID, r)
			return err
		}
	}
	return fmt.Errorf("dns record %s not found in zone %s", name, zoneID)
}

func (c *Client) do(method, path string, body interface{}, result interface{}) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
	}

	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Check Cloudflare API success before returning typed result
	var check struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &check); err == nil && !check.Success {
		if len(check.Errors) > 0 {
			return fmt.Errorf("cloudflare: %s", check.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare: API error")
	}

	return json.Unmarshal(raw, result)
}
