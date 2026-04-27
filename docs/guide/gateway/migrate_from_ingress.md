# Migrate from Ingress

!!! warning "Under Development"
    This tool is under active development.
    Features may change, and not all annotation translations are implemented yet.

`lbc-migrate` is a CLI tool that helps migrate AWS Load Balancer Controller (LBC) Ingress resources to Gateway API equivalents. It reads Ingress, Service, IngressClass, and IngressClassParams resources from YAML/JSON files or a live Kubernetes cluster, translates annotations to Gateway API CRD fields, and writes the output manifests.

## Prerequisites

- The Gateway API CRDs must be installed in your cluster at a compatible version before applying the generated manifests. See the [Gateway API installation guide](https://gateway-api.sigs.k8s.io/guides/getting-started/#installing-gateway-api) and the [AWS LBC Gateway API documentation](gateway.md) for supported versions.
- The tool assumes input Ingress resources are valid and currently working with the AWS Load Balancer Controller. It does not re-validate Ingress annotations or enforce the same constraints as the ingress controller (e.g., ALB rule limits, mutually exclusive annotation combinations). If an Ingress has invalid or conflicting annotations, the output may be incorrect or incomplete without warning.

## Installation

Build from source (requires Go):
```bash
# From the root of the aws-load-balancer-controller repo
make lbc-migrate
```

The binary will be at `bin/lbc-migrate`. To install it on your PATH (creates a symlink, so future `make lbc-migrate` rebuilds are picked up automatically):
```bash
make install-lbc-migrate
```

Alternatively, use `go run` directly without building:
```bash
go run ./cmd/lbc-migrate/ [flags]
```

## Usage

```
lbc-migrate [flags]
```

### Input Modes

The tool supports three input modes. You must provide at least one.

#### Option 1: Individual files
```bash
lbc-migrate -f ingress1.yaml,ingress2.yaml
```

#### Option 2: Directory of files
```bash
lbc-migrate --input-dir ./my-manifests/
```

#### Option 3: Read from a live cluster

!!! note "Read-only access"
    The `--from-cluster` mode only performs read operations (list/get). It never creates, updates, or deletes any cluster resources. We recommend using a user or service account with read-only RBAC permissions (e.g. a ClusterRole with only `get` and `list` verbs on Ingress, Service, IngressClass, and IngressClassParams resources).

```bash
lbc-migrate --from-cluster --all-namespaces
lbc-migrate --from-cluster --namespace production
```

You can combine `-f` and `--input-dir`, but `--from-cluster` cannot be used with file-based input.

### Flags

| Flag | Required | Description | Default |
|------|----------|-------------|---------|
| `-f, --file` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Comma-separated input YAML/JSON file paths (e.g. `-f file1.yaml,file2.yaml`) | |
| `--input-dir` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Directory containing YAML/JSON files | |
| `--from-cluster` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Read resources from a live Kubernetes cluster | |
| `--namespace` | Optional | Namespace to read from. Only valid with `--from-cluster`. Mutually exclusive with `--all-namespaces` | Current kubeconfig context namespace |
| `--all-namespaces` | Optional | Read from all namespaces. Only valid with `--from-cluster`. Mutually exclusive with `--namespace` | |
| `--kubeconfig` | Optional | Path to kubeconfig file. Only valid with `--from-cluster` | `$KUBECONFIG` or `~/.kube/config` |
| `--output-dir` | Optional | Directory to write output manifests | `./gateway-output` |
| `--output-format` | Optional | Output format: `yaml` or `json` | `yaml` |
| `--dry-run` | Optional | Add `gateway.k8s.aws/dry-run` annotation to generated Gateway manifests so LBC previews the plan without creating AWS resources | `false` |

## Output Resources

The tool generates a single manifest file (`gateway-resources.yaml` or `.json`) containing all translated Gateway API resources separated by `---`:

| Output Kind | API Group | Description |
|---|---|---|
| `GatewayClass` | `gateway.networking.k8s.io` | Static, always `controllerName: gateway.k8s.aws/alb`. One per run. |
| `Gateway` | `gateway.networking.k8s.io` | One per Ingress (or per `group.name` group, when supported). Listeners from `listen-ports`. |
| `HTTPRoute` | `gateway.networking.k8s.io` | One or more per Ingress. Routes from `spec.rules`. When the Ingress has a `defaultBackend` and host-based rules, a separate catch-all HTTPRoute is generated (see below). |
| `LoadBalancerConfiguration` | `gateway.k8s.aws` | LB-level settings. Only generated when LB-level annotations are present. |
| `TargetGroupConfiguration` | `gateway.k8s.aws` | Per-service TG settings. Only generated when TG-level annotations are present. |
| `ListenerRuleConfiguration` | `gateway.k8s.aws` | Auth, fixed-response, source-ip conditions. |

Existing `Deployment` and `Service` resources are reused as-is and are not generated by the tool. Gateway API HTTPRoute `backendRefs` point directly to your existing Services by name — no changes to your application workload are needed. You keep your existing Deployment and Service manifests and replace only the Ingress manifest with the generated Gateway API resources.

### Default Backend Handling

Gateway API has no `defaultBackend` equivalent ([upstream docs](https://gateway-api.sigs.k8s.io/guides/getting-started/migrating-from-ingress/#default-backend)). When an Ingress has both a `defaultBackend` and host-based rules, the tool generates a separate catch-all HTTPRoute with no hostnames or match conditions. This results in one additional ALB listener rule compared to the original Ingress, which is the expected Gateway API behavior. Additionally, the gateway controller scopes target groups per route, so the separate default-backend HTTPRoute creates its own target group even if it points to the same service as other rules. This means the migrated setup may have one extra target group compared to the Ingress setup, where a single target group is shared across all rules for the same service.

### IngressGroup Support

The migration tool detects `alb.ingress.kubernetes.io/group.name` and produces one shared Gateway + LoadBalancerConfiguration per group, with separate HTTPRoutes per member Ingress (preserving team ownership).

LB-level annotations (scheme, subnets, security groups, tags, etc.) must be consistent across all Ingresses in a group — the tool errors if conflicting values are detected. TLS certificates (`certificate-arn`) are unioned across members. Tags and load-balancer-attributes are unioned with per-key conflict detection.

Each member's `listen-ports` are unioned to build the shared Gateway's listeners. Each member's HTTPRoutes are scoped to only the listeners that member declared via `sectionName` in `parentRefs`. When `ssl-redirect` is set on any member, it applies group-wide: all members' routes attach to the HTTPS listener only, and a single redirect HTTPRoute is generated for HTTP listeners.

Cross-namespace groups (Ingresses in different namespaces with the same `group.name`) are supported. When detected, the migration tool generates the Gateway in the first member's namespace (based on input file order) and sets `allowedRoutes.namespaces.from: All` on each listener, which permits HTTPRoutes from any namespace to attach. You can move the Gateway to a different namespace after generation if needed.

!!! warning "Security consideration"
    `From: All` allows HTTPRoutes from any namespace to attach to the Gateway. If you need tighter scoping, you can manually change `From: All` to `From: Selector` with a label selector after generation.


### Known Differences from Ingress

#### Rule Priority and `group.order`

The `alb.ingress.kubernetes.io/group.order` annotation has no equivalent in Gateway API. In the Ingress model, `group.order` gives explicit control over ALB listener rule priority — rules from a lower-order Ingress always get lower priority numbers (higher precedence). In Gateway API, rule precedence is determined by the [Gateway API specification](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRouteRule): hostname specificity, path match type, path length, header/query param count, route creation timestamp, and route name (alphabetical) etc. There is no annotation or field to override this ordering.

For most configurations — where grouped Ingresses have non-overlapping hosts or paths — this produces equivalent behavior. If your Ingresses rely on `group.order` to resolve overlapping rules (same host + same path across different Ingresses), the migrated Gateway API rules may be evaluated in a different order. Review the generated HTTPRoutes and verify with the dry-run feature before switching traffic.

The migration tool emits a warning when `group.order` is detected.

#### ALB Listener Rule Count and Priority Order

The migrated Gateway API manifests may produce more ALB listener rules than the original Ingress. This happens because ALB supports OR within a single condition (e.g., one `http-request-method` condition with values `[GET, HEAD]`), while Gateway API represents OR as separate `HTTPRouteMatch` entries — each becoming its own ALB rule. The routing behavior is functionally equivalent, but the rule count and priority numbers may differ. This affects conditions with multiple values for `path-pattern`, `http-request-method`, and `query-string`.

Additionally, ALB listener rule priority order may differ between Ingress and Gateway. The Ingress controller assigns priorities based on Ingress spec rule ordering, while the gateway controller follows the [Gateway API precedence specification](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.HTTPRouteRule) (path tyoe, path length, reation timestamp etc). For most configurations this produces equivalent behavior, but users with overlapping rules that depend on specific priority ordering should verify after migration.

#### TargetGroupBinding

AWS Load Balancer Controller uses `TargetGroupBinding` (TGB) internally to register pod targets for Ingress, Service, and Gateway resources. These controller-managed TGBs are created and deleted automatically — when you delete an Ingress, its TGBs are cleaned up, and the Gateway controller creates new ones when it reconciles. You do not need to migrate these.

If you have user-created TGBs that register pods into externally-managed AWS Target Groups (see [TargetGroupBinding documentation](../targetgroupbinding/targetgroupbinding.md)), these are independent of Ingress and Gateway resources. They are reconciled by the TargetGroupBinding controller regardless of how traffic is routed. No changes needed during migration.

#### External Target Group References in Actions

If your Ingress uses `actions.*` annotations that reference external target groups (via `targetGroupARN` or `targetGroupName`), the migration tool translates these to Gateway API `backendRefs` with `kind: TargetGroupName`. When the original annotation uses an ARN, the tool extracts the target group name from it. See [Specify out-of-band Target Groups](gateway.md#specify-out-of-band-target-groups) for how the gateway controller handles these references.

Note: external target groups can only be associated with one ALB at a time. During side-by-side migration (Phase 3-5), the Gateway ALB cannot attach the same external TG that the Ingress ALB is using. You must either delete the Ingress rules referencing the external TG before applying Gateway manifests (cutover), or create a duplicate TG and update the generated manifest to use the new name.

#### Limited Support for Certain Ingress Annotations
The following Ingress annotation types have limited support in the migration tool and Gatewaty API:

- `host-header` conditions — values are passed through to Gateway API hostnames. Since hostnames are route-level in Gateway API (not per-rule), rules with host-header conditions are split into separate HTTPRoutes with their own hostnames. Complex wildcards (e.g., `www.*.example.com`) or regex values that don't conform to Gateway API hostname format will be rejected by the K8s API server when the manifest is applied.
- `url-rewrite` and `host-header-rewrite` transforms with regex capture group references (e.g., `replace: "/$1"` or `replace: "$1.example.org"`) — Gateway API's `URLRewrite` filter only supports static replacements (`ReplaceFullPath` for paths, `PreciseHostname` for hostnames). Transforms with static replacements (no `$N` references) are translated; transforms with capture group references are skipped. Additionally, the original Ingress transform regex is discarded during migration — the gateway controller generates its own fixed regex from the Gateway API filter type (`^([^?]*)` for path rewrites, `.*` for hostname rewrites). This means the ALB transform will always match all paths or all hostnames for the rule, regardless of what the original Ingress regex was. This is functionally equivalent because the ALB rule's conditions (from the HTTPRoute match) already narrow the traffic before the transform is applied. Verify the generated output if your Ingress uses transforms.

!!! important "What changes and what stays the same"
    The generated output **replaces only your Ingress resource**. Everything else stays untouched:

    - **Namespace** — already exists, no changes needed.
    - **Deployment** — unchanged, continues running your application pods.
    - **Service** — unchanged, the new HTTPRoute `backendRefs` reference it by name.

    Once equivalent gateway manifest is generated from ingress manifests, apply the generated Gateway API manifest alongside your existing workload.


### Annotation Priority Chain

LBC resolves annotations with a priority chain (highest wins):

1. **IngressClassParams** (cluster-level policy)
2. **Service annotations** (per-backend override)
3. **Ingress annotations** (per-Ingress config)
4. **LBC controller defaults**

The migration tool applies this same priority chain when translating. For the most accurate results, use `--from-cluster` which automatically fetches all referenced resources (Services, IngressClasses, IngressClassParams).

## Examples

### Basic: single Ingress file
```bash
lbc-migrate -f ingress.yaml --output-dir ./gateway-output/
```

### Directory of manifests
```bash
lbc-migrate --input-dir ./k8s-manifests/ --output-dir ./gateway-output/
```

### From a live cluster (all namespaces)
```bash
lbc-migrate --from-cluster --all-namespaces --output-dir ./gateway-output/
```

### From a live cluster (specific namespace)
```bash
lbc-migrate --from-cluster --namespace production --output-dir ./gateway-output/
```

## Missing Resource Warnings

When using file-based input (`-f` or `--input-dir`), the tool checks for missing referenced resources and emits warnings to stderr. For example, if an Ingress references a Service that was not provided in the input files:

```
WARNING: Ingress "default/my-ingress" references Service "api-service"
  but it was not provided. Service-level annotation overrides may be missing.

WARNING: Ingress "default/my-ingress" uses IngressClass "alb"
  but it was not provided. IngressClassParams overrides may be missing.

Tip: Use --from-cluster to automatically read all referenced resources,
     or include Service/IngressClass/IngressClassParams files in your input.
```

These warnings are informational — the tool still generates output. For the most accurate results, use `--from-cluster` which automatically fetches all referenced resources.

## Preview Gateway resources with Dry-Run

Before applying the generated Gateway manifests to create real AWS resources, you can use
dry-run mode to preview exactly what the AWS Load Balancer Controller would create. When a
Gateway is annotated with `gateway.k8s.aws/dry-run: "true"`, LBC builds its internal model
stack and writes the serialized plan back to the Gateway as an annotation — **without
creating any AWS resources**.

### How it works

1. Apply a Gateway with the `gateway.k8s.aws/dry-run: "true"` annotation.
2. LBC resolves the GatewayClass, LoadBalancerConfiguration, and attached routes as usual.
3. LBC builds the internal resource model (LoadBalancer, Listeners, ListenerRules,
   TargetGroups, and all their configuration including tags, health checks, attributes,
   and security groups).
4. Instead of calling AWS to create resources, LBC marshals the model to JSON and writes it to
   the Gateway's `gateway.k8s.aws/dry-run-plan` annotation.
5. When you remove the `gateway.k8s.aws/dry-run` annotation, LBC cleans up the `dry-run-plan`
   annotation, then proceeds with normal reconciliation (creating the ALB).

### Enabling dry-run on a Gateway

Use the `--dry-run` flag when running `lbc-migrate` to automatically add the annotation to
the generated Gateway manifests:

```bash
lbc-migrate -f ingress.yaml --output-dir ./gw/ --dry-run
# or from a live cluster
lbc-migrate --from-cluster --namespace production --output-dir ./gw/ --dry-run
```

Alternatively, add the `gateway.k8s.aws/dry-run: "true"` annotation manually to any Gateway
manifest:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  namespace: default
  annotations:
    gateway.k8s.aws/dry-run: "true"
spec:
  gatewayClassName: aws-alb
  listeners:
    - name: https
      port: 443
      protocol: HTTPS
      tls:
        certificateRefs:
          - name: tls-secret
```

Apply it:

```bash
kubectl apply -f my-gateway.yaml
```

### Viewing the dry-run plan

Check that the dry-run plan annotation is populated:

```bash
kubectl get gateway my-gateway \
  -o jsonpath='{.metadata.annotations.gateway\.k8s\.aws/dry-run-plan}' | jq .
```

Example output:

```json
{
  "id": "default/my-gateway",
  "resources": {
    "AWS::ElasticLoadBalancingV2::LoadBalancer": {
      "LoadBalancer": {
        "spec": {
          "name": "k8s-default-mygatew-abc123",
          "type": "application",
          "scheme": "internet-facing",
          "ipAddressType": "ipv4",
          "subnetMapping": [
            {"subnetID": "subnet-aaa"},
            {"subnetID": "subnet-bbb"}
          ],
          "securityGroups": ["sg-xxx"]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::Listener": {
      "443": {
        "spec": {
          "port": 443,
          "protocol": "HTTPS",
          "sslPolicy": "ELBSecurityPolicy-TLS-1-2-2017-01",
          "certificates": [
            {"certificateARN": "arn:aws:acm:us-west-2:123:cert/abc"}
          ]
        }
      }
    },
    "AWS::ElasticLoadBalancingV2::TargetGroup": {
      "default-api-service-8080": {
        "spec": {
          "name": "k8s-default-apisvc-xyz789",
          "targetType": "ip",
          "port": 8080,
          "protocol": "HTTP",
          "healthCheckConfig": {
            "path": "/health",
            "intervalSeconds": 15
          }
        }
      }
    }
  }
}
```

Confirm no AWS resources were created:

```bash
aws elbv2 describe-load-balancers --names k8s-default-mygatew-abc123
# → "LoadBalancer not found" (expected: dry-run did not deploy)
```

### Troubleshooting

If the `gateway.k8s.aws/dry-run-plan` annotation is empty after applying the Gateway, the
model build failed before reaching the dry-run step. Debug with:

1. Check Gateway events for build errors

2. Check Gateway status conditions for validation errors

3. Check controller logs for detailed errors

### Deploying after review

When the plan looks correct, remove the dry-run annotation to let LBC create the actual AWS
resources. The trailing `-` on the annotation key is `kubectl annotate` syntax for removing
an annotation:

```bash
# The trailing `-` tells kubectl to remove (not set) the annotation.
kubectl annotate -n <NAMESPACE> gateway <GATEWAY_NAME> gateway.k8s.aws/dry-run- 
```

This triggers a normal reconciliation:

- The `gateway.k8s.aws/dry-run-plan` annotation is cleared.
- LBC creates the ALB, listeners, target groups, and attaches routes.
- Once the ALB is active, the `Programmed` condition is set to `True` and the ALB DNS appears
  in `status.addresses`.

### What dry-run does NOT do

Dry-run intentionally skips every action that would touch AWS or cluster state beyond the
Gateway annotation/status it owns:

- No AWS resources are created, updated, or deleted.
- No finalizer is added to the Gateway.
- No backend security group is allocated or released.
- No Kubernetes Secrets are monitored for certificate rotation.
- No add-on state (WAF, Shield) is persisted.
- No `serviceReferenceCounter` relations are updated.

### Annotation reference

| Annotation                              | Set by     | Description                                                                 |
| --------------------------------------- | ---------- | --------------------------------------------------------------------------- |
| `gateway.k8s.aws/dry-run`               | User       | Set to `"true"` to enable dry-run mode on a Gateway.                        |
| `gateway.k8s.aws/dry-run-plan`          | Controller | Serialized stack JSON written by LBC when dry-run is enabled. Do not edit. |
