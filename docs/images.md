# Image builds

## Registry

Images published at `ghcr.io/jasonmadigan/oinc`.

Tags follow the pattern: `<okd-version>-<arch>`, e.g. `4.21.0-okd-scos.ec.15-arm64`.

## Source

Images are built from pre-built RPMs published by [microshift-io/microshift](https://github.com/microshift-io/microshift/releases). No SRPM compilation, no Red Hat pull secrets needed.

## Base image

`quay.io/centos/centos:stream9`

## Build args

| Arg | Description | Example |
|-|-|-|
| `RPM_TARBALL` | filename of the RPM tarball | `microshift-rpms-x86_64.tgz` |
| `OCP_VERSION` | OCP version for openshift deps mirror URL | `4.21` |
| `WITH_OLM` | set to `1` to install OLM packages | `1` |

## What the image contains

1. **MicroShift** -- `microshift`, `microshift-release-info`, `microshift-kindnet`, `microshift-kindnet-release-info`
2. **OLM** (when `WITH_OLM=1`) -- `microshift-olm`, `microshift-olm-release-info`
3. **CNI plugins** -- downloaded from `containernetworking/plugins` (v1.8.0), required by kindnet
4. **Firewall rules** -- trusted zone for pod CIDR (10.42.0.0/16) and link-local (169.254.169.1), public zone for API (6443) and etcd (2379/2380)
5. **DNS config** -- base domain set to `127.0.0.1.nip.io`

The OpenShift dependencies RPM mirror (`mirror.openshift.com`) provides packages needed by MicroShift at install time. This repo is removed after install.

## Build workflow

`.github/workflows/images.yml` -- manual dispatch via `workflow_dispatch`.

Matrix builds all version/arch combinations in parallel. Each job:
1. Downloads RPM tarball from microshift-io release (via `gh release download`)
2. Builds image with podman on native arch runner (amd64 on `ubuntu-24.04`, arm64 on `ubuntu-24.04-arm`)
3. Pushes to GHCR

Optional `version` input filters to a single OCP version.

## Adding a new version

1. Find the release tag on [microshift-io/microshift](https://github.com/microshift-io/microshift/releases):
   ```
   gh release list -R microshift-io/microshift
   ```

2. Add matrix entries in `.github/workflows/images.yml` for both `amd64` and `arm64`

3. Add catalogue entry in `pkg/version/version.go`:
   ```go
   {
       Version:       "4.22",
       MicroShiftTag: "4.22.0-okd-scos.1",
       ConsoleTag:    "4.22",
       APIBranch:     "release-4.22",
       Arches:        []string{"amd64", "arm64"},
   },
   ```

4. Run the image workflow, then rebuild the CLI
