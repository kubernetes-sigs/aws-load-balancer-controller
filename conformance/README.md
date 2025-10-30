
# Gateway API Conformance Testing

This directory contains conformance tests for the AWS Load Balancer Controller's Gateway API implementation. These tests validate compliance with the [Kubernetes Gateway API specification](https://gateway-api.sigs.k8s.io/concepts/conformance/).

## Prerequisites

- Kubernetes cluster (v1.19+)
- kubectl configured to access your cluster
- Gateway API CRDs installed
- Go 1.21+ (for running tests locally)

## Setup

### 1. Install AWS Load Balancer Controller

Follow the [official installation guide](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.14/deploy/installation/) to deploy the controller to your cluster.

### 2. Create a GatewayClass

Create a GatewayClass resource for conformance testing:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: REPLACE_WITH_GATEWAY_CLASS_NAME
spec:
  controllerName: ingress.k8s.aws/alb
```

Apply it:
```bash
kubectl apply -f gatewayclass.yaml
```

### 3. Configure Controller Deployment

Edit the controller deployment to enable Gateway API features:

```bash
kubectl edit deployment -n kube-system aws-load-balancer-controller
```

Ensure these arguments are present:
```yaml
spec:
  template:
    spec:
      containers:
      - args:
        - --default-target-type=ip
        - --feature-gates=NLBGatewayAPI=true,ALBGatewayAPI=true
```

## Running Tests

### Run a Single Test Case

Useful for debugging specific functionality:

```bash
go test ./conformance -run TestConformance -v -args \
--gateway-class=REPLACE_WITH_GATEWAY_CLASS_NAME \
--run-test=HTTPRouteExactPathMatching --allow-crds-mismatch=true --cleanup-base-resources=false \
--debug=true
```

**Arguments explained:**
- `--gateway-class`: Name of the GatewayClass to use for testing
- `--run-test`: Specific test to run (e.g., HTTPRouteExactPathMatching)
- `--allow-crds-mismatch=true`: Allow CRD version mismatches between test suite and cluster
- `--cleanup-base-resources=false`: Keep resources after test for debugging
- `--debug=true`: Enable verbose debug logging

### Run a Conformance Profile

Run a complete profile (e.g., GATEWAY-HTTP) with custom configuration:

```bash
go test -v ./conformance \
--gateway-class=REPLACE_WITH_YOUR_GW_CLASS \
--allow-crds-mismatch=true --cleanup-base-resources=false \
--conformance-profiles=GATEWAY-HTTP \
--supported-features=Gateway,HTTPRoute \
--skip-tests=GatewaySecretInvalidReferenceGrant \
--debug=true
```

**Arguments explained:**
- `--conformance-profiles`: Profile to test (GATEWAY-HTTP, GATEWAY-GRPC, etc.)
- `--supported-features`: Comma-separated list of features your implementation supports
- `--skip-tests`: Comma-separated list of tests to skip


## Exempt Features

Features not yet implemented (tests will be skipped):
- `GatewayStaticAddresses`
- `GatewayHTTPListenerIsolation`
- `HTTPRouteRequestMultipleMirrors`
- `HTTPRouteRequestTimeout`
- `HTTPRouteBackendTimeout`
- `HTTPRouteParentRefPort`
- continue updating...

## Troubleshooting

### CRD Version Mismatch

If you see CRD version errors, use `--allow-crds-mismatch=true` or update your Gateway API CRDs:
```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
```

### Target Group Port Empty
If you see 
```
"error":"TargetGroup port is empty. When using Instance targets, your service must be of type 'NodePort' or 'LoadBalancer'"
```
Make sure argument `--default-target-type=ip` is added to deployment