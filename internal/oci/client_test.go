package oci

import (
	"errors"
	"testing"

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
