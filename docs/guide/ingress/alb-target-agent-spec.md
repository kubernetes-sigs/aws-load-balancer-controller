# ALBTargetControlConfigSpec

ALBTargetControlConfigSpec defines the desired state of ALBTargetControlConfig

_Appears in:_
- [ALBTargetControlConfig](#albtargetcontrolconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image specifies the container image for the ALB target control agent sidecar. The agent is available as a Docker image at: public.ecr.aws/aws-elb/target-optimizer/target-control-agent:latest |  | Required: {} |
| `dataAddress` _string_ | DataAddress specifies the socket (IP:port) where the agent receives application traffic from the load balancer. The port in this socket is the application traffic port you configure for your target group |  | Pattern: `^.+:[0-9]+$`<br />Required: {} |
| `controlAddress` _string_ | ControlAddress specifies the socket (IP:port) where the load balancer exchanges management traffic with agents. The port in the socket is the target control port you configure for the target group |  | Pattern: `^.+:[0-9]+$`<br />Required: {} |
| `destinationAddress` _string_ | DestinationAddress specifies the socket (IP:port) where the agent proxies application traffic. Your application should be listening on this port |  | Pattern: `^.+:[0-9]+$`<br />Required: {} |
| `maxConcurrency` _integer_ | MaxConcurrency specifies the maximum number of concurrent requests that the target receives from the load balancer | 1 | Maximum: 1000<br />Minimum: 0 |
| `tlsCertPath` _string_ | TLSCertPath specifies the location of the TLS certificate that the agent provides to the load balancer during TLS handshake. By default, the agent generates a self-signed certificate in-memory |  |  |
| `tlsKeyPath` _string_ | TLSKeyPath specifies the location of the private key corresponding to the TLS certificate that the agent provides to the load balancer during TLS handshake. By default, the agent generates a private key in memory |  |  |
| `tlsSecurityPolicy` _string_ | TLSSecurityPolicy specifies the ELB security policy that you configure for the target group |  |  |
| `protocolVersion` _string_ | ProtocolVersion specifies the protocol through which the load balancer communicates with the agent. Possible values are HTTP1, HTTP2, GRPC |  | Enum: [HTTP1 HTTP2 GRPC] |
| `rustLog` _string_ | RustLog specifies the log level of the agent process. The agent software is written in Rust |  | Enum: [debug info error] |
| `resources` _ResourceRequirements_ | Resources specifies the resource requirements for the ALB target control agent sidecar |  |  |