# Migrate from Ingress to Gateway API

!!! warning "Under Development"
    This tool is under active development.
    Features may change, and not all annotation translations are implemented yet.

This guide covers migrating AWS Load Balancer Controller (LBC) Ingress resources to Gateway API, step by step. The migration is designed to be safe and non-disruptive — new ALBs are created alongside existing ones, so current workloads stay running throughout the process.

Two tools are provided to help:

- **[`lbc-migrate` CLI](lbc_migrate_reference.md)** — translates Ingress manifests (annotations, rules, groups) into equivalent Gateway API YAML.
- **[Migration Console](in_cluster_console.md)** — a local web UI that compares the AWS resource plans produced by both the ingress and gateway controllers field by field, to verify equivalence before applying Gateway manifests for real.

## Overview

The migration follows six steps. Each step is safe to pause at — you can stop and resume at any point.

![Migration flow overview](assets/migration/migration_flow.png)

<<<<<<< Updated upstream
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
lbc-migrate --from-cluster --namespaces production
lbc-migrate --from-cluster --namespaces production --ingress-name my-ingress
```

You can combine `-f` and `--input-dir`, but `--from-cluster` cannot be used with file-based input.

### Flags

| Flag | Required | Description | Default |
|------|----------|-------------|---------|
| `-f, --file` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Comma-separated input YAML/JSON file paths (e.g. `-f file1.yaml,file2.yaml`) | |
| `--input-dir` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Directory containing YAML/JSON files | |
| `--from-cluster` | One of `-f`, `--input-dir`, or `--from-cluster` is required | Read resources from a live Kubernetes cluster | |
| `--namespaces` | Optional | Comma-separated namespaces to read from (e.g. `--namespaces ns-a,ns-b`). Only valid with `--from-cluster`. Mutually exclusive with `--all-namespaces` | |
| `--all-namespaces` | Optional | Read from all namespaces. Only valid with `--from-cluster`. Mutually exclusive with `--namespaces` | |
| `--ingress-name` | Optional | Name of a specific Ingress to migrate. Requires exactly one `--namespaces` value. Only valid with `--from-cluster`. Mutually exclusive with `--all-namespaces` | |
| `--kubeconfig` | Optional | Path to kubeconfig file. Only valid with `--from-cluster` | `$KUBECONFIG` or `~/.kube/config` |
| `--output-dir` | Optional | Directory to write output manifests | `./gateway-output` |
| `--output-format` | Optional | Output format: `yaml` or `json` | `yaml` |
| `--split` | Optional | Split output layout. Empty (default) writes one combined file; `namespace` writes one file per namespace plus a `gatewayclass` file for cluster-scoped resources | `""` |
| `--dry-run` | Optional | Add `gateway.k8s.aws/dry-run` annotation to generated Gateway manifests so LBC previews the plan without creating AWS resources | `false` |

## Output Resources

By default the tool generates a single manifest file (`gateway-resources.yaml` or `.json`) containing all translated Gateway API resources separated by `---`. To split the output into one file per namespace, see [Split output by namespace](#split-output-by-namespace) below.

| Output Kind | API Group | Description |
|---|---|---|
| `GatewayClass` | `gateway.networking.k8s.io` | Static, always `controllerName: gateway.k8s.aws/alb`. One per run. |
| `Gateway` | `gateway.networking.k8s.io` | One per Ingress (or per `group.name` group, when supported). Listeners from `listen-ports`. |
| `HTTPRoute` | `gateway.networking.k8s.io` | One or more per Ingress. Routes from `spec.rules`. When the Ingress has a `defaultBackend` and host-based rules, a separate catch-all HTTPRoute is generated (see below). |
| `LoadBalancerConfiguration` | `gateway.k8s.aws` | LB-level settings. Only generated when LB-level annotations are present. |
| `TargetGroupConfiguration` | `gateway.k8s.aws` | Per-service TG settings. Only generated when TG-level annotations are present. |
| `ListenerRuleConfiguration` | `gateway.k8s.aws` | Auth, fixed-response, source-ip conditions. |

Existing `Deployment` and `Service` resources are reused as-is and are not generated by the tool. Gateway API HTTPRoute `backendRefs` point directly to your existing Services by name — no changes to your application workload are needed. You keep your existing Deployment and Service manifests and replace only the Ingress manifest with the generated Gateway API resources.

### Split output by namespace

By default the tool writes a single `gateway-resources.<ext>` file containing every generated resource. Pass `--split=namespace` to produce one file per namespace plus a file for cluster-scoped resources:

```bash
lbc-migrate --from-cluster --all-namespaces --output-dir ./gw/ --split=namespace
```

Resulting layout:

```
gw/
├── gatewayclass.yaml                    # GatewayClass (cluster-scoped)
├── team-a/gateway-resources.yaml        # Gateway resources scoped to team-a
└── team-b/gateway-resources.yaml        # Gateway resources scoped to team-b
```

Apply everything recursively:

```bash
kubectl apply -R -f ./gw/
```

For cross-namespace IngressGroups (Ingresses in different namespaces sharing a `group.name`), the generated resources naturally split across namespace directories: the shared `Gateway` and `LoadBalancerConfiguration` land in the primary member's namespace (determined by `group.order`, then lexical `namespace/name`), while each member's `HTTPRoute` lands in its own namespace. No `ReferenceGrant` is required because HTTPRoutes only reference backends in their own namespace and the generated Gateway sets `allowedRoutes.namespaces.from: All`.

### Default Backend Handling

Gateway API has no `defaultBackend` equivalent ([upstream docs](https://gateway-api.sigs.k8s.io/guides/getting-started/migrating-from-ingress/#default-backend)). When an Ingress has both a `defaultBackend` and host-based rules, the tool generates a separate catch-all HTTPRoute with no hostnames or match conditions. This results in one additional ALB listener rule compared to the original Ingress, which is the expected Gateway API behavior. Additionally, the gateway controller scopes target groups per route, so the separate default-backend HTTPRoute creates its own target group even if it points to the same service as other rules. This means the migrated setup may have one extra target group compared to the Ingress setup, where a single target group is shared across all rules for the same service.

### IngressGroup Support

The migration tool detects `alb.ingress.kubernetes.io/group.name` and produces one shared Gateway + LoadBalancerConfiguration per group, with separate HTTPRoutes per member Ingress (preserving team ownership).

LB-level annotations (scheme, subnets, security groups, tags, etc.) must be consistent across all Ingresses in a group — the tool errors if conflicting values are detected. TLS certificates (`certificate-arn`) are unioned across members. Tags and load-balancer-attributes are unioned with per-key conflict detection.

Each member's `listen-ports` are unioned to build the shared Gateway's listeners. Each member's HTTPRoutes are scoped to only the listeners that member declared via `sectionName` in `parentRefs`. When `ssl-redirect` is set on any member, it applies group-wide: all members' routes attach to the HTTPS listener only, and a single redirect HTTPRoute is generated for HTTP listeners.

Cross-namespace groups (Ingresses in different namespaces with the same `group.name`) are supported. When detected, the migration tool generates the Gateway in the primary member's namespace (determined by `group.order` annotation, then lexical `namespace/name` — matching LBC's runtime behavior) and sets `allowedRoutes.namespaces.from: All` on each listener, which permits HTTPRoutes from any namespace to attach. You can move the Gateway to a different namespace after generation if needed.

For cross-namespace groups, use `--namespaces ns-a,ns-b` (listing all namespaces the group spans) or `--all-namespaces` to ensure all members are included. Using a single `--namespaces` that omits some members will produce incomplete output without warning.

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
=======
| Step | Action | What happens | Rollback |
|------|--------|--------------|----------|
| 1 | Translate manifests | `lbc-migrate` converts Ingress YAML to Gateway API YAML | Delete generated files |
| 2 | Preview with dry-run | Ingress controller writes its plan to annotation (requires feature gate). Gateway manifests applied with dry-run — gateway controller writes its plan without creating AWS resources. Console compares both plans. | Delete the dry-run Gateway |
| 3 | Apply Gateway manifests | LBC creates new ALBs for the Gateway resources alongside existing Ingress ALBs | Delete Gateway resources |
| 4 | Verify Gateway ALBs | Confirm new ALBs are healthy, configurations are correct, and metrics look normal | — |
| 5 | Shift traffic | Gradually move traffic from Ingress ALBs to Gateway ALBs | Shift traffic back |
| 6 | Cleanup | Delete Ingress and old resources | — |
>>>>>>> Stashed changes

!!! important "What changes and what stays the same"
    The generated output **replaces only your Ingress resource**. Everything else stays untouched:

    - **Namespace** — already exists, no changes needed.
    - **Deployment** — unchanged, continues running your application pods.
    - **Service** — unchanged, the new HTTPRoute `backendRefs` reference it by name.

![Service reuse during migration](assets/migration/service_reuse.png)

## Prerequisites

- AWS Load Balancer Controller installed in the cluster with Gateway API support enabled
- The [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/getting-started/#installing-gateway-api) installed at a compatible version (see [Gateway API documentation](gateway.md))
- `lbc-migrate` binary built (see [Installation](lbc_migrate_reference.md#installation))
- The tool assumes input Ingress resources are valid and currently working with the AWS Load Balancer Controller. It does not re-validate Ingress annotations.

---

## Step 1: Translate Manifests

<<<<<<< Updated upstream
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
lbc-migrate --from-cluster --namespaces production --output-dir ./gateway-output/
```

### From a live cluster (multiple namespaces)
```bash
lbc-migrate --from-cluster --namespaces team-a,team-b --output-dir ./gateway-output/
```

### From a live cluster (single Ingress)
```bash
lbc-migrate --from-cluster --namespaces production --ingress-name my-api --output-dir ./gateway-output/
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
lbc-migrate --from-cluster --namespaces production --output-dir ./gw/ --dry-run
```

Alternatively, add the `gateway.k8s.aws/dry-run: "true"` annotation manually to any Gateway
manifest:
=======
First, install the `lbc-migrate` CLI if you haven't already:

```bash
# Build from the aws-load-balancer-controller repo
make lbc-migrate

# (Optional) Install on your PATH
make install-lbc-migrate
```

See [Installation](lbc_migrate_reference.md#installation) for details.

Then run `lbc-migrate` to convert your Ingress resources to Gateway API equivalents.

### Quick start

```bash
# From YAML files
lbc-migrate -f ingress.yaml --output-dir ./gw/

# From a directory of manifests
lbc-migrate --input-dir ./k8s-manifests/ --output-dir ./gw/

# From a live cluster (recommended — automatically fetches referenced Services, IngressClass, etc.)
lbc-migrate --from-cluster --namespaces production --output-dir ./gw/
```

The tool generates Gateway, HTTPRoute, GatewayClass, and optional CRD resources (LoadBalancerConfiguration, TargetGroupConfiguration, ListenerRuleConfiguration).

!!! tip "Use `--from-cluster` for best results"
    File-based input may miss Service-level annotations and IngressClassParams overrides. `--from-cluster` automatically fetches all referenced resources for the most accurate translation.

**Output:** A directory of Gateway API YAML files ready to apply. By default, `--dry-run=true` is set, so the generated Gateway manifests include the `gateway.k8s.aws/dry-run: "true"` annotation.

For the full CLI reference (all flags, output formats, split modes, IngressGroup handling, annotation support table), see **[Migration Tool (lbc-migrate)](lbc_migrate_reference.md)**.

---

## Step 2: Preview with Dry-Run

Before creating any AWS resources, verify that the generated Gateway manifests will produce the same ALB configuration as your current Ingress.

![Dry-run verification workflow](assets/migration/dryrun_workflow.png)

### 2a. Enable the Ingress plan annotation

On the AWS Load Balancer Controller, enable the feature gate:

```
--feature-gates=IngressPlanAnnotation=true
```

Once enabled, the ingress controller writes its built model stack to each reconciled Ingress as `alb.ingress.kubernetes.io/dry-run-plan`.

!!! note "IngressGroup behavior"
    For an IngressGroup, all members share one plan. The controller writes the annotation only to the primary member (lowest `group.order`, tie-broken by lexical `namespace/name`).

### 2b. Apply the generated Gateway manifests

```bash
kubectl apply -f ./gw/gateway-resources.yaml
```

Because the Gateways carry `gateway.k8s.aws/dry-run: "true"`, the gateway controller builds its model but does **not** create an ALB. It writes the plan back to the Gateway as `gateway.k8s.aws/dry-run-plan`. **No AWS resources are created.**
>>>>>>> Stashed changes

### 2c. Launch the migration console

```bash
lbc-migrate --console
# or bind to a different port
lbc-migrate --console --port 9000
```

Open `http://localhost:8080` in your browser. The console reads both plan annotations and shows a field-by-field comparison of every AWS resource (LoadBalancer, Listeners, ListenerRules, TargetGroups, SecurityGroups) that each controller would create.

The console is read-only and uses your current kubeconfig context — it never modifies any cluster or AWS resources.

Look for:

- **"Changed" diffs** — fields that differ between ingress and gateway plans. Known migration artifacts (naming changes, health-check defaults) are filtered by default.
- **"Added" / "Removed"** — resources or fields present on only one side.

!!! success "When to proceed"
    Review the diffs and confirm they are expected. Some differences are intentional (e.g., you deliberately changed a health-check path). As long as you understand and accept all listed changes, you can proceed to Step 3.

For the full console walkthrough (UI guide, diff classification, export, RBAC, troubleshooting), see **[Migration Console](in_cluster_console.md)**.

### 2d. (Optional) Inspect the raw plan

You can also inspect the plan annotation directly:

```bash
kubectl get gateway my-gateway \
  -o jsonpath='{.metadata.annotations.gateway\.k8s\.aws/dry-run-plan}' | jq .
```

---

## Step 3: Apply Gateway Manifests

Once you've confirmed the dry-run plan matches, regenerate the manifests without dry-run and apply:

```bash
# Regenerate without dry-run
lbc-migrate --from-cluster --namespaces production --output-dir ./gw/ --dry-run=false

# Apply
kubectl apply -f ./gw/gateway-resources.yaml
```

Alternatively, remove the dry-run annotation from the existing Gateway:

```bash
kubectl annotate -n <NAMESPACE> gateway <GATEWAY_NAME> gateway.k8s.aws/dry-run-
```

**What happens:**

- LBC creates **new ALBs** for the Gateway resources (one per Gateway object)
- LBC creates new Target Groups pointing to the **same** Services/Pods
- Both your existing Ingress ALBs and the new Gateway ALBs now route to the same backend Pods
- Existing Ingress ALBs continue working as before — no disruption

!!! note "Service reuse"
    Gateway API HTTPRoute `backendRefs` point directly to your existing Kubernetes Services. No new Services, Deployments, or Pods are created. Both sets of ALBs register the same pod IPs.

---

## Step 4: Verify Gateway ALBs

Before shifting traffic, confirm the new Gateway ALBs are healthy and correctly configured.

### Check Gateway status

```bash
kubectl get gateway <GATEWAY_NAME> -n <NAMESPACE>
```

Look for:

- `Programmed: True` condition — the ALB is fully provisioned
- `status.addresses` — contains the new ALB DNS name

### Verify target group health

```bash
# Get the ALB ARN from Gateway status or AWS console
aws elbv2 describe-target-health --target-group-arn <TG_ARN>
```

All targets should show `healthy`.

### Verify ALB configuration

In the AWS Console or via CLI, confirm:

- Listeners match expected ports and protocols
- Security groups are correct
- SSL certificates are attached
- WAF/Shield settings are in place (if applicable)
- Tags are correct

### Monitor metrics

Check CloudWatch metrics for the new ALBs:

- `HealthyHostCount` — all targets registered and healthy
- `HTTPCode_ELB_5XX_Count` — no unexpected 5xx errors
- `TargetResponseTime` — latency within expected range

### Test directly

```bash
# Get Gateway ALB DNS
GATEWAY_ALB=$(kubectl get gateway <GATEWAY_NAME> -n <NAMESPACE> \
  -o jsonpath='{.status.addresses[0].value}')

# Test a request
curl -H "Host: your-app.example.com" http://$GATEWAY_ALB/health
```

!!! warning "Do not skip verification"
    Confirm the new ALBs serve correct responses and all configurations look right before shifting any production traffic.

---

## Step 5: Shift Traffic

Choose a traffic migration strategy based on your requirements. For each Ingress being migrated, you'll shift traffic from the Ingress ALB to the corresponding Gateway ALB.

![Traffic migration strategies](assets/migration/traffic_strategies.png)

| Strategy | Gradual | Requires Domain | Extra Cost | Risk Level |
|----------|---------|-----------------|------------|------------|
| **A: Sudden Cutover** | No | No | None | HIGH (dev/test only) |
| **B: Route 53 Weighted** | Yes | Yes (Route 53) | None | LOW |
| **C: Global Accelerator** | Yes | No | **Yes** ([AGA pricing](https://aws.amazon.com/global-accelerator/pricing/)) | LOW |

### Option A: Sudden Cutover

**Best for:** Dev/test environments where downtime is acceptable.

If you have a custom domain, update DNS to point to the new ALB:

```bash
# Get the Gateway ALB DNS
GATEWAY_ALB=$(kubectl get gateway <GATEWAY_NAME> -n <NAMESPACE> \
  -o jsonpath='{.status.addresses[0].value}')

# Update Route 53 (or your DNS provider)
aws route53 change-resource-record-sets --hosted-zone-id <ZONE_ID> --change-batch '{
  "Changes": [{
    "Action": "UPSERT",
    "ResourceRecordSet": {
      "Name": "api.example.com",
      "Type": "CNAME",
      "TTL": 60,
      "ResourceRecords": [{"Value": "'$GATEWAY_ALB'"}]
    }
  }]
}'
```

If clients access the ALB DNS directly (no custom domain), update all clients to use the new Gateway ALB DNS name.

### Option B: Route 53 Weighted Routing

**Best for:** Production workloads with a custom domain in Route 53.

Gradually shift traffic using weighted DNS records:

```bash
INGRESS_ALB="k8s-default-myingress-abc123.us-west-2.elb.amazonaws.com"
GATEWAY_ALB=$(kubectl get gateway <GATEWAY_NAME> -n <NAMESPACE> \
  -o jsonpath='{.status.addresses[0].value}')

# Start: 90% Ingress, 10% Gateway
aws route53 change-resource-record-sets --hosted-zone-id <ZONE_ID> --change-batch '{
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "api.example.com",
        "Type": "A",
        "SetIdentifier": "ingress-legacy",
        "Weight": 90,
        "AliasTarget": {
          "HostedZoneId": "<ALB_ZONE_ID>",
          "DNSName": "'$INGRESS_ALB'",
          "EvaluateTargetHealth": true
        }
      }
    },
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "api.example.com",
        "Type": "A",
        "SetIdentifier": "gateway-new",
        "Weight": 10,
        "AliasTarget": {
          "HostedZoneId": "<ALB_ZONE_ID>",
          "DNSName": "'$GATEWAY_ALB'",
          "EvaluateTargetHealth": true
        }
      }
    }
  ]
}'
```

Gradually increase the Gateway weight: 10% → 25% → 50% → 100%. Monitor error rates and latency at each step.

!!! note "DNS TTL"
    Route 53 weighted routing splits traffic at the DNS query level, not per-request. Set a low TTL (e.g., 60s) so changes take effect quickly. Rollback speed is limited by TTL propagation.

### Option C: Global Accelerator

**Best for:** Production workloads without a custom domain, or when you need per-request traffic splitting.

!!! warning "Cost"
    Global Accelerator incurs additional AWS charges (per-accelerator hourly fee + data transfer premium). Review [AWS Global Accelerator pricing](https://aws.amazon.com/global-accelerator/pricing/) before choosing this option. The accelerator can be deleted after migration is complete.

This uses the LBC Global Accelerator CRD for precise traffic control:

```yaml
apiVersion: aga.k8s.aws/v1beta1
kind: GlobalAccelerator
metadata:
  name: migration-aga
  namespace: <NAMESPACE>
spec:
  name: "ingress-gateway-migration"
  ipAddressType: IPV4
  listeners:
    - protocol: TCP
      portRanges:
        - fromPort: 80
          toPort: 80
        - fromPort: 443
          toPort: 443
      endpointGroups:
        - endpoints:
            - type: Ingress
              name: <INGRESS_NAME>
              namespace: <NAMESPACE>
              weight: 100
            - type: Gateway
              name: <GATEWAY_NAME>
              namespace: <NAMESPACE>
              weight: 0
```

**Migration steps:**

1. Apply the GlobalAccelerator CRD (100% to Ingress, 0% to Gateway)
2. Update DNS to point to the Global Accelerator endpoint
3. Gradually shift weights: `100/0` → `90/10` → `50/50` → `0/100`
4. Monitor at each step

See the [Global Accelerator documentation](../globalaccelerator/aga-controller.md) for installation and prerequisites.

---

## Step 6: Cleanup

After 100% traffic is on the Gateway ALBs and has been stable for your desired monitoring period:

### Delete the Ingress

```bash
kubectl delete ingress <INGRESS_NAME> -n <NAMESPACE>
```

This triggers LBC to delete the Ingress ALB and its associated Target Groups.

### Remove the Global Accelerator (if used)

```bash
kubectl delete globalaccelerator migration-aga -n <NAMESPACE>
```

### Disable the feature gate

Remove `IngressPlanAnnotation=true` from the controller's feature gates if no other migrations are in progress.

### Clean up DNS (if using Option B)

Remove the old weighted record set:

```bash
aws route53 change-resource-record-sets --hosted-zone-id <ZONE_ID> --change-batch '{
  "Changes": [{
    "Action": "DELETE",
    "ResourceRecordSet": {
      "Name": "api.example.com",
      "Type": "A",
      "SetIdentifier": "ingress-legacy",
      "Weight": 90,
      "AliasTarget": {
        "HostedZoneId": "<ALB_ZONE_ID>",
        "DNSName": "'$INGRESS_ALB'",
        "EvaluateTargetHealth": true
      }
    }
  }]
}'
```

Update the remaining Gateway record to a standard (non-weighted) record.

---

## Troubleshooting

### Dry-run plan annotation is empty

The model build failed before reaching the dry-run step:

1. Check Gateway events: `kubectl describe gateway <NAME>`
2. Check Gateway status conditions for validation errors
3. Check controller logs for detailed build errors

### Gateway stuck in "not Programmed"

After removing dry-run, if the Gateway doesn't reach `Programmed: True`:

1. Check events for AWS API errors (permissions, quotas)
2. Verify subnet tags and security group configurations
3. Check controller logs: `kubectl logs -n kube-system deployment/aws-load-balancer-controller`

### Console shows unexpected diffs

If the migration console shows real (non-artifact) differences:

1. Review which annotations on your Ingress may not be fully supported yet (see [annotation support](lbc_migrate_reference.md#annotation-support))
2. Check for IngressClassParams overrides that may need manual translation
3. File an issue if you believe the translation is incorrect

---

## Further Reading

- **[Migration Tool (lbc-migrate)](lbc_migrate_reference.md)** — full flag reference, annotation support table, output format details
- **[Migration Console](in_cluster_console.md)** — UI walkthrough, diff classification, export, RBAC
- **[Gateway API Overview](gateway.md)** — LBC's Gateway API support documentation
- **[Global Accelerator](../globalaccelerator/aga-controller.md)** — AGA CRD for traffic splitting
