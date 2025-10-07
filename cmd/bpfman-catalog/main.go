package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/openshift/bpfman-catalog/pkg/analysis"
	"github.com/openshift/bpfman-catalog/pkg/bundle"
	"github.com/openshift/bpfman-catalog/pkg/manifests"
	"github.com/openshift/bpfman-catalog/pkg/writer"
)

// Default output directories
const (
	DefaultArtefactsDir = "auto-generated/artefacts"
	DefaultManifestsDir = "auto-generated/manifests"
)

// GlobalContext contains global dependencies injected into commands.
type GlobalContext struct {
	Context context.Context
	Logger  *slog.Logger
}

// CLI defines the command-line interface structure.
type CLI struct {
	PrepareCatalogBuildFromBundle     PrepareCatalogBuildFromBundleCmd     `cmd:"prepare-catalog-build-from-bundle" help:"Prepare catalog build artefacts from a bundle image"`
	PrepareCatalogBuildFromYAML       PrepareCatalogBuildFromYAMLCmd       `cmd:"prepare-catalog-build-from-yaml" help:"Prepare catalog build artefacts from an existing catalog.yaml file"`
	PrepareCatalogDeploymentFromImage PrepareCatalogDeploymentFromImageCmd `cmd:"prepare-catalog-deployment-from-image" help:"Prepare deployment manifests from existing catalog image"`
	AnalyseBundle                     AnalyseBundleCmd                     `cmd:"analyse-bundle" help:"Analyse bundle contents and dependencies"`
	ListBundles                       ListBundlesCmd                       `cmd:"list-bundles" help:"List available bundle images"`

	// Global flags
	LogLevel  string `env:"LOG_LEVEL" default:"info" help:"Log level (debug, info, warn, error)"`
	LogFormat string `env:"LOG_FORMAT" default:"text" help:"Log format (text, json)"`
}

// PrepareCatalogBuildFromBundleCmd prepares catalog build artefacts from a bundle image.
type PrepareCatalogBuildFromBundleCmd struct {
	BundleImage string `arg:"" required:"" help:"Bundle image reference"`
	OutputDir   string `default:"${default_artefacts_dir}" help:"Output directory for generated artefacts"`
	OpmBin      string `type:"path" help:"Path to opm binary for external rendering (uses library by default)"`
}

// PrepareCatalogBuildFromYAMLCmd prepares catalog build artefacts from existing catalog.yaml.
type PrepareCatalogBuildFromYAMLCmd struct {
	CatalogYAML string `arg:"" type:"path" required:"" help:"Path to existing catalog.yaml file"`
	OutputDir   string `default:"${default_artefacts_dir}" help:"Output directory for generated artefacts"`
}

// PrepareCatalogDeploymentFromImageCmd prepares deployment manifests from catalog image.
type PrepareCatalogDeploymentFromImageCmd struct {
	CatalogImage string `arg:"" required:"" help:"Catalog image reference"`
	OutputDir    string `default:"${default_manifests_dir}" help:"Output directory for generated manifests"`
}

// AnalyseBundleCmd analyses bundle contents and dependencies.
type AnalyseBundleCmd struct {
	BundleImage string `arg:"" required:"" help:"Bundle image reference to analyse"`
	Format      string `default:"text" enum:"text,json" help:"Output format (text, json)"`
	ShowAll     bool   `help:"Show all images including inaccessible ones"`
}

// ListBundlesCmd lists available bundle images.
type ListBundlesCmd struct {
	Repository string `help:"Bundle repository (default: quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream)"`
	List       int    `default:"1" help:"Number of latest bundles to list"`
	Format     string `default:"text" enum:"text,json" help:"Output format (text, json)"`
}

func (r *PrepareCatalogBuildFromBundleCmd) Run(globals *GlobalContext) error {
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultArtefactsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	logger := globals.Logger
	logger.Debug("generating catalog artefacts from bundle", slog.String("bundle", r.BundleImage))

	var gen *bundle.Generator
	if r.OpmBin != "" {
		logger.Debug("using external opm binary", slog.String("opm_bin", r.OpmBin))
		gen = bundle.NewGeneratorWithOmp(r.BundleImage, "preview", r.OpmBin)
	} else {
		gen = bundle.NewGenerator(r.BundleImage, "preview")
	}

	artefacts, err := gen.Generate(globals.Context)
	if err != nil {
		logger.Debug("failed to generate bundle artefacts", slog.String("error", err.Error()))
		return fmt.Errorf("generating bundle artefacts: %w", err)
	}

	w := writer.New(r.OutputDir)

	if err := w.WriteSingle("fbc-template.yaml", []byte(artefacts.FBCTemplate)); err != nil {
		return fmt.Errorf("writing FBC template: %w", err)
	}

	if artefacts.CatalogYAML != "" {
		if err := w.WriteSingle("catalog.yaml", []byte(artefacts.CatalogYAML)); err != nil {
			return fmt.Errorf("writing catalog: %w", err)
		}
	}

	if err := w.WriteSingle("Dockerfile", []byte(artefacts.Dockerfile)); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}
	if err := w.WriteSingle("Makefile", []byte(artefacts.Makefile)); err != nil {
		return fmt.Errorf("writing Makefile: %w", err)
	}

	catalogRendered := artefacts.CatalogYAML != ""
	imageUUID, randomTTL := bundle.GenerateImageUUIDAndTTL()
	workflow := bundle.GenerateWorkflow(0, catalogRendered, r.OutputDir, imageUUID, randomTTL)
	if err := w.WriteSingle("WORKFLOW.txt", []byte(workflow)); err != nil {
		return fmt.Errorf("writing WORKFLOW.txt: %w", err)
	}

	logger.Debug("bundle artefacts generated successfully",
		slog.String("output_dir", r.OutputDir),
		slog.Bool("catalog_rendered", catalogRendered))

	fmt.Print(workflow)
	fmt.Printf("\nThis information is saved in %s/WORKFLOW.txt\n", r.OutputDir)
	return nil
}

func (r *PrepareCatalogBuildFromYAMLCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Debug("preparing catalog build artefacts from yaml", slog.String("catalog_yaml", r.CatalogYAML))

	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultArtefactsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	catalogContent, err := os.ReadFile(r.CatalogYAML)
	if err != nil {
		return fmt.Errorf("reading catalog.yaml: %w", err)
	}

	w := writer.New(r.OutputDir)

	if err := w.WriteSingle("catalog.yaml", catalogContent); err != nil {
		return fmt.Errorf("writing catalog.yaml: %w", err)
	}

	dockerfile := bundle.GenerateCatalogDockerfile()
	if err := w.WriteSingle("Dockerfile", []byte(dockerfile)); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "bpfman-catalog"
	}

	imageUUID, randomTTL := bundle.GenerateImageUUIDAndTTL()

	makefile := bundle.GenerateMakefile("from-yaml", execPath, imageUUID, randomTTL)
	if err := w.WriteSingle("Makefile", []byte(makefile)); err != nil {
		return fmt.Errorf("writing Makefile: %w", err)
	}

	workflow := bundle.GenerateWorkflow(0, true, r.OutputDir, imageUUID, randomTTL)
	if err := w.WriteSingle("WORKFLOW.txt", []byte(workflow)); err != nil {
		return fmt.Errorf("writing WORKFLOW.txt: %w", err)
	}

	logger.Debug("catalog build artefacts generated successfully",
		slog.String("output_dir", r.OutputDir))

	fmt.Print(workflow)
	fmt.Printf("\nThis information is saved in %s/WORKFLOW.txt\n", r.OutputDir)

	return nil
}

func (r *PrepareCatalogDeploymentFromImageCmd) Run(globals *GlobalContext) error {
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like '%s'", DefaultManifestsDir)
	}

	if err := os.RemoveAll(r.OutputDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleaning output directory: %w", err)
	}

	logger := globals.Logger
	logger.Debug("generating manifests",
		slog.String("output_dir", r.OutputDir),
		slog.String("catalog_image", r.CatalogImage))

	config := manifests.GeneratorConfig{
		Namespace:     "bpfman",
		UseDigestName: true,
		ImageRef:      r.CatalogImage,
	}

	generator := manifests.NewGenerator(config)

	logger.Debug("generating manifests from catalog", slog.String("catalog", r.CatalogImage))

	manifestSet, err := generator.GenerateFromCatalog(globals.Context)
	if err != nil {
		logger.Debug("failed to generate manifests", slog.String("error", err.Error()))
		return fmt.Errorf("generating manifests: %w", err)
	}

	writer := writer.New(r.OutputDir)
	if err := writer.WriteAll(manifestSet); err != nil {
		logger.Debug("failed to write manifests", slog.String("error", err.Error()))
		return fmt.Errorf("writing manifests: %w", err)
	}

	logger.Debug("manifests generated successfully",
		slog.String("output_dir", r.OutputDir),
		slog.String("catalog", manifestSet.CatalogSource.ObjectMeta.Name))

	fmt.Printf("Manifests generated in %s\n", r.OutputDir)
	return nil
}

func (r *AnalyseBundleCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Debug("analysing bundle",
		slog.String("bundle_image", r.BundleImage),
		slog.String("format", r.Format),
		slog.Bool("show_all", r.ShowAll))

	config := analysis.AnalyseConfig{
		ShowAll: r.ShowAll,
	}

	result, err := analysis.AnalyseBundleWithConfig(globals.Context, r.BundleImage, config)
	if err != nil {
		logger.Debug("bundle analysis failed", slog.String("error", err.Error()))
		return fmt.Errorf("failed to analyse bundle: %w", err)
	}

	output, err := analysis.FormatResult(result, r.Format)
	if err != nil {
		logger.Debug("output formatting failed", slog.String("error", err.Error()))
		return fmt.Errorf("failed to format output: %w", err)
	}

	fmt.Print(output)

	logger.Debug("bundle analysis completed successfully",
		slog.Int("total_images", result.Summary.TotalImages),
		slog.Int("accessible_images", result.Summary.AccessibleImages))

	return nil
}

func (r *ListBundlesCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger

	var bundleRef bundle.BundleRef
	var err error

	if r.Repository != "" {
		bundleRef, err = bundle.ParseBundleRef(r.Repository)
		if err != nil {
			return fmt.Errorf("parsing repository: %w", err)
		}
	} else {
		bundleRef = bundle.NewDefaultBundleRef()
	}

	logger.Debug("listing bundles",
		slog.String("repository", bundleRef.String()),
		slog.Int("limit", r.List))

	bundles, err := bundle.ListLatestBundles(globals.Context, bundleRef, r.List)
	if err != nil {
		logger.Debug("failed to list bundles", slog.String("error", err.Error()))
		return fmt.Errorf("listing bundles: %w", err)
	}

	if r.Format == "json" {
		output, err := formatBundlesJSON(bundles)
		if err != nil {
			return fmt.Errorf("formatting JSON output: %w", err)
		}
		fmt.Println(output)
	} else {
		formatBundlesText(bundles)
	}

	logger.Debug("bundles listed successfully", slog.Int("count", len(bundles)))
	return nil
}

func formatBundlesText(bundles []*bundle.BundleMetadata) {
	if len(bundles) == 1 {
		b := bundles[0]
		fmt.Printf("%s@%s %s\n", b.Image[:strings.LastIndex(b.Image, ":")], b.Digest, b.BuildDate)
	} else {
		fmt.Printf("Latest %d bundles (sorted by build date, newest first):\n\n", len(bundles))
		for _, b := range bundles {
			imageBase := b.Image[:strings.LastIndex(b.Image, ":")]
			fmt.Printf("%s@%s\n", imageBase, b.Digest)
			fmt.Printf("  Tag: %s\n", b.Tag)
			fmt.Printf("  Build Date: %s\n", b.BuildDate)
			if b.Version != "" {
				fmt.Printf("  Version: %s\n", b.Version)
			}
			fmt.Println()
		}
	}
}

func formatBundlesJSON(bundles []*bundle.BundleMetadata) (string, error) {
	type output struct {
		Count   int                      `json:"count"`
		Bundles []*bundle.BundleMetadata `json:"bundles"`
	}

	out := output{
		Count:   len(bundles),
		Bundles: bundles,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printWorkflowGuide() {
	fmt.Printf(`
Workflows:

1. Build catalog from a bundle image
   Generates complete build artefacts from a bundle

   # Generates: Dockerfile, catalog.yaml, Makefile
   $ bpfman-catalog prepare-catalog-build-from-bundle \
       quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

   # builds image, pushes to registry, deploys to cluster
   $ make -C %s all

2. Build catalog from catalog.yaml
   Wraps an existing or modified catalog.yaml with build artefacts:

   # Generates: Dockerfile, Makefile
   $ bpfman-catalog prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml

   # builds image, pushes to registry, deploys to cluster
   $ make -C %s all

3. Deploy existing catalog image
   Generates Kubernetes manifests to deploy a catalog to a cluster

   # Produces: CatalogSource, Namespace, IDMS
   $ bpfman-catalog prepare-catalog-deployment-from-image \
       quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest

   # Deploy catalog to cluster
   $ kubectl apply -f %s/
`, DefaultArtefactsDir, DefaultArtefactsDir, DefaultManifestsDir)
}

func main() {
	var cli CLI

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Check if help was requested to print workflow guide
	showWorkflowGuide := false
	for _, arg := range os.Args[1:] {
		if arg == "--help" || arg == "-h" {
			showWorkflowGuide = true
			break
		}
	}

	kongCtx := kong.Parse(&cli,
		kong.Name("bpfman-catalog"),
		kong.Description("Deploy and manage bpfman operator catalogs on OpenShift"),
		kong.UsageOnError(),
		kong.Vars{
			"default_artefacts_dir": DefaultArtefactsDir,
			"default_manifests_dir": DefaultManifestsDir,
		},
		kong.Exit(func(code int) {
			// Print workflow guide before exiting on help
			if showWorkflowGuide && len(os.Args) == 2 {
				printWorkflowGuide()
			}
			os.Exit(code)
		}),
	)

	// Print workflow guide after Kong help for non-exit cases
	if showWorkflowGuide && len(os.Args) == 2 {
		printWorkflowGuide()
	}

	logger := setupLogger(cli.LogLevel, cli.LogFormat)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	globals := &GlobalContext{
		Context: ctx,
		Logger:  logger,
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- kongCtx.Run(globals)
	}()

	select {
	case sig := <-sigChan:
		logger.Debug("received signal", slog.String("signal", sig.String()))
		cancel()
		if err := <-errChan; err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			logger.Debug("command failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case err := <-errChan:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			logger.Debug("command failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}

func setupLogger(level, format string) *slog.Logger {
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug", "trace":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
