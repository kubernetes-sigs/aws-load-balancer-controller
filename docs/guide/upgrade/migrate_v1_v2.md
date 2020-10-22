# Migrate from v1 to v2
This document contains the information necessary to migrate from an existing installation of AWSALBIngressController(v1) to the new AWSLoadBalancerController(v2).

## Prerequisites
* AWSALBIngressController >=v1.1.3

!!!warning ""
    If you have AWSALBIngressController(<1.1.3) installed, you need to upgrade to version>=v1.1.3(e.g. v1.1.9) first.

    
## Backwards compatibility
The AWSLoadBalancerController(v2.0.0) is backwards-compatible with AWSALBIngressController(>=v1.1.3).

It supports existing AWS resources provisioned by AWSALBIngressController(>=v1.1.3) for Ingress resources with below caveats:

1. The AWS LoadBalancer resource created for your Ingress will be preserved.
    
2. If a numeric TargetPort is used in your service, the AWS TargetGroups created for your Ingress will be re-created.

    !!!warning "downtimes"
        This would cause downtimes to your service during targets registration into new TargetGroups created.
    
    !!!tip "details"
        * The AWSALBIngressController always used `1` as TargetGroup's port.
        * The AWSLoadBalancerController will use 
            * the actual numeric TargetPort as TargetGroup's port if a numeric TargetPort used.
            * `1` as TargetGroup's port if a lexical TargetPort used.
        * The AWSLoadBalancerController will automatically create new TargetGroups and cleanup old TargetGroups if any.

3. If [security-groups](../../guide/ingress/annotations.md#security-groups) annotation used, the SecurityGroup rule on worker node's SecurityGroup that allow LoadBalancer traffic should be manually adjusted post migration.
    
    !!!tip "details"
        when [security-groups](../../guide/ingress/annotations.md#security-groups) annotation used:
        
        * a managed SecurityGroup will be created and attached to ALB. This SecurityGroup will be preserved.
        * an inbound rule will be added to your worker node securityGroups which allow traffic from the above managed SecurityGroup for ALB.
            * The AWSALBIngressController didn't add any description for that inbound rule.
            * The AWSLoadBalancerController will use `elbv2.k8s.aws/targetGroupBinding=shared` for that inbound rule
        * You'll need to manually add `elbv2.k8s.aws/targetGroupBinding=shared` description to that inbound rule so that AWSLoadBalancerController can delete such rule when you delete your Ingress.
    
    !!!tip "sample"
        inbound rule on worker node securityGroups that allow traffic from the managed LB securityGroup before migration:
        
        |Type    | Protocol |Port range|Source                     |Description - optional|
        |--------|----------|----------|---------------------------|----------------------|
        |All TCP |TCP       |0 - 65535 |sg-008c920b1(managed LB SG)|-                     |
        
        inbound rule on worker node securityGroups that allow traffic from the managed LB securityGroup after migration:
        
        |Type    | Protocol |Port range|Source                     |Description - optional|
        |--------|----------|----------|---------------------------|----------------------|
        |All TCP |TCP       |0 - 65535 |sg-008c920b1(managed LB SG)|elbv2.k8s.aws/targetGroupBinding=shared|                     |

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
    1. Install AWSLoadBalancerController(v2.0.0) by following the [installation instructions](../controller/installation.md)
    2. Grant [additional IAM policy](../../install/iam_policy_v1_to_v2_additional.json) needed for migration to the controller.

4. Verify all Ingresses works as expected.