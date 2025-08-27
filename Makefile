.PHONY: help build-bundle-tool ci-bundle-tool clean-bundle-tool all

# Default target
help:
	@echo "Available targets:"
	@echo "  build-bundle-tool  - Build the bundle-tool binary"
	@echo "  ci-bundle-tool     - Run full CI pipeline for bundle-tool (deps, fmt, vet, lint, test, build)"
	@echo "  clean-bundle-tool  - Clean bundle-tool build artifacts"
	@echo "  all                - Build and test all components"

# Build the bundle-tool binary
build-bundle-tool:
	@echo "Building bundle-tool..."
	cd bundle-tool && make build

# Run full CI pipeline for bundle-tool
ci-bundle-tool:
	@echo "Running CI pipeline for bundle-tool..."
	cd bundle-tool && make ci

# Clean bundle-tool build artifacts
clean-bundle-tool:
	@echo "Cleaning bundle-tool build artifacts..."
	cd bundle-tool && make clean

# Build and test all components
all: ci-bundle-tool
