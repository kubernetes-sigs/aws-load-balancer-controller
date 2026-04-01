# ListenerRuleConfiguration

ListenerRuleConfigurations may be attached to Routes within the same namespace of the LRC.

## Actions

### ForwardActionConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "forward"
      forwardConfig:
        targetGroupStickinessConfig:
          durationSeconds: 120
          enabled: true
```

Configure the stickiness setting TargetGroups referenced in the Listener Rule.

For more information, please see the [AWS documentation](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/rule-action-types.html#forward-actions) for stickiness

**Default** No stickiness

### RedirectActionConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "redirect"
      redirectConfig:
        query: "foo"
```

Use this configuration in conjunction with the Re-direct configuration in HTTPRouteFilter to add query param information to the redirect.

**Default** ""

### FixedResponseConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "fixed-response"
      fixedResponseConfig:
        statusCode: 404
        contentType: "text/plain"
        messageBody: "my fixed response"
```

Configures the ALB to send a [fixed response](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/rule-action-types.html#fixed-response-actions).

**Default** No fixed response injected.

### AuthenticateCognitoActionConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "authenticate-cognito"
      authenticateCognitoConfig:
        userPoolArn: "user-pool-arn"
        userPoolClientId: "cid"
        userPoolDomain: "example.com"
        onUnauthenticatedRequest: "authenticate/deny/allow"
```

Configures the ALB to authenticate users with [Cognito](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html#cognito-requirements) before forwarding the request to the backend.

**Default** No Cognito pre-routing check.

### AuthenticateOidcActionConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "authenticate-oidc"
      authenticateOIDCConfig:
        authorizationEndpoint: "https://my-auth-server.com"
        secret:
          name: "my-secret-name"
        issuer: "https://my-issuer.com"
        tokenEndpoint: "https://my-token-endpoint.com"
        userInfoEndpoint: "https://my-user-info-endpoint.com"
        onUnauthenticatedRequest: "authenticate/deny/allow"
```

**Important** When specifying the secret, the secret name must exist within the namespace of the ListenerRuleConfiguration.

Configures the ALB to authenticate users with an [OIDC Provider](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html#oidc-requirements) before forwarding the request to the backend.

**Default** No OIDC pre-routing check.

### JwtValidationActionConfig

```yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: example-lrc-config
  namespace: example-ns
spec:
  actions:
    - type: "jwt-validation"
      jwtValidationConfig:
        jwksEndpoint: "https://example.com/.well-known/jwks.json"
        issuer: "https://example.com"
        additionalClaims:
          - name: "admin"
            format: "single-string"
            values: ["true"]
          - name: "roles"
            format: "string-array"
            values: ["admin", "editor", "viewer"]
          - name: "scope"
            format: "space-separated-values"
            values: ["read:api", "write:api"]
```

Configures the ALB to validate JSON Web Tokens (JWTs) before forwarding requests to the backend. The `exp` and `iss` claims are always validated by default. If present, the `nbf` and `iat` claims are also automatically validated.

**Important** JWT validation is only supported for HTTPS listeners.

**Default** No JWT validation.

## Conditions

### ListenerRuleCondition

```yaml
# source-ip-condition.yaml
apiVersion: gateway.k8s.aws/v1beta1
kind: ListenerRuleConfiguration
metadata:
  name: custom-rule-config-source-ip
  namespace: example-ns
spec:
  conditions:
    - field: source-ip
      sourceIPConfig:
        values:
          - 10.0.0.0/5
```

Adds Source IP conditions into the routing rules. For granular control of which rules to apply the LRC to, use the matchIndex field.

