#!/bin/bash

set -e

SDK_VENDOR_PATH="./scripts/aws_sdk_model_override"
SDK_MODEL_OVERRIDE_DST_PATH="${SDK_VENDOR_PATH}/awsSdkGoV2/service/elasticloadbalancingv2"

# Use the vendored version of aws-sdk-go
go mod edit -replace github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2="${SDK_MODEL_OVERRIDE_DST_PATH}"
