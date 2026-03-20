package version

import (
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantVer string
		wantErr bool
	}{
		{"empty returns default", "", Default().Version, false},
		{"valid version", "4.20", "4.20", false},
		{"invalid version", "3.99", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", got.Version, tt.wantVer)
			}
		})
	}
}

func TestResolveFromImage(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantVer string
		wantOK  bool
	}{
		{
			"valid image tag",
			"ghcr.io/jasonmadigan/oinc:4.21.0-okd-scos.ec.15-arm64",
			"4.21",
			true,
		},
		{
			"unknown image tag",
			"ghcr.io/jasonmadigan/oinc:9.99.0-unknown",
			"",
			false,
		},
		{
			"no colon in image",
			"ghcr.io/jasonmadigan/oinc",
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveFromImage(tt.image)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", got.Version, tt.wantVer)
			}
		})
	}
}
