package analysis

import (
	"context"
	"fmt"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/execregistry"
	"github.com/sirupsen/logrus"
)

// ExtractImageReferences extracts all image references from a bundle image by directly inspecting it.
func ExtractImageReferences(ctx context.Context, bundleRef ImageRef) ([]string, error) {
	// Suppress noisy INFO logs from operator-registry library
	logrus.SetLevel(logrus.WarnLevel)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)

	// Create registry client
	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating image registry: %w", err)
	}
	defer registry.Destroy()

	// Create migrations
	migs, err := migrations.NewMigrations("bundle-object-to-csv-metadata")
	if err != nil {
		return nil, fmt.Errorf("creating migrations: %w", err)
	}

	// Render the bundle to get its declarative config
	r := action.Render{
		Refs:           []string{bundleRef.String()},
		Registry:       registry,
		AllowedRefMask: action.RefBundleImage,
		Migrations:     migs,
	}

	cfg, err := r.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("rendering bundle: %w", err)
	}

	// Extract image references from all bundles
	var images []string
	for _, bundle := range cfg.Bundles {
		// Add the bundle image itself
		if bundle.Image != "" {
			images = append(images, bundle.Image)
		}

		// Add all related images
		for _, relatedImage := range bundle.RelatedImages {
			if relatedImage.Image != "" {
				images = append(images, relatedImage.Image)
			}
		}
	}

	return deduplicateStrings(images), nil
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
