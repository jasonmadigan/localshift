package addons

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fetchURL downloads a URL using curl (Go's net stack breaks in privileged containers on macOS).
func fetchURL(ctx context.Context, url string) ([]byte, error) {
	if _, err := exec.LookPath("curl"); err != nil {
		return nil, fmt.Errorf("curl is required but not found in PATH")
	}
	data, err := exec.CommandContext(ctx, "curl", "-sSL", "--retry", "3", "--max-time", "60", url).Output()
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	return data, nil
}

// applyManifests applies a multi-document YAML manifest using kubectl server-side apply.
func applyManifests(ctx context.Context, cfg *Config, manifest []byte) error {
	if _, err := exec.LookPath("kubectl"); err != nil {
		return fmt.Errorf("kubectl is required but not found in PATH")
	}
	kubeconfigFile, err := os.CreateTemp("", "oinc-kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("creating temp kubeconfig: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(cfg.Kubeconfig); err != nil {
		kubeconfigFile.Close()
		return fmt.Errorf("writing temp kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	cmd := exec.CommandContext(ctx, "kubectl", "apply",
		"--kubeconfig", kubeconfigFile.Name(),
		"--server-side",
		"--force-conflicts",
		"-f", "-",
	)
	cmd.Stdin = bytes.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %s: %w", string(out), err)
	}
	return nil
}

// applyManifestURL fetches a URL and applies all objects.
func applyManifestURL(ctx context.Context, cfg *Config, url string) error {
	cfg.Logger.Info("fetching manifests", "url", url)
	data, err := fetchURL(ctx, url)
	if err != nil {
		return err
	}
	return applyManifests(ctx, cfg, data)
}

var deploymentGVR = schema.GroupVersionResource{
	Group: "apps", Version: "v1", Resource: "deployments",
}

// waitForDeployment polls until a deployment has available replicas.
func waitForDeployment(ctx context.Context, cfg *Config, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dep, err := cfg.DynamicClient.Resource(deploymentGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			avail, _, _ := unstructured.NestedInt64(dep.Object, "status", "availableReplicas")
			if avail > 0 {
				cfg.Logger.Info("deployment ready", "namespace", namespace, "name", name)
				return nil
			}
		}
		cfg.Logger.Debug("waiting for deployment", "namespace", namespace, "name", name)
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("deployment %s/%s not ready after %s", namespace, name, timeout)
}

