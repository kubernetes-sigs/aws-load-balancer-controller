module sigs.k8s.io/aws-load-balancer-controller

go 1.16

require (
	github.com/aws/aws-sdk-go v1.41.0
	github.com/gavv/httpexpect/v2 v2.3.1
	github.com/go-logr/logr v0.4.0
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.6
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.17.0
	golang.org/x/time v0.0.0-20210611083556-38a9dc6acbc6
	gomodules.xyz/jsonpatch/v2 v2.2.0
	helm.sh/helm/v3 v3.6.1
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/cli-runtime v0.21.2
	k8s.io/client-go v0.21.2
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/controller-runtime v0.9.2
)

replace golang.org/x/sys => golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40
