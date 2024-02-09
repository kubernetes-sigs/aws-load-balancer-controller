# AWS Load Balancer Controller Development Guide

We'll walk you through the setup to start contributing to the AWS Load Balancer
Controller project. No matter if you're contributing code or docs,
follow the steps below to set up your development environment.

!!! tip "Issue before PR"
    Of course we're happy about code drops via PRs, however, in order to give
    us time to plan ahead and also to avoid disappointment, consider creating
    an issue first and submit a PR later. This also helps us to coordinate
    between different contributors and should in general help keeping everyone
    happy.


## Prerequisites

Please ensure that you have [properly installed Go][install-go].

[install-go]: https://golang.org/doc/install

!!! note "Go version"
    We recommend to use a Go version of `1.14` or above for development.

## Fork upstream repository

The first step in setting up your AWS Load Balancer controller development
environment is to fork the upstream AWS Load Balancer controller repository to your
personal Github account.


## Ensure source code organization directories exist

Make sure in your `$GOPATH/src` that you have directories for the
`sigs.k8s.io` organization:

```bash
mkdir -p $GOPATH/src/github.com/sigs.k8s.io
```


## `git clone` forked repository and add upstream remote

For the forked repository, you will `git clone` the repository into
the appropriate folder in your `$GOPATH`. Once `git clone`'d, you will want to
set up a Git remote called "upstream" (remember that "origin" will be pointing
at your forked repository location in your personal Github space).

You can use this script to do this for you:

```bash
GITHUB_ID="your GH username"

cd $GOPATH/src/github.com/sigs.k8s.io
git clone git@github.com:$GITHUB_ID/aws-load-balancer-controller
cd aws-load-balancer-controller/
git remote add upstream git@github.com:kubernetes-sigs/aws-load-balancer-controller
git fetch --all

```

## Create your local branch

Next, you create a local branch where you work on your feature or bug fix.
Let's say you want to enhance the docs, so set `BRANCH_NAME=docs-improve` and
then:

```
git fetch --all && git checkout -b $BRANCH_NAME upstream/main
```

## Commit changes

Make your changes locally, commit and push using:

```
git commit -a -m "improves the docs a lot"

git push origin $BRANCH_NAME
```

## Create a pull request

Finally, submit a pull request against the upstream source repository.

We monitor the GitHub repo and try to follow up with comments within a working
day.


## Building the controller

To build the controller binary, run the following command.

```bash
make controller
```

To install CRDs into a Kubernetes cluster, run the following command.

```bash
make install
```

To uninstall CRD from a Kubernetes cluster, run the following command.

```bash
make uninstall
```

To build the container image for the controller and push to a container registry, run the following command.

```bash
make docker-push
```

To deploy the CRDs and the container image to a Kubernetes cluster, run the following command.

```bash
make deploy
```
