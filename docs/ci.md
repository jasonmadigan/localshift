# CI / CD

## Workflows

### Image builds (`.github/workflows/images.yml`)

Builds MicroShift container images. **Manual dispatch only** (`workflow_dispatch`).

- Triggered manually from GitHub Actions UI
- Optional `version` input to build a single OCP version (blank = all)
- Matrix: version x arch (currently 4.20 + 4.21, each with amd64 + arm64 = 4 jobs)
- ARM builds run on `ubuntu-24.04-arm` runners (native, no emulation)
- Uses podman for builds (not docker)
- Pushes to `ghcr.io/jasonmadigan/oinc`

### CLI releases (`.github/workflows/release.yml`)

Builds and releases CLI binaries. **Triggered by pushing a `v*` tag.**

```bash
git tag v0.1.0
git push origin v0.1.0
```

- Cross-compiles for darwin/linux x amd64/arm64 (4 binaries)
- Injects version via `-ldflags "-X main.buildVersion=${VERSION}"`
- Creates GitHub release with auto-generated notes
- Binaries named `oinc-<os>-<arch>` (e.g. `oinc-darwin-arm64`)

## Permissions

- Image workflow needs `packages: write` to push to GHCR
- Release workflow needs `contents: write` to create releases
- Both use `${{ github.token }}` (no external secrets)

## Runner architecture

ARM images must be built on native ARM runners (`ubuntu-24.04-arm`). Cross-compilation via QEMU is too slow and unreliable for full OS image builds.

The CLI cross-compiles fine on any runner (pure Go, no CGO).
