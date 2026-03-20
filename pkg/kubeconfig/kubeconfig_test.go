package kubeconfig

import (
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func setupTempKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	t.Setenv("KUBECONFIG", path)
	return path
}

func TestUpdate(t *testing.T) {
	path := setupTempKubeconfig(t)

	incoming := api.NewConfig()
	incoming.Clusters["oinc"] = &api.Cluster{Server: "https://127.0.0.1:6443"}
	incoming.Contexts["oinc"] = &api.Context{Cluster: "oinc", AuthInfo: "oinc-admin"}
	incoming.AuthInfos["oinc-admin"] = &api.AuthInfo{Token: "test-token"}
	incoming.CurrentContext = "oinc"

	raw, err := clientcmd.Write(*incoming)
	if err != nil {
		t.Fatalf("serialising kubeconfig: %v", err)
	}

	if err := Update(raw); err != nil {
		t.Fatalf("Update: %v", err)
	}

	result, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("loading result: %v", err)
	}

	if result.CurrentContext != "oinc" {
		t.Errorf("CurrentContext = %q, want %q", result.CurrentContext, "oinc")
	}
	if c, ok := result.Clusters["oinc"]; !ok {
		t.Error("cluster 'oinc' not found")
	} else if c.Server != "https://127.0.0.1:6443" {
		t.Errorf("Server = %q, want %q", c.Server, "https://127.0.0.1:6443")
	}
	if _, ok := result.Contexts["oinc"]; !ok {
		t.Error("context 'oinc' not found")
	}
	if _, ok := result.AuthInfos["oinc-admin"]; !ok {
		t.Error("authinfo 'oinc-admin' not found")
	}
}

func TestRemove(t *testing.T) {
	path := setupTempKubeconfig(t)

	cfg := api.NewConfig()
	cfg.Clusters["oinc"] = &api.Cluster{Server: "https://127.0.0.1:6443"}
	cfg.Clusters["other"] = &api.Cluster{Server: "https://other:6443"}
	cfg.Contexts["oinc"] = &api.Context{Cluster: "oinc", AuthInfo: "oinc-admin"}
	cfg.Contexts["other"] = &api.Context{Cluster: "other", AuthInfo: "other-admin"}
	cfg.AuthInfos["oinc-admin"] = &api.AuthInfo{Token: "test-token"}
	cfg.AuthInfos["other-admin"] = &api.AuthInfo{Token: "other-token"}
	cfg.CurrentContext = "oinc"

	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	result, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("loading result: %v", err)
	}

	if _, ok := result.Clusters["oinc"]; ok {
		t.Error("cluster 'oinc' should have been removed")
	}
	if _, ok := result.Contexts["oinc"]; ok {
		t.Error("context 'oinc' should have been removed")
	}
	if _, ok := result.AuthInfos["oinc-admin"]; ok {
		t.Error("authinfo 'oinc-admin' should have been removed")
	}
	if result.CurrentContext != "" {
		t.Errorf("CurrentContext = %q, want empty", result.CurrentContext)
	}
	if _, ok := result.Clusters["other"]; !ok {
		t.Error("cluster 'other' should still exist")
	}
}

func TestRemoveNoOp(t *testing.T) {
	path := setupTempKubeconfig(t)

	cfg := api.NewConfig()
	cfg.Clusters["other"] = &api.Cluster{Server: "https://other:6443"}
	cfg.Contexts["other"] = &api.Context{Cluster: "other", AuthInfo: "other-admin"}
	cfg.AuthInfos["other-admin"] = &api.AuthInfo{Token: "other-token"}
	cfg.CurrentContext = "other"

	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if err := Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	result, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("loading result: %v", err)
	}

	if result.CurrentContext != "other" {
		t.Errorf("CurrentContext = %q, want %q", result.CurrentContext, "other")
	}
	if _, ok := result.Clusters["other"]; !ok {
		t.Error("cluster 'other' should still exist")
	}
}

func TestRemoveNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBECONFIG", filepath.Join(dir, "nonexistent"))

	if err := Remove(); err != nil {
		t.Fatalf("Remove on missing file: %v", err)
	}
}

func TestUpdateMergesIntoExisting(t *testing.T) {
	path := setupTempKubeconfig(t)

	existing := api.NewConfig()
	existing.Clusters["other"] = &api.Cluster{Server: "https://other:6443"}
	existing.Contexts["other"] = &api.Context{Cluster: "other", AuthInfo: "other-admin"}
	existing.AuthInfos["other-admin"] = &api.AuthInfo{Token: "other-token"}
	existing.CurrentContext = "other"

	if err := clientcmd.WriteToFile(*existing, path); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	incoming := api.NewConfig()
	incoming.Clusters["oinc"] = &api.Cluster{Server: "https://127.0.0.1:6443"}
	incoming.Contexts["oinc"] = &api.Context{Cluster: "oinc", AuthInfo: "oinc-admin"}
	incoming.AuthInfos["oinc-admin"] = &api.AuthInfo{Token: "test-token"}
	incoming.CurrentContext = "oinc"

	raw, err := clientcmd.Write(*incoming)
	if err != nil {
		t.Fatalf("serialising kubeconfig: %v", err)
	}

	if err := Update(raw); err != nil {
		t.Fatalf("Update: %v", err)
	}

	result, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("loading result: %v", err)
	}

	if len(result.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(result.Clusters))
	}
	if _, ok := result.Clusters["other"]; !ok {
		t.Error("existing cluster 'other' should be preserved")
	}
	if _, ok := result.Clusters["oinc"]; !ok {
		t.Error("new cluster 'oinc' should be added")
	}
	if result.CurrentContext != "oinc" {
		t.Errorf("CurrentContext = %q, want %q", result.CurrentContext, "oinc")
	}
}
