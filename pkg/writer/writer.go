package writer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openshift/bpfman-catalog/pkg/manifests"
	"sigs.k8s.io/yaml"
)

// ManifestWriter writes manifests to files
type ManifestWriter struct {
	outputDir string
}

// New creates a new manifest writer
func New(outputDir string) *ManifestWriter {
	return &ManifestWriter{
		outputDir: outputDir,
	}
}

// WriteAll writes all manifests in a ManifestSet to files
func (w *ManifestWriter) WriteAll(manifestSet *manifests.ManifestSet) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// Write each manifest
	if manifestSet.Namespace != nil {
		if err := w.writeManifest("00-namespace.yaml", manifestSet.Namespace); err != nil {
			return fmt.Errorf("writing namespace: %w", err)
		}
	}

	if manifestSet.IDMS != nil {
		if err := w.writeManifest("01-idms.yaml", manifestSet.IDMS); err != nil {
			return fmt.Errorf("writing IDMS: %w", err)
		}
	}

	if manifestSet.CatalogSource != nil {
		if err := w.writeManifest("02-catalogsource.yaml", manifestSet.CatalogSource); err != nil {
			return fmt.Errorf("writing CatalogSource: %w", err)
		}
	}

	if manifestSet.OperatorGroup != nil {
		if err := w.writeManifest("03-operatorgroup.yaml", manifestSet.OperatorGroup); err != nil {
			return fmt.Errorf("writing OperatorGroup: %w", err)
		}
	}

	if manifestSet.Subscription != nil {
		if err := w.writeManifest("04-subscription.yaml", manifestSet.Subscription); err != nil {
			return fmt.Errorf("writing Subscription: %w", err)
		}
	}

	return nil
}

// writeManifest writes a single manifest to a file
func (w *ManifestWriter) writeManifest(filename string, manifest interface{}) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	path := filepath.Join(w.outputDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing file %s: %w", path, err)
	}

	return nil
}

// WriteSingle writes a single manifest to a specific file
func (w *ManifestWriter) WriteSingle(filename string, manifest interface{}) error {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	return w.writeManifest(filename, manifest)
}
