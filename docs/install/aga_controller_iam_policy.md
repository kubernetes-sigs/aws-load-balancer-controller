# IAM Policy for AWS Global Accelerator Controller

This document outlines the required IAM permissions for the AWS Global Accelerator Controller feature of the AWS Load Balancer Controller.

## IAM Policy

Create an IAM policy with the following permissions to allow the controller to manage AWS Global Accelerator resources:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "iam:CreateServiceLinkedRole"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "iam:AWSServiceName": [
                        "globalaccelerator.amazonaws.com"
                    ]
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:DescribeAccelerator",
                "globalaccelerator:DescribeEndpointGroup",
                "globalaccelerator:DescribeListener",
                "globalaccelerator:ListAccelerators",
                "globalaccelerator:ListEndpointGroups",
                "globalaccelerator:ListListeners",
                "globalaccelerator:ListTagsForResource",
                "ec2:DescribeRegions"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:CreateAccelerator"
            ],
            "Resource": "*",
            "Condition": {
                "Null": {
                    "aws:RequestTag/elbv2.k8s.aws/cluster": "false"
                },
                "StringEquals": {
                    "aws:RequestTag/aga.k8s.aws/resource": "GlobalAccelerator"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:UpdateAccelerator",
                "globalaccelerator:DeleteAccelerator",
                "globalaccelerator:CreateListener",
                "globalaccelerator:UpdateListener",
                "globalaccelerator:DeleteListener",
                "globalaccelerator:CreateEndpointGroup",
                "globalaccelerator:UpdateEndpointGroup",
                "globalaccelerator:DeleteEndpointGroup",
                "globalaccelerator:AddEndpoints",
                "globalaccelerator:RemoveEndpoints"
            ],
            "Resource": [
                "arn:aws:globalaccelerator::*:accelerator/*",
                "arn:aws:globalaccelerator::*:accelerator/*/listener/*",
                "arn:aws:globalaccelerator::*:accelerator/*/listener/*/endpoint-group/*"
            ],
            "Condition": {
                "Null": {
                    "aws:ResourceTag/elbv2.k8s.aws/cluster": "false"
                },
                "StringEquals": {
                    "aws:ResourceTag/aga.k8s.aws/resource": "GlobalAccelerator"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:TagResource",
                "globalaccelerator:UntagResource"
            ],
            "Resource": "arn:aws:globalaccelerator::*:accelerator/*",
            "Condition": {
                "Null": {
                    "aws:RequestTag/elbv2.k8s.aws/cluster": "true",
                    "aws:ResourceTag/elbv2.k8s.aws/cluster": "false"
                },
                "StringEquals": {
                    "aws:ResourceTag/aga.k8s.aws/resource": "GlobalAccelerator"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "globalaccelerator:TagResource"
            ],
            "Resource": "arn:aws:globalaccelerator::*:accelerator/*",
            "Condition": {
                "Null": {
                    "aws:RequestTag/elbv2.k8s.aws/cluster": "false"
                },
                "StringEquals": {
                    "aws:RequestTag/aga.k8s.aws/resource": "GlobalAccelerator"
                }
            }
        }
    ]
}
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
