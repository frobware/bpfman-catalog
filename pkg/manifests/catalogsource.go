package manifests

import (
	"fmt"
	"strings"
)

// CatalogMetadata contains information extracted from a catalog image
type CatalogMetadata struct {
	Image       string
	Digest      string // sha256:abc123...
	ShortDigest string // First 8 chars of digest
	CatalogType string // catalog-ystream, catalog-zstream, etc
	Version     string // 4.19, 4.20, etc
}

// NewCatalogSource creates a CatalogSource manifest
func NewCatalogSource(meta CatalogMetadata) *CatalogSource {
	name := generateCatalogName(meta.ShortDigest)
	displayName := generateDisplayName(meta)

	return &CatalogSource{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "CatalogSource",
		},
		ObjectMeta: ObjectMeta{
			Name:      name,
			Namespace: "openshift-marketplace",
		},
		Spec: CatalogSourceSpec{
			SourceType:  "grpc",
			Image:       meta.Image,
			DisplayName: displayName,
			Publisher:   "Red Hat",
		},
	}
}

// generateCatalogName creates a unique catalog name using the digest
func generateCatalogName(shortDigest string) string {
	if shortDigest == "" {
		return "fbc-bpfman-catalogsource"
	}
	return fmt.Sprintf("fbc-bpfman-catalogsource-%s", shortDigest)
}

// generateDisplayName creates a human-readable display name
func generateDisplayName(meta CatalogMetadata) string {
	catalogType := formatCatalogType(meta.CatalogType)

	if meta.Version != "" {
		if meta.ShortDigest != "" {
			return fmt.Sprintf("Bpfman %s v%s-%s", catalogType, meta.Version, meta.ShortDigest)
		}
		return fmt.Sprintf("Bpfman %s v%s", catalogType, meta.Version)
	}

	if meta.ShortDigest != "" {
		return fmt.Sprintf("Bpfman %s-%s", catalogType, meta.ShortDigest)
	}

	return fmt.Sprintf("Bpfman %s", catalogType)
}

// formatCatalogType converts catalog type to display format
func formatCatalogType(catalogType string) string {
	switch catalogType {
	case "catalog-ystream":
		return "Y-stream"
	case "catalog-zstream":
		return "Z-stream"
	case "":
		return "Catalog"
	default:
		// Clean up the catalog type for display
		return strings.TrimPrefix(catalogType, "catalog-")
	}
}
