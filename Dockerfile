# syntax=docker/dockerfile:experimental

FROM --platform=${BUILDPLATFORM} golang:1.15.0 AS base
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY=direct go mod download

FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} COMPILE_OUTPUT="/out/controller" make compile

FROM golangci/golangci-lint:v1.27-alpine AS lint-base

FROM base AS lint
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/golangci-lint \
    --mount=from=lint-base,src=/usr/bin/golangci-lint,target=/usr/bin/golangci-lint \
    golangci-lint run --timeout 10m0s ./...


FROM amazonlinux:2 as amazonlinux
FROM scratch AS bin-unix
COPY --from=build /out/controller /controller
COPY --from=amazonlinux /etc/ssl/certs/ca-bundle.crt /etc/ssl/certs/
ENTRYPOINT ["/controller"]

FROM bin-unix AS bin-linux
FROM bin-unix AS bin-darwin

FROM bin-${TARGETOS} as bin