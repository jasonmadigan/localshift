package kubeconfig

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const clusterName = "oinc"

func Path() string {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		return p
	}
	return clientcmd.RecommendedHomeFile
}

// Read returns the raw kubeconfig bytes from the user's kubeconfig file.
func Read() ([]byte, error) {
	return os.ReadFile(Path())
}

// Update merges the given kubeconfig bytes into the user's kubeconfig file.
// Cluster, context, and user are all renamed to "oinc" regardless of what
// MicroShift generates internally.
func Update(raw []byte) error {
	existing, err := clientcmd.LoadFromFile(Path())
	if err != nil {
		existing = api.NewConfig()
	}

	incoming, err := clientcmd.Load(raw)
	if err != nil {
		return fmt.Errorf("parsing kubeconfig: %w", err)
	}

	// take the first cluster/context/user from the incoming config and rename to "oinc"
	for _, cluster := range incoming.Clusters {
		existing.Clusters[clusterName] = cluster
		break
	}
	userName := clusterName + "-admin"
	for _, auth := range incoming.AuthInfos {
		existing.AuthInfos[userName] = auth
		break
	}
	existing.Contexts[clusterName] = &api.Context{
		Cluster:  clusterName,
		AuthInfo: userName,
	}
	existing.CurrentContext = clusterName

	return clientcmd.WriteToFile(*existing, Path())
}

// Remove deletes the oinc cluster/context/user from the kubeconfig.
func Remove() error {
	config, err := clientcmd.LoadFromFile(Path())
	if err != nil {
		return nil
	}

	if _, exists := config.Clusters[clusterName]; !exists {
		return nil
	}

	delete(config.Clusters, clusterName)

	for ctxName, ctx := range config.Contexts {
		if ctx.Cluster == clusterName {
			if config.CurrentContext == ctxName {
				config.CurrentContext = ""
			}
			delete(config.AuthInfos, ctx.AuthInfo)
			delete(config.Contexts, ctxName)
		}
	}

	return clientcmd.WriteToFile(*config, Path())
}
