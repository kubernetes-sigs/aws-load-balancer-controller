# Migrate from Ingress

!!! warning "Under Development"
    This tool is under active development and is not ready for production use.
    Features may change, and annotation translation is not yet implemented.
    Do not use this tool for production migrations at this time.

`lbc-migrate` is a CLI tool that helps migrate AWS Load Balancer Controller (LBC) Ingress resources to Gateway API equivalents. It reads Ingress, Service, IngressClass, and IngressClassParams resources from YAML/JSON files or a live Kubernetes cluster, and writes them to an output directory.

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

### Supported Input Formats

Both YAML and JSON Kubernetes manifests are supported. Multi-document YAML files (separated by `---`) are handled automatically. The tool recognizes the following resource types:

- `networking.k8s.io/v1` Ingress
- `networking.k8s.io/v1` IngressClass
- `v1` Service
- `elbv2.k8s.aws/v1beta1` IngressClassParams

Unrecognized resource types in the input are silently skipped.

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

### Custom kubeconfig
```bash
lbc-migrate --from-cluster --all-namespaces --kubeconfig ~/.kube/staging-config --output-dir ./gateway-output/
```

### JSON output format
```bash
lbc-migrate -f ingress.yaml --output-dir ./gateway-output/ --output-format json
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
