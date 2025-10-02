package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/types"
)

// InspectImage performs comprehensive inspection of a single image reference.
func InspectImage(ctx context.Context, imageRefStr string) (*ImageResult, error) {
	imageRef, err := ParseImageRef(imageRefStr)
	if err != nil {
		return &ImageResult{
			Reference:  imageRefStr,
			Accessible: false,
			Registry:   NotAccessible,
			Error:      fmt.Sprintf("invalid image reference: %v", err),
		}, nil
	}

	result := &ImageResult{
		Reference: imageRefStr,
	}

	// Try the image at its specified registry
	if info, err := inspectImageRef(ctx, imageRef); err == nil {
		result.Accessible = true
		// Determine registry type based on the actual registry and repository
		if imageRef.Registry == "registry.redhat.io" {
			result.Registry = DownstreamRegistry
		} else if imageRef.Registry == "quay.io" && strings.Contains(imageRef.Repo, "redhat-user-workloads") {
			result.Registry = TenantWorkspace
		} else {
			result.Registry = DownstreamRegistry // Default for other registries
		}
		result.Info = convertToImageInfo(info)
		return result, nil
	}

	// Try tenant workspace conversion
	tenantRef, err := imageRef.ConvertToTenantWorkspace()
	if err != nil {
		result.Accessible = false
		result.Registry = NotAccessible
		result.Error = "not accessible in any registry"
		return result, nil
	}

	if info, err := inspectImageRef(ctx, tenantRef); err == nil {
		result.Accessible = true
		result.Registry = TenantWorkspace
		result.Info = convertToImageInfo(info)
		return result, nil
	}

	result.Accessible = false
	result.Registry = NotAccessible
	result.Error = "not accessible in downstream or tenant registry"
	return result, nil
}

// inspectImageRef inspects a specific image reference and returns metadata.
func inspectImageRef(ctx context.Context, imageRef ImageRef) (*types.ImageInspectInfo, error) {
	ref, err := docker.ParseReference("//" + imageRef.String())
	if err != nil {
		return nil, fmt.Errorf("failed to parse reference: %w", err)
	}

	systemCtx := &types.SystemContext{}
	img, err := ref.NewImage(ctx, systemCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create image: %w", err)
	}
	defer img.Close()

	return img.Inspect(ctx)
}

// convertToImageInfo converts types.ImageInspectInfo to our ImageInfo structure.
func convertToImageInfo(info *types.ImageInspectInfo) *ImageInfo {
	if info == nil {
		return nil
	}

	imageInfo := &ImageInfo{}

	// Extract creation time
	if info.Created != nil {
		imageInfo.Created = info.Created
	}

	// Extract metadata from labels
	if info.Labels != nil {
		imageInfo.Version = info.Labels["version"]

		// Git-related labels (common patterns)
		if commit := info.Labels["io.openshift.build.commit.id"]; commit != "" {
			imageInfo.GitCommit = commit
		} else if commit := info.Labels["vcs-ref"]; commit != "" {
			imageInfo.GitCommit = commit
		}

		if url := info.Labels["io.openshift.build.source-location"]; url != "" {
			imageInfo.GitURL = url
		} else if url := info.Labels["vcs-url"]; url != "" {
			imageInfo.GitURL = url
		}

		// Additional metadata that might be useful
		if buildName := info.Labels["io.openshift.build.name"]; buildName != "" {
			// Sometimes build name contains useful information
			imageInfo.PRTitle = buildName
		}
	}

	return imageInfo
}

// InspectImages performs batch inspection of multiple image references.
func InspectImages(ctx context.Context, imageRefs []string) ([]ImageResult, error) {
	results := make([]ImageResult, len(imageRefs))

	for i, ref := range imageRefs {
		result, err := InspectImage(ctx, ref)
		if err != nil {
			results[i] = ImageResult{
				Reference:  ref,
				Accessible: false,
				Registry:   NotAccessible,
				Error:      fmt.Sprintf("inspection failed: %v", err),
			}
		} else {
			results[i] = *result
		}
	}

	return results, nil
}

// CalculateSummary generates summary statistics from image results.
func CalculateSummary(results []ImageResult) Summary {
	summary := Summary{
		TotalImages: len(results),
	}

	for _, result := range results {
		if result.Accessible {
			summary.AccessibleImages++
			switch result.Registry {
			case DownstreamRegistry:
				summary.DownstreamImages++
			case TenantWorkspace:
				summary.TenantImages++
			}
		} else {
			summary.InaccessibleImages++
		}
	}

	return summary
}
