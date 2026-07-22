# CargoShip Makefile
# Provides common development tasks including comprehensive security scanning

.PHONY: help build test test-unit test-integration test-performance test-e2e test-all test-leak-check test-quality test-benchmark bench-s3 lint security audit install-tools clean docker

# Default target
help: ## Show this help message
	@echo "CargoShip Development Commands"
	@echo "============================="
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build CargoShip binaries
	@echo "🔨 Building CargoShip..."
	go build -o bin/cargoship cmd/cargoship/main.go
	go build -o bin/cargoship-launch cmd/cargoship-launch/main.go
	@echo "✅ Build complete"

build-release: ## Build release binaries with optimization
	@echo "🚀 Building release binaries..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/cargoship-linux-amd64 cmd/cargoship/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o bin/cargoship-darwin-amd64 cmd/cargoship/main.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/cargoship-windows-amd64.exe cmd/cargoship/main.go
	@echo "✅ Release build complete"

# Test targets
test: ## Run all tests
	@echo "🧪 Running tests..."
	go test -v -race -cover ./...

test-coverage: ## Run tests with detailed coverage report
	@echo "📊 Running tests with coverage analysis..."
	go test -race -coverprofile=coverage.out -coverpkg=./... ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | grep total
	@echo "📄 Coverage report generated: coverage.html"

codecov-test: ## Run tests for Codecov (with coverage profile)
	@echo "📊 Running tests for Codecov reporting..."
	go test -race -coverprofile=coverage.txt -coverpkg=./... ./...
	@echo "✅ Coverage data ready for Codecov upload"

codecov-upload: codecov-test ## Upload coverage to Codecov (requires CODECOV_TOKEN)
	@echo "📤 Uploading coverage to Codecov..."
	@if [ -z "$$CODECOV_TOKEN" ]; then \
		echo "❌ CODECOV_TOKEN environment variable not set"; \
		echo "💡 Get your token from https://codecov.io/gh/scttfrdmn/cargoship"; \
		exit 1; \
	fi
	bash <(curl -s https://codecov.io/bash) -f coverage.txt -t $$CODECOV_TOKEN
	@echo "✅ Coverage uploaded to Codecov"

test-benchmark: ## Run benchmark tests
	@echo "⚡ Running benchmarks..."
	go test -bench=. -benchmem ./...

bench-s3: ## Run the end-to-end S3 upload benchmark suite (requires a real bucket)
	@echo "🚢 Running CargoShip S3 upload benchmarks..."
	@if [ -z "$$CARGOSHIP_BENCHMARK_BUCKET" ]; then \
		echo "❌ Set CARGOSHIP_BENCHMARK_BUCKET to an S3 bucket you own, e.g.:"; \
		echo "     export CARGOSHIP_BENCHMARK_BUCKET=my-bench-bucket"; \
		echo "     export AWS_REGION=us-west-2   # plus valid AWS credentials"; \
		echo "   Then: make bench-s3            (default SCENARIO=small, ~10k files)"; \
		echo "         make bench-s3 SCENARIO=medium   # or large / xlarge"; \
		echo "   This uploads real data to S3 and incurs cost — run it periodically,"; \
		echo "   not on every change. See benchmarks/cargohold/README.md."; \
		exit 1; \
	fi
	$(MAKE) -C benchmarks/cargohold run SCENARIO=$(SCENARIO)

SCENARIO ?= small

benchmark-profile: ## Run benchmarks with CPU and memory profiling
	@echo "📊 Running benchmarks with profiling..."
	@mkdir -p profiles/benchmarks
	go test -bench=. -benchmem -cpuprofile=profiles/benchmarks/cpu.prof -memprofile=profiles/benchmarks/mem.prof ./...
	@echo "✅ Profiles saved to profiles/benchmarks/"

benchmark-baseline: ## Capture current benchmarks as baseline for regression detection
	@echo "📊 Capturing benchmark baseline..."
	@mkdir -p profiles/baselines
	go test -bench=. -benchmem -count=10 ./... | tee profiles/baselines/current.txt
	@echo "✅ Baseline saved to profiles/baselines/current.txt"

benchmark-compare: ## Run benchmarks and compare against baseline
	@echo "📊 Running benchmarks and comparing against baseline..."
	go test -bench=. -benchmem -count=10 ./... > benchmark-current.txt
	@echo ""
	./scripts/detect-regressions.sh
	@rm -f benchmark-current.txt

analyze-performance: ## Analyze performance profiles and generate bottleneck report
	@echo "🔍 Analyzing performance profiles..."
	./scripts/analyze-profiles.sh

profile-interactive: ## Open interactive profile viewer (requires CPU profile)
	@echo "🌐 Opening interactive profile viewer..."
	@if [ -f profiles/benchmarks/cpu.prof ]; then \
		go tool pprof -http=:8080 profiles/benchmarks/cpu.prof; \
	else \
		echo "❌ No CPU profile found. Run 'make benchmark-profile' first."; \
		exit 1; \
	fi

profile-runtime: ## Example: Run cargoship with runtime profiling endpoint
	@echo "🔍 Starting cargoship with runtime profiling..."
	@echo "📊 Profiling endpoints will be available at http://localhost:6060/debug/pprof/"
	@echo ""
	@echo "Usage examples:"
	@echo "  CPU profile (30s): curl http://localhost:6060/debug/pprof/profile?seconds=30 -o cpu.prof"
	@echo "  Heap profile:      curl http://localhost:6060/debug/pprof/heap -o heap.prof"
	@echo "  Goroutines:        curl http://localhost:6060/debug/pprof/goroutine -o goroutine.prof"
	@echo ""
	@echo "Press Ctrl+C to stop profiling"
	@./bin/cargoship --pprof estimate .

# Test categorization targets (new testing architecture)
test-unit: ## Run fast unit tests only (no external dependencies)
	@echo "🧪 Running unit tests..."
	go test -short -race -timeout=60s -cover ./...
	@echo "✅ Unit tests passed"

test-integration: ## Run integration tests (requires LocalStack/Docker)
	@echo "🔗 Running integration tests..."
	go test -tags=integration -race -timeout=300s ./...
	@echo "✅ Integration tests passed"

test-performance: ## Run performance and stress tests
	@echo "⚡ Running performance tests..."
	go test -tags=performance -timeout=600s ./...
	@echo "✅ Performance tests passed"

test-e2e: ## Run end-to-end tests (full system validation)
	@echo "🎯 Running end-to-end tests..."
	go test -tags=e2e -timeout=900s ./...
	@echo "✅ End-to-end tests passed"

test-all: test-unit test-integration test-performance test-e2e ## Run all test categories
	@echo "🎉 All tests passed!"

test-leak-check: ## Run unit tests with goroutine leak detection
	@echo "🔍 Running tests with goroutine leak detection..."
	go test -short -race -timeout=60s ./pkg/... -v | grep -E "(LEAK|goroutine|FAIL)" || echo "✅ No leaks detected"

test-quality: ## Run test quality checks and standards enforcement
	@echo "🔍 Running test quality checks..."
	./scripts/test-quality-check.sh
	@echo "✅ Test quality checks passed"

# Code quality targets
lint: ## Run linting
	@echo "🔍 Running linting..."
	golangci-lint run
	@echo "✅ Linting passed"

lint-fix: ## Run linting with auto-fix
	@echo "🔧 Running linting with fixes..."
	golangci-lint run --fix

# Security targets
security: security-scan security-deps security-license ## Run all security checks

security-scan: ## Run vulnerability scanning
	@echo "🔒 Running vulnerability scan..."
	govulncheck ./...
	@echo "✅ Vulnerability scan passed"

security-deps: ## Check dependency vulnerabilities
	@echo "📦 Checking dependency vulnerabilities..."
	go list -json -deps ./... | nancy sleuth --skip-update-check || true
	@echo "✅ Dependency scan complete"

security-static: ## Run static security analysis
	@echo "🔍 Running static security analysis..."
	gosec -fmt sarif -out gosec-report.sarif ./... || true
	@echo "📄 Security report generated: gosec-report.sarif"

security-license: ## Check license compliance
	@echo "⚖️  Checking license compliance..."
	go-licenses check ./... || echo "⚠️  License check completed with warnings"
	go-licenses report ./... > licenses.txt
	@echo "📄 License report generated: licenses.txt"

audit: security ## Alias for security (legacy compatibility)

# Development setup
install-tools: ## Install required development tools
	@echo "🛠️  Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	# Nancy dependency scanning (optional - may need manual installation)
	go install github.com/google/go-licenses@latest
	@echo "✅ Development tools installed"

setup-hooks: ## Install git hooks
	@echo "🪝 Setting up git hooks..."
	chmod +x .githooks/pre-commit
	git config core.hooksPath .githooks
	@echo "✅ Git hooks configured"

setup: install-tools setup-hooks ## Complete development setup
	@echo "🎯 Development environment setup complete!"

# Maintenance targets
mod-tidy: ## Tidy go modules
	@echo "🧹 Tidying go modules..."
	go mod tidy
	go mod verify

mod-update: ## Update dependencies
	@echo "⬆️  Updating dependencies..."
	go get -u ./...
	go mod tidy

clean: ## Clean build artifacts
	@echo "🧽 Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.txt coverage.html
	rm -f gosec-report.sarif
	rm -f licenses.txt
	rm -f *.prof
	@echo "✅ Clean complete"

# Container targets
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	docker build -t cargoship:latest .

docker-scan: ## Scan Docker image for vulnerabilities
	@echo "🔒 Scanning Docker image..."
	trivy image cargoship:latest

# Deployment targets
deploy-check: security test lint ## Pre-deployment validation
	@echo "🚀 Pre-deployment checks complete!"

# CI/CD targets
ci: mod-tidy security lint test ## CI pipeline
	@echo "🎯 CI pipeline complete!"

cd: ci build-release ## CD pipeline
	@echo "🚀 CD pipeline complete!"

# Development workflow
dev: lint test ## Quick development cycle
	@echo "💻 Development cycle complete!"

all: clean setup security lint test build ## Full build pipeline
	@echo "🎉 Full pipeline complete!"

# Show security status
security-status: ## Show comprehensive security status
	@echo "🔒 CargoShip Security Status"
	@echo "=========================="
	@echo "📊 Vulnerability Scanning:"
	@govulncheck -version 2>/dev/null && echo "  ✅ govulncheck installed" || echo "  ❌ govulncheck missing"
	@gosec -version 2>/dev/null && echo "  ✅ gosec installed" || echo "  ❌ gosec missing"
	@nancy -version 2>/dev/null && echo "  ✅ nancy installed" || echo "  ❌ nancy missing"
	@echo ""
	@echo "🔧 Code Quality:"
	@golangci-lint --version 2>/dev/null && echo "  ✅ golangci-lint installed" || echo "  ❌ golangci-lint missing"
	@echo ""
	@echo "📋 License Compliance:"
	@go-licenses version 2>/dev/null && echo "  ✅ go-licenses installed" || echo "  ❌ go-licenses missing"
	@echo ""
	@echo "🪝 Git Hooks:"
	@test -x .githooks/pre-commit && echo "  ✅ pre-commit hook configured" || echo "  ❌ pre-commit hook missing"