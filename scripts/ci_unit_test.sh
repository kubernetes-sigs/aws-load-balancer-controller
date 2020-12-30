#!/usr/bin/env bash

set -ueo pipefail
set -x

go get github.com/mikefarah/yq/v4
make test
