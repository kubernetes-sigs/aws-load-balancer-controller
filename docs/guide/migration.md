# Migrating from V1 to V2

AWS LoadBalancer Controller v2, formerly known as aws-alb-ingress-controller now supports the ingress grouping feature wherein it will be feasible to configure a single Application Load Balancer (ALB) for multiple ingress objects. Support is also available for target group binding where an existing group can be used in the ALB.

This document contians the information necessary to migrate from an existing installation of aws-alb-ingress-controller V1 to the new V2 controller.

## Backwards compatibility
The new controller fully supports the existing Ingress objects, no changes will be necessary. Ingresses by default do not belong to any group. It will also not modify the existing ALB and target groups provisioned for existing ingress objects.

## TargetGroupBinding CRD
The new controller requires the TargetGroupBinding CRD to be installed. TargetGroupBinding provides the following benefits -
* Ability to use existing target groups
* More resilient target group handling in case of malformed ingress spec
* Better AWS API usage

### CRD installation
```sh
kubectl apply -k github.com/aws/eks-charts/stable/aws-loadbalancer-controller//crds?ref=master
```

## Uninstalling existing controller
The existing aws-alb-ingress-controller needs to be uninstalled first before upgrading to the new controller. The existing ingress resources need not be deleted.
### Kubectl installation
```shell script
$ kubectl delete deploy -n kube-system alb-ingress-controller
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/VERSION/docs/examples/rbac-role.yaml
```
Where, VERSION is the current version of the ingress controller. To determine the version,
```shell script
$ kubectl describe deployment  -n kube-system  alb-ingress-controller |grep Image
      Image:      docker.io/amazon/aws-alb-ingress-controller:v1.1.8
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.8/docs/examples/rbac-role.yaml
```
### Helm installation
```shell script
$ helm delete aws-alb-ingress-controller -n kube-system
```

## Installing new controller
### IAM permissions
If upgrading from aws-alb-ingress-controller v1, existing IAM permissions will be sufficient.

### Installation
```shell script
# Add the eks-charts helm repo if not already
helm repo add eks https://aws.github.io/eks-charts

# Install the AWS loadbalancer chart
# Please note that the clusterName value must be set via command line or the helm values.yaml. If cluster name is not set, helm chart
# installation will fail. <k8s-cluster-name> is the name of your k8s cluster.
helm install aws-loadbalancer-controller eks/aws-loadbalancer-controller -n kube-system --set clusterName=<k8s-cluster-name>
```
