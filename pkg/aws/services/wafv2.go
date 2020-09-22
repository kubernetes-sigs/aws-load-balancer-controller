package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafv2"
	"github.com/aws/aws-sdk-go/service/wafv2/wafv2iface"
)

type WAFv2 interface {
	wafv2iface.WAFV2API
}

// NewWAFv2 constructs new WAFv2 implementation.
func NewWAFv2(session *session.Session) WAFv2 {
	return &defaultWAFv2{
		WAFV2API: wafv2.New(session),
	}
}

// default implementation for WAFv2.
type defaultWAFv2 struct {
	wafv2iface.WAFV2API
}
