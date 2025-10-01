package manifests

import (
	"context"
	"fmt"

	"github.com/openshift/bpfman-catalog/pkg/catalog"
)

// GeneratorConfig contains configuration for manifest generation
type GeneratorConfig struct {
	ImageRef      string // Catalog or bundle image reference
	Namespace     string // Target namespace (default: bpfman)
	UseDigestName bool   // Whether to suffix resources with digest
}

// Generator creates Kubernetes manifests
type Generator struct {
	config GeneratorConfig
}

// NewGenerator creates a new manifest generator
func NewGenerator(config GeneratorConfig) *Generator {
	if config.Namespace == "" {
		config.Namespace = "bpfman"
	}
	config.UseDigestName = true // Always use digest naming for clarity

	return &Generator{
		config: config,
	}
}

// GenerateFromCatalog generates manifests for a catalog image
func (g *Generator) GenerateFromCatalog(ctx context.Context) (*ManifestSet, error) {
	// Extract metadata from the catalog image
	meta, err := catalog.ExtractMetadata(ctx, g.config.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("extracting catalog metadata: %w", err)
	}

	// Create catalog metadata for manifest generation
	catalogMeta := CatalogMetadata{
		Image:       g.config.ImageRef,
		Digest:      string(meta.Digest),
		ShortDigest: meta.ShortDigest,
		CatalogType: meta.CatalogType,
		Version:     meta.Version,
	}

	// Generate all manifests
	manifestSet := &ManifestSet{}

	// Determine if we should use digest suffix
	digestSuffix := ""
	if g.config.UseDigestName && meta.ShortDigest != "" {
		digestSuffix = meta.ShortDigest
	}

	// Generate namespace
	manifestSet.Namespace = NewNamespace(g.config.Namespace, digestSuffix)

	// Generate IDMS
	manifestSet.IDMS = NewImageDigestMirrorSet(digestSuffix)

	// Generate CatalogSource
	manifestSet.CatalogSource = NewCatalogSource(catalogMeta)

	// Generate OperatorGroup (uses the potentially suffixed namespace name)
	namespaceName := manifestSet.Namespace.ObjectMeta.Name
	manifestSet.OperatorGroup = NewOperatorGroup(namespaceName, digestSuffix)

	// Generate Subscription
	catalogSourceName := manifestSet.CatalogSource.ObjectMeta.Name
	manifestSet.Subscription = NewSubscription(namespaceName, catalogSourceName, digestSuffix)

	return manifestSet, nil
}

// GenerateFromBundle generates artifacts for a bundle image
// This creates FBC template, catalog.yaml, and Dockerfile but does NOT build the image
func (g *Generator) GenerateFromBundle(ctx context.Context) (*BundleArtifacts, error) {
	// For bundles, we generate artifacts for the user to build their own catalog
	// We don't generate deployment manifests directly since they need to build and push first

	return nil, fmt.Errorf("bundle support should use GenerateBundleArtifacts instead")
}

// BundleArtifacts contains generated files for building a catalog from a bundle
type BundleArtifacts struct {
	FBCTemplate  string // FBC template YAML
	CatalogYAML  string // Rendered catalog (if opm is available)
	Dockerfile   string // Dockerfile for building catalog image
	Instructions string // Build and deploy instructions
}
