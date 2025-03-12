go version 1.24

### Installation
```bash
go get ./...
go mod tidy
go install github.com/onsi/ginkgo/v2/ginkgo@latest
go get github.com/joho/godotenv
```
### Set the path to your local kube config in .env file
```bash
KUBECONFIG=/path/to/.kube/config
```

### Run tests

### Simple connectivity test (make sure you connect to the cluster):
```bash
go test -v ./simple_connectivity_test.go -ginkgo.focus "Basic cluster connectivity test"
```

### Deployment tests
```bash
go test -v ./topology_constraint_deployment_test.go -ginkgo.focus "Deployment Topology E2E test"
```