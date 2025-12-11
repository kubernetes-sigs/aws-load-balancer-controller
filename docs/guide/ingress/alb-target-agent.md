# ALB Target Optimizer

## Overview

Target optimizer for Application Load Balancer enables ALB customers to configure capacity-aware load balancing, useful for workloads that have strict limitations on how many concurrent requests each target can process.

You can enable target optimizer on a target group. Target optimizer lets you accurately enforce a maximum number of concurrent requests on a target. It works with the help of an agent that you install and configure on targets.

## How It Works

Target Optimizer requires two steps:

**Step 1: Enable Target Control Port**

Specify the [`target-control-port`](annotations.md#target-control-port) annotation when creating target groups. This port is used for management traffic between the agents and load balancer.

Target optimizer can only be enabled during target group creation. Target control port once specified cannot be modified. Controller will create a new target group with modified target control port and reassociate it with the listener.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
  annotations:
    alb.ingress.kubernetes.io/target-control-port.${serviceName}.${servicePort}: 3000 # Must match agent controlAddress port
spec:
```

**Step 2: Deploy the ALB Target Control Agent**

The agent serves as an inline proxy between the load balancer and your application. You configure the agent to enforce a maximum number of concurrent requests that the load balancer can send to the target.

The agent tracks the number of requests the target is processing. When the number falls below your configured maximum value, the agent sends a signal to the load balancer letting it know that the target is ready to process another request.

Deploy the ALB Target Control Agent as a sidecar container in your pods. The agent automatically handles the gRPC control protocol - it listens on the data port, communicates target capacity to ALB, and proxies traffic to your application container.

The agent is available as a Docker image at: https://gallery.ecr.aws/aws-elb/target-optimizer/target-control-agent

### Deployment Options

You can deploy the ALB Target Control Agent in two ways:

For more details about this feature, see the [ALB Target Optimizer announcement](https://aws.amazon.com/about-aws/whats-new/2025/11/aws-application-load-balancer-target-optimizer).

#### Option 1: Manual Sidecar Deployment

Manually add the ALB Target Control Agent container to your pod specifications.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
    - image: application:latest
      imagePullPolicy: Always
      name: app
      ports:
        - containerPort: 8080
  initContainers:
    - image: public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest
      imagePullPolicy: Always
      restartPolicy: Always
      name: alb-target-control-agent
      env:
        - name: TARGET_CONTROL_DATA_ADDRESS
          value: "0.0.0.0:80"
        - name: TARGET_CONTROL_CONTROL_ADDRESS
          value: "0.0.0.0:3000"
        - name: TARGET_CONTROL_DESTINATION
          value: "127.0.0.1:8080"
        - name: TARGET_CONTROL_MAX_CONCURRENCY
          value: "1000"
      ports:
        - containerPort: 80
        - containerPort: 3000
```

#### Option 2: Automatic Injection

Use the AWS Load Balancer Controller's built-in injector to automatically add the agent as a sidecar container to your pods. This is the easiest approach and provides centralized configuration management.

---

## Automatic Injection Setup

### Prerequisites

To use ALB target control agent injection, you need **all** of the following:

1. **Apply ALBTargetControlConfig CRD** - Install the ALBTargetControlConfig Custom Resource Definition
2. **Apply mutating webhooks** - Install pod and namespace webhooks for automatic sidecar injection
3. **Create ALBTargetControlConfig resource** - Configure agent settings (controller namespace configuration serves as default, pod namespace configuration takes precedence)
4. **Enable ALB target control agent in the controller** - Set the `ALBTargetControlAgent` feature gate to `true`
5. **Enable injection** - Add labels to namespaces or pods to control where injection occurs

Without all steps, no sidecar injection will happen.

## Setup

### Step 1: Install CRDs

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/refs/heads/main/config/crd/bases/elbv2.k8s.aws_albtargetcontrolconfigs.yaml
```

### Step 2: Apply Mutating Webhooks

The controller uses two mutating webhooks to automatically inject the agent sidecar:

- **Namespace-level webhook** (`alb-target-control.namespace.elbv2.k8s.aws`) - Injects sidecar for all pods in labeled namespaces
- **Pod-level webhook** (`alb-target-control.object.elbv2.k8s.aws`) - Injects sidecar for individually labeled pods

**With Helm:**
Webhooks are automatically applied during `helm upgrade`.

**Without Helm:**
Manually apply the webhook configurations:

```bash
kubectl apply -k https://github.com/kubernetes-sigs/aws-load-balancer-controller/config/webhook
```

### Step 3: Update RBAC Permissions

If you're using custom RBAC or have modified the default controller permissions, ensure the controller has access to ALBTargetControlConfig resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aws-load-balancer-controller-role
rules:
  # ... existing rules ...
  - apiGroups:
      - elbv2.k8s.aws
    resources:
      - albtargetcontrolconfigs
    verbs:
      - get
```

**Note**: If you installed the controller using the official Helm chart or manifests, these permissions are already included.

### Step 4: Create ALBTargetControlConfig

```yaml
apiVersion: elbv2.k8s.aws/v1beta1
kind: ALBTargetControlConfig
metadata:
  name: aws-load-balancer-controller-alb-target-control-agent-config
spec:
  image: "public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest"
  destinationAddress: "127.0.0.1:8080"
  maxConcurrency: 1
  controlAddress: "0.0.0.0:3000"
  dataAddress: "0.0.0.0:80"
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
```

### Step 5: Enable ALB Target Control Agent in Controller

**With Helm:**

```bash
helm upgrade aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set controllerConfig.featureGates.ALBTargetControlAgent=true
```

**Without Helm:**
Add `--feature-gates=ALBTargetControlAgent=true` to controller args in the deployment.

### Step 6: Enable Injection

Add labels to namespaces or pods to control where injection occurs:

### Namespace-Level Control

Enable injection for all pods in a namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  labels:
    elbv2.k8s.aws/alb-target-control-agent-injection: enabled # Enable for all pods in namespace
```

### Pod-Level Control (Highest Priority)

Override namespace settings per pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  labels:
    elbv2.k8s.aws/alb-target-control-agent-inject: "true"   # Force injection
    # OR
    elbv2.k8s.aws/alb-target-control-agent-inject: "false"  # Block injection
spec:
  containers:
  - name: app
    image: myapp:latest
```

### Pod-Specific Configuration

Override sidecar settings per pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  annotations:
    # Container configuration
    elbv2.k8s.aws/alb-target-control-agent-image: "alb-target-control:custom-version"
    # Address configuration
    elbv2.k8s.aws/alb-target-control-agent-destination-address: "127.0.0.1:9090"
    elbv2.k8s.aws/alb-target-control-agent-max-concurrency: "2"
    elbv2.k8s.aws/alb-target-control-agent-control-address: "0.0.0.0:3001"
    elbv2.k8s.aws/alb-target-control-agent-data-address: "0.0.0.0:8080"
    # TLS configuration
    elbv2.k8s.aws/alb-target-control-agent-tls-cert-path: "/etc/tls/tls.crt"
    elbv2.k8s.aws/alb-target-control-agent-tls-key-path: "/etc/tls/tls.key"
    # Logging configuration
    elbv2.k8s.aws/alb-target-control-agent-rust-log: "debug"
    # Resource limits
    elbv2.k8s.aws/alb-target-control-agent-cpu-request: "200m"
    elbv2.k8s.aws/alb-target-control-agent-cpu-limit: "1000m"
    elbv2.k8s.aws/alb-target-control-agent-memory-request: "256Mi"
    elbv2.k8s.aws/alb-target-control-agent-memory-limit: "1Gi"
  labels:
    elbv2.k8s.aws/alb-target-control-agent-inject: "true"
spec:
  containers:
    - name: app
      image: myapp:latest
```

**Note**: After enabling ALB target control agent injection, only newly created pods will have the sidecar injected. Existing pods need to be restarted to get the sidecar.

---

## Labels and Annotations Reference

| Type            | Key                                                          | Values              | Description                        |
| --------------- | ------------------------------------------------------------ | ------------------- | ---------------------------------- |
| Pod Label       | `elbv2.k8s.aws/alb-target-control-agent-inject`              | `"true"`, `"false"` | Force enable/disable injection     |
| Namespace Label | `elbv2.k8s.aws/alb-target-control-agent-injection`           | `"enabled"`         | Enable injection for namespace     |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-image`               | Image name          | Override sidecar image             |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-destination-address` | host:port format    | Override application destination   |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-max-concurrency`     | Number              | Override max concurrency           |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-control-address`     | host:port format    | Override control interface address |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-data-address`        | host:port format    | Override data interface address    |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-cpu-request`         | CPU value           | Override CPU request               |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-cpu-limit`           | CPU value           | Override CPU limit                 |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-memory-request`      | Memory value        | Override memory request            |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-memory-limit`        | Memory value        | Override memory limit              |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-tls-cert-path`       | File path           | Override TLS certificate path      |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-tls-key-path`        | File path           | Override TLS private key path      |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-rust-log`            | debug,info,error    | Override Rust log level            |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-policy`              | string              | Override restart policy            |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-protocol-version`    | string              | Override protocol version          |
| Pod Annotation  | `elbv2.k8s.aws/alb-target-control-agent-injected`            | `"true"`            | Added after successful injection   |

---

## Complete Example

Here's a minimal working example that demonstrates ALB Target Optimizer with manual agent injection:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: alb-target-control-example
---
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: alb-target-control-example
  name: echo-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: echo-app
  template:
    metadata:
      labels:
        app: echo-app
    spec:
      containers:
        - name: echo-server
          image: k8s.gcr.io/e2e-test-images/echoserver:2.5
          ports:
            - containerPort: 8080
      initContainers:
        - name: alb-target-control-agent
          image: public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest
          imagePullPolicy: Always
          restartPolicy: Always
          env:
            - name: TARGET_CONTROL_DATA_ADDRESS
              value: "0.0.0.0:80" # Agent listens on port 80 for data
            - name: TARGET_CONTROL_CONTROL_ADDRESS
              value: "0.0.0.0:3000" # Agent listens on port 3000 for control
            - name: TARGET_CONTROL_DESTINATION
              value: "127.0.0.1:8080" # Forward to echo server
            - name: TARGET_CONTROL_MAX_CONCURRENCY
              value: "100"
          ports:
            - containerPort: 80 # Data port
            - containerPort: 3000 # Control port
---
apiVersion: v1
kind: Service
metadata:
  namespace: alb-target-control-example
  name: echo-service
spec:
  selector:
    app: echo-app
  ports:
    - port: 80
      targetPort: 80 # Target agent's data port
      protocol: TCP
  type: NodePort
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  namespace: alb-target-control-example
  name: echo-ingress
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip # Required for target control
    alb.ingress.kubernetes.io/target-control-port.echo-service.80: "3000" # Must match agent control port
spec:
  ingressClassName: alb
  rules:
    - http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: echo-service
                port:
                  number: 80
```

**Key Points:**

- Service targets agent's **data port** (80) where traffic flows
- Ingress annotation specifies agent's **control port** (3000) for ALB communication
- Target type must be `ip` for target control functionality
- Agent forwards traffic from data port (80) to application (8080)
