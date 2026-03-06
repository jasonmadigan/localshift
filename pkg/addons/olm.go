package addons

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var namespaceGVR = schema.GroupVersionResource{
	Version: "v1", Resource: "namespaces",
}

func ensureNamespace(ctx context.Context, cfg *Config, name string) error {
	_, err := cfg.DynamicClient.Resource(namespaceGVR).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	ns := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata":   map[string]any{"name": name},
		},
	}
	_, err = cfg.DynamicClient.Resource(namespaceGVR).Create(ctx, ns, metav1.CreateOptions{})
	return err
}

// ensureResource creates an unstructured resource if it doesn't exist,
// retrying to handle CRD propagation delay.
func ensureResource(ctx context.Context, cfg *Config, gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
	ns := obj.GetNamespace()
	name := obj.GetName()

	var client dynamic.ResourceInterface
	if ns != "" {
		client = cfg.DynamicClient.Resource(gvr).Namespace(ns)
	} else {
		client = cfg.DynamicClient.Resource(gvr)
	}

	_, err := client.Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	for range 6 {
		_, err = client.Create(ctx, obj, metav1.CreateOptions{})
		if err == nil {
			return nil
		}
		cfg.Logger.Debug("waiting for CRD", "kind", obj.GetKind(), "err", err)
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("creating %s/%s: %w", obj.GetKind(), name, err)
}
