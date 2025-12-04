# IAM Policy for AWS Global Accelerator Controller

This document outlines the required IAM permissions for the AWS Global Accelerator Controller feature of the AWS Load Balancer Controller.

## IAM Policy

Create an IAM policy with the following permissions to allow the controller to manage AWS Global Accelerator resources. The policy is defined in the [aga_controller_iam_policy.json](./aga_controller_iam_policy.json) file.

You can fetch the policy directly using curl:

```bash
curl -o aga_controller_iam_policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/install/aga_controller_iam_policy.json
```

Then create the IAM policy using the AWS CLI:

```bash
aws iam create-policy \
  --policy-name AWSLoadBalancerControllerAGAIAMPolicy \
  --policy-document file://aga_controller_iam_policy.json
```

## Permission Requirements Explanation

This policy provides fine-grained access to AWS Global Accelerator resources:

### Service-Linked Role Creation

Allows the controller to create service-linked roles required by Global Accelerator:
- `iam:CreateServiceLinkedRole` for `globalaccelerator.amazonaws.com`

### Read Permissions

Allows listing and describing Global Accelerator resources:
- `globalaccelerator:Describe*` and `globalaccelerator:List*` operations
- `ec2:DescribeRegions` for cross-region endpoint configuration

### Resource Creation and Management

Allows creation, updating, and deletion of Global Accelerator resources with appropriate tagging:
- Resource creation limited by required tags
- Resource modification limited to resources with appropriate tags
- Endpoint management tied to tagged resources

### Tag Management

Allows the controller to manage tags on Global Accelerator resources:
- Tagging operations constrained by tag conditions to prevent modification of resources not owned by the controller

## AWS Load Balancer Controller Integration

When configuring the AWS Load Balancer Controller to use this policy:

1. Create the IAM policy in your AWS account
2. Attach the policy to the IAM role used by the controller
3. Ensure the controller's service account is configured to use the role via IRSA (IAM Roles for Service Accounts)

For more details on setting up IAM roles for the controller, see the [main installation documentation](../deploy/installation.md).
