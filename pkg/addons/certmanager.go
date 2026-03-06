package addons

import (
	"context"
	"fmt"
	"time"
)

const defaultCertManagerVersion = "1.17.1"

func init() { Register(&certManager{}) }

type certManager struct {
	version string
}

func (c *certManager) Name() string           { return "cert-manager" }
func (c *certManager) Dependencies() []string { return nil }

func (c *certManager) SetOptions(opts map[string]string) {
	if v, ok := opts["version"]; ok {
		c.version = v
	}
}

func (c *certManager) resolveVersion() string {
	if c.version != "" {
		return c.version
	}
	return defaultCertManagerVersion
}

func (c *certManager) Install(ctx context.Context, cfg *Config) error {
	v := c.resolveVersion()
	url := fmt.Sprintf("https://github.com/cert-manager/cert-manager/releases/download/v%s/cert-manager.yaml", v)
	return applyManifestURL(ctx, cfg, url)
}

func (c *certManager) Ready(ctx context.Context, cfg *Config) error {
	return waitForDeployment(ctx, cfg, "cert-manager", "cert-manager-webhook", 5*time.Minute)
}
