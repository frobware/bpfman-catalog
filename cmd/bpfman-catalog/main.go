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
	Render  RenderCmd  `cmd:"" help:"Generate Kubernetes manifests for bpfman operator deployment"`
	Install InstallCmd `cmd:"" help:"Install bpfman operator directly to cluster"`
	Status  StatusCmd  `cmd:"" help:"Check bpfman operator installation status"`

	// Global flags
	LogLevel  string `env:"LOG_LEVEL" default:"info" help:"Log level (debug, info, warn, error)"`
	LogFormat string `env:"LOG_FORMAT" default:"text" help:"Log format (text, json)"`
}

// ImageSource defines mutually exclusive image source options
type ImageSource struct {
	FromCatalog string `xor:"source" help:"Deploy from catalog image reference"`
	FromBundle  string `xor:"source" help:"Deploy from bundle image reference"`
}

// RenderCmd generates manifests without applying them
type RenderCmd struct {
	ImageSource
	OutputDir string `type:"path" default:"./manifests" help:"Output directory for generated manifests"`
}

func (r *RenderCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Info("rendering manifests",
		slog.String("output_dir", r.OutputDir),
		slog.Bool("from_catalog", r.FromCatalog != ""),
		slog.Bool("from_bundle", r.FromBundle != ""))

	// Validate inputs
	if r.FromCatalog == "" && r.FromBundle == "" {
		return fmt.Errorf("either --from-catalog or --from-bundle must be specified")
	}

	// Create generator
	config := manifests.GeneratorConfig{
		Namespace:     "bpfman", // Default namespace
		UseDigestName: true,
	}

	if r.FromCatalog != "" {
		config.ImageRef = r.FromCatalog
		generator := manifests.NewGenerator(config)

		logger.Debug("generating manifests from catalog", slog.String("catalog", r.FromCatalog))

		// Generate manifests
		manifestSet, err := generator.GenerateFromCatalog(globals.Context)
		if err != nil {
			logger.Error("failed to generate manifests", slog.String("error", err.Error()))
			return fmt.Errorf("generating manifests: %w", err)
		}

		// Write manifests
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
		return fmt.Errorf("bundle support not yet implemented")
	}

	return nil
}

// InstallCmd installs the operator directly to the cluster
type InstallCmd struct {
	ImageSource
	Namespace string `default:"bpfman" help:"Target namespace"`
}

func (i *InstallCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Info("installing operator",
		slog.String("namespace", i.Namespace),
		slog.Bool("from_catalog", i.FromCatalog != ""),
		slog.Bool("from_bundle", i.FromBundle != ""))

	// TODO: Implement install logic
	return fmt.Errorf("install not yet implemented")
}

// StatusCmd checks the installation status
type StatusCmd struct {
	Namespace string `default:"bpfman" help:"Target namespace"`
}

func (s *StatusCmd) Run(globals *GlobalContext) error {
	logger := globals.Logger
	logger.Debug("checking status", slog.String("namespace", s.Namespace))

	// TODO: Implement status logic
	return fmt.Errorf("status not yet implemented")
}

func main() {
	var cli CLI

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse command line first to get log settings
	kongCtx := kong.Parse(&cli,
		kong.Name("bpfman-catalog"),
		kong.Description("Deploy and manage bpfman operator catalogs on OpenShift"),
		kong.UsageOnError(),
	)

	// Setup logging based on CLI flags
	logger := setupLogger(cli.LogLevel, cli.LogFormat)

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create global context for dependency injection
	globals := &GlobalContext{
		Context: ctx,
		Logger:  logger,
	}

	// Run command in goroutine
	errChan := make(chan error, 1)
	go func() {
		// Call the command with global context
		errChan <- kongCtx.Run(globals)
	}()

	// Wait for completion or signal
	select {
	case sig := <-sigChan:
		logger.Info("received signal", slog.String("signal", sig.String()))
		cancel()
		// Wait for graceful shutdown
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
	// Parse log level
	logLevel := parseLogLevel(level)

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	// Choose handler based on format
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
