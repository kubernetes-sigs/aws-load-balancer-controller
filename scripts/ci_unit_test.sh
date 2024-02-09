#!/usr/bin/env bash

set -ueo pipefail
set -x

go install github.com/mikefarah/yq/v4@v4.6.1
make test
