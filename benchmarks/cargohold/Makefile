.PHONY: build test run clean help

# Default target
.DEFAULT_GOAL := help

# Configuration
BINARY := cargohold-benchmark
SCENARIO ?= small
TOOLS ?= cargohold,s5cmd,mc,tar
BUCKET ?= $(CARGOSHIP_BENCHMARK_BUCKET)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build benchmark suite
	@echo "Building benchmark suite..."
	go build -o $(BINARY) .

test: ## Run unit tests
	@echo "Running tests..."
	go test -v ./...

run: build ## Run benchmark (requires CARGOSHIP_BENCHMARK_BUCKET)
	@if [ -z "$(BUCKET)" ]; then \
		echo "Error: CARGOSHIP_BENCHMARK_BUCKET not set"; \
		exit 1; \
	fi
	./$(BINARY) \
		-scenario $(SCENARIO) \
		-bucket $(BUCKET) \
		-tools $(TOOLS) \
		-data-dir ./test-data \
		-results-dir ./results

run-small: SCENARIO=small ## Run small scenario (10k files)
run-small: run

run-medium: SCENARIO=medium ## Run medium scenario (100k files)
run-medium: run

run-large: SCENARIO=large ## Run large scenario (1M files)
run-large: run

run-xlarge: SCENARIO=xlarge ## Run xlarge scenario (10M files)
run-xlarge: run

run-all: build ## Run all scenarios
	@for scenario in small medium large xlarge; do \
		echo "Running $$scenario scenario..."; \
		$(MAKE) run SCENARIO=$$scenario || true; \
	done

regression: build ## Run regression analysis
	@./$(BINARY) \
		-scenario $(SCENARIO) \
		-bucket $(BUCKET) \
		-tools cargohold \
		-data-dir ./test-data \
		-results-dir ./results

clean: ## Clean build artifacts and test data
	@echo "Cleaning..."
	rm -f $(BINARY)
	rm -rf test-data results

clean-data: ## Clean only test data
	@echo "Cleaning test data..."
	rm -rf test-data

clean-results: ## Clean only results
	@echo "Cleaning results..."
	rm -rf results

install-deps: ## Install required dependencies
	@echo "Installing dependencies..."
	@which cargoship > /dev/null || echo "⚠️  cargoship not found"
	@which aws > /dev/null || echo "⚠️  aws CLI not found"
	@which s5cmd > /dev/null || echo "ℹ️  s5cmd not found (optional)"
	@which mc > /dev/null || echo "ℹ️  mc not found (optional)"
	@echo "✅ Dependency check complete"

.PHONY: build test run run-small run-medium run-large run-xlarge run-all regression clean clean-data clean-results install-deps help
