# walkthrough: echoserver

In this walkthrough, you'll

- Create a cluster with EKS
- Deploy an alb-ingress-controller
- Create deployments and ingress resources in the cluster
- Use [external-dns](https://github.com/kubernetes-incubator/external-dns) to create a DNS record
    - This assumes you have a route53 hosted zone available. Otherwise you can skip this, but you'll only be able to address the service from the ALB's DNS.

## Create the EKS cluster
1. Install `eksctl`: https://eksctl.io

2. Create EKS cluster via eksctl

    ```bash
    eksctl create cluster
    ```
    
    ```console
    2018-08-14T11:19:09-07:00 [ℹ]  setting availability zones to [us-west-2c us-west-2a us-west-2b]
    2018-08-14T11:19:09-07:00 [ℹ]  importing SSH public key "/Users/kamador/.ssh/id_rsa.pub" as "eksctl-exciting-gopher-1534270749-b7:71:da:f6:f3:63:7a:ee:ad:7a:10:37:28:ff:44:d1"
    2018-08-14T11:19:10-07:00 [ℹ]  creating EKS cluster "exciting-gopher-1534270749" in "us-west-2" region
    2018-08-14T11:19:10-07:00 [ℹ]  creating ServiceRole stack "EKS-exciting-gopher-1534270749-ServiceRole"
    2018-08-14T11:19:10-07:00 [ℹ]  creating VPC stack "EKS-exciting-gopher-1534270749-VPC"
    2018-08-14T11:19:50-07:00 [✔]  created ServiceRole stack "EKS-exciting-gopher-1534270749-ServiceRole"
    2018-08-14T11:20:30-07:00 [✔]  created VPC stack "EKS-exciting-gopher-1534270749-VPC"
    2018-08-14T11:20:30-07:00 [ℹ]  creating control plane "exciting-gopher-1534270749"
    2018-08-14T11:31:52-07:00 [✔]  created control plane "exciting-gopher-1534270749"
    2018-08-14T11:31:52-07:00 [ℹ]  creating DefaultNodeGroup stack "EKS-exciting-gopher-1534270749-DefaultNodeGroup"
    2018-08-14T11:35:33-07:00 [✔]  created DefaultNodeGroup stack "EKS-exciting-gopher-1534270749-DefaultNodeGroup"
    2018-08-14T11:35:33-07:00 [✔]  all EKS cluster "exciting-gopher-1534270749" resources has been created
    2018-08-14T11:35:33-07:00 [✔]  saved kubeconfig as "/Users/kamador/.kube/config"
    2018-08-14T11:35:34-07:00 [ℹ]  the cluster has 0 nodes
    2018-08-14T11:35:34-07:00 [ℹ]  waiting for at least 2 nodes to become ready
    2018-08-14T11:36:05-07:00 [ℹ]  the cluster has 2 nodes
    2018-08-14T11:36:05-07:00 [ℹ]  node "ip-192-168-139-176.us-west-2.compute.internal" is ready
    2018-08-14T11:36:05-07:00 [ℹ]  node "ip-192-168-214-126.us-west-2.compute.internal" is ready
    2018-08-14T11:36:05-07:00 [✔]  EKS cluster "exciting-gopher-1534270749" in "us-west-2" region is ready
    ```

## Deploy the ALB ingress controller
1. Download the example alb-ingress-manifest locally.

    ```bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/alb-ingress-controller.yaml
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/rbac-role.yaml
    ```

1. Edit the manifest and set the following parameters and environment variables.
    - `cluster-name`: name of the cluster.
    
    - `AWS_ACCESS_KEY_ID`: access key id that alb controller can use to communicate with AWS. This is only used for convenience of this example. It will keep the credentials in plain text within this manifest. It's recommended a project such as kube2iam is used to resolve access. **You will need to uncomment this from the manifest**.

        ``` yaml
        - name: AWS_ACCESS_KEY_ID
          value: KEYVALUE
        ```
    
    - `AWS_SECRET_ACCESS_KEY`: secret access key that alb controller can use to communicate with AWS. This is only used for convenience of this example. It will keep the credentials in plain text within this manifest. It's recommended a project such as kube2iam is used to resolve access. **You will need to uncomment this from the manifest**.

        ``` yaml
        - name: AWS_SECRET_ACCESS_KEY
          value: SECRETVALUE
        ```

1.  Deploy the modified alb-ingress-controller.

    ```bash
    kubectl apply -f rbac-role.yaml
    kubectl apply -f alb-ingress-controller.yaml
    ```
    
    > The manifest above will deploy the controller to the `kube-system` namespace.
    
1.  Verify the deployment was successful and the controller started.

    ```bash
    kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o alb-ingress[a-zA-Z0-9-]+)
    ```

    Should display output similar to the following.

    ```
    -------------------------------------------------------------------------------
    AWS ALB Ingress controller
    Release:    UNKNOWN
    Build:      UNKNOWN
    Repository: UNKNOWN
    -------------------------------------------------------------------------------

    I0725 11:22:06.464996   16433 main.go:159] Creating API client for http://localhost:8001
    I0725 11:22:06.563336   16433 main.go:203] Running in Kubernetes cluster version v1.8+ (v1.8.9+coreos.1) - git (clean) commit cd373fe93e046b0a0bc7e4045af1bf4171cea395 - platform linux/amd64
    I0725 11:22:06.566255   16433 alb.go:80] ALB resource names will be prefixed with 2f92da62
    I0725 11:22:06.645910   16433 alb.go:163] Starting AWS ALB Ingress controller
    ```
    
## Deploy the echoserver resources

1.  Deploy all the echoserver resources (namespace, service, deployment)

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/echoservice/echoserver-namespace.yaml &&\
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/echoservice/echoserver-service.yaml &&\
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/echoservice/echoserver-deployment.yaml
    ```

1.  List all the resources to ensure they were created.

    ```bash
    kubectl get -n echoserver deploy,svc
    ```

    Should resolve similar to the following.

    ```console
    NAME             CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
    svc/echoserver   10.3.31.76   <nodes>       80:31027/TCP   4d

    NAME                DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
    deploy/echoserver   1         1         1            1           4d
    ```

## Deploy ingress for echoserver

1.  Download the echoserver ingress manifest locally.

    ```bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/echoservice/echoserver-ingress.yaml
    ```

1.  Configure the subnets, either by add annotation to the ingress or add tags to subnets.
    
    !!!tip
        If you'd like to use external dns, alter the host field to a domain that you own in Route 53. Assuming you managed `example.com` in Route 53.

    - Edit the `alb.ingress.kubernetes.io/subnets` annotation to include at least two subnets.
        ```bash
        eksctl get cluster exciting-gopher-1534270749
        ```
    
        ```console
        NAME		                VERSION STATUS         CREATED			VPC						SUBNETS				                SECURITYGROUPS
        exciting-gopher-1534270749	1.10	ACTIVE	2018-08-14T18:20:32Z	vpc-0aa01b07b3c922c9c	subnet-05e1c98ed0f5b109e,subnet-07f5bb81f661df61b,subnet-0a4e6232630820516	sg-05ceb5eee9fd7cac4
        ```

        ```yaml
        apiVersion: extensions/v1beta1
        kind: Ingress
        metadata:
            name: echoserver
            namespace: echoserver
            annotations:
                alb.ingress.kubernetes.io/scheme: internet-facing
                alb.ingress.kubernetes.io/target-type: ip
                alb.ingress.kubernetes.io/subnets: subnet-05e1c98ed0f5b109e,subnet-07f5bb81f661df61b,subnet-0a4e6232630820516
                alb.ingress.kubernetes.io/tags: Environment=dev,Team=test
        spec:
            rules:
            - host: echoserver.example.com
                http:
                paths:
        ```

	- Adding tags to subnets for auto-discovery(instead of `alb.ingress.kubernetes.io/subnets` annotation)
	    
	    you must include the following tags on desired subnets.

	    - `kubernetes.io/cluster/$CLUSTER_NAME` where `$CLUSTER_NAME` is the same `CLUSTER_NAME` specified in the above step.
	    - `kubernetes.io/role/internal-elb` should be set to `1` or an empty tag value for internal load balancers.
	    - `kubernetes.io/role/elb` should be set to `1` or an empty tag value for internet-facing load balancers.

	    An example of a subnet with the correct tags for the cluster `joshcalico` is as follows.
	    
	    ![subnet-tags](../../imgs/subnet-tags.png)

1.  Deploy the ingress resource for echoserver

    ```bash
    kubectl apply -f echoserver-ingress.yaml
    ```

1.  Verify the alb-ingress-controller creates the resources

    ```bash
    kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o 'alb-ingress[a-zA-Z0-9-]+') | grep 'echoserver\/echoserver'
    ```

    You should see similar to the following.

    ```console
    echoserver/echoserver: Start ELBV2 (ALB) creation.
    echoserver/echoserver: Completed ELBV2 (ALB) creation. Name: joshcalico-echoserver-echo-2ad7 | ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:loadbalancer/app/joshcalico-echoserver-echo-2ad7/4579643c6f757be9
    echoserver/echoserver: Start TargetGroup creation.
    echoserver/echoserver: Succeeded TargetGroup creation. ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:targetgroup/joshcalico-31027-HTTP-6657576/77ef58891a00263e | Name: joshcalico-31027-HTTP-6657576.
    echoserver/echoserver: Start Listener creation.
    echoserver/echoserver: Completed Listener creation. ARN: arn:aws:elasticloadbalancing:us-east-2:432733164488:listener/app/joshcalico-echoserver-echo-2ad7/4579643c6f757be9/2b2987fa3739c062 | Port: 80 | Proto: HTTP.
    echoserver/echoserver: Start Rule creation.
    echoserver/echoserver: Completed Rule creation. Rule Priority: "1" | Condition: [{    Field: "host-header",    Values: ["echoserver.joshrosso.com"]  },{    Field: "path-pattern",    Values: ["/"]  }]
    ```

1.  Check the events of the ingress to see what has occur.

    ```bash
    kubectl describe ing -n echoserver echoserver
    ```

    You should see similar to the following.

    ```console
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

    The address seen above is the ALB's DNS record. This will be referenced via records created by external-dns.


## Setup external-DNS to manage DNS automatically
 
1.  Ensure your nodes (on which External DNS runs) have the correct IAM permission required for external-dns. See https://github.com/kubernetes-incubator/external-dns/blob/master/docs/tutorials/aws.md#iam-permissions.

1.  Download external-dns to manage Route 53.

    ```bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/external-dns.yaml
    ```

1.  Edit the `--domain-filter` flag to include your hosted zone(s)

    The following example is for a hosted zone test-dns.com

    ```yaml
    args:
    - --source=service
    - --source=ingress
    - --domain-filter=test-dns.com # will make ExternalDNS see only the hosted zones matching provided domain, omit to process all available hosted zones
    - --provider=aws
    - --policy=upsert-only # would prevent ExternalDNS from deleting any records, omit to enable full synchronization
    ```

1.  Deploy external-dns

    ```bash
    kubectl apply -f external-dns.yaml
    ```

1.  Verify the DNS has propagated

    ```bash
    dig echoserver.josh-test-dns.com
    ```
    
    ```console
    ;; QUESTION SECTION:
    ;echoserver.josh-test-dns.com.  IN      A

    ;; ANSWER SECTION:
    echoserver.josh-test-dns.com. 60 IN     A       13.59.147.105
    echoserver.josh-test-dns.com. 60 IN     A       18.221.65.39
    echoserver.josh-test-dns.com. 60 IN     A       52.15.186.25
    ```

1.  Once it has, you can make a call to echoserver and it should return a response payload.

    ```bash
    curl echoserver.josh-test-dns.com
    ```
    
    ```console
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

## Kube2iam setup
follow below steps if you want to use kube2iam to provide the AWS credentials

1. configure the proper policy
    The policy to be used can be fetched from https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/v1.1.4/docs/examples/iam-policy.json

1. configure the proper role and create the trust relationship
    You have to find which role is associated with your K8S nodes. Once you found take note of the full arn:

    ```
    arn:aws:iam::XXXXXXXXXXXX:role/k8scluster-node
    ```

1. create the role, called k8s-alb-controller, attach the above policy and add a Trust Relationship like:

    ```
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Sid": "",
          "Effect": "Allow",
          "Principal": {
            "Service": "ec2.amazonaws.com"
          },
          "Action": "sts:AssumeRole"
        },
        {
          "Sid": "",
          "Effect": "Allow",
          "Principal": {
            "AWS": "arn:aws:iam::XXXXXXXXXXXX:role/k8scluster-node"
          },
          "Action": "sts:AssumeRole"
        }
      ]
    }
    ```

    The new role will have a similar arn:

    ```
    arn:aws:iam:::XXXXXXXXXXXX:role/k8s-alb-controller
    ```

1.  update the alb-ingress-controller.yaml

    Add the annotations in the template's metadata point

    ```yaml
    spec:
    replicas: 1
    selector:
      matchLabels:
        app: alb-ingress-controller
    strategy:
      rollingUpdate:
        maxSurge: 1
        maxUnavailable: 1
      type: RollingUpdate
    template:
      metadata:
        annotations:
          iam.amazonaws.com/role: arn:aws:iam:::XXXXXXXXXXXX:role/k8s-alb-controller
    ```
