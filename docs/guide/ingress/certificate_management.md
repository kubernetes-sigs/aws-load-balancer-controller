# ACM Certificate Management

TLS certificates for ALB Listeners can be automatically created with hostnames from Ingress resources if enabled.

## Enabling Certificate Management

To enable the controller to automatically create TLS certificates in ACM, set the [EnableCertificateManagement](../../deploy/configurations.md#feature-gates) feature gate.

!!!note "Permisisons"
    This feature also requires additional permissions in the IAM role of the controller. You can find an appropriate policy statement to attach to the existing IAM role [here](../../install/iam_policy_acm_certs.json).

## Create TLS certificates for all ingresses

Currently the feature has to be enabled both in the controller and for every ingress object individually. There's no option to request certificates by default for all ingresses.



## Create Per-Ingress TLS certificates

Use the [create-acm-cert annotation](annotations.md#create-acm-cert) to enable automatic certificate creation for an ingress object. All hosts referenced in the ingress object are included as Subject Alternative Names in the certificate, whilst the first host found is set as the Domain Name. 

!!!info
    This annotation **disables** certificate discovery. Any existing certificates are detached from the ALB Listeners during reconciliation.

### Conflict with explicitly configured certificate ARNs

Explicitly configured Certificate ARNs such as the [certificate-arn annotation](annotations.md#certificate-arn) or the [certificateArn field](ingress_class.md#speccertificatearn) on the IngressClass take precedence over this feature. No new certificate is created if one of these settings is present.

### Disable TLS certificates

If you choose to opt-out of this feature by removing the annotation from an ingress object, the controller will fallback to the [certificate discovery mechanism](cert_discovery.md) or if configured, explicitly certificate ARNs.

This reconciliation might fail if there are no certificates present to autodiscover. The certificate discovery mechanism is configured to ignore certificates created by the controller. Thus create a matching certificate manually for the controller to reconcile the ingress object back to using a non-managed certificate. 

### Certificate Validation

Amazon Issued certificates are currently validated using DNS Method and Route53 records. Validation can take [up to 30 minutes](https://docs.aws.amazon.com/acm/latest/userguide/dns-validation.html). 
E-Mail validation is not supported due to significant higher delays between requesting a certificate and it's issuance. 
When using a PCA, certificates don't have to be validated.

## PCA-Support

If you don't want Amazon Issued certificates you can issue certificates from an existing PCA in your AWS account.

The ARN of the PCA can be configured on two different levels:

- controller flag: This implies for all certificates created that they are "private". If a specific ingress object wants to use Amazon Issued certificates again, it's only option is using the [certificate-arn annotation](annotations.md#certificate-arn)
- ingress annotation: an ingress object can use any PCA by using the [acm-pca-arn annotation](annotations.md#acm-pca-arn) to either override the default PCA or switch from to a private certificate if the controller flag is not present. Note that specifying a value of empty string won't override a default provided on the controller.

### CA Rotation

For platform operators who want to rotate PCAs without interaction from ingress users, it's suggested to prevent use of the [acm-pca-arn annotation](annotations.md#acm-pca-arn) using a policy-engine. This ensures a smooth and controlled PCA rotation, as adjusting the PCA ARN on the controller's flag will cause the controller to reissue all certificates for all enabled ingress objects, effectively rotating to a new pre-provisioned PCA.
