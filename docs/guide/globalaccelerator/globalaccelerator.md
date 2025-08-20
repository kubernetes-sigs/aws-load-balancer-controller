# AWS Global Accelerator

The AWS Load Balancer Controller supports managing AWS Global Accelerator accelerators through Kubernetes custom resources. This allows you to create and manage Global Accelerators that route traffic to your load balancers across multiple AWS regions.

## Prerequisites

- AWS Load Balancer Controller v2.8.0+ with Global Accelerator support
- IAM permissions for Global Accelerator operations
- Load balancers (ALB/NLB) deployed in target regions

## IAM Permissions

The AWS Load Balancer Controller requires the following IAM permissions to manage Global Accelerator resources:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:CreateAccelerator",
                "globalaccelerator:DescribeAccelerator",
                "globalaccelerator:UpdateAccelerator",
                "globalaccelerator:DeleteAccelerator",
                "globalaccelerator:ListAccelerators",
                "globalaccelerator:CreateListener",
                "globalaccelerator:DescribeListener",
                "globalaccelerator:UpdateListener",
                "globalaccelerator:DeleteListener",
                "globalaccelerator:ListListeners",
                "globalaccelerator:CreateEndpointGroup",
                "globalaccelerator:DescribeEndpointGroup",
                "globalaccelerator:UpdateEndpointGroup",
                "globalaccelerator:DeleteEndpointGroup",
                "globalaccelerator:ListEndpointGroups",
                "globalaccelerator:TagResource",
                "globalaccelerator:UntagResource",
                "globalaccelerator:ListTagsForResource",
                "globalaccelerator:UpdateAcceleratorAttributes",
                "globalaccelerator:DescribeAcceleratorAttributes"
            ],
            "Resource": "*"
        }
    ]
}
```

## Basic Usage

### Creating a Global Accelerator

Create a basic Global Accelerator that routes TCP traffic on port 80 to a load balancer:

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: example-ga
  namespace: default
spec:
  name: "my-global-accelerator"
  enabled: true
  listeners:
    - protocol: "TCP"
      portRanges:
        - fromPort: 80
          toPort: 80
  endpointGroups:
    - region: "us-west-2"
      endpoints:
        - endpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/1234567890abcdef"
```

### Service Integration

You can automatically discover load balancers from Kubernetes LoadBalancer services:

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: service-integrated-ga
spec:
  listeners:
    - protocol: "TCP"
      portRanges:
        - fromPort: 80
          toPort: 80
  endpointGroups:
    - region: "us-west-2"
      endpoints: []  # Will be populated automatically
  serviceEndpoints:
    - name: "my-service"
      weight: 100
```

## Configuration Reference

### GlobalAccelerator Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | `string` | Name of the Global Accelerator. Defaults to `{namespace}-{name}` | No |
| `enabled` | `bool` | Whether the accelerator is enabled. Default: `true` | No |
| `ipAddressType` | `string` | IP address type: `IPv4` or `DUAL_STACK`. Default: `IPv4` | No |
| `listeners` | `[]GlobalAcceleratorListener` | List of listeners | Yes |
| `endpointGroups` | `[]EndpointGroup` | List of endpoint groups | Yes |
| `serviceEndpoints` | `[]ServiceEndpointReference` | Services to discover endpoints from | No |
| `tags` | `[]Tag` | Tags to apply to the Global Accelerator | No |

### GlobalAcceleratorListener

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `protocol` | `string` | Protocol: `TCP` or `UDP` | Yes |
| `portRanges` | `[]PortRange` | Port ranges to listen on | Yes |
| `clientAffinity` | `string` | Client affinity: `NONE` or `SOURCE_IP` | No |

### PortRange

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `fromPort` | `int32` | Start port (1-65535) | Yes |
| `toPort` | `int32` | End port (1-65535) | Yes |

### EndpointGroup

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `region` | `string` | AWS region for this endpoint group | Yes |
| `endpoints` | `[]GlobalAcceleratorEndpoint` | List of endpoints | Yes |
| `trafficDialPercentage` | `*int32` | Percentage of traffic to route (0-100) | No |
| `healthCheckIntervalSeconds` | `*int32` | Health check interval (10-30 seconds) | No |
| `healthCheckPath` | `*string` | Health check path | No |
| `thresholdCount` | `*int32` | Health check threshold (1-10) | No |
| `portOverrides` | `[]PortOverride` | Port overrides for endpoints | No |

### GlobalAcceleratorEndpoint

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `endpointID` | `string` | Endpoint ID (ARN or IP) | Yes |
| `weight` | `*int32` | Endpoint weight (0-255) | No |
| `clientIPPreservationEnabled` | `*bool` | Whether to preserve client IP | No |

### ServiceEndpointReference

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `name` | `string` | Service name | Yes |
| `namespace` | `*string` | Service namespace (defaults to GlobalAccelerator namespace) | No |
| `weight` | `*int32` | Endpoint weight (0-255) | No |

## Advanced Configuration

### Multi-Region Setup

Configure a Global Accelerator with multiple regions and traffic splitting:

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: multi-region-ga
spec:
  name: "multi-region-accelerator"
  listeners:
    - protocol: "TCP"
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      clientAffinity: "SOURCE_IP"
  endpointGroups:
    - region: "us-west-2"
      trafficDialPercentage: 70
      healthCheckIntervalSeconds: 30
      thresholdCount: 3
      endpoints:
        - endpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/west-alb/1234567890abcdef"
          weight: 128
    - region: "us-east-1"
      trafficDialPercentage: 30
      healthCheckIntervalSeconds: 30
      thresholdCount: 3
      endpoints:
        - endpointID: "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/east-alb/abcdef1234567890"
          weight: 128
```

### Port Overrides

Redirect traffic from listener ports to different endpoint ports:

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: port-override-ga
spec:
  listeners:
    - protocol: "TCP"
      portRanges:
        - fromPort: 443
          toPort: 443
  endpointGroups:
    - region: "us-west-2"
      endpoints:
        - endpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/1234567890abcdef"
      portOverrides:
        - listenerPort: 443
          endpointPort: 8443  # Route HTTPS traffic to port 8443 on the endpoint
```

### Health Check Configuration

Configure custom health check settings:

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: health-check-ga
spec:
  listeners:
    - protocol: "TCP"
      portRanges:
        - fromPort: 80
          toPort: 80
  endpointGroups:
    - region: "us-west-2"
      healthCheckIntervalSeconds: 10  # Check every 10 seconds
      healthCheckPath: "/health"      # Custom health check path
      thresholdCount: 2               # Require 2 successful checks
      endpoints:
        - endpointID: "arn:aws:elasticloadbalancing:us-west-2:123456789012:loadbalancer/app/my-alb/1234567890abcdef"
```

## Status and Monitoring

The GlobalAccelerator resource provides status information:

```yaml
status:
  acceleratorARN: "arn:aws:globalaccelerator::123456789012:accelerator/abcd1234-5678-9012-abcd-1234567890ab"
  observedGeneration: 1
```

You can check the status using kubectl:

```bash
kubectl get globalaccelerator example-ga -o yaml
```

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure the AWS Load Balancer Controller has the required Global Accelerator IAM permissions.

2. **Endpoint Not Found**: Verify that the endpoint ARN or IP address is correct and the resource exists in the specified region.

3. **Invalid Configuration**: Check the validation webhook logs for specific configuration errors:
   ```bash
   kubectl logs -n kube-system deployment/aws-load-balancer-controller
   ```

4. **Health Check Failures**: Verify that your endpoints are healthy and responding to health checks on the configured path.

### Validation Errors

The webhook validator will reject invalid configurations:

- At least one listener is required
- At least one endpoint group is required
- Number of endpoint groups must be at least the number of listeners
- Port ranges must be valid (1-65535)
- Protocol must be TCP or UDP
- Region must be specified for each endpoint group

### Debugging

Enable debug logging to get more detailed information:

```bash
kubectl patch deployment aws-load-balancer-controller -n kube-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"controller","args":["--cluster-name=my-cluster","--v=2"]}]}}}}'
```

## Best Practices

1. **Health Checks**: Always configure appropriate health check paths and intervals for your endpoints.

2. **Traffic Distribution**: Use traffic dial percentage to gradually shift traffic between regions during deployments.

3. **Client Affinity**: Use SOURCE_IP client affinity when you need session persistence.

4. **Monitoring**: Monitor Global Accelerator metrics in CloudWatch to track performance and health.

5. **Cost Optimization**: Global Accelerator charges for data transfer and hourly usage. Consider your traffic patterns when configuring multiple endpoint groups.

6. **Security**: Ensure your load balancers have appropriate security groups and access controls configured.

## Examples

See the [examples directory](../../examples/globalaccelerator.yaml) for more comprehensive configuration examples.

## Limitations

- Global Accelerator is only available in specific AWS regions
- Each accelerator can have up to 10 listeners
- Each listener can have up to 10 endpoint groups
- Each endpoint group can have up to 10 endpoints
- Global Accelerator supports TCP and UDP protocols only
- HTTPS/HTTP protocols are not directly supported (use TCP with appropriate port mappings)