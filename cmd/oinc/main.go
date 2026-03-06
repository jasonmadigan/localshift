package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jasonmadigan/oinc/pkg/addons"
	"github.com/jasonmadigan/oinc/pkg/kubeconfig"
	"github.com/jasonmadigan/oinc/pkg/oinc"
	"github.com/jasonmadigan/oinc/pkg/runtime"
	"github.com/jasonmadigan/oinc/pkg/version"
	"github.com/spf13/cobra"
)

var (
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

var (
	flagRuntime        string
	flagVersion        string
	flagHTTPPort       int
	flagHTTPSPort      int
	flagConsolePort    int
	flagConsPlugin     string
	flagAddons         string
	flagLogLevel       string
	flagOutput         string
	flagKubeconfigPrint bool
)

var buildVersion = "dev"

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}

func main() {
	root := &cobra.Command{
		Use:   "oinc",
		Short: "OKD in a container",
		Long:  oinc.Pig("oinc ~ OKD in a container"),
	}

	root.PersistentFlags().StringVar(&flagRuntime, "runtime", "", "container runtime (auto-detected if empty)")
	root.PersistentFlags().StringVarP(&flagLogLevel, "log-level", "l", "info", "log level (debug, info, warn, error)")

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(flagLogLevel)
			return oinc.Create(oinc.CreateOpts{
				Version:         flagVersion,
				RuntimeOverride: flagRuntime,
				HTTPPort:        flagHTTPPort,
				HTTPSPort:       flagHTTPSPort,
				ConsolePort:     flagConsolePort,
				ConsolePlugin:   flagConsPlugin,
				Addons:          flagAddons,
			}, logger)
		},
	}
	createCmd.Flags().StringVar(&flagVersion, "version", "", "OCP version (default: latest)")
	createCmd.Flags().IntVar(&flagHTTPPort, "http-port", 9080, "HTTP route port")
	createCmd.Flags().IntVar(&flagHTTPSPort, "https-port", 9443, "HTTPS route port")
	createCmd.Flags().IntVar(&flagConsolePort, "console-port", 9000, "console port")
	createCmd.Flags().StringVar(&flagConsPlugin, "console-plugin", "", "console plugin wiring (name=url)")
	createCmd.Flags().StringVar(&flagAddons, "addons", "", "comma-separated addons to install")

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete the cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(flagLogLevel)
			return oinc.Delete(flagRuntime, logger)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show cluster status",
		RunE: func(cmd *cobra.Command, args []string) error {
			s := oinc.GetStatus(flagRuntime)
			if flagOutput == "json" {
				out, _ := json.MarshalIndent(s, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Print(s.Render())
			return nil
		},
	}
	statusCmd.Flags().StringVarP(&flagOutput, "output", "o", "", "output format (json)")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show oinc version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(oinc.Pig("oinc " + buildVersion))
		},
	}

	versionListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available OCP versions",
		Run: func(cmd *cobra.Command, args []string) {
			def := version.Default()
			for _, v := range version.All() {
				marker := ""
				if v.Version == def.Version {
					marker = "  " + green.Render("[default]")
				}
				fmt.Printf("  %-6s %s%s\n",
					v.Version, dim.Render(fmt.Sprintf("microshift: %s, console: %s", v.MicroShiftTag, v.ConsoleTag)), marker)
			}
		},
	}
	versionCmd.AddCommand(versionListCmd)

	switchCmd := &cobra.Command{
		Use:   "switch <version>",
		Short: "Switch to a different OCP version (delete + create)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(flagLogLevel)
			logger.Info("switching version", "version", args[0])
			if err := oinc.Delete(flagRuntime, logger); err != nil {
				logger.Warn("delete failed, continuing", "err", err)
			}
			return oinc.Create(oinc.CreateOpts{
				Version:         args[0],
				RuntimeOverride: flagRuntime,
				HTTPPort:        flagHTTPPort,
				HTTPSPort:       flagHTTPSPort,
				ConsolePort:     flagConsolePort,
				ConsolePlugin:   flagConsPlugin,
			}, logger)
		},
	}
	switchCmd.Flags().IntVar(&flagHTTPPort, "http-port", 9080, "HTTP route port")
	switchCmd.Flags().IntVar(&flagHTTPSPort, "https-port", 9443, "HTTPS route port")
	switchCmd.Flags().IntVar(&flagConsolePort, "console-port", 9000, "console port")
	switchCmd.Flags().StringVar(&flagConsPlugin, "console-plugin", "", "console plugin wiring (name=url)")

	addonCmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage addons",
	}

	addonListCmd := &cobra.Command{
		Use:   "list",
		Short: "List available addons",
		Run: func(cmd *cobra.Command, args []string) {
			all := addons.All()
			names := make([]string, 0, len(all))
			for name := range all {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				a := all[name]
				deps := a.Dependencies()
				if len(deps) > 0 {
					fmt.Printf("  %-16s %s\n", name, dim.Render("requires: "+strings.Join(deps, ", ")))
				} else {
					fmt.Printf("  %s\n", name)
				}
			}
		},
	}

	addonInstallCmd := &cobra.Command{
		Use:   "install <addon>[,<addon>...]",
		Short: "Install addons into a running cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(flagLogLevel)

			rt, err := runtime.Detect(flagRuntime)
			if err != nil {
				return err
			}

			kc, err := kubeconfig.Read()
			if err != nil {
				return fmt.Errorf("reading kubeconfig: %w", err)
			}

			return oinc.InstallAddons(args[0], kc, rt, logger)
		},
	}

	addonCmd.AddCommand(addonListCmd, addonInstallCmd)

	kubeconfigCmd := &cobra.Command{
		Use:   "kubeconfig",
		Short: "Fetch cluster kubeconfig",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := newLogger(flagLogLevel)
			return oinc.Kubeconfig(flagRuntime, flagKubeconfigPrint, logger)
		},
	}
	kubeconfigCmd.Flags().BoolVarP(&flagKubeconfigPrint, "print", "p", false, "print raw kubeconfig to stdout")

	root.AddCommand(createCmd, deleteCmd, statusCmd, versionCmd, switchCmd, addonCmd, kubeconfigCmd)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
