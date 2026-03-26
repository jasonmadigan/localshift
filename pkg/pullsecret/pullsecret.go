package pullsecret

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasonmadigan/oinc/pkg/runtime"
)

const (
	configSubdir = "oinc"
	filename     = "pull-secret.json"
	// where CRI-O looks for auth inside the container
	containerAuthPath = "/run/containers/0/auth.json"
)

// PullSecretURL is where users can obtain a Red Hat pull secret.
const PullSecretURL = "https://cloud.redhat.com/openshift/install/pull-secret"

// configDir returns the platform config dir for oinc
// (macOS: ~/Library/Application Support/oinc, Linux: ~/.config/oinc).
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, configSubdir), nil
}

// Path returns the full path to the stored pull secret.
func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// Exists returns true if a pull secret is configured.
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads the stored pull secret bytes.
func Load() ([]byte, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("no pull secret configured. get one from %s and run: oinc pull-secret set <path>", PullSecretURL)
	}
	return data, nil
}

// Save stores a pull secret from the given file path.
func Save(srcPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcPath, err)
	}

	// validate it's valid JSON with an "auths" key
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("pull secret is not valid JSON: %w", err)
	}
	if _, ok := parsed["auths"]; !ok {
		return fmt.Errorf("pull secret is missing 'auths' key")
	}

	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	dst, err := Path()
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// Remove deletes the stored pull secret.
func Remove() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasRegistry checks if the stored pull secret has credentials for a specific registry.
func HasRegistry(registry string) bool {
	data, err := Load()
	if err != nil {
		return false
	}
	var parsed struct {
		Auths map[string]any `json:"auths"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return false
	}
	_, ok := parsed.Auths[registry]
	return ok
}

// InjectIntoContainer copies the pull secret into the MicroShift container
// so CRI-O can use it for authenticated image pulls.
func InjectIntoContainer(rt *runtime.Runtime, container string) error {
	data, err := Load()
	if err != nil {
		return err
	}

	// write to a temp file, copy into container, clean up
	tmp, err := os.CreateTemp("", "oinc-pull-secret-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	// docker cp can't create intermediate dirs, so copy to /tmp then move
	tmpDst := "/tmp/oinc-pull-secret.json"
	if err := rt.CopyToContainer(container, tmp.Name(), tmpDst); err != nil {
		return fmt.Errorf("copying pull secret to container: %w", err)
	}

	if _, err := rt.ExecInContainer(container, "mkdir", "-p", filepath.Dir(containerAuthPath)); err != nil {
		return fmt.Errorf("creating auth dir in container: %w", err)
	}
	if _, err := rt.ExecInContainer(container, "mv", tmpDst, containerAuthPath); err != nil {
		return fmt.Errorf("moving pull secret into place: %w", err)
	}

	// tell CRI-O where to find it
	crioConf := fmt.Sprintf("[crio.image]\nglobal_auth_file = %q\n", containerAuthPath)
	if err := writeFileToContainer(rt, container, "/etc/crio/crio.conf.d/10-pull-secret.conf", []byte(crioConf)); err != nil {
		return fmt.Errorf("configuring CRI-O auth: %w", err)
	}

	// relax signature policy for registry.redhat.io so catalogue images can
	// be pulled. microshift enforces GPG signatures for RH registries which
	// blocks the operator index image in dev environments.
	fmt.Fprintln(os.Stderr, "warning: relaxing image signature policy for registry.redhat.io (dev environment)")
	policyJSON := `{
  "default": [{"type": "insecureAcceptAnything"}],
  "transports": {
    "docker": {
      "registry.redhat.io": [{"type": "insecureAcceptAnything"}],
      "registry.access.redhat.com": [{"type": "insecureAcceptAnything"}]
    },
    "docker-daemon": {
      "": [{"type": "insecureAcceptAnything"}]
    }
  }
}`
	if err := writeFileToContainer(rt, container, "/etc/containers/policy.json", []byte(policyJSON)); err != nil {
		return fmt.Errorf("updating signature policy: %w", err)
	}

	// restart CRI-O to pick up the config
	if _, err := rt.ExecInContainer(container, "systemctl", "restart", "crio"); err != nil {
		return fmt.Errorf("restarting CRI-O: %w", err)
	}

	return nil
}

// writeFileToContainer writes content to a file inside a container via
// docker/podman cp. avoids shell injection from sh -c echo patterns.
func writeFileToContainer(rt *runtime.Runtime, container, path string, content []byte) error {
	tmp, err := os.CreateTemp("", "oinc-container-file-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	return rt.CopyToContainer(container, tmp.Name(), path)
}
