module sigs.k8s.io/aws-alb-ingress-controller

go 1.13

require (
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.2.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.8.1
	github.com/stretchr/testify v1.4.0
	k8s.io/api v0.18.4
	k8s.io/apimachinery v0.18.4
	k8s.io/client-go v0.18.4
	sigs.k8s.io/controller-runtime v0.6.1
	sigs.k8s.io/structured-merge-diff v0.0.0-20190525122527-15d366b2352e // indirect
)
