package shared_constants

const (
	ICMPV4Protocol = "icmp"
	ICMPV6Protocol = "icmpv6"

	// ICMPv4 Type 3 (Destination Unreachable), Code 4 (Fragmentation Needed and Don't Fragment was Set)
	// https://www.iana.org/assignments/icmp-parameters/icmp-parameters.xhtml#icmp-parameters-codes-3
	ICMPV4TypeForPathMtu int32 = 3
	ICMPV4CodeForPathMtu int32 = 4

	// ICMPv6 Type 2 (Packet Too Big), Code 0
	// https://www.iana.org/assignments/icmpv6-parameters/icmpv6-parameters.xhtml#icmpv6-parameters-codes-2
	ICMPV6TypeForPathMtu int32 = 2
	ICMPV6CodeForPathMtu int32 = 0
)
