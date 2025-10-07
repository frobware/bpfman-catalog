# bpfman-catalog

A CLI tool for consuming and installing bpfman operator catalogs that have not been released yet.

## Purpose

This tool deploys bpfman operator versions from pre-release catalogs or custom bundles, particularly useful for:

- Testing unreleased bpfman operator versions from development branches
- Konflux-based workflows that produce catalog images with digest references
- Development environments where you need specific operator versions not yet available in official catalogs

## Usage

### Deploy from Catalog Image
```bash
# Generate deployment manifests from a pre-built catalog image
bpfman-catalog generate-manifests --from-catalog quay.io/namespace/catalog@sha256:abc123... --output-dir ./deploy
kubectl apply -f ./deploy/catalog/
```

### Generate Catalog from Bundle
```bash
# Generate catalog build artefacts from a bundle image
bpfman-catalog generate-artefacts --from-bundle quay.io/namespace/bundle@sha256:def456... --output-dir ./build

# Build and push the catalog image
cd ./build
podman build -f Dockerfile.catalog -t bpfman-catalog-sha-abc12345 .
podman tag bpfman-catalog-sha-abc12345 ttl.sh/unique-uuid:1h
podman push ttl.sh/unique-uuid:1h

# Generate deployment manifests from the catalog
bpfman-catalog generate-manifests --from-catalog ttl.sh/unique-uuid:1h --output-dir ./deploy
kubectl apply -f ./deploy/catalog/
```

## Operations Guide

### Resource Management

All resources created by bpfman-catalog are labeled for easy identification and cleanup:

```bash
# Find all resources from a specific deployment (using digest from image)
kubectl get all -l bpfman-catalog-cli=12345678

# Clean up everything from a deployment
kubectl delete all -l bpfman-catalog-cli=12345678

# Find all resources created by the tool across all deployments
kubectl get all -l app.kubernetes.io/created-by=bpfman-catalog-cli

# List all bpfman operator resources
kubectl get all -l app.kubernetes.io/name=bpfman-operator

# Check deployment status
kubectl get catalogsource -l bpfman-catalog-cli=12345678
kubectl get subscription -l bpfman-catalog-cli=12345678
```

### Resource Names

All resources follow a consistent naming pattern with SHA digest suffixes:
- Namespace: `bpfman-sha-12345678`
- CatalogSource: `bpfman-catalogsource-sha-12345678`
- OperatorGroup: `bpfman-operatorgroup-sha-12345678`
- Subscription: `bpfman-subscription-sha-12345678`
- ImageDigestMirrorSet: `bpfman-idms-sha-12345678`

The `-sha-` prefix clearly indicates the suffix is derived from the image SHA256 digest (first 8 characters), ensuring unique resource names when deploying multiple versions.

### Troubleshooting

```bash
# Check catalog source status
kubectl get catalogsource bpfman-catalogsource-sha-12345678 -o yaml

# Check subscription status and installed CSV
kubectl get subscription bpfman-subscription-sha-12345678 -o yaml
kubectl get csv -l app.kubernetes.io/name=bpfman-operator

# Check operator pod logs
kubectl logs -l app.kubernetes.io/name=bpfman-operator -n bpfman-sha-12345678

# Check catalog pod logs
kubectl logs -l olm.catalogSource=bpfman-catalogsource-sha-12345678 -n openshift-marketplace
```

## Building

```bash
# Using Makefile (recommended)
make build-cli

# Manual build
CGO_ENABLED=0 go build -tags "json1,containers_image_openpgp" ./cmd/bpfman-catalog
```