package oinc

import (
	"fmt"
	"log/slog"

	"github.com/jasonmadigan/oinc/pkg/kubeconfig"
	"github.com/jasonmadigan/oinc/pkg/runtime"
)

func Kubeconfig(runtimeOverride string, printOnly bool, logger *slog.Logger) error {
	rt, err := runtime.Detect(runtimeOverride)
	if err != nil {
		return err
	}

	if !rt.ContainerRunning(containerName) {
		return fmt.Errorf("oinc cluster is not running")
	}

	kcPath := fmt.Sprintf("%s/%s/kubeconfig", kubeconfigDir, hostname)
	raw, err := rt.CopyFromContainer(containerName, kcPath)
	if err != nil {
		return fmt.Errorf("fetching kubeconfig: %w", err)
	}

	if printOnly {
		fmt.Print(string(raw))
		return nil
	}

	if err := kubeconfig.Update(raw); err != nil {
		return fmt.Errorf("updating kubeconfig: %w", err)
	}

	logger.Info("kubeconfig merged", "path", kubeconfig.Path())
	return nil
}
