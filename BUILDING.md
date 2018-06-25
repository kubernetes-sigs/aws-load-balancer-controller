# Building

## Download this repo locally

```
$ go get -d github.com/kubernetes-sigs/aws-alb-ingress-controller
$ cd $GOPATH/src/github.com/kubernetes-sigs/aws-alb-ingress-controller
```

## Build the binary and container with the Makefile
```
$ make clean; make
```

## Verify the local container is known to your Docker daemon

```
$ docker images | grep -i alb-ingress-controller 

quay.io/coreos/alb-ingress-controller   1.0-alpha.9         78f356144e33        20 minutes ago      47.4MB
```

> Version can vary based on what's in the Makefile. If you wish to push to your own repo for testing, you can change the version and repo details in the Makefile then do a `docker push`.

## Running locally

If you'd like to make modifications and run this controller locally for the purpose of debugging, the following script can be used a basis for how to bootstrap the controller. It assumes you have a default kubeconfig for your cluster at `~/.kube/config`.

```bash
#!/bin/bash

KUBECTL_PROXY_PID=$(pgrep -fx "kubectl proxy")
echo $KUBECTL_PROXY_PID

if [[ -z $KUBECTL_PROXY_PID ]]
then
    echo "kubectl proxy was not running. Starting it."
else
    echo "Found kubectl proxy is running. Killing it. Starting it."
    kill $KUBECTL_PROXY_PID
fi
kubectl proxy &>/dev/null &

kubectl apply -f ./examples/echoservice/echoserver-namespace.yaml
kubectl apply -f ./examples/echoservice/echoserver-deployment.yaml
kubectl apply -f ./examples/echoservice/echoserver-service.yaml
kubectl apply -f ./examples/echoservice/echoserver-ingress2.yaml
kubectl apply -f ./examples/default-backend.yaml

AWS_REGION=us-east-2 POD_NAME=alb-ingress-controller POD_NAMESPACE=kube-system go run cmd/main.go --apiserver-host=http://localhost:8001 --clusterName=devcluster --ingress-class=alb --default-backend-service=kube-system/default-http-backend
```
