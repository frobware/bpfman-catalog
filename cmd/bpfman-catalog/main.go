package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/openshift/bpfman-catalog/pkg/bundle"
	"github.com/openshift/bpfman-catalog/pkg/manifests"
	"github.com/openshift/bpfman-catalog/pkg/writer"
)

// GlobalContext contains global dependencies injected into commands
type GlobalContext struct {
	Context context.Context
	Logger  *slog.Logger
}

// CLI defines the command-line interface structure
type CLI struct {
	GenerateArtifacts GenerateArtifactsCmd `cmd:"generate-artifacts" help:"Generate catalog build artifacts from bundle image"`
	GenerateManifests GenerateManifestsCmd `cmd:"generate-manifests" help:"Generate Kubernetes manifests from catalog image"`

	// Global flags
	LogLevel  string `env:"LOG_LEVEL" default:"info" help:"Log level (debug, info, warn, error)"`
	LogFormat string `env:"LOG_FORMAT" default:"text" help:"Log format (text, json)"`
}

// GenerateArtifactsCmd generates catalog build artifacts from bundle
type GenerateArtifactsCmd struct {
	FromBundle string `required:"" help:"Bundle image reference"`
	OutputDir  string `default:"./artifacts" help:"Output directory for generated artifacts"`
	OmpBin     string `type:"path" help:"Path to opm binary for external rendering (uses library by default)"`
}

// GenerateManifestsCmd generates Kubernetes manifests from catalog
type GenerateManifestsCmd struct {
	FromCatalog string `required:"" help:"Catalog image reference"`
	OutputDir   string `default:"./manifests" help:"Output directory for generated manifests"`
}

func (r *GenerateArtifactsCmd) Run(globals *GlobalContext) error {
	// Validate output directory - prevent overwriting current directory
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like './artifacts'")
	}

	logger := globals.Logger
	logger.Debug("generating artifacts",
		slog.String("output_dir", r.OutputDir),
		slog.String("from_bundle", r.FromBundle))

	logger.Debug("generating catalog artifacts from bundle", slog.String("bundle", r.FromBundle))

	var gen *bundle.Generator
	if r.OmpBin != "" {
		logger.Debug("using external opm binary", slog.String("omp_bin", r.OmpBin))
		gen = bundle.NewGeneratorWithOmp(r.FromBundle, "preview", r.OmpBin)
	} else {
		gen = bundle.NewGenerator(r.FromBundle, "preview")
	}

	artifacts, err := gen.Generate(globals.Context)
	if err != nil {
		logger.Debug("failed to generate bundle artifacts", slog.String("error", err.Error()))
		return fmt.Errorf("generating bundle artifacts: %w", err)
	}

	writer := writer.New(r.OutputDir)

	if err := writer.WriteSingle("fbc-template.yaml", []byte(artifacts.FBCTemplate)); err != nil {
		return fmt.Errorf("writing FBC template: %w", err)
	}

	if artifacts.CatalogYAML != "" {
		if err := writer.WriteSingle("catalog.yaml", []byte(artifacts.CatalogYAML)); err != nil {
			return fmt.Errorf("writing catalog: %w", err)
		}
	}

	if err := writer.WriteSingle("Dockerfile.catalog", []byte(artifacts.Dockerfile)); err != nil {
		return fmt.Errorf("writing Dockerfile: %w", err)
	}
	if err := writer.WriteSingle("Makefile", []byte(artifacts.Makefile)); err != nil {
		return fmt.Errorf("writing Makefile: %w", err)
	}

	logger.Debug("bundle artifacts generated successfully",
		slog.String("output_dir", r.OutputDir),
		slog.Bool("catalog_rendered", artifacts.CatalogYAML != ""))

	// Generate build instructions with correct output directory
	includeOpmStep := artifacts.CatalogYAML == ""
	instructions := bundle.GenerateBuildInstructions(r.OutputDir, includeOpmStep, r.FromBundle)
	fmt.Print(instructions)
	return nil
}

func (r *GenerateManifestsCmd) Run(globals *GlobalContext) error {
	// Validate output directory - prevent overwriting current directory
	if filepath.Clean(r.OutputDir) == "." {
		return fmt.Errorf("output directory cannot be the current working directory, please specify a named subdirectory like './manifests'")
	}

	logger := globals.Logger
	logger.Debug("generating manifests",
		slog.String("output_dir", r.OutputDir),
		slog.String("from_catalog", r.FromCatalog))

	config := manifests.GeneratorConfig{
		Namespace:     "bpfman", // Default namespace
		UseDigestName: true,
		ImageRef:      r.FromCatalog,
	}

	generator := manifests.NewGenerator(config)

	logger.Debug("generating manifests from catalog", slog.String("catalog", r.FromCatalog))

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
	fmt.Printf("To apply: kubectl apply -f %s\n", r.OutputDir)
	return nil
}

func main() {
	var cli CLI

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kongCtx := kong.Parse(&cli,
		kong.Name("bpfman-catalog"),
		kong.Description("Deploy and manage bpfman operator catalogs on OpenShift"),
		kong.UsageOnError(),
	)

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
