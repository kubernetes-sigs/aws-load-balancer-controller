package services

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafregional"
	"github.com/aws/aws-sdk-go/service/wafregional/wafregionaliface"
)

type WAFRegional interface {
	wafregionaliface.WAFRegionalAPI
}

// NewWAFRegional constructs new WAFRegional implementation.
func NewWAFRegional(session *session.Session) WAFRegional {
	return &defaultWAFRegional{
		WAFRegionalAPI: wafregional.New(session),
	}
}

// default implementation for WAFRegional.
type defaultWAFRegional struct {
	wafregionaliface.WAFRegionalAPI
}
