#!/usr/bin/env bash
# This shell script is used to generate & update mock objects for testing
mockery -name CloudAPI -dir ./internal/aws/
mockery -name Storer -dir ./internal/ingress/controller/store/ -inpkg

mockery -name Controller -dir ./internal/alb/tags/ -inpkg
mockery -name Controller -dir ./internal/alb/ls/ -inpkg
mockery -name Controller -dir ./internal/alb/rs/ -inpkg

mockery -name ACMAPI -dir ./vendor/github.com/aws/aws-sdk-go/service/acm/acmiface
mockery -name EC2API -dir ./vendor/github.com/aws/aws-sdk-go/service/ec2/ec2iface
mockery -name ELBV2API -dir ./vendor/github.com/aws/aws-sdk-go/service/elbv2/elbv2iface
mockery -name IAMAPI -dir ./vendor/github.com/aws/aws-sdk-go/service/iam/iamiface
mockery -name ResourceGroupsTaggingAPIAPI -dir ./vendor/github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi/resourcegroupstaggingapiiface
mockery -name WAFRegionalAPI -dir ./vendor/github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface

