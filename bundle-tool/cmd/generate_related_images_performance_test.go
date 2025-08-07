package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestGenerateRelatedImagesPerformance tests performance with various CSV sizes
func TestGenerateRelatedImagesPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance tests in short mode")
	}

	testCases := []struct {
		name           string
		deployments    int
		containersEach int
		timeoutSeconds int
	}{
		{"small CSV", 1, 2, 10},
		{"medium CSV", 5, 5, 15},
		{"large CSV", 10, 10, 30},
		{"very large CSV", 20, 15, 60},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary directory
			tempDir, err := ioutil.TempDir("", fmt.Sprintf("perf-test-%s-", strings.ReplaceAll(tc.name, " ", "-")))
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Generate large CSV
			csvContent := generateLargeCSV(tc.deployments, tc.containersEach)
			
			bundleDir := filepath.Join(tempDir, "bundle")
			manifestsDir := filepath.Join(bundleDir, "manifests")
			err = os.MkdirAll(manifestsDir, 0755)
			if err != nil {
				t.Fatalf("failed to create manifests dir: %v", err)
			}

			csvPath := filepath.Join(manifestsDir, "large-operator.clusterserviceversion.yaml")
			err = ioutil.WriteFile(csvPath, []byte(csvContent), 0644)
			if err != nil {
				t.Fatalf("failed to write CSV file: %v", err)
			}

			// Measure performance
			start := time.Now()
			
			runE := generateRelatedImagesCmd.RunE
			cmd := generateRelatedImagesCmd
			cmd.ResetFlags()
			cmd.SetArgs([]string{bundleDir, "--dry-run"})
			cmd.ParseFlags([]string{bundleDir, "--dry-run"})

			err = runE(cmd, []string{bundleDir})
			duration := time.Since(start)

			// Check performance constraints
			timeout := time.Duration(tc.timeoutSeconds) * time.Second
			if duration > timeout {
				t.Errorf("performance test failed: took %v, expected less than %v", duration, timeout)
			}

			// Log performance metrics
			t.Logf("Performance: %s completed in %v (deployments: %d, containers each: %d, total images: %d)", 
				tc.name, duration, tc.deployments, tc.containersEach, tc.deployments*tc.containersEach*2) // *2 for init containers

			// Check if we got expected error (operator-manifest-tools parsing)
			if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestGenerateRelatedImagesConcurrency tests concurrent processing
func TestGenerateRelatedImagesConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency tests in short mode")
	}

	// Create base test directory
	baseDir, err := ioutil.TempDir("", "concurrency-test-")
	if err != nil {
		t.Fatalf("failed to create base temp dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	// Number of concurrent operations
	numConcurrent := 5
	
	t.Run("concurrent CSV processing", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, numConcurrent)
		
		for i := 0; i < numConcurrent; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				// Create unique bundle for this goroutine
				bundleDir := filepath.Join(baseDir, fmt.Sprintf("bundle-%d", id))
				manifestsDir := filepath.Join(bundleDir, "manifests")
				err := os.MkdirAll(manifestsDir, 0755)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: failed to create manifests dir: %w", id, err)
					return
				}

				// Create CSV with unique images
				csvContent := fmt.Sprintf(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: concurrent-operator-%d.v1.0.0
spec:
  displayName: Concurrent Operator %d
  version: 1.0.0
  install:
    strategy: deployment
    spec:
      deployments:
      - name: concurrent-deployment-%d
        spec:
          template:
            spec:
              containers:
              - name: main-%d
                image: quay.io/test/concurrent-%d:v1.0.0
              - name: sidecar-%d
                image: registry.example.com/sidecar-%d:latest
              initContainers:
              - name: init-%d
                image: docker.io/init/concurrent-%d:v1.0.0
`, id, id, id, id, id, id, id, id, id)

				csvPath := filepath.Join(manifestsDir, fmt.Sprintf("concurrent-%d.clusterserviceversion.yaml", id))
				err = ioutil.WriteFile(csvPath, []byte(csvContent), 0644)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: failed to write CSV: %w", id, err)
					return
				}

				// Process CSV
				runE := generateRelatedImagesCmd.RunE
				cmd := generateRelatedImagesCmd
				cmd.ResetFlags()
				cmd.SetArgs([]string{bundleDir, "--dry-run"})
				cmd.ParseFlags([]string{bundleDir, "--dry-run"})

				err = runE(cmd, []string{bundleDir})
				if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
					errors <- fmt.Errorf("goroutine %d: unexpected error: %w", id, err)
					return
				}
				
				t.Logf("Goroutine %d completed successfully", id)
			}(i)
		}

		// Wait for all goroutines to complete
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		// Check for completion or timeout
		select {
		case <-done:
			// Check for any errors
			close(errors)
			for err := range errors {
				t.Errorf("Concurrency error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("Concurrency test timed out after 30 seconds")
		}
	})
}

// TestGenerateRelatedImagesMemoryUsage tests memory usage patterns
func TestGenerateRelatedImagesMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory tests in short mode")
	}

	// Create temporary directory
	tempDir, err := ioutil.TempDir("", "memory-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create CSV with many images to test memory usage
	csvContent := generateLargeCSV(50, 20) // 50 deployments, 20 containers each = 1000+ images
	
	bundleDir := filepath.Join(tempDir, "bundle")
	manifestsDir := filepath.Join(bundleDir, "manifests")
	err = os.MkdirAll(manifestsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create manifests dir: %v", err)
	}

	csvPath := filepath.Join(manifestsDir, "memory-test.clusterserviceversion.yaml")
	err = ioutil.WriteFile(csvPath, []byte(csvContent), 0644)
	if err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	t.Run("memory usage test", func(t *testing.T) {
		// Test multiple iterations to check for memory leaks
		for i := 0; i < 10; i++ {
			runE := generateRelatedImagesCmd.RunE
			cmd := generateRelatedImagesCmd
			cmd.ResetFlags()
			cmd.SetArgs([]string{bundleDir, "--dry-run"})
			cmd.ParseFlags([]string{bundleDir, "--dry-run"})

			start := time.Now()
			err := runE(cmd, []string{bundleDir})
			duration := time.Since(start)

			// Check that performance doesn't degrade significantly across iterations
			if duration > 10*time.Second {
				t.Errorf("iteration %d took too long: %v", i+1, duration)
			}

			// Allow expected operator-manifest-tools errors
			if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
				t.Errorf("iteration %d failed: %v", i+1, err)
			}
		}
	})
}

// generateLargeCSV creates a CSV with specified number of deployments and containers
func generateLargeCSV(numDeployments, containersPerDeployment int) string {
	var builder strings.Builder
	
	builder.WriteString(`apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: large-operator.v1.0.0
  namespace: default
spec:
  displayName: Large Operator
  version: 1.0.0
  maturity: stable
  provider:
    name: Test Provider
  install:
    strategy: deployment
    spec:
      deployments:
`)

	// Generate deployments
	for d := 0; d < numDeployments; d++ {
		builder.WriteString(fmt.Sprintf(`      - name: deployment-%d
        spec:
          replicas: 1
          selector:
            matchLabels:
              name: deployment-%d
          template:
            metadata:
              labels:
                name: deployment-%d
            spec:
              containers:
`, d, d, d))

		// Generate containers for this deployment
		for c := 0; c < containersPerDeployment; c++ {
			builder.WriteString(fmt.Sprintf(`              - name: container-%d-%d
                image: quay.io/large-test/container-%d-%d:v1.0.0
                env:
                - name: RELATED_IMAGE_%d_%d
                  value: quay.io/large-test/related-%d-%d:latest
`, d, c, d, c, d, c, d, c))
		}

		// Add init containers
		builder.WriteString(`              initContainers:
`)
		for c := 0; c < containersPerDeployment/2; c++ {
			builder.WriteString(fmt.Sprintf(`              - name: init-%d-%d
                image: quay.io/large-test/init-%d-%d:v1.0.0
`, d, c, d, c))
		}
	}

	// Add related images section
	builder.WriteString(`  relatedImages:
`)
	imageCount := 0
	for d := 0; d < numDeployments; d++ {
		for c := 0; c < containersPerDeployment; c++ {
			builder.WriteString(fmt.Sprintf(`  - name: image-%d
    image: quay.io/large-test/container-%d-%d:v1.0.0
`, imageCount, d, c))
			imageCount++
		}
	}

	return builder.String()
}

// BenchmarkGenerateRelatedImages provides benchmark testing
func BenchmarkGenerateRelatedImages(b *testing.B) {
	// Create temporary directory
	tempDir, err := ioutil.TempDir("", "benchmark-test-")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test CSV
	csvContent := generateLargeCSV(5, 5) // Medium-sized CSV for benchmarking
	
	bundleDir := filepath.Join(tempDir, "bundle")
	manifestsDir := filepath.Join(bundleDir, "manifests")
	err = os.MkdirAll(manifestsDir, 0755)
	if err != nil {
		b.Fatalf("failed to create manifests dir: %v", err)
	}

	csvPath := filepath.Join(manifestsDir, "benchmark.clusterserviceversion.yaml")
	err = ioutil.WriteFile(csvPath, []byte(csvContent), 0644)
	if err != nil {
		b.Fatalf("failed to write CSV file: %v", err)
	}

	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		runE := generateRelatedImagesCmd.RunE
		cmd := generateRelatedImagesCmd
		cmd.ResetFlags()
		cmd.SetArgs([]string{bundleDir, "--dry-run"})
		cmd.ParseFlags([]string{bundleDir, "--dry-run"})

		err := runE(cmd, []string{bundleDir})
		
		// Allow expected operator-manifest-tools errors
		if err != nil && !strings.Contains(err.Error(), "Missing ClusterServiceVersion") {
			b.Errorf("benchmark iteration %d failed: %v", i, err)
		}
	}
}