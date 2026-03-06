# localshift

A local OCP-like cluster that just works. MicroShift under the hood, but with Console, OLM, and ConsolePlugin CRD out of the box.

```
localshift create
```

That's it. You get a single-node cluster with the OpenShift Console on `localhost:9000`, OLM running, and the ConsolePlugin CRD available -- close enough to real OCP for local dev work.

## Features

- **Auto-detects container runtime** (docker, podman) -- no flags needed
- **Version switching** -- `localshift create --version 4.18` to target a specific OCP release
- **Console included** -- OpenShift Console runs as a sidecar, no separate setup
- **OLM included** -- baked into the image, operator workflows work out of the box
- **Addon system** -- layer on Gateway API, cert-manager, MetalLB, Istio as needed
- **Console plugin support** -- `--console-plugin "my-plugin=http://localhost:9001"` for plugin dev

## Quick start

```bash
# install
go install github.com/jasonmadigan/localshift@latest

# create cluster (latest OCP version, auto-detect runtime)
localshift create

# create with a specific version
localshift create --version 4.18

# create with addons
localshift create --addons gateway-api,cert-manager

# wire in a console plugin dev server
localshift create --console-plugin "my-plugin=http://host.docker.internal:9001"

# switch OCP version
localshift switch 4.18

# list available versions
localshift version list

# tear down
localshift delete
```

## Addons

The base cluster includes MicroShift + OLM + Console + ConsolePlugin CRD. Addons layer extra infrastructure on top:

| Addon | What it provides |
|-|-|
| `gateway-api` | Kubernetes Gateway API CRDs |
| `cert-manager` | Certificate management |
| `metallb` | LoadBalancer IP allocation |
| `istio` | Istio service mesh via Sail operator |

```bash
# at create time
localshift create --addons gateway-api,cert-manager,metallb,istio

# or post-hoc
localshift addon install gateway-api
localshift addon list
```

## Requirements

- Docker (including OrbStack) or Podman
- ~4GB RAM available for the container

## Acknowledgements

localshift builds on the work of several projects:

- [MicroShift](https://github.com/openshift/microshift) -- the lightweight OpenShift runtime that powers the cluster
- [OKD](https://www.okd.io/) -- the community distribution of Kubernetes that powers OpenShift
- [OpenShift Console](https://github.com/openshift/console) -- the web UI
