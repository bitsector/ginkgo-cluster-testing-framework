# Golang Ginkgo E2E cluster-tester

### Go version
go version 1.24.1


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
ACCESS_MODE=KUBECONFIG, LOCAL_K8S_API or EXTERNAL_K8S_API
ALLOWED_TO_FAIL=StatefulSetPDBTest,DeploymentPDBTest # all tags are listed in .env
```

### Make sure the nodes are in seperate regions
```bash
kubectl get nodes -o custom-columns='NAME:.metadata.name,ZONE:.metadata.labels.topology\.kubernetes\.io/zone'
```

### Run tests

### Simple connectivity test (make sure you connect to the cluster):
```bash
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Basic cluster connectivity test" ./...

```

### Deployment tests
```bash
go test -v -ginkgo.label-filter=safe-in-production -ginkgo.focus="Deployment Anti Affinity E2E test" ./...
```

## Documentation - The test cases and how they work:

### Connectivity Test
A basic connectivity test. Will attempt to connect to the cluster, list nodes, create a namespace and finish.
Files: 
- simple_connectivity_test.go

 
### Deployment Anti Affinity E2E test
The test will deploy a zone-marker pod (placed a random zone by K8s), deploy an HPA, and a dependent-app deployment with a pod anti affinity 
requirement (podAntiAffinity). The goal of the test is to trigger the deployment to create more pods and
assert that all these pods satisfy the anti affinity requirement, relative to the zone-marker pod. The deployment's first pod will start running,
simulate high CPU demand, this will trigger the HPA to create more of the deployment's pods. The test code will then verify that all 
the pods are placed outside the zone of the zone-marker pod. The test will fail if and only if this condition is not met.  
Files: 
- anti_affinity_deployment_test.go
- anti_affinity_test_deployment_yamls/anti-affinity-dependent-app.yaml 
- anti_affinity_test_deployment_yamls/hpa-trigger.yaml 
- anti_affinity_test_deployment_yamls/zone-marker.yaml

