package catalog

import (
	"context"
	"fmt"
	"strings"

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

// ExtractMetadata extracts metadata from an image reference.
func ExtractMetadata(ctx context.Context, imageRef string) (*ImageMetadata, error) {
	meta := &ImageMetadata{
		OriginalRef: imageRef,
	}

	if err := parseImageReference(imageRef, meta); err != nil {
		return nil, fmt.Errorf("parsing image reference: %w", err)
	}

	if meta.Digest == "" {
		if err := fetchDigest(ctx, imageRef, meta); err != nil {
			return nil, fmt.Errorf("fetching digest: %w", err)
		}
	}

	if meta.Digest != "" {
		digestStr := string(meta.Digest)
		if strings.HasPrefix(digestStr, "sha256:") && len(digestStr) >= 15 {
			meta.ShortDigest = digestStr[7:15] // Skip "sha256:" and take 8 chars
		}
	}

	if strings.Contains(meta.Repository, "catalog") {
		parts := strings.Split(meta.Repository, "-")
		if len(parts) >= 2 {
			meta.CatalogType = strings.Join(parts[0:2], "-") // e.g., "catalog-ystream"
		}
	}

	meta.Version = extractVersion(meta.Repository, meta.Tag)

	return meta, nil
}

// parseImageReference parses an image reference into its components.
func parseImageReference(imageRef string, meta *ImageMetadata) error {
	// Handle different formats:
	// quay.io/namespace/repo:tag
	// quay.io/namespace/repo@sha256:digest
	// registry.redhat.io/namespace/repo:tag

	imageRef = strings.TrimPrefix(imageRef, "docker://")

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

	parts := strings.Split(baseRef, ":")
	if len(parts) == 2 {
		meta.Tag = parts[1]
		baseRef = parts[0]
	}

	pathParts := strings.Split(baseRef, "/")
	if len(pathParts) < 2 {
		return fmt.Errorf("invalid image reference format: %s", imageRef)
	}

	if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") {
		meta.Registry = pathParts[0]
		pathParts = pathParts[1:]
	} else {
		meta.Registry = "docker.io"
	}

	if len(pathParts) > 0 {
		meta.Repository = pathParts[len(pathParts)-1]
		if len(pathParts) > 1 {
			meta.Namespace = strings.Join(pathParts[:len(pathParts)-1], "/")
		}
	}

	return nil
}

// fetchDigest is temporarily disabled to avoid CGO dependency conflicts
// TODO: Re-implement using operator-registry's image handling or a pure Go alternative
func fetchDigest(ctx context.Context, imageRef string, meta *ImageMetadata) error {
	// For now, generate a placeholder digest or skip if no digest in reference
	// This allows the tool to work without pulling from registry
	return fmt.Errorf("digest fetching temporarily disabled - please use image references with explicit digests (@sha256:...)")
}

// extractVersion attempts to extract version from repository or tag.
func extractVersion(repository, tag string) string {
	if tag != "" {
		if strings.HasPrefix(tag, "v") {
			return strings.TrimPrefix(tag, "v")
		}
		parts := strings.Split(tag, ".")
		if len(parts) >= 2 {
			return tag
		}
	}

	if strings.Contains(repository, "ocp4-") {
		parts := strings.Split(repository, "-")
		for i, part := range parts {
			if strings.HasPrefix(part, "ocp4") && i+1 < len(parts) {
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
