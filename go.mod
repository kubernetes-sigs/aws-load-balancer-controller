module sigs.k8s.io/aws-load-balancer-controller

go 1.13

require (
	github.com/aws/aws-sdk-go v1.35.4
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.2.0
	github.com/google/go-cmp v0.4.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.10.0
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	gomodules.xyz/jsonpatch/v2 v2.0.1
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	sigs.k8s.io/controller-runtime v0.6.3
	sigs.k8s.io/structured-merge-diff v0.0.0-20190525122527-15d366b2352e // indirect
)
