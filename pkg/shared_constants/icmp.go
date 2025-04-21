package shared_constants

const (
	ICMPV4Protocol = "icmp"
	ICMPV6Protocol = "icmpv6"

	ICMPV4CodeForPathMtu = 3 // https://www.iana.org/assignments/icmp-parameters/icmp-parameters.xhtml#icmp-parameters-codes-3
	ICMPV6CodeForPathMtu = 4

	ICMPV4TypeForPathMtu = 2 // https://www.iana.org/assignments/icmpv6-parameters/icmpv6-parameters.xhtml#icmpv6-parameters-codes-2
	ICMPV6TypeForPathMtu = 0
)
