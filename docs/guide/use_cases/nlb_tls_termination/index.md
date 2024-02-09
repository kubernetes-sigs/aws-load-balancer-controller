---
title: TLS Termination with Network Load Balancer
---

## Motivation

![diagram illustrating connection between network load balancer and cluster](fig.jpg)

Managing TLS certificates (and related configuration) for production cluster workloads is both time consuming, and high risk. For example, storing multiple copies of a certificate secret key in the cluster may increases the chances of it being compromised. Additionally, TLS can be complicated to configure and implement properly. 

Traditionally, TLS termination at the load balancer step required using more expensive application load balancers (ALBs). AWS introduced TLS termination for network load balancers (NLBs) for enhanced security and cost effectiveness. 

The TLS implementation used by the AWS NLB is formally verified and maintained. Additionally, AWS Certificate Manager (ACM) is used, fully isolating your cluster from access to the private key. 

## Solution Overview

An external client transmits a request to the NLB. The request is encrypted with TLS using the production (e.g., client facing) certificate, and on port 443. 

The NLB decrypts the request, and transmits it on to your cluster on port 80. It follows the standard request routing configured within the cluster. Notably, the request received within the cluster includes the actual origin IP address of the external client. Alternate ports may be configured.  

!!! note
    The NLB may be configured to maintain the source (i.e., client) IP address. However, there are some limitations. Review [Client IP Preservation](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#client-ip-preservation) in the AWS docs. 

## Prerequisites

✅ Access to DNS records for domain name.

[Review the docs on registering domains with AWS's Route 53.](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/registrar.html) Alternate DNS providers may be used, such as Google Domains or Namecheap.

Later, a subdomain (e.g., demo-service.gcline.us) will be created, pointing to the NLB. Access to the DNS records is required to generate a TLS certificate for use by the NLB. 

✅  [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/installation/) Installed 

Generally, setting up the Load Balancer Controller has two steps: enabling IAM roles for service accounts, and adding the controller to the cluster. The IAM role allows the controller in the Kubernetes cluster to manage AWS resources. [Learn more about IAM roles for service accounts.](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)

## Configure

### Generate TLS Certificate

Create a public TLS certificate for the domain using AWS Certificate Manager (ACM). This is streamlined when the domain is managed by Route 53. Review the [AWS Certificate Manager Docs.](https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-request-public.html#request-public-console)

The domain name on the TLS certificate must correspond to the planned domain name for the kubernetes service. The domain name may be specified explicitly (e.g., tls-demo.gcline.us), or a wildcard certificate can be used (e.g., *.gcline.us).

If the domain is registered with Route53, the TLS certificate request will automatically be approved. Otherwise, follow ACM console the instructions to create a DNS record to validate the domain. 

After validation, the certificate will be available for use in your AWS account. 

Note the ARN of the certificate, which uniquely identifies it in kubernetes config files. 

![screenshot indicating location of ARN value in web console](cert_arn.png)

### Create Service with new NLB

Add annotations to a load balancer service to enable NLB TLS termination, before the traffic reaches Envoy. The annotations are actioned by the load balancer controller. [Review all the NLB annotations on GitHub.](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/service/annotations/)

| annotation name | value | meaning | 
| ----- | --- | ----- |
| service.beta.kubernetes.io/aws-load-balancer-type | external | explicitly requires an NLB, instead of an ALB |
| service.beta.kubernetes.io/aws-load-balancer-nlb-target-type | ip | route traffic directly to the pod IP |
| service.beta.kubernetes.io/aws-load-balancer-scheme | internet-facing | An internet-facing load balancer has a publicly resolvable DNS name |
| service.beta.kubernetes.io/aws-load-balancer-ssl-cert | "arn:aws:acm:..." | identifies the TLS certificate used by the NLB |
| service.beta.kubernetes.io/aws-load-balancer-ssl-ports | 443 | determines the port the NLB should listen for TLS traffic on| 

Example: 

```
apiVersion: v1
kind: Service
metadata:
  name: MyAppSvc
  namespace: dev
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: external
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
    service.beta.kubernetes.io/aws-load-balancer-ssl-cert: "arn:aws:acm:us-east-2:185309785115:certificate/7610ed7d-5a81-4ea2-a18a-7ba1606cca3e"
    service.beta.kubernetes.io/aws-load-balancer-ssl-ports: "443"
spec:
  externalTrafficPolicy: Local
  ports:
  - port: 443
    targetPort: 80
    name: http
    protocol: TCP
  selector:
    app: MyApp
  type: LoadBalancer
``` 

### Configure DNS

**Get domain name using kubectl.** 

The service name and namespace were defined above.

```
kubectl get svc MyAppSvc --namespace dev
```

```
NAME    TYPE           CLUSTER-IP      EXTERNAL-IP                                                                     PORT(S)        AGE
envoy   LoadBalancer   10.100.24.154   k8s-<namespace>-<service_name>-xxxxxxxxxx-xxxxxxxxxxxxxxxx.elb.<region_code>.amazonaws.com   443:31606/TCP   40d
```

Note the last 4 digits of the domain name for the NLB. For example, "bb1f". 

**Setup DNS alias for NLB**

Create a DNS record pointing from a friendly name (e.g., tls-demo.gcline.us) to the NLB domain (e.g., bb1f.elb.us-east-2.amazonaws.com). 

For AWS's Route 53, follow the instructions below. If you use a different DNS provider, follow their instructions for [creating a CNAME record](https://docs.digitalocean.com/products/networking/dns/how-to/manage-records/#cname-records). 

First, create a new record in Route 53. 

Use the "A" record type, and enable the ["alias" option.](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resource-record-sets-values-alias.html) This option attaches the DNS record to the AWS resource, without requiring an extra lookup step for clients. 

Select the NLB resource. Double check the region, and use the last 4 digits (noted earlier) to select the proper resource. 

![screenshot of Route 53 New Record Console](dns_record.png)

## Verify

Attempt to access the NLB domain at port 443 with HTTPS/TLS. Is the connection successful? What certificate is used? Does it reach the expected endpoint within the cluster? 
