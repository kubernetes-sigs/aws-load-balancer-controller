# Pod readiness gate

AWS Load Balancer controller supports [»Pod readiness gates«](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate) to indicate that pod is registered to the ALB/NLB and healthy to receive traffic.
The controller automatically injects the necessary readiness gate configuration to the pod spec via mutating webhook during pod creation.

For readiness gate configuration to be injected to the pod spec, you need to apply the label `elbv2.k8s.aws/pod-readiness-gate-inject: enabled` to the pod namespace. However, note that this only works with `target-type: ip`, since when using `target-type: instance`, it's the node used as backend, the ALB itself is not aware of pod/podReadiness in such case.

The pod readiness gate is needed under certain circumstances to achieve full zero downtime rolling deployments. Consider the following example:

* Low number of replicas in a deployment
* Start a rolling update of the deployment
* Rollout of new pods takes less time than it takes the AWS Load Balancer controller to register the new pods and for their health state turn »Healthy« in the target group
* At some point during this rolling update, the target group might only have registered targets that are in »Initial« or »Draining« state; this results in service outage

In order to avoid this situation, the AWS Load Balancer controller can set the readiness condition on the pods that constitute your ingress or service backend. The condition status on a pod will be set to `True` only when the corresponding target in the ALB/NLB target group shows a health state of »Healthy«.
This prevents the rolling update of a deployment from terminating old pods until the newly created pods are »Healthy« in the ALB/NLB target group and ready to take traffic.

!!!note "upgrading from AWS ALB ingress controller"
    If you have a pod spec with legacy readiness gate configuration, ensure you label the namespace and create the Service/Ingress objects before applying the pod/deployment manifest.
    The load balancer controller will remove all legacy readiness-gate configuration and add new ones during pod creation.

## Configuration
Pod readiness gate support is enabled by default on the AWS load balancer controller. You need to apply the readiness gate inject label to each of the namespace that you would
like to use this feature. You can create and label a namespace as follows -

```
$ kubectl create namespace readiness
namespace/readiness created

$ kubectl label namespace readiness elbv2.k8s.aws/pod-readiness-gate-inject=enabled
namespace/readiness labeled

$ kubectl describe namespace readiness
Name:         readiness
Labels:       elbv2.k8s.aws/pod-readiness-gate-inject=enabled
Annotations:  <none>
Status:       Active
```
Once labelled, the controller will add the pod readiness gates config to all the pods created subsequently that meet all the following conditions

* There exists a service matching the pod labels in the same namespace
* There exists at least one target group binding that refers to the matching service
* The target type is IP

The readiness gates have the prefix `target-health.elbv2.k8s.aws` and the controller injects the config to the pod spec only during pod creation.

!!!tip "create ingress or service before pod"
    To ensure all of your pods in a namespace get the readiness gate config, you need create your Ingress or Service and label the namespace before creating the pods

## Object Selector
The default webhook configuration matches all pods in the namespaces containing the label `elbv2.k8s.aws/pod-readiness-gate-inject=enabled`. You can modify the webhook configuration further
to select specific pods from the labeled namespace by specifying the `objectSelector`. For example, in order to select resources with `elbv2.k8s.aws/pod-readiness-gate-inject: enabled` label,
you can add the following `objectSelector` to the webhook:
```
  objectSelector:
    matchLabels:
      elbv2.k8s.aws/pod-readiness-gate-inject: enabled
```
To edit,
```
$ kubectl edit mutatingwebhookconfigurations aws-load-balancer-webhook
  ...
  name: mpod.elbv2.k8s.aws
  namespaceSelector:
    matchExpressions:
    - key: elbv2.k8s.aws/pod-readiness-gate-inject
      operator: In
      values:
      - enabled
  objectSelector:
    matchLabels:
      elbv2.k8s.aws/pod-readiness-gate-inject: enabled
  ...
```
When you specify multiple selectors, pods matching all the conditions will get mutated.

## Upgrading from AWS ALB Ingress controller
If you have a pod spec with the AWS ALB ingress controller (aka v1) style readiness-gate configuration, the controller will automatically remove the legacy readiness gates config and add new ones during pod creation if the pod namespace is labelled correctly. Other than the namespace labeling, no further configuration is necessary.
The legacy readiness gates have the `target-health.alb.ingress.k8s.aws` prefix.

## Disabling the readiness gate inject
You can specify the controller flag `--enable-pod-readiness-gate-inject=false` during controller startup to disable the controller from modifying the pod spec.

## Checking the pod condition status

The status of the readiness gates can be verified with `kubectl get pod -o wide`:
```
NAME                          READY   STATUS    RESTARTS   AGE   IP         NODE                       READINESS GATES
nginx-test-5744b9ff84-7ftl9   1/1     Running   0          81s   10.1.2.3   ip-10-1-2-3.ec2.internal   0/1
```

When the target is registered and healthy in the ALB/NLB, the output will look like:
```
NAME                          READY   STATUS    RESTARTS   AGE   IP         NODE                       READINESS GATES
nginx-test-5744b9ff84-7ftl9   1/1     Running   0          81s   10.1.2.3   ip-10-1-2-3.ec2.internal   1/1
```

If a readiness gate doesn't get ready, you can check the reason via:

```console
$ kubectl get pod nginx-test-545d8f4d89-l7rcl -o yaml | grep -B7 'type: target-health'
status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: null
    message: Initial health checks in progress
    reason: Elb.InitialHealthChecking
    status: "True"
    type: target-health.elbv2.k8s.aws/k8s-readines-perf1000-7848e5026b
```
