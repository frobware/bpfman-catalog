# bpfman-catalog

OLM catalog tooling for deploying the OpenShift eBPF Manager Operator.

## Overview

This repository provides two complementary approaches for managing BPFMan operator catalogs:

1. **Template-Based Releases** - Curated catalog templates for formal releases with controlled upgrade paths, built using traditional Makefile workflows
2. **CLI Tool** - `bpfman-catalog` command-line tool for rapid development iteration, enabling developers to quickly build and deploy catalogs from bundle images

Both approaches generate OLM File-Based Catalogs (FBC) and package them as OCI images for deployment to OpenShift clusters.

## Quick Start

### For Development (CLI Tool)

```bash
# Build the CLI tool
make build-cli

# Generate catalog from a bundle and deploy
./bin/bpfman-catalog prepare-catalog-build-from-bundle \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest
make -C auto-generated/artefacts all
```

### For Releases (Templates)

```bash
# Generate, build, and deploy from templates
make generate-catalogs build-image push-image deploy
```

## Directory Structure

- `templates/` - Curated catalog templates for releases
  - `y-stream.yaml` - Y-stream minor version releases
  - `z-stream.yaml` - Z-stream patch releases
- `auto-generated/` - Generated output directories
  - `catalog/` - Catalogs from templates
  - `artefacts/` - CLI-generated build files
  - `manifests/` - CLI-generated Kubernetes manifests
- `cmd/bpfman-catalog/` - CLI tool source code
- `pkg/` - Go packages for catalog operations
- `Dockerfile` - Container definition for building catalog images
- `catalog-source.yaml` - CatalogSource resource template

## Configuration

Environment variables and Make variables:

- `IMAGE` - Target image name (default: `quay.io/$USER/bpfman-operator-catalog:latest`)
- `BUILD_STREAM` - Template to use (default: `y-stream`, options: `y-stream`, `z-stream`)
- `OCI_BIN` - Container runtime (`docker` or `podman`, auto-detected)
- `LOG_LEVEL` - CLI logging level (default: `info`, options: `debug`, `info`, `warn`, `error`)
- `LOG_FORMAT` - CLI log format (default: `text`, options: `text`, `json`)

## CLI Tool Workflows

The `bpfman-catalog` CLI tool streamlines catalog creation and deployment by automating the generation of build artefacts and Kubernetes manifests. It supports three primary workflows for different stages of the development and release process.

### Building the CLI

```bash
make build-cli
```

The tool will be available at `./bin/bpfman-catalog`. Run `./bin/bpfman-catalog --help` for detailed usage information.

### Workflow 1: Testing Development Bundles

**User Story**: As an OpenShift developer, I want to quickly test a newly built operator bundle by deploying it to my cluster without manually creating catalog configurations.

```bash
# Generate complete catalog build artefacts from a bundle image
./bin/bpfman-catalog prepare-catalog-build-from-bundle \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

# Build catalog image, push to registry, and deploy to cluster
make -C auto-generated/artefacts all
```

This workflow generates a complete catalog from a single bundle image, including the FBC template, rendered catalog, Dockerfile, and deployment Makefile.

### Workflow 2: Customising Existing Catalogs

**User Story**: As an OpenShift developer, I want to modify an existing catalog configuration and rebuild it for testing changes to channel structure or bundle versions.

```bash
# Edit the catalog YAML to add/remove bundles or modify channels
vim auto-generated/catalog/y-stream.yaml

# Generate build artefacts from the modified catalog
./bin/bpfman-catalog prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml

# Build catalog image, push to registry, and deploy to cluster
make -C auto-generated/artefacts all
```

This workflow wraps an existing or hand-edited catalog.yaml with the necessary build infrastructure.

### Workflow 3: Deploying Pre-built Catalogs

**User Story**: As an OpenShift developer, I want to deploy a catalog image that's already been built and published to a registry without rebuilding it locally.

```bash
# Generate Kubernetes manifests for an existing catalog image
./bin/bpfman-catalog prepare-catalog-deployment-from-image \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest

# Deploy the catalog to the cluster
kubectl apply -f auto-generated/manifests/
```

This workflow generates only the deployment manifests (CatalogSource, Namespace, ImageDigestMirrorSet) for a pre-existing catalog image.

## Template-Based Release Workflow

For formal releases, use the template-based workflow which builds catalog images from curated templates that define specific operator versions and upgrade paths.

**User Story**: As an OpenShift release engineer, I want to publish a catalog containing specific operator versions with controlled upgrade paths, then make it available in the cluster for users to install via the console.

```bash
# Generate catalogs from templates
make generate-catalogs

# Build catalog image (defaults to y-stream)
make build-image

# Or build z-stream for patch releases
make build-image BUILD_STREAM=z-stream

# Push to registry
make push-image

# Deploy CatalogSource to cluster
make deploy
```

After deployment, the operator becomes available in the OpenShift console under **Operators → OperatorHub** where users can install it through the UI.

This workflow creates only the CatalogSource resource, allowing administrators to control when and how the operator is installed rather than automatically subscribing to it.
