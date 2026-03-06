package addons

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const defaultGatewayAPIVersion = "1.2.1"

func init() { Register(&gatewayAPI{}) }

type gatewayAPI struct {
	version string
}

func (g *gatewayAPI) Name() string           { return "gateway-api" }
func (g *gatewayAPI) Dependencies() []string { return nil }

func (g *gatewayAPI) SetOptions(opts map[string]string) {
	if v, ok := opts["version"]; ok {
		g.version = v
	}
}

func (g *gatewayAPI) resolveVersion() string {
	if g.version != "" {
		return g.version
	}
	return defaultGatewayAPIVersion
}

func (g *gatewayAPI) Install(ctx context.Context, cfg *Config) error {
	v := g.resolveVersion()
	url := fmt.Sprintf("https://github.com/kubernetes-sigs/gateway-api/releases/download/v%s/standard-install.yaml", v)
	cfg.Logger.Info("fetching gateway-api CRDs", "url", url)

	if _, err := exec.LookPath("curl"); err != nil {
		return fmt.Errorf("curl is required but not found in PATH")
	}
	data, err := exec.CommandContext(ctx, "curl", "-sSL", "--retry", "3", "--max-time", "30", url).Output()
	if err != nil {
		return fmt.Errorf("downloading gateway-api manifests: %w", err)
	}

	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding gateway-api manifest: %w", err)
		}

		if obj.Object == nil {
			continue
		}

		cfg.Logger.Info("applying", "kind", obj.GetKind(), "name", obj.GetName())

		existing, err := cfg.DynamicClient.Resource(crdGVR).Get(ctx, obj.GetName(), metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				if _, err := cfg.DynamicClient.Resource(crdGVR).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
					return fmt.Errorf("creating %s: %w", obj.GetName(), err)
				}
				continue
			}
			return fmt.Errorf("getting %s: %w", obj.GetName(), err)
		}

		obj.SetResourceVersion(existing.GetResourceVersion())
		if _, err := cfg.DynamicClient.Resource(crdGVR).Update(ctx, obj, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating %s: %w", obj.GetName(), err)
		}
	}

	return nil
}

func (g *gatewayAPI) Ready(ctx context.Context, cfg *Config) error {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	expected := []string{
		"gatewayclasses.gateway.networking.k8s.io",
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
	}

	for range 30 {
		allReady := true
		for _, name := range expected {
			crd, err := cfg.DynamicClient.Resource(crdGVR).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				allReady = false
				break
			}

			conditions, found, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
			if !found {
				allReady = false
				break
			}

			established := false
			for _, c := range conditions {
				cm, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if cm["type"] == "Established" && cm["status"] == "True" {
					established = true
					break
				}
			}
			if !established {
				allReady = false
				break
			}
		}

		if allReady {
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("gateway-api CRDs not established after 60s")
}
