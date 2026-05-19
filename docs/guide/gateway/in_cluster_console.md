# In-Cluster Migration Console

<!--
Screenshot assets live in docs/guide/gateway/assets/.
When regenerating, keep these rules in mind:
  - Blur or scrub AWS account IDs, ARNs, certificate ARNs, subnet IDs,
    security group IDs, tokens, and any real domain hostnames. Cluster names
    may stay visible.
  - Use a red rectangle (no blur) to highlight the element each caption
    calls out.

Captured images:

  1. console/landing.png
       Landing page at http://localhost:8080.
       Shows the info alert, namespace list with gateway counts.

  2. console/gateway-list.png
       Gateway list for a namespace, showing summary pills per gateway.

  3. console/comparison-overview.png
       Comparison view with view tabs (Resource Map / Diff List), toolbar
       with Hide known changes toggle and Export buttons.

  4. console/resource-map.png
       Resource Map view showing AWS resources arranged left-to-right by type,
       with lines connecting related resources and colors indicating diff status.

  5. console/diff-list.png
       Diff List view with segmented filter control and split columns
       showing resource cards with status tags.

  6. console/detail-drawer.png
       Field-level drawer opened for a LoadBalancer showing the per-field
       diff table with status and Known Cause columns.
-->

!!! warning "Under Development"
    The migration tool and its console are under active development.
    Features may change.

The migration console is a local web UI bundled into the `lbc-migrate` binary.
It compares the ingress controller's dry-run plan against the gateway controller's
dry-run plan side by side, field by field, so you can confirm the generated
Gateway manifests will create same AWS resources as the current Ingres before switching traffic.

The console is read-only. It connects to your Kubernetes cluster using the
current kubeconfig context (the same config used by `kubectl`). It reads Gateway
and Ingress resources and their annotations. It never creates, updates, or
deletes cluster or AWS resources.

## What it supports

- **Cluster-wide discovery** of namespaces that hold dry-run Gateways,
  each paired with its source Ingress via the `gateway.k8s.aws/migrated-from`
  tag.
- **Side-by-side comparison** of the full built model stack — LoadBalancer,
  Listeners, ListenerRules, TargetGroups, TargetGroupBindings, SecurityGroups —
  produced by each controller.
- **Two views** — a Resource Map showing the relationships between AWS resources
  with color-coded status, and a Diff List with side-by-side field-level comparison.
- **Field-level diff** with four statuses: `same`, `changed`, `added`, `removed`.
  Slice fields (e.g. SecurityGroup ingress rules) are compared as multisets.
- **Known-change filtering** that hides known migration artifacts (migrated-from
  tag, controller-generated names, health-check default drift, forward weights,
  `targetGroupARN.$ref` pointer churn) so genuine user-visible changes stand out.
- **Resource correlation across controllers**: `TargetGroup` and
  `TargetGroupBinding` are keyed by `serviceRef.name:port` so the same backing
  service shows as one correlated row instead of a removed+added pair, even
  though the two controllers generate different raw stack IDs.
- **IngressGroup resolution** including cross-namespace groups, using the
  `migrated-from` tag plus a cluster-wide list of Ingresses to locate the single
  group member that carries the plan annotation.
- **Export** — download a self-contained HTML report or raw JSON to share the
  diff with team members who do not have cluster access.

What it does not do:

- It does not call AWS or check live resource state. It only diffs the two
  dry-run plans.
- It does not have a namespace-scoped mode. Cross-namespace ingress groups
  require cluster-wide list permission on Ingresses and Gateways.
- It does not modify any Kubernetes or AWS resource.

## End-to-end dry-run workflow

The console is the last step of a four-step preview flow. It is used to review what AWS resources will be created and how they differs from resources created by Ingress manifests. Each step populates
or consumes a specific annotation; nothing talks to AWS until the final cutover.

### 1. Enable the ingress plan annotation

On the AWS Load Balancer Controller, enable the feature gate:

```
--feature-gates=IngressPlanAnnotation=true
```

This is off by default. Once enabled, the ingress controller writes its
built model stack to each reconciled Ingress as
`alb.ingress.kubernetes.io/dry-run-plan` on every reconcile.

!!! note "IngressGroup behavior"
    For an IngressGroup, all members share one plan. To avoid duplication, the
    controller writes the annotation only to the primary member — the one
    with the lowest `alb.ingress.kubernetes.io/group.order`, tie-broken by
    lexical order of `<namespace>/<name>`. Other members of the group carry
    no plan annotation; the controller actively clears the annotation from
    non-primary members so the console always sees exactly one holder per
    group. The console resolves the holder automatically via the
    `migrated-from` tag.

### 2. Generate the Gateway manifest

Run `lbc-migrate` against your Ingresses. Dry-run is the default, so every
generated Gateway carries `gateway.k8s.aws/dry-run: "true"`:

```bash
lbc-migrate --from-cluster --namespace production --output-dir ./gw/
```

See [Migrate from Ingress](migrate_from_ingress.md) for the full set of input
modes and flags.

### 3. Apply the manifest

```bash
kubectl apply -f ./gw/gateway-resources.yaml
```

Because the Gateways carry the dry-run annotation, the gateway controller
builds the full built model but does **not** create an ALB. It writes the
serialized plan back to the Gateway as `gateway.k8s.aws/dry-run-plan`.

At this point both controllers have attached their plans as annotations on
their respective resources. No AWS resource has been created.

### 4. Launch the console

```bash
lbc-migrate --console
# or bind to a different port
lbc-migrate --console --port 9000
```

The console binds to `http://localhost:8080` by default and operates cluster-wide
using your current kubeconfig context. Open that URL in a browser.

## Using the console

### Landing page

The landing page lists every namespace that has at least one Gateway carrying
a `gateway.k8s.aws/dry-run-plan` annotation, with a count of those Gateways.
Namespaces without dry-run plans do not appear.

An info alert at the top reminds you that the console reads from the cluster
using your current kubeconfig context and that all operations are read-only.

![Landing page showing namespaces with dry-run plans](assets/console/landing.png)

Click a namespace to see its gateways.

### Gateway list

After selecting a namespace, the console lists all Gateways with dry-run plans
in that namespace. Each gateway row shows a summary of its diff status (how
many fields are same, changed, removed, or added).

![Gateway list showing summary pills per gateway](assets/console/gateway-list.png)

Click a gateway to enter the comparison view.

### Comparison view

The comparison view shows two view modes for the selected gateway, toggled via
tabs at the top:

![Comparison view with view tabs and toolbar](assets/console/comparison-overview.png)

**Toolbar** — always visible across both views:

- **View tabs** — switch between Resource Map and Diff List.
- **Hide known changes toggle** — filters out diffs classified as known
  migration artifacts (see [How diffs are classified](#how-diffs-are-classified)
  below). On by default. Affects both the resource map node colors and the diff
  list filtering.
- **Export buttons** — download the diff as a self-contained HTML report or raw
  JSON. A confirmation dialog warns that the exported file can be viewed without
  cluster access.

#### Resource Map

The default view when entering a comparison. It renders the AWS resources
planned by the gateway controller as a left-to-right flow diagram, with each
node color-coded to show how it compares to the ingress model:

`Load Balancer → Listeners → Listener Rules → Target Groups → Target Group Bindings`

Security Groups are shown below the Load Balancer in the same column.

![Resource Map showing AWS resources with color-coded nodes](assets/console/resource-map.png)

Each node is color-coded by its aggregate diff status:

- **Green** — all fields are the same between ingress and gateway models.
- **Yellow** — at least one field differs.
- **Blue** — resource only exists in the gateway model (added).

Resources removed from gateway model (exists in ingress model) are not displayed in resource map. Use diff list view to check them.


When "Hide known changes" is on, nodes whose diffs are all known artifacts
display as green (same) — since the change is expected and do not need
attention.

Lines connect related resources, showing the resource relationships. Click any node
to open the detail drawer with field-level diffs for that resource. Connected
edges highlight when a node is selected.

#### Diff List

The field-level comparison view, showing two columns side by side:

![Diff List with filter controls and resource columns](assets/console/diff-list.png)

- **Segmented filter control** — buttons for `All`, `Same`, `Changed`,
  `Removed`, `Added` with counts. Click a button to scope the view to only
  resources in that status. Clicking an active filter resets back to All.
- **Ingress model** (left) and **Gateway model** (right) — each resource in the
  stack appears as a card. When a status filter is active, each card shows a
  status tag next to the resource ID for accessibility.

Click a card to open the detail drawer.

#### Detail drawer

The drawer lists every field with its ingress-side value, gateway-side value,
and status. It carries its own "Hide known changes" checkbox for per-resource
filtering. When a known cause is identified, it appears in the "Known Cause"
column.

![Detail drawer showing field-level diff with status and Known Cause columns](assets/console/detail-drawer.png)

In Resource Map view, clicking a node opens the drawer with all fields for that
resource (no status filter applied). In Diff List view, the drawer respects the
active status filter.

### Breadcrumb navigation

The top bar shows breadcrumbs for your current location:
`Namespaces / <namespace> / <gateway>`. Click any breadcrumb to navigate back
to that level. The "← Back" button goes up one level.

### How resources are correlated

Resources are matched across the two plans by a correlation key:

- For most resource types the raw stack ID is stable, so the key is the ID itself.
- For `TargetGroup` and `TargetGroupBinding` the two controllers generate
  different raw IDs even when pointing at the same backing service. The console
  keys these on the TGB's `serviceRef.name:port`, producing a single correlated
  row with field-level diffs instead of a removed+added pair.

### How diffs are classified

Every field diff is assigned one of four statuses:

- **same** — both sides produce the same value. Slice fields are compared as
  multisets (`[80, 81]` equals `[81, 80]`) because ALB treats things like
  SecurityGroup ingress rules as unordered.
- **changed** — both sides have the field, values differ.
- **added** — only the gateway side has the field.
- **removed** — only the ingress side has the field.

The `Hide known changes` toggle filters out the following known-artifact cases:

- **`migrated-from` tag added to any resource** — the migration tool stamps
  `spec.tags.gateway.k8s.aws/migrated-from` on every generated resource, so an
  `added` entry for this tag is expected on the gateway side.
- **Controller-generated name change on ALB-family resources** — `spec.name` on
  `LoadBalancer` and `TargetGroup`, `spec.groupName` on `SecurityGroup`, and
  `spec.template.metadata.name` on `TargetGroupBinding`. Marked expected only
  when both sides match the controller-generated format (`k8s-<...>-<10 hex>`,
  two or three dash sections before the suffix). A custom name set via
  annotation on either side still surfaces as a real changed diff.
- **Health-check default drift on TargetGroup** —
  `spec.healthCheckConfig.healthyThresholdCount`,
  `spec.healthCheckConfig.unhealthyThresholdCount`, and
  `spec.healthCheckConfig.matcher.httpCode`. The ingress controller defaults
  to 2 / 2 / 200; the gateway controller defaults to 3 / 3 / 200–399.
- **`weight` added on ListenerRule forward actions** — any field under
  `forwardConfig.targetGroups[...]` ending in `.weight`. The gateway controller
  always emits a weight on every forward target group; the ingress controller
  omits it.
- **`targetGroupARN.$ref` string change** — on ListenerRule
  (`forwardConfig.targetGroups[...].targetGroupARN.$ref`) and on
  TargetGroupBinding (`spec.template.spec.targetGroupARN.$ref`). These `$ref`
  values are JSON pointers into another resource's raw stack ID; the stack
  IDs differ per controller even when they point at the same backing service,
  so the string always differs. Real target-group differences surface on the
  correlated `TargetGroup` row.

Everything not matching these rules is shown as-is, so genuine user-visible
changes are never silently hidden.

### Exporting results

Click **Export HTML** or **Export JSON** in the toolbar to share the diff:

- **HTML** — a self-contained report with embedded styles and a full table of
  all diff entries. Open in any browser with no dependencies.
- **JSON** — the raw diff payload including namespace, gateway, timestamp, and
  all entries. Useful for storing in a ticket or processing programmatically.

Both exports trigger a confirmation dialog warning that the file contains full
resource configurations viewable without cluster access.

### Resolving the ingress source for a Gateway

Each Gateway is paired with the Ingress that holds its dry-run plan. The
console derives the pairing from the `gateway.k8s.aws/migrated-from` tag on
the LoadBalancer resource of every generated plan:

- `ingress/<namespace>/<name>` — standalone Ingress, direct pointer.
- `ingress-group/<group-name>` — the console lists Ingresses cluster-wide,
  filters by `alb.ingress.kubernetes.io/group.name == <group-name>`, and
  returns whichever member currently carries a non-empty `dry-run-plan`
  annotation.

On a healthy group, exactly one member holds the plan. If the console finds
zero or more than one, it surfaces an error on the Gateway card.

## RBAC

The console needs these permissions in the context it runs under:

- Cluster-wide `list` on `gateways.gateway.networking.k8s.io` — for the landing
  page and per-namespace gateway lists.
- Cluster-wide `list` on `ingresses.networking.k8s.io` — for resolving group
  plan holders, since ingress groups can span namespaces.
- `get` on `ingresses.networking.k8s.io` in any namespace that appears on the
  landing page — to read the plan annotation once the holder is resolved.

## Troubleshooting

**"could not determine ingress plan holder: no migrated-from tag found on
LoadBalancer in gateway model"** — the Gateway's dry-run plan lacks a
`migrated-from` tag, which means it was not generated by `lbc-migrate`.
Confirm you applied the output of `lbc-migrate` and not a hand-authored
Gateway.

**"no ingress in group `<name>` carries a dry-run-plan annotation"** — the
ingress controller has not yet written the plan annotation for any member of
that group. Confirm the `IngressPlanAnnotation` feature gate is enabled and
the ingress controller has reconciled the group at least once.

**"multiple ingresses in group `<name>` carry a dry-run-plan annotation"** —
usually a stale annotation left behind after group membership changed.
Manually clear the annotation from all but one member and refresh the console.

**Empty drawer / browser shows stale content** — the console's static assets
are embedded in the binary, so after upgrading `lbc-migrate` you need to
restart it and hard-refresh the browser (DevTools → Network → Disable cache,
or Cmd/Ctrl+Shift+R).

## Limitations

- The console does not verify AWS resource state. It only compares the two
  dry-run plans.
- Cross-namespace explicit groups require cluster-wide list permission on
  Ingresses (see [RBAC](#rbac)). There is no namespace-scoped mode.
