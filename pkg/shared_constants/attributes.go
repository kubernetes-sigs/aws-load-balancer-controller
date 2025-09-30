package shared_constants

const (
	LBAttributeDeletionProtection                  = "deletion_protection.enabled"
	LBAttributeAccessLogsS3Enabled                 = "access_logs.s3.enabled"
	LBAttributeAccessLogsS3Bucket                  = "access_logs.s3.bucket"
	LBAttributeAccessLogsS3Prefix                  = "access_logs.s3.prefix"
	LBAttributeLoadBalancingCrossZoneEnabled       = "load_balancing.cross_zone.enabled"
	LBAttributeLoadBalancingDnsClientRoutingPolicy = "dns_record.client_routing_policy"

	LBAttributeAvailabilityZoneAffinity        = "availability_zone_affinity"
	LBAttributePartialAvailabilityZoneAffinity = "partial_availability_zone_affinity"
	LBAttributeAnyAvailabilityZone             = "any_availability_zone"
)

const (
	TGAttributeProxyProtocolV2Enabled  = "proxy_protocol_v2.enabled"
	TGAttributePreserveClientIPEnabled = "preserve_client_ip.enabled"
)
