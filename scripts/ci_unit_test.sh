#!/usr/bin/env bash

set -ueo pipefail
set -x

go get github.com/mikefarah/yq/v3
make test
