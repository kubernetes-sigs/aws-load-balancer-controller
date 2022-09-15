# Setup External DNS
[external-dns](https://github.com/kubernetes-incubator/external-dns) provisions DNS records based on the host information. This project will setup and manage records in Route 53 that point to controller deployed ALBs.

## Prerequisites
### Role Permissions
Adequate roles and policies must be configured in AWS and available to the node(s) running the external-dns. See https://github.com/kubernetes-incubator/external-dns/blob/master/docs/tutorials/aws.md#iam-permissions.

## Installation
1. Download sample `external-dns` manifest

    ```bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.0.0/docs/examples/external-dns.yaml
    ```

2. Edit the `--domain-filter` flag to include your hosted zone(s)

    The following example is for a hosted zone `test-dns.com`:

    ```yaml
    args:
    - --source=service
    - --source=ingress
    - --domain-filter=test-dns.com # will make ExternalDNS see only the hosted zones matching provided domain, omit to process all available hosted zones
    - --provider=aws
    - --policy=upsert-only # would prevent ExternalDNS from deleting any records, omit to enable full synchronization
    - --aws-zone-type=public # only look at public hosted zones (valid values are public, private or no value for both)
    - --registry=txt
    - --txt-owner-id=my-identifier
    ```

3. Deploy external-dns

    ```bash
    kubectl apply -f external-dns.yaml
    ```

4. Verify it deployed successfully.

    ```bash
    kubectl logs -f $(kubectl get po | egrep -o 'external-dns[A-Za-z0-9-]+')
    ```

    Should display output similar to the following:
    ```
    time="2019-12-11T10:26:05Z" level=info msg="config: {Master: KubeConfig: RequestTimeout:30s IstioIngressGateway:istio-system/istio-ingressgateway Sources:[service ingress] Namespace: AnnotationFilter: FQDNTemplate: CombineFQDNAndAnnotation:false Compatibility: PublishInternal:false PublishHostIP:false ConnectorSourceServer:localhost:8080 Provider:aws GoogleProject: DomainFilter:[test-dns.com] ZoneIDFilter:[] AlibabaCloudConfigFile:/etc/kubernetes/alibaba-cloud.json AlibabaCloudZoneType: AWSZoneType:public AWSAssumeRole: AWSBatchChangeSize:4000 AWSBatchChangeInterval:1s AWSEvaluateTargetHealth:true AzureConfigFile:/etc/kubernetes/azure.json AzureResourceGroup: CloudflareProxied:false InfobloxGridHost: InfobloxWapiPort:443 InfobloxWapiUsername:admin InfobloxWapiPassword: InfobloxWapiVersion:2.3.1 InfobloxSSLVerify:true DynCustomerName: DynUsername: DynPassword: DynMinTTLSeconds:0 OCIConfigFile:/etc/kubernetes/oci.yaml InMemoryZones:[] PDNSServer:http://localhost:8081 PDNSAPIKey: PDNSTLSEnabled:false TLSCA: TLSClientCert: TLSClientCertKey: Policy:upsert-only Registry:txt TXTOwnerID:my-identifier TXTPrefix: Interval:1m0s Once:false DryRun:false LogFormat:text MetricsAddress::7979 LogLevel:info TXTCacheInterval:0s ExoscaleEndpoint:https://api.exoscale.ch/dns ExoscaleAPIKey: ExoscaleAPISecret: CRDSourceAPIVersion:externaldns.k8s.io/v1alpha CRDSourceKind:DNSEndpoint ServiceTypeFilter:[] RFC2136Host: RFC2136Port:0 RFC2136Zone: RFC2136Insecure:false RFC2136TSIGKeyName: RFC2136TSIGSecret: RFC2136TSIGSecretAlg: RFC2136TAXFR:false}"
    time="2019-12-11T10:26:05Z" level=info msg="Created Kubernetes client https://10.100.0.1:443"
    ```

## Usage
1. To create a record set in the subdomain, from your ingress which has been created by the ingress-controller, simply add the following annotation in the ingress object specification and apply the manifest:

    ```yaml
    annotations:
      kubernetes.io/ingress.class: alb
      alb.ingress.kubernetes.io/scheme: internet-facing

      # for creating record-set
      external-dns.alpha.kubernetes.io/hostname: my-app.test-dns.com # give your domain name here
    ```

2. Similar entries should appear in the ExternalDNS pod log:

    ```
    time="2019-12-11T10:26:08Z" level=info msg="Desired change: CREATE my-app.test-dns.com A"
    time="2019-12-11T10:26:08Z" level=info msg="Desired change: CREATE my-app.test-dns.com TXT"
    time="2019-12-11T10:26:08Z" level=info msg="2 record(s) in zone my-app.test-dns.com. were successfully updated"
    ```
