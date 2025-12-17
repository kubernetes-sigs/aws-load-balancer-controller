#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Required parameters
CLUSTER_NAME="${CLUSTER_NAME:-}"
AWS_REGION="${AWS_REGION:-us-west-2}"
VPC_ID="${VPC_ID:-}"

# Print usage
usage() {
    echo "Usage: CLUSTER_NAME=<name> VPC_ID=<vpc-id> $0"
    echo ""
    echo "Environment variables:"
    echo "  CLUSTER_NAME  - EKS cluster name (required)"
    echo "  VPC_ID        - VPC ID (required)"
    echo "  AWS_REGION    - AWS region (default: us-west-2)"
    echo ""
    echo "Example:"
    echo "  CLUSTER_NAME=awslbc-loadtest VPC_ID=vpc-123456 ./scripts/run_trust_store_test.sh"
    echo ""
    echo "Note: Requires ginkgo CLI installed. Install with:"
    echo "  go install github.com/onsi/ginkgo/v2/ginkgo@latest"
    exit 1
}

# Validate required parameters
if [ -z "$CLUSTER_NAME" ] || [ -z "$VPC_ID" ]; then
    usage
fi

# Get script directory and project root BEFORE changing directories
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Create temporary directory
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

cd $TEMP_DIR

echo -e "${GREEN}=== Creating Test Certificates ===${NC}"

# Generate CA certificate
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout ca-key.pem -out ca-cert.pem -days 365 \
  -subj "/C=US/ST=WA/L=Seattle/O=TestOrg/CN=Test-CA" 2>/dev/null

# Generate server certificate
openssl genrsa -out server-key.pem 2048 2>/dev/null
openssl req -new -key server-key.pem -out server-csr.pem \
  -subj "/C=US/ST=WA/O=TestOrg/CN=*.elb.${AWS_REGION}.amazonaws.com" 2>/dev/null

cat > server-ext.cnf <<EOF
subjectAltName = DNS:*.elb.${AWS_REGION}.amazonaws.com,DNS:*.elb.amazonaws.com
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
EOF

openssl x509 -req -in server-csr.pem \
  -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out server-cert.pem -days 365 -extfile server-ext.cnf 2>/dev/null

# Generate client certificate
openssl genrsa -out client-key.pem 2048 2>/dev/null
openssl req -new -key client-key.pem -out client-csr.pem \
  -subj "/C=US/ST=WA/O=TestOrg/CN=test-client" 2>/dev/null

cat > client-ext.cnf <<EOF
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
EOF

openssl x509 -req -in client-csr.pem \
  -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
  -out client-cert.pem -days 365 -extfile client-ext.cnf 2>/dev/null

echo -e "${GREEN}✓ Certificates created${NC}"

echo -e "${GREEN}=== Uploading Server Certificate to ACM ===${NC}"

SERVER_CERT_ARN=$(aws acm import-certificate \
  --certificate fileb://server-cert.pem \
  --private-key fileb://server-key.pem \
  --certificate-chain fileb://ca-cert.pem \
  --region $AWS_REGION \
  --query 'CertificateArn' \
  --output text)

echo -e "${GREEN}✓ Server certificate imported: $SERVER_CERT_ARN${NC}"

# Cleanup function for AWS resources
cleanup_aws() {
    echo -e "${YELLOW}=== Cleaning up AWS resources ===${NC}"
    
    if [ ! -z "$TRUST_STORE_ARN" ]; then
        aws elbv2 delete-trust-store \
          --trust-store-arn $TRUST_STORE_ARN \
          --region $AWS_REGION 2>/dev/null || true
        echo -e "${GREEN}✓ Trust store deleted${NC}"
    fi
    
    if [ ! -z "$SERVER_CERT_ARN" ]; then
        aws acm delete-certificate \
          --certificate-arn $SERVER_CERT_ARN \
          --region $AWS_REGION 2>/dev/null || true
        echo -e "${GREEN}✓ ACM certificate deleted${NC}"
    fi
    
    if [ ! -z "$BUCKET_NAME" ]; then
        aws s3 rb s3://$BUCKET_NAME --force 2>/dev/null || true
        echo -e "${GREEN}✓ S3 bucket deleted${NC}"
    fi
}

trap cleanup_aws EXIT

echo -e "${GREEN}=== Creating S3 Bucket and Trust Store ===${NC}"

BUCKET_NAME="mtls-test-$(date +%s)-$(openssl rand -hex 4)"
aws s3 mb s3://$BUCKET_NAME --region $AWS_REGION >/dev/null
aws s3 cp ca-cert.pem s3://$BUCKET_NAME/ca-bundle.pem >/dev/null

TRUST_STORE_ARN=$(aws elbv2 create-trust-store \
  --name mtls-test-$(date +%s) \
  --ca-certificates-bundle-s3-bucket $BUCKET_NAME \
  --ca-certificates-bundle-s3-key ca-bundle.pem \
  --region $AWS_REGION \
  --query 'TrustStores[0].TrustStoreArn' \
  --output text)

echo -e "${GREEN}✓ Trust store created: $TRUST_STORE_ARN${NC}"

# Wait for trust store to be active
echo -e "${YELLOW}Waiting for trust store to become active...${NC}"
for i in {1..30}; do
    STATUS=$(aws elbv2 describe-trust-stores \
      --trust-store-arns $TRUST_STORE_ARN \
      --region $AWS_REGION \
      --query 'TrustStores[0].Status' \
      --output text)
    
    if [ "$STATUS" = "ACTIVE" ]; then
        echo -e "${GREEN}✓ Trust store is active${NC}"
        break
    fi
    
    if [ $i -eq 30 ]; then
        echo -e "${RED}✗ Trust store failed to become active${NC}"
        exit 1
    fi
    
    sleep 5
done

echo ""
echo -e "${GREEN}=== Running Trust Store E2E Tests ===${NC}"
echo ""

# Navigate to project root
cd $PROJECT_ROOT

# Run the test using ginkgo from project root
echo "Running ginkgo from: $(pwd)"
echo "Test directory: test/e2e/gateway"
echo "Focus: test ALB Gateway with Trust Store for mTLS"

ginkgo -v -r test/e2e/gateway -- \
  --kubeconfig=$KUBECONFIG \
  --cluster-name=$CLUSTER_NAME \
  --aws-region=$AWS_REGION \
  --aws-vpc-id=$VPC_ID \
  --certificate-arns=$SERVER_CERT_ARN \
  --trust-store-arn=$TRUST_STORE_ARN \
  --client-cert-path=$TEMP_DIR/client-cert.pem \
  --client-key-path=$TEMP_DIR/client-key.pem \
  --enable-gateway-tests \
  -ginkgo.focus="test ALB Gateway with Trust Store for mTLS"

TEST_EXIT_CODE=$?

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}=== All Tests Passed ===${NC}"
else
    echo -e "${RED}=== Tests Failed ===${NC}"
fi

exit $TEST_EXIT_CODE
