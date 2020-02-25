# Using pod conditions / pod readiness gates

One can add so-called [»Pod readiness gates«](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate) to Kubernetes pods. A readiness gate can be used by e.g. a controller to mark a pod as ready or as unready by setting a custom condition on the pod.

The AWS ALB ingress controller can set such a condition on your pods. This is needed under certain circumstances to achieve full zero downtime rolling deployments. Consider the following example:
* low number of replicas in a deployment (e.g. one to three)
* start a rolling update of the deployment
* rollout of new pods takes less time than it takes the ALB ingress controller to register the new pods and for their health state turn »Healthy« in the target group
* at some point during this rolling update, the target group might only have registered targets that are in »Initial« or »Draining« state; this results in service outage

In order to avoid this situation, the AWS ALB ingress controller can set the before mentioned condition on the pods that constitute your ingress backend services. The condition status on a pod will only be set to `True` when the corresponding target in the ALB target group shows a health state of »Healthy«. This prevents the rolling update of a deployment from terminating old pods until the newly created pods are »Healthy« in the ALB target group and ready to take traffic.


## Pod configuration

Add a readiness gate with `conditionType: target-health.alb.ingress.kubernetes.io/<ingress name>_<service name>_<service port>` to your pod.

Example:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx-service
spec:
  clusterIP: None
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: nginx
---
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: nginx-ingress
  annotations:
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/scheme: internal
spec:
  rules:
    - http:
        paths:
          - backend:
              serviceName: nginx-service
              servicePort: 80
            path: /*
---
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
```

If your pod is part of multiple ingresses / target groups and you want to make sure your pod is `Healthy` in all of them before it is marked as `Ready`, add one `readinessGate` per ingress.


## <a name="annotations">Ingress annotations</a>

The following annotations can be used on the `Ingress` to control the reconcilation behavior:

* `alb.ingress.kubernetes.io/target-health-reconciliation-strategy`: can be either `initial` (default) or `continuous`
  * `initial`: pod condition statuses are only reconiliated as long as the pod is unready; once it becomes ready, reconciliatio is stopped – it is started again if the pod becomes unready during its runtime
  * `continuous`: pod condition statuses are reconciled as long as the ingress / target group exists; use with care as this can potentially cause a lot of AWS API calls if there are many target groups
* `alb.ingress.kubernetes.io/target-health-reconciliation-interval-seconds`: defines how often the target health is queries from AWS while the reconiliation is running (defaults to 10)
