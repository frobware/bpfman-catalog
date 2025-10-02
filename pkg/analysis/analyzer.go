package analysis

import (
	"context"
	"fmt"
)

// AnalyzeBundle performs comprehensive analysis of a bundle image.
func AnalyzeBundle(ctx context.Context, bundleRefStr string) (*BundleAnalysis, error) {
	bundleRef, err := ParseImageRef(bundleRefStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bundle reference: %w", err)
	}

	analysis := &BundleAnalysis{
		BundleRef: bundleRef,
		Images:    []ImageResult{},
	}

	// Extract bundle metadata
	bundleInfo, err := extractBundleMetadata(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bundle metadata: %w", err)
	}
	analysis.BundleInfo = bundleInfo

	// Extract image references from the bundle
	imageRefs, err := ExtractImageReferences(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	// Inspect each image reference
	imageResults, err := InspectImages(ctx, imageRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect images: %w", err)
	}
	analysis.Images = imageResults

	// Generate summary statistics
	analysis.Summary = CalculateSummary(imageResults)

	return analysis, nil
}

// extractBundleMetadata extracts metadata from the bundle image itself.
func extractBundleMetadata(ctx context.Context, bundleRef ImageRef) (*ImageInfo, error) {
	// Try to inspect the bundle directly
	if info, err := ExtractImageMetadata(ctx, bundleRef); err == nil {
		return info, nil
	}

	// Try tenant workspace conversion if direct access fails
	tenantRef, err := bundleRef.ConvertToTenantWorkspace()
	if err != nil {
		return nil, fmt.Errorf("bundle not accessible and cannot convert to tenant workspace: %w", err)
	}

	info, err := ExtractImageMetadata(ctx, tenantRef)
	if err != nil {
		return nil, fmt.Errorf("bundle not accessible in any registry: %w", err)
	}

	return info, nil
}

// AnalyzeConfig holds configuration options for bundle analysis.
type AnalyzeConfig struct {
	ShowAll bool // Include inaccessible images in results
}

// AnalyzeBundleWithConfig performs analysis with specific configuration options.
func AnalyzeBundleWithConfig(ctx context.Context, bundleRefStr string, config AnalyzeConfig) (*BundleAnalysis, error) {
	analysis, err := AnalyzeBundle(ctx, bundleRefStr)
	if err != nil {
		return nil, err
	}

	// Filter results based on configuration
	if !config.ShowAll {
		filteredImages := make([]ImageResult, 0, len(analysis.Images))
		for _, img := range analysis.Images {
			if img.Accessible {
				filteredImages = append(filteredImages, img)
			}
		}
		analysis.Images = filteredImages

		// Recalculate summary for filtered results
		analysis.Summary = CalculateSummary(filteredImages)
	}

	return analysis, nil
}
