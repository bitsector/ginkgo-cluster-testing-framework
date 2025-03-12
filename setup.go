package example

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Global variable for kubeconfig path
var KubeconfigPath string

func initKubeconfig() error {
	// Try to load .env file
	err := godotenv.Load(".env")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	// Get kubeconfig path from environment
	KubeconfigPath = os.Getenv("KUBECONFIG")

	// Fallback to default if not set
	if KubeconfigPath == "" {
		if os.IsNotExist(err) { // .env doesn't exist
			home := homedir.HomeDir()
			if home == "" {
				return fmt.Errorf("no home directory found")
			}
			KubeconfigPath = filepath.Join(home, ".kube", "config")
		} else { // .env exists but KUBECONFIG is empty
			panic(".env file format error, please use KUBECONFIG=/path/to/.kube/config")
		}
	}

	// Verify kubeconfig file exists
	if _, err := os.Stat(KubeconfigPath); err != nil {
		return fmt.Errorf("kubeconfig not found: %w (checked: %s)", err, KubeconfigPath)
	}

	return nil
}

func GetClient() (*kubernetes.Clientset, error) {
	if err := initKubeconfig(); err != nil {
		return nil, err
	}

	config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("config creation error: %w", err)
	}

	return kubernetes.NewForConfig(config)
}

func GetTopologyDeploymentTestFiles() ([]byte, []byte, error) {
	hpaPath := filepath.Join("topology_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	deploymentPath := filepath.Join("topology_test_deployment_yamls", "topology-dep.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, deploymentContent, nil
}
