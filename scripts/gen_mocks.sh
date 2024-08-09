## Note: mocks for interfaces from this project should be along with the original package.
##       mocks for interfaces from 3rd-party project should be put inside ./mocks folder.

MOCKGEN=${MOCKGEN:-~/go/bin/mockgen}

$MOCKGEN -package=mock_client -destination=./mocks/controller-runtime/client/client_mocks.go sigs.k8s.io/controller-runtime/pkg/client Client
$MOCKGEN -package=services -destination=./pkg/aws/services/elbv2_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services ELBV2
$MOCKGEN -package=services -destination=./pkg/aws/services/ec2_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services EC2
$MOCKGEN -package=services -destination=./pkg/aws/services/shield_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services Shield
$MOCKGEN -package=webhook -destination=./pkg/webhook/mutator_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/webhook Mutator
$MOCKGEN -package=webhook -destination=./pkg/webhook/validator_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/webhook Validator
$MOCKGEN -package=k8s -destination=./pkg/k8s/finalizer_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/k8s FinalizerManager
$MOCKGEN -package=k8s -destination=./pkg/k8s/pod_info_repo_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/k8s PodInfoRepo
$MOCKGEN -package=networking -destination=./pkg/networking/security_group_manager_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking SecurityGroupManager
$MOCKGEN -package=networking -destination=./pkg/networking/subnet_resolver_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking SubnetsResolver
$MOCKGEN -package=networking -destination=./pkg/networking/az_info_provider_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking AZInfoProvider
$MOCKGEN -package=networking -destination=./pkg/networking/node_info_provider_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking NodeInfoProvider
$MOCKGEN -package=networking -destination=./pkg/networking/vpc_info_provider_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking VPCInfoProvider
$MOCKGEN -package=networking -destination=./pkg/networking/backend_sg_provider_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking BackendSGProvider
$MOCKGEN -package=networking -destination=./pkg/networking/security_group_resolver_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/networking SecurityGroupResolver
$MOCKGEN -package=ingress -destination=./pkg/ingress/cert_discovery_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/ingress CertDiscovery
$MOCKGEN -package=elbv2 -destination=./pkg/deploy/elbv2/tagging_manager_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/elbv2 TaggingManager
$MOCKGEN -package=shield -destination=./pkg/deploy/shield/protection_manager_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/shield ProtectionManager
$MOCKGEN -package=wafv2 -destination=./pkg/deploy/wafv2/web_acl_association_manager_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/wafv2 WebACLAssociationManager
$MOCKGEN -package=wafregional -destination=./pkg/deploy/wafregional/web_acl_association_manager_mocks.go sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/wafregional WebACLAssociationManager