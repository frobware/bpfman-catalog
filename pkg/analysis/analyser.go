package analysis

import (
	"context"
	"fmt"
)

// AnalyseBundle performs comprehensive analysis of a bundle image.
func AnalyseBundle(ctx context.Context, bundleRefStr string) (*BundleAnalysis, error) {
	bundleRef, err := ParseImageRef(bundleRefStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bundle reference: %w", err)
	}

	analysis := &BundleAnalysis{
		BundleRef: bundleRef,
		Images:    []ImageResult{},
	}

	// Extract bundle metadata.
	bundleInfo, err := extractBundleMetadata(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bundle metadata: %w", err)
	}
	analysis.BundleInfo = bundleInfo

	// Extract image references from the bundle.
	imageRefs, err := ExtractImageReferences(ctx, bundleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image references: %w", err)
	}

	// Inspect each image reference.
	imageResults := make([]ImageResult, len(imageRefs))
	for i, ref := range imageRefs {
		result, err := InspectImage(ctx, ref)
		if err != nil {
			imageResults[i] = ImageResult{
				Reference:  ref,
				Accessible: false,
				Registry:   NotAccessible,
				Error:      fmt.Sprintf("inspection failed: %v", err),
			}
		} else {
			imageResults[i] = *result
		}
	}
	analysis.Images = imageResults

	// Generate summary statistics.
	analysis.Summary = CalculateSummary(imageResults)

	return analysis, nil
}

// extractBundleMetadata extracts metadata from the bundle image
// itself.
func extractBundleMetadata(ctx context.Context, bundleRef ImageRef) (*ImageInfo, error) {
	// Try to inspect the bundle directly.
	if info, err := ExtractImageMetadata(ctx, bundleRef); err == nil {
		return info, nil
	}

	// Try tenant workspace conversion if direct access fails.
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

// AnalyseConfig holds configuration options for bundle analysis.
type AnalyseConfig struct {
	ShowAll bool // Include inaccessible images in results.
}

// AnalyseBundleWithConfig performs analysis with specific
// configuration options.
