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
# Deploy from a pre-built catalog image
bpfman-catalog render --from-catalog quay.io/namespace/catalog@sha256:abc123... --output-dir ./deploy
kubectl apply -f ./deploy
```

### Generate Catalog from Bundle
```bash
# Generate catalog artifacts from a bundle image
bpfman-catalog render --from-bundle quay.io/namespace/bundle@sha256:def456... --output-dir ./build

# Build and push the catalog image
cd ./build
podman build -f Dockerfile.catalog -t my-catalog:dev .
podman push my-catalog:dev ttl.sh/my-catalog:dev

# Generate deployment manifests
bpfman-catalog render --from-catalog ttl.sh/my-catalog:dev --output-dir ./deploy
kubectl apply -f ./deploy
```

## Operations Guide

### Resource Management

All resources created by bpfman-catalog are labeled for easy identification and cleanup:

```bash
# Find all resources from a specific deployment (using digest from image)
kubectl get all -l bpfman-catalog-cli=17645607

# Clean up everything from a deployment
kubectl delete all -l bpfman-catalog-cli=17645607

# Find all resources created by the tool across all deployments
kubectl get all -l app.kubernetes.io/created-by=bpfman-catalog-cli

# List all bpfman operator resources
kubectl get all -l app.kubernetes.io/name=bpfman-operator

# Check deployment status
kubectl get catalogsource -l bpfman-catalog-cli=17645607
kubectl get subscription -l bpfman-catalog-cli=17645607
```

### Resource Names

All resources follow a consistent naming pattern with digest suffixes:
- Namespace: `bpfman-17645607`
- CatalogSource: `bpfman-catalogsource-17645607`
- OperatorGroup: `bpfman-operatorgroup-17645607`
- Subscription: `bpfman-subscription-17645607`
- ImageDigestMirrorSet: `bpfman-idms-17645607`

The digest suffix (first 8 characters of the image SHA256) ensures unique resource names when deploying multiple versions.

### Troubleshooting

```bash
# Check catalog source status
kubectl get catalogsource bpfman-catalogsource-17645607 -o yaml

# Check subscription status and installed CSV
kubectl get subscription bpfman-subscription-17645607 -o yaml
kubectl get csv -l app.kubernetes.io/name=bpfman-operator

# Check operator pod logs
kubectl logs -l app.kubernetes.io/name=bpfman-operator -n bpfman-17645607

# Check catalog pod logs
kubectl logs -l olm.catalogSource=bpfman-catalogsource-17645607 -n openshift-marketplace
```

## Building

```bash
# Using Makefile (recommended)
make build-cli

# Manual build
CGO_ENABLED=0 go build -tags "json1,containers_image_openpgp" ./cmd/bpfman-catalog
```