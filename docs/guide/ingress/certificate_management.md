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

## Ingress Group Behavior

When using certificate management with [IngressGroups](ingress_class.md#specgroup), each ingress in the group gets its own certificate based on its own hostnames. All certificates are attached to the shared ALB's HTTPS listener.

For example, if an ingress group has three members:

- Ingress A with `create-acm-cert: "true"` and host `app-a.example.com`
- Ingress B with `create-acm-cert: "true"` and host `app-b.example.com`
- Ingress C with `certificate-arn` pointing to an existing cert

The controller creates two managed certificates (for A and B) and uses the manually specified cert for C. All three certificates are attached to the same HTTPS:443 listener on the shared ALB.

Each ingress in the group can independently choose its certificate strategy:

- `create-acm-cert: "true"` — controller creates and manages a certificate
- `certificate-arn` — use a manually specified certificate (takes precedence over `create-acm-cert`)
- Neither — use certificate discovery (existing behavior)

Deleting one ingress from the group removes only its managed certificate. The other ingresses and their certificates remain unaffected.

!!!note "Same hostname across multiple ingresses"
    Each ingress in a group gets its own certificate, even if multiple ingresses share the same hostname. If two ingresses both have `create-acm-cert: "true"` with the same host, two separate certificates are created and both are attached to the listener. To avoid duplicate certificates for the same domain, only enable `create-acm-cert` on one ingress per hostname — the other ingresses with the same hostname can omit the annotation and will still have their listener rules created on the shared ALB.

## PCA-Support

If you don't want Amazon Issued certificates you can issue certificates from an existing PCA in your AWS account.

The ARN of the PCA can be configured on two different levels:

- controller flag: This implies for all certificates created that they are "private". If a specific ingress object wants to use Amazon Issued certificates again, it's only option is using the [certificate-arn annotation](annotations.md#certificate-arn)
- ingress annotation: an ingress object can use any PCA by using the [acm-pca-arn annotation](annotations.md#acm-pca-arn) to either override the default PCA or switch from to a private certificate if the controller flag is not present. Note that specifying a value of empty string won't override a default provided on the controller.

### CA Rotation

For platform operators who want to rotate PCAs without interaction from ingress users, it's suggested to prevent use of the [acm-pca-arn annotation](annotations.md#acm-pca-arn) using a policy-engine. This ensures a smooth and controlled PCA rotation, as adjusting the PCA ARN on the controller's flag will cause the controller to reissue all certificates for all enabled ingress objects, effectively rotating to a new pre-provisioned PCA.

## Limitations

By using this feature the number of hostnames (Subject Alternative Names) you can specify in `spec.tls[].hosts` on a single ingress resource is limited by your AWS account's ACM SAN quota. The [default limit](https://docs.aws.amazon.com/acm/latest/userguide/acm-limits.html#general-limits) is 10 SANs per certificate. If you need more, request a quota increase via the AWS Service Quotas console.

## Security Considerations

The controller operates with a single IAM role at the cluster level and does not enforce namespace-level domain restrictions. Any namespace with Ingress creation permissions can request certificates for any domain. This is consistent with how `certificate-arn` annotations and certificate discovery work — they also operate without namespace-level domain isolation.

ACM DNS validation provides the primary security boundary: a certificate is only issued if the controller can create validation records in a Route53 hosted zone, which requires the appropriate IAM permissions configured on the controller's role.

For multi-tenant clusters, consider the following mitigations:

- **Kubernetes RBAC**: Restrict which namespaces or users can create Ingress resources
- **Route53 IAM scoping**: Scope the controller's Route53 permissions to specific hosted zone ARNs (e.g., `arn:aws:route53:::hostedzone/ZXXXXX`) rather than `Resource: "*"` to limit which domains can be validated

### IAM Policy Scoping

The sample IAM policy provided with this feature uses `Resource: "*"` for Route53 and PCA operations for simplicity. In production, you should scope these permissions to reduce the blast radius:

**Route53** — Restrict `route53:ChangeResourceRecordSets` to specific hosted zones:

```json
{
    "Effect": "Allow",
    "Action": [
        "route53:ChangeResourceRecordSets",
        "route53:ListResourceRecordSets"
    ],
    "Resource": "arn:aws:route53:::hostedzone/<YOUR_ZONE_ID>"
}
```

This ensures the controller can only create DNS validation records in the hosted zones you explicitly allow, preventing modification of unrelated DNS records in the account.

**PCA** — If using private certificates, restrict `acm-pca:IssueCertificate` to specific certificate authorities:

```json
{
    "Effect": "Allow",
    "Action": ["acm-pca:IssueCertificate"],
    "Resource": "arn:aws:acm-pca:<region>:<account>:certificate-authority/<YOUR_CA_ID>"
}
```

This prevents the controller from issuing certificates from any PCA in the account other than the one you designate.

!!!warning "Default policy is permissive"
    The default IAM policy uses `Resource: "*"` for Route53 and PCA operations. This means the controller can modify DNS records in **any** hosted zone and issue certificates from **any** PCA in the account. Always scope these permissions in production environments.
