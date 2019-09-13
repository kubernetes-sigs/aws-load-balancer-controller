module github.com/kubernetes-sigs/aws-alb-ingress-controller

require (
	github.com/aws/aws-k8s-tester/e2e/tester v0.0.0-20190907061006-260b0e114d90
	github.com/aws/aws-sdk-go v1.16.35
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/glogr v0.1.0
	github.com/gogo/protobuf v1.2.0 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/mock v1.2.0
	github.com/golangci/golangci-lint v1.18.0 // indirect
	github.com/hashicorp/golang-lru v0.5.0 // indirect
	github.com/imdario/mergo v0.3.7
	github.com/magiconair/properties v1.8.0
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/onsi/ginkgo v1.7.0
	github.com/onsi/gomega v1.4.3
	github.com/pborman/uuid v1.2.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model v0.0.0-20180712105110-5c3871d89910
	github.com/prometheus/common v0.0.0-20181126121408-4724e9255275
	github.com/spf13/pflag v1.0.3
	github.com/stretchr/objx v0.1.1 // indirect
	github.com/stretchr/testify v1.3.0
	golang.org/x/oauth2 v0.0.0-20190212230446-3e8b2be13635 // indirect
	golang.org/x/time v0.0.0-20181108054448-85acf8d2951c // indirect
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/apiserver v0.0.0-20190214201149-f9f16382a346
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v0.3.0
	k8s.io/kube-openapi v0.0.0-20190208205540-d7c86cdc46e3 // indirect
	sigs.k8s.io/controller-runtime v0.2.0
)
