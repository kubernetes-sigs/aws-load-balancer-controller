# Services of type ExternalName

If a an ingress refers to a service of type [ExternalName](https://kubernetes.io/docs/concepts/services-networking/service/#externalname) then the value of the `spec.externalName`
parameter will be used to query the DNS server for an A record. The result is then used to populate
the target group. Then target type will be `ip` and IP address type `ipv4`.


The normal conditions for such addresses in 
[such a target group](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-target-groups.html#target-group-ip-address-type) 
apply. 

Specifying `instance` as
[`target-type`](annotations.md#target-type) for an ingress referring to an external name service will
fail and an error will be logged.

The TTL of the DNS response is used to schedule the next DNS lookup. 10 secunds is the minimum
interval between DNS checks, which is also used when there is no result for a DNS query.    