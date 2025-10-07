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

## Development vs Release Workflows

### For Daily Development
Use the `bpfman-catalog` CLI tool to create catalogs from individual bundles:
```bash
# Build the CLI tool
make build-cli

# Generate catalog from a bundle image
./bin/bpfman-catalog prepare-catalog-build-from-bundle \
  quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest

# Build and deploy the generated catalog
make -C artifacts all
```

See `./bin/bpfman-catalog --help` for more workflows including editing existing catalogs and deploying catalog images.

### For Y-Stream Releases
Use the `y-stream` template for minor version releases:
```bash
make generate-catalogs
make build-image
make push-image
make deploy
```

### For Z-Stream Releases
Use the `z-stream` template for patch releases:
```bash
make generate-catalogs
make build-image BUILD_STREAM=z-stream
make push-image
make deploy
```
