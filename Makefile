.DEFAULT_GOAL := all

IMAGE ?= quay.io/$(USER)/bpfman-operator-catalog:latest
BUILD_STREAM ?= y-stream
BASE_IMAGE ?= registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.20
BUILDVERSION ?= 4.20.0
COMMIT ?= $(shell git rev-parse HEAD)

# Image building tool - podman only for catalog generation
OCI_BIN_PATH := $(shell which podman)
OCI_BIN ?= $(shell if [ -n "${OCI_BIN_PATH}" ]; then basename ${OCI_BIN_PATH}; else echo "podman"; fi)
export OCI_BIN

LOCALBIN ?= $(shell pwd)/bin

# Go build configuration - tags required for operator-registry library integration
export GOFLAGS := -tags=json1,containers_image_openpgp
export CGO_ENABLED := 0

# OPM configuration - v1.52 for local install (v1.54 has go.mod replace issues)
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

# Template files
TEMPLATES := $(wildcard templates/*.yaml)
CATALOGS := $(patsubst templates/%.yaml,auto-generated/catalog/%.yaml,$(TEMPLATES))

auto-generated/catalog:
	mkdir -p $@

##@ Build

# Pattern rule: generate catalog from template
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
fmt: ## Run go fmt on the code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet on the code
	go vet ./...

.PHONY: test
test: ## Run unit tests
	go test -v ./...

.PHONY: build-cli
build-cli: fmt vet test opm ## Build the bpfman-catalog CLI tool
	go build -o $(LOCALBIN)/bpfman-catalog ./cmd/bpfman-catalog

.PHONY: install-cli
install-cli: build-cli ## Install the bpfman-catalog CLI to /usr/local/bin
	@echo "Installing bpfman-catalog to /usr/local/bin (may require sudo)"
	@cp $(LOCALBIN)/bpfman-catalog /usr/local/bin/bpfman-catalog 2>/dev/null || \
		sudo cp $(LOCALBIN)/bpfman-catalog /usr/local/bin/bpfman-catalog

.PHONY: test-cli
test-cli: build-cli ## Test the CLI with a sample catalog
	@echo "Testing with a sample catalog image..."
	@echo "Note: Use 'make test-cli-bundle' to test bundle support"
	@rm -rf /tmp/bpfman-catalog-test-manifests
	PATH="$(LOCALBIN):$$PATH" $(LOCALBIN)/bpfman-catalog prepare-catalog-deployment-from-image \
		quay.io/redhat-user-workloads/ocp-bpfman-tenant/catalog-ystream:latest \
		--output-dir /tmp/bpfman-catalog-test-manifests
	@echo "Test manifests generated in /tmp/bpfman-catalog-test-manifests"
	@ls -la /tmp/bpfman-catalog-test-manifests/catalog/

.PHONY: test-cli-bundle
test-cli-bundle: test-cli-bundle-opm-library test-cli-bundle-opm-binary ## Test the CLI with both OPM rendering methods

.PHONY: test-cli-bundle-opm-library
test-cli-bundle-opm-library: build-cli ## Test the CLI with a sample bundle (OPM library mode)
	@echo "Testing with a sample bundle image (OPM library mode)..."
	@rm -rf /tmp/bpfman-catalog-test-bundle-opm-library
	PATH="$(LOCALBIN):$$PATH" $(LOCALBIN)/bpfman-catalog prepare-catalog-build-from-bundle \
		quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest \
		--output-dir /tmp/bpfman-catalog-test-bundle-opm-library
	@echo "Bundle artifacts generated in /tmp/bpfman-catalog-test-bundle-opm-library"
	@ls -la /tmp/bpfman-catalog-test-bundle-opm-library/

.PHONY: test-cli-bundle-opm-binary
test-cli-bundle-opm-binary: build-cli ## Test the CLI with a sample bundle (OPM binary mode)
	@echo "Testing with a sample bundle image (OPM binary mode)..."
	@rm -rf /tmp/bpfman-catalog-test-bundle-opm-binary
	PATH="$(LOCALBIN):$$PATH" $(LOCALBIN)/bpfman-catalog prepare-catalog-build-from-bundle \
		quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest \
		--opm-bin $(LOCALBIN)/opm \
		--output-dir /tmp/bpfman-catalog-test-bundle-opm-binary
	@echo "Bundle artifacts generated in /tmp/bpfman-catalog-test-bundle-opm-binary"
	@ls -la /tmp/bpfman-catalog-test-bundle-opm-binary/

##@ General

.PHONY: all
all: help ## Default target: display help.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  %-28s %s\n", $$1, $$2 } \
		/^##@/ { printf "\n%s\n", substr($$0, 5) }' \
		$(MAKEFILE_LIST)
