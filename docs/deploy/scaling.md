# Scaling your controller deployment

The AWS Load Balancer Controller (LBC) implements a standard Kubernetes controller. The controller reads changes from the cluster
API server, calculates an intermediate representation (IR) of your AWS resources, then ensures the actual AWS resources match
the IR state. The controller can perform CRUD operations to ensure the Kubernetes and AWS resources stay in-sync. This page is
meant to 1/ inform users about some LBC internals and 2/ help users get higher performance out of their LBC.

As of writing, the controller uses a high-availability deployment model using an active-passive mode. When running multiple replicas
of the LBC, only one replica is responsible for talking to AWS to change the state of resources. The extra replicas are able to assist with
webhook invocations, e.g. for object validation or mutation, but will not change the state of any resources within AWS unless the active
leader replica relinquishes the leader lock. Generally, it is recommended to run at least two replicas for fast fail-over of leadership changes.
During leadership changes, there is a 15-second to 2 minute stoppage of CRUD operations that can lead to state drift between your cluster
and AWS resources. Another benefit of running multiple replicas is to alleviate some load from the leader replica, as more replicas
mean fewer webhook invocations on the leader replica.

## Resource Allocation

By default, the provided installation bundle sets the CPU and memory requests / limits to:

```
        resources:
          limits:
            cpu: 200m
            memory: 500Mi
          requests:
            cpu: 100m
            memory: 200Mi
```

these limits are used by the default threading model the LBC uses which is:

- 3 threads for Ingress management (ALB)
- 3 threads for Service management (NLB)
- 3 threads for ALB Gateway management (IF ENABLED)
- 3 threads for NLB Gateway management (IF ENABLED)
- 3 threads for TargetGroupBinding management (Target Registration for ALB / NLB)

For 99.9% of use-cases, these values are enough. When managing a large number of resources, the threads should be tuned in turn the
memory and CPU resources should be tuned. Here's a general formula:

** This formula is just a suggestion, and many workloads might perform differently. It's important to load test your exact scenario **

For every 200 Ingresses your controller manages, add three additional Ingress threads.

For every 400 Services your controller manages, add three additional Service threads.

For every 100 TargetGroupBindings, add three additional TargetGroupBinding threads.

** Gateway thread management still needs research **

A good formula to use for setting CPU requests / limit is to add 50m per 10 threads added.

A good formula to use for setting Memory requests / limit is to add 100Mi per 10 threads added.

Use these controller flags to update the threadpools:
```
--targetgroupbinding-max-concurrent-reconciles
--service-max-concurrent-reconciles
--ingress-max-concurrent-reconciles
--alb-gateway-max-concurrent-reconciles
--nlb-gateway-max-concurrent-reconciles
```


** Important **

When adding more threads, the LBC will call AWS APIs more often. See the next section for how to raise your AWS API limits to accommodate
more threads.


## API throttling


There is multiple layers of API throttling to consider.

### Kubernetes API <-> LBC

Cluster administrators may configure the Kubernetes API, LBC interaction using this document.
[Kubernetes Throttling](https://kubernetes.io/docs/concepts/cluster-administration/flow-control/)

### LBC <-> AWS APIs

The LBC uses clientside throttling and AWS APIs use server side throttling.

This document talks about the AWS API throttling mechanisms.
[AWS API Throttling](https://aws.amazon.com/blogs/mt/managing-monitoring-api-throttling-in-workloads/)

#### Clientside throttling

The LBC implements clientside throttling by default, to preserve AWS API throttle volume for other processes that
may need to communicate with AWS. By default, this is the clientside throttling configuration:

````
Elastic Load Balancing v2:RegisterTargets|DeregisterTargets=4:20,Elastic Load Balancing v2:.*=10:40
````

To decipher what this means, let's break it down. We are setting the ELBv2 APIs (the ELB APIs the controller talks to)
to limit the controller to four register / deregister calls per second with a token bucket allowance that allows spikes up to 20 tps.
The other (10:40) rule limits the overall calls to the ELBv2 APIs, no matter the API invoked. The overall allowance is 10 calls per second,
with a burst allowance of 40 tps.

#### AWS Serverside throttling

AWS allows for server-siding throttling limit increases for valid uses-cases, cut a support ticket with your use-case if you 
see throttling within the controller. Make sure to increase the clientside throttles when a limit increase is granted.





