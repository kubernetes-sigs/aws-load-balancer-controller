#!/usr/bin/env bash
# This shell script is used to generate & update mock objects for testing
mockery -name CloudAPI -dir ./internal/aws/
mockery -name Storer -dir ./internal/ingress/controller/store/ -inpkg

mockery -name Controller -dir ./internal/alb/ls/ -inpkg
mockery -name Controller -dir ./internal/alb/rs/ -inpkg