# Setup Cognito/ALB Ingress Controller

This document describes how to install ALB Ingress Controller with AWS Cognito integration to minimal capacity, other options and or configurations may be required for production, and on an app to app basis.  

## Assumptions

The following assumptions are observed regarding this procedure.

* ExternalDNS is installed to the cluster and will provide a custom URL for your ALB. To setup ExternalDNS refer to the [install instructions](../external-dns/setup.md).

## Cognitio Configuration

Configure Cognito for use with ALB Ingress Controller using the following links with specified caveats.

* [Create Cognito user pool](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pool-as-user-directory.html)
* [Configure application integration](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-user-pools-configuring-app-integration.html)
    * On step 11.c for the `Callback URL` enter `https://<your-domain>/oauth2/idpresponse`.
    * On step 11.d for `Allowed OAuth Flows` select `authorization code grant` and for `Allowed OAuth Scopes` select `openid`.

## ALB Ingress Controller Setup

Install the ALB Ingress Controller using the [install instructions](../controller/setup.md) with the following caveats.

* When setting up IAM Role Permissions, add the `cognito-idp:DescribeUserPoolClient` permission to the example policy.

## Deploying an Ingress

Using the [cognito-ingress-template](../../examples/cognito-ingress-template.yaml) you can fill in the `<required>` variables to create an ALB ingress connected to your Cognito user pool for authentication.
