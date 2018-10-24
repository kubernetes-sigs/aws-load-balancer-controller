#!/usr/bin/env bash
# This shell script is used to generate & update mock objects for testing
mockery -name ELBV2API -dir ./internal/aws/albelbv2/
mockery -name ResourceGroupsTaggingAPIAPI -dir ./internal/aws/albrgt/
mockery -name Storer -dir ./internal/ingress/controller/store/ -inpkg

mockery -name Controller -dir ./internal/alb/ls/ -inpkg
mockery -name Controller -dir ./internal/alb/rs/ -inpkg