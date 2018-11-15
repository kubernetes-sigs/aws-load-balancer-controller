# Setup External DNS
[external-dns](https://github.com/kubernetes-incubator/external-dns) provisions DNS records based on the host information. This project will setup and manage records in Route 53 that point to controller deployed ALBs.

## Prerequisites
### Role Permissions
Adequate roles and policies must be configured in AWS and available to the node(s) running the external-dns. See https://github.com/kubernetes-incubator/external-dns/blob/master/docs/tutorials/aws.md#iam-permissions.

## Installation
1. Download sample external-dns manifest
   
    ``` bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-alb-ingress-controller/master/docs/examples/external-dns.yaml
    ```

2. Edit the `--domain-filter` flag to include your hosted zone(s)

    The following example is for a hosted zone test-dns.com

    ```yaml
    args:
    - --source=service
    - --source=ingress
    - --domain-filter=test-dns.com # will make ExternalDNS see only the hosted zones matching provided domain, omit to process all available hosted zones
    - --provider=aws
    - --policy=upsert-only # would prevent ExternalDNS from deleting any records, omit to enable full synchronization
    ```

3. Deploy external-dns

    ``` bash
    kubectl apply -f external-dns.yaml
    ```

4. Verify it deployed successfully.

    ``` bash
    kubectl logs -f -n kube-system $(kubectl get po -n kube-system | egrep -o 'external-dns[A-Za-z0-9-]+')
    ```

    Should display output similar to the following.
    ```
    time="2017-09-19T02:51:54Z" level=info msg="config: &{Master: KubeConfig: Sources:[service ingress] Namespace: FQDNTemplate: Compatibility: Provider:aws GoogleProject: DomainFilter:[] AzureConfigFile:/etc/kuberne tes/azure.json AzureResourceGroup: Policy:upsert-only Registry:txt TXTOwnerID:my-identifier TXTPrefix: Interval:1m0s Once:false DryRun:false LogFormat:text MetricsAddress::7979 Debug:false}"
    time="2017-09-19T02:51:54Z" level=info msg="Connected to cluster at https://10.3.0.1:443"
    ```