#!/bin/bash

set -e

echo 'mode: count' > profile.cov

for pkg in $(go list ./... | grep -v '/vendor/');
do
    dir="$GOPATH/src/$pkg"
    len="${#PWD}"
    dir_relative=".${dir:$len}"


    go test -v -short -covermode=count -coverprofile="$dir_relative/profile.tmp" "$dir_relative"
    if [ -f "$dir_relative/profile.tmp" ]
    then
        cat "$dir_relative/profile.tmp" | tail -n +2 >> profile.cov
        rm "$dir_relative/profile.tmp"
    fi

done

# test coverage
echo 'processing code coverage...'
go tool cover -func profile.cov

# To submit the test coverage result to coveralls.io,
# use goveralls (https://github.com/mattn/goveralls)

goveralls -coverprofile=profile.cov -service=travis-ci -repotoken $COVERALLS_REPO_TOKEN || true
