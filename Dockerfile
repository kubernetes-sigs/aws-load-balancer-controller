# Copyright 2017[<0;55;12M The Kubernetes Authors. All rights reserved.
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

FROM golang:1.13.4-stretch as builder
WORKDIR /go/src/github.com/kubernetes-sigs/aws-alb-ingress-controller/
COPY . .
RUN make server

FROM amazonlinux:2 as amazonlinux
FROM scratch
COPY --from=amazonlinux /etc/ssl/certs/ca-bundle.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/kubernetes-sigs/aws-alb-ingress-controller/server server
ENTRYPOINT ["/server"]
