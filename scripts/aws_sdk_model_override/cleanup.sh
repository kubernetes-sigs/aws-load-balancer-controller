#!/bin/bash

SDK_VENDOR_PATH="./scripts/aws_sdk_model_override/awsSdkGoV2"
rm -rf "${SDK_VENDOR_PATH}"
go mod edit -dropreplace github.com/aws/aws-sdk-go-v2
