# Monitoring Controller Metrics with Prometheus
This document describes how to set up Prometheus for monitoring your AWS Load Balancer Controller, what are available metrics and how to access and query them for insights.

## Setting Up Prometheus
### Set up Prometheus with kube-prometheus-stack
To monitor the controller, Prometheus needs to be deployed and configured to scrape metrics from the controller’s HTTP endpoint, This can be done by manually deploy [Promethues Operator](https://github.com/prometheus-operator/prometheus-operator) and the controller expose a metric service and define ServiceMonitor to allow Prometheus to scrape its metric. The easiest way to do this is to install the [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) Helm chart by following the readme file, which provide an automated solution that performs all the set up for you.

### Verify Metric Collection
Based on your metric service config, running the similar command to access it locally

```bash
kubectl port-forward -n kube-system svc/aws-load-balancer-controller-metrics 8080:8080
```

Navigate to [http://localhost:8080/metrics](http://localhost:8080/metrics), you should be able to see all metric in one place.

## Metrics Available
### Controller-Runtime Build in Metric
This project use controller-runtime to implement controllers and admission webhooks, which automatically exposes key metrics for controllers and webhooks following kubernetes instrumentation guidelines. They are available via HTTP endpoint in prometheus metric format

Following metrics are instrumented by default: 

* Latency of processing admission requests.
* Current number of admission requests being served
* Histogram of the latency of processing admission requests
* Total number of admission requests by HTTP status code
* Total number of reconciliations per controller
* Total number of reconciliation errors per controller
* Total number of terminal reconciliation errors per controller
* Total number of reconciliation panics per controller
* Length of time per reconciliation per controller
* Maximum number of concurrent reconciles per controller
* Number of currently used workers per controller

### Custom Metrics from AWS Load Balancer Controller
In addition to the default metrics provided by controller-runtime, this project emits several custom metrics to provide fine-grained view of its behavior and performances. 

Following metrics are added: 

| Name | Type | Description |
|------|------|-------------|
| aws_api_calls_total | Counter | Total number of SDK API calls from the customer's code to AWS services |
| aws_api_call_duration_seconds | Histogram | Perceived latency from when your code makes an SDK call, includes retries |
| aws_api_call_call_retries | Counter | Number of times the SDK retried requests to AWS services for SDK API calls |
| aws_api_requests_total | Counter | Total number of HTTP requests that the SDK made |
| aws_request_duration_seconds | Histogram | Latency of an individual HTTP request to the service endpoint |
| api_call_permission_errors_total | Counter | Number of failed AWS API calls due to auth or authorization failures |
| api_call_service_limit_exceeded_errors_total | Counter | Number of failed AWS API calls due to exceeding service limit |
| api_call_throttled_errors_total | Counter| Number of failed AWS API calls due to throttling error |
| api_call_validation_errors_total | Counter | Number of failed AWS API calls due to validation error |
| awslbc_readiness_gate_ready_seconds | Histogram | Time to flip a readiness gate to true |
| awslbc_reconcile_stage_duration | Histogram | Latency of different reconcile stages |
| awslbc_reconcile_errors_total | Counter | Number of controller errors by error type |
| awslbc_webhook_validation_failures_total | Counter | Number of validation errors by webhook type |
| awslbc_webhook_mutation_failures_total | Counter | Number of mutation errors by webhook type |
| awslbc_top_talkers | Gauge | Number of reconciliations by resource |


##  Accessing and Querying the Metrics in Prometheus UI
To explore and query the collected metrics, access the Prometheus web UI. Running the following command to access it locally

```bash
kubectl port-forward -n prometheus svc/prometheus-operated 9090:9090
```

Navigate to [http://localhost:9090](http://localhost:9090) and check the Status - Target Health page. Ensure that your controller’s endpoint is listed and marked as UP.

Once inside the Prometheus UI, you can use PromQL queries. Here are some examples:

* Get the total reconcile count :  `sum(awslbc_controller_reconcile_errors_total)`
* Get the average reconcile duration for stage : `avg(awslbc_controller_reconcile_stage_duration_sum{controller="service", reconcile_stage="DNS_resolve"})`
* Get the cached object: `sum(awslbc_cache_object_total)`



##  Visualizing Metrics
If you want to further visualize there metrics, one of option is to use Grafana, For set up you can

* Use the Grafana instance included in the kube-promethues-stack.
* Deploy Grafana separately and connect it to Prometheus as a data source.
* Import or create custom dashboards to display relevant metrics.