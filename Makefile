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

# Build the default backend binary or image for amd64, arm, arm64 and ppc64le
#
# Usage:
# 	[PREFIX=gcr.io/google_containers/dummy-ingress-controller] [ARCH=amd64] [TAG=1.1] make (server|container|push)

all: container

TAG?=1.0-beta.4
BUILD=$(shell git log --pretty=format:'%h' -n 1)
PREFIX?=quay.io/coreos/alb-ingress-controller
ARCH?=amd64
TEMP_DIR:=$(shell mktemp -d)
LDFLAGS=-X github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/controller.Build=git-$(BUILD) -X github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/controller.Release=$(TAG)

server: cmd/main.go 
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) GOARM=6 go build -a -installsuffix cgo -ldflags '-w $(LDFLAGS)' -o server ./cmd

container: server
	docker build --pull -t $(PREFIX):$(TAG) .

push: push
	docker push $(PREFIX):$(TAG)

clean:
	rm -f server

