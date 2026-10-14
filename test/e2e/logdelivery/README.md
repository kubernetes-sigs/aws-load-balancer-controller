# Log delivery testing plan

The `LogDelivery` controller feature and this E2E suite are both opt-in. The suite
skips in `BeforeSuite` unless `--enable-log-delivery-tests` is set, before creating
AWS or Kubernetes clients. It creates temporary Kubernetes resources and load
balancers when enabled, and cleans up their controller-managed log deliveries.
The supplied log group, S3 bucket and optional existing delivery destination are
externally managed and are retained.

## Coverage

| Case | Checks | Additional prerequisites |
| --- | --- | --- |
| ALB Ingress to CloudWatch and S3 | All three ALB log types; shared CloudWatch destination; actual access-log arrival after HTTP traffic; cleanup after an empty annotation; LB still serves traffic | Log group and bucket |
| ALB Ingress S3 updates | Delivery ID stays the same after delimiter, suffix path and Hive settings change; output-format change replaces the delivery and destination; cleanup on Ingress deletion | Log group and bucket |
| Existing delivery destination | Annotation removal deletes the source and delivery; the supplied destination remains unchanged | `--log-delivery-existing-destination-arn` |
| NLB Service | One source with CloudWatch and S3 destinations; TLS traffic produces a CloudWatch log record; cleanup after an empty annotation | `--certificate-arns` |
| ALB Gateway | `LoadBalancerConfiguration.spec.logDelivery` creates a delivery; backend serves traffic; clearing the field removes delivery resources | `--enable-gateway-tests` |
| NLB Gateway | TLS listener and backend traffic; delivery created and removed through `spec.logDelivery` | Gateway flag and certificate |

Every fixture verifies that the supplied log group still exists with the same
retention and KMS key after cleanup. Delivery discovery uses the actual load
balancer ARN, independently of the controller's name-generation implementation.
Polling allows up to 15 minutes for reconciliation and log arrival, with repeated
requests because ELB log delivery is best-effort.

The suite verifies S3 delivery configuration through the AWS delivery APIs. It
does not read S3 objects or check their Parquet contents. Unit tests cover
validation and reconciliation, including load-balancer replacement and the
disabled feature gate. Firehose and cross-account delivery have no live E2E
cases in this suite.

## Prerequisites

1. An EKS test cluster in a region supporting ELB vended log delivery, with the
   usual E2E prerequisites (tagged subnets, test image access, controller service
   account, and base controller IAM policy).
2. This PR's controller image, installed with `LogDelivery=true`, and the
   additional [controller IAM policy](../../../docs/install/iam_policy_log_delivery.json).
   With `--controller-image`, the suite upgrades the controller via the supplied
   Helm chart and enables the gate itself. It verifies the deployment has rolled
   out before running cases.
3. An existing CloudWatch log group in the test account and region, and an S3
   bucket in the same region. Authorize `delivery.logs.amazonaws.com` for both as
   described in the [log delivery guide](../../../docs/guide/tasks/log_delivery.md#destination-permissions).
   Configure retention and encryption externally; for a customer-managed KMS
   key, include the destination's required KMS permissions.
4. The test runner needs the usual Kubernetes/ELB E2E permissions, plus
   `logs:DescribeDeliverySources`, `logs:DescribeDeliveryDestinations`,
   `logs:DescribeDeliveries`, `logs:GetDeliverySource`, `logs:GetDeliveryDestination`,
   `logs:GetDelivery`, `logs:DescribeLogGroups`, and `logs:FilterLogEvents`.
   These are runner permissions; the controller keeps its separate IAM role.
5. NLB cases need an issued ACM certificate in the test region. They skip if
   `--certificate-arns` is omitted because NLB access logs require a TLS listener.
6. Gateway cases need `--enable-gateway-tests`, Gateway API v1.6 CRDs (including
   TCPRoute), this PR's LBC Gateway CRDs, and the ALB/NLB Gateway controller gates.
   The optional existing-destination case needs a pre-authorized destination in
   the test account and region that accepts ALB access logs.

Use a dedicated test cluster: supplying `--controller-image` changes its
controller deployment. The suite adds a unique namespace to the supplied S3
prefix. Logs written to the external destinations remain for their configured
retention or lifecycle policies.

## Run against an existing cluster

From the repository root, set `KUBECONFIG`, `CLUSTER_NAME`, `AWS_REGION`, `VPC_ID`,
`CONTROLLER_IMAGE`, `LOG_GROUP_ARN`, and `S3_BUCKET_ARN` for the test environment.
Use the log group ARN without the IAM `:*` suffix.

```bash
ginkgo -v --procs=1 --timeout=2h --junit-report=log-delivery-junit.xml \
  ./test/e2e/logdelivery -- \
  --kubeconfig="$KUBECONFIG" \
  --cluster-name="$CLUSTER_NAME" \
  --aws-region="$AWS_REGION" \
  --aws-vpc-id="$VPC_ID" \
  --helm-chart=./helm/aws-load-balancer-controller \
  --controller-image="$CONTROLLER_IMAGE" \
  --enable-log-delivery-tests=true \
  --log-delivery-log-group-arn="$LOG_GROUP_ARN" \
  --log-delivery-s3-bucket-arn="$S3_BUCKET_ARN"
```

For all six cases, additionally pass `--certificate-arns="$CERTIFICATE_ARN"`,
`--enable-gateway-tests=true`, and
`--log-delivery-existing-destination-arn="$DELIVERY_DESTINATION_ARN"`.
Keep the verbose output and JUnit report to distinguish passed and skipped cases.
On failure, also collect controller logs and the affected object's events.

## Run through the existing CI script

The existing script builds the image and creates a test cluster. Enable the new
suite with:

```bash
ENABLE_LOG_DELIVERY_TESTS=true \
LOG_DELIVERY_LOG_GROUP_ARN="$LOG_GROUP_ARN" \
LOG_DELIVERY_S3_BUCKET_ARN="$S3_BUCKET_ARN" \
LOG_DELIVERY_EXISTING_DESTINATION_ARN="${DELIVERY_DESTINATION_ARN:-}" \
  ./scripts/ci_e2e_test.sh
```

The existing-destination variable is optional. The script attaches the additional
controller IAM policy only when enabled and removes that policy during cleanup.
The default remains `ENABLE_LOG_DELIVERY_TESTS=false`. It uses the existing
certificate configuration for the NLB Service case; run the dedicated suite with
the Gateway flag to include Gateway cases.

The script installs the Ginkgo CLI version pinned in `go.mod` and compiles one
E2E suite at a time to limit peak linker memory. When log delivery tests are
disabled, it excludes the `logdelivery` package from compilation and execution.

## Local validation and recorded results

These checks use Go 1.26.5. They do not create AWS resources:

```bash
go test -race ./pkg/... ./webhooks/... ./controllers/... ./cmd/lbc-migrate/... ./test/framework/...
go vet ./...
go test -run '^$' ./test/e2e/...
go test -race -v ./test/e2e/logdelivery
bash -n scripts/ci_e2e_test.sh
```

Recorded results are summarized in the PR's testing section. A successful default
invocation of `test/e2e/logdelivery` validates its disabled path and helper unit
tests; its six live specs are skipped. It is not evidence of live AWS success.
Run the opt-in command above in the maintainers' test account to obtain those
results.

Local validation on 2026-10-09 passed the race-enabled controller/framework tests,
`go vet ./...`, compilation of all E2E suites, and the CI script's shell syntax
check. The new suite's helper tests passed; its default run skipped all six live
specs as intended. Live AWS E2E results are pending.

After limiting CI compilation to `--compilers=1`, all eight default E2E binaries
and the opt-in log delivery binary compiled for Linux amd64 in a local container
capped at 8 GiB RAM and 4 CPUs. Peak container memory across those builds was
approximately 4.2 GiB. Command checks also verified that the CI script excludes
the log delivery package only when disabled and installs the CLI from `go.mod`.
