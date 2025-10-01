package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
	digest "github.com/opencontainers/go-digest"
)

// ImageMetadata contains metadata extracted from an image reference
type ImageMetadata struct {
	OriginalRef string        // Original image reference provided
	Registry    string        // e.g., quay.io
	Namespace   string        // e.g., redhat-user-workloads/ocp-bpfman-tenant
	Repository  string        // e.g., catalog-ystream
	Tag         string        // e.g., latest or v4.19
	Digest      digest.Digest // e.g., sha256:abc123...
	ShortDigest string        // First 8 chars of digest
	CatalogType string        // e.g., catalog-ystream, catalog-zstream
	Version     string        // e.g., 4.19, 4.20
}

// ExtractMetadata extracts metadata from an image reference
func ExtractMetadata(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	meta := &ImageMetadata{
		OriginalRef: imageRef,
	}

	// Parse the image reference
	if err := parseImageReference(imageRef, meta); err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	// If we don't have a digest, fetch it from the registry
	if meta.Digest == "" {
		if err := fetchDigest(ctx, imageRef, meta); err != nil {
			return nil, fmt.Errorf("fetching digest: %w", err)
		}
	}

	// Extract short digest
	if meta.Digest != "" {
		digestStr := string(meta.Digest)
		if strings.HasPrefix(digestStr, "sha256:") && len(digestStr) >= 15 {
			meta.ShortDigest = digestStr[7:15] // Skip "sha256:" and take 8 chars
		}
	}

	// Extract catalog type from repository
	if strings.Contains(meta.Repository, "catalog") {
		parts := strings.Split(meta.Repository, "-")
		if len(parts) >= 2 {
			meta.CatalogType = strings.Join(parts[0:2], "-") // e.g., "catalog-ystream"
		}
	}

	// Try to extract version from tag or catalog name
	meta.Version = extractVersion(meta.Repository, meta.Tag)

	return meta, nil
}

// parseImageReference parses an image reference into its components
func parseImageReference(imageRef string, meta *ImageMetadata) error {
	// Handle different formats:
	// quay.io/namespace/repo:tag
	// quay.io/namespace/repo@sha256:digest
	// registry.redhat.io/namespace/repo:tag

	// Remove docker:// prefix if present
	imageRef = strings.TrimPrefix(imageRef, "docker://")

	// Split by @ for digest references
	var baseRef string
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		baseRef = imageRef[:idx]
		digestStr := imageRef[idx+1:]
		if d, err := digest.Parse(digestStr); err == nil {
			meta.Digest = d
		}
	} else {
		baseRef = imageRef
	}

	// Split by : for tag
	parts := strings.Split(baseRef, ":")
	if len(parts) == 2 {
		meta.Tag = parts[1]
		baseRef = parts[0]
	}

	// Parse registry/namespace/repository
	pathParts := strings.Split(baseRef, "/")
	if len(pathParts) < 2 {
		return fmt.Errorf("invalid image reference format: %s", imageRef)
	}

	// First part is registry if it contains a dot or colon (port)
	if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") {
		meta.Registry = pathParts[0]
		pathParts = pathParts[1:]
	} else {
		meta.Registry = "docker.io" // Default registry
	}

	// Last part is repository
	if len(pathParts) > 0 {
		meta.Repository = pathParts[len(pathParts)-1]
		if len(pathParts) > 1 {
			meta.Namespace = strings.Join(pathParts[:len(pathParts)-1], "/")
		}
	}

	return nil
}

// fetchDigest fetches the digest from the registry if not present in the reference
func fetchDigest(ctx context.Context, imageRef string, meta *ImageMetadata) error {
	ref, err := docker.ParseReference("//" + imageRef)
	if err != nil {
		return fmt.Errorf("parsing docker reference: %w", err)
	}

	src, err := ref.NewImageSource(ctx, &types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: "amd64",
	})
	if err != nil {
		return fmt.Errorf("creating image source: %w", err)
	}
	defer src.Close()

	manifestBlob, _, err := src.GetManifest(ctx, nil)
	if err != nil {
		return fmt.Errorf("getting manifest: %w", err)
	}

	meta.Digest = digest.FromBytes(manifestBlob)
	return nil
}

// extractVersion attempts to extract version from repository or tag
func extractVersion(repository, tag string) string {
	// Check tag for version patterns
	if tag != "" {
		// Match patterns like v4.19, 4.20.0, etc.
		if strings.HasPrefix(tag, "v") {
			return strings.TrimPrefix(tag, "v")
		}
		// Check if tag looks like a version
		parts := strings.Split(tag, ".")
		if len(parts) >= 2 {
			return tag
		}
	}

	// Check repository for OCP version patterns
	if strings.Contains(repository, "ocp4-") {
		// Extract from patterns like catalog-ocp4-19
		parts := strings.Split(repository, "-")
		for i, part := range parts {
			if strings.HasPrefix(part, "ocp4") && i+1 < len(parts) {
				// Convert ocp4-19 to 4.19
				version := strings.TrimPrefix(part, "ocp")
				if i+1 < len(parts) {
					return version + "." + parts[i+1]
				}
				return version
			}
		}
	}

	return ""
}
