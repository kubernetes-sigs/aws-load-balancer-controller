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

# Verifies that the helm chart RBAC rules are in sync with the
# kubebuilder-generated role.yaml. Fails if they have drifted.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"${SCRIPT_DIR}/sync-rbac-to-helm.sh"

changed_files=$(git status --porcelain --untracked-files=no -- helm/aws-load-balancer-controller/templates/rbac.yaml || true)
if [ -n "${changed_files}" ]; then
   echo "Detected that helm RBAC is out of sync with kubebuilder RBAC; run 'make manifests'"
   echo "changed files:"
   printf "%s\n" "${changed_files}"
   echo "git diff:"
   git --no-pager diff -- helm/aws-load-balancer-controller/templates/rbac.yaml
   echo "To fix: run 'make manifests'"
   exit 1
fi
