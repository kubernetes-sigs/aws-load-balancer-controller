# AWS Load Balancer Controller

AWS Load Balancer controller Helm chart for Kubernetes

## TL;DR:
```sh
helm repo add eks https://aws.github.io/eks-charts
# If using IAM Roles for service account install as follows -  NOTE: you need to specify both of the chart values `serviceAccount.create=false` and `serviceAccount.name=aws-load-balancer-controller`
helm install aws-load-balancer-controller eks/aws-load-balancer-controller --set clusterName=my-cluster -n kube-system --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller
# If not using IAM Roles for service account
helm install aws-load-balancer-controller eks/aws-load-balancer-controller --set clusterName=my-cluster -n kube-system
```

## Introduction
AWS Load Balancer controller manages the following AWS resources
- Application Load Balancers to satisfy Kubernetes ingress objects
- Network Load Balancers to satisfy Kubernetes service objects of type LoadBalancer with appropriate annotations

## Security updates
**Note**: Deployed chart does not receive security updates automatically. You need to manually upgrade to a newer chart.
#### Node isolation
As a security best practice, we recommend isolating the controller deployment pods to specific node groups which run critical components. The helm chart provides parameters ```nodeSelector```, ```tolerations``` and ```affinity``` to configure node isolation. For more information, please refer to the guidance [here](https://aws.github.io/aws-eks-best-practices/security/docs/multitenancy/#isolating-tenant-workloads-to-specific-nodes).

## Prerequisites
- Supported Kubernetes Versions
  - Chart version v1.5.0+ requires Kubernetes 1.22+
  - Chart version v1.4.0+ requires Kubernetes 1.19+
  - Chart version v1.2.0 - v1.3.3 supports Kubernetes 1.16-1.21
  - Chart version v1.1.6 and before supports Kubernetes 1.15
- IAM permissions
- Helm v3
- Optional dependencies
  - cert-manager
  - Prometheus Operator

The controller runs on the worker nodes, so it needs access to the AWS ALB/NLB resources via IAM permissions. The
IAM permissions can either be setup via IAM roles for ServiceAccount or can be attached directly to the worker node IAM roles.

#### Setup IAM for ServiceAccount
1. Create IAM OIDC provider
    ```
    eksctl utils associate-iam-oidc-provider \
        --region <aws-region> \
        --cluster <your-cluster-name> \
        --approve
    ```
1. Download IAM policy for the AWS Load Balancer Controller
    ```
    curl -o iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/install/iam_policy.json
    ```
1. Create an IAM policy called AWSLoadBalancerControllerIAMPolicy
    ```
    aws iam create-policy \
        --policy-name AWSLoadBalancerControllerIAMPolicy \
        --policy-document file://iam-policy.json
    ```
    Take note of the policy ARN that is returned

1. Create a IAM role and ServiceAccount for the Load Balancer controller, use the ARN from the step above
    ```
    eksctl create iamserviceaccount \
    --cluster=<cluster-name> \
    --namespace=kube-system \
    --name=aws-load-balancer-controller \
    --attach-policy-arn=arn:aws:iam::<AWS_ACCOUNT_ID>:policy/AWSLoadBalancerControllerIAMPolicy \
    --approve
    ```
#### Setup IAM manually
If not setting up IAM for ServiceAccount, apply the IAM policies from the following URL at minimum.
```
https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/main/docs/install/iam_policy.json
```

#### Upgrading from ALB ingress controller
If migrating from ALB ingress controller, grant [additional IAM permissions](https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/install/iam_policy_v1_to_v2_additional.json).

#### Upgrading from  AWS Load Balancer controller v2.1.3 and earlier
- Additional IAM permissions required, ensure you have granted the [required IAM permissions](https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/install/iam_policy.json).
- CRDs need to be updated as follows
```shell script
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller//crds?ref=master"
```
- you can run helm upgrade without uninstalling the old chart completely

#### Installing cert-manager

If you are setting `enableCertManager: true` you need to have installed cert-manager and it's CRDs before installing this chart; to install [cert-manager](https://artifacthub.io/packages/helm/cert-manager/cert-manager) follow the installation guide.

The controller helm chart requires the cert-manager with apiVersion `cert-manager.io/v1`.

Set `cluster.dnsDomain` (default: `cluster.local`) to the actual DNS domain of your cluster to include the FQDN in requested TLS certificates.

#### Installing the Prometheus Operator

If you are setting `serviceMonitor.enabled: true` you need to have installed the Prometheus Operator ServiceMonitor CRD before installing this chart and have the operator running to collect the metrics. The easiest way to do this is to install the [kube-prometheus-stack](https://artifacthub.io/packages/helm/prometheus-community/kube-prometheus-stack) Helm chart using the installation guide.

## Installing the Chart
**Note**: You need to uninstall aws-alb-ingress-controller. Please refer to the [upgrade](#Upgrade) section below before you proceed.

**Note**: Starting chart version 1.4.1, you need to explicitly set `clusterSecretsPermissions.allowAllSecrets` to true to grant the controller permission to access all secrets for OIDC feature. We recommend configuring access to individual secrets resource separately [[link](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/examples/secrets_access/)].

**Note**: To ensure compatibility, we recommend installing the AWS Load Balancer controller image version with its compatible Helm chart version. Use the ```helm search repo eks/aws-load-balancer-controller --versions``` command to find the compatible versions.

Add the EKS repository to Helm:
```shell script
helm repo add eks https://aws.github.io/eks-charts
```

Install the TargetGroupBinding CRDs:

```shell script
kubectl apply -k "github.com/aws/eks-charts/stable/aws-load-balancer-controller/crds?ref=master"
```

Install the AWS Load Balancer controller, if using iamserviceaccount
```shell script
# NOTE: The clusterName value must be set either via the values.yaml or the Helm command line. The <k8s-cluster-name> in the command
# below should be replaced with name of your k8s cluster before running it.
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<k8s-cluster-name> --set serviceAccount.create=false --set serviceAccount.name=aws-load-balancer-controller
```

Install the AWS Load Balancer controller, if not using iamserviceaccount
```shell script
helm upgrade -i aws-load-balancer-controller eks/aws-load-balancer-controller -n kube-system --set clusterName=<k8s-cluster-name>
```

## Upgrade
The new controller is backwards compatible with the existing ingress objects. However, it will not coexist with the older aws-alb-ingress-controller.
The old controller must be uninstalled completely before installing the new version.
### Kubectl installation
If you had installed the previous version via kubectl, uninstall as follows
```shell script
$ kubectl delete deployment -n kube-system alb-ingress-controller
$ kubectl delete clusterRole alb-ingress-controller
$ kubectl delete ClusterRoleBinding alb-ingress-controller
$ kubectl delete ServiceAccount -n kube-system alb-ingress-controller

# Alternatively you can find the version of the controller and delete as follows
$ kubectl describe deployment  -n kube-system  alb-ingress-controller |grep Image
      Image:      docker.io/amazon/aws-alb-ingress-controller:v1.1.8
# You can delete the deployment now
$ kubectl delete deployment -n kube-system alb-ingress-controller
# In this case, the version is v1.1.8, the rbac roles can be removed as follows
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.8/docs/examples/rbac-role.yaml
```
### Helm installation
If you had installed the incubator/aws-alb-ingress-controller Helm chart, uninstall as follows
```shell script
# NOTE: If installed under a different chart name and namespace, please specify as appropriate
$ helm delete aws-alb-ingress-controller -n kube-system
```

If you had installed the 0.1.x version of eks-charts/aws-load-balancer-controller chart earlier, the upgrade to chart version 1.0.0 will
not work due to incompatibility of the webhook api version, uninstall as follows
```shell script
$ helm delete aws-load-balancer-controller -n kube-system
```

## Uninstalling the Chart
```sh
helm delete aws-load-balancer-controller -n kube-system
```

If you setup IAM Roles for ServiceAccount, you can cleanup as follows
```
eksctl delete iamserviceaccount --cluster <cluster-name> --namespace kube-system --name aws-load-balancer-controller
```

## HA configuration
Chart release v1.2.0 and later enables high availability configuration by default.
- The default number of replicas is 2. You can pass`--set replicaCount=1` flag during chart installation to disable this. Due to leader election, only one controller will actively reconcile resources.
- The default priority class for the controller pods is `system-cluster-critical`
- Soft pod anti-affinity is enabled for controller pods with `topologyKey: kubernetes.io/hostname` if you don't configure custom affinity and set `configureDefaultAffinity` to `true`
- Pod disruption budget (PDB) has not been set by default. If you plan on running at least 2 controller pods, you can pass `--set podDisruptionBudget.maxUnavailable=1` flag during chart installation

## Configuration

The following tables lists the configurable parameters of the chart and their default values.
The default values set by the application itself can be confirmed [here](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.4/deploy/configurations/#controller-configuration-options).


| Parameter                                      | Description                                                                                                                                                                                                                                                                                                                                  | Default                                           |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| `image.repository`                             | image repository                                                                                                                                                                                                                                                                                                                             | `public.ecr.aws/eks/aws-load-balancer-controller` |
| `image.tag`                                    | image tag                                                                                                                                                                                                                                                                                                                                    | `<VERSION>`                                       |
| `image.pullPolicy`                             | image pull policy                                                                                                                                                                                                                                                                                                                            | `IfNotPresent`                                    |
| `clusterName`                                  | Kubernetes cluster name                                                                                                                                                                                                                                                                                                                      | None                                              |
| `cluster.dnsDomain`                            | DNS domain of the Kubernetes cluster, included in TLS certificate requests                                                                                                                                                                                                                                                                   | `cluster.local`                                   |
| `securityContext`                              | Set to security context for pod                                                                                                                                                                                                                                                                                                              | `{}`                                              |
| `resources`                                    | Controller pod resource requests & limits                                                                                                                                                                                                                                                                                                    | `{}`                                              |
| `priorityClassName`                            | Controller pod priority class                                                                                                                                                                                                                                                                                                                | system-cluster-critical                           |
| `nodeSelector`                                 | Node labels for controller pod assignment                                                                                                                                                                                                                                                                                                    | `{}`                                              |
| `tolerations`                                  | Controller pod toleration for taints                                                                                                                                                                                                                                                                                                         | `{}`                                              |
| `affinity`                                     | Affinity for pod assignment                                                                                                                                                                                                                                                                                                                  | `{}`                                              |
| `configureDefaultAffinity`                     | Configure soft pod anti-affinity if custom affinity is not configured                                                                                                                                                                                                                                                                        | `true`                                            |
| `topologySpreadConstraints`                    | Topology Spread Constraints for pod assignment                                                                                                                                                                                                                                                                                               | `{}`                                              |
| `deploymentAnnotations`                        | Annotations to add to deployment                                                                                                                                                                                                                                                                                                             | `{}`                                              |
| `podAnnotations`                               | Annotations to add to each pod                                                                                                                                                                                                                                                                                                               | `{}`                                              |
| `podLabels`                                    | Labels to add to each pod                                                                                                                                                                                                                                                                                                                    | `{}`                                              |
| `additionalLabels`                             | Labels to add to all components                                                                                                                                                                                                                                                                                                              | `{}`                                              |
| `rbac.create`                                  | if `true`, create and use RBAC resources                                                                                                                                                                                                                                                                                                     | `true`                                            |
| `serviceAccount.annotations`                   | optional annotations to add to service account                                                                                                                                                                                                                                                                                               | None                                              |
| `serviceAccount.automountServiceAccountToken`  | Automount API credentials for a Service Account                                                                                                                                                                                                                                                                                              | `true`                                            |
| `serviceAccount.imagePullSecrets`              | List of image pull secrets to add to the Service Account                                                                                                                                                                                                                                                                                     | `[]`                                              |
| `serviceAccount.create`                        | If `true`, create a new service account                                                                                                                                                                                                                                                                                                      | `true`                                            |
| `serviceAccount.name`                          | Service account to be used                                                                                                                                                                                                                                                                                                                   | None                                              |
| `terminationGracePeriodSeconds`                | Time period for controller pod to do a graceful shutdown                                                                                                                                                                                                                                                                                     | 10                                                |
| `ingressClass`                                 | The ingress class to satisfy                                                                                                                                                                                                                                                                                                                 | alb                                               |
| `createIngressClassResource`                   | Create ingressClass resource                                                                                                                                                                                                                                                                                                                 | true                                              |
| `ingressClassParams.name`                      | IngressClassParams resource's name, default to the aws load balancer controller's name                                                                                                                                                                                                                                                       | None                                              |
| `ingressClassParams.create`                    | If `true`, create a new ingressClassParams                                                                                                                                                                                                                                                                                                   | true                                              |
| `ingressClassParams.spec`                      | IngressClassParams defined ingress specifications                                                                                                                                                                                                                                                                                            | {}                                                |
| `region`                                       | The AWS region for the kubernetes cluster                                                                                                                                                                                                                                                                                                    | None                                              |
| `vpcId`                                        | The VPC ID for the Kubernetes cluster                                                                                                                                                                                                                                                                                                        | None                                              |
| `awsApiEndpoints`                              | Custom AWS API Endpoints                                                                                                                                                                                                                                                                                                                     | None                                              |
| `awsApiThrottle`                               | Custom AWS API throttle settings                                                                                                                                                                                                                                                                                                             | None                                              |
| `awsMaxRetries`                                | Maximum retries for AWS APIs                                                                                                                                                                                                                                                                                                                 | None                                              |
| `defaultTargetType`                            | Default target type. Used as the default value of the `alb.ingress.kubernetes.io/target-type` and `service.beta.kubernetes.io/aws-load-balancer-nlb-target-type" annotations.`Possible values are `ip` and `instance`.                                                                                                                       | `instance`                                        |
| `enablePodReadinessGateInject`                 | If enabled, targetHealth readiness gate will get injected to the pod spec for the matching endpoint pods                                                                                                                                                                                                                                     | None                                              |
| `enableShield`                                 | Enable Shield addon for ALB                                                                                                                                                                                                                                                                                                                  | None                                              |
| `enableWaf`                                    | Enable WAF addon for ALB                                                                                                                                                                                                                                                                                                                     | None                                              |
| `enableWafv2`                                  | Enable WAF V2 addon for ALB                                                                                                                                                                                                                                                                                                                  | None                                              |
| `ingressMaxConcurrentReconciles`               | Maximum number of concurrently running reconcile loops for ingress                                                                                                                                                                                                                                                                           | None                                              |
| `logLevel`                                     | Set the controller log level - info, debug                                                                                                                                                                                                                                                                                                   | None                                              |
| `metricsBindAddr`                              | The address the metric endpoint binds to                                                                                                                                                                                                                                                                                                     | ""                                                |
| `webhookConfig.disableIngressValidation`       | Disables the validation of resources of kind Ingress                                                                                                                                                                                                                                                                                                      | None                                              |
| `webhookBindPort`                              | The TCP port the Webhook server binds to                                                                                                                                                                                                                                                                                                     | None                                              |
| `webhookTLS.caCert`                            | TLS CA certificate for webhook (auto-generated if not provided)                                                                                                                                                                                                                                                                              | ""                                                |
| `webhookTLS.cert`                              | TLS certificate for webhook (auto-generated if not provided)                                                                                                                                                                                                                                                                                 | ""                                                |
| `webhookTLS.key`                               | TLS private key for webhook (auto-generated if not provided)                                                                                                                                                                                                                                                                                 | ""                                                |
| `webhookNamespaceSelectors`                    | Namespace selectors for the wekbook                                                                                                                                                                                                                                                                                                          | None                                              |
| `keepTLSSecret`                                | Reuse existing TLS Secret during chart upgrade                                                                                                                                                                                                                                                                                               | `true`                                            |
| `serviceAnnotations`                           | Annotations to be added to the provisioned webhook service resource                                                                                                                                                                                                                                                                          | `{}`                                              |
| `serviceMaxConcurrentReconciles`               | Maximum number of concurrently running reconcile loops for service                                                                                                                                                                                                                                                                           | None                                              |
| `targetgroupbindingMaxConcurrentReconciles`    | Maximum number of concurrently running reconcile loops for targetGroupBinding                                                                                                                                                                                                                                                                | None                                              |
| `targetgroupbindingMaxExponentialBackoffDelay` | Maximum duration of exponential backoff for targetGroupBinding reconcile failures                                                                                                                                                                                                                                                            | None                                              |
| `syncPeriod`                                   | Period at which the controller forces the repopulation of its local object stores                                                                                                                                                                                                                                                            | None                                              |
| `watchNamespace`                               | Namespace the controller watches for updates to Kubernetes objects, If empty, all namespaces are watched                                                                                                                                                                                                                                     | None                                              |
| `disableIngressClassAnnotation`                | Disables the usage of kubernetes.io/ingress.class annotation                                                                                                                                                                                                                                                                                 | None                                              |
| `disableIngressGroupNameAnnotation`            | Disables the usage of alb.ingress.kubernetes.io/group.name annotation                                                                                                                                                                                                                                                                        | None                                              |
| `tolerateNonExistentBackendService`            | whether to allow rules that reference a backend service that does not exist. (When enabled, it will return 503 error if backend service not exist)                                                                                                                                                                                           | `true`                                            |
| `tolerateNonExistentBackendAction`             | whether to allow rules that reference a backend action that does not exist. (When enabled, it will return 503 error if backend action not exist)                                                                                                                                                                                             | `true`                                            |
| `defaultSSLPolicy`                             | Specifies the default SSL policy to use for HTTPS or TLS listeners                                                                                                                                                                                                                                                                           | None                                              |
| `externalManagedTags`                          | Specifies the list of tag keys on AWS resources that are managed externally                                                                                                                                                                                                                                                                  | `[]`                                              |
| `livenessProbe`                                | Liveness probe settings for the controller                                                                                                                                                                                                                                                                                                   | (see `values.yaml`)                               |
| `env`                                          | Environment variables to set for aws-load-balancer-controller pod                                                                                                                                                                                                                                                                            | None                                              |
| `envFrom`                                       | Environment variables to set for aws-load-balancer-controller pod from configMap or Secret                                                                                                                                                                                                                                                  | None                                              |
| `envSecretName`                                | AWS credentials as environment variables from Secret (Secret keys `key_id` and `access_key`).                                                                                                                                                                                                                                                | None                                              |
| `hostNetwork`                                  | If `true`, use hostNetwork                                                                                                                                                                                                                                                                                                                   | `false`                                           |
| `dnsPolicy`                                    | Set dnsPolicy if required                                                                                                                                                                                                                                                                                                                    | `ClusterFirst`                                    |
| `extraVolumeMounts`                            | Extra volume mounts for the pod                                                                                                                                                                                                                                                                                                              | `[]`                                              |
| `extraVolumes`                                 | Extra volumes for the pod                                                                                                                                                                                                                                                                                                                    | `[]`                                              |
| `defaultTags`                                  | Default tags to apply to all AWS resources managed by this controller                                                                                                                                                                                                                                                                        | `{}`                                              |
| `replicaCount`                                 | Number of controller pods to run, only one will be active due to leader election                                                                                                                                                                                                                                                             | `2`                                               |
| `revisionHistoryLimit`                         | Number of revisions to keep                                                                                                                                                                                                                                                                                                                  | `10`                                              |
| `podDisruptionBudget`                          | Limit the disruption for controller pods. Require at least 2 controller replicas and 3 worker nodes                                                                                                                                                                                                                                          | `{}`                                              |
| `updateStrategy`                               | Defines the update strategy for the deployment                                                                                                                                                                                                                                                                                               | `{}`                                              |
| `enableCertManager`                            | If enabled, cert-manager issues the webhook certificates instead of the helm template, requires cert-manager and it's CRDs to be installed                                                                                                                                                                                                   | `false`                                           |
| `enableEndpointSlices`                         | If enabled, controller uses k8s EndpointSlices instead of Endpoints for IP targets                                                                                                                                                                                                                                                           | `false`                                           |
| `enableBackendSecurityGroup`                   | If enabled, controller uses shared security group for backend traffic                                                                                                                                                                                                                                                                        | `true`                                            |
| `backendSecurityGroup`                         | Backend security group to use instead of auto created one if the feature is enabled                                                                                                                                                                                                                                                          | ``                                                |
| `disableRestrictedSecurityGroupRules`          | If disabled, controller will not specify port range restriction in the backend security group rules                                                                                                                                                                                                                                          | `false`                                           |
| `objectSelector.matchExpressions`              | Webhook configuration to select specific pods by specifying the expression to be matched                                                                                                                                                                                                                                                     | None                                              |
| `objectSelector.matchLabels`                   | Webhook configuration to select specific pods by specifying the key value label pair to be matched                                                                                                                                                                                                                                           | None                                              |
| `serviceMonitor.enabled`                       | Specifies whether a service monitor should be created, requires the ServiceMonitor CRD to be installed                                                                                                                                                                                                                                       | `false`                                           |
| `serviceMonitor.namespace`                     | Namespace in which to create the service monitor                                                                                                                                                                                                                                                                                             | None                                              |
| `serviceMonitor.additionalLabels`              | Labels to add to the service monitor                                                                                                                                                                                                                                                                                                         | `{}`                                              |
| `serviceMonitor.interval`                      | Prometheus scrape interval                                                                                                                                                                                                                                                                                                                   | `1m`                                              |
| `serviceMonitor.scrapeTimeout`                 | Prometheus scrape timeout                                                                                                                                                                                                                                                                                                                    | `1m`                                              |
| `serviceMonitor.relabelings`                   | Relabelings to apply to samples before ingestion                                                                                                                                                                                                                                                                                             | `1m`                                              |
| `serviceMonitor.metricRelabelings`             | Metric relabelings to apply to samples before ingestion                                                                                                                                                                                                                                                                                      | `1m`                                              |
| `clusterSecretsPermissions.allowAllSecrets`    | If `true`, controller has access to all secrets in the cluster.                                                                                                                                                                                                                                                                              | `false`                                           |
| `controllerConfig.featureGates`                | set of `key: value` pairs that describe AWS load balance controller features                                                                                                                                                                                                                                                                 | `{}`                                              |
| `ingressClassConfig.default`                   | If `true`, the ingressclass will be the default class of the cluster.                                                                                                                                                                                                                                                                        | `false`                                           |
| `enableServiceMutatorWebhook`                  | If `false`, disable the Service Mutator webhook which makes all new services of type LoadBalancer reconciled by the lb controller                                                                                                                                                                                                            | `true`                                            |
| `serviceMutatorWebhookConfig.failurePolicy`    | Failure policy for the Service Mutator webhook                                                                                                                                                                                                                                                                                               | `Fail`                                            |
| `serviceMutatorWebhookConfig.objectSelector`   | Object selector(s) to limit which objects will be mutated by the Service Mutator webhook                                                                                                                                                                                                                                                     | `[]`                                              |
| `serviceMutatorWebhookConfig.operations`       | List of operations that will trigger the the Service Mutator webhook                                                                                                                                                                                                                                                                         | `[ CREATE ]`                                      |
| `autoscaling`                                  | If `autoscaling.enabled=true`, enable the HPA on the controller mainly to survive load induced failure by the calls to the `aws-load-balancer-webhook-service`. Please keep in mind that the controller pods have `priorityClassName: system-cluster-critical`, enabling HPA may lead to the eviction of other low-priority pods in the node | `false`                                           |
| `serviceTargetENISGTags`                       | set of `key=value` pairs of AWS tags in addition to cluster name for finding the target ENI security group to which to add inbound rules from NLBs                                                                                                                                                                                           | None                                              |
| `loadBalancerClass`                            | Sets the AWS load balancer type to be used when the Kubernetes service requests an external load balancer                                                                                                                                                                                                                                    | `service.k8s.aws/nlb`                             |
| `creator`                                      | if set to a `value!=helm`, it will disable the addition of default helm labels                                                                                                                                                                                                                                                               | `helm`                                            |
| `runtimeClassName`                 | Runtime class name for the controller pods , such as `gvisor` or `kata`. An unspecified `nil` or empty `""` RuntimeClassName is equivalent to the backwards-compatible default behavior as if the RuntimeClass feature is disabled.                                                                                                                      | ""                                                | 
