package example

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/onsi/ginkgo/v2"
	"github.com/rs/zerolog"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var Logger zerolog.Logger
var LogBuffer *bytes.Buffer
var KubeconfigPath string
var AllowedToFailTags []string

func parseAllowedToFailTags() error {
	err := godotenv.Load(".env")
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	allowedToFailStr := os.Getenv("ALLOWED_TO_FAIL")
	if allowedToFailStr != "" {
		AllowedToFailTags = strings.Split(allowedToFailStr, ",")
		for i, tag := range AllowedToFailTags {
			AllowedToFailTags[i] = strings.TrimSpace(tag)
		}
	}

	return nil
}

func init() {
	LogBuffer = new(bytes.Buffer)
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    true,
		TimeFormat: "2006-01-02 15:04:05",
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.MessageFieldName,
		},
	}

	// Simplify field formatting
	consoleWriter.FormatFieldName = func(i interface{}) string { return "" }
	consoleWriter.FormatFieldValue = func(i interface{}) string { return "" }

	// Create a multi-writer to write to both stdout and LogBuffer
	multiWriter := zerolog.MultiLevelWriter(consoleWriter, LogBuffer)

	Logger = zerolog.New(multiWriter).
		With().
		Timestamp().
		Logger()

	if err := parseAllowedToFailTags(); err != nil {
		fmt.Printf("Warning: Failed to parse ALLOWED_TO_FAIL tags: %v", err)
	}
}

func GetLogger(tag string) zerolog.Logger {
	return Logger.With().Str("tag", tag).Logger()
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func IsTestAllowedToFail(testTag string) bool {
	return contains(AllowedToFailTags, testTag)
}

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

func getLocalClusterAPICreds() (*rest.Config, error) {
	// In-cluster configuration (auto-mounted)
	tokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	caPath := "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed reading token: %w", err)
	}

	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed reading CA cert: %w", err)
	}

	return &rest.Config{
		Host:        "https://kubernetes.default.svc",
		BearerToken: string(token),
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCert,
		},
	}, nil
}

func getExternalClusterAPICreds() (*rest.Config, error) {
	apiURL := os.Getenv("K8S_API_URL")
	if apiURL == "" {
		return nil, fmt.Errorf("K8S_API_URL environment variable not set")
	}

	token := os.Getenv("K8S_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("K8S_TOKEN environment variable not set")
	}

	caCert := os.Getenv("K8S_CA_CERT")
	if caCert == "" {
		return nil, fmt.Errorf("K8S_CA_CERT environment variable not set")
	}

	// Process escaped newlines in CA certificate
	caCert = strings.ReplaceAll(caCert, "\\n", "\n")

	caCertBytes, err := base64.StdEncoding.DecodeString(caCert)
	if err != nil {
		return nil, fmt.Errorf("CA cert decoding failed: %w", err)
	}

	return &rest.Config{
		Host:        apiURL,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCertBytes,
		},
	}, nil
}

func GetClient() (*kubernetes.Clientset, error) {
	// Load .env to get ACCESS_MODE
	logger := GetLogger("Setup")
	err := godotenv.Load(".env")
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	accessMode := os.Getenv("ACCESS_MODE")
	switch accessMode {
	case "KUBECONFIG":
		if err := initKubeconfig(); err != nil {
			return nil, err
		}

		config, err := clientcmd.BuildConfigFromFlags("", KubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("config creation error: %w", err)
		}
		logger.Info().Msgf("Running test with access mode KUBECONFIG")
		return kubernetes.NewForConfig(config)

	case "EXTERNAL_K8S_API":
		config, err := getExternalClusterAPICreds()
		if err != nil {
			return nil, fmt.Errorf("API credentials error: %w", err)
		}
		logger.Info().Msgf("Running test with access mode EXTERNAL_K8S_API")
		return kubernetes.NewForConfig(config)

	case "LOCAL_K8S_API":
		config, err := getLocalClusterAPICreds()
		if err != nil {
			return nil, fmt.Errorf("API credentials error: %w", err)
		}
		logger.Info().Msgf("Running test with access mode LOCAL_K8S_API")
		return kubernetes.NewForConfig(config)

	default:
		logger.Info().Msgf("Invalid .env ACCESS_MODE: %s. Must be KUBECONFIG, LOCAL_K8S_API or EXTERNAL_K8S_API\n", accessMode)
		os.Exit(1)
		return nil, fmt.Errorf(".env invalid access mode") // For compiler satisfaction
	}
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

func GetAffinityDeploymentTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("affinity_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("affinity_test_deployment_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("affinity_test_deployment_yamls", "affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}

func GetAntiAffinityTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("anti_affinity_test_deployment_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("anti_affinity_test_deployment_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	deploymentPath := filepath.Join("anti_affinity_test_deployment_yamls", "anti-affinity-dependent-app.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("anti-affinity-dependent deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	return hpaContent, zoneContent, deploymentContent, nil
}

func GetPDBDeploymentTestFiles() ([]byte, []byte, error) {
	deploymentPath := filepath.Join("pdb_deployment_test_yamls", "deployment.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("deployment file error: %w (checked: %s)", err, deploymentPath)
	}

	pdbPath := filepath.Join("pdb_deployment_test_yamls", "pdb.yaml")
	pdbContent, err := os.ReadFile(pdbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("PDB file error: %w (checked: %s)", err, pdbPath)
	}

	return pdbContent, deploymentContent, nil
}

func GetRollingUpdateDeploymentTestFiles() ([]byte, error) {
	startPath := filepath.Join("rolling_update_deployment_test_yamls", "deployment_start.yaml")
	startContent, err := os.ReadFile(startPath)
	if err != nil {
		return nil, fmt.Errorf("deployment start file error: %w (checked: %s)", err, startPath)
	}

	return startContent, nil
}

func GetAffinityStatefulSetTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("affinity_test_statefulset_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("affinity_test_statefulset_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	statefulSetPath := filepath.Join("affinity_test_statefulset_yamls", "affinity-dependent-app.yaml")
	statefulSetContent, err := os.ReadFile(statefulSetPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("affinity-dependent StatefulSet file error: %w (checked: %s)", err, statefulSetPath)
	}

	return hpaContent, zoneContent, statefulSetContent, nil
}

func GetAntiAffinityStatefulSetTestFiles() ([]byte, []byte, []byte, error) {
	hpaPath := filepath.Join("anti_affinity_statefulset_test_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("HPA trigger file error: %w (checked: %s)", err, hpaPath)
	}

	zonePath := filepath.Join("anti_affinity_statefulset_test_yamls", "zone-marker.yaml")
	zoneContent, err := os.ReadFile(zonePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("zone marker file error: %w (checked: %s)", err, zonePath)
	}

	statefulSetPath := filepath.Join("anti_affinity_statefulset_test_yamls", "anti-affinity-dependent-app.yaml")
	statefulSetContent, err := os.ReadFile(statefulSetPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("anti-affinity-dependent StatefulSet file error: %w (checked: %s)", err, statefulSetPath)
	}

	return hpaContent, zoneContent, statefulSetContent, nil
}

func GetStatefulSetTestFiles() ([]byte, []byte, error) {
	hpaPath := filepath.Join("topology_test_statefulset_yamls", "hpa-trigger.yaml")
	hpaContent, err := os.ReadFile(hpaPath)
	if err != nil {
		return nil, nil, fmt.Errorf("HPA file error: %w (checked: %s)", err, hpaPath)
	}

	statefulsetPath := filepath.Join("topology_test_statefulset_yamls", "topology-statefulset.yaml")
	statefulsetContent, err := os.ReadFile(statefulsetPath)
	if err != nil {
		return nil, nil, fmt.Errorf("StatefulSet file error: %w (checked: %s)", err, statefulsetPath)
	}

	return hpaContent, statefulsetContent, nil
}

func GetPDBStSTestFiles() ([]byte, []byte, error) {
	pdbPath := filepath.Join("pdb_statefulset_test_yamls", "pdb.yaml")
	pdbContent, err := os.ReadFile(pdbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("PDB file error: %w (checked: %s)", err, pdbPath)
	}

	stsPath := filepath.Join("pdb_statefulset_test_yamls", "sts.yaml")
	stsContent, err := os.ReadFile(stsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("StatefulSet file error: %w (checked: %s)", err, stsPath)
	}

	return pdbContent, stsContent, nil
}

func GetRollingUpdateStatefulSetTestFiles() ([]byte, error) {
	startPath := filepath.Join("rolling_update_sts_yamls", "sts_start.yaml")
	startContent, err := os.ReadFile(startPath)
	if err != nil {
		return nil, fmt.Errorf("statefulset start file error: %w (checked: %s)", err, startPath)
	}

	return startContent, nil
}

type FinalReport struct {
	TestTimestamp       string                              `json:"test_timestamp"`
	FailingTests        []string                            `json:"failing_tests"`
	SucceedingTests     []string                            `json:"succeeding_tests"`
	AllowedToFailTests  []string                            `json:"allowed_to_fail_tests"`
	FailedButNotAllowed []string                            `json:"failed_but_not_allowed_to_fail"`
	SuccessRatio        string                              `json:"success_ratio"`
	LogsByTags          map[string][]map[string]interface{} `json:"logs_by_tags"`
}

var _ = ginkgo.ReportAfterSuite("Test Suite Summary", func(report ginkgo.Report) {
	logger := GetLogger("FinalReportAfterSuite")

	dir := "./temp"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		logger.Error().Msgf("Error: Directory %s does not exist", dir)
		return
	}

	filename := filepath.Join(dir, fmt.Sprintf("test_suite_log_%s.json",
		time.Now().Format("20060102-150405")))

	lines := bytes.Split(LogBuffer.Bytes(), []byte("\n"))
	logsByTags := make(map[string][]map[string]interface{})
	failingTests := []string{}
	succeedingTests := []string{}
	allowedToFailTests := []string{}
	failedButNotAllowedToFail := []string{}
	allTags := make(map[string]bool)

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var logEntry map[string]interface{}
		if err := json.Unmarshal(line, &logEntry); err != nil {
			continue
		}

		if tagValue, ok := logEntry["tag"].(string); ok && tagValue != "Setup" {
			allTags[tagValue] = true

			if msg, ok := logEntry["message"].(string); ok && strings.Contains(msg, "TEST_FAILED") {
				failingTests = append(failingTests, tagValue)
				if contains(AllowedToFailTags, tagValue) {
					allowedToFailTests = append(allowedToFailTests, tagValue)
				} else {
					failedButNotAllowedToFail = append(failedButNotAllowedToFail, tagValue)
				}
			}

			delete(logEntry, "tag")
			delete(logEntry, "level")
			logsByTags[tagValue] = append(logsByTags[tagValue], logEntry)
		}
	}

	for tag := range allTags {
		if !contains(failingTests, tag) {
			succeedingTests = append(succeedingTests, tag)
		}
	}

	totalTests := len(failingTests) + len(succeedingTests)
	successRatio := float64(len(succeedingTests)) / float64(totalTests) * 100

	// Replace map with struct instance
	finalJSON := FinalReport{
		TestTimestamp:       time.Now().Format("01/02/2006 15:04:05"),
		FailingTests:        failingTests,
		SucceedingTests:     succeedingTests,
		AllowedToFailTests:  allowedToFailTests,
		FailedButNotAllowed: failedButNotAllowedToFail,
		SuccessRatio:        fmt.Sprintf("%.2f%%", successRatio),
		LogsByTags:          logsByTags,
	}

	jsonData, err := json.MarshalIndent(finalJSON, "", " ")
	if err != nil {
		logger.Error().Err(err).Msg("Failed to serialize logs to JSON")
		return
	}

	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		logger.Error().Err(err).Msg("Failed to write test suite log file")
	} else {
		logger.Info().Str("file", filename).Msg("Test suite log written successfully")
	}

	if totalTests > 2 { // if running single test  - Setup + The specific single tests - don't print this
		fmt.Printf("\n=== Test Suite Summary ===\n")
		fmt.Printf("Failing Tests (%d):\n", len(failingTests))
		for _, test := range failingTests {
			fmt.Printf("- %s\n", test)
		}
		fmt.Printf("\nSucceeding Tests (%d):\n", len(succeedingTests))
		for _, test := range succeedingTests {
			fmt.Printf("- %s\n", test)
		}
		fmt.Printf("\nAllowed to Fail Tests (%d):\n", len(allowedToFailTests))
		for _, test := range allowedToFailTests {
			fmt.Printf("- %s\n", test)
		}
		fmt.Printf("\nFailed but Not Allowed to Fail Tests (%d):\n", len(failedButNotAllowedToFail))
		for _, test := range failedButNotAllowedToFail {
			fmt.Printf("- %s\n", test)
		}
		fmt.Printf("\nSuccess Ratio: %.2f%%\n", successRatio)
	}
})
