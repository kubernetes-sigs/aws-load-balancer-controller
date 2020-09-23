mockgen -destination=./mocks/controller-runtime/client/mock_client.go sigs.k8s.io/controller-runtime/pkg/client Client
mockgen -destination=./mocks/aws/services/mock_elbv2.go sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services ELBV2
mockgen -destination=./mocks/aws/services/mock_ec2.go sigs.k8s.io/aws-load-balancer-controller/pkg/aws/services EC2
mockgen -destination=./mocks/webhook/mock_mutator.go sigs.k8s.io/aws-load-balancer-controller/pkg/webhook Mutator
mockgen -destination=./mocks/webhook/mock_validator.go sigs.k8s.io/aws-load-balancer-controller/pkg/webhook Validator
