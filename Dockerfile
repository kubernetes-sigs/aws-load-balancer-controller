# syntax=docker/dockerfile:experimental

FROM --platform=${BUILDPLATFORM} golang:1.15.0 AS base
WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN GOPROXY=direct go mod download

FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on \
    CGO_CPPFLAGS="-D_FORTIFY_SOURCE=2" \
    CGO_LDFLAGS="-Wl,-z,relro,-z,now" \
    go build -ldflags="-s -w" -buildmode=pie -a -o /out/controller main.go

FROM amazonlinux:2 as bin-unix
COPY --from=build /out/controller /controller
USER 1002
ENTRYPOINT ["/controller"]

FROM bin-unix AS bin-linux
FROM bin-unix AS bin-darwin

FROM bin-${TARGETOS} as bin
