.DEFAULT_GOAL := all

# Username for image references - priority: BPFMAN_CATALOG_QUAY_USER > USER > fallback.
QUAY_USER ?= $(or $(BPFMAN_CATALOG_QUAY_USER),$(USER),$(shell echo $$USER))
IMAGE ?= quay.io/$(QUAY_USER)/bpfman-operator-catalog:latest
BUILD_STREAM ?= y-stream
BASE_IMAGE ?= registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.20
BUILDVERSION ?= 4.20.0
COMMIT ?= $(shell git rev-parse HEAD)

# Image building tool - defaults to podman, override with OCI_BIN.
OCI_BIN_PATH := $(shell which podman)
OCI_BIN ?= $(shell if [ -n "${OCI_BIN_PATH}" ]; then basename ${OCI_BIN_PATH}; else echo "podman"; fi)
export OCI_BIN

LOCALBIN ?= $(shell pwd)/bin

# Go build configuration - tags required for operator-registry library integration.
export GOFLAGS := -tags=json1,containers_image_openpgp
export CGO_ENABLED := 0

# OPM configuration - v1.52 for local install (v1.54 has go.mod replace issues).
OPM_VERSION := v1.52.0
OPM_IMAGE := quay.io/operator-framework/opm:$(OPM_VERSION)
YQ_VERSION  := v4.35.2

OPM ?= $(LOCALBIN)/opm
YQ  ?= $(LOCALBIN)/yq

# Define go-install macro for installing Go tools.
# $1 = tool binary path
# $2 = tool package
# $3 = tool version
define go-install
	@[ -f $(1) ] || { \
		echo "Downloading $(notdir $(1)) $(3)..." ; \
		GOBIN=$(LOCALBIN) go install $(2)@$(3) ; \
	}
endef

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM):
	$(call go-install,$(OPM),github.com/operator-framework/operator-registry/cmd/opm,$(OPM_VERSION))

.PHONY: yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ):
	$(call go-install,$(YQ),github.com/mikefarah/yq/v4,$(YQ_VERSION))

.PHONY: prereqs
prereqs: opm yq

# Template files.
TEMPLATES := $(wildcard templates/*.yaml)
CATALOGS := $(patsubst templates/%.yaml,auto-generated/catalog/%.yaml,$(TEMPLATES))

auto-generated/catalog:
	mkdir -p $@

##@ Build

# Pattern rule: generate catalog from template.
auto-generated/catalog/%.yaml: templates/%.yaml | auto-generated/catalog prereqs
	$(OPM) alpha render-template basic --migrate-level=bundle-object-to-csv-metadata -o yaml $< > $@

.PHONY: generate-catalogs
generate-catalogs: $(CATALOGS) ## Generate catalogs from templates using local OPM.

.PHONY: generate-catalogs-container
generate-catalogs-container: | auto-generated/catalog ## Generate catalogs using OPM container (requires Podman auth).
	@if [ -z "$(OCI_BIN_PATH)" ]; then \
		echo "ERROR: Podman is required but not found in PATH" ; \
		echo "Please install podman first" ; \
		exit 1 ; \
	fi
	@if [ ! -f "$${XDG_RUNTIME_DIR}/containers/auth.json" ]; then \
		echo "ERROR: No Podman registry authentication found." ; \
		echo "Please login to registry first:" ; \
		echo "  podman login registry.redhat.io" ; \
		exit 1 ; \
	fi
	@echo "Generating catalogs using container approach..."
	@for template in $(TEMPLATES); do \
		template_name=$$(basename $$template) ; \
		catalog="auto-generated/catalog/$$template_name" ; \
		echo "Processing $$template_name..." ; \
		$(OCI_BIN) build --quiet \
			--secret id=dockerconfig,src=$${XDG_RUNTIME_DIR}/containers/auth.json \
			--build-arg TEMPLATE_FILE=$$template_name \
			-f Dockerfile.generate -t temp-catalog:$$template_name . && \
		$(OCI_BIN) create --name temp-$$template_name temp-catalog:$$template_name >/dev/null && \
		$(OCI_BIN) cp temp-$$template_name:/catalog.yaml $$catalog && \
		$(OCI_BIN) rm temp-$$template_name >/dev/null && \
		$(OCI_BIN) rmi temp-catalog:$$template_name >/dev/null || exit 1 ; \
	done
	@echo "Catalogs generated successfully"

.PHONY: build-image
build-image: ## Build catalog container image.
	$(OCI_BIN) build --build-arg INDEX_FILE="./auto-generated/catalog/$(BUILD_STREAM).yaml" --build-arg BASE_IMAGE="$(BASE_IMAGE)" --build-arg COMMIT="$(COMMIT)" --build-arg BUILDVERSION="$(BUILDVERSION)" -t $(IMAGE) -f Dockerfile .

.PHONY: push-image
push-image: ## Push catalog container image.
	$(OCI_BIN) push ${IMAGE}

##@ Deployment

.PHONY: deploy
deploy: yq ## Deploy catalog to OpenShift cluster.
	$(YQ) '.spec.image="$(IMAGE)"' ./catalog-source.yaml | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Remove catalog from OpenShift cluster.
	kubectl delete -f ./catalog-source.yaml

##@ Cleanup

.PHONY: clean-catalogs
clean-catalogs: ## Remove generated catalogs.
	rm -rf auto-generated/catalog/*.yaml

.PHONY: clean-bin
clean-bin: ## Remove bin directory (tools and built binaries).
	rm -rf $(LOCALBIN)

.PHONY: clean
clean: clean-catalogs clean-bin ## Remove all generated files and binaries.

##@ CLI Tool

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: ## Run unit tests.
	go test ./...

.PHONY: build-cli
build-cli: fmt vet test opm ## Build the bpfman-catalog CLI tool.
	go build -o $(LOCALBIN)/bpfman-catalog ./cmd/bpfman-catalog

# Define test-cli-run macro for running CLI tests
# $1 = workflow name
# $2 = output directory
# $3 = CLI command and arguments
define test-cli-run
	@echo "Testing $(1)..."
	@rm -rf $(2)
	PATH="$(LOCALBIN):$$PATH" $(LOCALBIN)/bpfman-catalog $(3) --output-dir $(2)
	@echo "Artefacts generated in $(2)"
	@ls -la $(2)/
endef

.PHONY: test-cli
test-cli: test-cli-bundle test-cli-yaml test-cli-image ## Test the CLI with all three workflow examples.

.PHONY: test-cli-bundle
test-cli-bundle: test-cli-bundle-opm-library test-cli-bundle-opm-binary ## Test workflow 1: Build catalog from bundle (both OPM methods).

.PHONY: test-cli-bundle-opm-library
test-cli-bundle-opm-library: build-cli ## Test workflow 1a: Build catalog from bundle (OPM library mode).
	$(call test-cli-run,workflow 1a: Build catalog from bundle (OPM library mode),/tmp/bpfman-catalog-test-bundle-opm-library,prepare-catalog-build-from-bundle quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-test-bundle-opm-library build-catalog-image IMAGE=bpfman-catalog-cli-test:opm-library
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:opm-library

.PHONY: test-cli-bundle-opm-binary
test-cli-bundle-opm-binary: build-cli ## Test workflow 1b: Build catalog from bundle (OPM binary mode).
	$(call test-cli-run,workflow 1b: Build catalog from bundle (OPM binary mode),/tmp/bpfman-catalog-test-bundle-opm-binary,prepare-catalog-build-from-bundle quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest --opm-bin $(LOCALBIN)/opm)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-test-bundle-opm-binary build-catalog-image IMAGE=bpfman-catalog-cli-test:opm-binary
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:opm-binary

.PHONY: test-cli-yaml
test-cli-yaml: build-cli generate-catalogs ## Test workflow 2: Build catalog from catalog.yaml.
	$(call test-cli-run,workflow 2: Build catalog from catalog.yaml,/tmp/bpfman-catalog-test-yaml,prepare-catalog-build-from-yaml auto-generated/catalog/y-stream.yaml)
	echo "Building catalog image to verify artefacts..."
	make -C /tmp/bpfman-catalog-test-yaml build-catalog-image IMAGE=bpfman-catalog-cli-test:yaml
	echo "Image built successfully, cleaning up..."
	$(OCI_BIN) rmi bpfman-catalog-cli-test:yaml

.PHONY: test-cli-image
test-cli-image: build-cli ## Test workflow 3: Deploy existing catalog image.
	$(call test-cli-run,workflow 3: Deploy existing catalog image,/tmp/bpfman-catalog-test-manifests,prepare-catalog-deployment-from-image quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest)
	@ls -la /tmp/bpfman-catalog-test-manifests/catalog/

##@ General

.PHONY: all
all: help ## Default target: display help.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  %-28s %s\n", $$1, $$2 } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)
