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
# 	[PREFIX=gcr.io/google_containers/dummy-ingress-controller] [ARCH=amd64] [TAG=1.1] make (server|container|push)

all: container

TAG?=v1.1.0
PREFIX?=amazon/aws-alb-ingress-controller
ARCH?=amd64
OS?=linux
PKG=github.com/kubernetes-sigs/aws-alb-ingress-controller
REPO_INFO=$(shell git config --get remote.origin.url)
GO111MODULE=on
GOBIN:=$(shell pwd)/.bin

.EXPORT_ALL_VARIABLES:

ifndef GIT_COMMIT
  GIT_COMMIT := git-$(shell git rev-parse --short HEAD)
endif

LDFLAGS=-X $(PKG)/version.COMMIT=$(GIT_COMMIT) -X $(PKG)/version.RELEASE=$(TAG) -X $(PKG)/version.REPO=$(REPO_INFO)

server: cmd/main.go
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -a -installsuffix cgo -ldflags '-s -w $(LDFLAGS)' -o server ./cmd

container: server
	docker build --pull -t $(PREFIX):$(TAG) .

push:
	docker push $(PREFIX):$(TAG)

clean:
	rm -f server

lint:
	go install -v github.com/golangci/golangci-lint/cmd/golangci-lint
	$(GOBIN)/golangci-lint run --deadline=10m

unit-test:
	@./scripts/ci_unit_test.sh
test:unit-test

# build & preview docs
docs-serve:
	pipenv run mkdocs serve
# deploy docs to github-pages(gh-pages branch)
docs-deploy:
	pipenv run mkdocs gh-deploy