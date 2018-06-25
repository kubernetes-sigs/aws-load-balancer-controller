package tg

import (
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/aws/albelbv2"
	"github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/log"
	util "github.com/kubernetes-sigs/aws-alb-ingress-controller/pkg/util/types"
)

// TargetGroups is a slice of TargetGroups
type TargetGroups []*TargetGroup

// TargetGroup contains the current & desired configuration
type TargetGroup struct {
	ID      string
	SvcName string

	tg         tg
	attributes attributes
	tags       tags
	targets    targets

	deleted bool
	logger  *log.Logger
}

type tg struct {
	current *elbv2.TargetGroup
	desired *elbv2.TargetGroup
}

type attributes struct {
	current albelbv2.TargetGroupAttributes
	desired albelbv2.TargetGroupAttributes
}

type targets struct {
	current util.AWSStringSlice
	desired util.AWSStringSlice
}

type tags struct {
	current util.Tags
	desired util.Tags
}

type ReconcileOptions struct {
	Eventf            func(string, string, string, ...interface{})
	VpcID             *string
	ManagedSGInstance *string
}

type tgChange uint

const (
	paramsModified tgChange = 1 << iota
	targetsModified
	tagsModified
	attributesModified
)
