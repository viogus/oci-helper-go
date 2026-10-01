package oci

import (
	"errors"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// fakeServiceError is a minimal common.ServiceError implementation for tests.
type fakeServiceError struct {
	status int
}

func (f fakeServiceError) GetHTTPStatusCode() int  { return f.status }
func (f fakeServiceError) GetMessage() string      { return "test error" }
func (f fakeServiceError) GetCode() string         { return "TestCode" }
func (f fakeServiceError) GetOpcRequestID() string { return "opc-request-id" }
func (f fakeServiceError) Error() string           { return f.GetMessage() }

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"plain error", errors.New("boom"), false},
		{"wrapped plain error", errors.New("outer: boom"), false},
		{"404 service error", fakeServiceError{status: 404}, true},
		{"wrapped 404 service error", errors.New("outer: 404"), true},
		{"400 service error", fakeServiceError{status: 400}, false},
		{"500 service error", fakeServiceError{status: 500}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error = tt.err
			if tt.name == "wrapped 404 service error" {
				// wrap the 404 so errors.As has to unwrap
				err = &wrapError{inner: fakeServiceError{status: 404}}
			}
			if got := isNotFound(err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// wrapError implements error with Unwrap for errors.As testing.
type wrapError struct{ inner error }

func (w *wrapError) Error() string { return "wrapped: " + w.inner.Error() }
func (w *wrapError) Unwrap() error { return w.inner }

func TestMonitoringInterval(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		start, end time.Time
		wantStr    string
		wantStep   time.Duration
		wantErr    error
	}{
		{"recent hour", now.Add(-time.Hour), now, "[1m]", time.Minute, nil},
		{"six days long, recent", now.Add(-6 * 24 * time.Hour), now, "[1m]", time.Minute, nil},
		{"twenty days back, one hour long", now.Add(-20 * 24 * time.Hour), now.Add(-20*24*time.Hour + time.Hour), "[5m]", 5 * time.Minute, nil},
		{"window longer than the lookback", now.Add(-24 * time.Hour), now.Add(20 * 24 * time.Hour), "[5m]", 5 * time.Minute, nil},
		// A 30-day window that ended last month is past the 5-minute cap, which
		// OCI measures from now: [5m] would return no data points at all.
		{"last month, thirty days long", now.Add(-61 * 24 * time.Hour), now.Add(-31 * 24 * time.Hour), "[1h]", time.Hour, nil},
		{"eighty-five days back", now.Add(-85 * 24 * time.Hour), now.Add(-60 * 24 * time.Hour), "[1h]", time.Hour, nil},
		{"beyond retention", now.Add(-120 * 24 * time.Hour), now.Add(-100 * 24 * time.Hour), "", 0, ErrMetricsTooOld},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStr, gotStep, err := monitoringInterval(tc.start, tc.end)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if gotStr != tc.wantStr || gotStep != tc.wantStep {
				t.Fatalf("interval = %q/%v, want %q/%v", gotStr, gotStep, tc.wantStr, tc.wantStep)
			}
		})
	}
}

func TestIpInCIDR(t *testing.T) {
	tests := []struct {
		ip, cidr string
		want     bool
	}{
		{"1.2.3.4", "1.2.3.0/24", true},
		{"1.2.4.4", "1.2.3.0/24", false},
		{"1.2.3.4", "0.0.0.0/0", true},
		{"1.2.3.4", "", false},
		{"not-an-ip", "1.2.3.0/24", false},
		{"1.2.3.4", "not-a-cidr", false},
		{"1.2.3.4", "10.0.0.0/8", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip+"/"+tt.cidr, func(t *testing.T) {
			if got := ipInCIDR(tt.ip, tt.cidr); got != tt.want {
				t.Errorf("ipInCIDR(%q, %q) = %v, want %v", tt.ip, tt.cidr, got, tt.want)
			}
		})
	}
}

var _ = common.ServiceError(fakeServiceError{})
