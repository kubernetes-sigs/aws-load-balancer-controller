# Install dependencies
```
$ glide install -v
$ go build
```

# Launch controller
```
$ POD_NAMESPACE=default AWS_REGION=us-east-1 AWS_PROFILE=tm-nonprod-Ops-Techops CLUSTER_NAME=dev ./alb-ingress-controller --apiserver-host http://127.0.0.1:8001 --default-backend-service kube-system/default-http-backend
I0321 16:36:25.073628   68809 ingress.go:161] Build up list of existing ingresses
I0321 16:36:26.310847   68809 ingress.go:170] Fetching tags for arn:aws:elasticloadbalancing:us-east-1:343550350117:loadbalancer/app/dev-616e5271984f508/d7e9b14423eadeec
I0321 16:36:26.645034   68809 ingress.go:200] Fetching resource recordset for prd427-prom/alertmanager alertmanager.prd427.dev.us-east-1.nonprod-tmaws.io
I0321 16:36:26.936247   68809 ingress.go:234] Fetching Targets for Target Group arn:aws:elasticloadbalancing:us-east-1:343550350117:targetgroup/dev-31266-HTTP-ead8675/2b0def78017b534a
I0321 16:36:27.219909   68809 ingress.go:250] Fetching Rules for Listener arn:aws:elasticloadbalancing:us-east-1:343550350117:listener/app/dev-616e5271984f508/d7e9b14423eadeec/71335039efb528f5
I0321 16:36:27.283640   68809 ingress.go:170] Fetching tags for arn:aws:elasticloadbalancing:us-east-1:343550350117:loadbalancer/app/dev-816657f3e4235f2/e98c5e83437d28fd
I0321 16:36:27.356121   68809 ingress.go:200] Fetching resource recordset for prd280/papi papi.dev1.us-east-1.nonprod-tmaws.io
I0321 16:36:27.635162   68809 ingress.go:234] Fetching Targets for Target Group arn:aws:elasticloadbalancing:us-east-1:343550350117:targetgroup/dev-31665-HTTP-6e5f021/0283bbba638dc070
I0321 16:36:27.892404   68809 ingress.go:250] Fetching Rules for Listener arn:aws:elasticloadbalancing:us-east-1:343550350117:listener/app/dev-816657f3e4235f2/e98c5e83437d28fd/4315728a37cb1a3d
.
.
.
```
