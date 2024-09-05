package ec2

type IPRange struct {
	CIDRIP string `json:"cidrIP"`
	// +optional
	Description string `json:"description,omitempty"`
}

type IPv6Range struct {
	CIDRIPv6 string `json:"cidrIPv6"`
	// +optional
	Description string `json:"description,omitempty"`
}

type UserIDGroupPair struct {
	GroupID string `json:"groupID"`
	// +optional
	Description string `json:"description,omitempty"`
}

type PrefixList struct {
	ListID string `json:"listID"`
	// +optional
	Description string `json:"description,omitempty"`
}

type IPPermission struct {
	IPProtocol string `json:"ipProtocol"`
	// +optional
	FromPort *int32 `json:"fromPort,omitempty"`
	// +optional
	ToPort *int32 `json:"toPort,omitempty"`
	// +optional
	IPRanges []IPRange `json:"ipRanges,omitempty"`
	// +optional
	IPv6Range []IPv6Range `json:"ipv6Ranges,omitempty"`
	// +optional
	UserIDGroupPairs []UserIDGroupPair `json:"userIDGroupPairs,omitempty"`
	// +optional
	PrefixLists []PrefixList `json:"prefixLists,omitempty"`
}
