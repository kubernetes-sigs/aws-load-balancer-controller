# walkthrough: grpcserver

In this walkthrough, you'll

- Deploy a grpc service to an existing EKS cluster
- Send a test message to the hosted service over TLS

## Prerequsites

The following resources are required prior to deployment:

- EKS cluster
- aws-load-balancer-controller
- external-dns

See [echo_server.md](echo_server.md) for setup instructions for those resources.

## Create an ACM certificate
> NOTE: An ACM certificate is required for this demo as the application uses the `grpc.secure_channel` method.

If you already have an ACM certificate (including wildcard certificates) for the domain you would like to use in this example, you can skip this step.

- Request a certificate for a domain you own using the steps described in the official AWS [documentation](https://docs.aws.amazon.com/acm/latest/userguide/gs-acm-request-public.html).
- Once the status for the certificate is "Issued" continue to the next step.

## Deploy the grpcserver manifests

1.  Deploy all the manifests from GitHub.

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/examples/grpc/grpcserver-namespace.yaml
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/examples/grpc/grpcserver-service.yaml
    kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/examples/grpc/grpcserver-deployment.yaml
    ```

1.  Confirm that all resources were created.

    ```bash
    kubectl get -n grpcserver all
    ```

    You should see the pod, service, and deployment.

    ```console
    NAME                             READY   STATUS    RESTARTS   AGE
    pod/grpcserver-5455b7d4d-jshk5   1/1     Running   0          35m

    NAME                 TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)     AGE
    service/grpcserver   ClusterIP   None         <none>        50051/TCP   77m

    NAME                         READY   UP-TO-DATE   AVAILABLE   AGE
    deployment.apps/grpcserver   1/1     1            1           77m

    NAME                                   DESIRED   CURRENT   READY   AGE
    replicaset.apps/grpcserver-5455b7d4d   1         1         1       35m
    ```

## Customize the ingress for grpcserver

1.  Download the grpcserver ingress manifest.

    ```bash
    wget https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/examples/grpc/grpcserver-ingress.yaml
    ```

1. Change the domain name from `grpcserver.example.com` to your desired domain.

> NOTE: This example manifest assumes that you have tagged your subnets for the aws-load-balancer-controller. Otherwise add your subnets using the annotations described in ingress annotations documentation.

1.  Deploy the ingress resource for grpcserver.

    ```bash
    kubectl apply -f grpcserver-ingress.yaml
    ```

1. Wait a few minutes for the ALB to provision and for DNS to update.

1.  Check the logs for `external-dns` and `aws-load-balancer-controller` to ensure the ALB is created and external-dns creates the record and points your domain to the ALB.

    ```bash
    kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o 'aws-load-balancer-controller[a-zA-Z0-9-]+') | grep 'grpcserver\/grpcserver'
    kubectl logs -n kube-system $(kubectl get po -n kube-system | egrep -o 'aws-load-balancer-controller[a-zA-Z0-9-]+') | grep 'YOUR_DOMAIN_NAME'
    ```

1.  Next check that your ingress shows the correct ALB address and custom domain name.

    ```bash
    kubectl get ingress -n grpcserver grpcserver
    ```

    You should see similar to the following.

    ```console
    NNAME         CLASS    HOSTS                       ADDRESS                                                                 PORTS   AGE
grpcserver   <none>   YOUR_DOMAIN_NAME   ALB-NAME.us-east-1.elb.amazonaws.com   80      90m
    ```

1. Finally, test your secure gRPC service by running the greeter client, substituting `YOUR_DOMAIN_NAME` for the domain you used in the ingress manifest.

    ```bash
    docker run --rm -it --env BACKEND=YOUR_DOMAIN_NAME placeexchange/grpc-demo:latest python greeter_client.py
    ```

    You should see the following response.
    ```console
    Greeter client received: Hello, you!
    ```
    
