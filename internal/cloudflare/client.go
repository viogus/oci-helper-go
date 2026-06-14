package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
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
	var resp apiResponse[[]Zone]
	if err := c.do("GET", "/zones", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (c *Client) ListDNSRecords(zoneID string) ([]DNSRecord, error) {
	var resp apiResponse[[]DNSRecord]
	if err := c.do("GET", "/zones/"+zoneID+"/dns_records", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
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
	for _, r := range records {
		if r.Name == name {
			r.Content = newIP
			_, err := c.UpdateDNSRecord(zoneID, r.ID, r)
			return err
		}
	}
	// create if not found
	_, err = c.CreateDNSRecord(zoneID, DNSRecord{
		Type:    "A",
		Name:    name,
		Content: newIP,
		Proxied: false,
		TTL:     120,
	})
	return err
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
