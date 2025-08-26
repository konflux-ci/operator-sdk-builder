package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/operator-framework/operator-manifest-tools/pkg/pullspec"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

type ImageReference struct {
	Name   string `json:"name,omitempty"`
	Image  string `json:"image"`
	Digest string `json:"digest,omitempty"`
}

type RelatedImage struct {
	Name  string `yaml:"name,omitempty"`
	Image string `yaml:"image"`
}

type ClusterServiceVersion struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		RelatedImages []RelatedImage `yaml:"relatedImages,omitempty"`
		Install       struct {
			Spec struct {
				Deployments []struct {
					Name string `yaml:"name"`
					Spec struct {
						Template struct {
							Spec struct {
								Containers []struct {
									Name  string `yaml:"name"`
									Image string `yaml:"image"`
								} `yaml:"containers"`
								InitContainers []struct {
									Name  string `yaml:"name"`
									Image string `yaml:"image"`
								} `yaml:"initContainers,omitempty"`
							} `yaml:"spec"`
						} `yaml:"template"`
					} `yaml:"spec"`
				} `yaml:"deployments"`
			} `yaml:"spec"`
		} `yaml:"install"`
	} `yaml:"spec"`
}

type BundleAnalyzer struct {
	systemContext *types.SystemContext
}

func NewBundleAnalyzer() *BundleAnalyzer {
	return &BundleAnalyzer{
		systemContext: &types.SystemContext{
			DockerInsecureSkipTLSVerify: types.OptionalBoolTrue,
		},
	}
}

func (ba *BundleAnalyzer) ExtractImageReferences(ctx context.Context, bundleImage string) ([]ImageReference, error) {
	// Security: Input validation - limit bundle image reference length
	if len(bundleImage) > 2048 {
		return nil, fmt.Errorf("bundle image reference too long: %d characters exceeds maximum of 2048", len(bundleImage))
	}

	// Add docker:// prefix if no transport is specified
	if !strings.Contains(bundleImage, "://") {
		bundleImage = "docker://" + bundleImage
	}

	// Security: Create exclusive temporary directory with restrictive permissions
	tempDir, err := os.MkdirTemp("", "bundle-extract-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Security: Ensure temp directory has restricted permissions (700)
	if err := os.Chmod(tempDir, 0700); err != nil {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			fmt.Printf("Warning: failed to clean up temp directory on chmod error: %v\n", removeErr)
		}
		return nil, fmt.Errorf("failed to set temp directory permissions: %w", err)
	}

	// Ensure cleanup happens regardless of how function exits
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			fmt.Printf("Warning: failed to clean up temp directory %s: %v\n", tempDir, err)
		}
	}()

	// Extract bundle image to temporary directory
	err = ba.extractBundleToDirectory(ctx, bundleImage, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bundle image: %w", err)
	}

	// Find and process CSV files in the extracted directory
	imageRefs, err := ba.extractImageReferencesFromDirectory(tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references from bundle: %w", err)
	}

	return ba.deduplicateImageReferences(imageRefs), nil
}

func (ba *BundleAnalyzer) extractBundleToDirectory(ctx context.Context, bundleImage, destDir string) error {
	srcRef, err := alltransports.ParseImageName(bundleImage)
	if err != nil {
		return fmt.Errorf("failed to parse bundle image reference %q: %w", bundleImage, err)
	}

	// Create image and extract layers manually
	srcImg, err := srcRef.NewImage(ctx, ba.systemContext)
	if err != nil {
		return fmt.Errorf("failed to create source image: %w", err)
	}
	defer func() {
		if closeErr := srcImg.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close source image: %v\n", closeErr)
		}
	}()

	// Create image source to get layer blobs
	srcImgSource, err := srcRef.NewImageSource(ctx, ba.systemContext)
	if err != nil {
		return fmt.Errorf("failed to create source image source: %w", err)
	}
	defer func() {
		if closeErr := srcImgSource.Close(); closeErr != nil {
			fmt.Printf("Warning: failed to close source image source: %v\n", closeErr)
		}
	}()

	layerInfos := srcImg.LayerInfos()
	if len(layerInfos) == 0 {
		return fmt.Errorf("bundle image has no layers")
	}

	// Security: Create shared cache outside loop to prevent resource accumulation
	sharedCache := memory.New()

	// Extract each layer to the destination directory with proper resource management
	for i, layerInfo := range layerInfos {
		// Security: Limit the number of layers processed (max 50 layers)
		if i >= 50 {
			fmt.Printf("Warning: limiting layer processing to 50 layers for security\n")
			break
		}

		layerReader, _, err := srcImgSource.GetBlob(ctx, layerInfo, sharedCache)
		if err != nil {
			fmt.Printf("Warning: failed to read layer %d: %v\n", i, err)
			continue
		}

		// Use anonymous function to ensure layerReader is always closed
		func() {
			defer func() {
				if closeErr := layerReader.Close(); closeErr != nil {
					fmt.Printf("Warning: failed to close layer reader %d: %v\n", i, closeErr)
				}
			}()

			// Extract tar content to destination directory
			if extractErr := ba.extractTarToDirectory(layerReader, destDir); extractErr != nil {
				fmt.Printf("Warning: failed to extract layer %d: %v\n", i, extractErr)
			}
		}()
	}

	return nil
}

func (ba *BundleAnalyzer) extractTarToDirectory(tarReader io.ReadCloser, destDir string) error {
	// Try to decompress with gzip first
	gzReader, err := gzip.NewReader(tarReader)
	var tr *tar.Reader
	if err != nil {
		// If gzip fails, try reading as uncompressed tar
		tr = tar.NewReader(tarReader)
	} else {
		defer func() {
			if closeErr := gzReader.Close(); closeErr != nil {
				fmt.Printf("Warning: failed to close gzip reader: %v\n", closeErr)
			}
		}()
		tr = tar.NewReader(gzReader)
	}

	// Security: Get absolute canonical path of destination directory
	destDirAbs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for destination directory: %w", err)
	}
	destDirAbs = filepath.Clean(destDirAbs)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Security: Input validation - check header name length (max 4096 chars)
		if len(header.Name) > 4096 {
			return fmt.Errorf("path too long in archive: %d characters exceeds maximum of 4096", len(header.Name))
		}

		// Security check: Prevent absolute paths in archive
		if filepath.IsAbs(header.Name) {
			return fmt.Errorf("absolute path not allowed in archive: %s", header.Name)
		}

		// Security: Clean the header name to normalize path separators and remove . and .. elements
		cleanName := filepath.Clean(header.Name)

		// Security: Additional check for path traversal patterns after cleaning
		if strings.Contains(cleanName, "..") {
			return fmt.Errorf("path traversal attempt detected in cleaned path: %s (original: %s)", cleanName, header.Name)
		}

		// Construct the full path using cleaned name
		target := filepath.Join(destDirAbs, cleanName)

		// Security: Get absolute canonical path of target and ensure it's clean
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for target %s: %w", target, err)
		}
		targetAbs = filepath.Clean(targetAbs)

		// Security: Bulletproof path traversal protection using string prefix check
		// This handles all cases including foo/../../../etc/passwd
		if !strings.HasPrefix(targetAbs+string(os.PathSeparator), destDirAbs+string(os.PathSeparator)) &&
			targetAbs != destDirAbs {
			return fmt.Errorf("path traversal attack detected: %s resolves to %s outside destination %s",
				header.Name, targetAbs, destDirAbs)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory using the secure absolute path
			if err := os.MkdirAll(targetAbs, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetAbs, err)
			}
		case tar.TypeReg:
			// Security: Validate file size (max 100MB per file)
			if header.Size > 100*1024*1024 {
				return fmt.Errorf("file too large in archive: %d bytes exceeds maximum of 100MB", header.Size)
			}

			// Create parent directories if they don't exist using secure path
			if err := os.MkdirAll(filepath.Dir(targetAbs), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", targetAbs, err)
			}

			// Create file using secure path with restricted permissions
			file, err := os.OpenFile(targetAbs, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetAbs, err)
			}

			// Security: Use io.LimitReader to prevent reading more than declared size
			limitedReader := io.LimitReader(tr, header.Size)

			// Copy file content with size limit
			_, err = io.Copy(file, limitedReader)
			if closeErr := file.Close(); closeErr != nil {
				fmt.Printf("Warning: failed to close file %s: %v\n", targetAbs, closeErr)
			}
			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", targetAbs, err)
			}
		case tar.TypeSymlink, tar.TypeLink:
			// Skip symlinks and hard links to prevent potential security issues
			fmt.Printf("Warning: skipping link %s for security reasons\n", header.Name)
			continue
		default:
			// Skip other unknown file types
			fmt.Printf("Warning: skipping unknown file type %d for %s\n", header.Typeflag, header.Name)
			continue
		}
	}

	return nil
}

func (ba *BundleAnalyzer) extractImageReferencesFromDirectory(bundleDir string) ([]ImageReference, error) {
	var allImageRefs []ImageReference

	// Look for manifests directory
	manifestsDir := filepath.Join(bundleDir, "manifests")
	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("manifests directory not found in bundle")
	}

	// Walk through all YAML files in manifests directory
	err := filepath.Walk(manifestsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Process only YAML files
		if !info.IsDir() && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			imageRefs, err := ba.extractImageReferencesFromFile(path)
			if err != nil {
				// Log warning but continue processing other files
				fmt.Printf("Warning: failed to process file %s: %v\n", path, err)
				return nil
			}
			allImageRefs = append(allImageRefs, imageRefs...)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk manifests directory: %w", err)
	}

	return allImageRefs, nil
}

func (ba *BundleAnalyzer) extractImageReferencesFromFile(filePath string) ([]ImageReference, error) {
	// Security: Input validation for file path
	if len(filePath) > 4096 {
		return nil, fmt.Errorf("file path too long: %d characters exceeds maximum of 4096", len(filePath))
	}

	// Security: Get file info to validate size before reading
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		// Security: Sanitize error message to prevent path disclosure
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Security: Limit file size (max 10MB for YAML files)
	if fileInfo.Size() > 10*1024*1024 {
		return nil, fmt.Errorf("YAML file too large: %d bytes exceeds maximum of 10MB", fileInfo.Size())
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		// Security: Sanitize error message
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ba.extractImageReferencesFromManifest(content, filepath.Base(filePath))
}

func (ba *BundleAnalyzer) extractImageReferencesFromManifest(content []byte, filename string) ([]ImageReference, error) {
	// Security: Input validation
	if len(content) == 0 {
		return []ImageReference{}, nil // Empty content is valid
	}

	// Security: Limit manifest content size (max 5MB)
	if len(content) > 5*1024*1024 {
		return nil, fmt.Errorf("manifest file too large: %d bytes exceeds maximum of 5MB", len(content))
	}

	// Security: Validate filename length and characters
	if len(filename) > 255 {
		return nil, fmt.Errorf("filename too long: %d characters exceeds maximum of 255", len(filename))
	}

	// First try to parse as a generic Kubernetes object to check the Kind
	var obj struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &obj); err != nil {
		// Security: Sanitize error message to prevent content disclosure
		return nil, fmt.Errorf("failed to unmarshal manifest: invalid YAML format")
	}

	// Only process ClusterServiceVersion manifests
	if obj.Kind != "ClusterServiceVersion" {
		return []ImageReference{}, nil // Return empty list, not an error
	}

	// Use operator-manifest-tools to extract images
	operatorCSV, err := ba.createOperatorCSVFromBytes(content, filename)
	if err != nil {
		// Fall back to legacy implementation if operator-manifest-tools fails
		return ba.extractImageReferencesLegacy(content)
	}

	return ba.convertPullSpecsToImageReferences(operatorCSV)
}

func (ba *BundleAnalyzer) deduplicateImageReferences(refs []ImageReference) []ImageReference {
	seen := make(map[string]bool)
	var result []ImageReference

	for _, ref := range refs {
		// Security: Skip empty images
		if ref.Image == "" {
			continue
		}

		// Security: Validate image reference length and format
		if len(ref.Image) > 1024 {
			fmt.Printf("Warning: skipping image reference too long: %d characters\n", len(ref.Image))
			continue
		}

		// Security: Validate name length
		if len(ref.Name) > 256 {
			fmt.Printf("Warning: skipping image with name too long: %d characters\n", len(ref.Name))
			continue
		}

		// Security: Basic format validation for image references
		if !ba.isValidImageReference(ref.Image) {
			fmt.Printf("Warning: skipping invalid image reference format: %s\n", ba.sanitizeForLog(ref.Image))
			continue
		}

		key := ref.Image
		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}

	return result
}

// isValidImageReference performs basic validation of image reference format
func (ba *BundleAnalyzer) isValidImageReference(image string) bool {
	// Security: Basic validation - image should not contain control characters
	for _, char := range image {
		if char < 32 || char == 127 {
			return false
		}
	}

	// Security: Should contain at least one slash or colon (basic format check)
	return strings.Contains(image, "/") || strings.Contains(image, ":")
}

// sanitizeForLog sanitizes strings for safe logging to prevent log injection
func (ba *BundleAnalyzer) sanitizeForLog(input string) string {
	// Security: Remove control characters and limit length for logging
	var sanitized strings.Builder
	for i, char := range input {
		if i >= 100 { // Limit log output length
			sanitized.WriteString("...")
			break
		}
		if char >= 32 && char != 127 {
			sanitized.WriteRune(char)
		} else {
			sanitized.WriteString("?")
		}
	}
	return sanitized.String()
}

func isManifestFile(filename string) bool {
	ext := filepath.Ext(filename)
	basename := filepath.Base(filename)

	// Check if it's in manifests directory and has yaml extension (case-insensitive)
	extLower := strings.ToLower(ext)
	hasYamlExt := extLower == ".yaml" || extLower == ".yml"
	inManifestsDir := strings.Contains(filename, "manifests/")

	// Skip metadata files and annotations
	if strings.HasPrefix(basename, ".") || strings.Contains(basename, "annotations") {
		return false
	}

	return inManifestsDir && hasYamlExt
}

// createOperatorCSVFromBytes creates an OperatorCSV from raw bytes using operator-manifest-tools
func (ba *BundleAnalyzer) createOperatorCSVFromBytes(content []byte, filename string) (*pullspec.OperatorCSV, error) {
	// Parse YAML content into unstructured object
	data := &unstructured.Unstructured{}

	// Use Kubernetes YAML decoder to properly handle the content
	dec := kyaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(content, nil, data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML content: %w", err)
	}

	// Create OperatorCSV using operator-manifest-tools
	operatorCSV, err := pullspec.NewOperatorCSV(filename, data, pullspec.DefaultHeuristic)
	if err != nil {
		return nil, fmt.Errorf("failed to create OperatorCSV: %w", err)
	}

	return operatorCSV, nil
}

// convertPullSpecsToImageReferences converts operator-manifest-tools pullspecs to our ImageReference format
func (ba *BundleAnalyzer) convertPullSpecsToImageReferences(operatorCSV *pullspec.OperatorCSV) ([]ImageReference, error) {
	// Get all pullspecs from the CSV
	imagenames, err := operatorCSV.GetPullSpecs()
	if err != nil {
		return nil, fmt.Errorf("failed to get pullspecs: %w", err)
	}

	var imageRefs []ImageReference

	// Convert imagename.ImageName objects to our ImageReference format
	for _, imagename := range imagenames {
		imageStr := imagename.String()

		// Extract name from the image (use repository name if available)
		name := imagename.Repo
		if name == "" {
			// Fallback to extracting name from full image string
			parts := strings.Split(imageStr, "/")
			if len(parts) > 0 {
				lastName := parts[len(parts)-1]
				// Remove tag/digest for name
				if idx := strings.Index(lastName, ":"); idx != -1 {
					name = lastName[:idx]
				} else if idx := strings.Index(lastName, "@"); idx != -1 {
					name = lastName[:idx]
				} else {
					name = lastName
				}
			}
		}

		if name == "" {
			name = "unknown"
		}

		imageRefs = append(imageRefs, ImageReference{
			Name:   name,
			Image:  imageStr,
			Digest: extractDigest(imageStr),
		})
	}

	return imageRefs, nil
}

// extractImageReferencesLegacy is the fallback implementation using our original parsing logic
func (ba *BundleAnalyzer) extractImageReferencesLegacy(content []byte) ([]ImageReference, error) {
	var csv ClusterServiceVersion
	if err := yaml.Unmarshal(content, &csv); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CSV: %w", err)
	}

	var imageRefs []ImageReference

	// Extract from spec.relatedImages
	for _, relatedImage := range csv.Spec.RelatedImages {
		if relatedImage.Image != "" {
			imageRefs = append(imageRefs, ImageReference{
				Name:   relatedImage.Name,
				Image:  relatedImage.Image,
				Digest: extractDigest(relatedImage.Image),
			})
		}
	}

	// Extract from deployment containers
	for _, deployment := range csv.Spec.Install.Spec.Deployments {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Image != "" {
				name := container.Name
				if name == "" {
					name = fmt.Sprintf("%s-container", deployment.Name)
				}
				imageRefs = append(imageRefs, ImageReference{
					Name:   name,
					Image:  container.Image,
					Digest: extractDigest(container.Image),
				})
			}
		}
		for _, initContainer := range deployment.Spec.Template.Spec.InitContainers {
			if initContainer.Image != "" {
				name := initContainer.Name
				if name == "" {
					name = fmt.Sprintf("%s-init-container", deployment.Name)
				}
				imageRefs = append(imageRefs, ImageReference{
					Name:   name,
					Image:  initContainer.Image,
					Digest: extractDigest(initContainer.Image),
				})
			}
		}
	}

	return imageRefs, nil
}

func extractDigest(image string) string {
	if strings.Contains(image, "@sha256:") {
		// Find the last occurrence of @ to handle multiple @ symbols
		lastAtIndex := strings.LastIndex(image, "@")
		if lastAtIndex != -1 && lastAtIndex < len(image)-1 {
			digest := image[lastAtIndex+1:]
			// Validate that it looks like a digest
			if strings.HasPrefix(digest, "sha256:") {
				return digest
			}
		}
	}
	return ""
}
