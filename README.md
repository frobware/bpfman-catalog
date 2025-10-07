# bpfman-catalog

OLM catalog used to release the Openshift eBPF Manager Operator.

## Overview

This repository builds the BPFMan OLM catalog used to release new versions of the OpenShift eBPF Manager Operator. It generates operator catalogs from templates and packages them as OCI images for deployment to OpenShift clusters.

## Directory Structure

- `templates/` - Source YAML files defining operator versions and channels
  - `y-stream.yaml` - Y-stream minor version releases (default)
  - `z-stream.yaml` - Z-stream patch releases
- `auto-generated/` - Generated catalogs (created by `make generate-catalogs`)
  - `catalog/` - Modern catalog format with bundle-object-to-csv-metadata migration
- `cmd/bpfman-catalog/` - CLI tool for catalog operations
- `Dockerfile` - Container definition for building catalog images
- `catalog-source.yaml` - CatalogSource resource for deploying to OpenShift

## Common Commands

Run `make` to list all available targets.

### Generate Catalogs

Generate catalogs from templates:
```bash
make generate-catalogs
```

### Build Container Image

Build catalog image (defaults to y-stream):
```bash
make build-image
```

Build with z-stream patch releases:
```bash
make build-image BUILD_STREAM=z-stream
```

Build with custom image tag:
```bash
make build-image IMAGE=quay.io/myuser/bpfman-catalog:latest
```

### Deploy to Cluster

Deploy catalog to OpenShift cluster:
```bash
make build-image push-image deploy
```

Individual steps:
```bash
make push-image    # Push built image
make deploy        # Deploy catalog source to cluster
make undeploy      # Remove catalog source from cluster
```

## Development Workflow

1. Modify template files in `templates/` directory
2. Run `make generate-catalogs` to update auto-generated catalogs
3. Build and test with `make build-image`
4. Push image with `make push-image`
5. Deploy to test cluster with `make deploy`

## Configuration Variables

- `IMAGE` - Target image name (default: quay.io/$USER/bpfman-operator-catalog:latest)
- `BUILD_STREAM` - Which template to use (default: y-stream, options: y-stream, z-stream)
- `OCI_BIN` - Container runtime (docker or podman, auto-detected)

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
