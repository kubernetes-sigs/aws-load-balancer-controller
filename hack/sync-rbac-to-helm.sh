#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script syncs RBAC rules from the kubebuilder-generated role.yaml
# into the helm chart's rbac.yaml template, keeping helm-specific sections
# (leader election, bindings, conditionals) intact.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

KUBEBUILDER_ROLE="${ROOT_DIR}/config/rbac/role.yaml"
HELM_RBAC="${ROOT_DIR}/helm/aws-load-balancer-controller/templates/rbac.yaml"

if [ ! -f "${KUBEBUILDER_ROLE}" ]; then
    echo "Error: kubebuilder role not found at ${KUBEBUILDER_ROLE}"
    echo "Run 'make manifests' first."
    exit 1
fi

if [ ! -f "${HELM_RBAC}" ]; then
    echo "Error: helm rbac template not found at ${HELM_RBAC}"
    exit 1
fi

# Extract rules from kubebuilder role.yaml and convert to helm flow style
# We use python3 to parse the kubebuilder YAML (simple structure, no external deps)
generate_helm_clusterrole_rules() {
    python3 -c '
import sys

def parse_role_yaml(filepath):
    """Parse kubebuilder role.yaml without external YAML library."""
    with open(filepath) as f:
        lines = f.readlines()

    rules = []
    current_rule = None
    current_key = None
    in_rules = False

    for line in lines:
        raw = line.rstrip("\n")
        stripped = raw.strip()

        if not stripped or stripped.startswith("#") or stripped.startswith("---"):
            continue

        # Detect top-level keys
        if not raw.startswith(" ") and not raw.startswith("-"):
            if stripped == "rules:":
                in_rules = True
            else:
                in_rules = False
            continue

        if not in_rules:
            continue

        # New rule: line starts with "- " at rule level (typically 0 or minimal indent)
        if raw.startswith("- "):
            if current_rule:
                rules.append(current_rule)
            current_rule = {}
            # Handle "- apiGroups:" on the same line
            key_part = stripped[2:]  # remove "- "
            if key_part.endswith(":"):
                current_key = key_part[:-1]
                current_rule[current_key] = []
            continue

        # Key at rule level (indented, no dash)
        if not stripped.startswith("-") and stripped.endswith(":"):
            current_key = stripped[:-1].strip()
            if current_rule is not None and current_key not in current_rule:
                current_rule[current_key] = []
            continue

        # List item
        if stripped.startswith("- "):
            value = stripped[2:].strip().strip("\"").strip("'\''")
            if current_rule is not None and current_key:
                current_rule[current_key].append(value)
            continue

    if current_rule:
        rules.append(current_rule)

    return rules

def format_list(items):
    return "[" + ", ".join(str(i) for i in items) + "]"

rules = parse_role_yaml(sys.argv[1])
for rule in rules:
    api_groups = rule.get("apiGroups", [])
    resources = rule.get("resources", [])
    verbs = rule.get("verbs", [])
    resource_names = rule.get("resourceNames", None)

    formatted_groups = []
    for g in api_groups:
        if g == "":
            formatted_groups.append("\"\"")
        else:
            formatted_groups.append("\"" + g + "\"")

    print("- apiGroups: [{}]".format(", ".join(formatted_groups)))
    print("  resources: {}".format(format_list(resources)))
    if resource_names:
        print("  resourceNames: {}".format(format_list(resource_names)))
    print("  verbs: {}".format(format_list(verbs)))
' "${KUBEBUILDER_ROLE}"
}

# Build the new helm rbac.yaml
# We preserve the leader election Role, RoleBinding, and ClusterRoleBinding
# from the existing template, and only replace the ClusterRole rules.

GENERATED_RULES=$(generate_helm_clusterrole_rules)

cat > "${HELM_RBAC}" << 'HELM_HEADER'
{{- if .Values.rbac.create }}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ template "aws-load-balancer-controller.fullname" . }}-leader-election-role
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "aws-load-balancer-controller.labels" . | nindent 4 }}
rules:
- apiGroups: [""]
  resources: [configmaps]
  verbs: [create]
- apiGroups: [""]
  resources: [configmaps]
  resourceNames: [aws-load-balancer-controller-leader]
  verbs: [get, patch, update]
- apiGroups:
  - "coordination.k8s.io"
  resources:
  - leases
  verbs:
  - create
- apiGroups:
  - "coordination.k8s.io"
  resources:
  - leases
  resourceNames:
  - aws-load-balancer-controller-leader
  verbs:
  - get
  - update
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ template "aws-load-balancer-controller.fullname" . }}-leader-election-rolebinding
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "aws-load-balancer-controller.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ template "aws-load-balancer-controller.fullname" . }}-leader-election-role
subjects:
- kind: ServiceAccount
  name: {{ template "aws-load-balancer-controller.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ template "aws-load-balancer-controller.fullname" . }}-role
  labels:
    {{- include "aws-load-balancer-controller.labels" . | nindent 4 }}
rules:
HELM_HEADER

# Append the generated rules (from kubebuilder source of truth)
echo "# AUTO-GENERATED from config/rbac/role.yaml by hack/sync-rbac-to-helm.sh" >> "${HELM_RBAC}"
echo "# Do not edit these rules manually. Run 'make manifests' to update." >> "${HELM_RBAC}"
echo "${GENERATED_RULES}" >> "${HELM_RBAC}"

# Append helm-specific conditional rules and the ClusterRoleBinding
cat >> "${HELM_RBAC}" << 'HELM_FOOTER'
{{- if .Values.clusterSecretsPermissions.allowAllSecrets }}
- apiGroups: [""]
  resources: [secrets]
  verbs: [get, list, watch]
{{- end }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ template "aws-load-balancer-controller.fullname" . }}-rolebinding
  labels:
    {{- include "aws-load-balancer-controller.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ template "aws-load-balancer-controller.fullname" . }}-role
subjects:
- kind: ServiceAccount
  name: {{ template "aws-load-balancer-controller.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
{{- end }}
HELM_FOOTER

echo "Synced RBAC rules from ${KUBEBUILDER_ROLE} to ${HELM_RBAC}"
