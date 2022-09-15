# RBAC configuration for secrets resources

In this walkthrough, you will

- configure RBAC permissions for the controller to access specific secrets resource in a particular namespace.

# Create Role
1. Prepare the role manifest with the appropriate name, namespace, and secretName, for example:

    ```
    apiVersion: rbac.authorization.k8s.io/v1
    kind: Role
    metadata:
        name: example-role
        namespace: example-namespace
    rules:
      - apiGroups:
           - ""
        resourceNames:
          - example-secret
        resources:
          - secrets
        verbs:
          - get
          - list
          - watch
    ```

2. Apply the role manifest

    ```
    kubectl apply -f role.yaml
    ```

# Create RoleBinding
1. Prepare the rolebinding manifest with the appropriate name, namespace and role reference. For example:

    ```
    apiVersion: rbac.authorization.k8s.io/v1
    kind: RoleBinding
    metadata:
        name: example-rolebinding
        namespace: example-namespace
    roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: Role
        name: example-role
    subjects:
      - kind: ServiceAccount
        name: aws-load-balancer-controller
        namespace: kube-system
    ```

2. Apply the rolebinding manifest

    ```
    kubectl apply -f rolebinding.yaml
    ```