package bundle

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/pkg/blobinfocache/memory"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go/v1"
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
	// Add docker:// prefix if no transport is specified
	if !strings.Contains(bundleImage, "://") {
		bundleImage = "docker://" + bundleImage
	}

	// Create temporary directory to extract bundle contents
	tempDir, err := os.MkdirTemp("", "bundle-extract-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

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
	defer srcImg.Close()

	// Create image source to get layer blobs
	srcImgSource, err := srcRef.NewImageSource(ctx, ba.systemContext)
	if err != nil {
		return fmt.Errorf("failed to create source image source: %w", err)
	}
	defer srcImgSource.Close()

	layerInfos := srcImg.LayerInfos()
	if len(layerInfos) == 0 {
		return fmt.Errorf("bundle image has no layers")
	}

	// Extract each layer to the destination directory
	for i, layerInfo := range layerInfos {
		cache := memory.New()
		layerReader, _, err := srcImgSource.GetBlob(ctx, layerInfo, cache)
		if err != nil {
			fmt.Printf("Warning: failed to read layer %d: %v\n", i, err)
			continue
		}

		// Extract tar content to destination directory
		err = ba.extractTarToDirectory(layerReader, destDir)
		layerReader.Close()
		if err != nil {
			fmt.Printf("Warning: failed to extract layer %d: %v\n", i, err)
			continue
		}
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
		defer gzReader.Close()
		tr = tar.NewReader(gzReader)
	}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Construct the full path
		target := filepath.Join(destDir, header.Name)

		// Ensure the target is within destDir (security check)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			// Create parent directories if they don't exist
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", target, err)
			}

			// Create file
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}

			// Copy file content
			_, err = io.Copy(file, tr)
			file.Close()
			if err != nil {
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
		default:
			// Skip other file types (symlinks, etc.)
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
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ba.extractImageReferencesFromManifest(content, filepath.Base(filePath))
}

func (ba *BundleAnalyzer) extractImageReferencesFromTar(tarReader io.ReadCloser) ([]ImageReference, error) {
	var imageRefs []ImageReference

	tr := tar.NewReader(tarReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if !isManifestFile(header.Name) {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("failed to read file content: %w", err)
		}

		refs, err := ba.extractImageReferencesFromManifest(content, header.Name)
		if err != nil {
			// Log warning but continue processing other files
			fmt.Printf("Warning: failed to parse manifest %s: %v\n", header.Name, err)
			continue
		}

		imageRefs = append(imageRefs, refs...)
	}

	return imageRefs, nil
}

func (ba *BundleAnalyzer) extractImageReferencesFromManifest(content []byte, filename string) ([]ImageReference, error) {
	// First try to parse as a generic Kubernetes object to check the Kind
	var obj struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(content, &obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
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
		// Skip empty images
		if ref.Image == "" {
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

func (ba *BundleAnalyzer) getLayerInfosFromManifest(manifestBlob []byte, manifestType string) ([]types.BlobInfo, error) {
	var layerInfos []types.BlobInfo

	switch manifestType {
	case v1.MediaTypeImageManifest:
		var manifest v1.Manifest
		if err := json.Unmarshal(manifestBlob, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse OCI manifest: %w", err)
		}

		for _, layer := range manifest.Layers {
			layerInfos = append(layerInfos, types.BlobInfo{
				Digest: digest.Digest(layer.Digest),
				Size:   layer.Size,
			})
		}

	case "application/vnd.docker.distribution.manifest.v2+json":
		var manifest struct {
			Layers []struct {
				Digest string `json:"digest"`
				Size   int64  `json:"size"`
			} `json:"layers"`
		}
		if err := json.Unmarshal(manifestBlob, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse Docker manifest v2: %w", err)
		}

		for _, layer := range manifest.Layers {
			layerInfos = append(layerInfos, types.BlobInfo{
				Digest: digest.Digest(layer.Digest),
				Size:   layer.Size,
			})
		}

	default:
		return nil, fmt.Errorf("unsupported manifest type: %s", manifestType)
	}

	return layerInfos, nil
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
