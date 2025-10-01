package bundle

import (
	"context"
	"fmt"

	"sigs.k8s.io/yaml"
)

// Artifacts contains generated files for building a catalog from a bundle
type Artifacts struct {
	FBCTemplate  string // FBC template YAML
	CatalogYAML  string // Rendered catalog (if opm is available)
	Dockerfile   string // Dockerfile for building catalog image
	Instructions string // Build and deploy instructions
}

// Generator handles bundle to catalog conversion
type Generator struct {
	bundleImage string
	channel     string
	ompBinPath  string // Optional path to external opm binary
}

// NewGenerator creates a new bundle generator
func NewGenerator(bundleImage, channel string) *Generator {
	if channel == "" {
		channel = "preview"
	}
	return &Generator{
		bundleImage: bundleImage,
		channel:     channel,
	}
}

// NewGeneratorWithOpm creates a new bundle generator with external opm binary
func NewGeneratorWithOmp(bundleImage, channel, ompBinPath string) *Generator {
	if channel == "" {
		channel = "preview"
	}
	return &Generator{
		bundleImage: bundleImage,
		channel:     channel,
		ompBinPath:  ompBinPath,
	}
}

// Generate creates all artifacts needed to build a catalog from a bundle
func (g *Generator) Generate(ctx context.Context) (*Artifacts, error) {
	// Generate FBC template
	fbcTemplate, err := GenerateFBCTemplate(g.bundleImage, g.channel)
	if err != nil {
		return nil, fmt.Errorf("generating FBC template: %w", err)
	}

	// Marshal template to YAML
	fbcYAML, err := yaml.Marshal(fbcTemplate)
	if err != nil {
		return nil, fmt.Errorf("marshaling FBC template: %w", err)
	}

	artifacts := &Artifacts{
		FBCTemplate: string(fbcYAML),
		Dockerfile:  GenerateCatalogDockerfile(),
	}

	// Render the catalog using either binary or library
	var catalogYAML string
	if g.ompBinPath != "" {
		catalogYAML, err = RenderCatalogWithBinary(ctx, fbcTemplate, g.ompBinPath)
	} else {
		catalogYAML, err = RenderCatalog(ctx, fbcTemplate)
	}

	if err != nil {
		// If rendering fails, provide instructions for manual rendering
		artifacts.Instructions = GenerateBuildInstructions(".", true, g.bundleImage)
		artifacts.Instructions = fmt.Sprintf("WARNING: Could not render catalog automatically: %v\n\n%s", err, artifacts.Instructions)
	} else {
		artifacts.CatalogYAML = catalogYAML
		artifacts.Instructions = GenerateBuildInstructions(".", false, g.bundleImage)
	}

	return artifacts, nil
}
