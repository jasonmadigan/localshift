package addons

import (
	"context"
	"fmt"
	"time"
)

const defaultMetalLBVersion = "0.14.9"

func init() { Register(&metalLB{}) }

type metalLB struct {
	version string
}

func (m *metalLB) Name() string           { return "metallb" }
func (m *metalLB) Dependencies() []string { return nil }

func (m *metalLB) SetOptions(opts map[string]string) {
	if v, ok := opts["version"]; ok {
		m.version = v
	}
}

func (m *metalLB) resolveVersion() string {
	if m.version != "" {
		return m.version
	}
	return defaultMetalLBVersion
}

func (m *metalLB) Install(ctx context.Context, cfg *Config) error {
	v := m.resolveVersion()
	url := fmt.Sprintf("https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", v)
	return applyManifestURL(ctx, cfg, url)
}

func (m *metalLB) Ready(ctx context.Context, cfg *Config) error {
	return waitForDeployment(ctx, cfg, "metallb-system", "controller", 5*time.Minute)
}

