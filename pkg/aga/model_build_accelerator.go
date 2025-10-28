package aga

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"k8s.io/apimachinery/pkg/util/sets"
	"regexp"
	agaapi "sigs.k8s.io/aws-load-balancer-controller/apis/aga/v1beta1"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/deploy/tracking"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/k8s"
	agamodel "sigs.k8s.io/aws-load-balancer-controller/pkg/model/aga"
	"sigs.k8s.io/aws-load-balancer-controller/pkg/model/core"
)

var invalidAcceleratorNamePattern = regexp.MustCompile("[^a-zA-Z0-9_-]")

// acceleratorBuilder builds Accelerator model resources
type acceleratorBuilder interface {
	Build(ctx context.Context, stack core.Stack, ga *agaapi.GlobalAccelerator) (*agamodel.Accelerator, error)
}

// NewAcceleratorBuilder constructs new acceleratorBuilder
func NewAcceleratorBuilder(trackingProvider tracking.Provider, clusterName string, defaultTags map[string]string, externalManagedTags []string, additionalTagsOverrideDefaultTags bool) acceleratorBuilder {
	externalManagedTagsSet := sets.New(externalManagedTags...)
	tagHelper := newTagHelper(externalManagedTagsSet, defaultTags, additionalTagsOverrideDefaultTags)

	return &defaultAcceleratorBuilder{
		trackingProvider: trackingProvider,
		clusterName:      clusterName,
		tagHelper:        tagHelper,
	}
}

var _ acceleratorBuilder = &defaultAcceleratorBuilder{}

type defaultAcceleratorBuilder struct {
	trackingProvider tracking.Provider
	clusterName      string
	tagHelper        tagHelper
}

func (b *defaultAcceleratorBuilder) Build(ctx context.Context, stack core.Stack, ga *agaapi.GlobalAccelerator) (*agamodel.Accelerator, error) {
	spec, err := b.buildAcceleratorSpec(ctx, stack, ga)
	if err != nil {
		return nil, err
	}

	accelerator := agamodel.NewAccelerator(stack, agamodel.ResourceIDAccelerator, spec)
	return accelerator, nil
}

func (b *defaultAcceleratorBuilder) buildAcceleratorSpec(ctx context.Context, stack core.Stack, ga *agaapi.GlobalAccelerator) (agamodel.AcceleratorSpec, error) {
	ipAddressType := b.buildAcceleratorIPAddressType(ctx, ga)

	name, err := b.buildAcceleratorName(ctx, ga, ipAddressType)
	if err != nil {
		return agamodel.AcceleratorSpec{}, err
	}

	ipAddresses := b.buildAcceleratorIPAddresses(ctx, ga)

	tags, err := b.buildAcceleratorTags(ctx, stack, ga)
	if err != nil {
		return agamodel.AcceleratorSpec{}, err
	}

	return agamodel.AcceleratorSpec{
		Name:          name,
		Enabled:       awssdk.Bool(true), // Controller always creates enabled accelerator
		IpAddresses:   ipAddresses,
		IPAddressType: ipAddressType,
		Tags:          tags,
	}, nil
}

func (b *defaultAcceleratorBuilder) buildAcceleratorName(_ context.Context, ga *agaapi.GlobalAccelerator, ipAddressType agamodel.IPAddressType) (string, error) {
	if ga.Spec.Name != nil {
		return *ga.Spec.Name, nil
	}

	// Generate unique name using SHA256 hash
	gaKey := k8s.NamespacedName(ga)

	uuidHash := sha256.New()
	_, _ = uuidHash.Write([]byte(b.clusterName))
	_, _ = uuidHash.Write([]byte(gaKey.Namespace))
	_, _ = uuidHash.Write([]byte(gaKey.Name))
	_, _ = uuidHash.Write([]byte(string(ipAddressType)))
	uuid := hex.EncodeToString(uuidHash.Sum(nil))

	sanitizedNamespace := invalidAcceleratorNamePattern.ReplaceAllString(gaKey.Namespace, "")
	sanitizedName := invalidAcceleratorNamePattern.ReplaceAllString(gaKey.Name, "")

	// AWS Global Accelerator name constraints: 1-64 characters, alphanumeric, underscores, and hyphens
	return fmt.Sprintf("k8s_%.16s_%.16s_%.25s", sanitizedNamespace, sanitizedName, uuid), nil
}

func (b *defaultAcceleratorBuilder) buildAcceleratorIPAddresses(_ context.Context, ga *agaapi.GlobalAccelerator) []string {
	if ga.Spec.IpAddresses != nil {
		return *ga.Spec.IpAddresses
	}

	// Return nil if not specified (AWS will assign automatically)
	return nil
}

func (b *defaultAcceleratorBuilder) buildAcceleratorIPAddressType(_ context.Context, ga *agaapi.GlobalAccelerator) agamodel.IPAddressType {
	switch ga.Spec.IPAddressType {
	case agaapi.IPAddressTypeIPV4:
		return agamodel.IPAddressTypeIPV4
	case agaapi.IPAddressTypeDualStack:
		return agamodel.IPAddressTypeDualStack
	default:
		// Default to IPv4
		return agamodel.IPAddressTypeIPV4
	}
}

func (b *defaultAcceleratorBuilder) buildAcceleratorTags(_ context.Context, stack core.Stack, ga *agaapi.GlobalAccelerator) (map[string]string, error) {
	// Get tags from tag helper (includes default tags and user-specified tags)
	tags, err := b.tagHelper.getAcceleratorTags(*ga)
	if err != nil {
		return nil, err
	}

	// Add tracking tags (includes cluster tag and stack tag)
	trackingTags := b.trackingProvider.StackTags(stack)
	for k, v := range trackingTags {
		tags[k] = v
	}

	// Add resource ID tag manually since we don't have the resource object yet
	tags[b.trackingProvider.ResourceIDTagKey()] = agamodel.ResourceIDAccelerator

	return tags, nil
}
