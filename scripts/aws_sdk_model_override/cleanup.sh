#!/bin/bash

SDK_VENDOR_PATH="./scripts/aws_sdk_model_override/aws-sdk-go"
rm -rf "${SDK_VENDOR_PATH}"
go mod edit -dropreplace github.com/aws/aws-sdk-go
