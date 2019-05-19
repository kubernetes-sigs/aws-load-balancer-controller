# Using pod conditions / pod readiness gates

One can add so-called [»Pod readiness gates«](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate) to Kubernetes pods. A readiness gate can be used by e.g. a controller to mark a pod as ready or as unready by setting a custom condition on the pod.

The AWS ALB ingress controller can set such a condition on your pods. This is needed under certain circumstances to achieve full zero downtime rolling deployments. Consider the following example:
* low number of replicas in a deployment (e.g. one to three)
* start a rolling update of the deployment
* rollout of new pods takes less time than it takes the ALB ingress controller to register the new pods and for their health state turn »Health« in the target group
* at some point during this rolling update, the target group might only have registered targets that are in »Initial« or »Draining« state; this results in a downtime of your service

In order to avoid this situation, the AWS ALB ingress controller can set the before mentioned condition on the pods that constitute your ingress backend services. The condition status on a pod will only be set to `true` when the corresponding target in the ALB target group shows a health state of »Healthy«. This prevents the rolling update of a deployment from terminating old pods until the newly created pods are »Healthy« in the ALB target group and ready to take traffic.


## Pod configuration

Add a readiness gate with `conditionType: target-health.alb.ingress.kubernetes.io/<ingress name>` to your pod:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  selector:
    matchLabels:
      app: nginx
  replicas: 2
  template:
    metadata:
      labels:
        app: nginx
    spec:
      readinessGates:
      - conditionType: target-health.alb.ingress.kubernetes.io/nginx-ingress_nginx-service_80
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80

When the pods of this deployment are selected by service which is a backend for the ingress `nginx-ingress`, the ALB ingress controller will set the condition on the pods according to their health state in the ALB target group, making sure the pods do not appear as »Ready« in Kubernetes unless the corresponding target in the ALB target group is consider »Healthy«.
