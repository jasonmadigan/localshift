package addons

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/jasonmadigan/oinc/pkg/pullsecret"
	"github.com/jasonmadigan/oinc/pkg/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ingressOperatorNS    = "openshift-ingress-operator"
	ingressOperatorImage = "quay.io/openshift/origin-cluster-ingress-operator"
	routerImage          = "quay.io/openshift/origin-haproxy-router"
	oincContainer        = "oinc"
	// istio version supported by the OSSM version shipped in the catalogue.
	// update when the catalogue ships a newer OSSM.
	istioVersion = "v1.27-latest"
)

func init() { Register(&ingressOperator{}) }

type ingressOperator struct{}

func (i *ingressOperator) Name() string            { return "ingress-operator" }
func (i *ingressOperator) Dependencies() []string  { return []string{"gateway-api"} }
func (i *ingressOperator) NeedsPullSecret() bool    { return true }
func (i *ingressOperator) PullSecretRegistries() []string {
	return []string{"registry.redhat.io"}
}

func (i *ingressOperator) Install(ctx context.Context, cfg *Config) error {
	ver, err := detectOCPVersion(cfg)
	if err != nil {
		return fmt.Errorf("detecting OCP version: %w", err)
	}

	cfg.Logger.Info("installing cluster ingress operator", "version", ver.Version)

	if err := installConfigCRDs(ctx, cfg, ver); err != nil {
		return fmt.Errorf("installing config CRDs: %w", err)
	}

	if err := installOperatorCRDs(ctx, cfg, ver); err != nil {
		return fmt.Errorf("installing operator CRDs: %w", err)
	}

	for _, ns := range []string{ingressOperatorNS, "openshift-config", "openshift-config-managed"} {
		if err := ensureNamespace(ctx, cfg, ns); err != nil {
			return fmt.Errorf("creating namespace %s: %w", ns, err)
		}
	}

	if err := installOperatorRBAC(ctx, cfg, ver); err != nil {
		return fmt.Errorf("installing RBAC: %w", err)
	}

	if err := createConfigStubs(ctx, cfg); err != nil {
		return fmt.Errorf("creating config stubs: %w", err)
	}

	if err := populateFeatureGates(ctx, cfg, ver); err != nil {
		return fmt.Errorf("populating feature gates: %w", err)
	}

	if err := annotateServiceCAConfigMap(ctx, cfg); err != nil {
		return fmt.Errorf("annotating service-ca configmap: %w", err)
	}

	if err := injectPullSecretAndCatalogue(ctx, cfg, ver); err != nil {
		return fmt.Errorf("configuring pull secret and catalogue: %w", err)
	}

	if err := installConsoleCLIDownloadCRD(ctx, cfg, ver); err != nil {
		return fmt.Errorf("installing ConsoleCLIDownload CRD: %w", err)
	}

	if err := deployIngressOperator(ctx, cfg, ver); err != nil {
		return fmt.Errorf("deploying operator: %w", err)
	}

	// create the GatewayClass to trigger the operator's OLM flow
	cfg.Logger.Info("creating openshift-default GatewayClass")
	gcGVR := schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses",
	}
	gc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "GatewayClass",
		"metadata":   map[string]any{"name": "openshift-default"},
		"spec":       map[string]any{"controllerName": "openshift.io/gateway-controller/v1"},
	}}
	if err := ensureResource(ctx, cfg, gcGVR, gc); err != nil {
		return fmt.Errorf("creating GatewayClass: %w", err)
	}

	return nil
}

func (i *ingressOperator) Ready(ctx context.Context, cfg *Config) error {
	if err := waitForDeployment(ctx, cfg, ingressOperatorNS, "ingress-operator", 5*time.Minute); err != nil {
		return err
	}

	// wait for OLM to install the OSSM operator (triggered by GatewayClass)
	cfg.Logger.Info("waiting for OSSM operator")
	if err := waitForDeployment(ctx, cfg, "openshift-operators", "servicemesh-operator3", 10*time.Minute); err != nil {
		return fmt.Errorf("OSSM operator not ready: %w", err)
	}

	cfg.Logger.Info("waiting for Istio CRD")
	if err := waitForCRD(ctx, cfg, "istios.sailoperator.io", 2*time.Minute); err != nil {
		return fmt.Errorf("Istio CRD not found: %w", err)
	}

	// the 4.21 operator can't create the Istio CR due to a controller-runtime
	// cache bug with cluster-scoped resources (DefaultNamespaces doesn't start
	// informers for them). create it directly with the same pilot env vars the
	// operator would use. safe to re-run (ensureResource is idempotent).
	cfg.Logger.Info("creating Istio CR")
	if err := ensureIstioCR(ctx, cfg); err != nil {
		return fmt.Errorf("creating Istio CR: %w", err)
	}

	cfg.Logger.Info("waiting for istiod")
	return waitForDeployment(ctx, cfg, "openshift-ingress", "istiod-openshift-gateway", 5*time.Minute)
}

func ensureIstioCR(ctx context.Context, cfg *Config) error {
	istioGVR := schema.GroupVersionResource{
		Group: "sailoperator.io", Version: "v1", Resource: "istios",
	}
	istio := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "sailoperator.io/v1",
		"kind":       "Istio",
		"metadata":   map[string]any{"name": "openshift-gateway"},
		"spec": map[string]any{
			"namespace": "openshift-ingress",
			"version":   istioVersion,
			"values": map[string]any{
				"pilot": map[string]any{
					"enabled": true,
					"env": map[string]any{
						"PILOT_ENABLE_GATEWAY_API":                         "true",
						"PILOT_ENABLE_ALPHA_GATEWAY_API":                   "false",
						"PILOT_ENABLE_GATEWAY_API_STATUS":                  "true",
						"PILOT_ENABLE_GATEWAY_API_DEPLOYMENT_CONTROLLER":   "true",
						"PILOT_ENABLE_GATEWAY_API_GATEWAYCLASS_CONTROLLER": "false",
						"PILOT_GATEWAY_API_DEFAULT_GATEWAYCLASS_NAME":      "openshift-default",
						"PILOT_GATEWAY_API_CONTROLLER_NAME":                "openshift.io/gateway-controller/v1",
						"PILOT_MULTI_NETWORK_DISCOVER_GATEWAY_API":         "false",
						"ENABLE_GATEWAY_API_MANUAL_DEPLOYMENT":             "false",
						"PILOT_ENABLE_GATEWAY_API_CA_CERT_ONLY":            "true",
						"PILOT_ENABLE_GATEWAY_API_COPY_LABELS_ANNOTATIONS": "false",
					},
				},
			},
		},
	}}
	return ensureResource(ctx, cfg, istioGVR, istio)
}

func installConsoleCLIDownloadCRD(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	url := fmt.Sprintf(
		"https://raw.githubusercontent.com/openshift/api/%s/console/v1/zz_generated.crd-manifests/00_consoleclidownloads.crd.yaml",
		ver.APIBranch,
	)
	return applyManifestURL(ctx, cfg, url)
}

func waitForCRD(ctx context.Context, cfg *Config, name string, timeout time.Duration) error {
	crdGVR := schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := cfg.DynamicClient.Resource(crdGVR).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("CRD %s not found after %s", name, timeout)
}

func detectOCPVersion(cfg *Config) (version.OCPVersion, error) {
	info, err := cfg.Runtime.InspectContainer(oincContainer)
	if err != nil {
		return version.OCPVersion{}, fmt.Errorf("inspecting oinc container: %w", err)
	}
	ver, ok := version.ResolveFromImage(info.Image)
	if !ok {
		return version.OCPVersion{}, fmt.Errorf("could not determine OCP version from image %s", info.Image)
	}
	return ver, nil
}

// config.openshift.io CRDs needed by the ingress operator
var configCRDs = []struct {
	resource string
	file     string
}{
	{"clusteroperators", "0000_00_cluster-version-operator_01_clusteroperators.crd.yaml"},
	{"clusterversions", "0000_00_cluster-version-operator_01_clusterversions-Default.crd.yaml"},
	{"proxies", "0000_03_config-operator_01_proxies.crd.yaml"},
	{"apiservers", "0000_10_config-operator_01_apiservers-Default.crd.yaml"},
	{"dnses", "0000_10_config-operator_01_dnses.crd.yaml"},
	{"featuregates", "0000_10_config-operator_01_featuregates.crd.yaml"},
	{"infrastructures", "0000_10_config-operator_01_infrastructures-Default.crd.yaml"},
	{"ingresses", "0000_10_config-operator_01_ingresses.crd.yaml"},
	{"networks", "0000_10_config-operator_01_networks.crd.yaml"},
}

func installConfigCRDs(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	cfg.Logger.Info("installing config.openshift.io CRDs", "branch", ver.APIBranch)
	base := fmt.Sprintf("https://raw.githubusercontent.com/openshift/api/%s/config/v1/zz_generated.crd-manifests", ver.APIBranch)
	for _, crd := range configCRDs {
		url := base + "/" + crd.file
		if err := applyManifestURL(ctx, cfg, url); err != nil {
			return fmt.Errorf("applying %s CRD: %w", crd.resource, err)
		}
	}
	return nil
}

// operator manifest files fetched from the release branch
var operatorCRDFiles = []string{
	"00-custom-resource-definition.yaml",
	"00-custom-resource-definition-internal.yaml",
}

var operatorRBACFiles = []string{
	"01-service-account.yaml",
	"00-cluster-role.yaml",
	"01-cluster-role-binding.yaml",
	"01-role.yaml",
	"01-role-binding.yaml",
}

func operatorManifestURL(ver version.OCPVersion, file string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/openshift/cluster-ingress-operator/release-%s/manifests/%s", ver.Version, file)
}

func installOperatorCRDs(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	cfg.Logger.Info("installing IngressController CRDs")
	for _, f := range operatorCRDFiles {
		if err := applyManifestURL(ctx, cfg, operatorManifestURL(ver, f)); err != nil {
			return err
		}
	}
	return nil
}

func installOperatorRBAC(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	cfg.Logger.Info("installing ingress operator RBAC")
	for _, f := range operatorRBACFiles {
		if err := applyManifestURL(ctx, cfg, operatorManifestURL(ver, f)); err != nil {
			return err
		}
	}
	return nil
}

func createConfigStubs(ctx context.Context, cfg *Config) error {
	cfg.Logger.Info("creating config.openshift.io stub resources")

	stubs := []struct {
		resource string
		obj      *unstructured.Unstructured
	}{
		{"clusterversions", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterVersion",
			"metadata":   map[string]any{"name": "version"},
			"spec":       map[string]any{"clusterID": "oinc-microshift", "channel": ""},
		}}},
		{"infrastructures", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Infrastructure",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{"platformSpec": map[string]any{"type": "None"}},
		}}},
		{"dnses", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "DNS",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{"baseDomain": "oinc.local"},
		}}},
		{"ingresses", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Ingress",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{"domain": "apps.oinc.local"},
		}}},
		{"proxies", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Proxy",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{},
		}}},
		{"networks", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "Network",
			"metadata":   map[string]any{"name": "cluster"},
			"spec": map[string]any{
				"clusterNetwork": []any{map[string]any{"cidr": "10.42.0.0/16", "hostPrefix": int64(24)}},
				"serviceNetwork": []any{"10.43.0.0/16"},
			},
		}}},
		{"apiservers", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "APIServer",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{},
		}}},
		{"clusteroperators", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "ClusterOperator",
			"metadata":   map[string]any{"name": "ingress"},
			"spec":       map[string]any{},
		}}},
		{"featuregates", &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "config.openshift.io/v1",
			"kind":       "FeatureGate",
			"metadata":   map[string]any{"name": "cluster"},
			"spec":       map[string]any{"featureSet": ""},
		}}},
	}

	for _, s := range stubs {
		gvr := schema.GroupVersionResource{
			Group: "config.openshift.io", Version: "v1", Resource: s.resource,
		}
		if err := ensureResource(ctx, cfg, gvr, s.obj); err != nil {
			return fmt.Errorf("creating %s: %w", s.resource, err)
		}
	}

	// infrastructure needs status set via subresource
	infraGVR := schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "infrastructures"}
	infra, err := cfg.DynamicClient.Resource(infraGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting infrastructure: %w", err)
	}
	if err := unstructured.SetNestedMap(infra.Object, map[string]any{
		"platform": "None",
		"platformStatus": map[string]any{
			"type": "None",
		},
		"infrastructureName":     "oinc",
		"controlPlaneTopology":   "SingleReplica",
		"infrastructureTopology": "SingleReplica",
		"apiServerURL":           "https://127.0.0.1:6443",
		"etcdDiscoveryDomain":    "oinc.local",
	}, "status"); err != nil {
		return fmt.Errorf("setting infrastructure status: %w", err)
	}
	_, err = cfg.DynamicClient.Resource(infraGVR).UpdateStatus(ctx, infra, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating infrastructure status: %w", err)
	}

	// clusterversion needs capabilities for OLM mode
	cvGVR := schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "clusterversions"}
	cv, err := cfg.DynamicClient.Resource(cvGVR).Get(ctx, "version", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting clusterversion: %w", err)
	}
	if err := unstructured.SetNestedField(cv.Object, map[string]any{
		"desired": map[string]any{
			"version": "4.21.0",
			"image":   "oinc",
		},
		"availableUpdates":  []any{},
		"observedGeneration": int64(1),
		"versionHash":       "oinc",
		"capabilities": map[string]any{
			"enabledCapabilities": []any{
				"OperatorLifecycleManager",
				"marketplace",
			},
			"knownCapabilities": []any{
				"OperatorLifecycleManager",
				"marketplace",
			},
		},
	}, "status"); err != nil {
		return fmt.Errorf("setting clusterversion status: %w", err)
	}
	_, err = cfg.DynamicClient.Resource(cvGVR).UpdateStatus(ctx, cv, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating clusterversion status: %w", err)
	}

	return nil
}

// populateFeatureGates fetches features.go from openshift/api, parses all
// feature gate names, determines which are enabled by default, and populates
// the FeatureGate status subresource.
func populateFeatureGates(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	cfg.Logger.Info("populating feature gate status")

	url := fmt.Sprintf("https://raw.githubusercontent.com/openshift/api/%s/features/features.go", ver.APIBranch)
	source, err := fetchURL(ctx, url)
	if err != nil {
		return fmt.Errorf("fetching features.go: %w", err)
	}

	enabled, disabled := parseFeatureGates(string(source))
	if len(enabled)+len(disabled) == 0 {
		return fmt.Errorf("no feature gates found in features.go")
	}


	fgGVR := schema.GroupVersionResource{Group: "config.openshift.io", Version: "v1", Resource: "featuregates"}
	fg, err := cfg.DynamicClient.Resource(fgGVR).Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting featuregate: %w", err)
	}

	enabledList := make([]any, len(enabled))
	for i, name := range enabled {
		enabledList[i] = map[string]any{"name": name}
	}
	disabledList := make([]any, len(disabled))
	for i, name := range disabled {
		disabledList[i] = map[string]any{"name": name}
	}

	ocpVersion := ver.Version + ".0"
	if err := unstructured.SetNestedField(fg.Object, map[string]any{
		"featureGates": []any{
			map[string]any{
				"version":  ocpVersion,
				"enabled":  enabledList,
				"disabled": disabledList,
			},
		},
	}, "status"); err != nil {
		return fmt.Errorf("setting featuregate status: %w", err)
	}

	_, err = cfg.DynamicClient.Resource(fgGVR).UpdateStatus(ctx, fg, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating featuregate status: %w", err)
	}

	cfg.Logger.Info("feature gates populated", "enabled", len(enabled), "disabled", len(disabled))
	return nil
}

// parseFeatureGates extracts feature gate names from features.go source and
// sorts them into enabled-by-default and disabled lists.
// depends on the upstream pattern: newFeatureGate("Name").enableIn(configv1.Default, ...)
// if upstream changes this function signature, this parser will return zero
// results and the caller will fail with "no feature gates found".
func parseFeatureGates(source string) (enabled, disabled []string) {
	nameRe := regexp.MustCompile(`newFeatureGate\("([^"]+)"\)`)
	matches := nameRe.FindAllStringSubmatchIndex(source, -1)

	for i, match := range matches {
		name := source[match[2]:match[3]]

		// extract the block from this feature gate to the next one
		blockEnd := len(source)
		if i+1 < len(matches) {
			blockEnd = matches[i+1][0]
		}
		block := source[match[0]:blockEnd]

		if isEnabledByDefault(block) {
			enabled = append(enabled, name)
		} else {
			disabled = append(disabled, name)
		}
	}
	return
}

func isEnabledByDefault(block string) bool {
	idx := strings.Index(block, "enableIn(")
	if idx < 0 {
		return false
	}
	rest := block[idx:]
	parenEnd := strings.Index(rest, ")")
	if parenEnd < 0 {
		return false
	}
	return strings.Contains(rest[:parenEnd], "configv1.Default")
}

// annotateServiceCAConfigMap patches the existing service-ca-bundle configmap
// (created by MicroShift) to add the annotation the ingress operator expects.
// without this annotation the operator panics on nil annotations.
func annotateServiceCAConfigMap(ctx context.Context, cfg *Config) error {
	cmGVR := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	cm, err := cfg.DynamicClient.Resource(cmGVR).Namespace("openshift-ingress").Get(ctx, "service-ca-bundle", metav1.GetOptions{})
	if err != nil {
		// doesn't exist yet, create it
		cm = &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "service-ca-bundle",
				"namespace": "openshift-ingress",
				"annotations": map[string]any{
					"service.beta.openshift.io/inject-cabundle": "true",
				},
			},
			"data": map[string]any{"ca-bundle.crt": ""},
		}}
		_, err = cfg.DynamicClient.Resource(cmGVR).Namespace("openshift-ingress").Create(ctx, cm, metav1.CreateOptions{})
		return err
	}

	annotations := cm.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	if annotations["service.beta.openshift.io/inject-cabundle"] == "true" {
		return nil
	}
	annotations["service.beta.openshift.io/inject-cabundle"] = "true"
	cm.SetAnnotations(annotations)
	_, err = cfg.DynamicClient.Resource(cmGVR).Namespace("openshift-ingress").Update(ctx, cm, metav1.UpdateOptions{})
	return err
}

// injectPullSecretAndCatalogue configures the Red Hat pull secret in the
// MicroShift container and creates the redhat-operators CatalogSource.
func injectPullSecretAndCatalogue(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	cfg.Logger.Info("injecting pull secret into container")
	if err := pullsecret.InjectIntoContainer(cfg.Runtime, oincContainer); err != nil {
		return fmt.Errorf("injecting pull secret: %w", err)
	}

	cfg.Logger.Info("creating redhat-operators catalogue source")
	if err := ensureNamespace(ctx, cfg, "openshift-marketplace"); err != nil {
		return fmt.Errorf("creating openshift-marketplace namespace: %w", err)
	}

	csGVR := schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1alpha1", Resource: "catalogsources",
	}
	cs := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "CatalogSource",
		"metadata": map[string]any{
			"name":      "redhat-operators",
			"namespace": "openshift-marketplace",
		},
		"spec": map[string]any{
			"sourceType":  "grpc",
			"image":       fmt.Sprintf("registry.redhat.io/redhat/redhat-operator-index:v%s", ver.Version),
			"displayName": "Red Hat Operators",
			"publisher":   "Red Hat",
		},
	}}
	return ensureResource(ctx, cfg, csGVR, cs)
}

func deployIngressOperator(ctx context.Context, cfg *Config, ver version.OCPVersion) error {
	imageTag := ver.Version
	operatorRef := fmt.Sprintf("%s:%s", ingressOperatorImage, imageTag)
	routerRef := fmt.Sprintf("%s:%s", routerImage, imageTag)

	// the operator image may be amd64-only. if the container's arch doesn't
	// match, pull the amd64 variant by digest so CRI-O can find it.
	if needsDigestPull(cfg, operatorRef) {
		digest, err := getAMD64Digest(cfg, operatorRef)
		if err != nil {
			return fmt.Errorf("getting amd64 digest: %w", err)
		}
		digestRef := fmt.Sprintf("%s@%s", ingressOperatorImage, digest)
		cfg.Logger.Info("pre-pulling amd64 operator image", "ref", digestRef)
		if _, err := cfg.Runtime.ExecInContainer(oincContainer, "crictl", "pull", digestRef); err != nil {
			return fmt.Errorf("pre-pulling image: %w", err)
		}
		operatorRef = digestRef
	}

	cfg.Logger.Info("deploying ingress operator")
	dep := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "ingress-operator",
			"namespace": ingressOperatorNS,
		},
		"spec": map[string]any{
			"replicas": int64(1),
			"strategy": map[string]any{"type": "Recreate"},
			"selector": map[string]any{
				"matchLabels": map[string]any{"name": "ingress-operator"},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]any{"name": "ingress-operator"},
				},
				"spec": map[string]any{
					"securityContext": map[string]any{
						"runAsNonRoot": true,
						"seccompProfile": map[string]any{
							"type": "RuntimeDefault",
						},
					},
					"nodeSelector": map[string]any{
						"kubernetes.io/os":                      "linux",
						"node-role.kubernetes.io/control-plane": "",
					},
					"tolerations": []any{
						map[string]any{
							"key":      "node-role.kubernetes.io/master",
							"operator": "Exists",
							"effect":   "NoSchedule",
						},
					},
					"serviceAccountName": "ingress-operator",
					"containers": []any{
						map[string]any{
							"name":            "ingress-operator",
							"image":           operatorRef,
							"imagePullPolicy": "IfNotPresent",
							"securityContext": map[string]any{
								"allowPrivilegeEscalation": false,
								"capabilities":             map[string]any{"drop": []any{"ALL"}},
							},
							"command": []any{
								"ingress-operator", "start",
								"--namespace", ingressOperatorNS,
								"--image", routerRef,
								"--canary-image", operatorRef,
								"--release-version", ver.Version + ".0",
							},
							"resources": map[string]any{
								"requests": map[string]any{
									"cpu":    "10m",
									"memory": "56Mi",
								},
							},
						},
					},
				},
			},
		},
	}}

	return ensureResource(ctx, cfg, deploymentGVR, dep)
}

// needsDigestPull checks whether the image manifest lacks a variant for the
// container's architecture. if so, we need to pull the amd64 variant by digest.
func needsDigestPull(cfg *Config, imageRef string) bool {
	out, err := exec.Command(cfg.Runtime.Name(), "manifest", "inspect", imageRef).Output()
	if err != nil {
		return false
	}
	var manifest struct {
		Manifests []struct {
			Platform struct {
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(out, &manifest); err != nil {
		return false
	}

	// check what arch the oinc container runs
	containerArch := goruntime.GOARCH
	if info, err := cfg.Runtime.InspectContainer(oincContainer); err == nil {
		// the image tag tells us the arch (e.g. "4.21.0-okd-scos.ec.15-arm64")
		if strings.Contains(info.Image, "-arm64") {
			containerArch = "arm64"
		} else if strings.Contains(info.Image, "-amd64") {
			containerArch = "amd64"
		}
	}

	for _, m := range manifest.Manifests {
		if m.Platform.Architecture == containerArch {
			return false
		}
	}
	return true
}

// getAMD64Digest inspects a multi-arch manifest and returns the amd64 image digest.
func getAMD64Digest(cfg *Config, imageRef string) (string, error) {
	out, err := exec.Command(cfg.Runtime.Name(), "manifest", "inspect", imageRef).Output()
	if err != nil {
		return "", fmt.Errorf("inspecting manifest for %s: %w", imageRef, err)
	}

	var manifest struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(out, &manifest); err != nil {
		return "", fmt.Errorf("parsing manifest: %w", err)
	}

	for _, m := range manifest.Manifests {
		if m.Platform.Architecture == "amd64" {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("no amd64 manifest found for %s", imageRef)
}
