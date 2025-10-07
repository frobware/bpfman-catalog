package manifests

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/bpfman-catalog/pkg/catalog"
)

// GeneratorConfig contains configuration for manifest generation
type GeneratorConfig struct {
	ImageRef      string // Catalog or bundle image reference
	Namespace     string // Target namespace (default: bpfman)
	UseDigestName bool   // Whether to suffix resources with digest
}

// LabelContext contains labeling information for consistent resource labeling
type LabelContext struct {
	ShortDigest    string            // Digest suffix for unique identification
	StandardLabels map[string]string // Standard labels applied to all resources
	CustomLabels   map[string]string // Additional custom labels
}

// Generator creates Kubernetes manifests
type Generator struct {
	config       GeneratorConfig
	labelContext *LabelContext
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

// setupLabelContext initializes the label context with digest and standard labels
func (g *Generator) setupLabelContext(shortDigest string) {
	standardLabels := map[string]string{
		"app.kubernetes.io/name":       "bpfman-operator",
		"app.kubernetes.io/created-by": "bpfman-catalog-cli",
		"app.kubernetes.io/version":    "latest", // Could be made configurable
	}

	// Add our digest-based identification labels
	if shortDigest != "" {
		standardLabels["bpfman-catalog-cli"] = shortDigest
		standardLabels["bpfman-catalog-cli/digest"] = shortDigest
	}

	g.labelContext = &LabelContext{
		ShortDigest:    shortDigest,
		StandardLabels: standardLabels,
		CustomLabels:   make(map[string]string),
	}
}

// generateResourceName creates a resource name with optional digest suffix
func (g *Generator) generateResourceName(baseName string) string {
	if g.labelContext.ShortDigest != "" {
		return fmt.Sprintf("%s-sha-%s", baseName, g.labelContext.ShortDigest)
	}
	return baseName
}

// getMergedLabels returns standard labels merged with any custom labels and additional labels
func (g *Generator) getMergedLabels(additionalLabels map[string]string) map[string]string {
	merged := make(map[string]string)

	// Start with standard labels
	for k, v := range g.labelContext.StandardLabels {
		merged[k] = v
	}

	// Add custom labels
	for k, v := range g.labelContext.CustomLabels {
		merged[k] = v
	}

	// Add any additional labels (e.g., resource-specific ones)
	for k, v := range additionalLabels {
		merged[k] = v
	}

	return merged
}

// NewNamespace creates a namespace manifest with consistent labeling
func (g *Generator) NewNamespace(baseName string) *Namespace {
	// Namespace-specific labels (like monitoring)
	additionalLabels := map[string]string{
		"openshift.io/cluster-monitoring": "true",
	}

	return &Namespace{
		TypeMeta: TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: ObjectMeta{
			Name:   baseName, // Use baseName directly, no digest suffix for namespace
			Labels: g.getMergedLabels(additionalLabels),
		},
	}
}

// NewCatalogSource creates a catalog source manifest with consistent labeling
func (g *Generator) NewCatalogSource(meta CatalogMetadata) *CatalogSource {
	return &CatalogSource{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "CatalogSource",
		},
		ObjectMeta: ObjectMeta{
			Name:      g.generateResourceName("bpfman-catalogsource"),
			Namespace: "openshift-marketplace",
			Labels:    g.getMergedLabels(nil),
		},
		Spec: func() CatalogSourceSpec {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			return CatalogSourceSpec{
				SourceType:  "grpc",
				Image:       meta.Image,
				DisplayName: fmt.Sprintf("bpfman-catalog CLI (ephemeral-%s - %s)", meta.ShortDigest, timestamp),
				Publisher:   fmt.Sprintf("bpfman-catalog CLI (ephemeral-%s - %s)", meta.ShortDigest, timestamp),
			}
		}(),
	}
}

// NewOperatorGroup creates an operator group manifest with consistent labeling
func (g *Generator) NewOperatorGroup(namespace string) *OperatorGroup {
	return &OperatorGroup{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1",
			Kind:       "OperatorGroup",
		},
		ObjectMeta: ObjectMeta{
			Name:      g.generateResourceName("bpfman-operatorgroup"),
			Namespace: namespace,
			Labels:    g.getMergedLabels(nil),
		},
		Spec: OperatorGroupSpec{},
	}
}

// NewSubscription creates a subscription manifest with consistent labeling
func (g *Generator) NewSubscription(namespace, catalogSourceName, channel string) *Subscription {
	return &Subscription{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "Subscription",
		},
		ObjectMeta: ObjectMeta{
			Name:      g.generateResourceName("bpfman-subscription"),
			Namespace: namespace,
			Labels:    g.getMergedLabels(nil),
		},
		Spec: SubscriptionSpec{
			Channel:             channel,
			Name:                "bpfman-operator",
			Source:              catalogSourceName,
			SourceNamespace:     "openshift-marketplace",
			InstallPlanApproval: "Automatic",
		},
	}
}

// NewImageDigestMirrorSet creates an IDMS manifest with consistent labeling
func (g *Generator) NewImageDigestMirrorSet() *ImageDigestMirrorSet {
	return &ImageDigestMirrorSet{
		TypeMeta: TypeMeta{
			APIVersion: "config.openshift.io/v1",
			Kind:       "ImageDigestMirrorSet",
		},
		ObjectMeta: ObjectMeta{
			Name:   g.generateResourceName("bpfman-idms"),
			Labels: g.getMergedLabels(nil),
		},
		Spec: ImageDigestMirrorSetSpec{
			ImageDigestMirrors: []ImageDigestMirror{
				{
					Source: "registry.redhat.io/openshift4",
					Mirrors: []string{
						"registry.stage.redhat.io/openshift4",
						"registry-proxy.engineering.redhat.com/rh-osbs/openshift4",
					},
				},
			},
		},
	}
}

// GenerateFromCatalog generates manifests for a catalog image
func (g *Generator) GenerateFromCatalog(ctx context.Context) (*ManifestSet, error) {
	// Extract metadata from the catalog image
	meta, err := catalog.ExtractMetadata(ctx, g.config.ImageRef)
	if err != nil {
		return nil, fmt.Errorf("extracting catalog metadata: %w", err)
	}

	// Setup label context for consistent labeling
	digestSuffix := ""
	if g.config.UseDigestName && meta.ShortDigest != "" {
		digestSuffix = meta.ShortDigest
	}
	g.setupLabelContext(digestSuffix)

	// Create catalog metadata for manifest generation
	// Use the digest-based image reference for the actual catalog deployment
	digestRef := meta.GetDigestRef()
	catalogMeta := CatalogMetadata{
		Image:       digestRef,
		Digest:      string(meta.Digest),
		ShortDigest: meta.ShortDigest,
		CatalogType: meta.CatalogType,
	}

	// Generate all manifests
	manifestSet := &ManifestSet{}

	// Generate namespace
	manifestSet.Namespace = g.NewNamespace(g.config.Namespace)

	// Generate IDMS
	manifestSet.IDMS = g.NewImageDigestMirrorSet()

	// Generate CatalogSource
	manifestSet.CatalogSource = g.NewCatalogSource(catalogMeta)

	// Generate OperatorGroup (uses the potentially suffixed namespace name)
	namespaceName := manifestSet.Namespace.ObjectMeta.Name
	manifestSet.OperatorGroup = g.NewOperatorGroup(namespaceName)

	// Generate Subscription - use the detected default channel
	catalogSourceName := manifestSet.CatalogSource.ObjectMeta.Name
	channel := meta.DefaultChannel
	if channel == "" {
		return nil, fmt.Errorf("no default channel found in catalog metadata")
	}
	manifestSet.Subscription = g.NewSubscription(namespaceName, catalogSourceName, channel)

	return manifestSet, nil
}

// GenerateFromBundle generates artefacts for a bundle image
// This creates FBC template, catalog.yaml, and Dockerfile but does NOT build the image
func (g *Generator) GenerateFromBundle(ctx context.Context) (*BundleArtefacts, error) {
	// For bundles, we generate artefacts for the user to build their own catalog
	// We don't generate deployment manifests directly since they need to build and push first

	return nil, fmt.Errorf("bundle support should use GenerateBundleArtefacts instead")
}

// BundleArtefacts contains generated files for building a catalog from a bundle
type BundleArtefacts struct {
	FBCTemplate  string // FBC template YAML
	CatalogYAML  string // Rendered catalog (if opm is available)
	Dockerfile   string // Dockerfile for building catalog image
	Instructions string // Build and deploy instructions
}
