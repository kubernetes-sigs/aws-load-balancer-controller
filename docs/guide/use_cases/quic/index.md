## Background

QUIC is not an acronym; it is a name given to the protocol. The original QUIC draft was proposed by Google in 12 2012. 
An IETF committee made a significant number of changes over the years and the evolved draft became an IETF standard [RFC 9000](https://datatracker.ietf.org/doc/html/rfc9000) in May 2021. 
Unlike existing networking standards that are written for static nodes, QUIC is written fundamentally for mobile nodes. In the “mobile first” world, 
user sessions:

- must be processed with lowest latency
- must have data security
- must be resilient to changes in underlying IP addresses or port numbers.


QUIC takes a new approach to achieve these diverse goals, and combines UDP, TLS1.3, HTTP/3 and congestion control elements, all into one single QUIC protocol stack. 
QUIC reduces latency by minimizing handshakes and by reducing packet roundtrips. Despite being UDP-based, it ensures reliable data delivery and congestion control. 
QUIC provides security by encrypting data with TLS1.3. Finally, it uses a unique Connection ID to ensure that the packets for a QUIC connection continue to be delivered to the endpoint, even when the IP addresses and port numbers change in the transport layer.

## Solution Overview

This solution will:

- Enable a namespace to be QUIC supported
- Set up an NLB with a QUIC listener and target group.
- Set up an example QUIC server
- Make QUIC traffic flow from a QUIC client to the sample QUIC server.

## Prerequisites

✅  Kubernetes Cluster Provisioned

✅  [AWS Load Balancer Controller](https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/installation/) Installed

✅ [Curl with HTTP3 Support](https://curl.se/docs/http3.html) Installed

## Configure

### Configure Namespace

First, we will create a namespace to house our NLB and QUIC application.

```
kubectl create ns quic-example
kubectl label namespace quic-example elbv2.k8s.aws/quic-server-id-inject=enabled
```

By labeling our namespace with `elbv2.k8s.aws/quic-server-id-inject=enabled` we are telling the LBC to attempt
server id injections into our pods environment variables in the `quic-example` namespace.  
Note that only pods annotated with `service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers` will
have server ids injected.

### Deploying a QUIC enabled application

In this example, we are using [Envoy](https://www.envoyproxy.io/) to host our QUIC server.
NOTE: This is only a demonstration and is insecure.


#### Create Envoy Config in ConfigMap
```
apiVersion: v1
data:
  envoy.yaml: |
    static_resources:
      listeners:
      - name: listener_0
        address:
          socket_address:
            protocol: TCP
            address: 0.0.0.0
            port_value: 3000
        filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificates:
                - certificate_chain:
                    filename: /etc/envoy/secrets/tls.crt
                  private_key:
                    filename: /etc/envoy/secrets/tls.key
                alpn_protocols: ["h2", "http/1.1"]
          filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              stat_prefix: ingress_http2
              http2_protocol_options: {}
              route_config:
                name: local_route
                virtual_hosts:
                - name: local_service
                  domains: ["*"]
                  routes:
                  - match:
                      prefix: "/"
                    direct_response:
                      status: 200
                      body:
                        inline_string: "h2 success!"
              http_filters:
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
      - name: listener_udp
        address:
          socket_address:
            protocol: UDP
            address: 0.0.0.0
            port_value: 3000
        udp_listener_config:
          quic_options:
            connection_id_generator_config:
              name: envoy.quic.connection_id_generator.quic_lb
              typed_config:
                '@type': type.googleapis.com/envoy.extensions.quic.connection_id_generator.quic_lb.v3.Config
                unsafe_unencrypted_testing_mode: true
                server_id_base64_encoded: true
                server_id:
                  environment_variable: AWS_LBC_QUIC_SERVER_ID
                nonce_length_bytes: 10
                encryption_parameters:
                  name: quic_lb
                  sds_config:
                    path_config_source:
                      path: /etc/quic-lb/quic_lb_key.yaml
          downstream_socket_config:
            prefer_gro: true
        filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.quic
            typed_config:
              '@type': type.googleapis.com/envoy.extensions.transport_sockets.quic.v3.QuicDownstreamTransport
              downstream_tls_context:
                common_tls_context:
                  tls_certificates:
                  - certificate_chain:
                      filename: /etc/envoy/secrets/tls.crt
                    private_key:
                      filename: /etc/envoy/secrets/tls.key
                  alpn_protocols: ["h3"]
          filters:
          - name: envoy.filters.network.http_connection_manager
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
              codec_type: HTTP3
              stat_prefix: ingress_http
              route_config:
                name: local_route
                virtual_hosts:
                - name: local_service
                  domains: ["*"]
                  routes:
                  - match:
                      prefix: "/"
                    direct_response:
                      status: 200
                      body:
                        inline_string: |
                          h3 success!
              http3_protocol_options:
              http_filters:
              - name: envoy.filters.http.router
                typed_config:
                  "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
    admin:
      address:
        socket_address:
          address: 127.0.0.1
          port_value: 9901
    node:
      id: unused
      cluster: unused
kind: ConfigMap
metadata:
  name: envoy-config
  namespace: quic-example
```

#### Create a fake encryption key

```
apiVersion: v1
data:
  quic_lb_key.yaml: |
    resources:
      - "@type": "type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.Secret"
        name: quic_lb
        generic_secret:
          secrets:
            encryption_key:
              inline_string: "0000000000000000"
            configuration_version:
              # 0, base64 encoded
              inline_bytes: AA==
kind: ConfigMap
metadata:
  name: quic-lb-key
  namespace: quic-example
```

#### Generate a self-signed cert and insert it into a secret

`openssl req -new -x509 -key private_key.pem -out public_certificate.pem -days 365`
`kubectl -n quic-example create secret tls quic-cert --cert=public_certificate.pem --key=private_key.pem`


#### Deploy Envoy with QUIC enabled server.

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: envoy
  namespace: quic-example
spec:
  progressDeadlineSeconds: 600
  replicas: 10
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: envoy-app
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers: envoy-sidecar
      creationTimestamp: null
      labels:
        app: envoy-app
    spec:
      containers:
      - args:
        - --config-path
        - /etc/envoy/envoy.yaml
        image: envoyproxy/envoy:dev
        imagePullPolicy: Always
        name: envoy-sidecar
        ports:
        - containerPort: 3000
          name: udp
          protocol: UDP
        - containerPort: 3000
          name: tcp
          protocol: TCP
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /etc/envoy
          name: envoy-config-volume
        - mountPath: /etc/quic-lb
          name: quic-lb-config-volume
        - mountPath: /etc/envoy/secrets
          name: quic-cert
      dnsPolicy: ClusterFirst
      nodeSelector:
        topology.kubernetes.io/zone: us-east-1d
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext:
        sysctls:
        - name: net.ipv4.ip_unprivileged_port_start
          value: "0"
      terminationGracePeriodSeconds: 30
      volumes:
      - configMap:
          defaultMode: 420
          name: envoy-config
        name: envoy-config-volume
      - configMap:
          defaultMode: 420
          name: quic-lb-key
        name: quic-lb-config-volume
      - name: quic-cert
        secret:
          defaultMode: 420
          secretName: quic-cert
```


The pod spec given in the deployment specifies an annotation:
`service.beta.kubernetes.io/aws-load-balancer-quic-enabled-containers: envoy-sidecar`

Which tells the LBC to inject a Server ID into the container environment. The envoy config is specified to look for
the Server ID under the environment variable `AWS_LBC_QUIC_SERVER_ID`.

You can change the environment variable name by changing the controller flag `quic-environment-variable-name`


After creating the deployment, validate the LBC injected the Server ID into each pod:
`kubectl -n quic-example get po -o yaml | grep 'AWS_LBC_QUIC_SERVER_ID' | wc -l`

5. Create the QUIC enabled NLB

```
apiVersion: v1
kind: Service
metadata:
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-attributes: load_balancing.cross_zone.enabled=true
    service.beta.kubernetes.io/aws-load-balancer-disable-nlb-sg: "true"
    service.beta.kubernetes.io/aws-load-balancer-enable-tcp-udp-listener: "true"
    service.beta.kubernetes.io/aws-load-balancer-name: quic-example
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip
    service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports: "443"
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
    service.beta.kubernetes.io/aws-load-balancer-subnets: subnet-0216f01774539f74c
  name: quic-service
  namespace: quic-example
spec:
  allocateLoadBalancerNodePorts: true
  externalTrafficPolicy: Cluster
  internalTrafficPolicy: Cluster
  ipFamilies:
  - IPv4
  ipFamilyPolicy: SingleStack
  loadBalancerClass: service.k8s.aws/nlb
  ports:
  - name: http3
    port: 443
    protocol: UDP
    targetPort: 3000
  - name: http2
    port: 443
    protocol: TCP
    targetPort: 3000
  selector:
    app: envoy-app
  sessionAffinity: None
  type: LoadBalancer
```

Annotations used:

- `service.beta.kubernetes.io/aws-load-balancer-disable-nlb-sg` Used to disable Frontend SG creation, as QUIC is does not work NLB SG.
- `service.beta.kubernetes.io/aws-load-balancer-enable-tcp-udp-listener` Allows the controller to combine the UDP and TCP port found on port 443 into a single NLB listener.
- `service.beta.kubernetes.io/aws-load-balancer-quic-enabled-ports` Upgrades the UDP protocol to QUIC.

## Verify

Retrieve the NLB DNS name:
`kubectl -n quic-example get svc quic-service -o yaml | grep 'hostname:'`

Verify HTTP2 (TCP) connectivity works:
```
curl --http2 https://$LB_DNS -k
h2 success!
```


Verify HTTP3 (UDP) connectivity works:
```
curl --http3 https://$LB_DNS -k
h3 success!
```