package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	GenerateManifests GenerateManifestsCmd `cmd:"generate-manifests" help:"Generate Kubernetes manifests for bpfman operator deployment"`

	// Global flags
	LogLevel  string `env:"LOG_LEVEL" default:"info" help:"Log level (debug, info, warn, error)"`
	LogFormat string `env:"LOG_FORMAT" default:"text" help:"Log format (text, json)"`
}

// ImageSource defines mutually exclusive image source options
type ImageSource struct {
	FromCatalog string `xor:"source" help:"Deploy from catalog image reference"`
	FromBundle  string `xor:"source" help:"Deploy from bundle image reference"`
}

// GenerateManifestsCmd generates manifests without applying them
type GenerateManifestsCmd struct {
	ImageSource
	OutputDir string `type:"path" default:"./manifests" help:"Output directory for generated manifests"`
	OmpBin    string `type:"path" help:"Path to opm binary for external rendering (uses library by default)"`
}

func (r *GenerateManifestsCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Info("rendering manifests",
		slog.String("output_dir", r.OutputDir),
		slog.Bool("from_catalog", r.FromCatalog != ""),
		slog.Bool("from_bundle", r.FromBundle != ""))

	if r.FromCatalog == "" && r.FromBundle == "" {
		return fmt.Errorf("either --from-catalog or --from-bundle must be specified")
	}

	config := manifests.GeneratorConfig{
		Namespace:     "bpfman", // Default namespace
		UseDigestName: true,
	}

	if r.FromCatalog != "" {
		config.ImageRef = r.FromCatalog
		generator := manifests.NewGenerator(config)

		logger.Debug("generating manifests from catalog", slog.String("catalog", r.FromCatalog))

		manifestSet, err := generator.GenerateFromCatalog(globals.Context)
		if err != nil {
			logger.Error("failed to generate manifests", slog.String("error", err.Error()))
			return fmt.Errorf("generating manifests: %w", err)
		}

		writer := writer.New(r.OutputDir)
		if err := writer.WriteAll(manifestSet); err != nil {
			logger.Error("failed to write manifests", slog.String("error", err.Error()))
			return fmt.Errorf("writing manifests: %w", err)
		}

		logger.Info("manifests generated successfully",
			slog.String("output_dir", r.OutputDir),
			slog.String("catalog", manifestSet.CatalogSource.ObjectMeta.Name))

		fmt.Printf("Manifests generated in %s\n", r.OutputDir)
		fmt.Printf("To apply: kubectl apply -f %s\n", r.OutputDir)
		return nil
	}

	if r.FromBundle != "" {
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
			logger.Error("failed to generate bundle artifacts", slog.String("error", err.Error()))
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

		logger.Info("bundle artifacts generated successfully",
			slog.String("output_dir", r.OutputDir),
			slog.Bool("catalog_rendered", artifacts.CatalogYAML != ""))

		fmt.Println(artifacts.Instructions)
		return nil
	}

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
		logger.Info("received signal", slog.String("signal", sig.String()))
		cancel()
		if err := <-errChan; err != nil {
			logger.Error("command failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case err := <-errChan:
		if err != nil {
			logger.Error("command failed", slog.String("error", err.Error()))
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
