#!/bin/bash
set -euo pipefail
set +x

SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"
BUILD_DIR="${SCRIPTPATH}/../build"

REPO="kubernetes-sigs/aws-load-balancer-controller"
HELM_CHART_NAME="aws-load-balancer-controller"
HELM_CHART_BASE_BIR="${SCRIPTPATH}/../helm"

CHARTS_REPO="aws/eks-charts"
CHARTS_REPO_NAME=$(echo ${CHARTS_REPO} | cut -d'/' -f2)

HELM_CHART_DIR="${HELM_CHART_BASE_BIR}/${HELM_CHART_NAME}"
PR_ID=$(uuidgen | cut -d '-' -f1)

SYNC_DIR="${BUILD_DIR}/eks-charts-sync"
FORK_DIR="${SYNC_DIR}/${CHARTS_REPO_NAME}"

BINARY_BASE=""
INCLUDE_NOTES=0

GH_CLI_VERSION="0.10.1"
GH_CLI_CONFIG_PATH="${HOME}/.config/gh/config.yml"
KERNEL=$(uname -s | tr '[:upper:]' '[:lower:]')
OS="${KERNEL}"
if [[ "${KERNEL}" == "darwin" ]]; then
  OS="macOS"
fi

VERSION=$(echo $(grep -m 1 IMG "${SCRIPTPATH}/../Makefile") | cut -d':' -f2)

USAGE=$(cat << EOM
  Usage: sync-to-eks-charts  -r <repo>
  Syncs Helm chart to aws/eks-charts

  Example: sync-to-eks-charts -r "${REPO}"
          Required:
            -b          Binary basename (i.e. -b "${HELM_CHART_NAME}")

          Optional:
            -r          Github repo to sync to in the form of "org/name"  (i.e. -r "${REPO}")
            -n          Include application release notes in the sync PR
EOM
)

# Process our input arguments
while getopts "b:r:n" opt; do
  case ${opt} in
    r ) # Github repo
        REPO="$OPTARG"
      ;;
    b ) # binary basename
        BINARY_BASE="$OPTARG"
      ;;
    n ) # Include release notes
        INCLUDE_NOTES=1
      ;;
    \? )
        echo "$USAGE" 1>&2
        exit
      ;;
  esac
done


if [[ -z "${REPO}" ]]; then
  echo "Repo (-r) must be specified if no \"make repo-full-name\" target exists"
fi

echo $REPO

if [[ -z $(command -v gh) ]] || [[ ! $(gh --version) =~ $GH_CLI_VERSION ]]; then
  mkdir -p "${BUILD_DIR}"/gh
  curl -Lo "${BUILD_DIR}"/gh/gh.tar.gz "https://github.com/cli/cli/releases/download/v${GH_CLI_VERSION}/gh_${GH_CLI_VERSION}_${OS}_amd64.tar.gz"
  tar -C "${BUILD_DIR}"/gh -xvf "${BUILD_DIR}/gh/gh.tar.gz"
  export PATH="${BUILD_DIR}/gh/gh_${GH_CLI_VERSION}_${OS}_amd64/bin:$PATH"
  if [[ ! $(gh --version) =~ $GH_CLI_VERSION ]]; then
    echo "❌ Failed install of github cli"
    exit 4
  fi
fi

function restore_gh_config() {
  mv -f "${GH_CLI_CONFIG_PATH}.bkup" "${GH_CLI_CONFIG_PATH}" || :
}

if [[ -n $(env | grep GITHUB_TOKEN) ]] && [[ -n "${GITHUB_TOKEN}" ]]; then
  trap restore_gh_config EXIT INT TERM ERR
  mkdir -p "${HOME}/.config/gh"
  cp -f "${GH_CLI_CONFIG_PATH}" "${GH_CLI_CONFIG_PATH}.bkup" || :
  cat << EOF > "${GH_CLI_CONFIG_PATH}"
hosts:
    github.com:
        oauth_token: ${GITHUB_TOKEN}
        user: ${GITHUB_USERNAME}
EOF
fi

function fail() {
  echo "❌ EKS charts sync failed"
  exit 5
}

trap fail ERR TERM INT

rm -rf "${SYNC_DIR}"
mkdir -p "${SYNC_DIR}"

cd "${SYNC_DIR}"
gh repo fork $CHARTS_REPO --clone --remote
cd "${FORK_DIR}"
git remote set-url origin https://"${GITHUB_USERNAME}":"${GITHUB_TOKEN}"@github.com/"${GITHUB_USERNAME}"/"${CHARTS_REPO_NAME}".git
DEFAULT_BRANCH=$(git rev-parse --abbrev-ref HEAD | tr -d '\n')


if diff -x ".*" -r "$HELM_CHART_DIR/" "${FORK_DIR}/stable/${HELM_CHART_NAME}/" &> /dev/null ; then
  echo " ✅  Charts already in sync; no updates needed"
  exit
else
  echo "📊 Charts are NOT in sync proceeding with PR"
fi

git config user.name "eks-bot"
git config user.email "eks-bot@users.noreply.github.com"

# Sync the fork
git pull upstream "${DEFAULT_BRANCH}"
git push -u origin "${DEFAULT_BRANCH}"

FORK_RELEASE_BRANCH="${BINARY_BASE}-${VERSION}-${PR_ID}"
git checkout -b "${FORK_RELEASE_BRANCH}" upstream/"${DEFAULT_BRANCH}"

rm -rf "${FORK_DIR}"/stable/${HELM_CHART_NAME}/
cp -r "$HELM_CHART_DIR/" "${FORK_DIR}/stable/${HELM_CHART_NAME}/"

git add --all
git commit -m "${BINARY_BASE}: ${VERSION}"

PR_BODY=$(cat << EOM
## ${BINARY_BASE} ${VERSION} Automated Chart Sync! 🤖🤖
EOM
)

if [[ "${INCLUDE_NOTES}" -eq 1 ]]; then
  RELEASE_ID=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
    https://api.github.com/repos/"${REPO}"/releases | \
    jq --arg VERSION "$VERSION" '.[] | select(.tag_name==$VERSION) | .id')

  RELEASE_NOTES=$(curl -s -H "Authorization: token ${GITHUB_TOKEN}" \
    https://api.github.com/repos/"${REPO}"/releases/"${RELEASE_ID}" | \
    jq -r '.body')

  PR_BODY=$(cat << EOM
  ## ${BINARY_BASE} ${VERSION} Automated Chart Sync! 🤖🤖

  ### Release Notes 📝:

  ${RELEASE_NOTES}
EOM
)
fi

  git push -u origin "${FORK_RELEASE_BRANCH}"
  gh pr create --title "🥳 ${BINARY_BASE} ${VERSION} Automated Release! 🥑" \
    --body "${PR_BODY}" --repo ${CHARTS_REPO}

echo "✅ EKS charts sync complete"
