# Cert Manager Integration

The AWS Load Balancer Controller uses admission webhooks to validate and mutate resources. These webhooks require TLS certificates to operate securely. You can use cert-manager to automatically provision and manage these certificates.

## Upgrade Notes

When upgrading from a previous version, the following scenarios are handled automatically:

- If you're using cert-manager with a custom issuer:
   - Set `certManager.issuerRef` to keep using your issuer
   - The new CA hierarchy will not be created
   - Your existing certificate configuration is preserved
- If you're using cert-manager without a custom issuer:
   - A new CA hierarchy will be created
   - New certificates will be issued using this CA
   - The transition is handled automatically by cert-manager

## How it Works

When using cert-manager integration, the controller creates a certificate hierarchy that consists of:

1. A self-signed issuer used only to create the root CA certificate
2. A root CA certificate with a 5-year validity period
3. A CA issuer that uses the root certificate to sign webhook serving certificates
4. Webhook serving certificates with 1-year validity that are automatically renewed

This setup prevents race conditions during certificate renewal by:
- Using a long-lived (5 years) root CA certificate that remains stable
- Only renewing the serving certificates while keeping the CA constant
- Letting cert-manager's CA injector handle caBundle updates in webhook configurations

## Configuration

To enable cert-manager integration, set `enableCertManager: true` in your Helm values.

You can customize the certificate configuration through these values:

```yaml
enableCertManager: true

certManager:
  # Webhook serving certificate configuration
  duration: "8760h0m0s"    # 1 year (default)
  renewBefore: "720h0m0s"  # 30 days (optional)
  revisionHistoryLimit: 10 # Optional

  # Root CA certificate configuration
  rootCert:
    duration: "43800h0m0s" # 5 years (default)

  # Optional: Use your own issuer instead of the auto-generated one
  # issuerRef:
  #   name: my-issuer
  #   kind: ClusterIssuer
```

### Using Custom Issuers

If you want to use your own cert-manager issuer instead of the auto-generated CA, you can configure it through `certManager.issuerRef`:

```yaml
certManager:
  issuerRef:
    name: my-issuer
    kind: ClusterIssuer # or Issuer
```

When a custom issuer is specified:
- The controller will not create its own CA certificate chain
- The specified issuer will be used directly to issue webhook serving certificates
- You are responsible for ensuring the issuer is properly configured and available

### Certificate Renewal

1. Root CA Certificate:
   - Valid for 5 years by default
   - Used only for signing webhook certificates
   - Not renewed automatically to maintain stability

2. Webhook Serving Certificates:
   - Valid for 1 year by default
   - Renewed automatically 30 days before expiry
   - Updates handled seamlessly by cert-manager

### Best Practices

1. Use the default certificate hierarchy unless you have specific requirements
2. If using a custom issuer, ensure it's highly available and properly configured
3. Monitor certificate resources for renewal status and potential issues
4. Keep cert-manager up to date to benefit from the latest improvements