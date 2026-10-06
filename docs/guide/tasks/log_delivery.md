# Log Delivery

Load balancers can send their logs through [CloudWatch Logs vended log delivery](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-cloudwatch-logs.html) to CloudWatch Logs, Amazon S3 or Amazon Data Firehose. The controller can manage this delivery for the load balancers it creates:

| Log type                | Load balancer             |
|-------------------------|---------------------------|
| `ALB_ACCESS_LOGS`       | Application Load Balancer |
| `ALB_CONNECTION_LOGS`   | Application Load Balancer |
| `ALB_HEALTH_CHECK_LOGS` | Application Load Balancer |
| `NLB_ACCESS_LOGS`       | Network Load Balancer     |

Vended log delivery is separate from the `access_logs.s3.*` and `connection_logs.s3.*` load balancer attributes, which AWS now calls legacy logs. Both can be enabled on the same load balancer, so you can move from one to the other without a gap.

## Enabling log delivery

Set the `LogDelivery` [feature gate](../../deploy/configurations.md#feature-gates):

```
--feature-gates=LogDelivery=true
```

or, with Helm:

```
helm upgrade aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system \
  --set controllerConfig.featureGates.LogDelivery=true
```

!!!note "Permissions"
    The controller needs extra permissions in its IAM role. You can find a policy to attach to the existing role [here](../../install/iam_policy_log_delivery.json). It only lets the controller manage delivery sources and destinations whose names start with `k8s-`.

While the feature gate is disabled, the controller ignores the log delivery settings below.

For the opt-in E2E testing plan, prerequisites and run commands, see
[log delivery tests](https://github.com/kubernetes-sigs/aws-load-balancer-controller/tree/main/test/e2e/logdelivery).

### Destination permissions

CloudWatch Logs writes to the destination as the `delivery.logs.amazonaws.com` service principal, so every destination needs a resource policy that allows it:

* a log group needs a CloudWatch Logs resource policy, see [logs sent to CloudWatch Logs](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/AWS-logs-infrastructure-V2-CloudWatchLogs.html),
* a bucket needs a bucket policy, see [logs sent to Amazon S3](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/AWS-logs-infrastructure-V2-S3.html),
* a Firehose stream needs the `LogDeliveryEnabled=true` tag and the `AWSServiceRoleForLogDelivery` service-linked role, see [logs sent to Firehose](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/AWS-logs-infrastructure-V2-Firehose.html).

Set these up when you create the destination. That way the controller never changes a policy.

CloudWatch Logs can also add the policy itself when the caller is allowed to change it. If you want that, grant the controller the extra permissions only for the destinations it may use, never for `*`:

```json
{
    "Effect": "Allow",
    "Action": ["s3:GetBucketPolicy", "s3:PutBucketPolicy"],
    "Resource": "arn:aws:s3:::my-alb-logs"
}
```

The equivalents are `logs:PutResourcePolicy`, `logs:DescribeResourcePolicies` and `logs:DescribeLogGroups` for log groups, and `firehose:TagDeliveryStream` plus `iam:CreateServiceLinkedRole` on `AWSServiceRoleForLogDelivery` for Firehose.

!!!warning "Security Risk"
    Anyone who can create or modify an Ingress, Service or LoadBalancerConfiguration chooses where that load balancer's logs go. With the policy above, they can only use destinations that already allow `delivery.logs.amazonaws.com`. Granting `s3:PutBucketPolicy`, `logs:PutResourcePolicy` or `firehose:TagDeliveryStream` broadly would let them make CloudWatch Logs change the policy of any bucket, log group or stream in the account. Keep those permissions scoped to log destinations. For IngressGroups, see [IngressGroup Security Risk](../ingress/annotations.md#group.name).

## Configuration

Each load balancer takes a list of deliveries. Every entry has these fields:

| Field                                              | Description |
|----------------------------------------------------|-------------|
| `logType`                                          | One of the log types above. |
| `destinationArn`                                   | ARN of a CloudWatch Logs log group, an S3 bucket (optionally followed by a prefix, such as `arn:aws:s3:::my-bucket/my-prefix`) or a Firehose delivery stream. The controller creates a delivery destination for it. |
| `deliveryDestinationArn`                           | ARN of an existing delivery destination, for example one that a central logging account shares with you. The controller uses it as is. Set either `destinationArn` or `deliveryDestinationArn`. |
| `outputFormat`                                     | CloudWatch Logs: `plain`, `json`. Firehose: `plain`, `json`, `raw`. S3: `plain`, `json`, `w3c`, `parquet`. Only with `destinationArn`. Defaults to the CloudWatch Logs default for the destination. |
| `fieldDelimiter`                                   | A tab (`"\t"`), a space or a comma, for `plain` and `w3c` output. |
| `s3DeliveryConfiguration.suffixPath`               | Path appended to the S3 object key, using the variables `{accountid}`, `{region}`, `{yyyy}`, `{MM}`, `{dd}` and `{HH}`. |
| `s3DeliveryConfiguration.enableHiveCompatiblePath` | Writes the path variables as `key=value`. |

An empty list turns log delivery off and removes the deliveries the controller created.

### Ingress

Use the [log-delivery annotation](../ingress/annotations.md#log-delivery). Ingresses in the same IngressGroup that set it must set the same value.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: echoserver
  annotations:
    alb.ingress.kubernetes.io/log-delivery: |
      [
        {"logType": "ALB_ACCESS_LOGS", "destinationArn": "arn:aws:logs:us-west-2:111122223333:log-group:/aws/vendedlogs/alb/echoserver", "outputFormat": "json"},
        {"logType": "ALB_HEALTH_CHECK_LOGS", "destinationArn": "arn:aws:s3:::my-alb-logs", "outputFormat": "parquet",
         "s3DeliveryConfiguration": {"enableHiveCompatiblePath": true}}
      ]
```

### Service

Use the [aws-load-balancer-log-delivery annotation](../service/annotations.md#log-delivery):

```yaml
apiVersion: v1
kind: Service
metadata:
  name: echoserver
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-log-delivery: |
      [{"logType": "NLB_ACCESS_LOGS", "destinationArn": "arn:aws:s3:::my-nlb-logs", "outputFormat": "parquet"}]
```

### Gateway

Use the [logDelivery field](../gateway/loadbalancerconfig.md#logdelivery) of a LoadBalancerConfiguration:

```yaml
apiVersion: gateway.k8s.aws/v1
kind: LoadBalancerConfiguration
metadata:
  name: example-config
  namespace: echoserver
spec:
  logDelivery:
    - logType: ALB_ACCESS_LOGS
      destinationArn: arn:aws:logs:us-west-2:111122223333:log-group:/aws/vendedlogs/alb/echoserver
      outputFormat: json
    - logType: ALB_CONNECTION_LOGS
      deliveryDestinationArn: arn:aws:logs:us-west-2:444455556666:delivery-destination:central-alb-logs
```

## Resources the controller creates

For each load balancer the controller creates:

* one delivery source per log type, named `k8s-<namespace>-<name>-<hash>-<log type>`, for example `k8s-echoserv-echoserv-0123456789abcdef0123-alb_access`,
* one delivery destination per `destinationArn` and output format, named `k8s-<namespace>-<name>-<hash>-<cwl|s3|fh>-<hash>`,
* one delivery per entry.

The name prefix is unique to the cluster, controller type and Ingress group, Service or Gateway, and the controller uses it to find its resources. Ingresses, Services and Gateways with the same namespace and name have different prefixes. The resources also carry the usual `elbv2.k8s.aws/cluster` and stack tags plus the load balancer's tags. The controller skips a resource with its prefix whose cluster or stack tag has a different value.

The controller removes these resources when you remove an entry, set an empty list, or delete the Ingress, Service or Gateway. When a load balancer is replaced, the controller recreates its delivery source for the new load balancer. Delivery destinations passed as `deliveryDestinationArn` are never changed or deleted.

## Things to know

* A load balancer can only have one delivery source per log type. If one already exists that the controller didn't create, for example from Terraform or a CloudWatch telemetry enablement rule, the controller reports a conflict instead of taking it over.
* CloudWatch Logs can't filter delivery listings, so each reconcile lists the account's delivery sources, and also its delivery destinations and deliveries when the load balancer has log delivery.
* The controller only compares `fieldDelimiter` and `s3DeliveryConfiguration` values that you set. If you remove one of them, the delivery keeps its current value. Tags are set when a resource is created and aren't updated afterwards.
* Cross-account delivery supports S3 and Firehose destinations only. The destination account must allow it with a delivery destination policy. See the [cross-account example](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/vended-logs-crossaccount-example.html).
* Vended logs are charged by CloudWatch Logs. See [CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/).
