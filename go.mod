module sigs.k8s.io/aws-alb-ingress-controller

go 1.13

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/go-logr/logr v0.1.0
	github.com/golang/mock v1.2.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/common v0.0.0-20181126121408-4724e9255275
	github.com/stretchr/testify v1.4.0
	go.uber.org/zap v1.9.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6 // indirect
	k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	sigs.k8s.io/controller-runtime v0.4.0
)
