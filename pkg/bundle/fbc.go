package bundle

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"

	"github.com/operator-framework/operator-registry/alpha/action"
	"github.com/operator-framework/operator-registry/alpha/action/migrations"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/template/basic"
	"github.com/operator-framework/operator-registry/pkg/containertools"
	"github.com/operator-framework/operator-registry/pkg/image/execregistry"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

//go:embed templates/Makefile.tmpl
var makefileTemplate string

//go:embed templates/WORKFLOW.txt.tmpl
var workflowTemplate string

// GenerateImageUUIDAndTTL generates a double UUID and random TTL for ttl.sh examples
func GenerateImageUUIDAndTTL() (string, string) {
	imageUUID := fmt.Sprintf("%s-%s", uuid.New().String(), uuid.New().String())
	randomTTL := generateRandomTTL()
	return imageUUID, randomTTL
}

// generateRandomTTL generates a random TTL between 15m and 30m for ttl.sh
func generateRandomTTL() string {
	// Seed random number generator with current time
	rand.Seed(time.Now().UnixNano())

	// Generate random duration between 15 and 30 minutes
	minMinutes := 15
	maxMinutes := 30
	randomMinutes := rand.Intn(maxMinutes-minMinutes+1) + minMinutes

	return fmt.Sprintf("%dm", randomMinutes)
}

// BundleInfo contains extracted bundle metadata
type BundleInfo struct {
	Name    string
	Package string
}

// FBCTemplate represents a File-Based Catalog template
type FBCTemplate struct {
	Schema  string        `yaml:"schema"`
	Entries []interface{} `yaml:"entries"`
}

// PackageEntry defines an OLM package
type PackageEntry struct {
	Schema         string `yaml:"schema"`
	Name           string `yaml:"name"`
	DefaultChannel string `yaml:"defaultChannel"`
}

// ChannelEntry defines an OLM channel
type ChannelEntry struct {
	Schema  string             `yaml:"schema"`
	Package string             `yaml:"package"`
	Name    string             `yaml:"name"`
	Entries []ChannelEntryItem `yaml:"entries"`
}

// ChannelEntryItem represents an entry in a channel
type ChannelEntryItem struct {
	Name     string `yaml:"name"`
	Replaces string `yaml:"replaces,omitempty"`
}

// BundleEntry defines an OLM bundle
type BundleEntry struct {
	Schema string `yaml:"schema"`
	Image  string `yaml:"image"`
	Name   string `yaml:"name"`
}

// GenerateFBCTemplate generates an FBC template for a bundle image
func GenerateFBCTemplate(bundleImage string, channel string) (*FBCTemplate, error) {
	if bundleImage == "" {
		return nil, fmt.Errorf("bundle image cannot be empty")
	}

	if channel == "" {
		channel = "preview"
	}

	// Keep channel simple - always use "preview"

	// Extract bundle metadata to get the real bundle name
	bundleInfo, err := extractBundleInfo(bundleImage)
	if err != nil {
		return nil, fmt.Errorf("extracting bundle info: %w", err)
	}

	bundleName := bundleInfo.Name
	packageName := bundleInfo.Package

	template := &FBCTemplate{
		Schema: "olm.template.basic",
		Entries: []interface{}{
			PackageEntry{
				Schema:         "olm.package",
				Name:           packageName,
				DefaultChannel: channel,
			},
			ChannelEntry{
				Schema:  "olm.channel",
				Package: packageName,
				Name:    channel,
				Entries: []ChannelEntryItem{
					{Name: bundleName},
				},
			},
			BundleEntry{
				Schema: "olm.bundle",
				Image:  bundleImage,
				Name:   bundleName,
			},
		},
	}

	return template, nil
}

// GenerateMultiBundleFBCTemplate generates an FBC template for multiple bundle images
func GenerateMultiBundleFBCTemplate(ctx context.Context, bundles []*BundleMetadata, channel string) (*FBCTemplate, error) {
	if len(bundles) == 0 {
		return nil, fmt.Errorf("no bundles provided")
	}

	if channel == "" {
		channel = "preview"
	}

	// Extract bundle info for all bundles
	var bundleInfos []struct {
		info  *BundleInfo
		image string
		meta  *BundleMetadata
	}

	for _, b := range bundles {
		info, err := extractBundleInfo(b.Image)
		if err != nil {
			return nil, fmt.Errorf("extracting bundle info for %s: %w", b.Image, err)
		}
		bundleInfos = append(bundleInfos, struct {
			info  *BundleInfo
			image string
			meta  *BundleMetadata
		}{info, b.Image, b})
	}

	// Use package name from first bundle
	packageName := bundleInfos[0].info.Package

	// Build channel entries with replaces chain
	var channelEntries []ChannelEntryItem
	for i, bi := range bundleInfos {
		// Generate catalog entry name: bpfman-operator.v{VERSION}-g{SHORT_SHA}-{TIMESTAMP}
		catalogName := generateCatalogEntryName(bi.meta)

		entry := ChannelEntryItem{
			Name: catalogName,
		}

		// Add replaces field for all except the oldest
		if i > 0 {
			prevName := generateCatalogEntryName(bundleInfos[i-1].meta)
			entry.Replaces = prevName
		}

		channelEntries = append(channelEntries, entry)
	}

	// Build bundle entries
	var entries []interface{}
	entries = append(entries, PackageEntry{
		Schema:         "olm.package",
		Name:           packageName,
		DefaultChannel: channel,
	})

	entries = append(entries, ChannelEntry{
		Schema:  "olm.channel",
		Package: packageName,
		Name:    channel,
		Entries: channelEntries,
	})

	for _, bi := range bundleInfos {
		catalogName := generateCatalogEntryName(bi.meta)
		entries = append(entries, BundleEntry{
			Schema: "olm.bundle",
			Image:  bi.image,
			Name:   catalogName,
		})
	}

	template := &FBCTemplate{
		Schema:  "olm.template.basic",
		Entries: entries,
	}

	return template, nil
}

// generateCatalogEntryName generates a catalog entry name from bundle metadata
// Format: bpfman-operator.v{VERSION}-g{SHORT_SHA}-{TIMESTAMP}
func generateCatalogEntryName(meta *BundleMetadata) string {
	// Use version as-is from metadata
	version := meta.Version

	// Get short SHA (first 8 chars)
	shortSHA := meta.Tag
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}

	// Format timestamp as YYYY-MM-DDTHHMM (keep date hyphens, remove time colons)
	timestamp := ""
	if meta.BuildDate != "" {
		// BuildDate format: 2025-10-02T12:05:37Z
		// Extract date and time parts
		if len(meta.BuildDate) >= 16 {
			datePart := meta.BuildDate[:10]                        // 2025-10-02
			timePart := meta.BuildDate[11:16]                      // 12:05
			timeFormatted := strings.ReplaceAll(timePart, ":", "") // 1205
			timestamp = fmt.Sprintf("%sT%s", datePart, timeFormatted)
		}
	}

	return fmt.Sprintf("bpfman-operator.v%s-g%s-%s", version, shortSHA, timestamp)
}

// MarshalFBCTemplate marshals an FBC template to YAML
func MarshalFBCTemplate(template *FBCTemplate) (string, error) {
	data, err := yaml.Marshal(template)
	if err != nil {
		return "", fmt.Errorf("marshaling FBC template: %w", err)
	}
	return string(data), nil
}

// RenderCatalog uses the OPM library to render the FBC template into a full catalog.
func RenderCatalog(ctx context.Context, fbcTemplate *FBCTemplate) (string, error) {
	templateYAML, err := yaml.Marshal(fbcTemplate)
	if err != nil {
		return "", fmt.Errorf("marshaling FBC template: %w", err)
	}

	// Suppress noisy INFO logs from operator-registry library
	logrus.SetLevel(logrus.WarnLevel)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)

	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		return "", fmt.Errorf("creating image registry: %w", err)
	}

	template := basic.Template{
		RenderBundle: func(ctx context.Context, image string) (*declcfg.DeclarativeConfig, error) {
			migs, err := migrations.NewMigrations("bundle-object-to-csv-metadata")
			if err != nil {
				return nil, fmt.Errorf("creating migrations: %w", err)
			}

			r := action.Render{
				Refs:           []string{image},
				Registry:       registry,
				AllowedRefMask: action.RefBundleImage,
				Migrations:     migs,
			}
			return r.Run(ctx)
		},
	}

	reader := bytes.NewReader(templateYAML)
	cfg, err := template.Render(ctx, reader)
	if err != nil {
		return "", fmt.Errorf("rendering template: %w", err)
	}

	var buf bytes.Buffer
	if err := declcfg.WriteYAML(*cfg, &buf); err != nil {
		return "", fmt.Errorf("writing catalog YAML: %w", err)
	}

	return buf.String(), nil
}

// GenerateCatalogDockerfile generates a Dockerfile for building a catalog image
func GenerateCatalogDockerfile() string {
	return `# Catalog Dockerfile for bpfman-operator
# Generated by bpfman-catalog tool
#
# Build with:
#   podman build -f Dockerfile.catalog -t my-catalog:dev .
#
# Push to a registry:
#   podman push my-catalog:dev ttl.sh/my-catalog:dev

FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.20 AS builder

# Copy the rendered catalog
COPY catalog.yaml /configs/catalog.yaml

# Validate the catalog
RUN opm validate /configs

# Final stage - minimal image with just the catalog
FROM registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.20

# Copy validated catalog from builder
COPY --from=builder /configs /configs

# Label for OpenShift
LABEL io.openshift.release.operator=true

# Serve the catalog
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs"]
`
}

// extractDigestSuffix extracts the first 8 characters of a digest from an image reference
func extractDigestSuffix(imageRef string) string {
	// Look for @sha256: pattern
	if idx := strings.Index(imageRef, "@sha256:"); idx != -1 {
		digestStr := imageRef[idx+8:] // Skip "@sha256:"
		if len(digestStr) >= 8 {
			return digestStr[:8] // First 8 chars of digest
		}
	}
	return ""
}

// extractBundleInfo extracts bundle name and package from bundle metadata
func extractBundleInfo(bundleImage string) (*BundleInfo, error) {
	// Suppress noisy INFO logs from operator-registry library
	logrus.SetLevel(logrus.WarnLevel)

	logger := logrus.NewEntry(logrus.New())
	logger.Logger.SetLevel(logrus.WarnLevel)

	registry, err := execregistry.NewRegistry(containertools.PodmanTool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating image registry: %w", err)
	}

	migs, err := migrations.NewMigrations("bundle-object-to-csv-metadata")
	if err != nil {
		return nil, fmt.Errorf("creating migrations: %w", err)
	}

	r := action.Render{
		Refs:           []string{bundleImage},
		Registry:       registry,
		AllowedRefMask: action.RefBundleImage,
		Migrations:     migs,
	}

	cfg, err := r.Run(context.Background())
	if err != nil {
		return nil, fmt.Errorf("rendering bundle: %w", err)
	}

	// Find the bundle entry in the rendered config
	for _, bundle := range cfg.Bundles {
		if bundle.Image == bundleImage {
			return &BundleInfo{
				Name:    bundle.Name,
				Package: bundle.Package,
			}, nil
		}
	}

	return nil, fmt.Errorf("bundle not found in rendered config")
}

// RenderCatalogWithBinary uses an external opm binary to render the FBC template into a full catalog.
func RenderCatalogWithBinary(ctx context.Context, fbcTemplate *FBCTemplate, ompBinPath string) (string, error) {
	templateYAML, err := yaml.Marshal(fbcTemplate)
	if err != nil {
		return "", fmt.Errorf("marshaling FBC template: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "bpfman-catalog-render-")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	templateFile := filepath.Join(tempDir, "template.yaml")
	if err := os.WriteFile(templateFile, templateYAML, 0644); err != nil {
		return "", fmt.Errorf("writing template file: %w", err)
	}

	cmd := exec.CommandContext(ctx, ompBinPath, "alpha", "render-template", "basic",
		"--migrate-level=bundle-object-to-csv-metadata",
		"-o", "yaml",
		templateFile)
	cmd.Dir = tempDir

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("omp command failed with exit code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("running omp command: %w", err)
	}

	return string(output), nil
}

// GenerateMakefile generates a Makefile for building and deploying the catalog
func GenerateMakefile(bundleImage, binaryPath, imageUUID, randomTTL string) string {
	digestSuffix := extractDigestSuffix(bundleImage)

	localTag := "bpfman-catalog"
	if digestSuffix != "" {
		localTag = fmt.Sprintf("bpfman-catalog-sha-%s", digestSuffix)
	}

	tmpl, err := template.New("makefile").Parse(makefileTemplate)
	if err != nil {
		// Fallback to static template if parsing fails
		return fmt.Sprintf("# Error parsing Makefile template: %v\n# Bundle: %s\n", err, bundleImage)
	}

	data := struct {
		BundleImage string
		LocalTag    string
		BinaryPath  string
		ImageUUID   string
		RandomTTL   string
	}{
		BundleImage: bundleImage,
		LocalTag:    localTag,
		BinaryPath:  binaryPath,
		ImageUUID:   imageUUID,
		RandomTTL:   randomTTL,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Fallback to static template if execution fails
		return fmt.Sprintf("# Error executing Makefile template: %v\n# Bundle: %s\n", err, bundleImage)
	}

	return buf.String()
}

// GenerateWorkflow generates a WORKFLOW.txt file with deployment instructions
func GenerateWorkflow(bundleCount int, catalogRendered bool, outputDir, imageUUID, randomTTL string) string {
	tmpl, err := template.New("workflow").Parse(workflowTemplate)
	if err != nil {
		return fmt.Sprintf("# Error parsing WORKFLOW template: %v\n", err)
	}

	data := struct {
		BundleCount     int
		CatalogRendered bool
		ImageUUID       string
		RandomTTL       string
		OutputDir       string
	}{
		BundleCount:     bundleCount,
		CatalogRendered: catalogRendered,
		ImageUUID:       imageUUID,
		RandomTTL:       randomTTL,
		OutputDir:       outputDir,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("# Error executing WORKFLOW template: %v\n", err)
	}

	return buf.String()
}
