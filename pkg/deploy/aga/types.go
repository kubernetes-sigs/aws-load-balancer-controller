package aga

import (
	globalacceleratortypes "github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
)

// AcceleratorWithTags represents an AWS Global Accelerator with its associated tags.
type AcceleratorWithTags struct {
	Accelerator *globalacceleratortypes.Accelerator
	Tags        map[string]string
}

// ListenerResource represents an AWS Global Accelerator Listener.
type ListenerResource struct {
	Listener *globalacceleratortypes.Listener
}
