# Walkthough: echoservice

In this example, you'll

- Deploy a default-backend service (required for alb-ingress-controller)
- Deploy an alb-ingress-controller
- Create deployments and ingress resources in the cluster
- Use [external-dns](https://github.com/kubernetes-incubator/external-dns) to create a DNS record
  - This assumes you have a route53 hosted zone available. Otherwise you can skip this, but you'll only be able to address the service from the ALB's DNS.

# Deploy the alb-ingress-controller

1. Deploy the default-backend service

       ```
       kubectl apply -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/default-backend.yaml
       ```

1. Download the example alb-ingress-manifest locally.

	```
	wget https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/alb-ingress-controller.yaml
	```

1. Edit the manifest and set the following attributes

    - `AWS_REGION`: region in AWS this cluster exists.

		```yaml
		- name: AWS_REGION
		  value: us-west-1
		```

	 - `CLUSTER_NAME`: name of the cluster. 

		```yaml
		- name: CLUSTER_NAME
		  value: devCluster
		```

	  - `AWS_ACCESS_KEY_ID`: access key id that alb controller can use to communicate with AWS. This is only used for convenience of this example. It will keep the credentials in plain text within this manifest. It's recommended a project such as kube2iam is used to resolve access. **You will need to uncomment this from the manifest**.

		```yaml
		- name: AWS_ACCESS_KEY_ID
		  value: KEYVALUE
		```

	  - `AWS_SECRET_ACCESS_KEY`: secret access key that alb controller can use to communicate with AWS. This is only used for convenience of this example. It will keep the credentials in plain text within this manifest. It's recommended a project such as kube2iam is used to resolve access. **You will need to uncomment this from the manifest**.

		```yaml
		- name: AWS_SECRET_ACCESS_KEY
		  value: SECRETVALUE
		```

1. Deploy the modified alb-ingress-controller.

	```bash
	$ kubectl apply -f alb-ingress-controller.yaml
	```

	> The manifest above will deploy the controller to the `kube-system` namespace. If you deploy it outside of `kube-system` and are using RBAC, you may need to adjust RBAC roles and bindings.

1. Verify the deployment was successful and the controller started.

	```bash
	$ kubectl logs -n kube-system \
	    $(kubectl get po -n kube-system | \
	    egrep -o alb-ingress[a-zA-Z0-9-]+) | \
	    egrep -o '\[ALB-INGRESS.*$'
	```

	Should display output similar to the following.

	```
	[ALB-INGRESS] [controller] [INFO]: Log level read as "", defaulting to INFO. To change, set LOG_LEVEL environment variable to WARN, ERROR, or DEBUG.
	[ALB-INGRESS] [controller] [INFO]: Ingress class set to alb
	[ALB-INGRESS] [ingresses] [INFO]: Build up list of existing ingresses
	[ALB-INGRESS] [ingresses] [INFO]: Assembled 0 ingresses from existing AWS resources
	```

1. Create all the echoserver resources (namespace, service, deployment)

	```
	$ kubectl apply -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/echoservice/echoserver-namespace.yaml &&\
		kubectl apply -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/echoservice/echoserver-service.yaml &&\
		kubectl apply -f https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/echoservice/echoserver-deployment.yaml &&\
	```

1. List all the resources to ensure they were created.

	```
	$ kubectl get -n echoserver deploy,svc
	```

	Should resolve similar to the following.

	```
	NAME             CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
    svc/echoserver   10.3.31.76   <nodes>       80:31027/TCP   4d

    NAME                DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
    deploy/echoserver   1         1         1            1           4d
	```

1. Download the echoserver ingress manifest locally.

	```
	wget https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/echoservice/echoserver-ingress.yaml
	```

1. Alter the host field to a domain that you own in Route 53

	Assuming you managed `example.com` in Route 53.

	```yaml
	spec:
	  rules:
	  - host: echoserver.example.com
        http:
          paths:
	```

1. Add tags to subnets where ALBs should be deployed.

	In order for the alb-ingress-controller to know where to deploy its ALBs, you must include the following tags on desired subnets.

	- `kubernetes.io/cluster/$CLUSTER_NAME` where `$CLUSTER_NAME` is the same `CLUSTER_NAME` specified in the above step. The value of this tag must be `shared`
	- `kubernetes.io/role/alb-ingress` the value of this tag should be empty.

	In order for the ALB to be able to reach the workers, you'll want to ensure you have these tags present subnets for each azs you expect your workers to exist in.

	An example of a subnet with the correct tags for the cluster `joshcalico` is as follows.

	<img src="imgs/subnet-tags.png" width="600">

1. Deploy the ingress resource for echoserver

	```bash
	$ kubectl apply -f echoserver-ingress.yaml
	```

1. Verify the alb-ingress-controller creates the resources

	```bash
	$ kubectl logs -n kube-system \
        $(kubectl get po -n kube-system | \
        egrep -o alb-ingress[a-zA-Z0-9-]+) | \
        egrep -o '\[ALB-INGRESS.*$' | \
        grep 'echoserver\/echoserver'
	```

	You should see simlar to the following.

	```
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Start ELBV2 (ALB) creation.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Completed ELBV2 (ALB) creation. Name: joshcalico-echoserver-echo-2ad7 | ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:loadbalancer/app/joshcalico-echoserver-echo-2ad7/4579643c6f757be9
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Start TargetGroup creation.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Succeeded TargetGroup creation. ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:targetgroup/joshcalico-31027-HTTP-6657576/77ef58891a00263e | Name: joshcalico-31027-HTTP-6657576.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Start Listener creation.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Completed Listener creation. ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:listener/app/joshcalico-echoserver-echo-2ad7/4579643c6f757be9/2b2987fa3739c062 | Port: 80 | Proto: HTTP.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Start Rule creation.
	[ALB-INGRESS] [echoserver/echoserver] [INFO]: Completed Rule creation. Rule Priority: "1" | Condition: [{    Field: "host-header",    Values: ["echoserver.joshrosso.com"]  },{    Field: "path-pattern",    Values: ["/"]  }]
	```

1. Check the events of the ingress to see what has occured.

	```bash
	$ kubectl describe ing -n echoserver echoserver
	```

	You should see simlar to the following.

	```
	Name:                   echoserver
	Namespace:              echoserver
	Address:                joshcalico-echoserver-echo-2ad7-1490890749.us-east-2.elb.amazonaws.com
	Default backend:        default-http-backend:80 (10.2.1.28:8080)
	Rules:
	  Host                          Path    Backends
	  ----                          ----    --------
	  echoserver.joshrosso.com
									/       echoserver:80 (<none>)
	Annotations:
	Events:
	  FirstSeen     LastSeen        Count   From                    SubObjectPath   Type            Reason  Message
	  ---------     --------        -----   ----                    -------------   --------        ------  -------
	  3m            3m              1       ingress-controller                      Normal          CREATE  Ingress echoserver/echoserver
	  3m            32s             3       ingress-controller                      Normal          UPDATE  Ingress echoserver/echoserver
	```

	The Address seen above is the ALB's DNS record. This will be referenced via records created by external-dns.

1. Ensure your instance has the correct IAM permission required for external-dns. See https://github.com/kubernetes-incubator/external-dns/blob/master/docs/tutorials/aws.md#iam-permissions.

1. Download external-dns to manage Route 53.

	```bash
	$ wget https://raw.githubusercontent.com/coreos/alb-ingress-controller/master/examples/external-dns.yaml
	```

1. Edit the `--domain-filter` flag to include your hosted zone(s)

	The following example is for a hosted zone test-dns.com

	```yaml
	args:
	- --source=service
	- --source=ingress
	- --domain-filter=test-dns.com # will make ExternalDNS see only the hosted zones matching provided domain, omit to process all available hosted zones
	- --provider=aws
	- --policy=upsert-only # would prevent ExternalDNS from deleting any records, omit to enable full synchronization
	```

1. Verify the DNS has propogated

	```bash
	dig echoserver.josh-test-dns.com

	;; QUESTION SECTION:
	;echoserver.josh-test-dns.com.  IN      A

	;; ANSWER SECTION:
	echoserver.josh-test-dns.com. 60 IN     A       13.59.147.105
	echoserver.josh-test-dns.com. 60 IN     A       18.221.65.39
	echoserver.josh-test-dns.com. 60 IN     A       52.15.186.25
	```

1. Once it has, you can make a call to echoserver and it should return a response payload.

	```
	$ curl echoserver.josh-test-dns.com

	CLIENT VALUES:
	client_address=10.0.50.185
	command=GET
	real path=/
	query=nil
	request_version=1.1
	request_uri=http://echoserver.josh-test-dns.com:8080/

	SERVER VALUES:
	server_version=nginx: 1.10.0 - lua: 10001

	HEADERS RECEIVED:
	accept=*/*
	host=echoserver.josh-test-dns.com
	user-agent=curl/7.54.0
	x-amzn-trace-id=Root=1-59c08da5-113347df69640735312371bd
	x-forwarded-for=67.173.237.250
	x-forwarded-port=80
	x-forwarded-proto=http
	BODY:
	```
