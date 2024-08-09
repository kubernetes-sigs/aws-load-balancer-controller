
MAKEFILE_PATH = $(dir $(realpath -s $(firstword $(MAKEFILE_LIST))))

# Image URL to use all building/pushing image targets
IMG ?= public.ecr.aws/eks/aws-load-balancer-controller:v2.8.2
# Image URL to use for builder stage in Docker build
GOLANG_VERSION ?= $(shell cat .go-version)
BUILD_IMAGE ?= public.ecr.aws/docker/library/golang:$(GOLANG_VERSION)
# Image URL to use for base layer in Docker build
BASE_IMAGE ?= public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-nonroot:2024-04-01-1711929684.2
IMG_PLATFORM ?= linux/amd64,linux/arm64
# ECR doesn't appear to support SPDX SBOM
IMG_SBOM ?= none


CRD_OPTIONS ?= "crd:crdVersions=v1"

# Whether to override AWS SDK models. set to 'y' when we need to build against custom AWS SDK models.
AWS_SDK_MODEL_OVERRIDE ?= "n"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

export GOSUMDB = sum.golang.org
export GOTOOLCHAIN = go$(GOLANG_VERSION)

all: controller

# Run tests
test: generate fmt vet manifests helm-lint
	go test -race ./pkg/... ./webhooks/... -coverprofile cover.out

# Build controller binary
controller: generate fmt vet
	go build -o bin/controller main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/controller && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen kustomize
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=controller-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	yq eval '.metadata.name = "webhook"' -i config/webhook/manifests.yaml

crds: manifests
	$(KUSTOMIZE) build config/crd > helm/aws-load-balancer-controller/crds/crds.yaml


# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

helm-lint:
	${MAKEFILE_PATH}/test/helm/helm-lint.sh

# Generate code
.PHONY: generate
generate: aws-sdk-model-override controller-gen mockgen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	MOCKGEN=$(MOCKGEN) ./scripts/gen_mocks.sh

aws-sdk-model-override:
	@if [ "$(AWS_SDK_MODEL_OVERRIDE)" = "y" ] ; then \
		./scripts/aws_sdk_model_override/setup.sh ; \
	else \
		./scripts/aws_sdk_model_override/cleanup.sh ; \
	fi

.PHONY: docker-push
docker-push: aws-load-balancer-controller-push

.PHONY: aws-load-balancer-controller-push
aws-load-balancer-controller-push: ko
	KO_DOCKER_REPO=$(firstword $(subst :, ,${IMG})) \
    GIT_VERSION=$(shell git describe --tags --dirty --always) \
    GIT_COMMIT=$(shell git rev-parse HEAD)  \
    BUILD_DATE=$(shell date +%Y-%m-%dT%H:%M:%S%z) \
    ko build --tags $(word 2,$(subst :, ,${IMG})) --platform=${IMG_PLATFORM} --bare --sbom ${IMG_SBOM} .

# Push the docker image using docker buildx
docker-push-w-buildx:
	docker buildx build . --target bin \
        		--tag $(IMG) \
				--build-arg BASE_IMAGE=$(BASE_IMAGE) \
				--build-arg BUILD_IMAGE=$(BUILD_IMAGE) \
				--push \
        		--platform ${IMG_PLATFORM}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# find or download mockgen
# download mockgen if necessary
.PHONY: mockgen
mockgen:
ifeq (, $(shell which mockgen))
	@{ \
	set -e ;\
	MOCKGEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$MOCKGEN_TMP_DIR ;\
	go mod init tmp ;\
	go install github.com/golang/mock/mockgen@v1.6.0 ;\
	rm -rf $$MOCKGEN_TMP_DIR ;\
	}
MOCKGEN=$(GOBIN)/mockgen
else
MOCKGEN=$(shell which mockgen)
endif

# install kustomize if not found
kustomize:
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_TMP_DIR ;\
	go mod init tmp ;\
	GO111MODULE=on go get sigs.k8s.io/kustomize/kustomize/v3 ;\
	rm -rf $$KUSTOMIZE_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: ko
ko:
	hack/install-ko.sh

# preview docs
docs-preview: docs-dependencies
	pipenv run mkdocs serve

# publish the versioned docs using mkdocs mike util
docs-publish: docs-dependencies
	pipenv run mike deploy v2.8 latest -p --update-aliases

# install dependencies needed to preview and publish docs
docs-dependencies:
	pipenv install --dev

lint:
	echo "TODO"

.PHONY: quick-ci
quick-ci: verify-versions verify-generate verify-crds
	echo "Done!"

.PHONY: verify-generate
verify-generate:
	hack/verify-generate.sh

.PHONY: verify-crds
verify-crds:
	hack/verify-crds.sh

.PHONY: verify-versions
verify-versions:
	hack/verify-versions.sh

unit-test:
	./scripts/ci_unit_test.sh

e2e-test:
	./scripts/ci_e2e_test.sh
