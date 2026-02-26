package guestapi

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestGuestAPI_Validate(t *testing.T) {
	tests := []struct {
		name    string
		g       *GuestAPI
		wantErr bool
	}{
		{"nil is valid", nil, false},
		{"valid port", &GuestAPI{Port: 10000}, false},
		{"port 1 is valid", &GuestAPI{Port: 1}, false},
		{"port zero", &GuestAPI{Port: 0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.g.Validate()
			if tt.wantErr {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
			}
		})
	}
}

func TestUDSPath(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		want       string
	}{
		{"empty", "", ""},
		{
			"standard jailer layout",
			"/srv/jailer/firecracker/abc123/root/run/firecracker.socket",
			"/srv/jailer/firecracker/abc123/root/v.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UDSPath(tt.socketPath)
			must.Eq(t, tt.want, got)
		})
	}
}
