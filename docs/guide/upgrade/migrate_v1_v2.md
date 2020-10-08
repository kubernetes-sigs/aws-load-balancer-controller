# Migrate from v1 to v2
This document contains the information necessary to migrate from an existing installation of AWSALBIngressController(v1) to the new AWSLoadBalancerController(v2).

## Prerequisites
* AWSALBIngressController >=v1.1.3

!!!warning ""
    If you have AWSALBIngressController(<1.1.3) installed, you need to upgrade to version>=v1.1.3(e.g. v1.1.9) first.
    
    

## Backwards compatibility
The AWSLoadBalancerController(v2.0.0) is backwards-compatible with AWSALBIngressController(>=v1.1.3).

It supports existing AWS resources provisioned by AWSALBIngressController(>=v1.1.3) for Ingress resources.

## Upgrade steps
1. Determine existing installed AWSALBIngressController version.
```console
foo@bar:~$ kubectl describe deployment  -n kube-system  alb-ingress-controller | grep Image
    Image:      docker.io/amazon/aws-alb-ingress-controller:v1.1.9
```

2. Uninstalling existing AWSALBIngressController(>=v1.1.3).

    Existing AWSALBIngressController needs to be uninstalled first before install new AWSLoadBalancerController.
    
    !!!note ""
        Existing Ingress resources do not need to be deleted.

3. Install new AWSLoadBalancerController

    Install AWSLoadBalancerController(v2.0.0) by following the [installation instructions](../controller/installation.md)

4. Verify all Ingresses works as expected.