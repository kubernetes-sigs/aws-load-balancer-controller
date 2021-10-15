#!/bin/bash

set -e

SDK_VENDOR_PATH="./scripts/aws_sdk_model_override/aws-sdk-go"
SDK_MODEL_OVERRIDE_DST_PATH="${SDK_VENDOR_PATH}/models"
SDK_MODEL_OVERRIDE_SRC_PATH="./scripts/aws_sdk_model_override/models"

# Clone the SDK to the vendor path (removing an old one if necessary)
rm -rf "${SDK_VENDOR_PATH}"
git clone --depth 1 https://github.com/aws/aws-sdk-go.git "${SDK_VENDOR_PATH}"

# Override the SDK models
cp -r "${SDK_MODEL_OVERRIDE_SRC_PATH}"/* "${SDK_MODEL_OVERRIDE_DST_PATH}"/.

# Generate the SDK
pushd "${SDK_VENDOR_PATH}"
make generate
popd

# Use the vendored version of aws-sdk-go
go mod edit -replace github.com/aws/aws-sdk-go="${SDK_VENDOR_PATH}"
