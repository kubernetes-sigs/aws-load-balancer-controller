package v1alpha1

import "github.com/pkg/errors"

type Protocol string

const (
	ProtocolHTTP  Protocol = "HTTP"
	ProtocolHTTPS          = "HTTPS"
	ProtocolTCP            = "TCP"
	ProtocolTLS            = "TLS"
)

func ParseProtocol(protocol string) (Protocol, error) {
	switch protocol {
	case string(ProtocolHTTP):
		return ProtocolHTTP, nil
	case string(ProtocolHTTPS):
		return ProtocolHTTPS, nil
	case string(ProtocolTCP):
		return ProtocolTCP, nil
	case string(ProtocolTLS):
		return ProtocolTLS, nil
	}
	return Protocol(""), errors.Errorf("unknown protocol: %v", protocol)
}

func (protocol Protocol) String() string {
	return string(protocol)
}
