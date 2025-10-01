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

// GenerateFromBundle generates manifests for a bundle image
func (g *Generator) GenerateFromBundle(ctx context.Context) (*ManifestSet, error) {
	// TODO: Implement bundle-to-catalog conversion
	// This will:
	// 1. Generate an FBC template
	// 2. Call opm to render the catalog
	// 3. Build a catalog image locally
	// 4. Then generate manifests as with catalog

	return nil, fmt.Errorf("bundle support not yet implemented")
}
