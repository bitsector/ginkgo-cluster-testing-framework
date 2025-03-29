
FROM golang:1.24.1-bullseye AS builder

WORKDIR /app

RUN mkdir -p /app/temp && \
    chown -R 65534:65534 /app/temp && \
    chmod 755 /app/temp

# Copy go module files first (for caching)
COPY go.mod .
COPY go.sum .

# Install dependencies explicitly
RUN go mod download
RUN go get github.com/joho/godotenv

# Copy all Go files at root
COPY *.go ./

# Copy .env file explicitly
COPY .env .
RUN sed -i 's/ACCESS_MODE=KUBECONFIG/ACCESS_MODE=LOCAL_K8S_API/g' .env

# Explicitly copy all *test_yamls directories and their contents
COPY anti_affinity_test_deployment_yamls ./anti_affinity_test_deployment_yamls

# Allos non root user 65534 access all thefiles
RUN chown -R 65534:65534 . && \
    chmod -R 755 . && \
    find . -type f -exec chmod 644 {} \;


# Build binary (assuming you have setup.go/util.go as well)
# Build binary (compile all .go files explicitly)
RUN CGO_ENABLED=0 GOOS=linux go test -c -o cluster-tester \
    ./main_test.go\
    ./setup.go \
    ./util.go \
    ./anti_affinity_deployment_test.go 
    
FROM gcr.io/distroless/static-debian11:debug 

# Copy binary and manifests from builder stage explicitly
COPY --from=builder /app/cluster-tester /app/
COPY --from=builder /app/.env /app/
COPY --from=builder --chown=65534:65534 /app/temp /app/temp

# Explicitly copy each *test_yamls directory separately into container /app/ dir
COPY --from=builder /app/anti_affinity_test_deployment_yamls /app/anti_affinity_test_deployment_yamls

WORKDIR /app

USER 65534:65534

ENTRYPOINT ["sh", "-c", "./cluster-tester -test.v || true; sleep 19800"]
