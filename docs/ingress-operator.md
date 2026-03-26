# Ingress Operator Addon

Installs the [OpenShift cluster ingress operator](https://github.com/openshift/cluster-ingress-operator) on MicroShift with Gateway API support.

## What it does

- Manages IngressControllers (HAProxy router for Routes/Ingress)
- Provides Gateway API via OLM-installed OSSM (OpenShift Service Mesh v3)
- Creates `openshift-default` GatewayClass with controller `openshift.io/gateway-controller/v1`
- Automatically provisions istiod and per-Gateway proxy deployments

## Prerequisites

A Red Hat pull secret is required for the `registry.redhat.io` operator catalogue.

```
oinc pull-secret set ~/path/to/pull-secret.json
```

Get one from: https://cloud.redhat.com/openshift/install/pull-secret

## Usage

```
oinc create
oinc addon install ingress-operator
```

Then create a Gateway:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: http
    port: 80
    protocol: HTTP
```

## What gets installed

| Component | Source | Purpose |
|-|-|-|
| 9 config.openshift.io CRDs | openshift/api (release branch) | Infrastructure, DNS, FeatureGate, etc. |
| IngressController + DNSRecord CRDs | cluster-ingress-operator/manifests | Operator's own CRDs |
| ConsoleCLIDownload CRD | inline (minimal) | Required by OSSM CSV |
| Operator RBAC | cluster-ingress-operator/manifests | SA, ClusterRole, bindings |
| Config stub resources | inline | Infrastructure, ClusterVersion, DNS, etc. |
| FeatureGate status | parsed from openshift/api features.go | All feature gates with correct enabled/disabled state |
| redhat-operators CatalogSource | registry.redhat.io | OLM catalogue for OSSM |
| Ingress operator deployment | inline | The operator itself (amd64 image, digest-pinned on arm64) |
| openshift-default GatewayClass | inline | Triggers the OLM flow |
| Istio CR | inline | Workaround for operator cache bug (see below) |

## How the OLM flow works

1. Addon creates GatewayClass with `controllerName: openshift.io/gateway-controller/v1`
2. Ingress operator detects it, creates OLM Subscription for `servicemeshoperator3`
3. OLM resolves from `redhat-operators` catalogue, installs OSSM v3
4. OSSM installs Sail operator CRDs (`istios.sailoperator.io`, etc.)
5. Addon creates Istio CR `openshift-gateway` (workaround, see below)
6. OSSM deploys `istiod-openshift-gateway` in `openshift-ingress`
7. When user creates a Gateway, istiod provisions a proxy deployment

## Known workarounds

**Istio CR creation:** The 4.21 operator can't create the cluster-scoped Istio CR due to a controller-runtime cache bug (`DefaultNamespaces` doesn't handle cluster-scoped resources). The addon creates it directly with the pilot env vars the operator would use. This workaround can be removed when a newer operator image fixes the cache issue.

**ConsoleCLIDownload CRD:** OSSM's CSV includes this resource type which MicroShift doesn't have. A minimal CRD is created inline.

**Signature policy:** MicroShift enforces GPG signatures for `registry.redhat.io`. The addon relaxes this for the catalogue image pull.

**Service-ca-bundle annotation:** MicroShift creates the configmap without annotations. The operator panics on nil annotations. The addon patches it.

## Keeping it up to date

### CRDs and manifests

Config CRDs are fetched from `openshift/api` at the release branch matching the MicroShift version (e.g. `release-4.21`). The branch is derived automatically from the running container's image tag.

Operator CRDs and RBAC are fetched from `openshift/cluster-ingress-operator` master branch. Pin to a release branch if stability is preferred over freshness.

### Feature gates

Feature gates are parsed at install time from `openshift/api/{branch}/features/features.go`. This automatically picks up new gates when the MicroShift version changes.

### Operator image

Currently uses `quay.io/openshift/origin-cluster-ingress-operator:{version}`. This is the OKD origin build. When OKD SCOS releases include newer images (check `quay.io/okd/scos-content` in release payloads), those could be used instead.

### OSSM version

Controlled by the OLM catalogue. The catalogue image is `registry.redhat.io/redhat/redhat-operator-index:v{version}` — it ships whatever OSSM version Red Hat has published for that OCP release.

### Istio version in the CR

Currently hardcoded to `v1.27-latest` (what OSSM v3.2 supports). Should be updated when the catalogue ships a newer OSSM version with different supported Istio versions. Check supported versions with:

```
kubectl get istio openshift-gateway -o jsonpath='{.status}'
```

## Future improvements

1. **Build operator from source (master):** Eliminates the Istio CR workaround (cache bug fixed) and enables Sail Library mode (no OLM/catalogue/pull secret needed).
2. **Track OKD SCOS releases:** Use SCOS operator images when available. Check for Sail Library support with `strings <binary> | grep sail-operator`.
3. **Make Istio version configurable:** Derive from the OSSM CSV or make it a configurable option.
