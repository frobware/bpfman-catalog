package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/bpfman-catalog/pkg/bundle"
	"sigs.k8s.io/yaml"
)

// ExtractImageReferences extracts all image references from a bundle image using existing bundle processing logic.
func ExtractImageReferences(ctx context.Context, bundleRef ImageRef) ([]string, error) {
	// Generate FBC template from the bundle
	fbcTemplate, err := bundle.GenerateFBCTemplate(bundleRef.String(), "preview")
	if err != nil {
		return nil, fmt.Errorf("failed to generate FBC template: %w", err)
	}

	// Render the catalog to get processed bundle content
	catalogYAML, err := bundle.RenderCatalog(ctx, fbcTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to render catalog: %w", err)
	}

	// Parse image references from the rendered catalog
	images, err := parseImageReferencesFromYAML([]byte(catalogYAML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse image references: %w", err)
	}

	return deduplicateStrings(images), nil
}

// parseImageReferencesFromYAML extracts image references from YAML content.
func parseImageReferencesFromYAML(data []byte) ([]string, error) {
	// Parse as YAML documents (bundle may contain multiple)
	documents := strings.Split(string(data), "---")
	var allImages []string

	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var parsed interface{}
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			// Skip documents that can't be parsed as YAML
			continue
		}

		images := extractImagesFromParsedYAML(parsed)
		allImages = append(allImages, images...)
	}

	return allImages, nil
}

// extractImagesFromParsedYAML recursively extracts image references from parsed YAML.
func extractImagesFromParsedYAML(data interface{}) []string {
	var images []string

	switch v := data.(type) {
	case map[string]interface{}:
		// Check for image fields at this level
		if img, ok := v["image"].(string); ok && img != "" && isValidImageRef(img) {
			images = append(images, img)
		}

		// Recursively check all values
		for _, value := range v {
			childImages := extractImagesFromParsedYAML(value)
			images = append(images, childImages...)
		}

	case []interface{}:
		// Recursively check array elements
		for _, item := range v {
			childImages := extractImagesFromParsedYAML(item)
			images = append(images, childImages...)
		}

	case string:
		// Check if the string itself is an image reference
		if isValidImageRef(v) {
			images = append(images, v)
		}
	}

	return images
}

// isValidImageRef checks if a string looks like a container image reference.
func isValidImageRef(s string) bool {
	if s == "" {
		return false
	}

	// Must contain a registry (domain with dots or localhost)
	if !strings.Contains(s, "/") {
		return false
	}

	// Should contain a digest (@sha256:) or tag (:)
	hasDigest := strings.Contains(s, "@sha256:")
	hasTag := strings.Contains(s, ":") && !hasDigest

	if !hasDigest && !hasTag {
		return false
	}

	// Must look like a registry path
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return false
	}

	// First part should look like a registry (contain dots or be localhost)
	registry := parts[0]
	if !strings.Contains(registry, ".") && registry != "localhost" {
		return false
	}

	return true
}

// deduplicateStrings removes duplicate strings from a slice.
func deduplicateStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}
