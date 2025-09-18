.PHONY: build clean test docker-build docker-push deploy undeploy lint

# Variables
APP_NAME = 5glos
VERSION ?= latest
DOCKER_REGISTRY ?= localhost:5000
DOCKER_IMAGE = $(DOCKER_REGISTRY)/$(APP_NAME):$(VERSION)
NAMESPACE = free5gc

# Build the application
build:
    go build -o bin/$(APP_NAME) ./cmd

# Clean build artifacts
clean:
    rm -rf bin/

# Run tests
test:
    go test -v ./...

# Run linter
lint:
    golangci-lint run

# Build Docker image
docker-build:
    docker build -t $(DOCKER_IMAGE) .

# Push Docker image
docker-push: docker-build
    docker push $(DOCKER_IMAGE)

# Deploy to Kubernetes
deploy:
    kubectl apply -f deployments/k8s/

# Remove from Kubernetes
undeploy:
    kubectl delete -f deployments/k8s/

# Build and deploy
all: docker-build docker-push deploy

# Development
dev-run:
    go run ./cmd -config config.yaml

# Generate mocks (if using gomock)
generate:
    go generate ./...

# Install dependencies
deps:
    go mod tidy
    go mod download

# Format code
fmt:
    go fmt ./...

# Vet code
vet:
    go vet ./...

help:
    @echo "Available targets:"
    @echo "  build       - Build the application"
    @echo "  clean       - Clean build artifacts"
    @echo "  test        - Run tests"
    @echo "  lint        - Run linter"
    @echo "  docker-build- Build Docker image"
    @echo "  docker-push - Push Docker image"
    @echo "  deploy      - Deploy to Kubernetes"
    @echo "  undeploy    - Remove from Kubernetes"
    @echo "  all         - Build, push and deploy"
    @echo "  dev-run     - Run locally for development"
    @echo "  help        - Show this help"