package oinc

import (
	"log/slog"

	"github.com/jasonmadigan/oinc/pkg/kubeconfig"
	"github.com/jasonmadigan/oinc/pkg/runtime"
	"github.com/jasonmadigan/oinc/pkg/tui"
)

func DeleteSteps(runtimeOverride string) ([]*tui.Step, error) {
	rt, err := runtime.Detect(runtimeOverride)
	if err != nil {
		return nil, err
	}

	return []*tui.Step{
		{Name: "removing cluster container", Run: func() error { return rt.RemoveContainer(containerName) }},
		{Name: "removing console container", Run: func() error { return rt.RemoveContainer(consoleContainer) }},
		{Name: "cleaning kubeconfig", Run: func() error { return kubeconfig.Remove() }},
	}, nil
}

func Delete(runtimeOverride string, logger *slog.Logger) error {
	steps, err := DeleteSteps(runtimeOverride)
	if err != nil {
		return err
	}

	if tui.IsTTY() {
		return tui.RunSteps("deleting cluster", steps)
	}

	// plain fallback
	rt, err := runtime.Detect(runtimeOverride)
	if err != nil {
		return err
	}

	logger.Info("removing container", "name", containerName)
	if err := rt.RemoveContainer(containerName); err != nil {
		logger.Warn("failed to remove container", "err", err)
	}

	logger.Info("removing console container", "name", consoleContainer)
	if err := rt.RemoveContainer(consoleContainer); err != nil {
		logger.Warn("failed to remove console container", "err", err)
	}

	if err := kubeconfig.Remove(); err != nil {
		logger.Warn("failed to clean kubeconfig", "err", err)
	}

	logger.Info("cluster deleted")
	return nil
}
