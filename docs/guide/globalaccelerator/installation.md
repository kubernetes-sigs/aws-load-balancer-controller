# Installation and Prerequisites for AWS Global Accelerator Controller

This guide covers the prerequisites and installation steps required to use the AWS Global Accelerator Controller feature in the AWS Load Balancer Controller.

## Prerequisites

> **Important**: AWS Global Accelerator is only available in the commercial AWS partition. It is not available in other partitions such as the AWS GovCloud (aws-us-gov) or AWS China (aws-cn) partitions.

### Configure IAM

**Additional IAM Permissions for Global Accelerator**:

In addition to the standard AWS Load Balancer Controller permissions that you already have configured, you'll need to add specific permissions for the Global Accelerator controller feature. We recommend creating a dedicated policy named `AWSGlobalAcceleratorControllerIAMPolicy` that includes these additional permissions.

- [Complete IAM Policy for Global Accelerator Controller](../../install/aga_controller_iam_policy.md)

This additional policy includes permissions for:

- Creating and managing Global Accelerator resources (accelerators, listeners, endpoint groups, endpoints)
- Tagging resources for proper identification and management
- Creating service-linked roles required by Global Accelerator
- Reading load balancer information for endpoint discovery

You can attach this policy using the same method you've used for the AWS Load Balancer Controller permissions - either through IAM Roles for Service Accounts (IRSA) or by attaching it to your worker node IAM roles, depending on your cluster setup.

### Kubernetes Cluster Requirements

1. **Kubernetes Version**: The AWS Global Accelerator Controller requires Kubernetes version 1.19 or later.

2. **AWS Load Balancer Controller**: The Global Accelerator feature is integrated into the AWS Load Balancer Controller version 2.17.0 or later.

3. **IAM Permissions**: The IAM role used by the AWS Load Balancer Controller must include the Global Accelerator permissions listed above.

## Installation

The AWS Global Accelerator Controller is built into the AWS Load Balancer Controller and requires minimal additional configuration. Follow these steps to install and enable the feature:

-  **Follow the standard AWS Load Balancer Controller installation steps** from the [official installation guide](../../deploy/installation.md).

-  **Install the GlobalAccelerator Custom Resource Definition (CRD)**:

   ```bash
   kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/config/crd/aga/bases/aga.k8s.aws_globalaccelerators.yaml
   ```

   Verify the CRD is installed:

   ```bash
   kubectl get crd | grep globalaccelerators.aga.k8s.aws
   ```

- **Enable the required feature gates** by adding the following flags to your controller deployment:
   
   ```
   --feature-gates=GlobalAcceleratorController=true,EnableRGTAPI=true
   ```

   When using Helm, these can be enabled with the following parameters:
   
   ```
   --set controllerConfig.featureGates.GlobalAcceleratorController=true --set controllerConfig.featureGates.EnableRGTAPI=true
   ```

> **Note**: Both feature gates are required for the AWS Global Accelerator Controller to function properly:

> - `GlobalAcceleratorController`: Enables the core Global Accelerator controller functionality
> - `EnableRGTAPI`: Enables the Resource Group Tagging API integration needed for tagging
   
## Configuration Options

The AWS Global Accelerator Controller supports the following configuration options that can be set as command-line flags for the AWS Load Balancer Controller:

| Flag | Type | Default | Description                                                                      |
| --- | --- |------|----------------------------------------------------------------------------------|
| `--feature-gates=GlobalAcceleratorController` | boolean | false | Enable the Global Accelerator controller feature                                 |
| `--feature-gates=EnableRGTAPI` | boolean | false | Enable the Resource Group Tagging API integration for tagging                    |
| `--global-accelerator-max-concurrent-reconciles` | integer | 1    | Maximum number of concurrent reconciles for Global Accelerator resources         |
| `--global-accelerator-max-exponential-backoff-delay` | duration | 16m40s     | Maximum delay for exponential backoff for Global Accelerator resource reconciles |

## AWS Global Accelerator Service Quotas

For the most up-to-date quotas, refer to the [AWS Global Accelerator quotas documentation](https://docs.aws.amazon.com/global-accelerator/latest/dg/limits-global-accelerator.html).

## Next Steps

1. [AWS Global Accelerator Controller Guide](aga-controller.md)
2. [GlobalAccelerator CRD Reference](spec.md)
3. [Examples](examples.md)
