# Copyright 2017 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Usage:
# 	[IMG_REPO=gcr.io/google_containers/dummy-ingress-controller] [TAG=v1.0.0] make (compile|lint|unit-test|e2e-test|docs-serve|docs-deploy)

GOOS?=linux
GOARCH?=amd64
GOBIN:=$(shell pwd)/.bin

IMG_REPO?=amazon/aws-alb-ingress-controller
IMG_TAG?=v1.1.9
GIT_REPO=$(shell git config --get remote.origin.url)
GIT_COMMIT=$(shell git rev-parse --short HEAD)


VERSION_PKG=github.com/kubernetes-sigs/aws-alb-ingress-controller/version
VERSION_LD_FLAGS=-X $(VERSION_PKG).RELEASE=$(IMG_TAG) -X $(VERSION_PKG).REPO=$(GIT_REPO) -X $(VERSION_PKG).COMMIT=$(GIT_COMMIT)
COMPILE_OUTPUT?=controller

all: compile

.PHONY: compile
compile:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-s -w $(VERSION_LD_FLAGS)" -a -installsuffix cgo  -o ${COMPILE_OUTPUT} ./cmd

.PHONY: lint
lint:
	GOBIN=$(GOBIN) go install -v github.com/golangci/golangci-lint/cmd/golangci-lint
	$(GOBIN)/golangci-lint run --deadline=10m

.PHONY: unit-test
unit-test:
	GOBIN=$(GOBIN) ./scripts/ci_unit_test.sh

.PHONY: e2e-test
e2e-test:
	GOBIN=$(GOBIN) go get github.com/aws/aws-k8s-tester/e2e/tester/cmd/k8s-e2e-tester@master
	GOBIN=$(GOBIN) TESTCONFIG=./tester/test-config.yaml ${GOBIN}/k8s-e2e-tester

test: lint unit-test

# build & preview docs
docs-serve:
	pipenv install && pipenv run mkdocs serve
# deploy docs to github-pages(gh-pages branch)
docs-deploy:
	pipenv install && pipenv run mkdocs gh-deploy

release:
	docker buildx build . --target bin \
		--tag $(IMG_REPO):$(IMG_TAG) \
		--push \
		--platform linux/amd64,linux/arm64